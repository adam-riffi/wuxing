// Command wuxing is the kernel daemon — the long-running, compiled control
// plane. Its defining verb is "read cfg → route". It loads the boot manifest,
// opens the fact store, assembles the kernel faces (bus, scheduler, sessions,
// triggers, interpreter, library, lineage) into a live control plane, then idles
// until signalled and shuts down cleanly. The run loop that drives triggers and
// admits/launches jobs, and registration of tool handlers with real backends,
// land on top of this assembly.
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

	"github.com/rs/zerolog"

	"github.com/adam-riffi/wuxing/internal/kernel"
	"github.com/adam-riffi/wuxing/internal/manifest"
	"github.com/adam-riffi/wuxing/internal/storage"
)

// schedulerCapacityBytes is a placeholder memory pool for the scheduler; the
// kernel should measure real available memory and make this configurable.
const schedulerCapacityBytes = 2 << 30 // 2 GiB

func main() {
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
