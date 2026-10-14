package web

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"argus/internal/logger"
	"argus/internal/notification"
)

// Phase 24 Stage 4 Step 4.3: GET /api/v1/ws, the live half of the API.
// internal/notification.WebSocketHub does the fan-out; this file is only the
// wire protocol, which is why the event bus itself still has no websocket
// dependency (internal/bot imports it, and shouldn't inherit one).
//
// github.com/coder/websocket is the one dependency added for this. Hand-rolling
// RFC 6455 over http.Hijacker is not "a few lines" — framing, client-frame
// unmasking, the close handshake and ping/pong all have to be right — and this
// library has zero transitive dependencies of its own.

// wsEvent is the on-the-wire shape. Deliberately the same four fields
// notification.Event carries: Text is already the fully rendered message
// (see Event's doc comment), so there is still nothing for a structured
// Payload field to add — the deferral recorded in Stage 2 holds.
type wsEvent struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	Level string `json:"level"`
	Time  int64  `json:"time"`
}

// wsWriteTimeout bounds one frame write. Without it a client that stops
// reading TCP entirely would park this goroutine indefinitely holding a
// subscription.
const wsWriteTimeout = 10 * time.Second

// handleWS upgrades and then streams events until the client goes away or
// the server shuts down.
//
// Auth is the query parameter `?token=<access token>` rather than an
// Authorization header, because the browser WebSocket API cannot set
// headers. requireAPIAuth is therefore not usable here; the check below is
// the same one, reading a different place. A native app can use either
// (X-API-Key still works, since it can set headers).
//
// `?types=stop_loss,price_event` filters to those event types; absent means
// everything. That is the whole of the plan doc's "subscription" model —
// there is one user, so there is nothing else to scope a subscription by.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	if !s.wsAuthorized(r) {
		writeAPIError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		// Accept has already written a response.
		logger.Errorf("web: ws accept: %v", err)
		return
	}
	defer conn.CloseNow()

	wanted := wsTypeFilter(r.URL.Query().Get("types"))
	events, unsubscribe := s.events.Subscribe()
	defer unsubscribe()

	// CloseRead answers pings and watches for the client's close frame,
	// cancelling the returned context when it arrives — this handler never
	// reads application messages, so there's nothing to lose by discarding
	// them.
	ctx := conn.CloseRead(r.Context())
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-events:
			if !ok {
				return
			}
			if wanted != nil && !wanted[e.Type] {
				continue
			}
			writeCtx, cancel := context.WithTimeout(ctx, wsWriteTimeout)
			err := wsjson.Write(writeCtx, conn, wsEvent{Type: e.Type, Text: e.Text, Level: string(e.Level), Time: e.Time.Unix()})
			cancel()
			if err != nil {
				return
			}
		}
	}
}

// wsAuthorized mirrors requireAPIAuth for a connection that can't send an
// Authorization header.
func (s *Server) wsAuthorized(r *http.Request) bool {
	if key := r.Header.Get("X-API-Key"); key != "" && s.apiKey != "" &&
		subtle.ConstantTimeCompare([]byte(key), []byte(s.apiKey)) == 1 {
		return true
	}
	if token := r.URL.Query().Get("token"); token != "" {
		return s.parseAPIToken(token, apiTokenAccess) == nil
	}
	return false
}

// wsTypeFilter turns "a,b" into a lookup set; nil (no filter) for an empty
// or all-blank parameter, so "subscribe to everything" stays the default.
func wsTypeFilter(raw string) map[string]bool {
	out := make(map[string]bool)
	for _, t := range strings.Split(raw, ",") {
		if t = strings.TrimSpace(t); t != "" {
			out[t] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// wsHub is the Server's view of the hub — an interface only so a test can
// drive the handler without the rest of the notification wiring.
type wsHub interface {
	Subscribe() (<-chan notification.Event, func())
}
