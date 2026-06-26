package cfg

import "testing"

const mtgCfg = `
name: mtg
version: "1"
envelope:
  request: 268435456
  limit: 536870912
  priority: background
  max_wait: 5m
allow:
  - tool: connectors
    operation: write
    target: mtg.cards
triggers:
  - kind: cron
    spec: "0 * * * *"
workflow:
  - id: check
    tool: connectors
    operation: query
    branch:
      - when: "new_set == true"
        goto: write
  - id: write
    tool: connectors
    operation: write
successors:
  - service: notifier
    topic: "mtg-db updated"
    when: "new_set == true"
`

func TestParse_MTG(t *testing.T) {
	s, err := Parse([]byte(mtgCfg))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if s.Name != "mtg" {
		t.Errorf("name: got %q", s.Name)
	}
	if s.Envelope.Request != 268435456 || s.Envelope.MaxWait != "5m" {
		t.Errorf("envelope: %+v", s.Envelope)
	}
	if len(s.Workflow) != 2 || s.Workflow[0].Branch[0].Goto != "write" {
		t.Errorf("workflow: %+v", s.Workflow)
	}
	if len(s.Successors) != 1 || s.Successors[0].Service != "notifier" {
		t.Errorf("successors: %+v", s.Successors)
	}
}

func TestValidate_Valid(t *testing.T) {
	s, _ := Parse([]byte(mtgCfg))
	if err := s.Validate(DefaultVocabulary()); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestValidate_UnknownToolOperation(t *testing.T) {
	s, _ := Parse([]byte(mtgCfg))
	s.Workflow[1].Operation = "delete" // connectors has no "delete"
	if err := s.Validate(DefaultVocabulary()); err == nil {
		t.Fatal("expected error for unknown tool.operation")
	}
}

func TestValidate_Errors(t *testing.T) {
	base := func() *Service { s, _ := Parse([]byte(mtgCfg)); return s }

	cases := map[string]func(*Service){
		"no name":         func(s *Service) { s.Name = "" },
		"zero request":    func(s *Service) { s.Envelope.Request = 0 },
		"bad priority":    func(s *Service) { s.Envelope.Priority = "urgent" },
		"bad max_wait":    func(s *Service) { s.Envelope.MaxWait = "soon" },
		"duplicate step":  func(s *Service) { s.Workflow[1].ID = "check" },
		"branch dangling": func(s *Service) { s.Workflow[0].Branch[0].Goto = "nowhere" },
		"next dangling":   func(s *Service) { s.Workflow[0].Next = "nowhere" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s := base()
			mutate(s)
			if err := s.Validate(DefaultVocabulary()); err == nil {
				t.Errorf("expected error for %s", name)
			}
		})
	}
}

func TestVocabulary_Known(t *testing.T) {
	v := DefaultVocabulary()
	if !v.Known("connectors", "write") {
		t.Error("connectors.write should be known")
	}
	if v.Known("connectors", "drop") {
		t.Error("connectors.drop should be unknown")
	}
	if v.Known("library", "register") {
		t.Error("library is not a service-callable workflow verb")
	}
}
