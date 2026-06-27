package library

import (
	"reflect"
	"testing"

	"github.com/adam-riffi/wuxing/internal/contracts/cfg"
)

func mtg() *cfg.Service {
	return &cfg.Service{
		Name:       "mtg",
		Triggers:   []cfg.Trigger{{Kind: "cron", Spec: "0 * * * *"}},
		Successors: []cfg.Successor{{Service: "notifier", Topic: "mtg-db updated", When: "new_set == true"}},
	}
}

func TestCatalog_RegisterAndGet(t *testing.T) {
	c := New()
	if err := c.Register(mtg()); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, err := c.GetDefinition("mtg")
	if err != nil || got.Name != "mtg" {
		t.Fatalf("GetDefinition: %+v, %v", got, err)
	}
}

func TestCatalog_RegisterErrors(t *testing.T) {
	c := New()
	if err := c.Register(&cfg.Service{}); err == nil {
		t.Error("expected error registering a nameless service")
	}
	_ = c.Register(mtg())
	if err := c.Register(mtg()); err == nil {
		t.Error("expected error on duplicate registration")
	}
}

func TestCatalog_GetUnknown(t *testing.T) {
	c := New()
	if _, err := c.GetDefinition("ghost"); err == nil {
		t.Error("expected error for an unregistered service")
	}
}

func TestCatalog_ServeDeclarations(t *testing.T) {
	c := New()
	_ = c.Register(mtg())

	succ, err := c.GetSuccessors("mtg")
	if err != nil || len(succ) != 1 || succ[0].Service != "notifier" {
		t.Errorf("GetSuccessors: %+v, %v", succ, err)
	}
	trig, err := c.GetTriggers("mtg")
	if err != nil || len(trig) != 1 || trig[0].Kind != "cron" {
		t.Errorf("GetTriggers: %+v, %v", trig, err)
	}
}

func TestCatalog_ListAndDeregister(t *testing.T) {
	c := New()
	_ = c.Register(mtg())
	_ = c.Register(&cfg.Service{Name: "notifier"})

	if got := c.List(); len(got) != 2 {
		t.Errorf("List: got %v want 2 entries", got)
	}
	if err := c.Deregister("mtg"); err != nil {
		t.Fatalf("Deregister: %v", err)
	}
	if got := c.List(); !reflect.DeepEqual(got, []string{"notifier"}) {
		t.Errorf("List after deregister: got %v want [notifier]", got)
	}
	if err := c.Deregister("mtg"); err == nil {
		t.Error("deregistering an unknown service should error")
	}
}

func TestCatalog_Diff(t *testing.T) {
	c := New()
	_ = c.Register(mtg())

	same, err := c.Diff("mtg", mtg())
	if err != nil || same {
		t.Errorf("Diff against identical: got drift=%v, %v", same, err)
	}

	changed := mtg()
	changed.Version = "2"
	drift, err := c.Diff("mtg", changed)
	if err != nil || !drift {
		t.Errorf("Diff against changed: got drift=%v, %v", drift, err)
	}

	if _, err := c.Diff("ghost", mtg()); err == nil {
		t.Error("Diff on an unregistered service should error")
	}
}
