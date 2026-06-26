// Command wuxing is the kernel daemon — the long-running, compiled control
// plane. Its defining verb is "read cfg → route". At Phase 0 it boots, loads
// the boot manifest, logs the tools it would load, idles, and shuts down
// cleanly on a signal. The kernel faces (bus, scheduler, sessions, triggers,
// launcher, lineage, interpreter) are filled in by later phases.
package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"

	"github.com/adam-riffi/wuxing/internal/manifest"
)

func main() {
	manifestPath := flag.String("manifest", "manifest/boot.yml", "path to the boot manifest")
	flag.Parse()

	log := zerolog.New(os.Stdout).With().Timestamp().Str("component", "kernel").Logger()

	// The daemon idles until interrupted; main owns the signal wiring so that
	// run() stays driven by a plain context and is testable without signals.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, *manifestPath, log); err != nil {
		log.Error().Err(err).Msg("kernel exited with error")
		os.Exit(1)
	}
}

// run boots the kernel: load the manifest, log the tools it would load, then
// idle until ctx is cancelled and shut down cleanly. Phase 3+ wires the bus,
// scheduler, sessions, triggers and launcher in place of the idle wait.
func run(ctx context.Context, manifestPath string, log zerolog.Logger) error {
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

	log.Info().Msg("kernel idle — no manifest work yet, waiting for signal")
	<-ctx.Done()

	log.Info().Msg("shutdown signal received, draining")
	// Phase 3+ drains running sessions here before returning.
	log.Info().Msg("kernel stopped cleanly")
	return nil
}
