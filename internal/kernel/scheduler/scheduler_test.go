package scheduler

import (
	"reflect"
	"testing"
)

// recorder captures admission order through the onAdmit callback.
type recorder struct{ ids []string }

func (r *recorder) admit(j Job) { r.ids = append(r.ids, j.ID) }

func TestScheduler_AdmitsWhenFits(t *testing.T) {
	rec := &recorder{}
	s := New(100, rec.admit)

	if err := s.Submit(Job{ID: "a", Request: 40}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if got := s.Running(); !reflect.DeepEqual(got, []string{"a"}) {
		t.Errorf("running: got %v want [a]", got)
	}
	if s.Free() != 60 {
		t.Errorf("free: got %d want 60", s.Free())
	}
	if !reflect.DeepEqual(rec.ids, []string{"a"}) {
		t.Errorf("admitted: got %v want [a]", rec.ids)
	}
}

func TestScheduler_QueuesWhenFull(t *testing.T) {
	s := New(100, nil)
	_ = s.Submit(Job{ID: "a", Request: 80})
	_ = s.Submit(Job{ID: "b", Request: 40}) // does not fit

	if got := s.Running(); !reflect.DeepEqual(got, []string{"a"}) {
		t.Errorf("running: got %v want [a]", got)
	}
	if got := s.Queued(); !reflect.DeepEqual(got, []string{"b"}) {
		t.Errorf("queued: got %v want [b]", got)
	}
}

func TestScheduler_NeverPartial(t *testing.T) {
	s := New(10, nil)
	_ = s.Submit(Job{ID: "a", Request: 8}) // admitted, free=2
	_ = s.Submit(Job{ID: "b", Request: 5}) // does not fit in 2; never partial

	if got := s.Running(); !reflect.DeepEqual(got, []string{"a"}) {
		t.Errorf("running: got %v want [a]", got)
	}
	if got := s.Queued(); !reflect.DeepEqual(got, []string{"b"}) {
		t.Errorf("queued: got %v want [b]", got)
	}
}

func TestScheduler_BackfillsGapTooSmallForHead(t *testing.T) {
	s := New(10, nil)
	_ = s.Submit(Job{ID: "a", Request: 8}) // running, free=2
	_ = s.Submit(Job{ID: "b", Request: 5}) // head, blocked (5 > 2)
	_ = s.Submit(Job{ID: "c", Request: 2}) // backfills the gap the head can't use

	if got := s.Running(); !reflect.DeepEqual(got, []string{"a", "c"}) {
		t.Errorf("running: got %v want [a c] (c should backfill)", got)
	}
	if got := s.Queued(); !reflect.DeepEqual(got, []string{"b"}) {
		t.Errorf("queued: got %v want [b] (head stays queued)", got)
	}
}

func TestScheduler_HeadKeepsClaimOnCompletion(t *testing.T) {
	rec := &recorder{}
	s := New(10, rec.admit)
	_ = s.Submit(Job{ID: "a", Request: 8}) // running, free=2
	_ = s.Submit(Job{ID: "b", Request: 5}) // head, blocked
	_ = s.Submit(Job{ID: "c", Request: 2}) // backfilled, free=0

	s.Complete("a") // frees 8 -> the head b is tried first and admitted

	if got := s.Running(); !reflect.DeepEqual(got, []string{"b", "c"}) {
		t.Errorf("running: got %v want [b c]", got)
	}
	if !reflect.DeepEqual(rec.ids, []string{"a", "c", "b"}) {
		t.Errorf("admission order: got %v want [a c b]", rec.ids)
	}
}

func TestScheduler_CompleteAdmitsQueued(t *testing.T) {
	rec := &recorder{}
	s := New(10, rec.admit)
	_ = s.Submit(Job{ID: "a", Request: 10}) // fills capacity, free=0
	_ = s.Submit(Job{ID: "b", Request: 5})  // queued
	_ = s.Submit(Job{ID: "c", Request: 2})  // queued (nothing to backfill into)

	s.Complete("a") // frees 10 -> b (5) then c (2) admit

	if got := s.Running(); !reflect.DeepEqual(got, []string{"b", "c"}) {
		t.Errorf("running: got %v want [b c]", got)
	}
	if got := s.Queued(); len(got) != 0 {
		t.Errorf("queued: got %v want empty", got)
	}
	if !reflect.DeepEqual(rec.ids, []string{"a", "b", "c"}) {
		t.Errorf("admission order: got %v want [a b c]", rec.ids)
	}
}

func TestScheduler_PriorityBeforeFCFS(t *testing.T) {
	rec := &recorder{}
	s := New(10, rec.admit)
	_ = s.Submit(Job{ID: "r", Request: 10}) // fills capacity

	_ = s.Submit(Job{ID: "bg", Priority: PriorityBackground, Request: 5})
	_ = s.Submit(Job{ID: "user", Priority: PriorityUser, Request: 5})

	// user outranks bg even though it was submitted later.
	if got := s.Queued(); !reflect.DeepEqual(got, []string{"user", "bg"}) {
		t.Errorf("queued order: got %v want [user bg]", got)
	}

	s.Complete("r")
	if !reflect.DeepEqual(rec.ids, []string{"r", "user", "bg"}) {
		t.Errorf("admission order: got %v want [r user bg]", rec.ids)
	}
}

func TestScheduler_FCFSWithinClass(t *testing.T) {
	s := New(10, nil)
	_ = s.Submit(Job{ID: "r", Request: 10})
	_ = s.Submit(Job{ID: "first", Request: 5})
	_ = s.Submit(Job{ID: "second", Request: 5})

	if got := s.Queued(); !reflect.DeepEqual(got, []string{"first", "second"}) {
		t.Errorf("queued order: got %v want [first second]", got)
	}
}

func TestScheduler_AIJobNeedsBothResources(t *testing.T) {
	s := New(100, nil, WithAIWindow(10))

	// Fits memory and window.
	if err := s.Submit(Job{ID: "a", Request: 50, AIRequest: 6}); err != nil {
		t.Fatal(err)
	}
	if got := s.Running(); !reflect.DeepEqual(got, []string{"a"}) {
		t.Fatalf("running: got %v want [a]", got)
	}
	if s.Window() != 4 {
		t.Errorf("window: got %d want 4", s.Window())
	}

	// Memory is free, but the window (4) is too small for AIRequest 6: queued.
	_ = s.Submit(Job{ID: "b", Request: 10, AIRequest: 6})
	if got := s.Queued(); !reflect.DeepEqual(got, []string{"b"}) {
		t.Errorf("queued on window exhaustion: got %v want [b]", got)
	}
}

func TestScheduler_NonAIJobIgnoresWindow(t *testing.T) {
	s := New(100, nil) // no AI window configured (window == 0)
	if err := s.Submit(Job{ID: "a", Request: 50}); err != nil {
		t.Fatal(err)
	}
	if got := s.Running(); !reflect.DeepEqual(got, []string{"a"}) {
		t.Errorf("non-AI job should ignore the window: running %v", got)
	}
}

func TestScheduler_RefillAdmitsWaitingAIJob(t *testing.T) {
	rec := &recorder{}
	s := New(100, rec.admit, WithAIWindow(10))
	_ = s.Submit(Job{ID: "a", Request: 10, AIRequest: 10}) // drains window to 0
	_ = s.Submit(Job{ID: "b", Request: 10, AIRequest: 5})  // queued: no window

	if got := s.Queued(); !reflect.DeepEqual(got, []string{"b"}) {
		t.Fatalf("queued: got %v want [b]", got)
	}

	s.RefillWindow(5) // clock refill -> b now fits
	if got := s.Running(); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("running after refill: got %v want [a b]", got)
	}
	if !reflect.DeepEqual(rec.ids, []string{"a", "b"}) {
		t.Errorf("admission order: got %v want [a b]", rec.ids)
	}
}

