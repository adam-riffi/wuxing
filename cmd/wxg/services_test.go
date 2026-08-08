package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adam-riffi/wuxing/internal/storage"
)

// seedAdminStore creates a fresh fact store with one service record and returns
// its path. The connection is closed so the command opens its own.
func seedAdminStore(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "wuxing.db")
	db, err := storage.OpenSQLite(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	err = db.RecordService(context.Background(), storage.ServiceRecord{
		ServiceID: "testsvc",
		Name:      "testsvc",
		Version:   "1",
		Status:    "live",
		CfgJSON:   `{"name":"testsvc"}`,
	})
	if err != nil {
		t.Fatalf("RecordService: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestServicesCmd(t *testing.T) {
	path := seedAdminStore(t)
	out := runCmd(t, "services", "--store", path)
	for _, want := range []string{"testsvc", "live", "1"} {
		if !strings.Contains(out, want) {
			t.Errorf("services output missing %q:\n%s", want, out)
		}
	}
}

func TestServicesCmd_Empty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wuxing.db")
	db, err := storage.OpenSQLite(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	out := runCmd(t, "services", "--store", path)
	if !strings.Contains(out, "no services") {
		t.Errorf("empty services: expected 'no services' message, got:\n%s", out)
	}
}

func TestServicesCmd_MissingStore(t *testing.T) {
	root := newRootCmd()
	root.SetArgs([]string{"services", "--store", filepath.Join(t.TempDir(), "nope.db")})
	if err := root.Execute(); err == nil {
		t.Error("expected error when store does not exist")
	}
}
