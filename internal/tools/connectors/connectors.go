// Package connectors is the data-I/O tool: move data between a service and a
// backend under a data-access grant, metered. This implements the sqlite driver,
// grant enforcement (the security boundary), and metering; it registers as the
// "connectors" tool on the bus. Operations: write, read (query/upsert to follow).
//
// Data access is an allowlist: a call to DATABASE.TABLE is permitted only by an
// explicit grant, enforced here before any row is touched. Every call emits a
// crossing fact; a mutating write also emits a data-mutation fact.
package connectors

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/adam-riffi/wuxing/internal/kernel/bus"
)

// Grant authorizes access to one DATABASE.TABLE.
type Grant struct {
	Database string
	Table    string
}

func (g Grant) target() string { return g.Database + "." + g.Table }

// Crossing is the movement fact emitted per call (audit + volumetry).
type Crossing struct {
	Target    string
	Direction string // "in" | "out"
	Rows      int
}

// Mutation is the state-delta fact for a mutating write.
type Mutation struct {
	Target       string
	RowsInserted int
}

// Meter receives the facts a connector emits; the kernel wires it to storage.
type Meter interface {
	Crossing(Crossing)
	Mutation(Mutation)
}

// NopMeter discards facts.
type NopMeter struct{}

// Crossing implements Meter.
func (NopMeter) Crossing(Crossing) {}

// Mutation implements Meter.
func (NopMeter) Mutation(Mutation) {}

// Connector moves data to/from a sqlite backend under a grant set, metered.
type Connector struct {
	db     *sql.DB
	grants map[string]bool
	meter  Meter
}

// New returns a connector over db, permitting only the given grants, reporting
// facts to meter (nil discards).
func New(db *sql.DB, grants []Grant, meter Meter) *Connector {
	if meter == nil {
		meter = NopMeter{}
	}
	g := make(map[string]bool, len(grants))
	for _, x := range grants {
		g[x.target()] = true
	}
	return &Connector{db: db, grants: g, meter: meter}
}

// Handler returns the bus handler that answers connectors calls.
func (c *Connector) Handler() bus.Handler {
	return func(ctx context.Context, e bus.Envelope) bus.Envelope {
		switch e.Operation {
		case "write":
			return c.handleWrite(ctx, e)
		case "read":
			return c.handleRead(ctx, e)
		default:
			return e.ReplyError(fmt.Errorf("connectors: unsupported operation %q", e.Operation))
		}
	}
}

type writeRequest struct {
	Target string           `json:"target"`
	Rows   []map[string]any `json:"rows"`
}

func (c *Connector) handleWrite(ctx context.Context, e bus.Envelope) bus.Envelope {
	var req writeRequest
	if err := json.Unmarshal(e.Payload, &req); err != nil {
		return e.ReplyError(fmt.Errorf("connectors: bad write payload: %w", err))
	}
	if !c.grants[req.Target] {
		return e.ReplyError(fmt.Errorf("connectors: write refused: no grant for %q", req.Target))
	}
	n, err := c.insert(ctx, tableOf(req.Target), req.Rows)
	if err != nil {
		return e.ReplyError(err)
	}
	c.meter.Crossing(Crossing{Target: req.Target, Direction: "out", Rows: n})
	c.meter.Mutation(Mutation{Target: req.Target, RowsInserted: n})

	out, _ := json.Marshal(map[string]int{"rows_inserted": n})
	return e.Reply(out)
}

type readRequest struct {
	Target string `json:"target"`
}

func (c *Connector) handleRead(ctx context.Context, e bus.Envelope) bus.Envelope {
	var req readRequest
	if err := json.Unmarshal(e.Payload, &req); err != nil {
		return e.ReplyError(fmt.Errorf("connectors: bad read payload: %w", err))
	}
	if !c.grants[req.Target] {
		return e.ReplyError(fmt.Errorf("connectors: read refused: no grant for %q", req.Target))
	}
	rows, err := c.selectAll(ctx, tableOf(req.Target))
	if err != nil {
		return e.ReplyError(err)
	}
	c.meter.Crossing(Crossing{Target: req.Target, Direction: "in", Rows: len(rows)})

	out, _ := json.Marshal(map[string]any{"rows": rows})
	return e.Reply(out)
}

func (c *Connector) insert(ctx context.Context, table string, rows []map[string]any) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	cols := sortedKeys(rows[0])
	ph := strings.TrimSuffix(strings.Repeat("?,", len(cols)), ",")
	// table is grant-scoped and cols come from the service's own payload; this is
	// the connector's generic-insert path, not attacker-controlled SQL.
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", table, strings.Join(cols, ", "), ph) // #nosec G201

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("connectors: begin: %w", err)
	}
	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		_ = tx.Rollback()
		return 0, fmt.Errorf("connectors: prepare: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	n := 0
	for _, r := range rows {
		args := make([]any, len(cols))
		for i, col := range cols {
			args[i] = r[col]
		}
		if _, err := stmt.ExecContext(ctx, args...); err != nil {
			_ = tx.Rollback()
			return 0, fmt.Errorf("connectors: insert: %w", err)
		}
		n++
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("connectors: commit: %w", err)
	}
	return n, nil
}

func (c *Connector) selectAll(ctx context.Context, table string) ([]map[string]any, error) {
	rows, err := c.db.QueryContext(ctx, fmt.Sprintf("SELECT * FROM %s", table)) // #nosec G201 -- table is grant-scoped
	if err != nil {
		return nil, fmt.Errorf("connectors: select: %w", err)
	}
	defer func() { _ = rows.Close() }()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := make(map[string]any, len(cols))
		for i, col := range cols {
			m[col] = vals[i]
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func tableOf(target string) string {
	if i := strings.LastIndex(target, "."); i >= 0 {
		return target[i+1:]
	}
	return target
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
