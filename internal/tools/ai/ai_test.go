package ai

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/adam-riffi/wuxing/internal/kernel/bus"
	"github.com/adam-riffi/wuxing/internal/kernel/lineage"
)

type fakeBackend struct {
	result Result
	err    error
	gotReq Request
	calls  int
}

func (f *fakeBackend) Infer(_ context.Context, req Request) (Result, error) {
	f.calls++
	f.gotReq = req
	return f.result, f.err
}

type recMeter struct{ facts []CallFact }

func (m *recMeter) Call(f CallFact) { m.facts = append(m.facts, f) }

func stamp() lineage.Stamp { return lineage.NewSequence("seq").WithRun("run", 0) }

func inferCall(payload string) bus.Envelope {
	return bus.NewCall(stamp(), "ai", "infer", json.RawMessage(payload))
}

func TestAI_Infer(t *testing.T) {
	be := &fakeBackend{result: Result{
		Output: json.RawMessage(`"a new set dropped"`),
		Model:  "gpt-x", TokensIn: 12, TokensOut: 8, Cost: 0.0021, TTFTMs: 90,
	}}
	meter := &recMeter{}
	tool := New(be, meter)

	ret := tool.Handler()(context.Background(), inferCall(`{"prompt":"summarize","model":"gpt-x"}`))
	if ret.Error != "" {
		t.Fatalf("infer errored: %s", ret.Error)
	}
	if string(ret.Payload) != `"a new set dropped"` {
		t.Errorf("output: got %s", ret.Payload)
	}
	if be.gotReq.Prompt != "summarize" {
		t.Errorf("prompt not passed: %q", be.gotReq.Prompt)
	}
	if len(meter.facts) != 1 || meter.facts[0].Cost != 0.0021 || meter.facts[0].TurnCount != 1 {
		t.Errorf("cost fact: %+v", meter.facts)
	}
}

func TestAI_SchemaFailureStillRecordsCost(t *testing.T) {
	// Backend returns non-JSON output but the caller asked for a schema.
	be := &fakeBackend{result: Result{Output: json.RawMessage(`not json`), Cost: 0.005, Model: "m"}}
	meter := &recMeter{}
	tool := New(be, meter)

	ret := tool.Handler()(context.Background(), inferCall(`{"prompt":"p","schema":{"type":"object"}}`))
	if ret.Error == "" {
		t.Fatal("expected schema validation error")
	}
	if len(meter.facts) != 1 || meter.facts[0].Cost != 0.005 {
		t.Errorf("cost must be recorded at incur-time even on schema failure: %+v", meter.facts)
	}
}

func TestAI_BackendError(t *testing.T) {
	be := &fakeBackend{err: errors.New("model unavailable")}
	meter := &recMeter{}
	tool := New(be, meter)

	ret := tool.Handler()(context.Background(), inferCall(`{"prompt":"p"}`))
	if ret.Error == "" {
		t.Error("backend error should surface")
	}
	if len(meter.facts) != 0 {
		t.Error("no cost fact when the backend itself failed")
	}
}

func TestAI_UnknownOperation(t *testing.T) {
	tool := New(&fakeBackend{}, nil)
	ret := tool.Handler()(context.Background(), bus.NewCall(stamp(), "ai", "dream", nil))
	if ret.Error == "" {
		t.Error("unknown operation should error")
	}
}

func TestAI_RegistersOnBus(t *testing.T) {
	be := &fakeBackend{result: Result{Output: json.RawMessage(`"ok"`)}}
	tool := New(be, nil)
	b := bus.New()
	if err := b.Register("ai", tool.Handler()); err != nil {
		t.Fatal(err)
	}
	ret, err := b.Call(context.Background(), inferCall(`{"prompt":"hi"}`))
	if err != nil || ret.Error != "" {
		t.Fatalf("end-to-end infer over the bus failed: err=%v ret.Error=%s", err, ret.Error)
	}
}
