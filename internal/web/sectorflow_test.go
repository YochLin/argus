package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/market"
)

// fakeSector implements data.SectorProvider for tests.
type fakeSector struct {
	info map[string]data.SectorInfo
}

func (f *fakeSector) GetSector(ticker string) (data.SectorInfo, error) {
	return f.info[ticker], nil
}

// fakeIndustryMap implements data.IndustryMapProvider for tests.
type fakeIndustryMap struct {
	m map[string]string
}

func (f *fakeIndustryMap) GetIndustryMap() (map[string]string, error) {
	return f.m, nil
}

func flowCandle(price float64, volume int64) data.Candle {
	return data.Candle{Open: price, High: price, Low: price, Close: price, Volume: volume}
}

// TestRunSectorFlowScanUS confirms per-ticker classification/history are
// aggregated to the sector level (summed, not averaged — see
// signals.TickerMoneyFlow's doc comment) and that a held position is
// reflected in both the sector's HeldCount and its item's Held flag.
func TestRunSectorFlowScanUS(t *testing.T) {
	orig := sectorFlowRequestDelay
	sectorFlowRequestDelay = 0
	defer func() { sectorFlowRequestDelay = orig }()

	s := &Server{
		db: &fakeDB{
			universe:  []db.UniverseEntry{{Ticker: "AAPL"}, {Ticker: "MSFT"}, {Ticker: "TSLA"}},
			positions: []db.Position{{Ticker: "AAPL"}},
		},
		sector: &fakeSector{info: map[string]data.SectorInfo{
			"AAPL": {Industry: "Tech", MarketCapMillion: 1000},
			"MSFT": {Industry: "Tech", MarketCapMillion: 2000},
			"TSLA": {Industry: "Auto", MarketCapMillion: 500},
		}},
		history: &fakeHistory{candles: map[string][]data.Candle{
			"AAPL": {flowCandle(100, 10), flowCandle(110, 20)}, // typical up -> posMF 110*20=2200
			"MSFT": {flowCandle(100, 15), flowCandle(90, 15)},  // typical down -> negMF 90*15=1350
			"TSLA": {flowCandle(50, 5), flowCandle(55, 5)},     // typical up -> posMF 55*5=275
		}},
		sectorFlow: newSectorFlowCache(),
	}

	s.RunSectorFlowScan(context.Background(), market.US)

	resp := s.sectorFlow.get(market.US, "1d")
	if !resp.Ready {
		t.Fatal("Ready = false, want true after a completed scan")
	}
	if len(resp.Sectors) != 2 {
		t.Fatalf("len(Sectors) = %d, want 2 (Tech, Auto)", len(resp.Sectors))
	}

	var tech, auto *SectorFlowSector
	for i := range resp.Sectors {
		switch resp.Sectors[i].Name {
		case "Tech":
			tech = &resp.Sectors[i]
		case "Auto":
			auto = &resp.Sectors[i]
		}
	}
	if tech == nil || auto == nil {
		t.Fatalf("missing sector, got %+v", resp.Sectors)
	}

	if want := 2200.0 - 1350.0; tech.NetFlow != want {
		t.Errorf("Tech.NetFlow = %v, want %v", tech.NetFlow, want)
	}
	if tech.TickerCount != 2 || tech.HeldCount != 1 {
		t.Errorf("Tech.TickerCount/HeldCount = %d/%d, want 2/1", tech.TickerCount, tech.HeldCount)
	}
	if tech.MarketCap != 3000 {
		t.Errorf("Tech.MarketCap = %v, want 3000", tech.MarketCap)
	}
	if want := 275.0; auto.NetFlow != want {
		t.Errorf("Auto.NetFlow = %v, want %v", auto.NetFlow, want)
	}
	if auto.TickerCount != 1 || auto.HeldCount != 0 {
		t.Errorf("Auto.TickerCount/HeldCount = %d/%d, want 1/0", auto.TickerCount, auto.HeldCount)
	}
}

// TestRunSectorFlowScanNoProviderSkips confirms a market with no
// classification source configured leaves the cache untouched (still not
// ready) rather than overwriting it with an empty result.
func TestRunSectorFlowScanNoProviderSkips(t *testing.T) {
	s := &Server{
		db:         &fakeDB{universe: []db.UniverseEntry{{Ticker: "AAPL"}}},
		sectorFlow: newSectorFlowCache(),
	}
	s.RunSectorFlowScan(context.Background(), market.US)

	resp := s.sectorFlow.get(market.US, "1d")
	if resp.Ready {
		t.Error("Ready = true, want false (no sector provider configured)")
	}
}

// TestHandleSectorFlow confirms the endpoint reads from the cache and
// defaults an invalid/absent tf to "1d".
func TestHandleSectorFlow(t *testing.T) {
	s := &Server{sectorFlow: newSectorFlowCache()}
	s.sectorFlow.set(market.US, map[string]SectorFlowResponse{
		"1d": {Ready: true, Sectors: []SectorFlowSector{{Name: "Tech", NetFlow: 42}}},
	})
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("GET /api/sectorflow", s.handleSectorFlow)

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/sectorflow?market=us&tf=bogus", nil))

	var resp SectorFlowResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp.Ready || len(resp.Sectors) != 1 || resp.Sectors[0].NetFlow != 42 {
		t.Errorf("resp = %+v, want the cached 1d snapshot (invalid tf should default to 1d)", resp)
	}
}

// TestHandleSectorFlowRefreshNoProvider confirms the manual-trigger endpoint
// reports a clear 503 instead of silently accepting a scan that would just
// no-op, for a market with no classification source configured.
func TestHandleSectorFlowRefreshNoProvider(t *testing.T) {
	s := &Server{sectorFlow: newSectorFlowCache()}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("POST /api/sectorflow/refresh", s.handleSectorFlowRefresh)

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/sectorflow/refresh?market=us", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

// TestHandleSectorFlowRefreshStartsScan confirms a configured market
// accepts the trigger (202) and the cache is populated once the background
// scan completes — the same tryStart/finish guard the scheduler's own runs
// go through, exercised here via the HTTP entry point instead.
func TestHandleSectorFlowRefreshStartsScan(t *testing.T) {
	orig := sectorFlowRequestDelay
	sectorFlowRequestDelay = 0
	defer func() { sectorFlowRequestDelay = orig }()

	s := &Server{
		db:         &fakeDB{universe: []db.UniverseEntry{{Ticker: "AAPL"}}},
		sector:     &fakeSector{info: map[string]data.SectorInfo{"AAPL": {Industry: "Tech", MarketCapMillion: 100}}},
		history:    &fakeHistory{candles: map[string][]data.Candle{"AAPL": {flowCandle(100, 10), flowCandle(110, 20)}}},
		sectorFlow: newSectorFlowCache(),
	}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("POST /api/sectorflow/refresh", s.handleSectorFlowRefresh)

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/sectorflow/refresh?market=us", nil))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}

	deadline := time.Now().Add(2 * time.Second)
	for !s.sectorFlow.get(market.US, "1d").Ready {
		if time.Now().After(deadline) {
			t.Fatal("scan did not populate the cache within 2s")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
