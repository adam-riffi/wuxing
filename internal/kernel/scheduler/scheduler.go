// Package scheduler is the kernel's resource-aware admission control — the only
// thing that sees total resources, every running job, and the queue.
//
// The unit of scheduling is the job (a script run), not the service. A job
// declares a request (the floor it needs to be admitted) and a limit (its burst
// ceiling, applied at launch). Admission is all-or-nothing: a job runs only when
// its full request fits — never partially — so the box never over-commits and
// deadlocks. When nothing fits, jobs queue; the queue is the back-pressure valve.
//
// This file covers single-resource (memory) admission with priority-class +
// FCFS ordering and strict head-of-line blocking. Backfill, the AI-quota window,
// and patience/escalation build on it.
package scheduler

import (
	"fmt"
	"sort"
	"sync"
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

// Job is the unit of scheduling and sizing.
type Job struct {
	ID       string
	Priority Priority
	// Request is the memory floor (bytes) that must be free to admit the job.
	Request int64
	// Limit is the memory burst ceiling applied at launch. The scheduler stores
	// it but admits on Request; the launcher enforces Limit on the container.
	Limit int64
}

// Scheduler admits jobs against a fixed memory capacity. It is safe for
// concurrent use. Admitted jobs are reported through the onAdmit callback, which
// is invoked outside the lock so it may call back into the scheduler.
type Scheduler struct {
	capacity int64
	onAdmit  func(Job)

	mu      sync.Mutex
	free    int64
	queue   []Job
	running map[string]Job
}

// New returns a scheduler with the given memory capacity (bytes). onAdmit is
// called once per job as it is admitted; pass nil to ignore admissions and poll
// Running instead.
func New(capacity int64, onAdmit func(Job)) *Scheduler {
	if onAdmit == nil {
		onAdmit = func(Job) {}
	}
	return &Scheduler{
		capacity: capacity,
		onAdmit:  onAdmit,
		free:     capacity,
		running:  make(map[string]Job),
	}
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
}

// pump admits jobs from the head of the queue while the head fits. Strict
// head-of-line: if the head does not fit, admission stops (no skipping yet).
func (s *Scheduler) pump() []Job {
	var admitted []Job
	for len(s.queue) > 0 {
		head := s.queue[0]
		if head.Request > s.free {
			break
		}
		s.queue = s.queue[1:]
		s.free -= head.Request
		s.running[head.ID] = head
		admitted = append(admitted, head)
	}
	return admitted
}

// Free reports the currently unreserved memory (bytes).
func (s *Scheduler) Free() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.free
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
