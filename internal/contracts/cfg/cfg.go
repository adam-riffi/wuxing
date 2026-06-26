// Package cfg defines the cfg schema contract — the structural grammar of a
// service's declarative config. "Config is the program": the cfg format is the
// language the interpreter reads. The grammar is derived from the tool function
// vocabulary, so Validate checks each workflow step's tool.operation against a
// Vocabulary (see docs/vocabulary/).
package cfg

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Service is a parsed service cfg — its ID card.
type Service struct {
	Name       string      `yaml:"name"`
	Version    string      `yaml:"version"`
	Envelope   Envelope    `yaml:"envelope"`
	Allow      []Grant     `yaml:"allow"`
	Triggers   []Trigger   `yaml:"triggers"`
	Workflow   []Step      `yaml:"workflow"`
	Successors []Successor `yaml:"successors"`
}

// Envelope is the service's resource/scheduling profile.
type Envelope struct {
	Request   int64  `yaml:"request"`    // memory floor (bytes) for admission
	Limit     int64  `yaml:"limit"`      // memory burst ceiling
	AIRequest int64  `yaml:"ai_request"` // AI-quota window cost (0 = non-AI)
	Priority  string `yaml:"priority"`   // "user" | "background" | ""
	MaxWait   string `yaml:"max_wait"`   // queue patience, a duration string
}

// Grant is an entry in the capability allowlist: a tool operation, optionally
// scoped to a target (e.g. a connectors write to DATABASE.TABLE).
type Grant struct {
	Tool      string `yaml:"tool"`
	Operation string `yaml:"operation"`
	Target    string `yaml:"target"`
}

// Trigger declares an external initiation (cron or event).
type Trigger struct {
	Kind string `yaml:"kind"` // "cron" | "event"
	Spec string `yaml:"spec"` // cron expression or event topic
}

// Step is one node of the service's internal workflow — a tool.operation call,
// optionally branching on its emitted value.
type Step struct {
	ID        string   `yaml:"id"`
	Tool      string   `yaml:"tool"`
	Operation string   `yaml:"operation"`
	Next      string   `yaml:"next"`   // unconditional successor step id
	Branch    []Branch `yaml:"branch"` // conditional routing on the step's output
}

// Branch routes to a step when a condition on the previous step's output holds.
type Branch struct {
	When string `yaml:"when"` // condition, e.g. "new_set == true"
	Goto string `yaml:"goto"` // target step id
}

// Successor declares what runs after this service emits an event (the only
// inter-service coupling — indirect, one-way).
type Successor struct {
	Service string `yaml:"service"`
	Topic   string `yaml:"topic"`
	When    string `yaml:"when"`
}

// Parse unmarshals a service cfg from YAML.
func Parse(data []byte) (*Service, error) {
	var s Service
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("cfg: parse: %w", err)
	}
	return &s, nil
}

// Validate checks the cfg's structure and that every workflow step calls a
// tool.operation the vocabulary knows.
func (s *Service) Validate(vocab Vocabulary) error {
	if s.Name == "" {
		return fmt.Errorf("cfg: service has no name")
	}
	if s.Envelope.Request <= 0 {
		return fmt.Errorf("cfg: %q: envelope.request must be positive", s.Name)
	}
	switch s.Envelope.Priority {
	case "", "user", "background":
	default:
		return fmt.Errorf("cfg: %q: invalid priority %q", s.Name, s.Envelope.Priority)
	}
	if s.Envelope.MaxWait != "" {
		if _, err := time.ParseDuration(s.Envelope.MaxWait); err != nil {
			return fmt.Errorf("cfg: %q: invalid max_wait %q: %w", s.Name, s.Envelope.MaxWait, err)
		}
	}

	ids := make(map[string]bool, len(s.Workflow))
	for i, step := range s.Workflow {
		if step.ID == "" {
			return fmt.Errorf("cfg: %q: workflow step %d has no id", s.Name, i)
		}
		if ids[step.ID] {
			return fmt.Errorf("cfg: %q: duplicate step id %q", s.Name, step.ID)
		}
		ids[step.ID] = true
		if !vocab.Known(step.Tool, step.Operation) {
			return fmt.Errorf("cfg: %q: step %q calls unknown %s.%s", s.Name, step.ID, step.Tool, step.Operation)
		}
	}

	// Branch/next targets must reference real steps.
	for _, step := range s.Workflow {
		if step.Next != "" && !ids[step.Next] {
			return fmt.Errorf("cfg: %q: step %q next %q is not a step", s.Name, step.ID, step.Next)
		}
		for _, b := range step.Branch {
			if !ids[b.Goto] {
				return fmt.Errorf("cfg: %q: step %q branch goto %q is not a step", s.Name, step.ID, b.Goto)
			}
		}
	}
	return nil
}
