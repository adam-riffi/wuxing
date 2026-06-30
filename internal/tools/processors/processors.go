// Package processors is the derive tool: turn the raw signals the platform
// already emits (the lineage spine + per-tool detail facts) into structured
// metrics — the derived rollup rows the observability layer queries. It only
// reduces and records; it never moves domain data, infers, or interprets cfg.
//
// It writes the processors_ domain's derived facts: one row per run
// (processors_ft_run) and per sequence (processors_ft_sequence). These are
// projections of the append-only facts, so they are recomputable and upserted —
// the run loop calls Summarize* as runs and sequences close.
package processors

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/adam-riffi/wuxing/internal/storage"
)

// RunSummary is the derived per-run rollup.
type RunSummary struct {
	RunID        string
	SequenceID   string
	ServiceID    string
	Outcome      *int
	DurationMs   int64
	SessionCount int
	AICalls      int
	AITokensIn   int
	AITokensOut  int
	AICost       float64
	Crossings    int
	RowsCrossed  int64
	BytesCrossed int64
	Mutations    int
	RowsWritten  int64
}

// SequenceSummary is the derived per-sequence rollup.
type SequenceSummary struct {
	SequenceID  string
	RunCount    int
	Outcome     *int
	AICalls     int
	AITokensIn  int
	AITokensOut int
	AICost      float64
	Crossings   int
	Mutations   int
}

// Processor derives summary rows from the raw fact tables.
type Processor struct {
	db *storage.DB
}

// New returns a Processor over db.
func New(db *storage.DB) *Processor { return &Processor{db: db} }

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

// SummarizeRun computes the rollup for one run from its spine + detail facts and
// upserts it into processors_ft_run.
func (p *Processor) SummarizeRun(ctx context.Context, runID string) (RunSummary, error) {
	s := RunSummary{RunID: runID}

	var seqID, serviceID *string
	var openedAt string
	var closedAt *string
	err := p.db.QueryRowContext(ctx, p.db.Rebind(
		`SELECT sequence_id, service_id, outcome_id, opened_at, closed_at FROM wuxing_ft_run WHERE run_id = ?`), runID).
		Scan(&seqID, &serviceID, &s.Outcome, &openedAt, &closedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return RunSummary{}, fmt.Errorf("processors: run %q not found", runID)
	}
	if err != nil {
		return RunSummary{}, wrap("read run", err)
	}
	s.SequenceID = deref(seqID)
	s.ServiceID = deref(serviceID)
	s.DurationMs = durationMs(openedAt, closedAt)

	if err := p.db.QueryRowContext(ctx, p.db.Rebind(
		`SELECT COUNT(*) FROM wuxing_ft_session WHERE run_id = ?`), runID).Scan(&s.SessionCount); err != nil {
		return RunSummary{}, wrap("count sessions", err)
	}
	if err := p.db.QueryRowContext(ctx, p.db.Rebind(
		`SELECT COUNT(*), COALESCE(SUM(tokens_in),0), COALESCE(SUM(tokens_out),0), COALESCE(SUM(cost),0)
			FROM ai_ft_call WHERE run_id = ?`), runID).
		Scan(&s.AICalls, &s.AITokensIn, &s.AITokensOut, &s.AICost); err != nil {
		return RunSummary{}, wrap("rollup ai", err)
	}
	if err := p.db.QueryRowContext(ctx, p.db.Rebind(
		`SELECT COUNT(*), COALESCE(SUM(row_count),0), COALESCE(SUM(byte_count),0)
			FROM connector_ft_crossing WHERE run_id = ?`), runID).
		Scan(&s.Crossings, &s.RowsCrossed, &s.BytesCrossed); err != nil {
		return RunSummary{}, wrap("rollup crossings", err)
	}
	if err := p.db.QueryRowContext(ctx, p.db.Rebind(
		`SELECT COUNT(*), COALESCE(SUM(rows_inserted + rows_updated + rows_deleted),0)
			FROM wuxing_ft_data_mutation WHERE run_id = ?`), runID).
		Scan(&s.Mutations, &s.RowsWritten); err != nil {
		return RunSummary{}, wrap("rollup mutations", err)
	}

	if err := p.upsertRun(ctx, s); err != nil {
		return RunSummary{}, err
	}
	return s, nil
}

