package web

import (
	"errors"
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/market"
)

// fakeDB implements dbReader for tests without touching real SQLite.
type fakeDB struct {
	positions []db.Position
	txs       []db.Transaction
	snapshots []db.DailySnapshot
	watchlist []string
	spy       db.DailySnapshot
	spyOK     bool
	twBench   db.DailySnapshot
	twBenchOK bool

	snapshotsErr error

	// netWorthOnOrBeforeFn/netWorthRangeFn are optional overrides for the
	// period-return tests below — nil (the zero value every other test
	// leaves them at) behaves as "no net worth data on record."
	netWorthOnOrBeforeFn func(date string, m market.MarketID) (float64, bool, error)
	netWorthRangeFn      func(from, to string, m market.MarketID) ([]db.NetWorthPoint, error)
}

func (f *fakeDB) GetPositions() ([]db.Position, error)          { return f.positions, nil }
func (f *fakeDB) GetAllTransactions() ([]db.Transaction, error) { return f.txs, nil }
func (f *fakeDB) GetDailySnapshotsForTickers(tickers []string, from, to string) ([]db.DailySnapshot, error) {
	return f.snapshots, f.snapshotsErr
}
func (f *fakeDB) GetWatchlist() ([]string, error) { return f.watchlist, nil }
func (f *fakeDB) GetLatestSnapshot(ticker string) (db.DailySnapshot, bool, error) {
	switch ticker {
	case spyTicker:
		return f.spy, f.spyOK, nil
	case twBenchmarkTicker:
		return f.twBench, f.twBenchOK, nil
	}
	return db.DailySnapshot{}, false, nil
}
func (f *fakeDB) GetNetWorthOnOrBefore(date string, m market.MarketID) (float64, bool, error) {
	if f.netWorthOnOrBeforeFn != nil {
		return f.netWorthOnOrBeforeFn(date, m)
	}
	return 0, false, nil
}
func (f *fakeDB) GetNetWorthRange(from, to string, m market.MarketID) ([]db.NetWorthPoint, error) {
	if f.netWorthRangeFn != nil {
		return f.netWorthRangeFn(from, to, m)
	}
	return nil, nil
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
	got, err := buildDashboard(&fakeDB{}, &fakeQuotes{}, market.US)
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

	got, err := buildDashboard(fdb, quotes, market.US)
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

	got := buildStatus(fdb, market.US)

	if got.WatchingCount != 3 {
		t.Errorf("WatchingCount = %d, want 3", got.WatchingCount)
	}
	if got.SPYChangePct != 0.42 || got.LastCloseDate != "2026-07-15" {
		t.Errorf("Status = %+v, want SPY +0.42%% on 2026-07-15", got)
	}
}

// TestBuildDashboard_MarketFilter is Phase 6's core correctness requirement
// for the web dashboard (docs/phase-6-tw-market.md §4.4): a TWD position/
// transaction must never leak into a market.US-scoped response (or vice
// versa) — mixing currencies into one KPI/curve would silently produce a
// meaningless number, not just a display glitch.
func TestBuildDashboard_MarketFilter(t *testing.T) {
	fdb := &fakeDB{
		positions: []db.Position{
			{Ticker: "AAPL", Shares: 10, AvgCost: 100},
			{Ticker: "2330", Shares: 1000, AvgCost: 900},
		},
		txs: []db.Transaction{
			tx("AAPL", "BUY", 10, 100, "2026-07-01"),
			tx("2330", "BUY", 1000, 900, "2026-07-01"),
		},
	}
	quotes := &fakeQuotes{quotes: map[string]*data.Quote{
		"AAPL": {Ticker: "AAPL", Price: 160},
		"2330": {Ticker: "2330", Price: 950},
	}}

	us, err := buildDashboard(fdb, quotes, market.US)
	if err != nil {
		t.Fatalf("buildDashboard(US) error = %v", err)
	}
	if len(us.Positions) != 1 || us.Positions[0].Ticker != "AAPL" {
		t.Errorf("buildDashboard(US) Positions = %+v, want only AAPL", us.Positions)
	}

	tw, err := buildDashboard(fdb, quotes, market.TW)
	if err != nil {
		t.Fatalf("buildDashboard(TW) error = %v", err)
	}
	if len(tw.Positions) != 1 || tw.Positions[0].Ticker != "2330" {
		t.Errorf("buildDashboard(TW) Positions = %+v, want only 2330", tw.Positions)
	}
}

func TestBuildStatus_MarketFilter(t *testing.T) {
	fdb := &fakeDB{
		watchlist: []string{"AAPL", "2330", "0050"},
		spy:       db.DailySnapshot{Date: "2026-07-15", ChangePercent: 0.42},
		spyOK:     true,
		twBench:   db.DailySnapshot{Date: "2026-07-15", ChangePercent: 1.1},
		twBenchOK: true,
	}

	us := buildStatus(fdb, market.US)
	if us.WatchingCount != 1 {
		t.Errorf("buildStatus(US) WatchingCount = %d, want 1 (AAPL only)", us.WatchingCount)
	}
	if us.SPYChangePct != 0.42 {
		t.Errorf("buildStatus(US) SPYChangePct = %v, want 0.42", us.SPYChangePct)
	}

	tw := buildStatus(fdb, market.TW)
	if tw.WatchingCount != 2 {
		t.Errorf("buildStatus(TW) WatchingCount = %d, want 2 (2330, 0050)", tw.WatchingCount)
	}
	if tw.SPYChangePct != 1.1 {
		t.Errorf("buildStatus(TW) SPYChangePct = %v, want 1.1 (0050's, not SPY's)", tw.SPYChangePct)
	}
}

