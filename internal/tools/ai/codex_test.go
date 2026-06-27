package ai

import (
	"context"
	"os"
	"testing"
	"time"
)

// codexHelper points a CodexCLI at this test binary running as the fake CLI,
// reading the prompt from its final argument (where `codex exec` would get it).
func codexHelper() *CodexCLI {
	return &CodexCLI{
		Command: os.Args[0],
		Args:    []string{"-test.run=TestHelperProcess", "--"},
		Env:     []string{"WUXING_HELPER_MODE=ok", "WUXING_HELPER_FROM_ARG=1"},
	}
}

func TestCodexCLI_Infer(t *testing.T) {
	res, err := codexHelper().Infer(context.Background(), Request{Prompt: "who was gorbachev"})
	if err != nil {
		t.Fatalf("Infer: %v", err)
	}
	if got := echoOf(t, res.Output); got != "who was gorbachev" {
		t.Errorf("prompt round-trip via codex exec: got %q", got)
	}
}

func TestCodexCLI_RunAgent(t *testing.T) {
	res, err := codexHelper().RunAgent(context.Background(), AgentRequest{Brief: "build a thing"})
	if err != nil {
		t.Fatal(err)
	}
	if got := echoOf(t, res.Output); got != "build a thing" {
		t.Errorf("brief round-trip: got %q", got)
	}
	if res.TurnCount != 1 {
		t.Errorf("TurnCount: got %d want 1", res.TurnCount)
	}
}

func TestCodexCLI_ImplementsInterfaces(t *testing.T) {
	var _ Backend = (*CodexCLI)(nil)
	var _ AgentBackend = (*CodexCLI)(nil)
}

func TestCodexFromEnv(t *testing.T) {
	t.Setenv("WUXING_CODEX_COMMAND", "/opt/codex")
	t.Setenv("WUXING_CODEX_ARGS", "exec --json")
	t.Setenv("WUXING_CODEX_MODEL", "o4")
	t.Setenv("WUXING_CODEX_TIMEOUT_SECONDS", "90")

	c := CodexFromEnv()
	if c.Command != "/opt/codex" || len(c.Args) != 2 || c.Args[1] != "--json" {
		t.Errorf("command/args: %+v", c)
	}
	if c.Model != "o4" || c.Timeout != 90*time.Second {
		t.Errorf("model/timeout: %+v", c)
	}
}

func TestCodexFromEnv_Defaults(t *testing.T) {
	t.Setenv("WUXING_CODEX_COMMAND", "")
	t.Setenv("WUXING_CODEX_ARGS", "")
	c := CodexFromEnv()
	if c.Command != "codex" || len(c.Args) != 2 || c.Args[0] != "exec" || c.Args[1] != "--skip-git-repo-check" {
		t.Errorf("defaults should be `codex exec --skip-git-repo-check`: %+v", c)
	}
}
