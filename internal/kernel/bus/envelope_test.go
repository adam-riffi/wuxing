package bus

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/adam-riffi/wuxing/internal/kernel/lineage"
)

func testStamp() lineage.Stamp {
	return lineage.NewSequence("seq-1").WithRun("run-1", 1).WithCall("call-1", 1)
}

func TestEnvelope_RoundTrip(t *testing.T) {
	cases := map[string]Envelope{
		"call":   NewCall(testStamp(), "connectors", "write", json.RawMessage(`{"rows":3}`)),
		"event":  NewEvent(testStamp(), "mtg-db updated", json.RawMessage(`{"new_set":true}`)),
		"return": NewCall(testStamp(), "connectors", "write", nil).Reply(json.RawMessage(`{"ok":true}`)),
	}

	for name, env := range cases {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(env)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var got Envelope
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got.Kind != env.Kind {
				t.Errorf("kind: got %q want %q", got.Kind, env.Kind)
			}
			if got.Stamp != env.Stamp {
				t.Errorf("stamp not preserved\n got: %+v\nwant: %+v", got.Stamp, env.Stamp)
			}
			if string(got.Payload) != string(env.Payload) {
				t.Errorf("payload: got %s want %s", got.Payload, env.Payload)
			}
		})
	}
}

func TestEnvelope_SequenceIDPreserved(t *testing.T) {
	env := NewEvent(testStamp(), "topic", nil)
	raw, _ := json.Marshal(env)
	var got Envelope
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Stamp.Sequence != "seq-1" {
		t.Fatalf("sequence id lost in round-trip: %q", got.Stamp.Sequence)
	}
}

func TestEnvelope_Reply(t *testing.T) {
	call := NewCall(testStamp(), "ai", "infer", nil)

	ok := call.Reply(json.RawMessage(`"done"`))
	if ok.Kind != KindReturn || ok.Stamp != call.Stamp || ok.Error != "" {
		t.Errorf("Reply: unexpected %+v", ok)
	}

	bad := call.ReplyError(errors.New("boom"))
	if bad.Kind != KindReturn || bad.Stamp != call.Stamp || bad.Error != "boom" {
		t.Errorf("ReplyError: unexpected %+v", bad)
	}
}

func TestEnvelope_Validate(t *testing.T) {
	valid := map[string]Envelope{
		"call":   NewCall(testStamp(), "connectors", "write", nil),
		"event":  NewEvent(testStamp(), "topic", nil),
		"return": NewCall(testStamp(), "ai", "infer", nil).Reply(nil),
	}
	for name, env := range valid {
		t.Run("valid/"+name, func(t *testing.T) {
			if err := env.Validate(); err != nil {
				t.Errorf("expected valid, got %v", err)
			}
		})
	}

	invalid := map[string]Envelope{
		"missing sequence": {Kind: KindEvent, Topic: "t"},
		"call no tool":     {Kind: KindCall, Stamp: testStamp(), Operation: "write"},
		"event no topic":   {Kind: KindEvent, Stamp: testStamp()},
		"unknown kind":     {Kind: "bogus", Stamp: testStamp()},
	}
	for name, env := range invalid {
		t.Run("invalid/"+name, func(t *testing.T) {
			if err := env.Validate(); err == nil {
				t.Errorf("expected error, got nil")
			}
		})
	}
}
