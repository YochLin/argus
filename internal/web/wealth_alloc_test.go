package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"argus/internal/data"
	"argus/internal/db"
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

// TestHandleWealthAllocOrdersAndLocked exercises the group-level math:
// growth is over target and should sell, hard is over target too but
// locked so it only shows up as a locked row, never an order.
func TestHandleWealthAllocOrdersAndLocked(t *testing.T) {
	fake := &fakeDB{wealthAssets: []db.AssetWithValue{
		{Asset: db.Asset{Side: "asset", AssetGroup: "liquid", Currency: "TWD"}, Value: floatPtr(200000)},
		{Asset: db.Asset{Side: "asset", AssetGroup: "growth", Currency: "TWD"}, Value: floatPtr(500000)},
		{Asset: db.Asset{Side: "asset", AssetGroup: "income", Currency: "TWD"}, Value: floatPtr(200000)},
		{Asset: db.Asset{Side: "asset", AssetGroup: "hard", Name: "公寓", Currency: "TWD"}, Value: floatPtr(100000)},
	}}
	s := newWealthAllocTestServer(fake, &fakeQuotes{})

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/alloc?model=balanced", nil))
	var got allocResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Allocation) != 4 {
		t.Fatalf("len(Allocation) = %d, want 4", len(got.Allocation))
	}
	if len(got.Orders) != 1 || got.Orders[0].Group != "growth" || got.Orders[0].Side != "sell" {
		t.Errorf("Orders = %+v, want one sell order for growth", got.Orders)
	}
	if len(got.Locked) != 1 || got.Locked[0].Group != "hard" {
		t.Errorf("Locked = %+v, want one row for hard", got.Locked)
	}
	if got.RiskPct == nil || *got.RiskPct != 50 {
		t.Errorf("RiskPct = %v, want 50 (growth's current%%)", got.RiskPct)
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
