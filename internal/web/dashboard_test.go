package web

import (
	"errors"
	"testing"

	"argus/internal/data"
	"argus/internal/db"
)

// fakeDB implements dbReader for tests without touching real SQLite.
type fakeDB struct {
	positions []db.Position
	txs       []db.Transaction
	snapshots []db.DailySnapshot
	watchlist []string
	spy       db.DailySnapshot
	spyOK     bool

	snapshotsErr error
}

func (f *fakeDB) GetPositions() ([]db.Position, error)          { return f.positions, nil }
func (f *fakeDB) GetAllTransactions() ([]db.Transaction, error) { return f.txs, nil }
func (f *fakeDB) GetDailySnapshotsForTickers(tickers []string, from, to string) ([]db.DailySnapshot, error) {
	return f.snapshots, f.snapshotsErr
}
func (f *fakeDB) GetWatchlist() ([]string, error) { return f.watchlist, nil }
func (f *fakeDB) GetLatestSnapshot(ticker string) (db.DailySnapshot, bool, error) {
	if ticker == spyTicker {
		return f.spy, f.spyOK, nil
	}
	return db.DailySnapshot{}, false, nil
}

// fakeQuotes implements quoteGetter for tests.
type fakeQuotes struct {
	quotes map[string]*data.Quote
	err    map[string]error
}

func (f *fakeQuotes) GetQuote(ticker string) (*data.Quote, error) {
	if err, ok := f.err[ticker]; ok {
		return nil, err
	}
	return f.quotes[ticker], nil
}

func TestBuildDashboard_Empty(t *testing.T) {
	got, err := buildDashboard(&fakeDB{}, &fakeQuotes{})
	if err != nil {
		t.Fatalf("buildDashboard() error = %v", err)
	}
	if got.Curve == nil || len(got.Curve) != 0 {
		t.Errorf("Curve = %v, want empty non-nil slice", got.Curve)
	}
	if got.KPIs.NetPnL != 0 || got.KPIs.WinRate != 0 {
		t.Errorf("KPIs = %+v, want all zero", got.KPIs)
	}
	if len(got.Positions) != 0 {
		t.Errorf("Positions = %v, want empty", got.Positions)
	}
}

func TestBuildDashboard_PositionsAndKPIs(t *testing.T) {
	msftSell := tx("MSFT", "SELL", 2, 320, "2026-07-03")
	msftSell.RealizedPnL = 40 // (320-300)*2, as db.RecordSell would have computed and stored

	fdb := &fakeDB{
		positions: []db.Position{{Ticker: "AAPL", Shares: 10, AvgCost: 100}},
		txs: []db.Transaction{
			tx("AAPL", "BUY", 10, 100, "2026-07-01"),
			tx("AAPL", "BUY", 5, 100, "2026-07-01"), // partial fill same-day, same price
			tx("MSFT", "BUY", 2, 300, "2026-07-02"),
			msftSell,
		},
		snapshots: []db.DailySnapshot{
			snap("AAPL", "2026-07-01", 100),
			snap("MSFT", "2026-07-02", 300),
			snap("MSFT", "2026-07-03", 310),
		},
		watchlist: []string{"AAPL", "MSFT", "NVDA"},
		spy:       db.DailySnapshot{Date: "2026-07-15", ChangePercent: 0.42},
		spyOK:     true,
	}
	quotes := &fakeQuotes{quotes: map[string]*data.Quote{
		"AAPL": {Ticker: "AAPL", Price: 160},
	}}

	got, err := buildDashboard(fdb, quotes)
	if err != nil {
		t.Fatalf("buildDashboard() error = %v", err)
	}

	if len(got.Positions) != 1 {
		t.Fatalf("Positions = %+v, want 1", got.Positions)
	}
	p := got.Positions[0]
	if p.Ticker != "AAPL" || p.Price != 160 || p.MarketValue != 1600 || p.UnrealizedPnL != 600 {
		t.Errorf("Positions[0] = %+v, want price 160 / marketValue 1600 / unrealizedPnL 600", p)
	}

	// MSFT round trip realized_pnl = (320-300)*2 = 40, the only SELL on
	// record → WinRate 1, ProfitFactor 0 (no losses), Expectancy 40.
	if got.KPIs.WinRate != 1 {
		t.Errorf("WinRate = %v, want 1", got.KPIs.WinRate)
	}
	if got.KPIs.Expectancy != 40 {
		t.Errorf("Expectancy = %v, want 40", got.KPIs.Expectancy)
	}
}

func TestBuildStatus(t *testing.T) {
	fdb := &fakeDB{
		watchlist: []string{"AAPL", "MSFT", "NVDA"},
		spy:       db.DailySnapshot{Date: "2026-07-15", ChangePercent: 0.42},
		spyOK:     true,
	}

	got := buildStatus(fdb)

	if got.WatchingCount != 3 {
		t.Errorf("WatchingCount = %d, want 3", got.WatchingCount)
	}
	if got.SPYChangePct != 0.42 || got.LastCloseDate != "2026-07-15" {
		t.Errorf("Status = %+v, want SPY +0.42%% on 2026-07-15", got)
	}
}

func TestBuildDashboard_QuoteFailureDegradesGracefully(t *testing.T) {
	fdb := &fakeDB{positions: []db.Position{{Ticker: "AAPL", Shares: 10, AvgCost: 100}}}
	quotes := &fakeQuotes{err: map[string]error{"AAPL": errors.New("provider down")}}

	got, err := buildDashboard(fdb, quotes)
	if err != nil {
		t.Fatalf("buildDashboard() error = %v", err)
	}
	if len(got.Positions) != 1 {
		t.Fatalf("Positions = %+v, want the position still listed despite the quote failure", got.Positions)
	}
	if got.Positions[0].Price != 0 || got.Positions[0].Ticker != "AAPL" {
		t.Errorf("Positions[0] = %+v, want ticker/shares/avgCost populated and price left at 0", got.Positions[0])
	}
}
