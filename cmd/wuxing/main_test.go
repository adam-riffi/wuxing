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

	var buf strings.Builder
	log := zerolog.New(&buf)

	// A cancelled context makes run return as soon as it reaches the idle wait,
	// after the manifest has been loaded and logged.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := run(ctx, path, log); err != nil {
		t.Fatalf("run: unexpected error: %v", err)
	}

	out := buf.String()
	for _, want := range []string{"manifest loaded", "stopped cleanly", "library"} {
		if !strings.Contains(out, want) {
			t.Errorf("log output missing %q\n--- got ---\n%s", want, out)
		}
	}
}

func TestRun_BadManifest(t *testing.T) {
	log := zerolog.New(&strings.Builder{})
	err := run(context.Background(), filepath.Join(t.TempDir(), "missing.yml"), log)
	if err == nil {
		t.Fatal("run: expected error for missing manifest, got nil")
	}
}
