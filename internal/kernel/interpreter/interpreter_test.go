package interpreter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/adam-riffi/wuxing/internal/contracts/cfg"
	"github.com/adam-riffi/wuxing/internal/kernel/bus"
	"github.com/adam-riffi/wuxing/internal/kernel/lineage"
)

func seqMinter() *lineage.Minter {
	var n int
	return lineage.NewMinterFunc(func() string { n++; return fmt.Sprintf("id-%d", n) })
}

func sessionStamp() lineage.Stamp {
	return lineage.NewSequence("seq").WithRun("run", 0).WithSession("sess", 0)
}

// connectorsBus registers a "connectors" handler that replies per operation and
// records the operations it was called with.
func connectorsBus(replies map[string]string) (*bus.Bus, *[]string) {
	b := bus.New()
	called := &[]string{}
	_ = b.Register("connectors", func(_ context.Context, e bus.Envelope) bus.Envelope {
		*called = append(*called, e.Operation)
		if r, ok := replies[e.Operation]; ok {
			return e.Reply(json.RawMessage(r))
		}
		return e.Reply(nil)
	})
	return b, called
}

func mtgService() *cfg.Service {
	return &cfg.Service{
		Name: "mtg",
		Workflow: []cfg.Step{
			{ID: "check", Tool: "connectors", Operation: "query", Branch: []cfg.Branch{{When: "new_set == true", Goto: "write"}}},
			{ID: "write", Tool: "connectors", Operation: "write"},
		},
		Successors: []cfg.Successor{{Service: "notifier", Topic: "mtg-db updated", When: "new_set == true"}},
	}
}

func TestInterpreter_Call(t *testing.T) {
	b, _ := connectorsBus(map[string]string{"write": `{"ok":true}`})
	in := New(b, cfg.DefaultVocabulary(), seqMinter())

	out, err := in.Call(context.Background(), sessionStamp(), "connectors", "write", nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if string(out) != `{"ok":true}` {
		t.Errorf("payload: got %s", out)
	}

	// Unknown tool.operation is rejected before routing.
	if _, err := in.Call(context.Background(), sessionStamp(), "connectors", "delete", nil); err == nil {
		t.Error("expected error for unknown connectors.delete")
	}
}

func TestInterpreter_RunTakesBranch(t *testing.T) {
	b, called := connectorsBus(map[string]string{
		"query": `{"new_set":true}`,
		"write": `{"ok":true}`,
	})
	in := New(b, cfg.DefaultVocabulary(), seqMinter())

	fact, err := in.Run(context.Background(), mtgService(), sessionStamp())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(*called) != 2 || (*called)[0] != "query" || (*called)[1] != "write" {
		t.Errorf("call order: got %v want [query write]", *called)
	}
	if fact["new_set"] != true || fact["ok"] != true {
		t.Errorf("accumulated fact: got %v", fact)
	}
}

func TestInterpreter_RunShortCircuitsBranch(t *testing.T) {
	b, called := connectorsBus(map[string]string{
		"query": `{"new_set":false}`, // branch condition fails; no Next -> ends
	})
	in := New(b, cfg.DefaultVocabulary(), seqMinter())

	if _, err := in.Run(context.Background(), mtgService(), sessionStamp()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(*called) != 1 || (*called)[0] != "query" {
		t.Errorf("expected only [query] to run, got %v", *called)
	}
}

func TestInterpreter_Successors(t *testing.T) {
	in := New(bus.New(), cfg.DefaultVocabulary(), seqMinter())
	svc := mtgService()

	fired, err := in.Successors(svc, Fact{"new_set": true})
	if err != nil {
		t.Fatal(err)
	}
	if len(fired) != 1 || fired[0].Service != "notifier" {
		t.Errorf("fired successors: got %+v want [notifier]", fired)
	}

	none, _ := in.Successors(svc, Fact{"new_set": false})
	if len(none) != 0 {
		t.Errorf("no successor should fire when condition fails: %+v", none)
	}
}

func TestInterpreter_HandlerErrorFailsRun(t *testing.T) {
	b := bus.New()
	_ = b.Register("connectors", func(_ context.Context, e bus.Envelope) bus.Envelope {
		return e.ReplyError(errors.New("db down"))
	})
	in := New(b, cfg.DefaultVocabulary(), seqMinter())

	if _, err := in.Run(context.Background(), mtgService(), sessionStamp()); err == nil {
		t.Error("expected Run to fail when a step's handler errors")
	}
}

func TestInterpreter_UnknownStepCall(t *testing.T) {
	in := New(bus.New(), cfg.DefaultVocabulary(), seqMinter())
	svc := &cfg.Service{
		Name:     "bad",
		Workflow: []cfg.Step{{ID: "x", Tool: "connectors", Operation: "delete"}},
	}
	if _, err := in.Run(context.Background(), svc, sessionStamp()); err == nil {
		t.Error("expected Run to fail on an unknown tool.operation")
	}
}
