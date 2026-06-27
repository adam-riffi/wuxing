package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/adam-riffi/wuxing/internal/tools/ai"
)

// newInferCmd is the operator's door to wuxing's AI: run model work through a
// backend CLI (currently Codex). `wxg infer chat "who was gorbachev" codex` is a
// one-shot completion; `wxg infer agent "<brief>" codex` runs a bounded agent
// session. The backend is opt-in — you name it (default "codex") and configure
// it via WUXING_CODEX_* (see .env.example) or the flags below.
func newInferCmd() *cobra.Command {
	infer := &cobra.Command{
		Use:   "infer",
		Short: "run model work through a backend (codex)",
	}
	infer.AddCommand(newChatCmd(), newAgentCmd())
	return infer
}

func codexFromFlags(model string, timeout int) *ai.CodexCLI {
	c := ai.CodexFromEnv()
	if model != "" {
		c.Model = model
	}
	if timeout > 0 {
		c.Timeout = time.Duration(timeout) * time.Second
	}
	return c
}

// selectBackend validates the optional [backend] positional (default "codex").
func selectBackend(args []string) error {
	backend := "codex"
	if len(args) == 2 {
		backend = args[1]
	}
	if backend != "codex" {
		return fmt.Errorf("unknown backend %q (supported: codex)", backend)
	}
	return nil
}

func newChatCmd() *cobra.Command {
	var model string
	var timeout int
	cmd := &cobra.Command{
		Use:   "chat <prompt> [backend]",
		Short: `one-shot completion, e.g. wxg infer chat "who was gorbachev" codex`,
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := selectBackend(args); err != nil {
				return err
			}
			res, err := codexFromFlags(model, timeout).Infer(cmd.Context(), ai.Request{Prompt: args[0], Model: model})
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), renderArtifact(res.Output))
			return err
		},
	}
	cmd.Flags().StringVar(&model, "model", "", "model label to pass through")
	cmd.Flags().IntVar(&timeout, "timeout", 0, "wall-clock timeout in seconds")
	return cmd
}

func newAgentCmd() *cobra.Command {
	var model string
	var timeout int
	cmd := &cobra.Command{
		Use:   "agent <brief> [backend]",
		Short: "run a bounded agent session in a scratch dir",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := selectBackend(args); err != nil {
				return err
			}
			res, err := codexFromFlags(model, timeout).RunAgent(cmd.Context(), ai.AgentRequest{Brief: args[0], Model: model})
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), renderArtifact(res.Output))
			return err
		},
	}
	cmd.Flags().StringVar(&model, "model", "", "model label to pass through")
	cmd.Flags().IntVar(&timeout, "timeout", 0, "wall-clock timeout in seconds")
	return cmd
}

// renderArtifact prints the answer readably: a JSON string is unwrapped to its
// text; structured JSON is shown as-is.
func renderArtifact(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return string(raw)
}
