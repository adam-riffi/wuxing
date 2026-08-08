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
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"github.com/adam-riffi/wuxing/internal/contracts/cfg"
	"github.com/adam-riffi/wuxing/internal/control"
	"github.com/adam-riffi/wuxing/internal/kernel"
	"github.com/adam-riffi/wuxing/internal/kernel/triggers"
	"github.com/adam-riffi/wuxing/internal/manifest"
	"github.com/adam-riffi/wuxing/internal/storage"
	"github.com/adam-riffi/wuxing/internal/tools/ai"
	"github.com/adam-riffi/wuxing/internal/tools/connectors"
)

// connectorGrants maps a service's declared connectors allowlist entries
// (target "DATABASE.TABLE") onto the connectors tool's grant type.
func connectorGrants(svc *cfg.Service) []connectors.Grant {
	var out []connectors.Grant
	for _, g := range svc.Allow {
		if g.Tool != "connectors" || g.Target == "" {
			continue
		}
		db, table, ok := strings.Cut(g.Target, ".")
		if !ok {
			continue
		}
		out = append(out, connectors.Grant{Database: db, Table: table})
	}
	return out
}

// stateFrom maps the kernel's live snapshot to the control API's wire state.
func stateFrom(k *kernel.Kernel) control.State {
	ks := k.Snapshot()
	st := control.State{
		Memory: control.Resource{
			Capacity: ks.Scheduler.MemoryCapacity,
			Used:     ks.Scheduler.MemoryUsed,
			Free:     ks.Scheduler.MemoryFree,
		},
		Window: control.Resource{
			Capacity: ks.Scheduler.WindowCapacity,
			Used:     ks.Scheduler.WindowCapacity - ks.Scheduler.WindowFree,
			Free:     ks.Scheduler.WindowFree,
		},
		RunningJobs: ks.Scheduler.RunningIDs,
		Queue:       ks.Scheduler.QueuedIDs,
	}
	for _, s := range ks.Sessions {
		st.Sessions = append(st.Sessions, control.SessionInfo{
			Session: s.Session, Run: s.Run, Service: s.Service,
		})
	}
	return st
}

// schedulerCapacityBytes is a placeholder memory pool for the scheduler; the
// kernel should measure real available memory and make this configurable.
const schedulerCapacityBytes = 2 << 30 // 2 GiB

// aiWindowCapacity is the AI-quota window (units per refill period): the
// dual-resource budget AI jobs draw on. Placeholder until it is configurable.
const aiWindowCapacity = 60

// aiWindowRefillEvery is how often the clock refills the AI window (the window
// refills on a clock, never on job completion — that's the design's rate lane).
const aiWindowRefillEvery = time.Hour

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
	apiAddr := flag.String("api", control.DefaultAddr, "loopback address for the control API (empty to disable)")
	servicesDir := flag.String("services", "", "directory of service folders (cfg + scripts) to load at boot")
	flag.Parse()

	log := zerolog.New(os.Stdout).With().Timestamp().Str("component", "kernel").Logger()

	// The daemon idles until interrupted; main owns the signal wiring so that
	// run() stays driven by a plain context and is testable without signals.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, *manifestPath, *storePath, *apiAddr, *servicesDir, log); err != nil {
		log.Error().Err(err).Msg("kernel exited with error")
		os.Exit(1)
	}
}

