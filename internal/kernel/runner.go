package kernel

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/adam-riffi/wuxing/internal/contracts/cfg"
	"github.com/adam-riffi/wuxing/internal/kernel/interpreter"
	"github.com/adam-riffi/wuxing/internal/kernel/lineage"
	"github.com/adam-riffi/wuxing/internal/kernel/scheduler"
	"github.com/adam-riffi/wuxing/internal/kernel/sessions"
	"github.com/adam-riffi/wuxing/internal/kernel/triggers"
	"github.com/adam-riffi/wuxing/internal/storage/facts"
)

// Runner drives the kernel: it turns fired triggers into admission-gated runs and
// propagates the successor cascade. Triggers' onFire submits a job; the
// scheduler's onAdmit executes the run once admitted, frees the job, then fires
// the run's successors (inheriting the sequence) so the cascade self-propagates.
// It tracks in-flight runs per sequence and closes the sequence when the last
// one completes — the chain head is opened by the external trigger and closed
// here when the cascade ends.
type Runner struct {
	k *Kernel

	mu       sync.Mutex
	pending  map[string]triggers.Run          // scheduler job id -> the run to execute
	inflight map[lineage.SequenceID]*seqState // open sequence -> in-flight bookkeeping
}

type seqState struct {
	inflight int
	failed   bool
}

func newRunner() *Runner {
	return &Runner{
		pending:  make(map[string]triggers.Run),
		inflight: make(map[lineage.SequenceID]*seqState),
	}
}

// onFire is the triggers callback: submit the fired run to the scheduler, keyed
// by its run id, carrying the service's full cfg envelope (request, limit, AI
// window, priority, patience), and count it as in-flight on its sequence.
func (rn *Runner) onFire(r triggers.Run) {
	if rn.skipOverlap(r.Service) {
		return // concurrency: forbid — a run of this service is already in flight
	}

	job := rn.jobFor(r)
	rn.mu.Lock()
	rn.pending[job.ID] = r
	st := rn.inflight[r.Stamp.Sequence]
	if st == nil {
		st = &seqState{}
		rn.inflight[r.Stamp.Sequence] = st
	}
	st.inflight++
	rn.mu.Unlock()

	if err := rn.k.Scheduler.Submit(job); err != nil {
		// The job was refused outright (never admitted): roll back the
		// bookkeeping and count it as a failed run on its sequence, so nothing
		// leaks. Register's Fits check makes this rare (duplicate ids, races).
		rn.mu.Lock()
		delete(rn.pending, job.ID)
		rn.mu.Unlock()
		rn.complete(r.Stamp.Sequence, true)
	}
}

// skipOverlap reports whether this fire should be dropped because the service
// declares `concurrency: forbid` and already has a run in flight — queued
// (pending) or executing (a live session). The k8s CronJob "Forbid" semantics:
// skip the overlapping fire, don't queue it.
func (rn *Runner) skipOverlap(service string) bool {
	svc, err := rn.k.Library.GetDefinition(service)
	if err != nil || svc.Concurrency != "forbid" {
		return false
	}
	if rn.k.Sessions.HasRunning(service) {
		return true
	}
	rn.mu.Lock()
	defer rn.mu.Unlock()
	for _, p := range rn.pending {
		if p.Service == service {
			return true
		}
	}
	return false
}

// onAdmit is the scheduler callback: execute the admitted run, free its
// resources, fire its successors so the cascade continues, then mark the run
// complete (closing the sequence if it was the last in-flight run).
func (rn *Runner) onAdmit(job scheduler.Job) {
	rn.mu.Lock()
	r, ok := rn.pending[job.ID]
	delete(rn.pending, job.ID)
	rn.mu.Unlock()
	if !ok {
		return
	}

	_, succ, err := rn.k.Run(context.Background(), r)
	rn.k.Scheduler.Complete(job.ID)

	if err == nil {
		fired := make(map[string]bool, len(succ))
		for _, s := range succ {
			if s.Topic == "" || fired[s.Topic] {
				continue
			}
			fired[s.Topic] = true
			// Emit the successor's topic, inheriting this run's sequence; the
			// registered successor services fire via onFire (incrementing inflight).
			rn.k.Triggers.OnEvent(s.Topic, r.Stamp)
		}
	}

	rn.complete(r.Stamp.Sequence, err != nil)
}

