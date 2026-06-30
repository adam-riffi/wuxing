package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adam-riffi/wuxing/internal/storage"
)

// seedStore writes a small closed cascade to a fresh fact store and returns its
// path. The seed connection is closed so the command opens its own.
func seedStore(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "wuxing.db")
	db, err := storage.OpenSQLite(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := context.Background()
	exec := func(q string) {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatalf("seed failed: %v\n%s", err, q)
		}
	}
	exec(`INSERT INTO wuxing_ft_sequence (sequence_id, opened_at, closed_at, outcome_id) VALUES ('seqAAAA1111','2026-01-01T00:00:00Z','2026-01-01T00:00:05Z',1)`)
	exec(`INSERT INTO wuxing_ft_run (run_id, sequence_id, sequence_order, opened_at, closed_at, outcome_id) VALUES ('runAAAA1111','seqAAAA1111',0,'2026-01-01T00:00:00Z','2026-01-01T00:00:02Z',1)`)
	exec(`INSERT INTO processors_ft_run (run_id, sequence_id, session_count, ai_calls, ai_cost, duration_ms, derived_at) VALUES ('runAAAA1111','seqAAAA1111',1,1,0.0021,2000,'2026-01-01T00:00:02Z')`)
	exec(`INSERT INTO processors_ft_sequence (sequence_id, run_count, ai_cost, derived_at) VALUES ('seqAAAA1111',1,0.0021,'2026-01-01T00:00:05Z')`)
	exec(`INSERT INTO ai_ft_call (call_id, run_id, sequence_id, model, mode, tokens_in, tokens_out, cost, recorded_at) VALUES ('c1','runAAAA1111','seqAAAA1111','demo-model','infer',12,34,0.0021,'2026-01-01T00:00:01Z')`)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func runCmd(t *testing.T, args ...string) string {
	t.Helper()
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("command %v failed: %v\n%s", args, err, out.String())
	}
	return out.String()
}

func TestRunsCmd(t *testing.T) {
	path := seedStore(t)
	out := runCmd(t, "runs", "--store", path)
	for _, want := range []string{"runAAAA1", "seqAAAA1", "success", "0.0021"} {
		if !strings.Contains(out, want) {
			t.Errorf("runs output missing %q:\n%s", want, out)
		}
	}
}

func TestShowCmd_ByPrefix(t *testing.T) {
	path := seedStore(t)
	// short prefix should resolve to the full sequence id
	out := runCmd(t, "show", "seqAAAA", "--store", path)
	for _, want := range []string{"sequence seqAAAA1111", "runs=1", "demo-model/infer", "success"} {
		if !strings.Contains(out, want) {
			t.Errorf("show output missing %q:\n%s", want, out)
		}
	}
}

func TestShowCmd_NotFound(t *testing.T) {
	path := seedStore(t)
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"show", "ghost", "--store", path})
	if err := root.Execute(); err == nil {
		t.Error("expected a not-found error for an unknown sequence")
	}
}

func TestRunsCmd_MissingStore(t *testing.T) {
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"runs", "--store", filepath.Join(t.TempDir(), "nope.db")})
	if err := root.Execute(); err == nil {
		t.Error("expected an error when the store does not exist")
	}
}