func TestNetWorthBaseline(t *testing.T) {
	t.Run("uses on-or-before baseline", func(t *testing.T) {
		fdb := &fakeDB{
			netWorthOnOrBeforeFn: func(date string, m market.MarketID) (float64, bool, error) {
				if date == "2025-12-31" && m == market.US {
					return 1000, true, nil
				}
				return 0, false, nil
			},
		}
		base, ok := netWorthBaseline(fdb, "2026-01-01", market.US, "2026-07-15")
		if !ok || base != 1000 {
			t.Errorf("netWorthBaseline() = (%v, %v), want (1000, true)", base, ok)
		}
	})

	t.Run("falls back to earliest in-range point when no prior snapshot", func(t *testing.T) {
		fdb := &fakeDB{
			netWorthRangeFn: func(from, to string, m market.MarketID) ([]db.NetWorthPoint, error) {
				if from == "2026-01-01" && to == "2026-07-15" && m == market.US {
					return []db.NetWorthPoint{{Date: "2026-03-10", Total: 800}, {Date: "2026-04-01", Total: 850}}, nil
				}
				return nil, nil
			},
		}
		base, ok := netWorthBaseline(fdb, "2026-01-01", market.US, "2026-07-15")
		if !ok || base != 800 {
			t.Errorf("netWorthBaseline() = (%v, %v), want (800, true) (earliest point in range)", base, ok)
		}
	})

	t.Run("no data at all", func(t *testing.T) {
		base, ok := netWorthBaseline(&fakeDB{}, "2026-01-01", market.US, "2026-07-15")
		if ok || base != 0 {
			t.Errorf("netWorthBaseline() = (%v, %v), want (0, false)", base, ok)
		}
	})
}

// TestBuildDashboard_PeriodReturns confirms buildDashboard wires YTD/QTD/HTD
// return% correctly through netWorthBaseline + PeriodReturnPct (both unit
// tested independently — this checks the assembly, not the math). Dates are
// computed relative to time.Now() so the test doesn't depend on which real
// calendar date it happens to run on.
func TestBuildDashboard_PeriodReturns(t *testing.T) {
	now := time.Now()
	ytdStart, err := time.Parse("2006-01-02", YTDStart(now))
	if err != nil {
		t.Fatalf("parse YTDStart: %v", err)
	}
	dayBeforeYTD := ytdStart.AddDate(0, 0, -1).Format("2006-01-02")
	today := now.Format("2006-01-02")

	fdb := &fakeDB{
		txs: []db.Transaction{tx("AAPL", "BUY", 10, 100, "2019-01-01")},
		snapshots: []db.DailySnapshot{
			snap("AAPL", "2019-01-01", 100), // buy correction: (100-100)*10 = 0
			snap("AAPL", dayBeforeYTD, 200), // mark-to-market before every period start: (200-100)*10=1000
			snap("AAPL", today, 250),        // mark-to-market within every period: (250-200)*10=500
		},
		netWorthOnOrBeforeFn: func(date string, m market.MarketID) (float64, bool, error) {
			return 1000, true, nil // fixed baseline regardless of exact date, for a deterministic pct
		},
	}

	got, err := buildDashboard(fdb, &fakeQuotes{}, market.US)
	if err != nil {
		t.Fatalf("buildDashboard() error = %v", err)
	}
	if got.KPIs.NetPnL != 1500 {
		t.Fatalf("NetPnL = %v, want 1500 (0+1000+500)", got.KPIs.NetPnL)
	}
	// Period P&L for all three windows = 1500 (today) - 1000 (last point
	// before dayBeforeYTD/ytdStart/qtdStart/htdStart, all of which fall
	// after dayBeforeYTD) = 500; baseline fixed at 1000 → 50%.
	for name, got := range map[string]*float64{
		"YTD": got.KPIs.YTDReturnPct,
		"QTD": got.KPIs.QTDReturnPct,
		"HTD": got.KPIs.HTDReturnPct,
	} {
		if got == nil {
			t.Errorf("%sReturnPct = nil, want non-nil", name)
			continue
		}
		if *got != 50 {
			t.Errorf("%sReturnPct = %v, want 50", name, *got)
		}
	}
}

// TestBuildDashboard_PeriodReturnsNilWhenNoBaseline confirms the honesty
// contract: with no net worth data on record at all, the three fields stay
// nil (JSON null), never a misleading 0.
func TestBuildDashboard_PeriodReturnsNilWhenNoBaseline(t *testing.T) {
	fdb := &fakeDB{
		txs:       []db.Transaction{tx("AAPL", "BUY", 10, 100, "2019-01-01")},
		snapshots: []db.DailySnapshot{snap("AAPL", "2019-01-01", 100)},
	}

	got, err := buildDashboard(fdb, &fakeQuotes{}, market.US)
	if err != nil {
		t.Fatalf("buildDashboard() error = %v", err)
	}
	if got.KPIs.YTDReturnPct != nil || got.KPIs.QTDReturnPct != nil || got.KPIs.HTDReturnPct != nil {
		t.Errorf("period return fields = %+v, want all nil (no net worth data)", got.KPIs)
	}
}

func TestBuildDashboard_QuoteFailureDegradesGracefully(t *testing.T) {
	fdb := &fakeDB{positions: []db.Position{{Ticker: "AAPL", Shares: 10, AvgCost: 100}}}
	quotes := &fakeQuotes{err: map[string]error{"AAPL": errors.New("provider down")}}

	got, err := buildDashboard(fdb, quotes, market.US)
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
