package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/adam-riffi/wuxing/internal/tools/ai"
)

// newInferCmd is the operator's door to wuxing's AI: run model work through an
// agent CLI that wuxing detects on your system — no manual wiring. With no
// [backend] given it auto-detects (codex, claude, gemini, hermes, …); name one to
// force it. `wxg infer detect` lists what it found. `wxg infer chat "…"` is a
// one-shot completion; `wxg infer agent "…"` runs a bounded agent session.
func newInferCmd() *cobra.Command {
	infer := &cobra.Command{
		Use:   "infer",
		Short: "run model work through a detected agent CLI",
	}
	infer.AddCommand(newChatCmd(), newAgentCmd(), newDetectCmd())
	return infer
}

func backendArg(args []string) string {
	if len(args) == 2 {
		return args[1]
	}
	return "" // empty → auto-detect
}

func newChatCmd() *cobra.Command {
	var model string
	var timeout int
	cmd := &cobra.Command{
		Use:   "chat <prompt> [backend]",
		Short: `one-shot completion, e.g. wxg infer chat "who was gorbachev"`,
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ag, label, err := ai.ResolveAgent(backendArg(args))
			if err != nil {
				return err
			}
			if timeout > 0 {
				ag.SetTimeout(time.Duration(timeout) * time.Second)
			}
			res, err := ag.Infer(cmd.Context(), ai.Request{Prompt: args[0], Model: model})
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "[wuxing: via %s]\n", label)
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
			ag, label, err := ai.ResolveAgent(backendArg(args))
			if err != nil {
				return err
			}
			req := ai.AgentRequest{Brief: args[0], Model: model}
			if timeout > 0 {
				req.Timeout = time.Duration(timeout) * time.Second
			}
			res, err := ag.RunAgent(cmd.Context(), req)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "[wuxing: via %s]\n", label)
			_, err = fmt.Fprintln(cmd.OutOrStdout(), renderArtifact(res.Output))
			return err
		},
	}
	cmd.Flags().StringVar(&model, "model", "", "model label to pass through")
	cmd.Flags().IntVar(&timeout, "timeout", 0, "wall-clock timeout in seconds")
	return cmd
}

func newDetectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "detect",
		Short: "list agent CLIs wuxing found on your PATH",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			w := cmd.OutOrStdout()
			found := ai.Detect()
			if len(found) == 0 {
				_, err := fmt.Fprintf(w, "No agent CLI detected on PATH.\nLooked for: %s\nInstall one (e.g. Codex) and retry.\n", ai.KnownAgentNames())
				return err
			}
			if _, err := fmt.Fprintln(w, "Detected agent CLIs (default first):"); err != nil {
				return err
			}
			for i, d := range found {
				marker := "  "
				if i == 0 {
					marker = "* "
				}
				if _, err := fmt.Fprintf(w, "%s%-12s %s\n", marker, d.Name, d.Path); err != nil {
					return err
				}
			}
			return nil
		},
	}
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
