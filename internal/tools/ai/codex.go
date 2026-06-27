package ai

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"
)

// CodexCLI drives the Codex CLI as wuxing's model backend. It satisfies BOTH
// Backend (infer) and AgentBackend (agent): Codex is itself agentic, so infer is
// a one-shot `codex exec <prompt>` and agent is the same call with a scratch dir
// the session can write into. It reuses the (tested) CLIAgent exec path.
//
// Default invocation: `codex exec "<prompt>"`, answer captured from stdout.
// Override the command/args/model/timeout via fields or WUXING_CODEX_* env.
type CodexCLI struct {
	Command string        // default "codex"
	Args    []string      // default ["exec"]; the prompt is appended as one arg
	Model   string        // model label recorded on the fact
	Timeout time.Duration // per-call wall-clock bound (0 → CLIAgent default)
	Env     []string      // extra KEY=VALUE pairs layered on the inherited env
}

// NewCodexCLI returns a CodexCLI with the default `codex exec` invocation.
// --skip-git-repo-check lets `codex exec` run in the throwaway scratch dir
// (it otherwise refuses outside a trusted git repo).
func NewCodexCLI() *CodexCLI {
	return &CodexCLI{Command: "codex", Args: []string{"exec", "--skip-git-repo-check"}}
}

// CodexFromEnv builds a CodexCLI, applying WUXING_CODEX_* overrides on top of the
// `codex exec` default.
func CodexFromEnv() *CodexCLI {
	c := NewCodexCLI()
	if v := strings.TrimSpace(os.Getenv("WUXING_CODEX_COMMAND")); v != "" {
		c.Command = v
	}
	if v := strings.TrimSpace(os.Getenv("WUXING_CODEX_ARGS")); v != "" {
		c.Args = strings.Fields(v)
	}
	if v := strings.TrimSpace(os.Getenv("WUXING_CODEX_MODEL")); v != "" {
		c.Model = v
	}
	if v := os.Getenv("WUXING_CODEX_TIMEOUT_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.Timeout = time.Duration(n) * time.Second
		}
	}
	return c
}

// agent builds the underlying CLI driver: the prompt goes in as the final
// positional argument (so `codex exec` receives it), the answer comes from stdout.
func (c *CodexCLI) agent() *CLIAgent {
	cmd := c.Command
	if cmd == "" {
		cmd = "codex"
	}
	return NewCLIAgent(AgentSpec{
		Name:      "codex",
		Command:   cmd,
		Args:      c.Args,
		PromptVia: PromptArg,
		OutputVia: OutputStdout,
		Model:     c.Model,
		Timeout:   c.Timeout,
		Env:       c.Env,
	})
}

// Infer runs a one-shot completion through Codex (Backend).
func (c *CodexCLI) Infer(ctx context.Context, req Request) (Result, error) {
	ar, err := c.agent().RunAgent(ctx, AgentRequest{Brief: req.Prompt, Model: req.Model})
	if err != nil {
		return Result{}, err
	}
	return Result{
		Output: ar.Output,
		Model:  ar.Model,
		TTFTMs: ar.TTFTMs,
	}, nil
}

// RunAgent runs a bounded Codex session in a scratch dir (AgentBackend).
func (c *CodexCLI) RunAgent(ctx context.Context, req AgentRequest) (AgentResult, error) {
	return c.agent().RunAgent(ctx, req)
}
