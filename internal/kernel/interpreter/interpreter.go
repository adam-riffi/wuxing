// Package interpreter is the kernel's central face: read cfg → route. It
// recognizes each tool.operation call in a service's workflow, validates it
// against the known vocabulary, routes it over the bus to the owning tool,
// feeds the return forward, and follows the branch whose condition holds on the
// emitted fact — then evaluates the service's successor declarations.
//
// The russian doll: Call parses/validates/routes a single call; Run sequences
// calls into the service's internal workflow and invokes Call per step.
package interpreter

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/adam-riffi/wuxing/internal/contracts/cfg"
	"github.com/adam-riffi/wuxing/internal/kernel/bus"
	"github.com/adam-riffi/wuxing/internal/kernel/lineage"
)

// Interpreter reads service cfgs and routes their workflow calls over the bus.
type Interpreter struct {
	bus    *bus.Bus
	vocab  cfg.Vocabulary
	minter *lineage.Minter
}

// New returns an interpreter that routes over b, validates against vocab, and
// mints call ids with minter.
func New(b *bus.Bus, vocab cfg.Vocabulary, minter *lineage.Minter) *Interpreter {
	return &Interpreter{bus: b, vocab: vocab, minter: minter}
}

// Call validates a single tool.operation against the vocabulary and routes it
// over the bus, returning the result payload. This is the irreducible "read and
// route".
func (in *Interpreter) Call(ctx context.Context, stamp lineage.Stamp, tool, operation string, payload json.RawMessage) (json.RawMessage, error) {
	if !in.vocab.Known(tool, operation) {
		return nil, fmt.Errorf("interpreter: unknown call %s.%s", tool, operation)
	}
	ret, err := in.bus.Call(ctx, bus.NewCall(stamp, tool, operation, payload))
	if err != nil {
		return nil, err
	}
	if ret.Error != "" {
		return nil, fmt.Errorf("interpreter: %s.%s failed: %s", tool, operation, ret.Error)
	}
	return ret.Payload, nil
}

// Run drives a service's internal workflow from its first step, routing each
// call, accumulating emitted facts, and following the branch whose condition
// holds (or Next) until no successor. It returns the accumulated fact.
func (in *Interpreter) Run(ctx context.Context, svc *cfg.Service, stamp lineage.Stamp) (Fact, error) {
	if len(svc.Workflow) == 0 {
		return Fact{}, nil
	}
	steps := make(map[string]cfg.Step, len(svc.Workflow))
	for _, s := range svc.Workflow {
		steps[s.ID] = s
	}

	acc := Fact{}
	var payload json.RawMessage
	cur := svc.Workflow[0].ID
	order := 0

	for cur != "" {
		step, ok := steps[cur]
		if !ok {
			return nil, fmt.Errorf("interpreter: %q: workflow references missing step %q", svc.Name, cur)
		}

		callStamp := stamp.WithCall(in.minter.CallID(), order)
		out, err := in.Call(ctx, callStamp, step.Tool, step.Operation, payload)
		if err != nil {
			return nil, err
		}
		mergeFact(acc, out)
		payload = out

		cur, err = nextStep(step, acc)
		if err != nil {
			return nil, err
		}
		order++
	}
	return acc, nil
}

// Successors returns the successor declarations whose conditions hold on the
// emitted fact — the cascade the kernel's triggers will fire (inheriting the
// sequence). A successor with no condition always fires.
func (in *Interpreter) Successors(svc *cfg.Service, fact Fact) ([]cfg.Successor, error) {
	var fired []cfg.Successor
	for _, s := range svc.Successors {
		if s.When == "" {
			fired = append(fired, s)
			continue
		}
		ok, err := evalCondition(s.When, fact)
		if err != nil {
			return nil, err
		}
		if ok {
			fired = append(fired, s)
		}
	}
	return fired, nil
}

// nextStep picks the next step: the first branch whose condition holds, else the
// unconditional Next (empty ends the workflow).
func nextStep(step cfg.Step, fact Fact) (string, error) {
	for _, b := range step.Branch {
		ok, err := evalCondition(b.When, fact)
		if err != nil {
			return "", err
		}
		if ok {
			return b.Goto, nil
		}
	}
	return step.Next, nil
}

func mergeFact(acc Fact, out json.RawMessage) {
	if len(out) == 0 {
		return
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		return // non-object output contributes no facts
	}
	for k, v := range m {
		acc[k] = v
	}
}