func TestScheduler_WindowNotFreedOnComplete(t *testing.T) {
	s := New(100, nil, WithAIWindow(10))
	_ = s.Submit(Job{ID: "a", Request: 10, AIRequest: 7})
	if s.Window() != 3 {
		t.Fatalf("window after admit: got %d want 3", s.Window())
	}
	s.Complete("a")
	// Memory returns, the window does not (it refills on a clock).
	if s.Window() != 3 {
		t.Errorf("window after complete: got %d want 3 (must not be freed)", s.Window())
	}
	if s.Free() != 100 {
		t.Errorf("memory after complete: got %d want 100", s.Free())
	}
}

func TestScheduler_RefillCapsAtWindowCapacity(t *testing.T) {
	s := New(100, nil, WithAIWindow(10))
	s.RefillWindow(50) // way over capacity
	if s.Window() != 10 {
		t.Errorf("window: got %d want 10 (capped)", s.Window())
	}
}

func TestScheduler_Rejects(t *testing.T) {
	s := New(100, nil)
	cases := map[string]Job{
		"no id":        {Request: 10},
		"zero request": {ID: "z", Request: 0},
		"exceeds cap":  {ID: "big", Request: 200},
	}
	for name, job := range cases {
		t.Run(name, func(t *testing.T) {
			if err := s.Submit(job); err == nil {
				t.Errorf("expected error for %s", name)
			}
		})
	}

	_ = s.Submit(Job{ID: "dup", Request: 10})
	if err := s.Submit(Job{ID: "dup", Request: 10}); err == nil {
		t.Error("expected error submitting a duplicate running id")
	}
}
