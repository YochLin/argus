package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"argus/internal/db"
)

// fakeAPIStore is apiV1Store by hand — the three reads /api/v1 needs beyond
// dbReader.
type fakeAPIStore struct {
	hits          map[string]string
	notifications []db.Notification
	markedRead    []int64
}

func (f *fakeAPIStore) GetScanHits(date string) (map[string]string, error) { return f.hits, nil }

func (f *fakeAPIStore) GetRecentNotifications(limit int) ([]db.Notification, error) {
	if limit < len(f.notifications) {
		return f.notifications[:limit], nil
	}
	return f.notifications, nil
}

func (f *fakeAPIStore) MarkNotificationRead(id int64) error {
	f.markedRead = append(f.markedRead, id)
	return nil
}

func authedGet(t *testing.T, s *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("X-API-Key", "script-key")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	return rec
}

func TestAPIScanHitsSortedAndDated(t *testing.T) {
	s := testAPIServer()
	store := &fakeAPIStore{hits: map[string]string{"ZM": "squeeze", "AAPL": "breakout", "MSFT": "box"}}
	s.apiDB = store

	rec := authedGet(t, s, "/api/v1/scan/hits?date=2026-08-19")
	if rec.Code != http.StatusOK {
		t.Fatalf("scan hits = %d, want 200", rec.Code)
	}
	var body struct {
		Data struct {
			Date string       `json:"date"`
			Hits []apiScanHit `json:"hits"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.Date != "2026-08-19" {
		t.Errorf("date = %q, want the requested one", body.Data.Date)
	}
	// A map's iteration order is random, so an unsorted response would pass
	// this only by luck.
	want := []string{"AAPL", "MSFT", "ZM"}
	for i, w := range want {
		if body.Data.Hits[i].Ticker != w {
			t.Fatalf("hits = %v, want sorted %v", body.Data.Hits, want)
		}
	}
}

func TestAPIScanHitsRejectsBadDate(t *testing.T) {
	s := testAPIServer()
	s.apiDB = &fakeAPIStore{}
	if rec := authedGet(t, s, "/api/v1/scan/hits?date=19-08-2026"); rec.Code != http.StatusBadRequest {
		t.Errorf("bad date = %d, want 400", rec.Code)
	}
}

func TestAPINotificationsLimit(t *testing.T) {
	s := testAPIServer()
	store := &fakeAPIStore{}
	for i := range 10 {
		store.notifications = append(store.notifications, db.Notification{ID: int64(i), Type: "stop_loss"})
	}
	s.apiDB = store

	rec := authedGet(t, s, "/api/v1/notifications?limit=3")
	var body struct {
		Data struct {
			Notifications []apiNotification `json:"notifications"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Data.Notifications) != 3 {
		t.Errorf("got %d notifications, want the requested 3", len(body.Data.Notifications))
	}
}

// TestAPILimitBounds covers the guard on the one query parameter that can ask
// this process to allocate: garbage and negatives fall back to the default,
// and an absurd value is capped rather than honored.
func TestAPILimitBounds(t *testing.T) {
	tests := []struct {
		query string
		want  int
	}{
		{"", 50},
		{"?limit=abc", 50},
		{"?limit=-5", 50},
		{"?limit=0", 50},
		{"?limit=10", 10},
		{"?limit=99999", 500},
	}
	for _, tt := range tests {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/notifications"+tt.query, nil)
		if got := apiLimit(r, 50, 500); got != tt.want {
			t.Errorf("apiLimit(%q) = %d, want %d", tt.query, got, tt.want)
		}
	}
}

func TestAPINotificationRead(t *testing.T) {
	s := testAPIServer()
	store := &fakeAPIStore{}
	s.apiDB = store

	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/42/read", nil)
	req.Header.Set("X-API-Key", "script-key")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("mark read = %d, want 200", rec.Code)
	}
	if len(store.markedRead) != 1 || store.markedRead[0] != 42 {
		t.Errorf("marked %v read, want [42]", store.markedRead)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/notifications/abc/read", nil)
	req.Header.Set("X-API-Key", "script-key")
	rec = httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("non-numeric id = %d, want 400", rec.Code)
	}
}

// TestAPITradeWithoutExecutor pins the 409 an app gets when Telegram isn't
// configured — distinguishable from a rejected trade (400) and from a
// missing route (404), which is exactly what the frontend needs to show the
// right message.
func TestAPITradeWithoutExecutor(t *testing.T) {
	s := testAPIServer() // no Trade configured
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trade/buy", nil)
	req.Header.Set("X-API-Key", "script-key")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Errorf("buy without a TradeExecutor = %d, want 409", rec.Code)
	}
}

// TestAPIResourcesRequireAuth: every v1 resource route must be behind
// requireAPIAuth. A route added without the wrapper would answer 200 here.
func TestAPIResourcesRequireAuth(t *testing.T) {
	s := testAPIServer()
	s.apiDB = &fakeAPIStore{}
	paths := []struct {
		method, path string
	}{
		{http.MethodGet, "/api/v1/portfolio"},
		{http.MethodGet, "/api/v1/watchlist"},
		{http.MethodPost, "/api/v1/watchlist"},
		{http.MethodPost, "/api/v1/watchlist/remove"},
		{http.MethodPost, "/api/v1/trade/buy"},
		{http.MethodPost, "/api/v1/trade/sell"},
		{http.MethodPost, "/api/v1/risk/stop"},
		{http.MethodGet, "/api/v1/recommendations/latest"},
		{http.MethodGet, "/api/v1/scan/hits"},
		{http.MethodGet, "/api/v1/notifications"},
		{http.MethodPost, "/api/v1/notifications/1/read"},
	}
	for _, p := range paths {
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, httptest.NewRequest(p.method, p.path, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s unauthenticated = %d, want 401", p.method, p.path, rec.Code)
		}
	}
}
