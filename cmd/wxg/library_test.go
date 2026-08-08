package main

import (
	"bytes"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adam-riffi/wuxing/internal/control"
)

// minimalCfgYML is the smallest valid service cfg for test fixtures.
const minimalCfgYML = `name: testsvc
envelope:
  request: 1
`

// svcDir creates a temp service folder containing a minimal cfg.yml.
func svcDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "testsvc")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("svcDir mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cfg.yml"), []byte(minimalCfgYML), 0o644); err != nil {
		t.Fatalf("svcDir write: %v", err)
	}
	return dir
}

func TestLibraryIndexCmd(t *testing.T) {
	var gotCfgJSON string
	catalog := control.CatalogFuncs{
		Index: func(cfgJSON string) (control.CatalogEntry, error) {
			gotCfgJSON = cfgJSON
			return control.CatalogEntry{Name: "testsvc", Status: "live"}, nil
		},
	}
	srv := httptest.NewServer(control.Handler(
		func() control.State { return control.State{} },
		func(control.RunRequest) (control.RunStarted, error) { return control.RunStarted{}, nil },
		catalog,
	))
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "http://")

	dir := svcDir(t)
	out := runCmd(t, "library", "index", dir, "--api", addr)
	if !strings.Contains(out, "indexed: testsvc") {
		t.Errorf("index output missing service name:\n%s", out)
	}
	if gotCfgJSON == "" {
		t.Error("daemon must receive cfg_json")
	}
	if !strings.Contains(gotCfgJSON, "testsvc") {
		t.Errorf("cfg_json must contain service name, got: %s", gotCfgJSON)
	}
}

func TestLibraryIndexCmd_DaemonDown(t *testing.T) {
	dir := svcDir(t)
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"library", "index", dir, "--api", "127.0.0.1:0"})
	if err := root.Execute(); err == nil {
		t.Error("expected error when daemon is unreachable")
	}
}

func TestLibraryIndexCmd_BadFolder(t *testing.T) {
	dir := svcDir(t)
	catalog := control.CatalogFuncs{
		Index: func(string) (control.CatalogEntry, error) { return control.CatalogEntry{}, nil },
	}
	srv := httptest.NewServer(control.Handler(
		func() control.State { return control.State{} },
		func(control.RunRequest) (control.RunStarted, error) { return control.RunStarted{}, nil },
		catalog,
	))
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "http://")

	// Point at a folder that has no cfg file.
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"library", "index", dir + "/no-such", "--api", addr})
	if err := root.Execute(); err == nil {
		t.Errorf("expected error for missing cfg folder; output:\n%s", out.String())
	}
}

func TestLibraryDeindexCmd(t *testing.T) {
	var gotName string
	catalog := control.CatalogFuncs{
		Deindex: func(name string) error {
			gotName = name
			return nil
		},
	}
	srv := httptest.NewServer(control.Handler(
		func() control.State { return control.State{} },
		func(control.RunRequest) (control.RunStarted, error) { return control.RunStarted{}, nil },
		catalog,
	))
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "http://")

	out := runCmd(t, "library", "deindex", "testsvc", "--api", addr)
	if gotName != "testsvc" {
		t.Errorf("daemon must receive service name %q, got %q", "testsvc", gotName)
	}
	if !strings.Contains(out, "deindexed: testsvc") {
		t.Errorf("deindex output missing confirmation:\n%s", out)
	}
}

func TestLibraryDeindexCmd_DrainError(t *testing.T) {
	catalog := control.CatalogFuncs{
		Deindex: func(string) error { return errors.New("has running sessions") },
	}
	srv := httptest.NewServer(control.Handler(
		func() control.State { return control.State{} },
		func(control.RunRequest) (control.RunStarted, error) { return control.RunStarted{}, nil },
		catalog,
	))
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "http://")

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"library", "deindex", "testsvc", "--api", addr})
	if err := root.Execute(); err == nil {
		t.Errorf("expected a drain error; output:\n%s", out.String())
	}
}
