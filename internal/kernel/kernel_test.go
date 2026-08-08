package kernel

import (
	"testing"

	"github.com/adam-riffi/wuxing/internal/contracts/cfg"
	"github.com/adam-riffi/wuxing/internal/kernel/lineage"
	"github.com/adam-riffi/wuxing/internal/kernel/sessions"
)

func TestKernel_RegisterDeregister(t *testing.T) {
	k := testKernel(t)

	svc := &cfg.Service{Name: "svc", Envelope: cfg.Envelope{Request: 1}}
	if err := k.Register(svc); err != nil {
		t.Fatalf("Register: %v", err)
	}
	names := k.Library.List()
	if len(names) != 1 || names[0] != "svc" {
		t.Fatalf("after Register: want [svc], got %v", names)
	}

	if err := k.Deregister("svc"); err != nil {
		t.Fatalf("Deregister: %v", err)
	}
	if names := k.Library.List(); len(names) != 0 {
		t.Errorf("after Deregister: want empty list, got %v", names)
	}
}

func TestKernel_Deregister_Drain(t *testing.T) {
	k := testKernel(t)

	svc := &cfg.Service{Name: "svc", Envelope: cfg.Envelope{Request: 1}}
	if err := k.Register(svc); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := k.Sessions.Open(sessions.Session{
		ID:      lineage.SessionID("s-1"),
		Run:     lineage.RunID("r-1"),
		Service: "svc",
	}); err != nil {
		t.Fatalf("Sessions.Open: %v", err)
	}

	err := k.Deregister("svc")
	if err == nil {
		t.Fatal("Deregister with running session: want error, got nil")
	}
}

func TestKernel_Deregister_Unknown(t *testing.T) {
	k := testKernel(t)

	if err := k.Deregister("ghost"); err == nil {
		t.Error("Deregister of unknown service: want error, got nil")
	}
}

func TestKernel_Deregister_RemovesTriggerRules(t *testing.T) {
	k := testKernel(t)

	svc := &cfg.Service{
		Name:       "svc",
		Envelope:   cfg.Envelope{Request: 1},
		Triggers:   []cfg.Trigger{{Kind: "cron", Spec: "@hourly"}},
		Successors: []cfg.Successor{{Service: "other", Topic: "svc-done", When: "true"}},
	}
	if err := k.Register(svc); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := k.Deregister("svc"); err != nil {
		t.Fatalf("Deregister: %v", err)
	}

	// After deregister the cron rule must be gone from the library list.
	if names := k.Library.List(); len(names) != 0 {
		t.Errorf("library must be empty after deregister, got %v", names)
	}
}
