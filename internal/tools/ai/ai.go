// Package ai is the inference tool: run model work for a service and return a
// result, registered as the "ai" tool on the bus. It implements one-shot infer
// (autocomplete is infer-family) and a bounded "agent" mode that drives a real
// agent CLI (Hermes Agent, Codex, Open Design, …) headlessly in a scratch
// directory — see agent.go.
//
// The model backend is an interface — Codex CLI in production, a fake in tests.
// Every call emits an ai detail fact (model, tokens, cost, ttft) via the Meter:
// the cost is recorded at incur-time, not derived, and emitted even when a
// schema-constrained output later fails validation.
package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/adam-riffi/wuxing/internal/kernel/bus"
	"github.com/adam-riffi/wuxing/internal/kernel/lineage"
)

// Request is an inference request.
type Request struct {
	Prompt string
	Model  string
	Schema json.RawMessage // optional; when set the output must be valid JSON
}

// Result is the model output plus the usage/cost a call incurred.
type Result struct {
	Output    json.RawMessage
	Model     string
	TokensIn  int
	TokensOut int
	Cost      float64
	TTFTMs    int
}

// Backend runs inference. Codex CLI implements it in production; tests use a fake.
type Backend interface {
	Infer(ctx context.Context, req Request) (Result, error)
}

// CallFact is the ai detail fact emitted per call (the cost/window-burn lane).
type CallFact struct {
	Stamp     lineage.Stamp
	Model     string
	Mode      string
	TokensIn  int
	TokensOut int
	Cost      float64
	TTFTMs    int
	TurnCount int
}

// Meter receives ai detail facts; the kernel wires it to storage.
type Meter interface {
	Call(CallFact)
}

// NopMeter discards facts.
type NopMeter struct{}

// Call implements Meter.
func (NopMeter) Call(CallFact) {}

// Tool is the ai tool over a backend, metered. An optional agent backend (set
// via WithAgent) enables the model-driven "agent" operation.
type Tool struct {
	backend Backend
	agent   AgentBackend
	meter   Meter
}

// New returns an ai tool over backend, reporting facts to meter (nil discards).
func New(backend Backend, meter Meter) *Tool {
	if meter == nil {
		meter = NopMeter{}
	}
	return &Tool{backend: backend, meter: meter}
}

// Handler returns the bus handler that answers ai calls.
func (t *Tool) Handler() bus.Handler {
	return func(ctx context.Context, e bus.Envelope) bus.Envelope {
		switch e.Operation {
		case "infer", "autocomplete":
			return t.handleInfer(ctx, e)
		case "agent":
			return t.handleAgent(ctx, e)
		default:
			return e.ReplyError(fmt.Errorf("ai: unsupported operation %q", e.Operation))
		}
	}
}

type inferRequest struct {
	Prompt string          `json:"prompt"`
	Model  string          `json:"model"`
	Schema json.RawMessage `json:"schema"`
}

func (t *Tool) handleInfer(ctx context.Context, e bus.Envelope) bus.Envelope {
	var req inferRequest
	if len(e.Payload) > 0 {
		if err := json.Unmarshal(e.Payload, &req); err != nil {
			return e.ReplyError(fmt.Errorf("ai: bad infer payload: %w", err))
		}
	}

	res, err := t.backend.Infer(ctx, Request(req))
	if err != nil {
		return e.ReplyError(fmt.Errorf("ai: infer: %w", err))
	}

	// Cost is recorded at incur-time — before output validation, so it is
	// captured even when a schema-constrained output fails to validate.
	t.meter.Call(CallFact{
		Stamp:     e.Stamp,
		Model:     res.Model,
		Mode:      "infer",
		TokensIn:  res.TokensIn,
		TokensOut: res.TokensOut,
		Cost:      res.Cost,
		TTFTMs:    res.TTFTMs,
		TurnCount: 1,
	})

	if len(req.Schema) > 0 && !json.Valid(res.Output) {
		return e.ReplyError(fmt.Errorf("ai: schema-constrained output is not valid JSON"))
	}
	return e.Reply(res.Output)
}
