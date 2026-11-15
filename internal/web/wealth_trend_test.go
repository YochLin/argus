package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"argus/internal/db"
)

func TestTrendDates(t *testing.T) {
	now := time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)
	cases := []struct {
		period, wantPeriod string
		first, last        string
		n                  int
	}{
		{"mtd", "mtd", "2026-10-31", "2026-11-15", 16},
		{"3m", "3m", "2026-08-16", "2026-11-15", 14},
		{"ytd", "ytd", "2026-01-01", "2026-11-15", 12},
		{"1y", "1y", "2025-11-15", "2026-11-15", 14},
		{"nonsense", "ytd", "2026-01-01", "2026-11-15", 12},
	}
	for _, c := range cases {
		period, got := trendDates(c.period, now)
		if period != c.wantPeriod {
			t.Errorf("%s: period = %q, want %q", c.period, period, c.wantPeriod)
		}
		if len(got) != c.n || got[0] != c.first || got[len(got)-1] != c.last {
			t.Errorf("%s: %d dates %v..%v, want %d %v..%v", c.period, len(got), got[0], got[len(got)-1], c.n, c.first, c.last)
		}
		for i := 1; i < len(got); i++ {
			if got[i] <= got[i-1] {
				t.Errorf("%s: dates not strictly increasing at %d: %v", c.period, i, got)
			}
		}
	}
}

func TestTrendDatesEdges(t *testing.T) {
	// 1 Jan: ytd's Jan-1 baseline and today are the same day, kept once.
	_, got := trendDates("ytd", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if !reflect.DeepEqual(got, []string{"2026-01-01"}) {
		t.Errorf("ytd on 1 Jan = %v", got)
	}
	// mtd in January reaches back across the year boundary.
	_, got = trendDates("mtd", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	if !reflect.DeepEqual(got, []string{"2025-12-31", "2026-01-01", "2026-01-02"}) {
		t.Errorf("mtd on 2 Jan = %v", got)
	}
}

// A USD asset on a day with no rate of its own borrows the newest one within
// the lookback; a day with nothing recorded is nil, not 0. (The lookback's
// cutoff itself is covered at the db layer.)
func TestHandleWealthTrendFXLookbackAndEmptyDays(t *testing.T) {
	now := time.Now()
	_, dates := trendDates("mtd", now)
	baseline, today := dates[0], dates[len(dates)-1]
	usd := func(v float64) []db.AssetWithValue {
		return []db.AssetWithValue{{Asset: db.Asset{Side: "asset", AssetGroup: "liquid", Currency: "USD"}, Value: floatPtr(v)}}
	}

	fake := &fakeDB{
		wealthAssets:     usd(100),
		wealthAssetsAsOf: map[string][]db.AssetWithValue{baseline: usd(100)},
	}
	fx := &fakeFXDB{rates: map[string]float64{
		fxKey(today, "USDTWD"): 30,
		// 3 days before the baseline: inside the 45-day lookback.
		fxKey(mustShift(baseline, -3), "USDTWD"): 31,
	}}
	s := &Server{db: fake, quotes: &fakeQuotes{}, fxDB: fx}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("GET /api/wealth/networth/trend", s.handleWealthTrend)

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/networth/trend?period=mtd", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got trendResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Period != "mtd" || len(got.Points) != len(dates) {
		t.Fatalf("period=%q points=%d, want mtd/%d", got.Period, len(got.Points), len(dates))
	}
	byDate := map[string]*float64{}
	for _, p := range got.Points {
		byDate[p.Date] = p.NetWorth
	}
	if v := byDate[today]; v == nil || *v != 3000 {
		t.Errorf("today = %v, want 3000 (100 USD x 30)", v)
	}
	if v := byDate[baseline]; v == nil || *v != 3100 {
		t.Errorf("baseline = %v, want 3100 (borrowed the rate from 3 days earlier)", v)
	}
	for _, d := range dates[1 : len(dates)-1] {
		if byDate[d] != nil {
			t.Errorf("%s = %v, want nil (nothing recorded that day)", d, *byDate[d])
		}
	}
}

func mustShift(date string, days int) string {
	t, err := time.Parse(dateLayout, date)
	if err != nil {
		panic(err)
	}
	return t.AddDate(0, 0, days).Format(dateLayout)
}
