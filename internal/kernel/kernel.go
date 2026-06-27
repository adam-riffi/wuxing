// Package kernel assembles the seven faces and the fact store into one live
// control plane. Assemble wires the bus, scheduler, sessions registry, triggers,
// interpreter, library catalog, and the storage-backed meter over an open fact
// store; the daemon (cmd/wuxing) drives it.
package kernel

import (
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
}

// Assemble constructs the kernel faces over an open fact store. memoryCapacity is
// the scheduler's memory pool (bytes). The scheduler's onAdmit and triggers'
// onFire callbacks are left unset here; the run loop wires them when it launches
// jobs and fires successor runs. Tool handlers are registered on Bus separately,
// once their backends/config exist (a connector's domain DB, the ai backend).
func Assemble(store *storage.DB, memoryCapacity int64) *Kernel {
	minter := lineage.NewMinter()
	b := bus.New()
	return &Kernel{
		Store:       store,
		Bus:         b,
		Minter:      minter,
		Scheduler:   scheduler.New(memoryCapacity, nil),
		Sessions:    sessions.New(),
		Triggers:    triggers.New(minter, nil),
		Interpreter: interpreter.New(b, cfg.DefaultVocabulary(), minter),
		Library:     library.New(),
		Meter:       metering.NewStoreMeter(store),
	}
}

// Close releases the kernel's resources: it closes the bus and the fact store.
func (k *Kernel) Close() error {
	k.Bus.Close()
	return k.Store.Close()
}
