package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"argus/internal/i18n"
)

func testServer() *Server {
	return &Server{
		db:     &fakeDB{},
		quotes: &fakeQuotes{},
		lang:   i18n.EN,
	}
}

func TestHandleConfig(t *testing.T) {
	s := testServer()
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("GET /api/config", s.handleConfig)

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got configResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Lang != "en" {
		t.Errorf("Lang = %q, want %q", got.Lang, "en")
	}
}

func TestHandleDashboard(t *testing.T) {
	s := testServer()
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("GET /api/dashboard", s.handleDashboard)

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/dashboard", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got dashboardResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
}

func TestHandleCalendar(t *testing.T) {
	s := testServer()
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("GET /api/calendar", s.handleCalendar)

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/calendar?month=2026-07", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	var got calendarResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Month != "2026-07" {
		t.Errorf("Month = %q, want 2026-07", got.Month)
	}
}

func TestSPAHandler_ServesIndexForUnknownRoute(t *testing.T) {
	handler := spaHandler()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/some/client/side/route", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (SPA fallback to index.html)", rec.Code)
	}
	if rec.Body.Len() == 0 {
		t.Error("body is empty, want the embedded index.html content")
	}
}

func TestSPAHandler_ServesRoot(t *testing.T) {
	handler := spaHandler()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}
