package notification

import (
	"context"
	"sync"

	"argus/internal/logger"
)

// wsSubscriberBuffer is how many events a subscriber can fall behind by
// before its next one is dropped. Deep enough to absorb a burst (a daily
// report's alert cluster), shallow enough that a client that stopped reading
// entirely is noticed rather than silently accumulating memory.
const wsSubscriberBuffer = 32

// WebSocketHub is Stage 2 Step 2.2's deferred WebSocketNotifier, split in
// two: this half is a plain fan-out that implements Notifier and knows
// nothing about HTTP, and internal/web's /api/v1/ws handler is the half that
// speaks the wire protocol. Keeping the transport out of this package is
// what stops the event bus from acquiring a websocket dependency it would
// then impose on internal/bot, which imports it.
//
// Subscribers are anonymous: Argus is single-user, so there is no per-viewer
// filtering to do beyond the event-type filter the handler applies itself.
type WebSocketHub struct {
	mu   sync.Mutex
	subs map[chan Event]struct{}
}

func NewWebSocketHub() *WebSocketHub {
	return &WebSocketHub{subs: make(map[chan Event]struct{})}
}

// Subscribe returns a channel of live events plus the function that stops the
// subscription. The caller must call it (defer it) — nothing else ever
// removes a subscriber, so a leaked one keeps receiving until the process
// ends.
func (h *WebSocketHub) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, wsSubscriberBuffer)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()

	var once sync.Once
	return ch, func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.subs, ch)
			h.mu.Unlock()
			close(ch)
		})
	}
}

// Send fans e out to every subscriber, dropping (not blocking on) any whose
// buffer is full. A stalled browser tab must never be able to hold up
// delivery to Telegram — Dispatcher.Publish calls its Notifiers in sequence,
// so a blocking send here would stall the whole event.
func (h *WebSocketHub) Send(ctx context.Context, e Event) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- e:
		default:
			logger.Warnf("notification: websocket subscriber behind, dropping %s", e.Type)
		}
	}
	return nil
}

// Subscribers reports how many clients are currently connected — for a
// status endpoint and for tests, not for any delivery decision.
func (h *WebSocketHub) Subscribers() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}
