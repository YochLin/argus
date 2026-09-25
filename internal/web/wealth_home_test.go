package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
	if got.TotalAssets == nil || *got.TotalAssets != 100000 {
		t.Errorf("TotalAssets = %v, want 100000", got.TotalAssets)
	}
	if got.TotalLiabilities == nil || *got.TotalLiabilities != 0 {
		t.Errorf("TotalLiabilities = %v, want 0", got.TotalLiabilities)
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

// TestHandleWealthFX pins that only priceable currencies appear: USD has a
// cached rate today, the rest have no rate and no live quote, so they're
// omitted rather than guessed.
func TestHandleWealthFX(t *testing.T) {
	today := time.Now().Format("2006-01-02")
	noQuote := errors.New("no quote")
	s := &Server{
		quotes: &fakeQuotes{err: map[string]error{"JPYTWD=X": noQuote, "EURTWD=X": noQuote, "CNYTWD=X": noQuote}},
		fxDB:   &fakeFXDB{rates: map[string]float64{fxKey(today, "USDTWD"): 31.8}},
	}
	rec := httptest.NewRecorder()
	s.handleWealthFX(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/fx", nil))
	var got struct {
		Rates map[string]float64 `json:"rates"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Rates) != 1 || got.Rates["USD"] != 31.8 {
		t.Errorf("rates = %v, want only USD=31.8", got.Rates)
	}
}
