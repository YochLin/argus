package web

import (
	"testing"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/market"
)

func TestBuildWatchlistSummary(t *testing.T) {
	fdb := &fakeDB{
		watchlist: []string{"AAPL", "MSFT"},
		positions: []db.Position{
			{Ticker: "AAPL", Shares: 10, AvgCost: 150},
			{Ticker: "NVDA", Shares: 5, AvgCost: 200},
		},
	}
	quotes := &fakeQuotes{
		quotes: map[string]*data.Quote{
			"AAPL": {Ticker: "AAPL", Price: 175, ChangePercent: 2.5},
			"MSFT": {Ticker: "MSFT", Price: 300, ChangePercent: -1.2},
			"NVDA": {Ticker: "NVDA", Price: 450, ChangePercent: 0.5},
		},
	}
	candles := levelBaseCandlesForTest(40)
	hist := &fakeHistory{
		candles: map[string][]data.Candle{
			"AAPL": candles,
			"MSFT": candles,
			"NVDA": candles,
		},
	}

	got, err := buildWatchlistSummary(fdb, quotes, hist, market.US)
	if err != nil {
		t.Fatalf("buildWatchlistSummary() error = %v", err)
	}

	if len(got.Tickers) != 3 {
		t.Fatalf("len(Tickers) = %d, want 3", len(got.Tickers))
	}

	// Tickers sorted: AAPL, MSFT, NVDA
	aapl := got.Tickers[0]
	if aapl.Ticker != "AAPL" || aapl.Price != 175 || aapl.ChangePct != 2.5 || aapl.HeldShares != 10 {
		t.Errorf("AAPL summary mismatch = %+v", aapl)
	}
	if len(aapl.Sparkline) != 40 {
		t.Errorf("AAPL sparkline len = %d, want 40", len(aapl.Sparkline))
	}

	msft := got.Tickers[1]
	if msft.Ticker != "MSFT" || msft.HeldShares != 0 {
		t.Errorf("MSFT summary mismatch = %+v", msft)
	}
}
