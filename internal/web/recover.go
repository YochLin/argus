package web

import (
	"net/http"
	"runtime/debug"
	"strings"

	"argus/internal/logger"
)

// recoverPanics turns a handler panic into a 500 in the shape the route
// promises: the /api/v1 envelope under that prefix, the pre-v1
// {"error": ...} everywhere else. net/http would only log and drop the
// connection, leaving the SPA with a network error instead of a message.
//
// Wrapping the whole mux (Server.Handler) replaces the per-handler
// defer-recover blocks and covers /api/v1, which never had one. It must not
// wrap w: the /api/v1/ws upgrade needs the raw ResponseWriter's Hijacker.
func recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			p := recover()
			if p == nil {
				return
			}
			if p == http.ErrAbortHandler {
				panic(p) // net/http's deliberate "abort this response" signal
			}
			logger.Errorf("web: panic serving %s %s: %v\n%s", r.Method, r.URL.Path, p, debug.Stack())
			if strings.HasPrefix(r.URL.Path, "/api/v1/") {
				writeAPIError(w, http.StatusInternalServerError, "internal error")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal error")
		}()
		next.ServeHTTP(w, r)
	})
}

// Handler is the server's root handler: the route mux behind recoverPanics.
func (s *Server) Handler() http.Handler {
	return recoverPanics(s.mux)
}
