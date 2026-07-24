package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
)

func testServer() *Server {
	return &Server{
		db:      &fakeDB{},
		quotes:  &fakeQuotes{},
		history: &fakeHistory{},
		lang:    i18n.EN,
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

func TestHandleStatus(t *testing.T) {
	s := testServer()
	s.db = &fakeDB{watchlist: []string{"AAPL", "MSFT"}}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("GET /api/status", s.handleStatus)

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/status", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	var got statusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.WatchingCount != 2 {
		t.Errorf("WatchingCount = %d, want 2", got.WatchingCount)
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

func TestHandleRounds(t *testing.T) {
	s := testServer()
	s.db = &fakeDB{txs: []db.Transaction{
		tx("AAPL", "BUY", 10, 100, "2026-06-01"),
	}}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("GET /api/rounds", s.handleRounds)

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/rounds", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	var got roundsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Rounds) != 1 || got.Rounds[0].Ticker != "AAPL" {
		t.Errorf("Rounds = %+v, want 1 open AAPL round", got.Rounds)
	}
}

func TestHandleReports(t *testing.T) {
	sell := tx("AAPL", "SELL", 10, 120, "2026-06-10")
	sell.RealizedPnL = 200
	s := testServer()
	s.db = &fakeDB{txs: []db.Transaction{
		tx("AAPL", "BUY", 10, 100, "2026-06-01"),
		sell,
	}}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("GET /api/reports", s.handleReports)

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/reports", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	var got reportsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.ByTicker) != 1 || got.ByTicker[0].Key != "AAPL" || got.ByTicker[0].N != 1 {
		t.Errorf("ByTicker = %+v, want 1 AAPL group with n=1", got.ByTicker)
	}
}

func TestHandleRoundDetail(t *testing.T) {
	s := testServer()
	s.db = &fakeDB{txs: []db.Transaction{
		tx("AAPL", "BUY", 10, 100, "2026-06-01"),
	}}
	s.history = &fakeHistory{candles: map[string][]data.Candle{
		"AAPL": {candle("2026-06-01", 100)},
	}}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("GET /api/round-detail", s.handleRoundDetail)

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/round-detail?ticker=AAPL&start=2026-06-01", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	var got roundDetailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Ticker != "AAPL" || got.Start != "2026-06-01" {
		t.Errorf("got = %+v, want ticker AAPL start 2026-06-01", got)
	}
}

func TestHandleRoundDetail_MissingParams(t *testing.T) {
	s := testServer()
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("GET /api/round-detail", s.handleRoundDetail)

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/round-detail", nil))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for missing ticker/start", rec.Code)
	}
}

func TestHandleRoundDetail_NotFound(t *testing.T) {
	s := testServer()
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("GET /api/round-detail", s.handleRoundDetail)

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/round-detail?ticker=AAPL&start=1999-01-01", nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for a nonexistent round", rec.Code)
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
