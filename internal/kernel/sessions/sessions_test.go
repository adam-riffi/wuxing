package sessions

import (
	"strconv"
	"sync"
	"testing"

	"github.com/adam-riffi/wuxing/internal/kernel/lineage"
)

func TestRegistry_OpenClose(t *testing.T) {
	r := New()
	if err := r.Open(Session{ID: "s1", Run: "r1", Service: "mtg"}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if r.Count() != 1 {
		t.Errorf("count: got %d want 1", r.Count())
	}

	got, ok := r.Close("s1")
	if !ok || got.ID != "s1" {
		t.Errorf("Close: got %+v, %v", got, ok)
	}
	if r.Count() != 0 {
		t.Errorf("count after close: got %d want 0", r.Count())
	}
}

func TestRegistry_OpenErrors(t *testing.T) {
	r := New()
	if err := r.Open(Session{Service: "mtg"}); err == nil {
		t.Error("expected error for missing id")
	}
	if err := r.Open(Session{ID: "s1"}); err == nil {
		t.Error("expected error for missing service")
	}
	if err := r.Open(Session{ID: "s1", Service: "mtg"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Open(Session{ID: "s1", Service: "mtg"}); err == nil {
		t.Error("expected error for duplicate open")
	}
}

func TestRegistry_CloseUnknown(t *testing.T) {
	r := New()
	if _, ok := r.Close("ghost"); ok {
		t.Error("closing an unknown session should report ok=false")
	}
}

func TestRegistry_HasRunningDrainCheck(t *testing.T) {
	r := New()
	_ = r.Open(Session{ID: "a1", Service: "mtg"})
	_ = r.Open(Session{ID: "a2", Service: "mtg"})
	_ = r.Open(Session{ID: "b1", Service: "notifier"})

	if !r.HasRunning("mtg") {
		t.Error("HasRunning(mtg): want true")
	}
	if !r.HasRunning("notifier") {
		t.Error("HasRunning(notifier): want true")
	}
	if r.HasRunning("absent") {
		t.Error("HasRunning(absent): want false")
	}

	// Draining mtg: only when every mtg session closes is it drained.
	r.Close("a1")
	if !r.HasRunning("mtg") {
		t.Error("mtg still has a2 open: want true")
	}
	r.Close("a2")
	if r.HasRunning("mtg") {
		t.Error("mtg fully drained: want false")
	}
}

func TestRegistry_RunningSnapshotIsolated(t *testing.T) {
	r := New()
	_ = r.Open(Session{ID: "s1", Service: "mtg"})

	snap := r.Running()
	if len(snap) != 1 {
		t.Fatalf("snapshot len: got %d want 1", len(snap))
	}
	// Mutating the snapshot must not affect the registry.
	snap[0].Service = "tampered"
	if r.Running()[0].Service != "mtg" {
		t.Error("registry state leaked through the Running snapshot")
	}
}

func TestRegistry_Concurrent(t *testing.T) {
	r := New()
	const n = 200
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := lineage.SessionID("s" + strconv.Itoa(i))
			_ = r.Open(Session{ID: id, Service: "svc"})
			_ = r.HasRunning("svc")
			r.Close(id)
		}(i)
	}
	wg.Wait()
	if r.Count() != 0 {
		t.Errorf("count after concurrent open/close: got %d want 0", r.Count())
	}
}
