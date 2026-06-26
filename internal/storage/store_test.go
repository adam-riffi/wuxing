package storage

import (
	"path/filepath"
	"testing"
)

func openTemp(t *testing.T) *DB {
	t.Helper()
	db, err := OpenSQLite(filepath.Join(t.TempDir(), "wuxing.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestOpen_AppliesMigrations(t *testing.T) {
	db := openTemp(t)

	var outcomes int
	if err := db.QueryRow(`SELECT COUNT(*) FROM wuxing_dt_outcome`).Scan(&outcomes); err != nil {
		t.Fatalf("query outcomes: %v", err)
	}
	if outcomes != 3 {
		t.Errorf("dt_outcome rows: got %d want 3", outcomes)
	}

	var name string
	if err := db.QueryRow(`SELECT name FROM wuxing_dt_outcome WHERE outcome_id = 1`).Scan(&name); err != nil {
		t.Fatalf("query outcome 1: %v", err)
	}
	if name != "success" {
		t.Errorf("outcome 1: got %q want success", name)
	}
}

func TestMigrate_RecordsVersion(t *testing.T) {
	db := openTemp(t)
	var v string
	if err := db.QueryRow(`SELECT version FROM schema_migrations ORDER BY version LIMIT 1`).Scan(&v); err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if v != "0001_core_dimensions.sql" {
		t.Errorf("recorded version: got %q", v)
	}
}

func TestOpen_Idempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wuxing.db")

	db1, err := OpenSQLite(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	var before int
	if err := db1.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&before); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	_ = db1.Close()
	if before == 0 {
		t.Fatal("expected at least one migration to be applied")
	}

	// Reopening the same file must not re-apply migrations or error.
	db2, err := OpenSQLite(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = db2.Close() }()

	var after int
	if err := db2.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&after); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if after != before {
		t.Errorf("reopen re-applied migrations: before %d, after %d", before, after)
	}
}

func TestOpen_ForeignKeysEnabled(t *testing.T) {
	db := openTemp(t)
	var on int
	if err := db.QueryRow(`PRAGMA foreign_keys`).Scan(&on); err != nil {
		t.Fatalf("pragma: %v", err)
	}
	if on != 1 {
		t.Errorf("foreign_keys: got %d want 1", on)
	}
}
