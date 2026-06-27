package kernel

import (
	"context"
	"fmt"

	"github.com/adam-riffi/wuxing/internal/contracts/cfg"
	"github.com/adam-riffi/wuxing/internal/kernel/interpreter"
	"github.com/adam-riffi/wuxing/internal/kernel/sessions"
	"github.com/adam-riffi/wuxing/internal/kernel/triggers"
	"github.com/adam-riffi/wuxing/internal/storage/facts"
)

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

	if runErr != nil {
		return nil, nil, runErr
	}

	succ, err := k.Interpreter.Successors(svc, fact)
	return fact, succ, err
}
