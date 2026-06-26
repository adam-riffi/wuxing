// Package scheduler is the kernel's resource-aware admission control — the only
// thing that sees total resources, every running job, and the queue.
//
// The unit of scheduling is the job (a script run), not the service. A job
// declares a request (the floor it needs to be admitted) and a limit (its burst
// ceiling, applied at launch). Admission is all-or-nothing: a job runs only when
// its full request fits — never partially — so the box never over-commits and
// deadlocks. When nothing fits, jobs queue; the queue is the back-pressure valve.
//
// This file covers dual-resource admission — memory plus the AI-quota window —
// with priority-class + FCFS ordering and backfill. The two resources refill
// oppositely: memory frees on completion, while the AI window refills on a clock
// (RefillWindow), not when a job finishes. An AI job is admitted only when both
// have room. When the head does not fit, smaller waiting jobs backfill the gap
// it cannot use; the head is retried first on every change, so it keeps its
// claim. Backfill is opportunistic — without job-duration estimates it cannot
// prove a backfilled job won't delay the head, so it only ever uses room the
// head currently cannot. Patience (max_wait) with escalate/fail on expiry layers
// on top via Expire; overclock into a reserve band is still to come.
package scheduler

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// Priority is a job's scheduling class. Higher classes are admitted first;
// within a class, ordering is first-come-first-served.
type Priority int

const (
	// PriorityBackground is routine, non-interactive work.
	PriorityBackground Priority = iota
	// PriorityUser is user-triggered work, admitted ahead of background.
	PriorityUser
)

// Expiry is what happens to a queued job that waits past its MaxWait.
type Expiry int

const (
	// ExpiryEscalate ages the job's priority up one class (the lean default).
	ExpiryEscalate Expiry = iota
	// ExpiryFail drops the job from the queue and reports it via the onExpire callback.
	ExpiryFail
)

// Job is the unit of scheduling and sizing.
type Job struct {
	ID       string
	Priority Priority
	// Request is the memory floor (bytes) that must be free to admit the job.
	Request int64
	// Limit is the memory burst ceiling applied at launch. The scheduler stores
	// it but admits on Request; the launcher enforces Limit on the container.
	Limit int64
	// AIRequest is the AI-quota window the job consumes (0 for non-AI jobs). An
	// AI job is admitted only when both memory and window have room. The window
	// is not returned on completion — it refills on a clock via RefillWindow.
	AIRequest int64
	// MaxWait is how long the job tolerates queueing before its OnExpiry policy
	// fires (0 means infinite patience).
	MaxWait time.Duration
	// OnExpiry is applied when the job has waited past MaxWait.
	OnExpiry Expiry
}

// Scheduler admits jobs against a fixed memory capacity. It is safe for
// concurrent use. Admitted jobs are reported through the onAdmit callback, which
// is invoked outside the lock so it may call back into the scheduler.
type Scheduler struct {
	capacity  int64
	windowCap int64
	onAdmit   func(Job)
	onExpire  func(Job)
	now       func() time.Time

	mu         sync.Mutex
	free       int64
	window     int64
	queue      []Job
	running    map[string]Job
	enqueuedAt map[string]time.Time
}

// Option configures a Scheduler.
type Option func(*Scheduler)

// WithAIWindow enables AI-job admission against an AI-quota window of the given
// capacity, starting full. Without it the window is zero and any job with a
// positive AIRequest can never be admitted.
func WithAIWindow(capacity int64) Option {
	return func(s *Scheduler) {
		s.windowCap = capacity
		s.window = capacity
	}
}

// WithOnExpire registers a callback invoked once per job dropped by an
// ExpiryFail policy (so the kernel can record the outcome).
func WithOnExpire(cb func(Job)) Option {
	return func(s *Scheduler) {
		if cb != nil {
			s.onExpire = cb
		}
	}
}

// WithClock overrides the clock used to age queued jobs (for tests).
func WithClock(now func() time.Time) Option {
	return func(s *Scheduler) {
		if now != nil {
			s.now = now
		}
	}
}

