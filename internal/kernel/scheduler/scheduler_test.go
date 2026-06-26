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

func TestScheduler_NeverPartialHeadOfLine(t *testing.T) {
	s := New(10, nil)
	_ = s.Submit(Job{ID: "a", Request: 8}) // admitted, free=2
	_ = s.Submit(Job{ID: "b", Request: 5}) // queued, does not fit
	_ = s.Submit(Job{ID: "c", Request: 2}) // fits free, but blocked behind b

	if got := s.Running(); !reflect.DeepEqual(got, []string{"a"}) {
		t.Errorf("running: got %v want [a] (c must not jump the head)", got)
	}
	if got := s.Queued(); !reflect.DeepEqual(got, []string{"b", "c"}) {
		t.Errorf("queued: got %v want [b c]", got)
	}
}

func TestScheduler_CompleteAdmitsQueued(t *testing.T) {
	rec := &recorder{}
	s := New(10, rec.admit)
	_ = s.Submit(Job{ID: "a", Request: 8})
	_ = s.Submit(Job{ID: "b", Request: 5})
	_ = s.Submit(Job{ID: "c", Request: 2})

	s.Complete("a") // frees 8 -> b (5) then c (2) admit

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
