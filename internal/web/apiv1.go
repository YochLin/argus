package web

import (
	"encoding/json"
	"net/http"
	"time"

	"argus/internal/logger"
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

// minJWTSecretLen is HS256's entropy floor here: a token's signature is only
// as hard to forge as this key. Anyone who ever sees one signed token can
// brute-force a short key offline and mint their own 30-day refresh token —
// unlike WEB_PASSWORD (a private-network cookie HMAC), this key protects
// tokens meant to leave the machine with a future mobile app. Fail-closed
// rather than warn-and-continue: an operator who sets a weak secret and sees
// the API working fine has no reason to go check the log.
const minJWTSecretLen = 32

// registerAPIV1 mounts the token-authenticated /api/v1 surface (Phase 24
// Stage 4). It is skipped — every route replying with a 404 in the standard
// envelope, not the SPA catch-all's 200 HTML — when JWT_SECRET is too short
// or unset, or WEB_PASSWORD is unset, following Config.LLMAudit's precedent
// that an API whose auth isn't configured has no safe degraded mode. Unlike
// LLMAudit's consumer (the SPA, which reads /api/config first), this
// surface's consumers are mobile apps and scripts with no fallback of their
// own — a 200 they can't parse as JSON is worse than a 404 they can.
func (s *Server) registerAPIV1() {
	if len(s.jwtSecret) < minJWTSecretLen || s.password == "" {
		if s.jwtSecret != "" && len(s.jwtSecret) < minJWTSecretLen {
			logger.Errorf("web: JWT_SECRET is too short (%d chars, need >= %d); /api/v1 disabled", len(s.jwtSecret), minJWTSecretLen)
		}
		s.mux.HandleFunc("/api/v1/", func(w http.ResponseWriter, r *http.Request) {
			writeAPIError(w, http.StatusNotFound, "api v1 is not configured")
		})
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
