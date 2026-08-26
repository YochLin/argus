package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"argus/internal/db"
)

func TestHandleEvents(t *testing.T) {
	fake := &fakeDB{priceEvents: []db.PriceEvent{
		{ID: 3, Ticker: "NVDA", Market: "us", Date: "2026-08-25", GapPct: -6.2, Summary: "gapped down on guidance"},
		{ID: 2, Ticker: "2330", Market: "tw", Date: "2026-08-25", ChangePct: 10.0},
		{ID: 1, Ticker: "NVDA", Market: "us", Date: "2026-08-20", CumulativePct: -9.1},
	}}
	s := &Server{db: fake}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("GET /api/events", s.handleEvents)

	get := func(t *testing.T, path string) eventsResponse {
		t.Helper()
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
		}
		var got eventsResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		return got
	}

	all := get(t, "/api/events")
	if len(all.Events) != 3 || all.Events[0].ID != 3 {
		t.Errorf("Events = %+v, want all 3 newest-first", all.Events)
	}
	if all.Events[0].Summary != "gapped down on guidance" || all.Events[1].Summary != "" {
		t.Errorf("summaries not passed through verbatim: %+v", all.Events)
	}

	byTicker := get(t, "/api/events?ticker=nvda")
	if len(byTicker.Events) != 2 {
		t.Fatalf("?ticker=nvda returned %d events, want 2 (case-insensitive match)", len(byTicker.Events))
	}
	for _, ev := range byTicker.Events {
		if ev.Ticker != "NVDA" {
			t.Errorf("?ticker=nvda leaked %s", ev.Ticker)
		}
	}
}
