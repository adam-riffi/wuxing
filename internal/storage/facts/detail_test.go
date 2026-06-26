package facts

import (
	"context"
	"testing"

	"github.com/adam-riffi/wuxing/internal/kernel/lineage"
)

func detailStamp() lineage.Stamp {
	return lineage.NewSequence("seq").WithRun("run", 0).WithSession("sess", 0).WithCall("call", 0)
}

func TestDetail_RecordCrossing(t *testing.T) {
	_, db := newSpine(t)
	d := NewDetail(db)
	ctx := context.Background()

	if err := d.RecordCrossing(ctx, detailStamp(), "sqlite", "mtg.cards", "out", 3, 128, 5); err != nil {
		t.Fatalf("RecordCrossing: %v", err)
	}

	var (
		seq, target, dir string
		rowCount         int
	)
	err := db.QueryRow(`SELECT sequence_id, target, direction, row_count FROM connector_ft_crossing WHERE call_id = 'call'`).
		Scan(&seq, &target, &dir, &rowCount)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if seq != "seq" || target != "mtg.cards" || dir != "out" || rowCount != 3 {
		t.Errorf("crossing row: seq=%q target=%q dir=%q rows=%d", seq, target, dir, rowCount)
	}
}

func TestDetail_RecordMutation(t *testing.T) {
	_, db := newSpine(t)
	d := NewDetail(db)

	if err := d.RecordMutation(context.Background(), detailStamp(), "mtg.cards", 3, 1, 0); err != nil {
		t.Fatalf("RecordMutation: %v", err)
	}

	var inserted, updated, deleted int
	err := db.QueryRow(`SELECT rows_inserted, rows_updated, rows_deleted FROM wuxing_ft_data_mutation WHERE call_id = 'call'`).
		Scan(&inserted, &updated, &deleted)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if inserted != 3 || updated != 1 || deleted != 0 {
		t.Errorf("mutation row: %d/%d/%d", inserted, updated, deleted)
	}
}

func TestDetail_RecordAICall(t *testing.T) {
	_, db := newSpine(t)
	d := NewDetail(db)

	if err := d.RecordAICall(context.Background(), detailStamp(), "gpt-x", "infer", 12, 8, 0.0021, 90, 1); err != nil {
		t.Fatalf("RecordAICall: %v", err)
	}

	var (
		model string
		cost  float64
		turns int
	)
	err := db.QueryRow(`SELECT model, cost, turn_count FROM ai_ft_call WHERE call_id = 'call'`).
		Scan(&model, &cost, &turns)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if model != "gpt-x" || cost != 0.0021 || turns != 1 {
		t.Errorf("ai_ft_call row: model=%q cost=%v turns=%d", model, cost, turns)
	}
}
