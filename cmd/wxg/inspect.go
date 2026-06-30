package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/adam-riffi/wuxing/internal/storage"
)

const defaultStore = "~/.wuxing/wuxing.db"

// errWriter is the standard "write a lot, check once" helper (Effective Go): it
// swallows further writes after the first error and surfaces it at the end.
type errWriter struct {
	w   io.Writer
	err error
}

func (e *errWriter) printf(format string, a ...any) {
	if e.err != nil {
		return
	}
	_, e.err = fmt.Fprintf(e.w, format, a...)
}

// newRunsCmd lists recent runs from the fact store — the wuxing equivalent of
// `kubectl get`. It reads the store directly (WAL allows concurrent reads while
// the daemon writes), so it needs no daemon connection.
func newRunsCmd() *cobra.Command {
	var store string
	var limit int
	cmd := &cobra.Command{
		Use:   "runs",
		Short: "list recent runs (newest first)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			db, err := openStore(store)
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			rows, err := db.QueryContext(cmd.Context(), db.Rebind(
				`SELECT r.run_id, r.sequence_id, r.opened_at, r.outcome_id,
					COALESCE(p.session_count, 0), COALESCE(p.ai_calls, 0),
					COALESCE(p.ai_cost, 0), COALESCE(p.duration_ms, 0)
				 FROM wuxing_ft_run r
				 LEFT JOIN processors_ft_run p ON p.run_id = r.run_id
				 ORDER BY r.opened_at DESC
				 LIMIT ?`), limit)
			if err != nil {
				return err
			}
			defer func() { _ = rows.Close() }()

			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			ew := &errWriter{w: tw}
			ew.printf("RUN\tSEQUENCE\tOUTCOME\tSESSIONS\tAI\tCOST\tDUR(ms)\tOPENED\n")
			n := 0
			for rows.Next() {
				var runID, seqID, opened string
				var outcome *int
				var sessions, aiCalls, dur int
				var cost float64
				if err := rows.Scan(&runID, &seqID, &opened, &outcome, &sessions, &aiCalls, &cost, &dur); err != nil {
					return err
				}
				ew.printf("%s\t%s\t%s\t%d\t%d\t%.4f\t%d\t%s\n",
					short(runID), short(seqID), outcomeLabel(outcome), sessions, aiCalls, cost, dur, opened)
				n++
			}
			if err := rows.Err(); err != nil {
				return err
			}
			if ew.err != nil {
				return ew.err
			}
			if err := tw.Flush(); err != nil {
				return err
			}
			if n == 0 {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "no runs recorded yet")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&store, "store", defaultStore, "path to the fact store")
	cmd.Flags().IntVar(&limit, "limit", 20, "max runs to show")
	return cmd
}

// newShowCmd drills into one sequence — the wuxing equivalent of
// `kubectl describe`: the run cascade, each run's rollup, and its tool facts.
func newShowCmd() *cobra.Command {
	var store string
	cmd := &cobra.Command{
		Use:   "show <sequence>",
		Short: "show a sequence: its run cascade, rollups, and tool facts",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openStore(store)
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()
			ctx := cmd.Context()
			ew := &errWriter{w: cmd.OutOrStdout()}

			seq, err := resolveSequence(ctx, db, args[0])
			if err != nil {
				return err
			}

			var opened string
			var closed *string
			var seqOutcome *int
			var runCount int
			var aiCost float64
			if err := db.QueryRowContext(ctx, db.Rebind(
				`SELECT s.opened_at, s.closed_at, s.outcome_id,
					COALESCE(p.run_count, 0), COALESCE(p.ai_cost, 0)
				 FROM wuxing_ft_sequence s
				 LEFT JOIN processors_ft_sequence p ON p.sequence_id = s.sequence_id
				 WHERE s.sequence_id = ?`), seq).
				Scan(&opened, &closed, &seqOutcome, &runCount, &aiCost); err != nil {
				return err
			}
			ew.printf("sequence %s\n", seq)
			ew.printf("  outcome=%s  runs=%d  ai_cost=%.4f  opened=%s%s\n\n",
				outcomeLabel(seqOutcome), runCount, aiCost, opened, closedSuffix(closed))

			rows, err := db.QueryContext(ctx, db.Rebind(
				`SELECT r.run_id, r.sequence_order, r.outcome_id,
					COALESCE(p.session_count, 0), COALESCE(p.ai_calls, 0),
					COALESCE(p.ai_cost, 0), COALESCE(p.duration_ms, 0)
				 FROM wuxing_ft_run r
				 LEFT JOIN processors_ft_run p ON p.run_id = r.run_id
				 WHERE r.sequence_id = ?
				 ORDER BY r.sequence_order`), seq)
			if err != nil {
				return err
			}
			defer func() { _ = rows.Close() }()

			type runRow struct {
				id              string
				order, sessions int
				aiCalls, dur    int
				outcome         *int
				cost            float64
			}
			var list []runRow
			for rows.Next() {
				var r runRow
				if err := rows.Scan(&r.id, &r.order, &r.outcome, &r.sessions, &r.aiCalls, &r.cost, &r.dur); err != nil {
					return err
				}
				list = append(list, r)
			}
			if err := rows.Err(); err != nil {
				return err
			}

			for _, r := range list {
				ew.printf("  [%d] run %s  %s  sessions=%d  ai_calls=%d  cost=%.4f  dur=%dms\n",
					r.order, short(r.id), outcomeLabel(r.outcome), r.sessions, r.aiCalls, r.cost, r.dur)
				printRunFacts(ctx, db, ew, r.id)
			}
			return ew.err
		},
	}
	cmd.Flags().StringVar(&store, "store", defaultStore, "path to the fact store")
	return cmd
}

