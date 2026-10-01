package web

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/db"
)

// risingCandles is n daily bars ending on end, close 100, 101, 102, ...
func risingCandles(n int, end time.Time) []data.Candle {
	out := make([]data.Candle, n)
	for i := range out {
		c := 100 + float64(i)
		out[i] = data.Candle{Date: end.AddDate(0, 0, i-n+1), Open: c, High: c + 1, Low: c - 1, Close: c, Volume: 1000}
	}
	return out
}

func TestBuildFillSnapshot(t *testing.T) {
	end := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	cs := risingCandles(150, end)
	fillDay := cs[100].Date.Format("2006-01-02")
	pub := time.Date(2026, 5, 1, 8, 30, 0, 0, time.UTC)
	fdb := &fakeDB{fillNews: map[string][]db.FillNews{
		"AAPL|" + fillDay: {
			{Headline: "AAPL beats", Source: "Reuters", URL: "https://x/1", PublishedAt: pub, Sentiment: "bull", Tag: "earn", Major: true},
			{Headline: "unlabelled", Source: "Wire", URL: "https://x/2"},
		},
	}}
	hist := &fakeHistory{candles: map[string][]data.Candle{"AAPL": cs}}

	got := buildFillSnapshot(fdb, hist, "AAPL", fillDay, 200, end)

	if got.Snapshot == nil || got.Snapshot.Date != fillDay || got.Snapshot.Close != 200 {
		t.Fatalf("snapshot = %+v", got.Snapshot)
	}
	if got.Snapshot.Trend != "bull" || got.Snapshot.Patterns == nil {
		t.Errorf("trend %q, patterns %v (must be [] not null)", got.Snapshot.Trend, got.Snapshot.Patterns)
	}
	// 49 bars exist after bar 100: both horizons are known.
	if got.Hindsight == nil || got.Hindsight.Fwd5Pct == nil || got.Hindsight.Fwd20Pct == nil {
		t.Fatalf("hindsight = %+v", got.Hindsight)
	}
	if want := (205.0/200 - 1) * 100; math.Abs(*got.Hindsight.Fwd5Pct-want) > 1e-9 {
		t.Errorf("fwd5 = %v, want %v", *got.Hindsight.Fwd5Pct, want)
	}

	if len(got.News) != 2 {
		t.Fatalf("news = %+v", got.News)
	}
	n := got.News[0]
	if n.URL != "https://x/1" || n.Sentiment != "bull" || n.Tag != "earn" || !n.Major || n.PublishedAt != "2026-05-01T08:30:00Z" {
		t.Errorf("labelled news = %+v", n)
	}
	if got.News[1].Sentiment != "" || got.News[1].PublishedAt != "" {
		t.Errorf("an unlabelled, undated headline must come through as such: %+v", got.News[1])
	}
	// The fill day is the one asked for, whatever bar it landed on.
	if got.Date != fillDay || got.Ticker != "AAPL" {
		t.Errorf("echo = %q %q", got.Ticker, got.Date)
	}
}

func TestBuildFillSnapshot_NoDataStillReturnsNews(t *testing.T) {
	end := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	fdb := &fakeDB{fillNews: map[string][]db.FillNews{"AAPL|2024-01-02": {{Headline: "old story"}}}}

	// The candles stop long after the fill date: no bar maps to it.
	hist := &fakeHistory{candles: map[string][]data.Candle{"AAPL": risingCandles(150, end)}}
	got := buildFillSnapshot(fdb, hist, "AAPL", "2024-01-02", 100, end)
	if got.Snapshot != nil || got.Hindsight != nil {
		t.Errorf("a fill older than the data must have no snapshot: %+v %+v", got.Snapshot, got.Hindsight)
	}
	if len(got.News) != 1 {
		t.Errorf("news must not depend on candles: %+v", got.News)
	}

	// A failing history provider degrades the same way, not to an error.
	got = buildFillSnapshot(fdb, &fakeHistory{err: http.ErrHandlerTimeout}, "AAPL", "2024-01-02", 100, end)
	if got.Snapshot != nil || len(got.News) != 1 {
		t.Errorf("history error: %+v", got)
	}
}

func TestHandleFillSnapshot(t *testing.T) {
	end := time.Now().UTC().Truncate(24 * time.Hour)
	cs := risingCandles(150, end)
	s := &Server{db: &fakeDB{}, history: &fakeHistory{candles: map[string][]data.Candle{"AAPL": cs}}}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("GET /api/fill-snapshot", s.handleFillSnapshot)

	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec
	}

	for _, bad := range []string{"/api/fill-snapshot", "/api/fill-snapshot?ticker=AAPL", "/api/fill-snapshot?ticker=AAPL&date=yesterday"} {
		if rec := get(bad); rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", bad, rec.Code)
		}
	}

	day := cs[100].Date.Format("2006-01-02")
	rec := get("/api/fill-snapshot?ticker=AAPL&date=" + day + "&price=200")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var got fillSnapshotResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Snapshot == nil || got.Hindsight == nil || got.Hindsight.Fwd5Pct == nil {
		t.Fatalf("body = %s", rec.Body.String())
	}

	// No price: the snapshot is still there, the after-the-fact moves are not.
	rec = get("/api/fill-snapshot?ticker=AAPL&date=" + day)
	got = fillSnapshotResponse{}
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Snapshot == nil || got.Hindsight == nil || got.Hindsight.Fwd5Pct != nil {
		t.Errorf("no price: %s", rec.Body.String())
	}
}

func TestBuildChart_Fills(t *testing.T) {
	fdb := &fakeDB{txs: []db.Transaction{
		{ID: 1, Date: "2026-01-05", Ticker: "AAPL", Side: "BUY", Shares: 10, Price: 150},
		{ID: 2, Date: "2026-02-01", Ticker: "AAPL", Side: "SELL", Shares: 10, Price: 160, RealizedPnL: 100},
		// Sold and re-bought the same day: two rounds share 2026-02-01.
		{ID: 3, Date: "2026-02-01", Ticker: "AAPL", Side: "BUY", Shares: 5, Price: 161},
		{ID: 4, Date: "2026-02-02", Ticker: "MSFT", Side: "BUY", Shares: 1, Price: 400},
	}}
	hist := &fakeHistory{candles: map[string][]data.Candle{"AAPL": levelBaseCandlesForTest(40)}}

	got, err := buildChart(fdb, &fakeQuotes{}, hist, "AAPL")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Fills) != 3 {
		t.Fatalf("fills = %+v, want only AAPL's three", got.Fills)
	}
	roundOf := map[int64]string{}
	for _, f := range got.Fills {
		roundOf[f.ID] = f.RoundStart
	}
	if roundOf[1] != "2026-01-05" || roundOf[2] != "2026-01-05" || roundOf[3] != "2026-02-01" {
		t.Errorf("round starts = %v", roundOf)
	}
	if got.Fills[0].ID != 1 || got.Fills[1].ID != 2 || got.Fills[2].ID != 3 {
		t.Errorf("fills must be oldest first: %+v", got.Fills)
	}
}
