package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/adam-riffi/wuxing/internal/kernel/bus"
)

// AgentRequest is a bounded agent session: drive a CLI agent to produce an
// artifact from a brief, inside an isolated scratch directory.
type AgentRequest struct {
	Brief   string
	Model   string
	Workdir string        // scratch dir; a temp dir is created and removed when empty
	Timeout time.Duration // overrides the spec default when > 0
}

// AgentResult is the artifact the agent produced plus any usage it reported.
type AgentResult struct {
	Output    json.RawMessage // the captured artifact (raw JSON, or a JSON string)
	Model     string
	TokensIn  int
	TokensOut int
	Cost      float64
	TTFTMs    int
	TurnCount int
}

// AgentBackend drives an agent session. CLIAgent shells out to a real agent CLI
// (Hermes Agent, Codex, Open Design, …) that "uses itself"; tests use a fake.
type AgentBackend interface {
	RunAgent(ctx context.Context, req AgentRequest) (AgentResult, error)
}

// How the brief reaches the CLI, and how its artifact comes back.
const (
	PromptStdin = "stdin" // piped to the process's stdin (default)
	PromptArg   = "arg"   // appended as the final positional argument
	PromptFile  = "file"  // written to PromptFile in the scratch dir

	OutputStdout = "stdout" // captured from the process's stdout (default)
	OutputFile   = "file"   // read back from OutputFile in the scratch dir
)

// AgentSpec configures a CLI agent backend: which binary to run, how to hand it
// the brief, and how to collect the artifact. It is host-agnostic — the same
// wuxing drives Hermes, Codex, or any agent CLI by changing this spec. The
// process inherits the daemon's environment (so API keys set in the shell flow
// through), plus any Env overrides here.
type AgentSpec struct {
	Name       string        // label recorded on the ai fact (e.g. "hermes")
	Command    string        // binary or path, e.g. "hermes"
	Args       []string      // fixed args; {{brief}} {{model}} {{workdir}} {{prompt_file}} {{output_file}} are expanded
	PromptVia  string        // PromptStdin (default) | PromptArg | PromptFile
	PromptFile string        // filename in the scratch dir when PromptVia == PromptFile
	OutputVia  string        // OutputStdout (default) | OutputFile
	OutputFile string        // filename in the scratch dir when OutputVia == OutputFile
	UsageFile  string        // optional JSON sidecar: {tokens_in,tokens_out,cost,turns}
	Env        []string      // extra KEY=VALUE pairs layered on the inherited env
	Timeout    time.Duration // per-session wall-clock bound (default 5m)
	Model      string        // default model label
}

// CLIAgent is an AgentBackend that runs a configured CLI in an isolated scratch
// directory and captures its artifact. The CLI drives itself; wuxing only bounds
// it — one scratch dir, one wall-clock timeout, one allow-listed binary.
type CLIAgent struct {
	spec AgentSpec
}

// NewCLIAgent returns a CLIAgent, applying defaults (stdin in / stdout out, 5m).
func NewCLIAgent(spec AgentSpec) *CLIAgent {
	if spec.PromptVia == "" {
		spec.PromptVia = PromptStdin
	}
	if spec.OutputVia == "" {
		spec.OutputVia = OutputStdout
	}
	if spec.Timeout == 0 {
		spec.Timeout = 5 * time.Minute
	}
	if spec.Name == "" {
		spec.Name = spec.Command
	}
	return &CLIAgent{spec: spec}
}