// complete records one run finishing on its sequence and closes the sequence
// (with the aggregate outcome) when no runs remain in flight.
func (rn *Runner) complete(seq lineage.SequenceID, failed bool) {
	rn.mu.Lock()
	st := rn.inflight[seq]
	if st == nil {
		rn.mu.Unlock()
		return
	}
	if failed {
		st.failed = true
	}
	st.inflight--
	done := st.inflight <= 0
	if done {
		delete(rn.inflight, seq)
	}
	aggregateFailed := st.failed
	rn.mu.Unlock()

	if !done {
		return
	}
	outcome := facts.OutcomeSuccess
	if aggregateFailed {
		outcome = facts.OutcomeFailure
	}
	ctx := context.Background()
	_ = facts.NewSpine(rn.k.Store).CloseSequence(ctx, seq, outcome)

	// Derive the sequence rollup once the whole cascade has closed (best-effort).
	_, _ = rn.k.Processors.SummarizeSequence(ctx, string(seq))
}

// jobFor maps a service's cfg envelope onto a scheduler job — the point where
// the declared contract (request/limit/ai_request/priority/max_wait/on_starve)
// becomes the enforced one. A service without a declared request gets a minimal
// floor so it can still be admitted.
func (rn *Runner) jobFor(r triggers.Run) scheduler.Job {
	job := scheduler.Job{ID: string(r.Stamp.Run), Request: 1}
	svc, err := rn.k.Library.GetDefinition(r.Service)
	if err != nil {
		return job
	}
	env := svc.Envelope
	if env.Request > 0 {
		job.Request = env.Request
	}
	job.Limit = env.Limit
	job.AIRequest = env.AIRequest
	if env.Priority == "user" {
		job.Priority = scheduler.PriorityUser
	}
	if env.MaxWait != "" {
		if d, perr := time.ParseDuration(env.MaxWait); perr == nil {
			job.MaxWait = d
		}
	}
	if env.OnStarve == "fail" {
		job.OnExpiry = scheduler.ExpiryFail // default is escalate
	}
	return job
}

// onExpire is the scheduler callback for a job dropped by ExpiryFail: the run
// never got admitted, so release its bookkeeping and count it as a failed run
// on its sequence (closing the sequence if it was the last in flight).
func (rn *Runner) onExpire(job scheduler.Job) {
	rn.mu.Lock()
	r, ok := rn.pending[job.ID]
	delete(rn.pending, job.ID)
	rn.mu.Unlock()
	if !ok {
		return
	}
	rn.complete(r.Stamp.Sequence, true)
}

// Run executes one triggered service run end to end (in-process, cfg-only path):
// it looks up the service, opens the spine (sequence/run/session), drives the
// service's workflow through the interpreter, closes the run with its outcome,
// and returns the successor declarations whose conditions fired (the kernel's
// triggers then start those, inheriting the sequence).
//
// An external trigger (cron/manual) opens a NEW sequence; an event trigger
// inherits one already opened by the originating run. Running a service whose
// scripts need a container is the launcher's job and needs the Docker engine.
func (k *Kernel) Run(ctx context.Context, r triggers.Run) (interpreter.Fact, []cfg.Successor, error) {
	svc, err := k.Library.GetDefinition(r.Service)
	if err != nil {
		return nil, nil, err
	}

	spine := facts.NewSpine(k.Store)
	st := r.Stamp

	if r.Kind != triggers.KindEvent {
		if err := spine.OpenSequence(ctx, st.Sequence, ""); err != nil {
			return nil, nil, err
		}
	}
	if err := spine.OpenRun(ctx, st); err != nil {
		return nil, nil, err
	}

	// A single unit of work within the run.
	st = st.WithSession(k.Minter.SessionID(), 0)
	if err := spine.OpenSession(ctx, st); err != nil {
		return nil, nil, err
	}
	if err := k.Sessions.Open(sessions.Session{ID: st.Session, Run: st.Run, Service: r.Service}); err != nil {
		return nil, nil, fmt.Errorf("kernel: register session: %w", err)
	}

	fact, runErr := k.Interpreter.Run(ctx, svc, st)

	outcome := facts.OutcomeSuccess
	if runErr != nil {
		outcome = facts.OutcomeFailure
	}
	_ = spine.CloseSession(ctx, st.Session, outcome)
	k.Sessions.Close(st.Session)
	_ = spine.CloseRun(ctx, st.Run, outcome)

	// Derive the run's rollup now that its facts are all recorded (best-effort:
	// a missed derivation is recomputable and must not fail the run).
	_, _ = k.Processors.SummarizeRun(ctx, string(st.Run))

	if runErr != nil {
		return nil, nil, runErr
	}

	succ, err := k.Interpreter.Successors(svc, fact)
	return fact, succ, err
}