// run boots the kernel: load the manifest, open the fact store, assemble the
// faces into a live kernel, then idle until ctx is cancelled and shut down
// cleanly (closing the bus and the store).
func run(ctx context.Context, manifestPath, storePath, apiAddr, servicesDir string, log zerolog.Logger) error {
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

	k := kernel.Assemble(store, schedulerCapacityBytes, aiWindowCapacity)
	defer func() {
		if cerr := k.Close(); cerr != nil {
			log.Error().Err(cerr).Msg("error closing kernel")
		}
	}()

	// The AI window refills on a clock (never on completion): the rate lane.
	go func() {
		ticker := time.NewTicker(aiWindowRefillEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				k.Scheduler.RefillWindow(aiWindowCapacity)
			}
		}
	}()
	log.Info().Msg("kernel ready — faces assembled (bus, scheduler, sessions, triggers, interpreter, library)")

	// Load the services directory: parse + validate each folder's cfg, register
	// it with the kernel (library + trigger wiring), and persist its ID card to
	// the admin index. The cfg is the program; this is where it gets loaded.
	var grants []connectors.Grant
	if servicesDir != "" {
		loaded, err := cfg.LoadDir(servicesDir, cfg.DefaultVocabulary())
		if err != nil {
			return err
		}
		// Boot-time persists get their own context: the signal context governs
		// the idle wait, not the boot sequence.
		bootCtx := context.Background()
		for _, l := range loaded {
			if err := k.Register(l.Service); err != nil {
				return fmt.Errorf("register service %q: %w", l.Service.Name, err)
			}
			grants = append(grants, connectorGrants(l.Service)...)
			cfgJSON, _ := json.Marshal(l.Service)
			if err := store.RecordService(bootCtx, storage.ServiceRecord{
				ServiceID: l.Service.Name,
				Name:      l.Service.Name,
				Version:   l.Service.Version,
				Status:    "live",
				CfgJSON:   string(cfgJSON),
			}); err != nil {
				log.Warn().Err(err).Str("service", l.Service.Name).Msg("admin index record failed (already indexed?)")
			}
			log.Info().Str("service", l.Service.Name).Str("cfg", l.Path).
				Int("steps", len(l.Service.Workflow)).Int("triggers", len(l.Service.Triggers)).
				Msg("service registered")
		}
		log.Info().Int("service_count", len(loaded)).Str("dir", servicesDir).Msg("services loaded")
	}

	// Reload any services that were dynamically indexed after the last boot (not
	// in --services) so the catalog is fully restored from the admin index.
	{
		bootCtx := context.Background()
		recs, err := store.ListServices(bootCtx)
		if err != nil {
			log.Warn().Err(err).Msg("admin index reload failed")
		} else {
			alreadyLoaded := map[string]bool{}
			for _, n := range k.Library.List() {
				alreadyLoaded[n] = true
			}
			for _, rec := range recs {
				if alreadyLoaded[rec.Name] {
					continue
				}
				var svc cfg.Service
				if err := json.Unmarshal([]byte(rec.CfgJSON), &svc); err != nil {
					log.Warn().Err(err).Str("service", rec.Name).Msg("admin index: invalid cfg JSON, skipping")
					continue
				}
				if err := k.Register(&svc); err != nil {
					log.Warn().Err(err).Str("service", rec.Name).Msg("admin index reload: register failed")
					continue
				}
				grants = append(grants, connectorGrants(&svc)...)
				log.Info().Str("service", rec.Name).Msg("service restored from admin index")
			}
		}
	}

	// Register the tool handlers on the bus so fired services can execute.
	// ai: over the auto-detected agent CLI (a missing backend degrades to a
	// clear per-call error, not a boot failure).
	if agent, label, err := ai.ResolveAgent(""); err == nil {
		_ = k.Bus.Register("ai", ai.New(agent, k.Meter).WithAgent(agent).Handler())
		log.Info().Str("backend", label).Msg("ai tool registered")
	} else {
		log.Warn().Err(err).Msg("ai tool not registered — ai steps will fail")
	}
	// connectors: over the domain store (the cargo DB), granted the union of the
	// loaded services' allowlists.
	domainPath := filepath.Join(filepath.Dir(resolved), "domain.db")
	domainDB, err := sql.Open("sqlite", "file:"+domainPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return fmt.Errorf("open domain store: %w", err)
	}
	defer func() { _ = domainDB.Close() }()
	_ = k.Bus.Register("connectors", connectors.New(domainDB, grants, k.Meter).Handler())
	log.Info().Str("domain", domainPath).Int("grants", len(grants)).Msg("connectors tool registered")

	// Start the cron watcher: fires due schedules as new sequences.
	watched, err := k.Triggers.StartCron(ctx)
	if err != nil {
		return err
	}
	log.Info().Int("schedules", watched).Msg("cron watcher started")

	// The control API: read side (live state) + write side (fire a service +
	// catalog management: index, deindex, list).
	var srv *http.Server
	if apiAddr != "" {
		apiCtx := context.Background()
		fire := func(req control.RunRequest) (control.RunStarted, error) {
			if _, err := k.Library.GetDefinition(req.Service); err != nil {
				return control.RunStarted{}, err
			}
			r := k.Triggers.FireExternal(req.Service, triggers.KindManual)
			return control.RunStarted{
				Service:  req.Service,
				Sequence: string(r.Stamp.Sequence),
				Run:      string(r.Stamp.Run),
			}, nil
		}
		catalog := control.CatalogFuncs{
			Index: func(cfgJSON string) (control.CatalogEntry, error) {
				var svc cfg.Service
				if err := json.Unmarshal([]byte(cfgJSON), &svc); err != nil {
					return control.CatalogEntry{}, fmt.Errorf("invalid cfg: %w", err)
				}
				if err := k.Register(&svc); err != nil {
					return control.CatalogEntry{}, err
				}
				rec := storage.ServiceRecord{
					ServiceID: svc.Name,
					Name:      svc.Name,
					Version:   svc.Version,
					Status:    "live",
					CfgJSON:   cfgJSON,
				}
				if err := store.RecordService(apiCtx, rec); err != nil {
					log.Warn().Err(err).Str("service", svc.Name).Msg("admin index record failed")
				}
				return control.CatalogEntry{Name: svc.Name, Version: svc.Version, Status: "live"}, nil
			},
			Deindex: func(name string) error {
				if err := k.Deregister(name); err != nil {
					return err
				}
				return store.RemoveService(apiCtx, name)
			},
			List: func() []control.CatalogEntry {
				recs, err := store.ListServices(apiCtx)
				if err != nil {
					log.Warn().Err(err).Msg("admin index list failed")
					return nil
				}
				entries := make([]control.CatalogEntry, len(recs))
				for i, r := range recs {
					entries[i] = control.CatalogEntry{
						Name:         r.Name,
						Version:      r.Version,
						Status:       r.Status,
						RegisteredAt: r.RegisteredAt,
					}
				}
				return entries
			},
		}
		srv = &http.Server{
			Addr:              apiAddr,
			Handler:           control.Handler(func() control.State { return stateFrom(k) }, fire, catalog),
			ReadHeaderTimeout: 5 * time.Second,
		}
		go func() {
			if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Error().Err(err).Msg("control API error")
			}
		}()
		log.Info().Str("api", apiAddr).Msg("control API listening")
	}

	log.Info().Msg("kernel idle — waiting for signal")
	<-ctx.Done()

	log.Info().Msg("shutdown signal received, draining")
	if srv != nil {
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = srv.Shutdown(shutCtx)
		cancel()
	}
	log.Info().Msg("kernel stopped cleanly")
	return nil
}

