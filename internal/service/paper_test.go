package service

import (
	"testing"

	"argus/internal/db"
	"argus/internal/llm"
	"argus/internal/market"
	"argus/internal/paper"
)

// mockPaperStore implements PaperStore over in-memory maps — real weighted-
// avg-cost/realized-P&L arithmetic on RecordBuy/RecordSell is internal/db's
// own job to test; this just needs to behave consistently enough for
// PaperService's load/apply/persist cycle to round-trip.
type mockPaperStore struct {
	positions   map[string]db.Position
	buyDates    map[string]string
	peakCloses  map[string]float64
	settings    map[string]string
	realizedPnL map[market.MarketID]float64
	snapshots   []db.DailySnapshot
	netWorth    map[string]float64 // date+market -> total
	resetCalled bool
}

func newMockPaperStore() *mockPaperStore {
	return &mockPaperStore{
		positions:   map[string]db.Position{},
		buyDates:    map[string]string{},
		peakCloses:  map[string]float64{},
		settings:    map[string]string{},
		realizedPnL: map[market.MarketID]float64{},
		netWorth:    map[string]float64{},
	}
}

func (s *mockPaperStore) GetPositions() ([]db.Position, error) {
	var out []db.Position
	for _, p := range s.positions {
		out = append(out, p)
	}
	return out, nil
}

func (s *mockPaperStore) GetEarliestBuyDate(ticker string) (string, bool, error) {
	d, ok := s.buyDates[ticker]
	return d, ok, nil
}

func (s *mockPaperStore) GetPeakClose(ticker, sinceDate string) (float64, bool, error) {
	v, ok := s.peakCloses[ticker]
	return v, ok, nil
}

func (s *mockPaperStore) GetSetting(key string) (string, bool, error) {
	v, ok := s.settings[key]
	return v, ok, nil
}

func (s *mockPaperStore) SetSetting(key, value string) error {
	s.settings[key] = value
	return nil
}

func (s *mockPaperStore) RecordBuy(ticker string, shares, price, fee float64, date string) (db.Position, error) {
	if _, ok := s.buyDates[ticker]; !ok {
		s.buyDates[ticker] = date
	}
	pos := s.positions[ticker]
	pos.Ticker = ticker
	totalCost := pos.AvgCost*pos.Shares + price*shares
	pos.Shares += shares
	pos.AvgCost = totalCost / pos.Shares
	s.positions[ticker] = pos
	return pos, nil
}

func (s *mockPaperStore) SetStopPrice(ticker string, price float64) error {
	pos := s.positions[ticker]
	pos.StopPrice = price
	s.positions[ticker] = pos
	return nil
}

func (s *mockPaperStore) RecordSell(ticker string, shares, price, fee float64, date string) (db.Position, float64, error) {
	pos := s.positions[ticker]
	realizedPnL := (price-pos.AvgCost)*shares - fee
	s.realizedPnL[market.Of(ticker)] += realizedPnL
	pos.Shares -= shares
	if pos.Shares <= 1e-9 {
		delete(s.positions, ticker)
		delete(s.buyDates, ticker)
		return db.Position{}, realizedPnL, nil
	}
	s.positions[ticker] = pos
	return pos, realizedPnL, nil
}

func (s *mockPaperStore) SaveSnapshot(snap db.DailySnapshot) error {
	s.snapshots = append(s.snapshots, snap)
	return nil
}

func (s *mockPaperStore) SaveNetWorthSnapshot(date string, m market.MarketID, total float64) error {
	s.netWorth[date+string(m)] = total
	return nil
}

func (s *mockPaperStore) GetRealizedPnL(m market.MarketID) (float64, error) {
	return s.realizedPnL[m], nil
}

func (s *mockPaperStore) ResetTradingData() error {
	s.resetCalled = true
	s.positions = map[string]db.Position{}
	s.settings = map[string]string{}
	return nil
}

func testPaperConfig() paper.Config {
	return paper.Config{RiskPct: 1.0, MaxPositionPct: 25, StopATRMult: 2.0, Market: market.US}
}