func (p *Processor) upsertRun(ctx context.Context, s RunSummary) error {
	if _, err := p.db.ExecContext(ctx, p.db.Rebind(
		`DELETE FROM processors_ft_run WHERE run_id = ?`), s.RunID); err != nil {
		return wrap("clear run rollup", err)
	}
	_, err := p.db.ExecContext(ctx, p.db.Rebind(
		`INSERT INTO processors_ft_run
			(run_id, sequence_id, service_id, outcome_id, duration_ms, session_count,
			 ai_calls, ai_tokens_in, ai_tokens_out, ai_cost,
			 crossings, rows_crossed, bytes_crossed, mutations, rows_written, derived_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		s.RunID, nullable(s.SequenceID), nullable(s.ServiceID), s.Outcome, s.DurationMs, s.SessionCount,
		s.AICalls, s.AITokensIn, s.AITokensOut, s.AICost,
		s.Crossings, s.RowsCrossed, s.BytesCrossed, s.Mutations, s.RowsWritten, now())
	return wrap("write run rollup", err)
}

// SummarizeSequence computes the rollup for one sequence and upserts it into
// processors_ft_sequence.
func (p *Processor) SummarizeSequence(ctx context.Context, seqID string) (SequenceSummary, error) {
	s := SequenceSummary{SequenceID: seqID}

	err := p.db.QueryRowContext(ctx, p.db.Rebind(
		`SELECT outcome_id FROM wuxing_ft_sequence WHERE sequence_id = ?`), seqID).Scan(&s.Outcome)
	if errors.Is(err, sql.ErrNoRows) {
		return SequenceSummary{}, fmt.Errorf("processors: sequence %q not found", seqID)
	}
	if err != nil {
		return SequenceSummary{}, wrap("read sequence", err)
	}

	if err := p.db.QueryRowContext(ctx, p.db.Rebind(
		`SELECT COUNT(*) FROM wuxing_ft_run WHERE sequence_id = ?`), seqID).Scan(&s.RunCount); err != nil {
		return SequenceSummary{}, wrap("count runs", err)
	}
	if err := p.db.QueryRowContext(ctx, p.db.Rebind(
		`SELECT COUNT(*), COALESCE(SUM(tokens_in),0), COALESCE(SUM(tokens_out),0), COALESCE(SUM(cost),0)
			FROM ai_ft_call WHERE sequence_id = ?`), seqID).
		Scan(&s.AICalls, &s.AITokensIn, &s.AITokensOut, &s.AICost); err != nil {
		return SequenceSummary{}, wrap("rollup ai", err)
	}
	if err := p.db.QueryRowContext(ctx, p.db.Rebind(
		`SELECT COUNT(*) FROM connector_ft_crossing WHERE sequence_id = ?`), seqID).Scan(&s.Crossings); err != nil {
		return SequenceSummary{}, wrap("count crossings", err)
	}
	if err := p.db.QueryRowContext(ctx, p.db.Rebind(
		`SELECT COUNT(*) FROM wuxing_ft_data_mutation WHERE sequence_id = ?`), seqID).Scan(&s.Mutations); err != nil {
		return SequenceSummary{}, wrap("count mutations", err)
	}

	if err := p.upsertSequence(ctx, s); err != nil {
		return SequenceSummary{}, err
	}
	return s, nil
}

func (p *Processor) upsertSequence(ctx context.Context, s SequenceSummary) error {
	if _, err := p.db.ExecContext(ctx, p.db.Rebind(
		`DELETE FROM processors_ft_sequence WHERE sequence_id = ?`), s.SequenceID); err != nil {
		return wrap("clear sequence rollup", err)
	}
	_, err := p.db.ExecContext(ctx, p.db.Rebind(
		`INSERT INTO processors_ft_sequence
			(sequence_id, run_count, outcome_id, ai_calls, ai_tokens_in, ai_tokens_out, ai_cost,
			 crossings, mutations, derived_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		s.SequenceID, s.RunCount, s.Outcome, s.AICalls, s.AITokensIn, s.AITokensOut, s.AICost,
		s.Crossings, s.Mutations, now())
	return wrap("write sequence rollup", err)
}

// GetRunSummary reads a persisted run rollup.
func (p *Processor) GetRunSummary(ctx context.Context, runID string) (RunSummary, error) {
	s := RunSummary{}
	var seqID, serviceID *string
	err := p.db.QueryRowContext(ctx, p.db.Rebind(
		`SELECT run_id, sequence_id, service_id, outcome_id, duration_ms, session_count,
			ai_calls, ai_tokens_in, ai_tokens_out, ai_cost,
			crossings, rows_crossed, bytes_crossed, mutations, rows_written
		 FROM processors_ft_run WHERE run_id = ?`), runID).
		Scan(&s.RunID, &seqID, &serviceID, &s.Outcome, &s.DurationMs, &s.SessionCount,
			&s.AICalls, &s.AITokensIn, &s.AITokensOut, &s.AICost,
			&s.Crossings, &s.RowsCrossed, &s.BytesCrossed, &s.Mutations, &s.RowsWritten)
	if errors.Is(err, sql.ErrNoRows) {
		return RunSummary{}, fmt.Errorf("processors: no rollup for run %q", runID)
	}
	if err != nil {
		return RunSummary{}, wrap("get run rollup", err)
	}
	s.SequenceID = deref(seqID)
	s.ServiceID = deref(serviceID)
	return s, nil
}

func durationMs(openedAt string, closedAt *string) int64 {
	if closedAt == nil {
		return 0
	}
	o, err1 := time.Parse(time.RFC3339Nano, openedAt)
	c, err2 := time.Parse(time.RFC3339Nano, *closedAt)
	if err1 != nil || err2 != nil {
		return 0
	}
	if d := c.Sub(o).Milliseconds(); d > 0 {
		return d
	}
	return 0
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// nullable maps an empty string to a NULL so optional FK columns stay clean.
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func wrap(op string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("processors: %s: %w", op, err)
}
