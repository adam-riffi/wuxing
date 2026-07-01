package cfg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeService(t *testing.T, root, name, cfgName, body string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, cfgName), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

const validCfg = `
name: mtg
envelope: { request: 1 }
workflow:
  - id: check
    tool: ai
    operation: infer
`

func TestLoadDir(t *testing.T) {
	root := t.TempDir()
	writeService(t, root, "mtg", "cfg.yml", validCfg)
	writeService(t, root, "unnamed", "service.yml", "envelope: { request: 1 }")
	// a folder without a cfg file is skipped, not an error
	if err := os.MkdirAll(filepath.Join(root, "scaffold"), 0o750); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadDir(root, DefaultVocabulary())
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 services, got %d", len(loaded))
	}
	byName := map[string]Loaded{}
	for _, l := range loaded {
		byName[l.Service.Name] = l
	}
	if _, ok := byName["mtg"]; !ok {
		t.Error("mtg not loaded")
	}
	// the folder names the service when the cfg doesn't
	if _, ok := byName["unnamed"]; !ok {
		t.Errorf("folder-name default missing: %v", byName)
	}
	if dir := byName["mtg"].Dir; !strings.HasSuffix(dir, "mtg") {
		t.Errorf("Dir should be the service folder: %q", dir)
	}
}

func TestLoadDir_InvalidCfgFailsWholeLoad(t *testing.T) {
	root := t.TempDir()
	writeService(t, root, "good", "cfg.yml", validCfg)
	writeService(t, root, "bad", "cfg.yml", `
name: bad
envelope: { request: 1 }
workflow:
  - id: x
    tool: nosuch
    operation: nope
`)
	if _, err := LoadDir(root, DefaultVocabulary()); err == nil {
		t.Fatal("an invalid cfg must fail the whole load (no half-loaded boot)")
	}
}

func TestLoadDir_MissingRoot(t *testing.T) {
	if _, err := LoadDir(filepath.Join(t.TempDir(), "nope"), DefaultVocabulary()); err == nil {
		t.Error("missing services dir should error")
	}
}

func TestValidate_ScriptSteps(t *testing.T) {
	vocab := DefaultVocabulary()

	ok := &Service{
		Name:     "s",
		Envelope: Envelope{Request: 1},
		Workflow: []Step{{ID: "check", Script: "scripts/check.py"}},
	}
	if err := ok.Validate(vocab); err != nil {
		t.Errorf("script step should validate without the vocabulary: %v", err)
	}
	if !ok.Workflow[0].IsScript() {
		t.Error("IsScript should be true")
	}

	both := &Service{
		Name:     "s",
		Envelope: Envelope{Request: 1},
		Workflow: []Step{{ID: "x", Script: "a.py", Tool: "ai", Operation: "infer"}},
	}
	if err := both.Validate(vocab); err == nil {
		t.Error("a step declaring both script and tool must fail")
	}

	for _, bad := range []string{"/abs/path.py", "../escape.py", "a/../../b.py"} {
		s := &Service{
			Name:     "s",
			Envelope: Envelope{Request: 1},
			Workflow: []Step{{ID: "x", Script: bad}},
		}
		if err := s.Validate(vocab); err == nil {
			t.Errorf("script path %q should be rejected", bad)
		}
	}
}
