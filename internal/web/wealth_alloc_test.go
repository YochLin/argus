package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/market"
)

func newWealthAllocTestServer(fake *fakeDB, quotes *fakeQuotes) *Server {
	s := &Server{db: fake, quotes: quotes, fxDB: &fakeFXDB{}}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("GET /api/wealth/alloc", s.handleWealthAlloc)
	return s
}

// TestHandleWealthAllocNoDataRendersEmpty pins §8.17.1: no priced assets
// means every total/list stays nil/empty, never a fabricated 0%.
func TestHandleWealthAllocNoDataRendersEmpty(t *testing.T) {
	s := newWealthAllocTestServer(&fakeDB{}, &fakeQuotes{})
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/alloc", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	var got allocResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.TotalAssets != nil {
		t.Errorf("TotalAssets = %v, want nil", got.TotalAssets)
	}
	if len(got.Allocation) != 0 || len(got.Orders) != 0 || len(got.Locked) != 0 {
		t.Errorf("got = %+v, want everything empty", got)
	}
	// Every slice field must serialize as [] not null — a nil Go slice and an
	// empty one round-trip identically through json.Unmarshal above, so that
	// assertion alone wouldn't catch a frontend `.length`-on-null crash the
	// way the raw body does. totalAssets/riskPct/riskTargetLow/riskTargetHigh
	// are legitimately nullable pointers, not lists — excluded on purpose.
	body := rec.Body.String()
	for _, field := range []string{"allocation", "orders", "locked", "currencyExposure", "concentration"} {
		if strings.Contains(body, `"`+field+`":null`) {
			t.Errorf("%s serialized as null, want []: %s", field, body)
		}
	}
}

// TestHandleWealthAllocOrdersAndLocked exercises the category-level math
// (nine-category taxonomy, §8.5): fund is over target and should sell,
// estate is over target too but locked so it only shows up as a locked
// row, never an order. Every other category sits exactly on target so it
// contributes neither an order nor drift noise to the assertions below.
// The equity category comes from the equity virtual row (net worth
// snapshot), same as production — never a real assets row (§9.1 rule 3).
func TestHandleWealthAllocOrdersAndLocked(t *testing.T) {
	fake := &fakeDB{
		wealthAssets: []db.AssetWithValue{
			{Asset: db.Asset{Side: "asset", Type: "deposit", Currency: "TWD"}, Value: floatPtr(150000)},           // cash 15%, on target
			{Asset: db.Asset{Side: "asset", Type: "fund", Currency: "TWD"}, Value: floatPtr(280000)},              // fund 28% vs target 15% -> sell
			{Asset: db.Asset{Side: "asset", Type: "bond", Currency: "TWD"}, Value: floatPtr(150000)},              // bond 15%, on target
			{Asset: db.Asset{Side: "asset", Type: "insurance", Currency: "TWD"}, Value: floatPtr(50000)},          // insurance 5%, on target
			{Asset: db.Asset{Side: "asset", Type: "estate", Name: "公寓", Currency: "TWD"}, Value: floatPtr(20000)}, // estate 2% vs target 15% -> locked, no order
			{Asset: db.Asset{Side: "asset", Type: "gold", Currency: "TWD"}, Value: floatPtr(50000)},               // gold 5%, on target
			{Asset: db.Asset{Side: "asset", Type: "crypto", Currency: "TWD"}, Value: floatPtr(30000)},             // crypto 3%, on target
			{Asset: db.Asset{Side: "asset", Type: "pension", Currency: "TWD"}, Value: floatPtr(20000)},            // pension 2%, on target
		},
		netWorthOnOrBeforeFn: func(date string, m market.MarketID) (float64, bool, error) {
			if m == market.TW {
				return 250000, true, nil // equity virtual row, 25% of the 1,000,000 total, on target
			}
			return 0, false, nil
		},
	}
	s := newWealthAllocTestServer(fake, &fakeQuotes{})

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/alloc?model=balanced", nil))
	var got allocResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Allocation) != 9 {
		t.Fatalf("len(Allocation) = %d, want 9", len(got.Allocation))
	}
	if len(got.Orders) != 1 || got.Orders[0].Category != "fund" || got.Orders[0].Side != "sell" {
		t.Errorf("Orders = %+v, want one sell order for fund", got.Orders)
	}
	// estate/gold/pension are all locked categories (assets.LockedCategories)
	// — they appear here regardless of deviation size, unlike orders.
	lockedCats := map[string]bool{}
	for _, l := range got.Locked {
		lockedCats[l.Category] = true
	}
	if len(got.Locked) != 3 || !lockedCats["estate"] || !lockedCats["gold"] || !lockedCats["pension"] {
		t.Errorf("Locked = %+v, want rows for estate/gold/pension", got.Locked)
	}
	if got.RiskPct == nil || !approxEqualFloat(*got.RiskPct, 56) {
		t.Errorf("RiskPct = %v, want 56 (equity 25 + fund 28 + crypto 3)", got.RiskPct)
	}
	if got.RiskTargetLow == nil || !approxEqualFloat(*got.RiskTargetLow, 38) || got.RiskTargetHigh == nil || !approxEqualFloat(*got.RiskTargetHigh, 48) {
		t.Errorf("RiskTargetLow/High = %v/%v, want 38/48", got.RiskTargetLow, got.RiskTargetHigh)
	}
	if len(got.CurrencyExposure) != 1 || got.CurrencyExposure[0].Currency != "TWD" || got.CurrencyExposure[0].Pct != 100 {
		t.Errorf("CurrencyExposure = %+v, want single TWD 100%%", got.CurrencyExposure)
	}
	// No open positions in this fixture, so Concentration stays empty — it
	// must still serialize as [], not null (the bug this pins: the frontend
	// calls .length on it unconditionally).
	if strings.Contains(rec.Body.String(), `"concentration":null`) {
		t.Errorf("concentration serialized as null, want []: %s", rec.Body.String())
	}
}

func approxEqualFloat(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 1e-6
}

// TestHandleWealthAllocConcentrationWarning pins §10.2③: a single position
// over ConcentrationThresholdPct of the combined position book is flagged.
func TestHandleWealthAllocConcentrationWarning(t *testing.T) {
	fake := &fakeDB{
		wealthAssets: []db.AssetWithValue{
			{Asset: db.Asset{Side: "asset", AssetGroup: "liquid", Currency: "TWD"}, Value: floatPtr(1000000)},
		},
		positions: []db.Position{
			{Ticker: "AAPL", Shares: 100, Market: "us"},
			{Ticker: "MSFT", Shares: 1, Market: "us"},
		},
	}
	quotes := &fakeQuotes{quotes: map[string]*data.Quote{
		"AAPL":     {Price: 200},
		"MSFT":     {Price: 1},
		"USDTWD=X": {Price: 32},
	}}
	s := newWealthAllocTestServer(fake, quotes)

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/alloc", nil))
	var got allocResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Concentration) != 1 || got.Concentration[0].Ticker != "AAPL" {
		t.Fatalf("Concentration = %+v, want one warning for AAPL", got.Concentration)
	}
	if got.Concentration[0].PctOfEquity < 99 {
		t.Errorf("PctOfEquity = %v, want ~100 (AAPL dwarfs MSFT)", got.Concentration[0].PctOfEquity)
	}
}
