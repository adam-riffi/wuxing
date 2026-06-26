package bus

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func echoHandler(_ context.Context, call Envelope) Envelope {
	return call.Reply(call.Payload)
}

func TestBus_CallRoundTrip(t *testing.T) {
	b := New()
	if err := b.Register("connectors", echoHandler); err != nil {
		t.Fatal(err)
	}

	call := NewCall(testStamp(), "connectors", "write", json.RawMessage(`{"rows":3}`))
	ret, err := b.Call(context.Background(), call)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if ret.Kind != KindReturn {
		t.Errorf("kind: got %q want return", ret.Kind)
	}
	if string(ret.Payload) != `{"rows":3}` {
		t.Errorf("payload round-trip: got %s", ret.Payload)
	}
	if ret.Stamp != call.Stamp {
		t.Errorf("stamp not preserved through the call")
	}
}

func TestBus_CallNoHandler(t *testing.T) {
	b := New()
	_, err := b.Call(context.Background(), NewCall(testStamp(), "ghost", "op", nil))
	if err == nil {
		t.Fatal("expected error for unregistered tool")
	}
}

func TestBus_CallWrongKind(t *testing.T) {
	b := New()
	_, err := b.Call(context.Background(), NewEvent(testStamp(), "topic", nil))
	if err == nil {
		t.Fatal("expected error calling with an event envelope")
	}
}

func TestBus_CallHandlerError(t *testing.T) {
	b := New()
	_ = b.Register("ai", func(_ context.Context, call Envelope) Envelope {
		return call.ReplyError(errors.New("model refused"))
	})
	ret, err := b.Call(context.Background(), NewCall(testStamp(), "ai", "infer", nil))
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if ret.Error != "model refused" {
		t.Errorf("handler error not propagated: %q", ret.Error)
	}
}

func TestBus_CallContextCancelled(t *testing.T) {
	b := New()
	release := make(chan struct{})
	defer close(release)
	// A handler that ignores its context and hangs until the test ends. Call
	// must still return when its own context is cancelled rather than block on
	// the hung handler forever.
	_ = b.Register("hung", func(_ context.Context, call Envelope) Envelope {
		<-release
		return call.Reply(nil)
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled before the call; the hung handler never replies

	_, err := b.Call(ctx, NewCall(testStamp(), "hung", "op", nil))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestBus_RegisterErrors(t *testing.T) {
	b := New()
	if err := b.Register("", echoHandler); err == nil {
		t.Error("expected error registering empty tool name")
	}
	if err := b.Register("x", echoHandler); err != nil {
		t.Fatal(err)
	}
	if err := b.Register("x", echoHandler); err == nil {
		t.Error("expected error on duplicate registration")
	}
}

func TestBus_PubSubFanout(t *testing.T) {
	b := New()
	a := b.Subscribe("mtg-db updated")
	c := b.Subscribe("mtg-db updated")

	ev := NewEvent(testStamp(), "mtg-db updated", json.RawMessage(`{"new_set":true}`))
	if err := b.Publish(context.Background(), ev); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	for i, ch := range []<-chan Envelope{a.C, c.C} {
		select {
		case got := <-ch:
			if got.Stamp.Sequence != "seq-1" {
				t.Errorf("subscriber %d: sequence id not propagated: %q", i, got.Stamp.Sequence)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d did not receive the event", i)
		}
	}
}

func TestBus_PublishNoSubscribers(t *testing.T) {
	b := New()
	if err := b.Publish(context.Background(), NewEvent(testStamp(), "void", nil)); err != nil {
		t.Fatalf("publishing to no subscribers should be a no-op, got %v", err)
	}
}

func TestBus_PublishWrongKind(t *testing.T) {
	b := New()
	if err := b.Publish(context.Background(), NewCall(testStamp(), "t", "op", nil)); err == nil {
		t.Fatal("expected error publishing a call envelope")
	}
}

func TestBus_Unsubscribe(t *testing.T) {
	b := New()
	sub := b.Subscribe("t")
	sub.Cancel()

	// After cancel the channel is closed.
	if _, open := <-sub.C; open {
		t.Fatal("channel should be closed after Cancel")
	}
	// And a publish reaches nobody (no panic on the removed subscriber).
	if err := b.Publish(context.Background(), NewEvent(testStamp(), "t", nil)); err != nil {
		t.Fatalf("Publish after Cancel: %v", err)
	}
	sub.Cancel() // idempotent, must not panic
}

func TestBus_SlowConsumerDoesNotBlockFast(t *testing.T) {
	b := New()
	const n = 50

	fast := b.SubscribeBuffered("t", n) // buffered enough to never drop
	slow := b.SubscribeBuffered("t", 1) // tiny buffer, never drained

	done := make(chan struct{})
	go func() {
		for i := 0; i < n; i++ {
			<-fast.C
		}
		close(done)
	}()

	// Non-blocking fan-out: the never-drained slow subscriber cannot stall this.
	for range n {
		if err := b.Publish(context.Background(), NewEvent(testStamp(), "t", nil)); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("fast consumer was starved by the slow consumer")
	}

	if slow.Dropped() == 0 {
		t.Error("expected the slow consumer to have dropped events")
	}
}

func TestBus_Close(t *testing.T) {
	b := New()
	sub := b.Subscribe("t")
	b.Close()

	if _, open := <-sub.C; open {
		t.Fatal("Close should close subscriber channels")
	}
	if err := b.Register("x", echoHandler); err == nil {
		t.Error("Register after Close should error")
	}
	b.Close() // idempotent, must not panic
}
