package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"argus/internal/notification"
)

func TestWSTypeFilter(t *testing.T) {
	tests := []struct {
		raw  string
		want map[string]bool
	}{
		{"", nil},
		{"  ,  ", nil},
		{"stop_loss", map[string]bool{"stop_loss": true}},
		{"stop_loss, price_event ", map[string]bool{"stop_loss": true, "price_event": true}},
	}
	for _, tt := range tests {
		got := wsTypeFilter(tt.raw)
		if len(got) != len(tt.want) {
			t.Errorf("wsTypeFilter(%q) = %v, want %v", tt.raw, got, tt.want)
			continue
		}
		for k := range tt.want {
			if !got[k] {
				t.Errorf("wsTypeFilter(%q) missing %q", tt.raw, k)
			}
		}
	}
}

func TestWSRejectsUnauthenticated(t *testing.T) {
	hub := notification.NewWebSocketHub()
	s := New(Config{Password: "hunter2", JWTSecret: "test-secret", APIKey: "script-key", Events: hub})
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("ws without a token = %d, want 401", rec.Code)
	}
}

// TestWSStreamsFilteredEvents runs the handler against a real server and a
// real client: an event of an unsubscribed type must not arrive, and a
// subscribed one must — the filter is the only logic in the streaming loop,
// and getting it inverted would be invisible without an end-to-end check.
func TestWSStreamsFilteredEvents(t *testing.T) {
	hub := notification.NewWebSocketHub()
	s := New(Config{Password: "hunter2", JWTSecret: "test-secret", APIKey: "script-key", Events: hub})
	srv := httptest.NewServer(s.mux)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	url := "ws" + srv.URL[len("http"):] + "/api/v1/ws?types=stop_loss"
	conn, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		HTTPHeader: http.Header{"X-API-Key": {"script-key"}},
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.CloseNow()

	// Wait for the handler's Subscribe to land before publishing, or the
	// event races the connection setup.
	for hub.Subscribers() == 0 {
		time.Sleep(time.Millisecond)
	}
	hub.Send(ctx, notification.Event{Type: "price_event", Text: "ignored", Time: time.Now()})
	hub.Send(ctx, notification.Event{Type: "stop_loss", Text: "AAPL broke its stop", Level: notification.LevelCritical, Time: time.Now()})

	var got wsEvent
	if err := wsjson.Read(ctx, conn, &got); err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.Type != "stop_loss" {
		t.Fatalf("first event = %+v, want the subscribed stop_loss (the filtered-out one leaked)", got)
	}
	if got.Text != "AAPL broke its stop" || got.Level != string(notification.LevelCritical) || got.Time == 0 {
		t.Errorf("event = %+v, want the full payload", got)
	}
}
