package lineage

import (
	"fmt"
	"sync"
	"testing"
)

func TestMinter_UniqueIDs(t *testing.T) {
	m := NewMinter()
	const n = 1000
	seen := make(map[string]struct{}, n)
	for range n {
		id := string(m.SequenceID())
		if _, dup := seen[id]; dup {
			t.Fatalf("minted a duplicate id: %q", id)
		}
		seen[id] = struct{}{}
	}
}

func TestMinter_ConcurrentUnique(t *testing.T) {
	m := NewMinter()
	const goroutines, per = 16, 200

	var mu sync.Mutex
	seen := make(map[string]struct{}, goroutines*per)
	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range per {
				id := string(m.CallID())
				mu.Lock()
				seen[id] = struct{}{}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if got := len(seen); got != goroutines*per {
		t.Fatalf("expected %d unique ids, got %d (collision)", goroutines*per, got)
	}
}

func TestMinter_Injectable(t *testing.T) {
	var i int
	m := NewMinterFunc(func() string { i++; return fmt.Sprintf("id-%d", i) })

	if got := m.SequenceID(); got != "id-1" {
		t.Errorf("SequenceID: got %q want id-1", got)
	}
	if got := m.RunID(); got != "id-2" {
		t.Errorf("RunID: got %q want id-2", got)
	}
}

func TestStamp_Derivation(t *testing.T) {
	s := NewSequence("seq").
		WithRun("run", 1).
		WithSession("sess", 2).
		WithCall("call", 3).
		WithStep("step", 4)

	want := Stamp{
		Sequence: "seq", Run: "run", Session: "sess", Call: "call", ToolFunction: "step",
		SequenceOrder: 1, RunOrder: 2, CallOrder: 3, StepOrder: 4,
	}
	if s != want {
		t.Fatalf("derived stamp mismatch\n got: %+v\nwant: %+v", s, want)
	}
}

func TestStamp_DerivationResetsDeeperLevels(t *testing.T) {
	// A fully-populated stamp...
	deep := NewSequence("seq").
		WithRun("run1", 1).
		WithSession("sess1", 9).
		WithCall("call1", 9).
		WithStep("step1", 9)

	// ...deriving a new run must clear session/call/tool_function and their orders.
	got := deep.WithRun("run2", 2)
	want := Stamp{Sequence: "seq", Run: "run2", SequenceOrder: 2}
	if got != want {
		t.Fatalf("WithRun did not reset deeper levels\n got: %+v\nwant: %+v", got, want)
	}
}

func TestStamp_OriginalUnchanged(t *testing.T) {
	// Stamp is a value type: deriving a child must not mutate the parent.
	parent := NewSequence("seq").WithRun("run", 1)
	_ = parent.WithSession("sess", 2)
	if parent.Session != "" {
		t.Fatalf("deriving a child mutated the parent: %+v", parent)
	}
}
