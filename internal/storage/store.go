// Package storage opens and migrates the wuxing fact store — the append-only
// SQLite database that holds the platform's own record (the lineage spine and,
// later, the per-tool detail tables). It owns the schema and the connection; the
// facts and dims subpackages read and write through it.
package storage

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // pure-Go sqlite driver, registered as "sqlite"
)

// DB is a handle to the fact store.
type DB struct {
	*sql.DB
}

// Open opens (creating if absent) the sqlite database at path, enables foreign
// keys, and applies any pending migrations. Use ":memory:" for an ephemeral
// store in tests, though a file in t.TempDir() exercises the real driver path.
func Open(path string) (*DB, error) {
	// Pragmas are set in the DSN so they apply to every pooled connection.
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", path)
	sdb, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("storage: open %q: %w", path, err)
	}
	// sqlite tolerates one writer; cap the pool so concurrent writes serialize
	// rather than racing for the file lock.
	sdb.SetMaxOpenConns(1)

	if err := sdb.Ping(); err != nil {
		_ = sdb.Close()
		return nil, fmt.Errorf("storage: ping %q: %w", path, err)
	}

	db := &DB{sdb}
	if err := db.migrate(); err != nil {
		_ = sdb.Close()
		return nil, err
	}
	return db, nil
}
