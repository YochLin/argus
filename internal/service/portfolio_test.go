package service

import (
	"errors"
	"testing"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/market"
)

type fakePortfolioStore struct {
	positions []db.Position
	settings  map[string]string
	realized  float64
}

func (f *fakePortfolioStore) GetPositions() ([]db.Position, error)            { return f.positions, nil }
func (f *fakePortfolioStore) GetRealizedPnL(market.MarketID) (float64, error) { return f.realized, nil }
func (f *fakePortfolioStore) GetSetting(key string) (string, bool, error) {
	v, ok := f.settings[key]
	return v, ok, nil
}

type fakePortfolioQuotes struct {
	quotes map[string]*data.Quote
	errors map[string]error
}

func (f *fakePortfolioQuotes) GetQuote(ticker string) (*data.Quote, error) {
	if err := f.errors[ticker]; err != nil {
		return nil, err
	}
	return f.quotes[ticker], nil
}

func TestPortfolioServiceSnapshotScopesAndValues(t *testing.T) {
	store := &fakePortfolioStore{
		positions: []db.Position{
			{Ticker: "AAPL", Shares: 10, AvgCost: 100},
			{Ticker: "2330", Shares: 1000, AvgCost: 500},
		},
		settings: map[string]string{CashSettingKeyUSD: "1000"},
		realized: 250,
	}
	quotes := &fakePortfolioQuotes{quotes: map[string]*data.Quote{
		"AAPL": {Ticker: "AAPL", Price: 120},
	}}
	svc := NewPortfolioService(store, quotes)

	snapshot, err := svc.Snapshot(market.US)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if len(snapshot.Positions) != 1 || snapshot.Positions[0].Position.Ticker != "AAPL" {
		t.Fatalf("Snapshot().Positions = %+v, want only AAPL", snapshot.Positions)
	}
	if snapshot.TotalMarketValue != 1200 || snapshot.Positions[0].UnrealizedPnL != 200 || snapshot.Positions[0].UnrealizedPnLPct != 20 {
		t.Errorf("valuation = %+v, want value=1200 pnl=200 pct=20", snapshot)
	}
	if snapshot.RealizedPnL != 250 || !snapshot.HasCash || snapshot.AccountValue != 2200 {
		t.Errorf("account summary = %+v, want realized=250 cash=1000 account=2200", snapshot)
	}
}

func TestPortfolioServiceRetainsQuoteAndCashErrors(t *testing.T) {
	store := &fakePortfolioStore{
		positions: []db.Position{{Ticker: "AAPL", Shares: 1, AvgCost: 100}},
		settings:  map[string]string{CashSettingKeyUSD: "not-a-number"},
	}
	quotes := &fakePortfolioQuotes{errors: map[string]error{"AAPL": errors.New("quote unavailable")}}
	snapshot, err := NewPortfolioService(store, quotes).Snapshot(market.US)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if snapshot.Positions[0].QuoteErr == nil || snapshot.CashErr == nil || snapshot.HasCash {
		t.Errorf("Snapshot() errors = %+v, want quote/cash errors and no cash", snapshot)
	}
}

type fakeSettings struct {
	values map[string]string
	err    error
}

func (f fakeSettings) GetSetting(key string) (string, bool, error) {
	v, ok := f.values[key]
	return v, ok, f.err
}

func TestLoadCash(t *testing.T) {
	settings := fakeSettings{values: map[string]string{
		CashSettingKeyUSD: "1234.50",
		CashSettingKeyTWD: "not a number",
	}}

	if got, ok, err := LoadCash(settings, market.US); got != 1234.5 || !ok || err != nil {
		t.Errorf("LoadCash(US) = %v, %v, %v; want 1234.5, true, nil", got, ok, err)
	}
	if got, ok, err := LoadCash(settings, market.TW); got != 0 || ok || err == nil {
		t.Errorf("LoadCash(TW) with an unparseable value = %v, %v, %v; want 0, false, error", got, ok, err)
	}
	if got, ok, err := LoadCash(fakeSettings{}, market.US); got != 0 || ok || err != nil {
		t.Errorf("LoadCash() with no declared balance = %v, %v, %v; want 0, false, nil", got, ok, err)
	}
	boom := errors.New("db down")
	if _, ok, err := LoadCash(fakeSettings{err: boom}, market.US); ok || !errors.Is(err, boom) {
		t.Errorf("LoadCash() with a store error = ok %v, err %v; want false, the store error", ok, err)
	}
}
