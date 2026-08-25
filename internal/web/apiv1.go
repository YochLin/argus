package web

import (
	"encoding/json"
	"net/http"
	"time"
)

// apiResponse is Step 4.2's standardized envelope. Every /api/v1 route
// answers with exactly this shape, success and failure alike, so a generated
// mobile client has one response type to unwrap instead of one per endpoint.
// The pre-v1 /api/* routes keep their bare-object shape — the existing SPA
// reads them and there's nothing to gain from churning both sides.
//
// Timestamp is Unix seconds, matching the plan doc's example payload.
type apiResponse struct {
	Success   bool   `json:"success"`
	Data      any    `json:"data"`
	Error     string `json:"error,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

func writeAPIOK(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: data, Timestamp: time.Now().Unix()})
}

func writeAPIError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, apiResponse{Success: false, Error: msg, Timestamp: time.Now().Unix()})
}

// decodeAPIJSON is decodeJSON with the v1 envelope on the failure path.
func decodeAPIJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

// registerAPIV1 mounts the token-authenticated /api/v1 surface (Phase 24
// Stage 4). It is skipped entirely — not 404'd per route — when JWT_SECRET
// or WEB_PASSWORD is unset, following Config.LLMAudit's precedent: an API
// whose auth isn't configured has no safe degraded mode, and registering
// unauthenticated routes to return 401 would still expose the route table.
// Callers of New see this reflected in /api/config.
func (s *Server) registerAPIV1() {
	if s.jwtSecret == "" || s.password == "" {
		return
	}
	s.mux.HandleFunc("POST /api/v1/auth/login", s.handleAPILogin)
	s.mux.HandleFunc("POST /api/v1/auth/refresh", s.handleAPIRefresh)
	// A trivial authenticated endpoint so a client can verify a token
	// without side effects — and, until Step 4.2 lands the resource routes,
	// the only thing requireAPIAuth guards.
	s.mux.HandleFunc("GET /api/v1/auth/me", s.requireAPIAuth(s.handleAPIMe))
}

func (s *Server) handleAPIMe(w http.ResponseWriter, r *http.Request) {
	writeAPIOK(w, map[string]any{"subject": apiTokenSubject, "authenticated": true})
}
