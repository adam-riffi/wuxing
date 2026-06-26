// Package storage opens and migrates the wuxing fact store — the append-only
// database that holds the platform's own record (the lineage spine and the
// per-tool detail tables). It supports two backends behind one Dialect: embedded
// SQLite (zero-ops, single-node, tests) and server Postgres (production, reachable
// from 3rd-party SQL tools). The backend is chosen by Config; the schema and the
// access-layer queries are shared, with placeholders rebound per dialect.
package storage

import (
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver "pgx" (postgres)
	_ "modernc.org/sqlite"             // database/sql driver "sqlite" (pure Go)
)

// Config selects and locates the fact-store backend.
type Config struct {
	Dialect Dialect
	DSN     string
}

// DB is a handle to the fact store, tagged with its dialect.
type DB struct {
	*sql.DB
	Dialect Dialect
}

// Rebind adapts a `?`-placeholder query to the store's dialect.
func (db *DB) Rebind(query string) string { return db.Dialect.Rebind(query) }

// Open connects to the configured backend and applies any pending migrations.
func Open(cfg Config) (*DB, error) {
	sdb, err := sql.Open(cfg.Dialect.Driver, cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("storage: open %s: %w", cfg.Dialect.Name, err)
	}
	// SQLite tolerates a single writer; cap the pool so concurrent writes
	// serialize rather than racing the file lock. Postgres handles its own pool.
	if cfg.Dialect.Name == SQLite.Name {
		sdb.SetMaxOpenConns(1)
	}

	if err := sdb.Ping(); err != nil {
		_ = sdb.Close()
		return nil, fmt.Errorf("storage: ping %s: %w", cfg.Dialect.Name, err)
	}

	db := &DB{DB: sdb, Dialect: cfg.Dialect}
	if err := db.migrate(); err != nil {
		_ = sdb.Close()
		return nil, err
	}
	return db, nil
}

// OpenSQLite opens (creating if absent) an embedded sqlite store at path, with
// foreign keys on. A convenience for single-node use and tests.
func OpenSQLite(path string) (*DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", path)
	return Open(Config{Dialect: SQLite, DSN: dsn})
}

// OpenPostgres opens a Postgres store at the given DSN (e.g.
// "postgres://user:pass@host:5432/wuxing?sslmode=require").
func OpenPostgres(dsn string) (*DB, error) {
	return Open(Config{Dialect: Postgres, DSN: dsn})
}
