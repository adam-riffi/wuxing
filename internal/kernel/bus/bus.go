package bus

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

// Handler answers a call routed to a tool. It receives the call envelope and
// returns a return envelope (built with Envelope.Reply / ReplyError). The
// context carries cancellation/deadline; a well-behaved handler honors it.
type Handler func(context.Context, Envelope) Envelope

// Bus is the in-process message channel. Tools register a Handler to answer
// calls (synchronous request/return); anyone may Subscribe to a topic and
// Publish events (asynchronous fan-out). It is safe for concurrent use.
//
// Calls are routed, never linked: a caller names a tool, the bus dispatches to
// whatever Handler is registered under that name — the seam a future
// out-of-process transport slots behind.
//
// Event fan-out is non-blocking and best-effort: each subscriber has a bounded
// buffer, and an event that finds a full buffer is dropped for that subscriber
// (counted via Subscription.Dropped). A slow subscriber therefore cannot block
// the publisher or any other subscriber. Durable capture is the storage layer's
// job; the bus is transport.
type Bus struct {
	bufSize int

	mu       sync.RWMutex
	handlers map[string]Handler
	subs     map[string][]*Subscription
	closed   bool
}

// Subscription is a live subscription to a topic.
type Subscription struct {
	C <-chan Envelope // receive events here

	ch        chan Envelope
	topic     string
	bus       *Bus
	dropped   atomic.Uint64
	closeOnce sync.Once
}

// Dropped reports how many events were dropped because this subscriber's buffer
// was full when they were published.
func (s *Subscription) Dropped() uint64 { return s.dropped.Load() }

// Cancel removes the subscription and closes its channel so a ranging consumer
// exits. It is safe to call more than once.
func (s *Subscription) Cancel() {
	s.bus.removeSub(s)
	s.closeChan()
}

func (s *Subscription) closeChan() { s.closeOnce.Do(func() { close(s.ch) }) }

// Option configures a Bus.
type Option func(*Bus)

// WithBuffer sets the default per-subscriber channel buffer (default 16).
func WithBuffer(n int) Option {
	return func(b *Bus) {
		if n >= 0 {
			b.bufSize = n
		}
	}
}

// New returns an empty bus ready for registration and subscription.
func New(opts ...Option) *Bus {
	b := &Bus{
		bufSize:  16,
		handlers: make(map[string]Handler),
		subs:     make(map[string][]*Subscription),
	}
	for _, o := range opts {
		o(b)
	}
	return b
}

// Register installs the handler that answers calls for tool. It errors on a
// duplicate registration or after the bus is closed.
func (b *Bus) Register(tool string, h Handler) error {
	if tool == "" {
		return fmt.Errorf("bus: cannot register an empty tool name")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return fmt.Errorf("bus: register on a closed bus")
	}
	if _, dup := b.handlers[tool]; dup {
		return fmt.Errorf("bus: tool %q already registered", tool)
	}
	b.handlers[tool] = h
	return nil
}

// Call routes a call envelope to the owning tool's handler and returns its
// return envelope. It errors if env is not a valid call or no tool is
// registered, and abandons a hung handler when ctx is cancelled.
func (b *Bus) Call(ctx context.Context, env Envelope) (Envelope, error) {
	if env.Kind != KindCall {
		return Envelope{}, fmt.Errorf("bus: Call requires a call envelope, got %q", env.Kind)
	}
	if err := env.Validate(); err != nil {
		return Envelope{}, err
	}

	b.mu.RLock()
	h, ok := b.handlers[env.Tool]
	b.mu.RUnlock()
	if !ok {
		return Envelope{}, fmt.Errorf("bus: no handler registered for tool %q", env.Tool)
	}

	done := make(chan Envelope, 1)
	go func() { done <- h(ctx, env) }()
	select {
	case <-ctx.Done():
		return Envelope{}, ctx.Err()
	case ret := <-done:
		return ret, nil
	}
}

// Subscribe registers a subscriber on topic with the bus's default buffer.
func (b *Bus) Subscribe(topic string) *Subscription {
	return b.SubscribeBuffered(topic, b.bufSize)
}

// SubscribeBuffered registers a subscriber on topic with an explicit buffer.
func (b *Bus) SubscribeBuffered(topic string, buffer int) *Subscription {
	if buffer < 0 {
		buffer = 0
	}
	ch := make(chan Envelope, buffer)
	s := &Subscription{C: ch, ch: ch, topic: topic, bus: b}

	b.mu.Lock()
	defer b.mu.Unlock()
	b.subs[topic] = append(b.subs[topic], s)
	return s
}

func (b *Bus) removeSub(target *Subscription) {
	b.mu.Lock()
	defer b.mu.Unlock()
	subs := b.subs[target.topic]
	for i, s := range subs {
		if s == target {
			b.subs[target.topic] = append(subs[:i], subs[i+1:]...)
			return
		}
	}
}

// Publish fans an event envelope out to every subscriber of its topic with a
// non-blocking send: a subscriber whose buffer is full has the event dropped
// (counted). An event with no subscribers is a no-op (a service emits blind).
// The fan-out runs under a read-lock so a subscription cannot be closed mid-send.
func (b *Bus) Publish(ctx context.Context, env Envelope) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if env.Kind != KindEvent {
		return fmt.Errorf("bus: Publish requires an event envelope, got %q", env.Kind)
	}
	if err := env.Validate(); err != nil {
		return err
	}

	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, s := range b.subs[env.Topic] {
		select {
		case s.ch <- env:
		default:
			s.dropped.Add(1)
		}
	}
	return nil
}

// Close stops the bus: no further registration, and every subscriber channel is
// closed so ranging consumers drain and exit. It is idempotent.
func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for topic, subs := range b.subs {
		for _, s := range subs {
			s.closeChan()
		}
		delete(b.subs, topic)
	}
}
