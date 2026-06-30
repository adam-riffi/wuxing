package scheduler

import "testing"

func TestSnapshot_ResourcesAndQueue(t *testing.T) {
	// Capacity fits one 60-unit job; a second 60-unit job must queue.
	s := New(100, nil)
	if err := s.Submit(Job{ID: "a", Request: 60}); err != nil {
		t.Fatal(err)
	}
	if err := s.Submit(Job{ID: "b", Request: 60}); err != nil {
		t.Fatal(err)
	}

	snap := s.Snapshot()
	if snap.MemoryCapacity != 100 || snap.MemoryUsed != 60 || snap.MemoryFree != 40 {
		t.Errorf("memory: %+v", snap)
	}
	if len(snap.RunningIDs) != 1 || snap.RunningIDs[0] != "a" {
		t.Errorf("running: %+v", snap.RunningIDs)
	}
	if len(snap.QueuedIDs) != 1 || snap.QueuedIDs[0] != "b" {
		t.Errorf("queued: %+v", snap.QueuedIDs)
	}

	// Completing the running job frees memory and admits the queued one.
	s.Complete("a")
	snap = s.Snapshot()
	if snap.MemoryUsed != 60 || len(snap.QueuedIDs) != 0 {
		t.Errorf("after complete: %+v", snap)
	}
	if len(snap.RunningIDs) != 1 || snap.RunningIDs[0] != "b" {
		t.Errorf("after complete running: %+v", snap.RunningIDs)
	}
}
