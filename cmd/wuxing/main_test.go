package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
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
	if err := run(ctx, path, storePath, "", log); err != nil {
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

func TestRun_BadManifest(t *testing.T) {
	log := zerolog.New(&strings.Builder{})
	storePath := filepath.Join(t.TempDir(), "wuxing.db")
	err := run(context.Background(), filepath.Join(t.TempDir(), "missing.yml"), storePath, "", log)
	if err == nil {
		t.Fatal("run: expected error for missing manifest, got nil")
	}
}
