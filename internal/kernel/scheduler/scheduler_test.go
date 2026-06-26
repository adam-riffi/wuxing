package scheduler

import (
	"reflect"
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

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

func TestScheduler_EscalateOnExpiry(t *testing.T) {
	clk := &fakeClock{t: time.Unix(0, 0)}
	rec := &recorder{}
	s := New(10, rec.admit, WithClock(clk.now))
	_ = s.Submit(Job{ID: "r", Request: 10}) // fills capacity

	_ = s.Submit(Job{ID: "first", Request: 5})                                                  // background, no patience limit
	_ = s.Submit(Job{ID: "second", Request: 5, MaxWait: time.Minute, OnExpiry: ExpiryEscalate}) // background

	if got := s.Queued(); !reflect.DeepEqual(got, []string{"first", "second"}) {
		t.Fatalf("initial queue: got %v want [first second]", got)
	}

	clk.advance(2 * time.Minute)
	s.Expire() // second has waited too long -> escalates above first

	if got := s.Queued(); !reflect.DeepEqual(got, []string{"second", "first"}) {
		t.Fatalf("queue after escalate: got %v want [second first]", got)
	}

	s.Complete("r")
	if !reflect.DeepEqual(rec.ids, []string{"r", "second", "first"}) {
		t.Errorf("admission order: got %v want [r second first]", rec.ids)
	}
}

func TestScheduler_FailOnExpiry(t *testing.T) {
	clk := &fakeClock{t: time.Unix(0, 0)}
	var expired []string
	s := New(
		10, nil,
		WithClock(clk.now),
		WithOnExpire(func(j Job) { expired = append(expired, j.ID) }),
	)
	_ = s.Submit(Job{ID: "r", Request: 10}) // fills capacity
	_ = s.Submit(Job{ID: "drop", Request: 5, MaxWait: time.Minute, OnExpiry: ExpiryFail})

	clk.advance(90 * time.Second)
	s.Expire()

	if got := s.Queued(); len(got) != 0 {
		t.Errorf("queue: got %v want empty (drop should be failed out)", got)
	}
	if !reflect.DeepEqual(expired, []string{"drop"}) {
		t.Errorf("onExpire: got %v want [drop]", expired)
	}
}

func TestScheduler_NoExpiryBeforeMaxWait(t *testing.T) {
	clk := &fakeClock{t: time.Unix(0, 0)}
	s := New(10, nil, WithClock(clk.now))
	_ = s.Submit(Job{ID: "r", Request: 10})
	_ = s.Submit(Job{ID: "wait", Request: 5, MaxWait: time.Minute, OnExpiry: ExpiryFail})

	clk.advance(30 * time.Second) // not yet expired
	s.Expire()

	if got := s.Queued(); !reflect.DeepEqual(got, []string{"wait"}) {
		t.Errorf("queue: got %v want [wait] (still patient)", got)
	}
}

func TestScheduler_InfinitePatience(t *testing.T) {
	clk := &fakeClock{t: time.Unix(0, 0)}
	s := New(10, nil, WithClock(clk.now))
	_ = s.Submit(Job{ID: "r", Request: 10})
	_ = s.Submit(Job{ID: "patient", Request: 5}) // MaxWait 0 -> never expires

	clk.advance(365 * 24 * time.Hour)
	s.Expire()

	if got := s.Queued(); !reflect.DeepEqual(got, []string{"patient"}) {
		t.Errorf("queue: got %v want [patient] (zero MaxWait never expires)", got)
	}
}

func TestScheduler_ReserveBandIsPrivilegedOnly(t *testing.T) {
	s := New(10, nil, WithReserve(3)) // soft budget 7, reserve band [7,10]

	_ = s.Submit(Job{ID: "a", Request: 7}) // background fills the soft budget; free=3
	_ = s.Submit(Job{ID: "b", Request: 2}) // background may not enter the reserve

	if got := s.Running(); !reflect.DeepEqual(got, []string{"a"}) {
		t.Fatalf("running: got %v want [a]", got)
	}
	if got := s.Queued(); !reflect.DeepEqual(got, []string{"b"}) {
		t.Fatalf("queued: got %v want [b] (background fenced from reserve)", got)
	}

	// A user-class job may draw the reserve band.
	_ = s.Submit(Job{ID: "u", Priority: PriorityUser, Request: 2})
	if got := s.Running(); !reflect.DeepEqual(got, []string{"a", "u"}) {
		t.Errorf("running: got %v want [a u] (user uses reserve)", got)
	}
}

func TestScheduler_EscalatedJobEntersReserve(t *testing.T) {
	clk := &fakeClock{t: time.Unix(0, 0)}
	s := New(10, nil, WithReserve(4), WithClock(clk.now)) // soft budget 6

	// A big background job fits memory (7<=10) but not the soft budget (7>6).
	_ = s.Submit(Job{ID: "big", Request: 7, MaxWait: time.Minute, OnExpiry: ExpiryEscalate})
	if got := s.Queued(); !reflect.DeepEqual(got, []string{"big"}) {
		t.Fatalf("queued: got %v want [big] (over soft budget)", got)
	}

	clk.advance(2 * time.Minute)
	s.Expire() // starving job escalates to user class -> may enter the reserve

	if got := s.Running(); !reflect.DeepEqual(got, []string{"big"}) {
		t.Errorf("running: got %v want [big] (escalation granted reserve access)", got)
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
