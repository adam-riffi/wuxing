package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func writeManifest(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "boot.yml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestLoad_Valid(t *testing.T) {
	path := writeManifest(t, `
version: 0
tools:
  - name: library
    package: internal/tools/library
    status: stub
  - name: ai
    package: internal/tools/ai
    status: stub
`)

	m, err := Load(path)
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}
	if got, want := len(m.Tools), 2; got != want {
		t.Fatalf("tools: got %d want %d", got, want)
	}
	if m.Tools[0].Name != "library" {
		t.Errorf("first tool: got %q want library", m.Tools[0].Name)
	}
}

func TestLoad_Errors(t *testing.T) {
	cases := map[string]string{
		"empty tools": `
version: 0
tools: []
`,
		"missing package": `
version: 0
tools:
  - name: library
`,
		"duplicate name": `
version: 0
tools:
  - name: library
    package: internal/tools/library
  - name: library
    package: internal/tools/other
`,
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(writeManifest(t, body)); err == nil {
				t.Fatalf("Load: expected error, got nil")
			}
		})
	}
}

func TestLoad_MissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.yml")); err == nil {
		t.Fatalf("Load: expected error for missing file, got nil")
	}
}
