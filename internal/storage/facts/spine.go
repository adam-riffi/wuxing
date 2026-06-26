// Package facts reads and writes the append-only fact tables — the structured
// event ledger. It owns the lineage spine (sequence/run/session) and, later, the
// per-tool detail facts. The kernel stamps these from the outside; a service
// never writes its own lineage, so it cannot forge it.
package facts

import (
	"context"
	"fmt"
	"time"

	"github.com/adam-riffi/wuxing/internal/kernel/lineage"
	"github.com/adam-riffi/wuxing/internal/storage"
)

// Outcome is the fixed terminal vocabulary stamped on a closed spine row. The
// values match the wuxing_dt_outcome rows seeded by the schema.
type Outcome int

const (
	// OutcomeSuccess is a run that completed as intended.
	OutcomeSuccess Outcome = 1
	// OutcomeFailure is a run that failed.
	OutcomeFailure Outcome = 2
	// OutcomeCancelled is a run that was cancelled before completing.
	OutcomeCancelled Outcome = 3
)

// Spine writes and reads the lineage spine: sequences, runs, and sessions.
type Spine struct {
	db *storage.DB
}

// NewSpine returns a Spine backed by db.
func NewSpine(db *storage.DB) *Spine { return &Spine{db: db} }

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

// OpenSequence records the head of a causal chain. originTrigger may be empty.
func (s *Spine) OpenSequence(ctx context.Context, id lineage.SequenceID, originTrigger string) error {
	var trig any
	if originTrigger != "" {
		trig = originTrigger
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO wuxing_ft_sequence (sequence_id, origin_trigger_id, opened_at) VALUES (?, ?, ?)`,
		string(id), trig, now())
	return wrap("open sequence", err)
}

// CloseSequence stamps the terminal outcome on a sequence. It fails if the
// sequence is unknown or already closed.
func (s *Spine) CloseSequence(ctx context.Context, id lineage.SequenceID, outcome Outcome) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE wuxing_ft_sequence SET closed_at = ?, outcome_id = ? WHERE sequence_id = ?`,
		now(), int(outcome), string(id))
	return closed("sequence", string(id), res, err)
}

// OpenRun records a triggered execution within its sequence, at st.SequenceOrder.
func (s *Spine) OpenRun(ctx context.Context, st lineage.Stamp) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO wuxing_ft_run (run_id, sequence_id, sequence_order, opened_at) VALUES (?, ?, ?, ?)`,
		string(st.Run), string(st.Sequence), st.SequenceOrder, now())
	return wrap("open run", err)
}

// CloseRun stamps the terminal outcome on a run.
func (s *Spine) CloseRun(ctx context.Context, id lineage.RunID, outcome Outcome) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE wuxing_ft_run SET closed_at = ?, outcome_id = ? WHERE run_id = ?`,
		now(), int(outcome), string(id))
	return closed("run", string(id), res, err)
}

// OpenSession records a unit of work within its run, at st.RunOrder.
func (s *Spine) OpenSession(ctx context.Context, st lineage.Stamp) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO wuxing_ft_session (session_id, run_id, sequence_id, run_order, opened_at) VALUES (?, ?, ?, ?, ?)`,
		string(st.Session), string(st.Run), string(st.Sequence), st.RunOrder, now())
	return wrap("open session", err)
}

// CloseSession stamps the terminal outcome on a session.
func (s *Spine) CloseSession(ctx context.Context, id lineage.SessionID, outcome Outcome) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE wuxing_ft_session SET closed_at = ?, outcome_id = ? WHERE session_id = ?`,
		now(), int(outcome), string(id))
	return closed("session", string(id), res, err)
}

// Run is a row of the run spine, returned in causal order.
type Run struct {
	ID            lineage.RunID
	SequenceOrder int
	Outcome       *Outcome
}

// RunsInSequence returns the runs of a sequence in causal order (by
// sequence_order), never by clock — parallel runs may share a timestamp.
func (s *Spine) RunsInSequence(ctx context.Context, seq lineage.SequenceID) ([]Run, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT run_id, sequence_order, outcome_id FROM wuxing_ft_run WHERE sequence_id = ? ORDER BY sequence_order`,
		string(seq))
	if err != nil {
		return nil, wrap("list runs", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Run
	for rows.Next() {
		var r Run
		var outcome *int
		if err := rows.Scan(&r.ID, &r.SequenceOrder, &outcome); err != nil {
			return nil, wrap("scan run", err)
		}
		if outcome != nil {
			o := Outcome(*outcome)
			r.Outcome = &o
		}
		out = append(out, r)
	}
	return out, wrap("list runs", rows.Err())
}

func wrap(op string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("facts: %s: %w", op, err)
}

func closed(kind, id string, res interface{ RowsAffected() (int64, error) }, err error) error {
	if err != nil {
		return wrap("close "+kind, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("facts: close %s: no open row %q", kind, id)
	}
	return nil
}
