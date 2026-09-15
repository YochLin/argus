package service

import (
	"testing"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/market"
)

// fakeWealthStore implements WealthStore over in-memory asset lists keyed
// by "live" vs a specific as-of date.
type fakeWealthStore struct {
	assets     []db.AssetWithValue
	assetsAsOf map[string][]db.AssetWithValue
	netWorth   map[market.MarketID]float64
	netWorthOK map[market.MarketID]bool
	settings   map[string]string
}

func (f *fakeWealthStore) ListAssetsWithValue(includeArchived bool) ([]db.AssetWithValue, error) {
	return f.assets, nil
}

func (f *fakeWealthStore) ListAssetsValueAsOf(asOfDate string, includeArchived bool) ([]db.AssetWithValue, error) {
	return f.assetsAsOf[asOfDate], nil
}

func (f *fakeWealthStore) GetNetWorthOnOrBefore(date string, m market.MarketID) (float64, bool, error) {
	return f.netWorth[m], f.netWorthOK[m], nil
}

func (f *fakeWealthStore) GetSetting(key string) (string, bool, error) {
	v, ok := f.settings[key]
	return v, ok, nil
}

type fakeWealthFX struct {
	rates map[string]float64
}

func fxKey(date, pair string) string { return date + "|" + pair }

func (f *fakeWealthFX) GetFXRate(date, pair string) (float64, bool, error) {
	r, ok := f.rates[fxKey(date, pair)]
	return r, ok, nil
}

func (f *fakeWealthFX) SaveFXRate(date, pair string, rate float64) error {
	if f.rates == nil {
		f.rates = map[string]float64{}
	}
	f.rates[fxKey(date, pair)] = rate
	return nil
}

type fakeWealthQuotes struct {
	quotes map[string]*data.Quote
}

func (f *fakeWealthQuotes) GetQuote(ticker string) (*data.Quote, error) {
	if q, ok := f.quotes[ticker]; ok {
		return q, nil
	}
	return nil, errNoQuote
}

var errNoQuote = &wealthTestError{"no quote"}

type wealthTestError struct{ msg string }

func (e *wealthTestError) Error() string { return e.msg }

func floatPtr(v float64) *float64 { return &v }

func TestWealthTotalsTWDOnlyNoFXNeeded(t *testing.T) {
	store := &fakeWealthStore{assets: []db.AssetWithValue{
		{Asset: db.Asset{Side: "asset", Type: "deposit", AssetGroup: "liquid", Currency: "TWD"}, Value: floatPtr(100000)},
		{Asset: db.Asset{Side: "liability", Type: "loan", AssetGroup: "hard", Currency: "TWD"}, Value: floatPtr(40000)},
	}}

	total, byGroup, ok := WealthTotals(store, &fakeWealthFX{}, &fakeWealthQuotes{}, "2026-09-15", true)
	if !ok {
		t.Fatal("WealthTotals() ok = false, want true (TWD-only needs no FX)")
	}
	if total != 60000 {
		t.Errorf("total = %v, want 60000 (100000 asset - 40000 liability)", total)
	}
	if byGroup["liquid"] != 100000 || byGroup["hard"] != -40000 {
		t.Errorf("byGroup = %+v, want liquid=100000 hard=-40000", byGroup)
	}
}

func TestWealthTotalsUSDLiveFetchConvertsAndCaches(t *testing.T) {
	store := &fakeWealthStore{assets: []db.AssetWithValue{
		{Asset: db.Asset{Side: "asset", Type: "other", AssetGroup: "growth", Currency: "USD"}, Value: floatPtr(1000)},
	}}
	quotes := &fakeWealthQuotes{quotes: map[string]*data.Quote{"USDTWD=X": {Price: 32}}}
	fxdb := &fakeWealthFX{}

	total, byGroup, ok := WealthTotals(store, fxdb, quotes, "2026-09-15", true)
	if !ok {
		t.Fatal("WealthTotals() ok = false, want true (live fetch should succeed)")
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
	store := &fakeWealthStore{assetsAsOf: map[string][]db.AssetWithValue{
		"2026-01-01": {{Asset: db.Asset{Side: "asset", AssetGroup: "growth", Currency: "USD"}, Value: floatPtr(1000)}},
	}}

	_, _, ok := WealthTotals(store, &fakeWealthFX{}, &fakeWealthQuotes{}, "2026-01-01", false)
	if ok {
		t.Error("WealthTotals() ok = true, want false (no historical fx rate on record, must not fabricate)")
	}
}

func TestWealthTotalsUsesCachedHistoricalRate(t *testing.T) {
	store := &fakeWealthStore{assetsAsOf: map[string][]db.AssetWithValue{
		"2026-01-01": {{Asset: db.Asset{Side: "asset", AssetGroup: "growth", Currency: "USD"}, Value: floatPtr(1000)}},
	}}
	fxdb := &fakeWealthFX{rates: map[string]float64{fxKey("2026-01-01", "USDTWD"): 30}}

	total, _, ok := WealthTotals(store, fxdb, &fakeWealthQuotes{}, "2026-01-01", false)
	if !ok {
		t.Fatal("WealthTotals() ok = false, want true (rate is on record)")
	}
	if total != 30000 {
		t.Errorf("total = %v, want 30000", total)
	}
}
