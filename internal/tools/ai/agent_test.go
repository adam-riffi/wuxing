package ai

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/adam-riffi/wuxing/internal/kernel/bus"
)

// TestHelperProcess is not a real test: the CLIAgent tests re-exec this binary
// as a fake agent CLI. It does nothing unless WUXING_HELPER_MODE is set, so a
// normal `go test` run just passes it. It must os.Exit so the test framework's
// trailing "PASS/ok" output never reaches the captured stdout artifact.
func TestHelperProcess(t *testing.T) {
	mode := os.Getenv("WUXING_HELPER_MODE")
	if mode == "" {
		return
	}
	switch mode {
	case "fail":
		_, _ = io.WriteString(os.Stderr, "boom")
		os.Exit(3)
	case "hang":
		time.Sleep(10 * time.Second)
		os.Exit(0)
	}

	// Read the brief from wherever the spec delivered it.
	var brief string
	switch {
	case os.Getenv("WUXING_HELPER_PROMPT_FILE") != "":
		b, _ := os.ReadFile(os.Getenv("WUXING_HELPER_PROMPT_FILE"))
		brief = string(b)
	case os.Getenv("WUXING_HELPER_FROM_ARG") == "1":
		brief = os.Args[len(os.Args)-1]
	default:
		b, _ := io.ReadAll(os.Stdin)
		brief = string(b)
	}

	artifact, _ := json.Marshal(map[string]string{"echo": brief})
	if out := os.Getenv("WUXING_HELPER_OUT_FILE"); out != "" {
		_ = os.WriteFile(out, artifact, 0o600)
	} else {
		_, _ = os.Stdout.Write(artifact)
	}
	if usage := os.Getenv("WUXING_HELPER_USAGE_FILE"); usage != "" {
		_ = os.WriteFile(usage, []byte(`{"tokens_in":11,"tokens_out":22,"cost":0.5,"turns":3}`), 0o600)
	}
	os.Exit(0)
}

// helperSpec builds a spec whose command is this test binary running as the fake
// CLI in the given mode.
func helperSpec(mode string, env ...string) AgentSpec {
	return AgentSpec{
		Command: os.Args[0],
		Args:    []string{"-test.run=TestHelperProcess", "--"},
		Env:     append([]string{"WUXING_HELPER_MODE=" + mode}, env...),
	}
}

func echoOf(t *testing.T, out json.RawMessage) string {
	t.Helper()
	var got map[string]string
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("artifact is not the expected JSON: %s", out)
	}
	return got["echo"]
}

func TestCLIAgent_StdinStdout(t *testing.T) {
	ag := NewCLIAgent(helperSpec("ok"))
	res, err := ag.RunAgent(context.Background(), AgentRequest{Brief: "hello mtg"})
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	if got := echoOf(t, res.Output); got != "hello mtg" {
		t.Errorf("brief round-trip via stdin/stdout: got %q", got)
	}
	if res.TurnCount != 1 {
		t.Errorf("default TurnCount: got %d want 1", res.TurnCount)
	}
}

func TestCLIAgent_PromptArg(t *testing.T) {
	spec := helperSpec("ok", "WUXING_HELPER_FROM_ARG=1")
	spec.PromptVia = PromptArg
	res, err := NewCLIAgent(spec).RunAgent(context.Background(), AgentRequest{Brief: "as an argument"})
	if err != nil {
		t.Fatal(err)
	}
	if got := echoOf(t, res.Output); got != "as an argument" {
		t.Errorf("brief via arg: got %q", got)
	}
}

func TestCLIAgent_FileInFileOut(t *testing.T) {
	spec := helperSpec("ok", "WUXING_HELPER_PROMPT_FILE=prompt.txt", "WUXING_HELPER_OUT_FILE=out.json")
	spec.PromptVia = PromptFile
	spec.PromptFile = "prompt.txt"
	spec.OutputVia = OutputFile
	spec.OutputFile = "out.json"
	res, err := NewCLIAgent(spec).RunAgent(context.Background(), AgentRequest{Brief: "from a file"})
	if err != nil {
		t.Fatal(err)
	}
	if got := echoOf(t, res.Output); got != "from a file" {
		t.Errorf("brief via file: got %q", got)
	}
}

