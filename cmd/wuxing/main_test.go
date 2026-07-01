package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/adam-riffi/wuxing/internal/storage"
)

func TestRun_LoadsManifestThenStops(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "boot.yml")
	body := `
version: 0
tools:
  - name: library
    package: internal/tools/library
    status: stub
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	storePath := filepath.Join(dir, "wuxing.db")

	var buf strings.Builder
	log := zerolog.New(&buf)

	// A cancelled context makes run return as soon as it reaches the idle wait,
	// after the manifest is loaded and the kernel is assembled.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Empty api disables the control server so the test binds no port.
	if err := run(ctx, path, storePath, "", "", log); err != nil {
		t.Fatalf("run: unexpected error: %v", err)
	}

	out := buf.String()
	for _, want := range []string{"manifest loaded", "fact store opened", "kernel ready", "stopped cleanly", "library"} {
		if !strings.Contains(out, want) {
			t.Errorf("log output missing %q\n--- got ---\n%s", want, out)
		}
	}

	// The fact store file was created on boot.
	if _, err := os.Stat(storePath); err != nil {
		t.Errorf("fact store not created at %s: %v", storePath, err)
	}
}

func TestRun_LoadsServicesDir(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "boot.yml")
	manifestBody := "version: 0\ntools:\n  - name: library\n    package: internal/tools/library\n    status: stub\n"
	if err := os.WriteFile(manifest, []byte(manifestBody), 0o600); err != nil {
		t.Fatal(err)
	}

	// One valid service folder.
	svcDir := filepath.Join(dir, "services", "mtg")
	if err := os.MkdirAll(svcDir, 0o750); err != nil {
		t.Fatal(err)
	}
	cfgBody := `
name: mtg
envelope: { request: 1 }
triggers:
  - { kind: cron, spec: "0 9 * * *" }
workflow:
  - id: check
    tool: ai
    operation: infer
`
	if err := os.WriteFile(filepath.Join(svcDir, "cfg.yml"), []byte(cfgBody), 0o600); err != nil {
		t.Fatal(err)
	}

	storePath := filepath.Join(dir, "wuxing.db")
	var buf strings.Builder
	log := zerolog.New(&buf)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := run(ctx, manifest, storePath, "", filepath.Join(dir, "services"), log); err != nil {
		t.Fatalf("run: %v", err)
	}

	out := buf.String()
	for _, want := range []string{`"service":"mtg"`, "service registered", "services loaded"} {
		if !strings.Contains(out, want) {
			t.Errorf("log missing %q\n--- got ---\n%s", want, out)
		}
	}

	// The service's ID card persisted to the admin index.
	db, err := storage.OpenSQLite(storePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	rec, err := db.GetService(context.Background(), "mtg")
	if err != nil {
		t.Fatalf("admin index: %v", err)
	}
	if rec.Status != "live" || !strings.Contains(rec.CfgJSON, `"check"`) {
		t.Errorf("admin record: %+v", rec)
	}
}

func TestRun_InvalidServicesDirFailsBoot(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "boot.yml")
	manifestBody := "version: 0\ntools:\n  - name: library\n    package: internal/tools/library\n    status: stub\n"
	if err := os.WriteFile(manifest, []byte(manifestBody), 0o600); err != nil {
		t.Fatal(err)
	}
	svcDir := filepath.Join(dir, "services", "bad")
	if err := os.MkdirAll(svcDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(svcDir, "cfg.yml"), []byte("name: bad\nenvelope: { request: 0 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	log := zerolog.New(&strings.Builder{})
	err := run(context.Background(), manifest, filepath.Join(dir, "wuxing.db"), "", filepath.Join(dir, "services"), log)
	if err == nil {
		t.Fatal("an invalid service cfg must fail the boot")
	}
}

func TestRun_BadManifest(t *testing.T) {
	log := zerolog.New(&strings.Builder{})
	storePath := filepath.Join(t.TempDir(), "wuxing.db")
	err := run(context.Background(), filepath.Join(t.TempDir(), "missing.yml"), storePath, "", "", log)
	if err == nil {
		t.Fatal("run: expected error for missing manifest, got nil")
	}
}
