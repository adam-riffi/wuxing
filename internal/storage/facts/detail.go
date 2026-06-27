package facts

import (
	"context"

	"github.com/adam-riffi/wuxing/internal/kernel/lineage"
	"github.com/adam-riffi/wuxing/internal/storage"
)

// Detail writes the per-tool detail fact tables (the typed measures behind the
// tool-call envelope), keyed by call_id and stamped with the lineage so they
// join the spine. All writes are append-only inserts.
type Detail struct {
	db *storage.DB
}

// NewDetail returns a Detail backed by db.
func NewDetail(db *storage.DB) *Detail { return &Detail{db: db} }

// RecordCrossing appends a connector crossing fact (movement: rows in/out).
func (d *Detail) RecordCrossing(ctx context.Context, st lineage.Stamp, backend, target, direction string, rowCount, byteCount, queryMs int) error {
	_, err := d.db.ExecContext(ctx, d.db.Rebind(
		`INSERT INTO connector_ft_crossing
			(call_id, sequence_id, run_id, session_id, backend, target, direction, row_count, byte_count, query_ms, recorded_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		string(st.Call), string(st.Sequence), string(st.Run), string(st.Session),
		backend, target, direction, rowCount, byteCount, queryMs, now())
	return wrap("record crossing", err)
}

// RecordMutation appends a data-mutation fact (state-delta: rows inserted/updated/deleted).
func (d *Detail) RecordMutation(ctx context.Context, st lineage.Stamp, target string, inserted, updated, deleted int) error {
	_, err := d.db.ExecContext(ctx, d.db.Rebind(
		`INSERT INTO wuxing_ft_data_mutation
			(call_id, sequence_id, run_id, session_id, target, rows_inserted, rows_updated, rows_deleted, recorded_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		string(st.Call), string(st.Sequence), string(st.Run), string(st.Session),
		target, inserted, updated, deleted, now())
	return wrap("record mutation", err)
}

// RecordAICall appends an ai detail fact (cost recorded at incur-time).
func (d *Detail) RecordAICall(ctx context.Context, st lineage.Stamp, model, mode string, tokensIn, tokensOut int, cost float64, ttftMs, turnCount int) error {
	_, err := d.db.ExecContext(ctx, d.db.Rebind(
		`INSERT INTO ai_ft_call
			(call_id, sequence_id, run_id, session_id, model, mode, tokens_in, tokens_out, cost, ttft_ms, turn_count, recorded_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		string(st.Call), string(st.Sequence), string(st.Run), string(st.Session),
		model, mode, tokensIn, tokensOut, cost, ttftMs, turnCount, now())
	return wrap("record ai call", err)
}
