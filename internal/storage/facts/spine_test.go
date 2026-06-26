package facts

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/adam-riffi/wuxing/internal/kernel/lineage"
	"github.com/adam-riffi/wuxing/internal/storage"
)

func newSpine(t *testing.T) (*Spine, *storage.DB) {
	t.Helper()
	db, err := storage.OpenSQLite(filepath.Join(t.TempDir(), "wuxing.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewSpine(db), db
}

func TestSpine_SequenceLifecycle(t *testing.T) {
	s, db := newSpine(t)
	ctx := context.Background()

	if err := s.OpenSequence(ctx, "seq", ""); err != nil {
		t.Fatalf("OpenSequence: %v", err)
	}
	if err := s.CloseSequence(ctx, "seq", OutcomeSuccess); err != nil {
		t.Fatalf("CloseSequence: %v", err)
	}

	var closedAt *string
	var outcome int
	err := db.QueryRow(`SELECT closed_at, outcome_id FROM wuxing_ft_sequence WHERE sequence_id = 'seq'`).
		Scan(&closedAt, &outcome)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if closedAt == nil {
		t.Error("closed_at not set after CloseSequence")
	}
	if Outcome(outcome) != OutcomeSuccess {
		t.Errorf("outcome: got %d want %d", outcome, OutcomeSuccess)
	}
}

func TestSpine_CloseOnceRejected(t *testing.T) {
	s, _ := newSpine(t)
	ctx := context.Background()
	_ = s.OpenSequence(ctx, "seq", "")
	if err := s.CloseSequence(ctx, "seq", OutcomeSuccess); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := s.CloseSequence(ctx, "seq", OutcomeFailure); err == nil {
		t.Fatal("second close should be rejected by the close-once trigger")
	}
}

func TestSpine_CloseUnknownFails(t *testing.T) {
	s, _ := newSpine(t)
	if err := s.CloseRun(context.Background(), "ghost", OutcomeSuccess); err == nil {
		t.Fatal("closing an unknown run should fail")
	}
}

func TestSpine_RunRequiresSequence(t *testing.T) {
	s, _ := newSpine(t)
	st := lineage.NewSequence("missing").WithRun("run", 1)
	if err := s.OpenRun(context.Background(), st); err == nil {
		t.Fatal("OpenRun against a missing sequence should violate the foreign key")
	}
}

func TestSpine_CausalOrderNotInsertOrder(t *testing.T) {
	s, _ := newSpine(t)
	ctx := context.Background()
	_ = s.OpenSequence(ctx, "seq", "")

	// Insert runs out of causal order: order 2 first, then order 1.
	base := lineage.NewSequence("seq")
	if err := s.OpenRun(ctx, base.WithRun("run-b", 2)); err != nil {
		t.Fatal(err)
	}
	if err := s.OpenRun(ctx, base.WithRun("run-a", 1)); err != nil {
		t.Fatal(err)
	}

	runs, err := s.RunsInSequence(ctx, "seq")
	if err != nil {
		t.Fatalf("RunsInSequence: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("got %d runs want 2", len(runs))
	}
	if runs[0].ID != "run-a" || runs[1].ID != "run-b" {
		t.Errorf("not in causal order: got %q, %q", runs[0].ID, runs[1].ID)
	}
}

func TestSpine_DeleteBlocked(t *testing.T) {
	s, db := newSpine(t)
	_ = s.OpenSequence(context.Background(), "seq", "")
	if _, err := db.Exec(`DELETE FROM wuxing_ft_sequence WHERE sequence_id = 'seq'`); err == nil {
		t.Fatal("DELETE on an append-only spine table should be rejected")
	}
}
