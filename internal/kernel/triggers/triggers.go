// Package triggers watches for initiation and fires runs. It is the kernel's
// flow-control entry: schedule (cron) and event (bus-event) rules decide what
// runs, and — crucially — it carries the sequence id across the event boundary.
//
// The one new mechanism the data model requires of the bus: an external trigger
// (cron/manual) opens a NEW sequence (the chain head), while an event trigger
// INHERITS the sequence carried by the triggering event, so a cascade of runs
// across services shares one sequence_id and can be read back in causal order.
package triggers

import (
	"sync"

	"github.com/adam-riffi/wuxing/internal/kernel/lineage"
)

// Kind is how a run was initiated.
type Kind string

const (
	// KindCron is a schedule-driven external trigger.
	KindCron Kind = "cron"
	// KindManual is an operator-driven external trigger.
	KindManual Kind = "manual"
	// KindEvent is a bus-event trigger (a successor in a cascade).
	KindEvent Kind = "event"
)

// Run is a fired trigger: the kernel should start Service with the lineage Stamp
// that triggers assigned (a fresh run id under either a new or inherited
// sequence).
type Run struct {
	Service string
	Kind    Kind
	Topic   string // the event topic, for event-triggered runs
	Stamp   lineage.Stamp
}

// Triggers holds the registered rules and fires runs through onFire.
type Triggers struct {
	minter *lineage.Minter
	onFire func(Run)

	mu    sync.RWMutex
	cron  map[string]string   // service -> cron spec
	event map[string][]string // topic -> services
}

// New returns a Triggers using minter to mint ids and reporting fired runs
// through onFire (nil ignores them; callers can use the returned Run).
func New(minter *lineage.Minter, onFire func(Run)) *Triggers {
	if onFire == nil {
		onFire = func(Run) {}
	}
	return &Triggers{
		minter: minter,
		onFire: onFire,
		cron:   make(map[string]string),
		event:  make(map[string][]string),
	}
}

// RegisterCron declares that service runs on the given cron spec.
func (t *Triggers) RegisterCron(service, spec string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cron[service] = spec
}

// RegisterEvent declares that service runs when an event fires on topic (a
// successor declaration).
func (t *Triggers) RegisterEvent(service, topic string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.event[topic] = append(t.event[topic], service)
}

// FireExternal fires service as the head of a NEW sequence (cron tick or manual
// trigger). The run is the first in its sequence (sequence_order 0).
func (t *Triggers) FireExternal(service string, kind Kind) Run {
	stamp := lineage.NewSequence(t.minter.SequenceID()).WithRun(t.minter.RunID(), 0)
	r := Run{Service: service, Kind: kind, Stamp: stamp}
	t.onFire(r)
	return r
}

// UnregisterService removes all cron and event rules declared for name. The
// running cron goroutine (started by StartCron) will still fire until the
// daemon restarts; the interpreter fails-fast on the stale fire because the
// service is no longer in the library.
func (t *Triggers) UnregisterService(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.cron, name)
	for topic, svcs := range t.event {
		out := svcs[:0]
		for _, s := range svcs {
			if s != name {
				out = append(out, s)
			}
		}
		if len(out) == 0 {
			delete(t.event, topic)
		} else {
			t.event[topic] = out
		}
	}
}

// OnEvent fires every service registered for topic, each INHERITING the
// sequence carried by the triggering event and taking the next sequence_order,
// so the cascade stays one sequence. Events with no successor are a no-op.
func (t *Triggers) OnEvent(topic string, cause lineage.Stamp) []Run {
	t.mu.RLock()
	services := append([]string(nil), t.event[topic]...)
	t.mu.RUnlock()

	runs := make([]Run, 0, len(services))
	for _, svc := range services {
		stamp := lineage.NewSequence(cause.Sequence).
			WithRun(t.minter.RunID(), cause.SequenceOrder+1)
		r := Run{Service: svc, Kind: KindEvent, Topic: topic, Stamp: stamp}
		t.onFire(r)
		runs = append(runs, r)
	}
	return runs
}
