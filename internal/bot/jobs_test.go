package bot

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/market"
)

// quoteProvider is a data.Provider stub that returns a fixed quote per
// ticker (or "no data" for anything else) — RunClosingSnapshot's market
// filtering/date-semantics tests need real, distinguishable quotes per
// ticker, unlike pending_actions_test.go's noDataProvider.
type quoteProvider struct {
	quotes map[string]*data.Quote
}

func (p quoteProvider) Name() string { return "stub" }
func (p quoteProvider) GetQuote(ticker string) (*data.Quote, error) {
	if q, ok := p.quotes[ticker]; ok {
		return q, nil
	}
	return nil, errors.New("no data")
}
func (p quoteProvider) GetNews(string, int) ([]data.NewsItem, error) { return nil, nil }
func (p quoteProvider) GetMarketMovers() ([]string, error)           { return nil, nil }

func newJobsTestBot(t *testing.T, quotes map[string]*data.Quote) (*Bot, *db.DB) {
	t.Helper()
	d, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("db.New() error = %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return &Bot{db: d, lang: i18n.EN, provider: quoteProvider{quotes: quotes}, history: noDataProvider{}}, d
}

// TestRunClosingSnapshot_MarketFilterAndDateSemantics is Phase 6 PR1's core
// correctness requirement for the closing-snapshot job
// (docs/phase-6-tw-market.md §4.3/§4.6): each market only snapshots its own
// watchlist tickers, and the two markets date their snapshot differently —
// US one day back (the US session that just closed is still "yesterday" in
// Taiwan terms at 05:30 CST), TW the same day (its 14:30 CST run is the
// same afternoon as the session it's recording).
func TestRunClosingSnapshot_MarketFilterAndDateSemantics(t *testing.T) {
	now := time.Now().In(cst)
	fresh := now.Add(-1 * time.Hour) // well within the 12h staleness guard

	b, d := newJobsTestBot(t, map[string]*data.Quote{
		"AAPL": {Ticker: "AAPL", Price: 160, Timestamp: fresh},
		"2330": {Ticker: "2330", Price: 950, Timestamp: fresh},
	})
	if err := d.AddTicker("AAPL"); err != nil {
		t.Fatalf("AddTicker(AAPL) error = %v", err)
	}
	if err := d.AddTicker("2330"); err != nil {
		t.Fatalf("AddTicker(2330) error = %v", err)
	}

	ctx := context.Background()
	b.RunClosingSnapshot(ctx, market.US)
	b.RunClosingSnapshot(ctx, market.TW)

	wantUSDate := now.AddDate(0, 0, -1).Format("2006-01-02")
	wantTWDate := now.Format("2006-01-02")

	if _, ok, err := d.GetSnapshotClose("AAPL", wantUSDate); err != nil || !ok {
		t.Errorf("AAPL snapshot on %s: ok=%v, err=%v; want a snapshot dated one day back", wantUSDate, ok, err)
	}
	if _, ok, err := d.GetSnapshotClose("2330", wantUSDate); err != nil || ok {
		t.Errorf("2330 leaked into the US closing snapshot's date (%s)", wantUSDate)
	}
	if _, ok, err := d.GetSnapshotClose("2330", wantTWDate); err != nil || !ok {
		t.Errorf("2330 snapshot on %s: ok=%v, err=%v; want a snapshot dated today", wantTWDate, ok, err)
	}
	if _, ok, err := d.GetSnapshotClose("AAPL", wantTWDate); err != nil || ok {
		if wantTWDate != wantUSDate { // don't false-fail on a date collision across a midnight boundary
			t.Errorf("AAPL leaked into the TW closing snapshot's date (%s)", wantTWDate)
		}
	}
}

// TestRunClosingSnapshot_NetWorthPerMarket verifies recordNetWorthSnapshot
// keeps each market's position value in its own net_worth_snapshots row
// (Phase 6 §3.2 — TWD/USD never sum together).
func TestRunClosingSnapshot_NetWorthPerMarket(t *testing.T) {
	now := time.Now().In(cst)
	fresh := now.Add(-1 * time.Hour)

	b, d := newJobsTestBot(t, map[string]*data.Quote{
		"AAPL": {Ticker: "AAPL", Price: 160, Timestamp: fresh},
		"2330": {Ticker: "2330", Price: 950, Timestamp: fresh},
	})
	if _, err := d.RecordBuy("AAPL", 10, 150, 0, "2026-07-01"); err != nil {
		t.Fatalf("RecordBuy(AAPL) error = %v", err)
	}
	if _, err := d.RecordBuy("2330", 1000, 900, 0, "2026-07-01"); err != nil {
		t.Fatalf("RecordBuy(2330) error = %v", err)
	}

	ctx := context.Background()
	b.RunClosingSnapshot(ctx, market.US)
	b.RunClosingSnapshot(ctx, market.TW)

	_, usTotal, ok, err := d.GetLatestNetWorth(market.US)
	if err != nil || !ok {
		t.Fatalf("GetLatestNetWorth(US) = _, %v, %v, %v", usTotal, ok, err)
	}
	if usTotal != 1600 { // 10 * $160
		t.Errorf("US net worth = %v, want 1600 (AAPL only)", usTotal)
	}

	_, twTotal, ok, err := d.GetLatestNetWorth(market.TW)
	if err != nil || !ok {
		t.Fatalf("GetLatestNetWorth(TW) = _, %v, %v, %v", twTotal, ok, err)
	}
	if twTotal != 950000 { // 1000 * NT$950
		t.Errorf("TW net worth = %v, want 950000 (2330 only)", twTotal)
	}
}

// TestIsTWMarketClosed is Phase 6 PR2's TW daily-report market-closed
// heuristic (§3.3/§5.1): a fresh 0050 quote means "open", a >12h-stale one
// (or a failed fetch entirely) means "closed" — the fail-safe direction
// matters here, since the report should be skipped rather than run off an
// unknown-freshness quote.
func TestIsTWMarketClosed(t *testing.T) {
	now := time.Now().In(cst)

	t.Run("fresh quote means open", func(t *testing.T) {
		b, _ := newJobsTestBot(t, map[string]*data.Quote{
			"0050": {Ticker: "0050", Price: 190, Timestamp: now.Add(-1 * time.Hour)},
		})
		if b.isTWMarketClosed() {
			t.Error("isTWMarketClosed() = true, want false for a fresh (1h old) quote")
		}
	})

	t.Run("stale quote means closed", func(t *testing.T) {
		b, _ := newJobsTestBot(t, map[string]*data.Quote{
			"0050": {Ticker: "0050", Price: 190, Timestamp: now.Add(-25 * time.Hour)},
		})
		if !b.isTWMarketClosed() {
			t.Error("isTWMarketClosed() = false, want true for a 25h-stale quote")
		}
	})

	t.Run("quote fetch failure fails safe to closed", func(t *testing.T) {
		b, _ := newJobsTestBot(t, map[string]*data.Quote{})
		if !b.isTWMarketClosed() {
			t.Error("isTWMarketClosed() = false, want true when the quote fetch itself fails")
		}
	})
}

// TestFilterPositionsByMarket verifies runDailyReport's exit-discipline
// checks only ever see the current market's own positions — without this
// filter, a position in the *other* market would still get checked (see
// runDailyReport's own doc comment on why: priceFor falls back to a live
// quote fetch for anything missing from its prefetch map, so a missing
// filter wouldn't even fail quietly).
func TestFilterPositionsByMarket(t *testing.T) {
	positions := map[string]db.Position{
		"AAPL": {Ticker: "AAPL", Shares: 10},
		"2330": {Ticker: "2330", Shares: 1000},
	}

	us := filterPositionsByMarket(positions, market.US)
	if len(us) != 1 || us["AAPL"].Shares != 10 {
		t.Errorf("filterPositionsByMarket(US) = %+v, want just AAPL", us)
	}

	tw := filterPositionsByMarket(positions, market.TW)
	if len(tw) != 1 || tw["2330"].Shares != 1000 {
		t.Errorf("filterPositionsByMarket(TW) = %+v, want just 2330", tw)
	}
}
