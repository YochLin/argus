package web

import (
	"encoding/json"
	"net/http"
	"time"

	"argus/internal/logger"
)

// Pre-v1 / SPA Response Protocol:
// Pre-v1 endpoints (/api/*) are primarily consumed by the internal React SPA.
// Success responses return raw domain JSON objects directly without an outer envelope,
// preserving lightweight frontend contracts and direct data deserialization.
// Error responses return a normalized JSON object: {"error": "<message>"}.

// errorResponse represents the standard Pre-v1 JSON error response shape.
type errorResponse struct {
	Error string `json:"error"`
}

// writeJSON writes status and JSON-encodes v to w with Content-Type: application/json.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		logger.Errorf("web: encode response: %v", err)
	}
}

// writeError writes a standard Pre-v1 JSON error payload with the given HTTP status code.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

// decodeJSON decodes the JSON request body into v. If decoding fails,
// it writes a 400 Bad Request error response in the Pre-v1 format and returns false.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

// API v1 Response Protocol:
// API v1 endpoints (/api/v1/*) are consumed by programmatic clients, external tools,
// mobile apps, and OpenAPI-generated SDKs.
// Every /api/v1 endpoint returns a uniform envelope:
//   {
//     "success": bool,
//     "data": <payload or null>,
//     "error": "<error message if success is false>",
//     "timestamp": <unix seconds>
//   }

// apiResponse is the standardized envelope for all /api/v1 responses.
type apiResponse struct {
	Success   bool   `json:"success"`
	Data      any    `json:"data"`
	Error     string `json:"error,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

// writeAPIResponse writes data wrapped in the standard v1 envelope with the given status code.
func writeAPIResponse(w http.ResponseWriter, status int, data any) {
	writeJSON(w, status, apiResponse{
		Success:   true,
		Data:      data,
		Timestamp: time.Now().Unix(),
	})
}

// writeAPIOK writes a 200 OK response wrapped in the standard v1 envelope.
func writeAPIOK(w http.ResponseWriter, data any) {
	writeAPIResponse(w, http.StatusOK, data)
}

// writeAPIError writes an error response wrapped in the standard v1 envelope with the given status code.
func writeAPIError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, apiResponse{
		Success:   false,
		Error:     msg,
		Timestamp: time.Now().Unix(),
	})
}

// decodeAPIJSON decodes the JSON request body into v. If decoding fails,
// it writes a 400 Bad Request error response in the v1 envelope format and returns false.
func decodeAPIJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}
