package triggers

import (
	"fmt"
	"testing"

	"github.com/adam-riffi/wuxing/internal/kernel/lineage"
)

// seqMinter returns a minter with deterministic, monotonically numbered ids.
func seqMinter() *lineage.Minter {
	var n int
	return lineage.NewMinterFunc(func() string {
		n++
		return fmt.Sprintf("id-%d", n)
	})
}

func TestTriggers_ExternalOpensNewSequence(t *testing.T) {
	var fired []Run
	tr := New(seqMinter(), func(r Run) { fired = append(fired, r) })

	r := tr.FireExternal("mtg", KindCron)

	if r.Stamp.Sequence == "" {
		t.Fatal("external trigger must open a sequence")
	}
	if r.Stamp.Run == "" {
		t.Fatal("external trigger must mint a run id")
	}
	if r.Stamp.SequenceOrder != 0 {
		t.Errorf("sequence_order: got %d want 0 (chain head)", r.Stamp.SequenceOrder)
	}
	if len(fired) != 1 || fired[0].Service != "mtg" || fired[0].Kind != KindCron {
		t.Errorf("onFire: got %+v", fired)
	}
}

func TestTriggers_ExternalSequencesAreDistinct(t *testing.T) {
	tr := New(seqMinter(), nil)
	a := tr.FireExternal("mtg", KindCron)
	b := tr.FireExternal("mtg", KindCron)
	if a.Stamp.Sequence == b.Stamp.Sequence {
		t.Errorf("each external trigger must open a distinct sequence: %q == %q",
			a.Stamp.Sequence, b.Stamp.Sequence)
	}
}

func TestTriggers_EventInheritsSequence(t *testing.T) {
	tr := New(seqMinter(), nil)
	tr.RegisterEvent("notifier", "mtg-db updated")

	// An event emitted by run "mtg" at sequence_order 0 within sequence "seq-1".
	cause := lineage.NewSequence("seq-1").WithRun("mtg-run", 0)

	runs := tr.OnEvent("mtg-db updated", cause)
	if len(runs) != 1 {
		t.Fatalf("got %d runs want 1", len(runs))
	}
	got := runs[0]
	if got.Service != "notifier" || got.Kind != KindEvent {
		t.Errorf("run: got %+v", got)
	}
	if got.Stamp.Sequence != "seq-1" {
		t.Errorf("sequence not inherited: got %q want seq-1", got.Stamp.Sequence)
	}
	if got.Stamp.SequenceOrder != 1 {
		t.Errorf("sequence_order: got %d want 1 (successor of order 0)", got.Stamp.SequenceOrder)
	}
	if got.Stamp.Run == "" || got.Stamp.Run == "mtg-run" {
		t.Errorf("successor must get a fresh run id, got %q", got.Stamp.Run)
	}
}

func TestTriggers_EventNoSubscribers(t *testing.T) {
	tr := New(seqMinter(), nil)
	cause := lineage.NewSequence("seq-1").WithRun("r", 0)
	if runs := tr.OnEvent("nobody-listening", cause); len(runs) != 0 {
		t.Errorf("got %d runs want 0 (emit blind)", len(runs))
	}
}

func TestTriggers_MultipleSuccessorsShareSequence(t *testing.T) {
	tr := New(seqMinter(), nil)
	tr.RegisterEvent("notifier", "mtg-db updated")
	tr.RegisterEvent("archiver", "mtg-db updated")

	cause := lineage.NewSequence("seq-9").WithRun("mtg-run", 2)
	runs := tr.OnEvent("mtg-db updated", cause)
	if len(runs) != 2 {
		t.Fatalf("got %d runs want 2", len(runs))
	}
	for _, r := range runs {
		if r.Stamp.Sequence != "seq-9" {
			t.Errorf("%s: sequence not inherited: %q", r.Service, r.Stamp.Sequence)
		}
		if r.Stamp.SequenceOrder != 3 {
			t.Errorf("%s: sequence_order got %d want 3", r.Service, r.Stamp.SequenceOrder)
		}
	}
	if runs[0].Stamp.Run == runs[1].Stamp.Run {
		t.Error("each successor must get a distinct run id")
	}
}

func TestTriggers_UnregisterService_RemovesCronAndEvent(t *testing.T) {
	tr := New(seqMinter(), nil)
	tr.RegisterCron("svc", "@hourly")
	tr.RegisterEvent("svc", "some-topic")
	tr.RegisterEvent("other", "some-topic") // must survive

	tr.UnregisterService("svc")

	tr.mu.RLock()
	defer tr.mu.RUnlock()
	if _, ok := tr.cron["svc"]; ok {
		t.Error("cron rule for svc must be removed after UnregisterService")
	}
	for _, svcs := range tr.event {
		for _, s := range svcs {
			if s == "svc" {
				t.Errorf("event rule listing svc must be removed: still in %v", tr.event)
			}
		}
	}
	if svcs, ok := tr.event["some-topic"]; !ok {
		t.Error("other service's event rule must survive")
	} else if len(svcs) != 1 || svcs[0] != "other" {
		t.Errorf("wrong survivors: %v", svcs)
	}
}

func TestTriggers_UnregisterService_UnknownIsNoOp(t *testing.T) {
	tr := New(seqMinter(), nil)
	tr.RegisterCron("svc", "@hourly")

	tr.UnregisterService("ghost") // must not panic

	tr.mu.RLock()
	defer tr.mu.RUnlock()
	if _, ok := tr.cron["svc"]; !ok {
		t.Error("existing cron rule must survive unregistering an unknown service")
	}
}

func TestTriggers_UnregisterService_EmptyTopicRemoved(t *testing.T) {
	tr := New(seqMinter(), nil)
	tr.RegisterEvent("svc", "solo-topic") // svc is the only subscriber

	tr.UnregisterService("svc")

	tr.mu.RLock()
	defer tr.mu.RUnlock()
	if _, ok := tr.event["solo-topic"]; ok {
		t.Error("topic with no subscribers must be removed from event map")
	}
}

func TestTriggers_UnregisterService_NoFireAfterUnregister(t *testing.T) {
	var fired []Run
	tr := New(seqMinter(), func(r Run) { fired = append(fired, r) })
	tr.RegisterEvent("svc", "topic")

	tr.UnregisterService("svc")

	cause := lineage.NewSequence("seq-1").WithRun("r", 0)
	runs := tr.OnEvent("topic", cause)
	if len(runs) != 0 {
		t.Errorf("deregistered service must not fire: got %d runs", len(runs))
	}
	if len(fired) != 0 {
		t.Errorf("onFire must not be called for deregistered service: got %v", fired)
	}
}
