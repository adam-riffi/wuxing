// Package bus is the in-process message channel every tool and service talks
// over: synchronous calls (request/return) and asynchronous events. Every
// message is an Envelope, stamped with its lineage so a whole cascade is
// traceable across hops.
package bus

import (
	"encoding/json"
	"fmt"

	"github.com/adam-riffi/wuxing/internal/kernel/lineage"
)

// Kind distinguishes the three message shapes carried on the bus.
type Kind string

const (
	// KindCall is a synchronous request routed to a tool's operation.
	KindCall Kind = "call"
	// KindReturn is the response to a call.
	KindReturn Kind = "return"
	// KindEvent is an asynchronous fact emitted on a topic.
	KindEvent Kind = "event"
)

// Envelope is the unit of communication on the bus. A single struct with a Kind
// discriminator carries all three shapes; the fields relevant to each kind are
// documented inline. Every envelope carries a lineage Stamp.
type Envelope struct {
	Kind  Kind          `json:"kind"`
	Stamp lineage.Stamp `json:"stamp"`

	// Call: the tool and operation to route to.
	Tool      string `json:"tool,omitempty"`
	Operation string `json:"operation,omitempty"`

	// Event: the topic the fact is emitted on.
	Topic string `json:"topic,omitempty"`

	// Payload is the call arguments, the return result, or the event fact.
	Payload json.RawMessage `json:"payload,omitempty"`

	// Return: a non-empty Error means the call failed.
	Error string `json:"error,omitempty"`
}

// NewCall builds a call envelope routed to tool.operation.
func NewCall(stamp lineage.Stamp, tool, operation string, payload json.RawMessage) Envelope {
	return Envelope{Kind: KindCall, Stamp: stamp, Tool: tool, Operation: operation, Payload: payload}
}

// NewEvent builds an event envelope emitted on topic.
func NewEvent(stamp lineage.Stamp, topic string, payload json.RawMessage) Envelope {
	return Envelope{Kind: KindEvent, Stamp: stamp, Topic: topic, Payload: payload}
}

// Reply builds a successful return for a call, preserving its lineage stamp.
func (e Envelope) Reply(payload json.RawMessage) Envelope {
	return Envelope{Kind: KindReturn, Stamp: e.Stamp, Payload: payload}
}

// ReplyError builds a failed return for a call, preserving its lineage stamp.
func (e Envelope) ReplyError(err error) Envelope {
	return Envelope{Kind: KindReturn, Stamp: e.Stamp, Error: err.Error()}
}

// Validate checks that the envelope carries the fields its kind requires.
func (e Envelope) Validate() error {
	if e.Stamp.Sequence == "" {
		return fmt.Errorf("bus: envelope missing sequence id")
	}
	switch e.Kind {
	case KindCall:
		if e.Tool == "" || e.Operation == "" {
			return fmt.Errorf("bus: call envelope missing tool/operation")
		}
	case KindEvent:
		if e.Topic == "" {
			return fmt.Errorf("bus: event envelope missing topic")
		}
	case KindReturn:
		// A return needs neither tool nor topic; Payload and Error are optional.
	default:
		return fmt.Errorf("bus: unknown envelope kind %q", e.Kind)
	}
	return nil
}
