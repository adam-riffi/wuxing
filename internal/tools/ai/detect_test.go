package ai

import (
	"errors"
	"strings"
	"testing"
)

// fakePath makes lookPath report only the named binaries as installed.
func fakePath(t *testing.T, installed ...string) {
	t.Helper()
	set := make(map[string]bool, len(installed))
	for _, b := range installed {
		set[b] = true
	}
	prev := lookPath
	lookPath = func(bin string) (string, error) {
		if set[bin] {
			return "/usr/bin/" + bin, nil
		}
		return "", errors.New("not found")
	}
	t.Cleanup(func() { lookPath = prev })
}

func TestDetect_FindsInstalledInPreferenceOrder(t *testing.T) {
	// gemini + codex installed; codex is higher preference, so it leads.
	fakePath(t, "gemini", "codex")

	found := Detect()
	if len(found) != 2 {
		t.Fatalf("expected 2 detected, got %d: %+v", len(found), found)
	}
	if found[0].Name != "codex" {
		t.Errorf("preference order: got %q first, want codex", found[0].Name)
	}
	if found[0].Path != "/usr/bin/codex" {
		t.Errorf("resolved path: %q", found[0].Path)
	}

	d, ok := DetectDefault()
	if !ok || d.Name != "codex" {
		t.Errorf("DetectDefault: %+v ok=%v", d, ok)
	}
}

func TestDetect_NoneInstalled(t *testing.T) {
	fakePath(t) // nothing installed
	if found := Detect(); len(found) != 0 {
		t.Errorf("expected nothing detected, got %+v", found)
	}
	if _, ok := DetectDefault(); ok {
		t.Error("DetectDefault should report false when nothing is installed")
	}
}

func TestResolveAgent_AutoDetect(t *testing.T) {
	t.Setenv("WUXING_AI_AGENT_COMMAND", "") // no manual override
	fakePath(t, "claude")

	ag, label, err := ResolveAgent("")
	if err != nil {
		t.Fatalf("ResolveAgent: %v", err)
	}
	if label != "claude" || ag == nil {
		t.Errorf("auto-detect: label=%q ag=%v", label, ag)
	}
}

func TestResolveAgent_ExplicitNotInstalled(t *testing.T) {
	fakePath(t, "codex")
	if _, _, err := ResolveAgent("gemini"); err == nil || !strings.Contains(err.Error(), "not on PATH") {
		t.Errorf("expected a not-on-PATH error for gemini, got %v", err)
	}
}

func TestResolveAgent_UnknownName(t *testing.T) {
	fakePath(t, "codex")
	if _, _, err := ResolveAgent("llama"); err == nil || !strings.Contains(err.Error(), "unknown agent") {
		t.Errorf("expected unknown-agent error, got %v", err)
	}
}

func TestResolveAgent_NoneDetected(t *testing.T) {
	t.Setenv("WUXING_AI_AGENT_COMMAND", "")
	fakePath(t) // nothing installed
	if _, _, err := ResolveAgent(""); err == nil || !strings.Contains(err.Error(), "no agent CLI detected") {
		t.Errorf("expected no-agent-detected error, got %v", err)
	}
}

func TestResolveAgent_EnvOverrideWins(t *testing.T) {
	t.Setenv("WUXING_AI_AGENT_COMMAND", "my-agent")
	fakePath(t) // nothing auto-detected, but the override should still resolve
	ag, label, err := ResolveAgent("")
	if err != nil {
		t.Fatalf("env override: %v", err)
	}
	if label != "my-agent" || ag == nil {
		t.Errorf("override: label=%q", label)
	}
}

func TestCLIAgent_ImplementsBothInterfaces(t *testing.T) {
	var _ Backend = (*CLIAgent)(nil)
	var _ AgentBackend = (*CLIAgent)(nil)
}
