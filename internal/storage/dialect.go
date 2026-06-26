package storage

import (
	"strconv"
	"strings"
)

// Dialect describes a SQL backend: its database/sql driver, its bind-parameter
// style, and which embedded migration set it uses. The same access-layer queries
// (written with `?` placeholders) run on either backend via Rebind.
type Dialect struct {
	Name       string // "sqlite" | "postgres"
	Driver     string // database/sql driver name
	dollarBind bool   // true => $1,$2 placeholders (postgres); false => ? (sqlite)
	migrations string // subdirectory under migrations/
}

// SQLite is the embedded, zero-ops backend (single-node, dev, tests).
var SQLite = Dialect{Name: "sqlite", Driver: "sqlite", dollarBind: false, migrations: "sqlite"}

// Postgres is the server backend (production, 3rd-party-accessible via DBeaver,
// Tableau, etc.). The DSN is supplied by the operator (local Postgres, Supabase, …).
var Postgres = Dialect{Name: "postgres", Driver: "pgx", dollarBind: true, migrations: "postgres"}

// Rebind converts a query's `?` placeholders to the dialect's style. For sqlite
// it is a no-op; for postgres it rewrites them to $1, $2, … (in order).
func (d Dialect) Rebind(query string) string {
	if !d.dollarBind {
		return query
	}
	var b strings.Builder
	b.Grow(len(query) + 8)
	n := 0
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			continue
		}
		b.WriteByte(query[i])
	}
	return b.String()
}
