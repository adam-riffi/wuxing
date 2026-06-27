// Command wuxing is the kernel daemon — the long-running, compiled control
// plane. Its defining verb is "read cfg → route". It loads the boot manifest,
// opens the fact store, assembles the kernel faces (bus, scheduler, sessions,
// triggers, interpreter, library, lineage) into a live control plane, then idles
// until signalled and shuts down cleanly. The run loop that drives triggers and
// admits/launches jobs, and registration of tool handlers with real backends,
// land on top of this assembly.
//
// Subcommands dispatch on the first argument; the bare `wuxing` runs the daemon.
// `wuxing agent --brief "…"` drives the configured agent CLI once (see
// docs/quickstart-ai-agent.md) — the smoke test for an agent integration.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"github.com/adam-riffi/wuxing/internal/kernel"
	"github.com/adam-riffi/wuxing/internal/manifest"
	"github.com/adam-riffi/wuxing/internal/storage"
	"github.com/adam-riffi/wuxing/internal/tools/ai"
)

// schedulerCapacityBytes is a placeholder memory pool for the scheduler; the
// kernel should measure real available memory and make this configurable.
const schedulerCapacityBytes = 2 << 30 // 2 GiB

func main() {
	// Subcommands dispatch on the first argument; the bare `wuxing` runs the daemon.
	if len(os.Args) > 1 && os.Args[1] == "agent" {
		if err := runAgentCmd(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "wuxing agent:", err)
			os.Exit(1)
		}
		return
	}

	manifestPath := flag.String("manifest", "manifest/boot.yml", "path to the boot manifest")
	storePath := flag.String("store", "~/.wuxing/wuxing.db", "path to the sqlite fact store")
	flag.Parse()

	log := zerolog.New(os.Stdout).With().Timestamp().Str("component", "kernel").Logger()

	// The daemon idles until interrupted; main owns the signal wiring so that
	// run() stays driven by a plain context and is testable without signals.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, *manifestPath, *storePath, log); err != nil {
		log.Error().Err(err).Msg("kernel exited with error")
		os.Exit(1)
	}
}

// run boots the kernel: load the manifest, open the fact store, assemble the
// faces into a live kernel, then idle until ctx is cancelled and shut down
// cleanly (closing the bus and the store).
func run(ctx context.Context, manifestPath, storePath string, log zerolog.Logger) error {
	log.Info().Str("manifest", manifestPath).Msg("booting wuxing kernel")

	m, err := manifest.Load(manifestPath)
	if err != nil {
		return err
	}
	for _, t := range m.Tools {
		log.Info().
			Str("tool", t.Name).
			Str("package", t.Package).
			Str("status", string(t.Status)).
			Msg("manifest tool")
	}
	log.Info().Int("tool_count", len(m.Tools)).Msg("manifest loaded")

	resolved, err := resolveStorePath(storePath)
	if err != nil {
		return err
	}
	store, err := storage.OpenSQLite(resolved)
	if err != nil {
		return err
	}
	log.Info().Str("store", resolved).Msg("fact store opened (sqlite, WAL)")

	k := kernel.Assemble(store, schedulerCapacityBytes)
	defer func() {
		if cerr := k.Close(); cerr != nil {
			log.Error().Err(cerr).Msg("error closing kernel")
		}
	}()
	log.Info().Msg("kernel ready — faces assembled (bus, scheduler, sessions, triggers, interpreter, library)")

	log.Info().Msg("kernel idle — waiting for signal")
	<-ctx.Done()

	log.Info().Msg("shutdown signal received, draining")
	log.Info().Msg("kernel stopped cleanly")
	return nil
}

// runAgentCmd is the `wuxing agent` subcommand: drive the configured agent CLI
// (WUXING_AI_AGENT_* env) once with a brief and print the artifact to stdout.
// This is the smoke test for an agent integration — it exercises the real CLI
// (Hermes, Codex, Open Design, …) without needing the rest of the kernel.
func runAgentCmd(args []string) error {
	fs := flag.NewFlagSet("agent", flag.ExitOnError)
	brief := fs.String("brief", "", "the task/brief to hand the agent (required)")
	model := fs.String("model", "", "model label to pass through (optional)")
	timeout := fs.Int("timeout", 0, "wall-clock timeout in seconds (0 = spec default)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*brief) == "" {
		return fmt.Errorf("--brief is required")
	}

	spec, err := ai.AgentSpecFromEnv()
	if err != nil {
		return fmt.Errorf("%w\nset WUXING_AI_AGENT_COMMAND (e.g. \"hermes\") — see docs/quickstart-ai-agent.md", err)
	}

	req := ai.AgentRequest{Brief: *brief, Model: *model}
	if *timeout > 0 {
		req.Timeout = time.Duration(*timeout) * time.Second
	}
	res, err := ai.NewCLIAgent(spec).RunAgent(context.Background(), req)
	if err != nil {
		return err
	}

	if _, err := os.Stdout.Write(res.Output); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "\n[wuxing agent: model=%s turns=%d tokens=%d/%d cost=%.4f %dms]\n",
		res.Model, res.TurnCount, res.TokensIn, res.TokensOut, res.Cost, res.TTFTMs)
	return nil
}

// resolveStorePath expands a leading ~ to the home directory and ensures the
// parent directory exists.
func resolveStorePath(path string) (string, error) {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve store path: %w", err)
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return "", fmt.Errorf("create store dir: %w", err)
		}
	}
	return path, nil
}
