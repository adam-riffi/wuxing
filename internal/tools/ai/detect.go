package ai

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// KnownAgent is a recognized agent CLI and how to drive it headlessly. The
// registry lets wuxing detect installed agents on PATH and run them with no
// manual configuration — the same idea as Hermes / Open Design auto-detecting
// agents on your system.
type KnownAgent struct {
	Name      string   // wuxing's label, e.g. "codex"
	Bin       string   // binary to look for on PATH
	Args      []string // headless invocation (the prompt is appended / piped)
	PromptVia string   // PromptArg or PromptStdin
	OutputVia string   // OutputStdout or OutputFile
}

// knownAgents is the detection registry, in preference order (first installed
// wins). The headless invocations are sensible defaults; if a CLI's flags differ
// in your version, override with WUXING_AI_AGENT_* (see AgentSpecFromEnv).
var knownAgents = []KnownAgent{
	// codex exec refuses to run outside a trusted git repo without this flag, and
	// the agent runs in a throwaway scratch dir — so pass it by default.
	{Name: "codex", Bin: "codex", Args: []string{"exec", "--skip-git-repo-check"}, PromptVia: PromptArg, OutputVia: OutputStdout},
	{Name: "claude", Bin: "claude", Args: []string{"-p"}, PromptVia: PromptArg, OutputVia: OutputStdout},
	{Name: "gemini", Bin: "gemini", Args: []string{"-p"}, PromptVia: PromptArg, OutputVia: OutputStdout},
	{Name: "hermes", Bin: "hermes", PromptVia: PromptStdin, OutputVia: OutputStdout},
	{Name: "open-design", Bin: "od", PromptVia: PromptStdin, OutputVia: OutputStdout},
}

// lookPath is exec.LookPath, indirected so tests can simulate an installed CLI.
var lookPath = exec.LookPath

// Detected is a known agent CLI found installed on the system.
type Detected struct {
	KnownAgent
	Path string // resolved location on PATH
}

// Agent builds a ready-to-run CLIAgent for the detected agent.
func (d Detected) Agent() *CLIAgent {
	return NewCLIAgent(AgentSpec{
		Name:      d.Name,
		Command:   d.Bin,
		Args:      d.Args,
		PromptVia: d.PromptVia,
		OutputVia: d.OutputVia,
	})
}

// Detect returns the known agent CLIs found on PATH, in preference order.
func Detect() []Detected {
	var found []Detected
	for _, ka := range knownAgents {
		if path, err := lookPath(ka.Bin); err == nil {
			found = append(found, Detected{KnownAgent: ka, Path: path})
		}
	}
	return found
}

// DetectDefault returns the highest-preference installed agent.
func DetectDefault() (Detected, bool) {
	found := Detect()
	if len(found) == 0 {
		return Detected{}, false
	}
	return found[0], true
}

func knownAgentByName(name string) (KnownAgent, bool) {
	for _, ka := range knownAgents {
		if ka.Name == name || ka.Bin == name {
			return ka, true
		}
	}
	return KnownAgent{}, false
}

// KnownAgentNames lists the registry names, for help and error messages.
func KnownAgentNames() string {
	names := make([]string, len(knownAgents))
	for i, ka := range knownAgents {
		names[i] = ka.Name
	}
	return strings.Join(names, ", ")
}

// ResolveAgent selects an agent backend with no manual wiring required:
//   - an explicit name → that known agent, if it's installed;
//   - empty + WUXING_AI_AGENT_COMMAND set → that manual override;
//   - empty → the auto-detected default (first installed known agent).
//
// It returns a *CLIAgent (which implements both Backend and AgentBackend) and the
// chosen agent's label, or an error explaining what to install.
func ResolveAgent(name string) (*CLIAgent, string, error) {
	if name != "" {
		ka, ok := knownAgentByName(name)
		if !ok {
			return nil, "", fmt.Errorf("unknown agent %q (known: %s)", name, KnownAgentNames())
		}
		if _, err := lookPath(ka.Bin); err != nil {
			return nil, "", fmt.Errorf("agent %q (%s) is not on PATH — install it, or run `wxg infer detect`", ka.Name, ka.Bin)
		}
		return Detected{KnownAgent: ka}.Agent(), ka.Name, nil
	}

	if cmd := strings.TrimSpace(os.Getenv("WUXING_AI_AGENT_COMMAND")); cmd != "" {
		spec, err := AgentSpecFromEnv()
		if err != nil {
			return nil, "", err
		}
		return NewCLIAgent(spec), spec.Name, nil
	}

	d, ok := DetectDefault()
	if !ok {
		return nil, "", fmt.Errorf("no agent CLI detected on PATH (looked for: %s) — install one (e.g. Codex) and retry", KnownAgentNames())
	}
	return d.Agent(), d.Name, nil
}