// runAgentCmd is the `wuxing agent` subcommand: run a brief through an agent CLI
// that wuxing auto-detects on PATH (override with a name via WUXING_AI_AGENT_*),
// printing the artifact to stdout. The smoke test for an agent integration — it
// exercises the real CLI (Codex, Claude, Hermes, …) without the rest of the kernel.
func runAgentCmd(args []string) error {
	fs := flag.NewFlagSet("agent", flag.ExitOnError)
	brief := fs.String("brief", "", "the task/brief to hand the agent (required)")
	model := fs.String("model", "", "model label to pass through (optional)")
	backend := fs.String("backend", "", "force an agent by name (default: auto-detect)")
	timeout := fs.Int("timeout", 0, "wall-clock timeout in seconds (0 = spec default)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*brief) == "" {
		return fmt.Errorf("--brief is required")
	}

	agent, label, err := ai.ResolveAgent(*backend)
	if err != nil {
		return err
	}

	req := ai.AgentRequest{Brief: *brief, Model: *model}
	if *timeout > 0 {
		req.Timeout = time.Duration(*timeout) * time.Second
	}
	res, err := agent.RunAgent(context.Background(), req)
	if err != nil {
		return err
	}

	if _, err := os.Stdout.Write(res.Output); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "\n[wuxing agent: via=%s model=%s turns=%d tokens=%d/%d cost=%.4f %dms]\n",
		label, res.Model, res.TurnCount, res.TokensIn, res.TokensOut, res.Cost, res.TTFTMs)
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
