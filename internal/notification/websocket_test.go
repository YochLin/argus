package notification

import (
	"context"
	"testing"
)

func TestWebSocketHubFanOut(t *testing.T) {
	h := NewWebSocketHub()
	a, stopA := h.Subscribe()
	b, stopB := h.Subscribe()
	defer stopB()

	if h.Subscribers() != 2 {
		t.Fatalf("Subscribers() = %d, want 2", h.Subscribers())
	}
	h.Send(context.Background(), Event{Type: "stop_loss", Text: "AAPL"})
	for name, ch := range map[string]<-chan Event{"a": a, "b": b} {
		select {
		case e := <-ch:
			if e.Type != "stop_loss" {
				t.Errorf("subscriber %s got %q", name, e.Type)
			}
		default:
			t.Errorf("subscriber %s got nothing", name)
		}
	}

	// After unsubscribing, a's channel is closed and only b is left.
	stopA()
	if h.Subscribers() != 1 {
		t.Errorf("Subscribers() after unsubscribe = %d, want 1", h.Subscribers())
	}
	if _, open := <-a; open {
		t.Error("unsubscribed channel still open")
	}
	// Unsubscribing twice must not panic on a double close — a handler that
	// defers its stop and also calls it on an error path would do exactly
	// this.
	stopA()
}

// TestWebSocketHubDropsWhenBehind is the important one: Dispatcher.Publish
// calls its Notifiers in sequence, so a subscriber that stopped reading must
// be dropped rather than blocking delivery to Telegram.
func TestWebSocketHubDropsWhenBehind(t *testing.T) {
	h := NewWebSocketHub()
	_, stop := h.Subscribe()
	defer stop()

	// One more than the buffer; the last Send must still return promptly.
	done := make(chan struct{})
	go func() {
		for range wsSubscriberBuffer + 1 {
			h.Send(context.Background(), Event{Type: "price_event"})
		}
		close(done)
	}()
	<-done
}