// printRunFacts lists the tool facts (ai calls, connector crossings) under a run.
func printRunFacts(ctx context.Context, db *storage.DB, ew *errWriter, runID string) {
	ai, err := db.QueryContext(ctx, db.Rebind(
		`SELECT model, mode, tokens_in, tokens_out, cost FROM ai_ft_call WHERE run_id = ?`), runID)
	if err == nil {
		defer func() { _ = ai.Close() }()
		for ai.Next() {
			var model, mode string
			var ti, to int
			var cost float64
			if ai.Scan(&model, &mode, &ti, &to, &cost) == nil {
				ew.printf("        ai    %s/%s tokens=%d/%d cost=%.4f\n", model, mode, ti, to, cost)
			}
		}
	}
	cr, err := db.QueryContext(ctx, db.Rebind(
		`SELECT target, direction, row_count FROM connector_ft_crossing WHERE run_id = ?`), runID)
	if err == nil {
		defer func() { _ = cr.Close() }()
		for cr.Next() {
			var target, dir string
			var rowCount int
			if cr.Scan(&target, &dir, &rowCount) == nil {
				ew.printf("        conn  %s %s rows=%d\n", dir, target, rowCount)
			}
		}
	}
}

// resolveSequence accepts a full id or an unambiguous short prefix (so the ids
// printed by `wxg runs` can be pasted directly).
func resolveSequence(ctx context.Context, db *storage.DB, idOrPrefix string) (string, error) {
	var full string
	err := db.QueryRowContext(ctx, db.Rebind(
		`SELECT sequence_id FROM wuxing_ft_sequence WHERE sequence_id = ?`), idOrPrefix).Scan(&full)
	if err == nil {
		return full, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}

	rows, err := db.QueryContext(ctx, db.Rebind(
		`SELECT sequence_id FROM wuxing_ft_sequence WHERE sequence_id LIKE ?`), idOrPrefix+"%")
	if err != nil {
		return "", err
	}
	defer func() { _ = rows.Close() }()
	var matches []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return "", err
		}
		matches = append(matches, s)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("sequence %q not found", idOrPrefix)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("sequence prefix %q is ambiguous (%d matches)", idOrPrefix, len(matches))
	}
}

func openStore(path string) (*storage.DB, error) {
	resolved, err := resolveStore(path)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(resolved); err != nil {
		return nil, fmt.Errorf("fact store not found at %s — start the daemon or pass --store", resolved)
	}
	return storage.OpenSQLite(resolved)
}

func resolveStore(path string) (string, error) {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	return path, nil
}

func outcomeLabel(o *int) string {
	if o == nil {
		return "running"
	}
	switch *o {
	case 1:
		return "success"
	case 2:
		return "failure"
	case 3:
		return "cancelled"
	default:
		return fmt.Sprintf("?%d", *o)
	}
}

func closedSuffix(closed *string) string {
	if closed == nil {
		return ""
	}
	return "  closed=" + *closed
}

func short(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
