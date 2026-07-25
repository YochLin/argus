package web

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/market"
)

// approxEqual guards against Go's untyped-constant-vs-runtime-float64
// rounding mismatch: a "want" value built from constant expressions is
// computed at arbitrary precision until assigned, while the code under test
// does the same arithmetic through actual float64 variables — the two can
// differ in the last bit even when the formula is identical.
func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-6
}

func TestBuildRisk_HeatOnlyCountsStopsSet(t *testing.T) {
	// NVDA has a stop set and is up: openRisk = (135.2-118)*10 = 172.
	// MSFT has no stop set at all: excluded from the heat sum, openRisk nil.
	fdb := &fakeDB{
		positions: []db.Position{
			{Ticker: "NVDA", Shares: 10, AvgCost: 120.5, StopPrice: 118},
			{Ticker: "MSFT", Shares: 5, AvgCost: 300},
		},
		settings: map[string]string{cashSettingKey: "5000"},
	}
	quotes := &fakeQuotes{quotes: map[string]*data.Quote{
		"NVDA": {Ticker: "NVDA", Price: 135.2},
		"MSFT": {Ticker: "MSFT", Price: 310},
	}}

	got, err := buildRisk(fdb, quotes, market.US, 6)
	if err != nil {
		t.Fatalf("buildRisk() error = %v", err)
	}

	wantAccountValue := 135.2*10 + 310*5 + 5000
	if !approxEqual(got.AccountValue, wantAccountValue) {
		t.Errorf("AccountValue = %v, want %v", got.AccountValue, wantAccountValue)
	}
	if got.Cash != 5000 {
		t.Errorf("Cash = %v, want 5000", got.Cash)
	}
	if got.HeatThresholdPct != 6 {
		t.Errorf("HeatThresholdPct = %v, want 6", got.HeatThresholdPct)
	}

	var nvda, msft riskPositionResponse
	for _, p := range got.Positions {
		switch p.Ticker {
		case "NVDA":
			nvda = p
		case "MSFT":
			msft = p
		}
	}

	wantOpenRisk := (135.2 - 118) * 10
	if nvda.OpenRisk == nil || !approxEqual(*nvda.OpenRisk, wantOpenRisk) {
		t.Errorf("NVDA OpenRisk = %v, want %v", nvda.OpenRisk, wantOpenRisk)
	}
	wantHeatPct := wantOpenRisk / wantAccountValue * 100
	if !approxEqual(got.HeatPct, wantHeatPct) {
		t.Errorf("HeatPct = %v, want %v (only NVDA's open risk, MSFT excluded)", got.HeatPct, wantHeatPct)
	}

	if msft.OpenRisk != nil {
		t.Errorf("MSFT OpenRisk = %v, want nil (no stop set)", *msft.OpenRisk)
	}
	if msft.OpenRiskPct != nil {
		t.Errorf("MSFT OpenRiskPct = %v, want nil (no stop set)", *msft.OpenRiskPct)
	}
}

func TestBuildRisk_BelowStopClampsToZero(t *testing.T) {
	// Price has already fallen through the stop — open risk is realized/
	// unrealized loss now, not "open" risk, so it clamps to 0 rather than
	// going negative.
	fdb := &fakeDB{
		positions: []db.Position{{Ticker: "TSLA", Shares: 10, AvgCost: 200, StopPrice: 180}},
	}
	quotes := &fakeQuotes{quotes: map[string]*data.Quote{
		"TSLA": {Ticker: "TSLA", Price: 170},
	}}

	got, err := buildRisk(fdb, quotes, market.US, 6)
	if err != nil {
		t.Fatalf("buildRisk() error = %v", err)
	}
	if len(got.Positions) != 1 {
		t.Fatalf("Positions = %+v, want 1", got.Positions)
	}
	p := got.Positions[0]
	if p.OpenRisk == nil || *p.OpenRisk != 0 {
		t.Errorf("OpenRisk = %v, want 0 (price below stop)", p.OpenRisk)
	}
	if got.HeatPct != 0 {
		t.Errorf("HeatPct = %v, want 0", got.HeatPct)
	}
}

func TestBuildRisk_MarketFilter(t *testing.T) {
	fdb := &fakeDB{
		positions: []db.Position{
			{Ticker: "AAPL", Shares: 10, AvgCost: 150},
			{Ticker: "2330", Shares: 1000, AvgCost: 600},
		},
	}
	quotes := &fakeQuotes{quotes: map[string]*data.Quote{
		"AAPL": {Ticker: "AAPL", Price: 160},
		"2330": {Ticker: "2330", Price: 620},
	}}

	gotUS, err := buildRisk(fdb, quotes, market.US, 6)
	if err != nil {
		t.Fatalf("buildRisk(US) error = %v", err)
	}
	if len(gotUS.Positions) != 1 || gotUS.Positions[0].Ticker != "AAPL" {
		t.Errorf("buildRisk(US) Positions = %+v, want just AAPL", gotUS.Positions)
	}

	gotTW, err := buildRisk(fdb, quotes, market.TW, 6)
	if err != nil {
		t.Fatalf("buildRisk(TW) error = %v", err)
	}
	if len(gotTW.Positions) != 1 || gotTW.Positions[0].Ticker != "2330" {
		t.Errorf("buildRisk(TW) Positions = %+v, want just 2330", gotTW.Positions)
	}
}

func TestBuildRisk_QuoteFailureDegradesToNilRisk(t *testing.T) {
	fdb := &fakeDB{
		positions: []db.Position{{Ticker: "AAPL", Shares: 10, AvgCost: 150, StopPrice: 140}},
	}
	quotes := &fakeQuotes{err: map[string]error{"AAPL": errors.New("boom")}}

	got, err := buildRisk(fdb, quotes, market.US, 6)
	if err != nil {
		t.Fatalf("buildRisk() error = %v", err)
	}
	if len(got.Positions) != 1 {
		t.Fatalf("Positions = %+v, want 1", got.Positions)
	}
	p := got.Positions[0]
	if p.OpenRisk != nil {
		t.Errorf("OpenRisk = %v, want nil (quote fetch failed, price unknown)", *p.OpenRisk)
	}
	if got.AccountValue != 0 {
		t.Errorf("AccountValue = %v, want 0 (no cash set, quote failed)", got.AccountValue)
	}
}

func TestBuildRisk_Empty(t *testing.T) {
	got, err := buildRisk(&fakeDB{}, &fakeQuotes{}, market.US, 6)
	if err != nil {
		t.Fatalf("buildRisk() error = %v", err)
	}
	if got.Positions == nil || len(got.Positions) != 0 {
		t.Errorf("Positions = %v, want empty non-nil slice", got.Positions)
	}
	if got.HeatPct != 0 || got.AccountValue != 0 {
		t.Errorf("got = %+v, want zero HeatPct/AccountValue", got)
	}
}

func TestHandleRisk_DefaultsToUSMarket(t *testing.T) {
	s := testServer()
	s.db = &fakeDB{positions: []db.Position{{Ticker: "2330", Shares: 1000, AvgCost: 600}}}
	s.heatThresholdPct = 6
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("GET /api/risk", s.handleRisk)

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/risk", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	var got riskResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Positions) != 0 {
		t.Errorf("Positions = %+v, want empty (2330 is TW, endpoint defaults to US)", got.Positions)
	}
}
