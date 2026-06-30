package kernel

import (
	"context"
	"fmt"
	"sync"

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
// by its run id, with the memory request from the service's cfg envelope, and
// count it as in-flight on its sequence.
func (rn *Runner) onFire(r triggers.Run) {
	job := scheduler.Job{ID: string(r.Stamp.Run), Request: rn.request(r.Service)}
	rn.mu.Lock()
	rn.pending[job.ID] = r
	st := rn.inflight[r.Stamp.Sequence]
	if st == nil {
		st = &seqState{}
		rn.inflight[r.Stamp.Sequence] = st
	}
	st.inflight++
	rn.mu.Unlock()
	_ = rn.k.Scheduler.Submit(job)
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

// request returns the memory request for a service from its cfg envelope, with a
// minimal floor so a service without a declared request can still be admitted.
func (rn *Runner) request(service string) int64 {
	if svc, err := rn.k.Library.GetDefinition(service); err == nil && svc.Envelope.Request > 0 {
		return svc.Envelope.Request
	}
	return 1
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