// New returns a scheduler with the given memory capacity (bytes). onAdmit is
// called once per job as it is admitted; pass nil to ignore admissions and poll
// Running instead.
func New(capacity int64, onAdmit func(Job), opts ...Option) *Scheduler {
	if onAdmit == nil {
		onAdmit = func(Job) {}
	}
	s := &Scheduler{
		capacity:   capacity,
		onAdmit:    onAdmit,
		onExpire:   func(Job) {},
		now:        time.Now,
		free:       capacity,
		running:    make(map[string]Job),
		enqueuedAt: make(map[string]time.Time),
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Submit validates and enqueues a job, then admits whatever now fits. It errors
// if the job is invalid or its request can never fit the capacity.
func (s *Scheduler) Submit(job Job) error {
	s.mu.Lock()
	if err := s.validate(job); err != nil {
		s.mu.Unlock()
		return err
	}
	s.enqueue(job)
	admitted := s.pump()
	s.mu.Unlock()

	for _, j := range admitted {
		s.onAdmit(j)
	}
	return nil
}

// Complete releases a running job's reserved memory and admits whatever now
// fits. Completing an unknown job is a no-op.
func (s *Scheduler) Complete(jobID string) {
	s.mu.Lock()
	if j, ok := s.running[jobID]; ok {
		s.free += j.Request
		delete(s.running, jobID)
	}
	admitted := s.pump()
	s.mu.Unlock()

	for _, j := range admitted {
		s.onAdmit(j)
	}
}

func (s *Scheduler) validate(job Job) error {
	if job.ID == "" {
		return fmt.Errorf("scheduler: job has no id")
	}
	if job.Request <= 0 {
		return fmt.Errorf("scheduler: job %q request must be positive", job.ID)
	}
	if job.Request > s.capacity {
		return fmt.Errorf("scheduler: job %q request %d exceeds capacity %d", job.ID, job.Request, s.capacity)
	}
	if job.AIRequest < 0 {
		return fmt.Errorf("scheduler: job %q ai request must not be negative", job.ID)
	}
	if job.AIRequest > s.windowCap {
		return fmt.Errorf("scheduler: job %q ai request %d exceeds window capacity %d", job.ID, job.AIRequest, s.windowCap)
	}
	if _, running := s.running[job.ID]; running {
		return fmt.Errorf("scheduler: job %q already running", job.ID)
	}
	for _, q := range s.queue {
		if q.ID == job.ID {
			return fmt.Errorf("scheduler: job %q already queued", job.ID)
		}
	}
	return nil
}

// enqueue inserts job after every waiting job of equal or higher priority, so
// the queue stays ordered by priority (desc) then FCFS.
func (s *Scheduler) enqueue(job Job) {
	i := sort.Search(len(s.queue), func(i int) bool { return s.queue[i].Priority < job.Priority })
	s.queue = append(s.queue, Job{})
	copy(s.queue[i+1:], s.queue[i:])
	s.queue[i] = job
	s.enqueuedAt[job.ID] = s.now()
}

// pump admits every waiting job that fits, scanning in priority/FCFS order. The
// head is tried first; a job the head is too large for is skipped so a smaller
// job behind it can backfill the gap. Because the queue stays ordered and the
// head is retried on every pump, the head keeps first claim on freed memory.
func (s *Scheduler) pump() []Job {
	var admitted []Job
	for i := 0; i < len(s.queue); {
		job := s.queue[i]
		if !s.fits(job) {
			i++ // head (or this job) does not fit; try to backfill the next
			continue
		}
		s.free -= job.Request
		s.window -= job.AIRequest
		s.running[job.ID] = job
		delete(s.enqueuedAt, job.ID)
		admitted = append(admitted, job)
		s.queue = append(s.queue[:i], s.queue[i+1:]...) // queue shifts into i
	}
	return admitted
}

// fits reports whether both resources have room for the job now.
func (s *Scheduler) fits(j Job) bool {
	return j.Request <= s.free && j.AIRequest <= s.window
}

// Free reports the currently unreserved memory (bytes).
func (s *Scheduler) Free() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.free
}

// Window reports the AI-quota window currently remaining.
func (s *Scheduler) Window() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.window
}

// Expire applies each waiting job's MaxWait/OnExpiry policy: a job queued longer
// than its MaxWait either escalates (priority up one class, wait timer reset) or
// fails (dropped, reported via the onExpire callback). It then admits whatever
// the re-ordering makes room for. Drive it periodically from the kernel.
func (s *Scheduler) Expire() {
	s.mu.Lock()
	now := s.now()
	var failed []Job
	escalated := false
	kept := s.queue[:0]
	for _, job := range s.queue {
		at, ok := s.enqueuedAt[job.ID]
		if job.MaxWait > 0 && ok && now.Sub(at) >= job.MaxWait {
			switch job.OnExpiry {
			case ExpiryFail:
				delete(s.enqueuedAt, job.ID)
				failed = append(failed, job)
				continue
			case ExpiryEscalate:
				if job.Priority < PriorityUser {
					job.Priority++
					escalated = true
				}
				s.enqueuedAt[job.ID] = now // reset the wait timer
			}
		}
		kept = append(kept, job)
	}
	s.queue = kept
	if escalated {
		sort.SliceStable(s.queue, func(i, j int) bool {
			return s.queue[i].Priority > s.queue[j].Priority
		})
	}
	admitted := s.pump()
	s.mu.Unlock()

	for _, j := range admitted {
		s.onAdmit(j)
	}
	for _, j := range failed {
		s.onExpire(j)
	}
}

// RefillWindow returns amount of AI-quota window (the clock-driven refill),
// capped at the window capacity, then admits whatever now fits.
func (s *Scheduler) RefillWindow(amount int64) {
	s.mu.Lock()
	s.window += amount
	if s.window > s.windowCap {
		s.window = s.windowCap
	}
	admitted := s.pump()
	s.mu.Unlock()

	for _, j := range admitted {
		s.onAdmit(j)
	}
}

// Running reports the ids of currently running jobs.
func (s *Scheduler) Running() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.running))
	for id := range s.running {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Queued reports the ids of waiting jobs in admission order.
func (s *Scheduler) Queued() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, len(s.queue))
	for i, j := range s.queue {
		ids[i] = j.ID
	}
	return ids
}
