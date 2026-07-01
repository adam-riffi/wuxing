package kernel

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/adam-riffi/wuxing/internal/contracts/cfg"
	"github.com/adam-riffi/wuxing/internal/kernel/bus"
	"github.com/adam-riffi/wuxing/internal/kernel/lineage"
	"github.com/adam-riffi/wuxing/internal/kernel/scheduler"
	"github.com/adam-riffi/wuxing/internal/kernel/triggers"
	"github.com/adam-riffi/wuxing/internal/storage"
)

// directRun builds a triggers.Run without going through FireExternal, so
// Kernel.Run can be exercised in isolation (FireExternal now auto-runs via the
// scheduler wiring).
func (k *Kernel) directRun(service string) triggers.Run {
	stamp := lineage.NewSequence(k.Minter.SequenceID()).WithRun(k.Minter.RunID(), 0)
	return triggers.Run{Service: service, Kind: triggers.KindCron, Stamp: stamp}
}

func testKernel(t *testing.T) *Kernel {
	t.Helper()
	store, err := storage.OpenSQLite(filepath.Join(t.TempDir(), "wuxing.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return Assemble(store, 1<<30, 100)
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

	// A run built directly (FireExternal would auto-run via the scheduler).
	r := k.directRun("mtg")

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

func TestKernel_Cascade_OneSequenceAcrossRuns(t *testing.T) {
	k := testKernel(t)

	var aiCalls int
	_ = k.Bus.Register("ai", func(_ context.Context, e bus.Envelope) bus.Envelope {
		aiCalls++
		return e.Reply(json.RawMessage(`{"new_set":true}`))
	})

	notifier := &cfg.Service{
		Name:     "notifier",
		Envelope: cfg.Envelope{Request: 1},
		Workflow: []cfg.Step{{ID: "compose", Tool: "ai", Operation: "infer"}},
	}
	mtg := &cfg.Service{
		Name:       "mtg",
		Envelope:   cfg.Envelope{Request: 1},
		Workflow:   []cfg.Step{{ID: "check", Tool: "ai", Operation: "infer"}},
		Successors: []cfg.Successor{{Service: "notifier", Topic: "mtg-db updated", When: "new_set == true"}},
	}
	if err := k.Register(notifier); err != nil {
		t.Fatal(err)
	}
	if err := k.Register(mtg); err != nil {
		t.Fatal(err)
	}

	// Firing mtg's external trigger drives the whole cascade synchronously
	// (onFire -> Submit -> onAdmit -> Run -> fire successor -> notifier).
	r := k.Triggers.FireExternal("mtg", triggers.KindCron)
	seq := string(r.Stamp.Sequence)

	if aiCalls != 2 {
		t.Errorf("expected mtg + notifier to each call ai once, got %d", aiCalls)
	}

	// Both runs landed in the spine under ONE sequence, ordered 0 then 1.
	if n := spineCount(t, k, "wuxing_ft_sequence", seq); n != 1 {
		t.Errorf("sequences: got %d want 1", n)
	}
	if n := spineCount(t, k, "wuxing_ft_run", seq); n != 2 {
		t.Fatalf("runs under the sequence: got %d want 2", n)
	}

	rows, err := k.Store.Query(`SELECT sequence_order FROM wuxing_ft_run WHERE sequence_id = ? ORDER BY sequence_order`, seq)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var orders []int
	for rows.Next() {
		var o int
		_ = rows.Scan(&o)
		orders = append(orders, o)
	}
	if len(orders) != 2 || orders[0] != 0 || orders[1] != 1 {
		t.Errorf("sequence_order: got %v want [0 1]", orders)
	}

	// The sequence closed when the cascade ended (no more in-flight runs).
	var closed *string
	if err := k.Store.QueryRow(`SELECT closed_at FROM wuxing_ft_sequence WHERE sequence_id = ?`, seq).Scan(&closed); err != nil {
		t.Fatal(err)
	}
	if closed == nil {
		t.Error("sequence was not closed after the cascade ended")
	}

	// The processors tool derived rollups automatically: a per-run row for each
	// run, and a per-sequence row counting both.
	var runRollups, seqRuns int
	if err := k.Store.QueryRow(`SELECT COUNT(*) FROM processors_ft_run WHERE sequence_id = ?`, seq).Scan(&runRollups); err != nil {
		t.Fatal(err)
	}
	if runRollups != 2 {
		t.Errorf("processors_ft_run rows: got %d want 2", runRollups)
	}
	if err := k.Store.QueryRow(`SELECT run_count FROM processors_ft_sequence WHERE sequence_id = ?`, seq).Scan(&seqRuns); err != nil {
		t.Fatalf("no sequence rollup derived: %v", err)
	}
	if seqRuns != 2 {
		t.Errorf("processors_ft_sequence.run_count: got %d want 2", seqRuns)
	}
}

func TestRunner_JobCarriesTheEnvelope(t *testing.T) {
	k := testKernel(t)
	svc := &cfg.Service{
		Name: "mtg",
		Envelope: cfg.Envelope{
			Request:   64,
			Limit:     128,
			AIRequest: 2,
			Priority:  "user",
			MaxWait:   "10m",
			OnStarve:  "fail",
		},
	}
	if err := k.Register(svc); err != nil {
		t.Fatal(err)
	}

	job := k.Runner.jobFor(k.directRun("mtg"))
	if job.Request != 64 || job.Limit != 128 || job.AIRequest != 2 {
		t.Errorf("resources not threaded: %+v", job)
	}
	if job.Priority != scheduler.PriorityUser {
		t.Errorf("priority not threaded: %+v", job.Priority)
	}
	if job.MaxWait != 10*time.Minute {
		t.Errorf("max_wait not threaded: %v", job.MaxWait)
	}
	if job.OnExpiry != scheduler.ExpiryFail {
		t.Errorf("on_starve not threaded: %v", job.OnExpiry)
	}

	// Defaults: unknown service gets the minimal floor + escalate.
	def := k.Runner.jobFor(k.directRun("ghost"))
	if def.Request != 1 || def.OnExpiry != scheduler.ExpiryEscalate || def.Priority != scheduler.PriorityBackground {
		t.Errorf("defaults: %+v", def)
	}
}

func TestRunner_OnExpireReleasesTheSequence(t *testing.T) {
	k := testKernel(t)
	rn := k.Runner

	r := k.directRun("mtg")
	job := scheduler.Job{ID: string(r.Stamp.Run)}
	rn.mu.Lock()
	rn.pending[job.ID] = r
	rn.inflight[r.Stamp.Sequence] = &seqState{inflight: 1}
	rn.mu.Unlock()

	rn.onExpire(job)

	rn.mu.Lock()
	defer rn.mu.Unlock()
	if _, ok := rn.pending[job.ID]; ok {
		t.Error("expired job should be released from pending")
	}
	if _, ok := rn.inflight[r.Stamp.Sequence]; ok {
		t.Error("expired job should release its sequence bookkeeping")
	}
}

func TestRunner_ConcurrencyForbidSkipsOverlap(t *testing.T) {
	k := testKernel(t)
	svc := &cfg.Service{
		Name:        "mtg",
		Envelope:    cfg.Envelope{Request: 1},
		Concurrency: "forbid",
	}
	if err := k.Register(svc); err != nil {
		t.Fatal(err)
	}

	// Simulate a run already in flight (queued but not admitted).
	rn := k.Runner
	prior := k.directRun("mtg")
	rn.mu.Lock()
	rn.pending["prior"] = prior
	rn.mu.Unlock()

	k.Triggers.FireExternal("mtg", triggers.KindCron)

	rn.mu.Lock()
	defer rn.mu.Unlock()
	if len(rn.pending) != 1 {
		t.Errorf("overlapping fire should be skipped, pending=%d", len(rn.pending))
	}

	// concurrency: allow (default) does not skip — the fire runs to completion
	// (synchronously) and lands in the spine.
	allow := &cfg.Service{Name: "other", Envelope: cfg.Envelope{Request: 1}}
	rn.mu.Unlock()
	if err := k.Register(allow); err != nil {
		t.Fatal(err)
	}
	r2 := k.Triggers.FireExternal("other", triggers.KindCron)
	rn.mu.Lock()
	if n := spineCount(t, k, "wuxing_ft_run", string(r2.Stamp.Sequence)); n != 1 {
		t.Errorf("default concurrency should run: got %d spine runs", n)
	}
}

func TestKernel_Run_UnknownService(t *testing.T) {
	k := testKernel(t)
	r := k.directRun("ghost")
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
