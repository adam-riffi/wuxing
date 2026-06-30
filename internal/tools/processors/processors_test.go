package processors

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/adam-riffi/wuxing/internal/storage"
)

func openStore(t *testing.T) *storage.DB {
	t.Helper()
	db, err := storage.OpenSQLite(filepath.Join(t.TempDir(), "wuxing.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func exec(t *testing.T, db *storage.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("seed exec failed: %v\n%s", err, query)
	}
}

// seedRun lays down one closed run with sessions + ai/connector/mutation facts.
func seedRun(t *testing.T, db *storage.DB) {
	t.Helper()
	exec(t, db, `INSERT INTO wuxing_ft_sequence (sequence_id, opened_at, closed_at, outcome_id) VALUES ('seq1','2026-01-01T00:00:00Z','2026-01-01T00:00:05Z',1)`)
	exec(t, db, `INSERT INTO wuxing_ft_run (run_id, sequence_id, sequence_order, opened_at, closed_at, outcome_id) VALUES ('run1','seq1',0,'2026-01-01T00:00:00Z','2026-01-01T00:00:02Z',1)`)
	exec(t, db, `INSERT INTO wuxing_ft_session (session_id, run_id, sequence_id, run_order, opened_at) VALUES ('s1','run1','seq1',0,'2026-01-01T00:00:00Z')`)
	exec(t, db, `INSERT INTO wuxing_ft_session (session_id, run_id, sequence_id, run_order, opened_at) VALUES ('s2','run1','seq1',1,'2026-01-01T00:00:01Z')`)
	exec(t, db, `INSERT INTO ai_ft_call (call_id, run_id, sequence_id, tokens_in, tokens_out, cost, recorded_at) VALUES ('c1','run1','seq1',10,20,0.50,'2026-01-01T00:00:01Z')`)
	exec(t, db, `INSERT INTO ai_ft_call (call_id, run_id, sequence_id, tokens_in, tokens_out, cost, recorded_at) VALUES ('c2','run1','seq1',5,5,0.25,'2026-01-01T00:00:01Z')`)
	exec(t, db, `INSERT INTO connector_ft_crossing (call_id, run_id, sequence_id, row_count, byte_count, recorded_at) VALUES ('x1','run1','seq1',100,2048,'2026-01-01T00:00:01Z')`)
	exec(t, db, `INSERT INTO wuxing_ft_data_mutation (call_id, run_id, sequence_id, rows_inserted, rows_updated, rows_deleted, recorded_at) VALUES ('m1','run1','seq1',3,1,0,'2026-01-01T00:00:01Z')`)
}

func TestSummarizeRun(t *testing.T) {
	db := openStore(t)
	ctx := context.Background()
	seedRun(t, db)

	s, err := New(db).SummarizeRun(ctx, "run1")
	if err != nil {
		t.Fatalf("SummarizeRun: %v", err)
	}

	if s.SequenceID != "seq1" || s.SessionCount != 2 || s.DurationMs != 2000 {
		t.Errorf("spine rollup: %+v", s)
	}
	if s.AICalls != 2 || s.AITokensIn != 15 || s.AITokensOut != 25 || s.AICost != 0.75 {
		t.Errorf("ai rollup: %+v", s)
	}
	if s.Crossings != 1 || s.RowsCrossed != 100 || s.BytesCrossed != 2048 {
		t.Errorf("crossing rollup: %+v", s)
	}
	if s.Mutations != 1 || s.RowsWritten != 4 {
		t.Errorf("mutation rollup: %+v", s)
	}
	if s.Outcome == nil || *s.Outcome != 1 {
		t.Errorf("outcome: %+v", s.Outcome)
	}

	// Persisted and re-readable.
	got, err := New(db).GetRunSummary(ctx, "run1")
	if err != nil {
		t.Fatalf("GetRunSummary: %v", err)
	}
	if got.AICost != 0.75 || got.SessionCount != 2 || got.RowsWritten != 4 {
		t.Errorf("persisted rollup mismatch: %+v", got)
	}
}

func TestSummarizeRun_Recomputes(t *testing.T) {
	db := openStore(t)
	ctx := context.Background()
	seedRun(t, db)
	p := New(db)

	if _, err := p.SummarizeRun(ctx, "run1"); err != nil {
		t.Fatal(err)
	}
	// A late ai fact arrives; re-summarizing must replace, not duplicate.
	exec(t, db, `INSERT INTO ai_ft_call (call_id, run_id, sequence_id, tokens_in, tokens_out, cost, recorded_at) VALUES ('c3','run1','seq1',1,1,1.00,'2026-01-01T00:00:02Z')`)
	s, err := p.SummarizeRun(ctx, "run1")
	if err != nil {
		t.Fatal(err)
	}
	if s.AICalls != 3 || s.AICost != 1.75 {
		t.Errorf("recompute: %+v", s)
	}

	var rows int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM processors_ft_run WHERE run_id = 'run1'`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Errorf("rollup should upsert (one row), got %d", rows)
	}
}

func TestSummarizeSequence(t *testing.T) {
	db := openStore(t)
	ctx := context.Background()
	seedRun(t, db)
	// a second run in the same sequence
	exec(t, db, `INSERT INTO wuxing_ft_run (run_id, sequence_id, sequence_order, opened_at, closed_at, outcome_id) VALUES ('run2','seq1',1,'2026-01-01T00:00:02Z','2026-01-01T00:00:03Z',1)`)
	exec(t, db, `INSERT INTO ai_ft_call (call_id, run_id, sequence_id, tokens_in, tokens_out, cost, recorded_at) VALUES ('c9','run2','seq1',2,2,0.10,'2026-01-01T00:00:02Z')`)

	s, err := New(db).SummarizeSequence(ctx, "seq1")
	if err != nil {
		t.Fatalf("SummarizeSequence: %v", err)
	}
	if s.RunCount != 2 || s.AICalls != 3 || s.Crossings != 1 || s.Mutations != 1 {
		t.Errorf("sequence rollup: %+v", s)
	}
	if s.AICost != 0.85 { // 0.50 + 0.25 + 0.10
		t.Errorf("sequence ai cost: got %v want 0.85", s.AICost)
	}
}

func TestSummarizeRun_Unknown(t *testing.T) {
	db := openStore(t)
	if _, err := New(db).SummarizeRun(context.Background(), "ghost"); err == nil {
		t.Error("summarizing an unknown run should error")
	}
}
