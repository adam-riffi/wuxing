package triggers

import (
	"context"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

// ParseCron validates a cron spec the way the watcher will read it: standard
// 5-field expressions, descriptors (@hourly, @every 10s), and an optional
// CRON_TZ=<zone> prefix. Shared with cfg validation so a bad spec fails at load,
// not at fire time.
func ParseCron(spec string) (cron.Schedule, error) {
	return cron.ParseStandard(spec)
}

// StartCron parses every registered cron rule and starts the watcher: a
// goroutine that sleeps until the earliest due schedule, fires each due service
// as the head of a new sequence (FireExternal), and reschedules. It returns the
// number of schedules watched; a bad spec fails the start (never a half-armed
// watcher). Rules registered after the start are not picked up — the daemon
// registers services before starting the watcher.
func (t *Triggers) StartCron(ctx context.Context) (int, error) {
	type entry struct {
		service string
		sched   cron.Schedule
		next    time.Time
	}

	t.mu.RLock()
	now := time.Now()
	entries := make([]*entry, 0, len(t.cron))
	for svc, spec := range t.cron {
		sched, err := ParseCron(spec)
		if err != nil {
			t.mu.RUnlock()
			return 0, fmt.Errorf("triggers: cron spec %q for service %q: %w", spec, svc, err)
		}
		entries = append(entries, &entry{service: svc, sched: sched, next: sched.Next(now)})
	}
	t.mu.RUnlock()

	if len(entries) == 0 {
		return 0, nil
	}

	go func() {
		timer := time.NewTimer(0)
		defer timer.Stop()
		for {
			// Sleep until the earliest schedule comes due.
			earliest := entries[0].next
			for _, e := range entries[1:] {
				if e.next.Before(earliest) {
					earliest = e.next
				}
			}
			timer.Reset(time.Until(earliest))

			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}

			now := time.Now()
			for _, e := range entries {
				if !e.next.After(now) {
					t.FireExternal(e.service, KindCron)
					e.next = e.sched.Next(now)
				}
			}
		}
	}()
	return len(entries), nil
}
