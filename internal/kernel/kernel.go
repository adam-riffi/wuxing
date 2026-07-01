// Package kernel assembles the seven faces and the fact store into one live
// control plane. Assemble wires the bus, scheduler, sessions registry, triggers,
// interpreter, library catalog, and the storage-backed meter over an open fact
// store; the daemon (cmd/wuxing) drives it.
package kernel

import (
	"fmt"

	"github.com/adam-riffi/wuxing/internal/contracts/cfg"
	"github.com/adam-riffi/wuxing/internal/kernel/bus"
	"github.com/adam-riffi/wuxing/internal/kernel/interpreter"
	"github.com/adam-riffi/wuxing/internal/kernel/lineage"
	"github.com/adam-riffi/wuxing/internal/kernel/scheduler"
	"github.com/adam-riffi/wuxing/internal/kernel/sessions"
	"github.com/adam-riffi/wuxing/internal/kernel/triggers"
	"github.com/adam-riffi/wuxing/internal/metering"
	"github.com/adam-riffi/wuxing/internal/storage"
	"github.com/adam-riffi/wuxing/internal/tools/library"
	"github.com/adam-riffi/wuxing/internal/tools/processors"
)

// Kernel holds the assembled control-plane faces over a fact store.
type Kernel struct {
	Store       *storage.DB
	Bus         *bus.Bus
	Minter      *lineage.Minter
	Scheduler   *scheduler.Scheduler
	Sessions    *sessions.Registry
	Triggers    *triggers.Triggers
	Interpreter *interpreter.Interpreter
	Library     *library.Catalog
	Meter       *metering.StoreMeter
	Processors  *processors.Processor
	Runner      *Runner
}

// Assemble constructs the kernel faces over an open fact store. memoryCapacity is
// the scheduler's memory pool (bytes). The scheduler's onAdmit and triggers'
// onFire callbacks are left unset here; the run loop wires them when it launches
// jobs and fires successor runs. Tool handlers are registered on Bus separately,
// once their backends/config exist (a connector's domain DB, the ai backend).
func Assemble(store *storage.DB, memoryCapacity, aiWindow int64) *Kernel {
	minter := lineage.NewMinter()
	b := bus.New()

	// The runner's callbacks drive the scheduler and triggers, but it needs the
	// assembled kernel; create it first and back-fill k after. Its methods are
	// only invoked at runtime (after Assemble returns), so k is set by then.
	rn := newRunner()
	k := &Kernel{
		Store:       store,
		Bus:         b,
		Minter:      minter,
		Scheduler:   scheduler.New(memoryCapacity, rn.onAdmit, scheduler.WithOnExpire(rn.onExpire), scheduler.WithAIWindow(aiWindow)),
		Sessions:    sessions.New(),
		Triggers:    triggers.New(minter, rn.onFire),
		Interpreter: interpreter.New(b, cfg.DefaultVocabulary(), minter),
		Library:     library.New(),
		Meter:       metering.NewStoreMeter(store),
		Processors:  processors.New(store),
		Runner:      rn,
	}
	rn.k = k
	return k
}

// Snapshot is a point-in-time view of the kernel's live runtime state: the
// scheduler's resources and queue, plus the running sessions. It reads in-memory
// state (never the fact store), so it reflects what is happening right now.
type Snapshot struct {
	Scheduler scheduler.Snapshot `json:"scheduler"`
	Sessions  []SessionInfo      `json:"sessions"`
}

// SessionInfo is a running unit of work in the registry.
type SessionInfo struct {
	Session string `json:"session"`
	Run     string `json:"run"`
	Service string `json:"service"`
}

// Snapshot gathers the kernel's live state for observability.
func (k *Kernel) Snapshot() Snapshot {
	snap := Snapshot{Scheduler: k.Scheduler.Snapshot()}
	for _, s := range k.Sessions.Running() {
		snap.Sessions = append(snap.Sessions, SessionInfo{
			Session: string(s.ID),
			Run:     string(s.Run),
			Service: s.Service,
		})
	}
	return snap
}

// Register adds a service to the catalog and wires its declarations into the
// triggers face: each successor becomes an event rule, and each external trigger
// (cron/event) is registered so the service starts when it fires.
func (k *Kernel) Register(svc *cfg.Service) error {
	// A service whose envelope can never be admitted is refused here, loudly,
	// instead of being silently dropped at fire time.
	if err := k.Scheduler.Fits(svc.Envelope.Request, svc.Envelope.AIRequest); err != nil {
		return fmt.Errorf("kernel: service %q can never be admitted: %w", svc.Name, err)
	}
	if err := k.Library.Register(svc); err != nil {
		return err
	}
	for _, s := range svc.Successors {
		if s.Topic != "" {
			k.Triggers.RegisterEvent(s.Service, s.Topic)
		}
	}
	for _, t := range svc.Triggers {
		switch t.Kind {
		case "cron":
			k.Triggers.RegisterCron(svc.Name, t.CronSpec())
		case "event":
			k.Triggers.RegisterEvent(svc.Name, t.Spec)
		}
	}
	return nil
}

// Close releases the kernel's resources: it closes the bus and the fact store.
func (k *Kernel) Close() error {
	k.Bus.Close()
	return k.Store.Close()
}