func TestCLIAgent_UsageSidecar(t *testing.T) {
	spec := helperSpec("ok", "WUXING_HELPER_USAGE_FILE=usage.json")
	spec.UsageFile = "usage.json"
	res, err := NewCLIAgent(spec).RunAgent(context.Background(), AgentRequest{Brief: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if res.TokensIn != 11 || res.TokensOut != 22 || res.Cost != 0.5 || res.TurnCount != 3 {
		t.Errorf("usage from sidecar not applied: %+v", res)
	}
}

func TestCLIAgent_Failure(t *testing.T) {
	_, err := NewCLIAgent(helperSpec("fail")).RunAgent(context.Background(), AgentRequest{Brief: "x"})
	if err == nil {
		t.Fatal("expected an error when the agent exits non-zero")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error should carry the agent's stderr: %v", err)
	}
}

func TestCLIAgent_Timeout(t *testing.T) {
	spec := helperSpec("hang")
	spec.Timeout = 200 * time.Millisecond
	_, err := NewCLIAgent(spec).RunAgent(context.Background(), AgentRequest{Brief: "x"})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected a timeout error, got %v", err)
	}
}

func TestCLIAgent_NoCommand(t *testing.T) {
	if _, err := NewCLIAgent(AgentSpec{}).RunAgent(context.Background(), AgentRequest{Brief: "x"}); err == nil {
		t.Error("a spec with no command should error")
	}
}

// fakeAgent is a stub AgentBackend for the Tool wiring tests.
type fakeAgent struct {
	res      AgentResult
	err      error
	gotBrief string
}

func (f *fakeAgent) RunAgent(_ context.Context, req AgentRequest) (AgentResult, error) {
	f.gotBrief = req.Brief
	return f.res, f.err
}

func agentCall(payload string) bus.Envelope {
	return bus.NewCall(stamp(), "ai", "agent", json.RawMessage(payload))
}

func TestTool_Agent_MetersCall(t *testing.T) {
	fa := &fakeAgent{res: AgentResult{
		Output: json.RawMessage(`{"ok":true}`),
		Model:  "hermes", TokensIn: 5, TokensOut: 7, Cost: 0.1, TurnCount: 4,
	}}
	meter := &recMeter{}
	tool := New(nil, meter).WithAgent(fa)

	ret := tool.Handler()(context.Background(), agentCall(`{"brief":"make a thing"}`))
	if ret.Error != "" {
		t.Fatalf("agent call errored: %s", ret.Error)
	}
	if string(ret.Payload) != `{"ok":true}` {
		t.Errorf("artifact: got %s", ret.Payload)
	}
	if fa.gotBrief != "make a thing" {
		t.Errorf("brief not passed through: %q", fa.gotBrief)
	}
	if len(meter.facts) != 1 {
		t.Fatalf("expected one fact, got %d", len(meter.facts))
	}
	f := meter.facts[0]
	if f.Mode != "agent" || f.Model != "hermes" || f.TokensIn != 5 || f.TurnCount != 4 {
		t.Errorf("agent fact: %+v", f)
	}
}

func TestTool_Agent_NoBackend(t *testing.T) {
	tool := New(nil, nil) // no WithAgent
	ret := tool.Handler()(context.Background(), agentCall(`{"brief":"x"}`))
	if ret.Error == "" {
		t.Error("agent op without a backend should error")
	}
}

func TestTool_Agent_NoBrief(t *testing.T) {
	tool := New(nil, nil).WithAgent(&fakeAgent{})
	ret := tool.Handler()(context.Background(), agentCall(`{}`))
	if ret.Error == "" {
		t.Error("agent op with no brief should error")
	}
}

func TestAgentSpecFromEnv(t *testing.T) {
	t.Setenv("WUXING_AI_AGENT_COMMAND", "hermes")
	t.Setenv("WUXING_AI_AGENT_ARGS", "--headless --json")
	t.Setenv("WUXING_AI_AGENT_PROMPT_VIA", "stdin")
	t.Setenv("WUXING_AI_AGENT_TIMEOUT_SECONDS", "120")
	t.Setenv("WUXING_AI_AGENT_MODEL", "hermes-4")

	spec, err := AgentSpecFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if spec.Command != "hermes" || len(spec.Args) != 2 || spec.Args[1] != "--json" {
		t.Errorf("spec args: %+v", spec)
	}
	if spec.Timeout != 120*time.Second || spec.Model != "hermes-4" {
		t.Errorf("spec timeout/model: %+v", spec)
	}
}

func TestAgentSpecFromEnv_RequiresCommand(t *testing.T) {
	t.Setenv("WUXING_AI_AGENT_COMMAND", "")
	if _, err := AgentSpecFromEnv(); err == nil {
		t.Error("expected an error when WUXING_AI_AGENT_COMMAND is unset")
	}
}