func TestPaperServiceLoadAccountSeedsInitialCash(t *testing.T) {
	store := newMockPaperStore()
	s := NewPaperService(store, nil, 100000, 1000000, 0)

	acct, err := s.LoadAccount(market.US)
	if err != nil {
		t.Fatalf("LoadAccount() error = %v", err)
	}
	if acct.Cash != 100000 {
		t.Errorf("Cash = %v, want 100000 (seeded)", acct.Cash)
	}
	if len(acct.Holdings) != 0 {
		t.Errorf("Holdings = %+v, want empty on a fresh account", acct.Holdings)
	}
}

func TestPaperServiceApplyTradesThenRunCloseRoundTrip(t *testing.T) {
	store := newMockPaperStore()
	s := NewPaperService(store, nil, 100000, 1000000, 0)
	cfg := testPaperConfig()

	results, err := s.ApplyTrades(
		[]llm.Recommendation{{Ticker: "AAPL", Action: "BUY"}},
		map[string]float64{"AAPL": 150}, map[string]float64{"AAPL": 3},
		market.US, cfg, "2024-01-02")
	if err != nil {
		t.Fatalf("ApplyTrades() error = %v", err)
	}
	if len(results) != 1 || results[0].Trade.Side != "BUY" {
		t.Fatalf("ApplyTrades() results = %+v, want exactly one BUY", results)
	}
	stop := results[0].Trade.Stop

	reloaded, err := s.LoadAccount(market.US)
	if err != nil {
		t.Fatalf("LoadAccount() error = %v", err)
	}
	h, held := reloaded.Holdings["AAPL"]
	if !held {
		t.Fatalf("reloaded account is missing the AAPL holding")
	}
	if h.AvgCost != 150 {
		t.Errorf("AvgCost = %v, want 150", h.AvgCost)
	}
	if h.Stop != stop {
		t.Errorf("Stop = %v, want %v (from paper.db's stop_price column)", h.Stop, stop)
	}

	closeResults, ran, err := s.RunClose(market.US, "2024-01-03", map[string]float64{"AAPL": stop - 1}, func(string) (float64, bool) { return 0, false }, cfg)
	if err != nil {
		t.Fatalf("RunClose() error = %v", err)
	}
	if !ran {
		t.Fatal("RunClose() ran = false, want true (holdings were non-empty)")
	}
	if len(closeResults) != 1 || closeResults[0].Trade.Reason != "stop" {
		t.Fatalf("RunClose() results = %+v, want exactly one stop-loss exit", closeResults)
	}

	final, err := s.LoadAccount(market.US)
	if err != nil {
		t.Fatalf("final LoadAccount() error = %v", err)
	}
	if _, held := final.Holdings["AAPL"]; held {
		t.Error("final account still shows AAPL held after the stop-loss exit")
	}
}

func TestPaperServiceRunCloseSkipsWhenNoHoldings(t *testing.T) {
	store := newMockPaperStore()
	s := NewPaperService(store, nil, 100000, 1000000, 0)

	results, ran, err := s.RunClose(market.US, "2024-01-03", nil, func(string) (float64, bool) { return 0, false }, testPaperConfig())
	if err != nil {
		t.Fatalf("RunClose() error = %v", err)
	}
	if ran || results != nil {
		t.Errorf("RunClose() = (%+v, %v), want (nil, false) for an empty account", results, ran)
	}
	if len(store.snapshots) != 0 {
		t.Errorf("snapshots = %+v, want none written when there are no holdings", store.snapshots)
	}
}

func TestPaperServiceReset(t *testing.T) {
	store := newMockPaperStore()
	s := NewPaperService(store, nil, 100000, 1000000, 0)
	if err := s.Reset(); err != nil {
		t.Fatalf("Reset() error = %v", err)
	}
	if !store.resetCalled {
		t.Error("Reset() did not call the store's ResetTradingData")
	}
}
