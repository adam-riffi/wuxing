package kernel

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/adam-riffi/wuxing/internal/contracts/cfg"
	"github.com/adam-riffi/wuxing/internal/kernel/bus"
	"github.com/adam-riffi/wuxing/internal/kernel/triggers"
	"github.com/adam-riffi/wuxing/internal/storage"
)

func testKernel(t *testing.T) *Kernel {
	t.Helper()
	store, err := storage.OpenSQLite(filepath.Join(t.TempDir(), "wuxing.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return Assemble(store, 1<<30)
}

func TestKernel_Run_WritesSpineAndExecutesWorkflow(t *testing.T) {
	k := testKernel(t)
	ctx := context.Background()

	// A stub connectors tool that records it was called.
	var called int
	_ = k.Bus.Register("connectors", func(_ context.Context, e bus.Envelope) bus.Envelope {
		called++
		return e.Reply(json.RawMessage(`{"new_set":true}`))
	})

	// Register a service whose workflow calls connectors, with a successor.
	svc := &cfg.Service{
		Name:       "mtg",
		Envelope:   cfg.Envelope{Request: 1},
		Workflow:   []cfg.Step{{ID: "check", Tool: "connectors", Operation: "read"}},
		Successors: []cfg.Successor{{Service: "notifier", Topic: "mtg-db updated", When: "new_set == true"}},
	}
	if err := k.Library.Register(svc); err != nil {
		t.Fatal(err)
	}

	// An external (cron) trigger opens a new sequence and fires the run.
	r := k.Triggers.FireExternal("mtg", triggers.KindCron)

	fact, succ, err := k.Run(ctx, r)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if called != 1 {
		t.Errorf("workflow did not run the tool: called=%d", called)
	}
	if fact["new_set"] != true {
		t.Errorf("emitted fact: %+v", fact)
	}
	if len(succ) != 1 || succ[0].Service != "notifier" {
		t.Errorf("successors: %+v", succ)
	}

	// The spine recorded one sequence, run, and session under one sequence_id,
	// and the run/session were closed.
	seq := string(r.Stamp.Sequence)
	if n := spineCount(t, k, "wuxing_ft_sequence", seq); n != 1 {
		t.Errorf("sequence rows: got %d want 1", n)
	}
	if n := spineCount(t, k, "wuxing_ft_run", seq); n != 1 {
		t.Errorf("run rows: got %d want 1", n)
	}
	if n := spineCount(t, k, "wuxing_ft_session", seq); n != 1 {
		t.Errorf("session rows: got %d want 1", n)
	}

	var closed *string
	_ = k.Store.QueryRow(`SELECT closed_at FROM wuxing_ft_run WHERE sequence_id = ?`, seq).Scan(&closed)
	if closed == nil {
		t.Error("run was not closed")
	}

	// No live sessions remain after the run completes.
	if k.Sessions.Count() != 0 {
		t.Errorf("sessions still open: %d", k.Sessions.Count())
	}
}

func TestKernel_Run_UnknownService(t *testing.T) {
	k := testKernel(t)
	r := k.Triggers.FireExternal("ghost", triggers.KindCron)
	if _, _, err := k.Run(context.Background(), r); err == nil {
		t.Error("running an unregistered service should error")
	}
}

func spineCount(t *testing.T, k *Kernel, table, seq string) int {
	t.Helper()
	var n int
	if err := k.Store.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE sequence_id = ?", seq).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}
