package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlerRecoversPanicInRouteShape(t *testing.T) {
	boom := func(http.ResponseWriter, *http.Request) { panic("boom") }
	s := &Server{mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /api/boom", boom)
	s.mux.HandleFunc("GET /api/v1/boom", boom)

	t.Run("pre-v1 gets the bare error body", func(t *testing.T) {
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/api/boom", nil))
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", rec.Code)
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("body %q is not JSON: %v", rec.Body.String(), err)
		}
		if body["error"] != "internal error" || body["success"] != nil {
			t.Errorf("body = %v, want only {error: internal error}", body)
		}
	})

	t.Run("v1 gets the envelope", func(t *testing.T) {
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/boom", nil))
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", rec.Code)
		}
		var env apiResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("body %q is not JSON: %v", rec.Body.String(), err)
		}
		if env.Success || env.Error != "internal error" {
			t.Errorf("envelope = %+v, want success=false error=internal error", env)
		}
	})
}

func TestRecoverPanicsLetsAbortHandlerThrough(t *testing.T) {
	h := recoverPanics(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) }))
	defer func() {
		if p := recover(); p != http.ErrAbortHandler {
			t.Errorf("recovered %v, want http.ErrAbortHandler re-panicked", p)
		}
	}()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/x", nil))
}
