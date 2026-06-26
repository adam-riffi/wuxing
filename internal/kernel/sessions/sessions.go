// Package sessions is the kernel's running registry: it tracks live instances —
// what is running right now and which service each belongs to. It answers the
// drain check the library's deregister consults: a service cannot be reindexed
// while it has running instances.
//
// "Is it running?" is a live read, never stored state. The registry holds only
// currently-open sessions; closing one removes it (its lasting record is the
// spine fact written by the lineage face, not kept here).
package sessions

import (
	"fmt"
	"sync"
	"time"

	"github.com/adam-riffi/wuxing/internal/kernel/lineage"
)

// Session is a live unit of work tracked in the running registry.
type Session struct {
	ID       lineage.SessionID
	Run      lineage.RunID
	Service  string
	OpenedAt time.Time
}

// Registry tracks the currently-open sessions. It is safe for concurrent use.
type Registry struct {
	now func() time.Time

	mu       sync.RWMutex
	sessions map[lineage.SessionID]Session
}

// Option configures a Registry.
type Option func(*Registry)

// WithClock overrides the clock used to stamp OpenedAt (for tests).
func WithClock(now func() time.Time) Option {
	return func(r *Registry) {
		if now != nil {
			r.now = now
		}
	}
}

// New returns an empty running registry.
func New(opts ...Option) *Registry {
	r := &Registry{now: time.Now, sessions: make(map[lineage.SessionID]Session)}
	for _, o := range opts {
		o(r)
	}
	return r
}

// Open registers a live session. It errors on a missing id/service or a
// duplicate id.
func (r *Registry) Open(s Session) error {
	if s.ID == "" {
		return fmt.Errorf("sessions: session has no id")
	}
	if s.Service == "" {
		return fmt.Errorf("sessions: session %q has no service", s.ID)
	}
	if s.OpenedAt.IsZero() {
		s.OpenedAt = r.now()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.sessions[s.ID]; dup {
		return fmt.Errorf("sessions: session %q already open", s.ID)
	}
	r.sessions[s.ID] = s
	return nil
}

// Close removes a session from the registry and returns it. The bool is false if
// the session was not open.
func (r *Registry) Close(id lineage.SessionID) (Session, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[id]
	if ok {
		delete(r.sessions, id)
	}
	return s, ok
}

// HasRunning reports whether the service has any live session — the drain check
// the library's deregister consults before removing a service.
func (r *Registry) HasRunning(service string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, s := range r.sessions {
		if s.Service == service {
			return true
		}
	}
	return false
}

// Running returns a snapshot of the currently-open sessions.
func (r *Registry) Running() []Session {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Session, 0, len(r.sessions))
	for _, s := range r.sessions {
		out = append(out, s)
	}
	return out
}

// Count reports the number of live sessions.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.sessions)
}