// RunAgent runs one bounded agent session and returns its artifact.
func (a *CLIAgent) RunAgent(ctx context.Context, req AgentRequest) (AgentResult, error) {
	if a.spec.Command == "" {
		return AgentResult{}, fmt.Errorf("ai: agent spec has no command")
	}

	// Scratch space: an isolated working directory the agent owns for this run.
	workdir := req.Workdir
	if workdir == "" {
		d, err := os.MkdirTemp("", "wuxing-agent-*")
		if err != nil {
			return AgentResult{}, fmt.Errorf("ai: agent scratch: %w", err)
		}
		workdir = d
		defer func() { _ = os.RemoveAll(d) }()
	}

	model := req.Model
	if model == "" {
		model = a.spec.Model
	}

	timeout := a.spec.Timeout
	if req.Timeout > 0 {
		timeout = req.Timeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	repl := strings.NewReplacer(
		"{{brief}}", req.Brief,
		"{{model}}", model,
		"{{workdir}}", workdir,
		"{{prompt_file}}", a.spec.PromptFile,
		"{{output_file}}", a.spec.OutputFile,
	)
	args := make([]string, 0, len(a.spec.Args)+1)
	for _, s := range a.spec.Args {
		args = append(args, repl.Replace(s))
	}

	// Deliver the brief.
	switch a.spec.PromptVia {
	case PromptArg:
		args = append(args, req.Brief)
	case PromptFile:
		if a.spec.PromptFile == "" {
			return AgentResult{}, fmt.Errorf("ai: agent PromptFile required for prompt-via=file")
		}
		if err := os.WriteFile(filepath.Join(workdir, a.spec.PromptFile), []byte(req.Brief), 0o600); err != nil {
			return AgentResult{}, fmt.Errorf("ai: write prompt file: %w", err)
		}
	}

	// #nosec G204 -- the command is the operator-configured agent spec, not
	// request input; running it is the entire purpose of this tool. The brief is
	// delivered as data (stdin/arg/file), never as the command.
	cmd := exec.CommandContext(runCtx, a.spec.Command, args...)
	cmd.Dir = workdir
	cmd.Env = append(os.Environ(), a.spec.Env...)
	if a.spec.PromptVia == PromptStdin {
		cmd.Stdin = strings.NewReader(req.Brief)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	elapsed := int(time.Since(start).Milliseconds())
	if err != nil {
		if runCtx.Err() == context.DeadlineExceeded {
			return AgentResult{}, fmt.Errorf("ai: agent %q timed out after %s", a.spec.Command, timeout)
		}
		return AgentResult{}, fmt.Errorf("ai: agent %q failed: %w: %s", a.spec.Command, err, strings.TrimSpace(stderr.String()))
	}

	artifact := stdout.Bytes()
	if a.spec.OutputVia == OutputFile {
		if a.spec.OutputFile == "" {
			return AgentResult{}, fmt.Errorf("ai: agent OutputFile required for output-via=file")
		}
		// #nosec G304 -- operator-configured artifact name, read from this run's
		// own scratch dir.
		b, rerr := os.ReadFile(filepath.Join(workdir, a.spec.OutputFile))
		if rerr != nil {
			return AgentResult{}, fmt.Errorf("ai: read agent artifact: %w", rerr)
		}
		artifact = b
	}

	res := AgentResult{
		Output:    normalizeJSON(artifact),
		Model:     model,
		TTFTMs:    elapsed,
		TurnCount: 1,
	}
	a.applyUsage(workdir, &res)
	return res, nil
}

type agentUsage struct {
	TokensIn  int     `json:"tokens_in"`
	TokensOut int     `json:"tokens_out"`
	Cost      float64 `json:"cost"`
	Turns     int     `json:"turns"`
}

// applyUsage reads the optional usage sidecar; usage is best-effort and never
// fails the run (the artifact is what matters; the fact still records the call).
func (a *CLIAgent) applyUsage(workdir string, res *AgentResult) {
	if a.spec.UsageFile == "" {
		return
	}
	// #nosec G304 -- operator-configured sidecar name, read from this run's own scratch dir.
	b, err := os.ReadFile(filepath.Join(workdir, a.spec.UsageFile))
	if err != nil {
		return
	}
	var u agentUsage
	if json.Unmarshal(b, &u) != nil {
		return
	}
	res.TokensIn, res.TokensOut, res.Cost = u.TokensIn, u.TokensOut, u.Cost
	if u.Turns > 0 {
		res.TurnCount = u.Turns
	}
}

// normalizeJSON returns the artifact as valid JSON: as-is when it already is,
// otherwise wrapped as a JSON string so it travels the bus unchanged.
func normalizeJSON(b []byte) json.RawMessage {
	t := bytes.TrimSpace(b)
	if len(t) > 0 && json.Valid(t) {
		return json.RawMessage(t)
	}
	enc, _ := json.Marshal(string(b))
	return json.RawMessage(enc)
}

// Infer runs a one-shot completion: the prompt is the brief, the artifact is the
// result. This makes CLIAgent a Backend as well as an AgentBackend, so a detected
// agent CLI can serve both ai operations.
func (a *CLIAgent) Infer(ctx context.Context, req Request) (Result, error) {
	ar, err := a.RunAgent(ctx, AgentRequest{Brief: req.Prompt, Model: req.Model})
	if err != nil {
		return Result{}, err
	}
	return Result{Output: ar.Output, Model: ar.Model, TTFTMs: ar.TTFTMs}, nil
}

// SetTimeout overrides the per-call wall-clock bound (no-op for d <= 0).
func (a *CLIAgent) SetTimeout(d time.Duration) {
	if d > 0 {
		a.spec.Timeout = d
	}
}

// WithAgent attaches an agent backend, enabling the "agent" operation.
func (t *Tool) WithAgent(a AgentBackend) *Tool {
	t.agent = a
	return t
}

type agentRequest struct {
	Brief          string `json:"brief"`
	Prompt         string `json:"prompt"` // alias for brief
	Model          string `json:"model"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

func (t *Tool) handleAgent(ctx context.Context, e bus.Envelope) bus.Envelope {
	if t.agent == nil {
		return e.ReplyError(fmt.Errorf("ai: no agent backend configured"))
	}
	var req agentRequest
	if len(e.Payload) > 0 {
		if err := json.Unmarshal(e.Payload, &req); err != nil {
			return e.ReplyError(fmt.Errorf("ai: bad agent payload: %w", err))
		}
	}
	brief := req.Brief
	if brief == "" {
		brief = req.Prompt
	}
	if brief == "" {
		return e.ReplyError(fmt.Errorf("ai: agent call has no brief"))
	}

	ar := AgentRequest{Brief: brief, Model: req.Model}
	if req.TimeoutSeconds > 0 {
		ar.Timeout = time.Duration(req.TimeoutSeconds) * time.Second
	}
	res, err := t.agent.RunAgent(ctx, ar)
	if err != nil {
		return e.ReplyError(fmt.Errorf("ai: agent: %w", err))
	}

	// Like infer, the cost is recorded at incur-time.
	t.meter.Call(CallFact{
		Stamp:     e.Stamp,
		Model:     res.Model,
		Mode:      "agent",
		TokensIn:  res.TokensIn,
		TokensOut: res.TokensOut,
		Cost:      res.Cost,
		TTFTMs:    res.TTFTMs,
		TurnCount: res.TurnCount,
	})
	return e.Reply(res.Output)
}

// AgentSpecFromEnv builds a spec from WUXING_AI_AGENT_* environment variables.
// WUXING_AI_AGENT_COMMAND is required; everything else has a sensible default
// (stdin in / stdout out). ARGS is split on spaces (use a wrapper script for
// arguments that contain spaces).
func AgentSpecFromEnv() (AgentSpec, error) {
	cmd := strings.TrimSpace(os.Getenv("WUXING_AI_AGENT_COMMAND"))
	if cmd == "" {
		return AgentSpec{}, fmt.Errorf("ai: WUXING_AI_AGENT_COMMAND is not set")
	}
	spec := AgentSpec{
		Name:       envOr("WUXING_AI_AGENT_NAME", cmd),
		Command:    cmd,
		PromptVia:  envOr("WUXING_AI_AGENT_PROMPT_VIA", PromptStdin),
		PromptFile: os.Getenv("WUXING_AI_AGENT_PROMPT_FILE"),
		OutputVia:  envOr("WUXING_AI_AGENT_OUTPUT_VIA", OutputStdout),
		OutputFile: os.Getenv("WUXING_AI_AGENT_OUTPUT_FILE"),
		UsageFile:  os.Getenv("WUXING_AI_AGENT_USAGE_FILE"),
		Model:      os.Getenv("WUXING_AI_AGENT_MODEL"),
	}
	if a := strings.TrimSpace(os.Getenv("WUXING_AI_AGENT_ARGS")); a != "" {
		spec.Args = strings.Fields(a)
	}
	if s := os.Getenv("WUXING_AI_AGENT_TIMEOUT_SECONDS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			spec.Timeout = time.Duration(n) * time.Second
		}
	}
	return spec, nil
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
