package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/market"
)

// fakeFXDB implements fxRateStore with an in-memory date+pair map.
type fakeFXDB struct {
	rates map[string]float64
}

func fxKey(date, pair string) string { return date + "|" + pair }

func (f *fakeFXDB) GetFXRate(date, pair string) (float64, bool, error) {
	r, ok := f.rates[fxKey(date, pair)]
	return r, ok, nil
}
func (f *fakeFXDB) SaveFXRate(date, pair string, rate float64) error {
	if f.rates == nil {
		f.rates = map[string]float64{}
	}
	f.rates[fxKey(date, pair)] = rate
	return nil
}

func floatPtr(v float64) *float64 { return &v }

func TestWealthTotalsTWDOnlyNoFXNeeded(t *testing.T) {
	fake := &fakeDB{wealthAssets: []db.AssetWithValue{
		{Asset: db.Asset{Side: "asset", Type: "deposit", AssetGroup: "liquid", Currency: "TWD"}, Value: floatPtr(100000)},
		{Asset: db.Asset{Side: "liability", Type: "loan", AssetGroup: "hard", Currency: "TWD"}, Value: floatPtr(40000)},
	}}
	s := &Server{db: fake, quotes: &fakeQuotes{}, fxDB: &fakeFXDB{}}

	total, byGroup, ok := s.wealthTotals("2026-09-15", true)
	if !ok {
		t.Fatal("wealthTotals() ok = false, want true (TWD-only needs no FX)")
	}
	if total != 60000 {
		t.Errorf("total = %v, want 60000 (100000 asset - 40000 liability)", total)
	}
	if byGroup["liquid"] != 100000 || byGroup["hard"] != -40000 {
		t.Errorf("byGroup = %+v, want liquid=100000 hard=-40000", byGroup)
	}
}

func TestWealthTotalsUSDLiveFetchConvertsAndCaches(t *testing.T) {
	fake := &fakeDB{wealthAssets: []db.AssetWithValue{
		{Asset: db.Asset{Side: "asset", Type: "other", AssetGroup: "growth", Currency: "USD"}, Value: floatPtr(1000)},
	}}
	quotes := &fakeQuotes{quotes: map[string]*data.Quote{"USDTWD=X": {Price: 32}}}
	fxdb := &fakeFXDB{}
	s := &Server{db: fake, quotes: quotes, fxDB: fxdb}

	total, byGroup, ok := s.wealthTotals("2026-09-15", true)
	if !ok {
		t.Fatal("wealthTotals() ok = false, want true (live fetch should succeed)")
	}
	if total != 32000 {
		t.Errorf("total = %v, want 32000 (1000 USD * 32)", total)
	}
	if byGroup["growth"] != 32000 {
		t.Errorf("byGroup[growth] = %v, want 32000", byGroup["growth"])
	}
	if r, ok, _ := fxdb.GetFXRate("2026-09-15", "USDTWD"); !ok || r != 32 {
		t.Errorf("fx rate was not cached: got %v, %v", r, ok)
	}
}

// TestWealthTotalsHistoricalWithoutCachedRateDegrades pins §8.17.1's rule:
// a historical date (live=false) with no recorded fx_rates row must not
// silently drop the USD asset from the sum — the whole total degrades to
// "not ok" instead.
func TestWealthTotalsHistoricalWithoutCachedRateDegrades(t *testing.T) {
	fake := &fakeDB{wealthAssetsAsOf: map[string][]db.AssetWithValue{
		"2026-01-01": {{Asset: db.Asset{Side: "asset", AssetGroup: "growth", Currency: "USD"}, Value: floatPtr(1000)}},
	}}
	s := &Server{db: fake, quotes: &fakeQuotes{}, fxDB: &fakeFXDB{}}

	_, _, ok := s.wealthTotals("2026-01-01", false)
	if ok {
		t.Error("wealthTotals() ok = true, want false (no historical fx rate on record, must not fabricate)")
	}
}

func TestWealthTotalsUsesCachedHistoricalRate(t *testing.T) {
	fake := &fakeDB{wealthAssetsAsOf: map[string][]db.AssetWithValue{
		"2026-01-01": {{Asset: db.Asset{Side: "asset", AssetGroup: "growth", Currency: "USD"}, Value: floatPtr(1000)}},
	}}
	fxdb := &fakeFXDB{rates: map[string]float64{fxKey("2026-01-01", "USDTWD"): 30}}
	s := &Server{db: fake, quotes: &fakeQuotes{}, fxDB: fxdb}

	total, _, ok := s.wealthTotals("2026-01-01", false)
	if !ok {
		t.Fatal("wealthTotals() ok = false, want true (rate is on record)")
	}
	if total != 30000 {
		t.Errorf("total = %v, want 30000", total)
	}
}

func newWealthHomeTestServer(fake *fakeDB, quotes *fakeQuotes) *Server {
	s := &Server{db: fake, quotes: quotes, fxDB: &fakeFXDB{}}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("GET /api/wealth/networth", s.handleWealthHome)
	return s
}

func TestHandleWealthHomeNoHistoryRendersNilYTDMoM(t *testing.T) {
	fake := &fakeDB{wealthAssets: []db.AssetWithValue{
		{Asset: db.Asset{Side: "asset", AssetGroup: "liquid", Currency: "TWD"}, Value: floatPtr(100000)},
	}}
	s := newWealthHomeTestServer(fake, &fakeQuotes{})

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/networth", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	var got wealthHomeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.NetWorth == nil || *got.NetWorth != 100000 {
		t.Errorf("NetWorth = %v, want 100000", got.NetWorth)
	}
	if got.YTDPct != nil || got.MoMPct != nil {
		t.Errorf("YTDPct=%v MoMPct=%v, want both nil (no baseline snapshot exists)", got.YTDPct, got.MoMPct)
	}
	if got.Model != "balanced" {
		t.Errorf("Model = %q, want default %q", got.Model, "balanced")
	}
	if len(got.Allocation) != 4 {
		t.Errorf("len(Allocation) = %d, want 4 (one row per asset_group)", len(got.Allocation))
	}
}

func TestHandleWealthHomeUnknownModelDefaultsToBalanced(t *testing.T) {
	s := newWealthHomeTestServer(&fakeDB{}, &fakeQuotes{})
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/networth?model=nonsense", nil))
	var got wealthHomeResponse
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Model != "balanced" {
		t.Errorf("Model = %q, want balanced fallback", got.Model)
	}
}

// TestHandleWealthHomeEquityVirtualRow pins §9.1 rule 3: the trading
// account's own net worth is folded in as a synthetic "growth" entry via
// GetNetWorthOnOrBefore, never a row the assets table itself produced.
func TestHandleWealthHomeEquityVirtualRow(t *testing.T) {
	fake := &fakeDB{
		netWorthOnOrBeforeFn: func(date string, m market.MarketID) (float64, bool, error) {
			if m == market.TW {
				return 500000, true, nil
			}
			return 0, false, nil
		},
	}
	s := newWealthHomeTestServer(fake, &fakeQuotes{})

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/networth", nil))
	var got wealthHomeResponse
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got.NetWorth == nil || *got.NetWorth != 500000 {
		t.Errorf("NetWorth = %v, want 500000 (TW equity virtual row, no assets rows at all)", got.NetWorth)
	}
}
