package storage

import (
	"embed"
	"fmt"
	"sort"
	"strings"
	"time"
)

//go:embed migrations/sqlite/*.sql migrations/postgres/*.sql
var migrationFS embed.FS

// migrate applies every embedded migration for the store's dialect that is not
// yet recorded, in lexical order, each in its own transaction. Forward-only: a
// file, once applied, is never re-run or rolled back.
func (db *DB) migrate() error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("storage: ensure schema_migrations: %w", err)
	}

	dir := "migrations/" + db.Dialect.migrations
	entries, err := migrationFS.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("storage: read migrations %q: %w", dir, err)
	}
	versions := make([]string, 0, len(entries))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			versions = append(versions, e.Name())
		}
	}
	sort.Strings(versions)

	for _, v := range versions {
		var applied int
		if err := db.QueryRow(db.Rebind(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`), v).Scan(&applied); err != nil {
			return fmt.Errorf("storage: check migration %q: %w", v, err)
		}
		if applied > 0 {
			continue
		}
		if err := db.applyMigration(dir, v); err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) applyMigration(dir, version string) error {
	body, err := migrationFS.ReadFile(dir + "/" + version)
	if err != nil {
		return fmt.Errorf("storage: read migration %q: %w", version, err)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("storage: begin %q: %w", version, err)
	}
	if _, err := tx.Exec(string(body)); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("storage: apply %q: %w", version, err)
	}
	if _, err := tx.Exec(db.Rebind(`INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)`), version, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("storage: record %q: %w", version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("storage: commit %q: %w", version, err)
	}
	return nil
}
