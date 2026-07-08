package db

import (
	"errors"
	"path/filepath"
	"testing"
)

// newTestDB opens a fresh sqlite file in a t.TempDir() so each test gets an
// isolated, real database (modernc.org/sqlite is pure Go — no cgo needed,
// nothing to mock).
func newTestDB(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	d, err := New(path)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestWatchlistRoundTrip(t *testing.T) {
	d := newTestDB(t)

	if got, err := d.GetWatchlist(); err != nil || len(got) != 0 {
		t.Fatalf("GetWatchlist() = %v, %v; want empty, nil", got, err)
	}

	if err := d.AddTicker("MSFT"); err != nil {
		t.Fatalf("AddTicker() error = %v", err)
	}
	if err := d.AddTicker("AAPL"); err != nil {
		t.Fatalf("AddTicker() error = %v", err)
	}

	got, err := d.GetWatchlist()
	if err != nil {
		t.Fatalf("GetWatchlist() error = %v", err)
	}
	want := []string{"AAPL", "MSFT"} // ORDER BY ticker
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("GetWatchlist() = %v, want %v", got, want)
	}
}

func TestAddTickerIsIdempotent(t *testing.T) {
	d := newTestDB(t)

	if err := d.AddTicker("AAPL"); err != nil {
		t.Fatalf("AddTicker() error = %v", err)
	}
	if err := d.AddTicker("AAPL"); err != nil {
		t.Fatalf("AddTicker() (duplicate) error = %v, want nil (INSERT OR IGNORE)", err)
	}

	got, err := d.GetWatchlist()
	if err != nil {
		t.Fatalf("GetWatchlist() error = %v", err)
	}
	if len(got) != 1 {
		t.Errorf("GetWatchlist() = %v, want exactly one entry", got)
	}
}

func TestRemoveTicker(t *testing.T) {
	d := newTestDB(t)

	if err := d.AddTicker("AAPL"); err != nil {
		t.Fatalf("AddTicker() error = %v", err)
	}
	if err := d.RemoveTicker("AAPL"); err != nil {
		t.Fatalf("RemoveTicker() error = %v", err)
	}

	got, err := d.GetWatchlist()
	if err != nil {
		t.Fatalf("GetWatchlist() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("GetWatchlist() = %v, want empty after removal", got)
	}

	// Removing a ticker that was never added should be a no-op, not an error.
	if err := d.RemoveTicker("NOPE"); err != nil {
		t.Errorf("RemoveTicker() on absent ticker error = %v, want nil", err)
	}
}

func TestSaveSnapshotUpsert(t *testing.T) {
	d := newTestDB(t)

	s := DailySnapshot{
		Ticker: "AAPL", Date: "2026-07-05",
		Open: 200, Close: 205, High: 206, Low: 199, Volume: 1000, ChangePercent: 2.5,
	}
	if err := d.SaveSnapshot(s); err != nil {
		t.Fatalf("SaveSnapshot() error = %v", err)
	}

	// Same ticker+date should replace, not conflict (INSERT OR REPLACE with a
	// UNIQUE(ticker, date) constraint).
	s.Close = 210
	if err := d.SaveSnapshot(s); err != nil {
		t.Fatalf("SaveSnapshot() (upsert) error = %v", err)
	}
}

func TestSaveRecommendations(t *testing.T) {
	d := newTestDB(t)

	recs := []Recommendation{
		{Ticker: "AAPL", Action: "BUY", Reason: "strong earnings", Price: 205.5},
		{Ticker: "MSFT", Action: "HOLD", Reason: "cloud growth", Price: 430},
	}
	if err := d.SaveRecommendations("2026-07-05", recs); err != nil {
		t.Fatalf("SaveRecommendations() error = %v", err)
	}

	got, err := d.GetRecommendationsSince("2026-07-01")
	if err != nil {
		t.Fatalf("GetRecommendationsSince() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("GetRecommendationsSince() returned %d recs, want 2", len(got))
	}
	if got[0].Ticker != "AAPL" || got[0].Action != "BUY" || got[0].Price != 205.5 || got[0].Date != "2026-07-05" {
		t.Errorf("GetRecommendationsSince()[0] = %+v, want AAPL/BUY/205.5/2026-07-05", got[0])
	}

	// A cutoff after the recommendation date excludes it.
	got, err = d.GetRecommendationsSince("2026-07-06")
	if err != nil {
		t.Fatalf("GetRecommendationsSince() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("GetRecommendationsSince(later date) = %v, want empty", got)
	}
}

func TestSaveRecommendationsEmptyIsNoop(t *testing.T) {
	d := newTestDB(t)

	if err := d.SaveRecommendations("2026-07-05", nil); err != nil {
		t.Errorf("SaveRecommendations(nil) error = %v, want nil", err)
	}
}

func TestSignalStateRoundTrip(t *testing.T) {
	d := newTestDB(t)

	state, err := d.GetSignalState("AAPL", "macd")
	if err != nil || state != "" {
		t.Fatalf("GetSignalState() before any set = %q, %v; want \"\", nil", state, err)
	}

	if err := d.SetSignalState("AAPL", "macd", "bullish"); err != nil {
		t.Fatalf("SetSignalState() error = %v", err)
	}
	if state, _ := d.GetSignalState("AAPL", "macd"); state != "bullish" {
		t.Errorf("GetSignalState() = %q, want bullish", state)
	}

	// Upsert replaces the previous state for the same (ticker, signal).
	if err := d.SetSignalState("AAPL", "macd", "bearish"); err != nil {
		t.Fatalf("SetSignalState() (upsert) error = %v", err)
	}
	if state, _ := d.GetSignalState("AAPL", "macd"); state != "bearish" {
		t.Errorf("GetSignalState() after upsert = %q, want bearish", state)
	}

	// Different signal family on the same ticker is independent.
	if state, _ := d.GetSignalState("AAPL", "rsi"); state != "" {
		t.Errorf("GetSignalState(rsi) = %q, want \"\" (unset)", state)
	}
}

func TestGetSnapshotClose(t *testing.T) {
	d := newTestDB(t)

	if err := d.SaveSnapshot(DailySnapshot{
		Ticker: "AAPL", Date: "2026-07-05",
		Open: 200, Close: 205, High: 206, Low: 199, Volume: 1000, ChangePercent: 2.5,
	}); err != nil {
		t.Fatalf("SaveSnapshot() error = %v", err)
	}

	close, ok, err := d.GetSnapshotClose("AAPL", "2026-07-05")
	if err != nil || !ok || close != 205 {
		t.Errorf("GetSnapshotClose() = %v, %v, %v; want 205, true, nil", close, ok, err)
	}

	_, ok, err = d.GetSnapshotClose("AAPL", "2026-07-06")
	if err != nil || ok {
		t.Errorf("GetSnapshotClose() for absent date: ok = %v, err = %v; want false, nil", ok, err)
	}
}

func TestRecordBuyWeightedAverageCost(t *testing.T) {
	d := newTestDB(t)

	pos, err := d.RecordBuy("AAPL", 10, 200, 1, "2026-07-01")
	if err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}
	// (10*200 + 1) / 10 = 200.1
	if pos.Shares != 10 || pos.AvgCost != 200.1 {
		t.Errorf("RecordBuy() = %+v, want Shares=10 AvgCost=200.1", pos)
	}

	// A second buy folds into a weighted average: (10*200.1 + 10*220) / 20 = 210.05
	pos, err = d.RecordBuy("AAPL", 10, 220, 0, "2026-07-02")
	if err != nil {
		t.Fatalf("RecordBuy() (second) error = %v", err)
	}
	if pos.Shares != 20 || pos.AvgCost != 210.05 {
		t.Errorf("RecordBuy() (second) = %+v, want Shares=20 AvgCost=210.05", pos)
	}

	got, ok, err := d.GetPosition("AAPL")
	if err != nil || !ok {
		t.Fatalf("GetPosition() = %+v, %v, %v", got, ok, err)
	}
	if got.Shares != 20 || got.AvgCost != 210.05 {
		t.Errorf("GetPosition() = %+v, want Shares=20 AvgCost=210.05", got)
	}
}

func TestRecordSellRealizedPnLAndPartialClose(t *testing.T) {
	d := newTestDB(t)

	if _, err := d.RecordBuy("AAPL", 10, 200, 0, "2026-07-01"); err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}

	// Sell 4 @ 220 with $1 fee: realized = (220-200)*4 - 1 = 79
	pos, pnl, err := d.RecordSell("AAPL", 4, 220, 1, "2026-07-05")
	if err != nil {
		t.Fatalf("RecordSell() error = %v", err)
	}
	if pnl != 79 {
		t.Errorf("RecordSell() realizedPnL = %v, want 79", pnl)
	}
	if pos.Shares != 6 || pos.AvgCost != 200 {
		t.Errorf("RecordSell() remaining position = %+v, want Shares=6 AvgCost=200", pos)
	}

	total, err := d.GetRealizedPnL()
	if err != nil || total != 79 {
		t.Errorf("GetRealizedPnL() = %v, %v; want 79, nil", total, err)
	}

	// Selling the rest closes the position out entirely.
	pos, _, err = d.RecordSell("AAPL", 6, 210, 0, "2026-07-06")
	if err != nil {
		t.Fatalf("RecordSell() (final) error = %v", err)
	}
	if pos.Shares != 0 {
		t.Errorf("RecordSell() (final) remaining shares = %v, want 0", pos.Shares)
	}
	if _, ok, err := d.GetPosition("AAPL"); err != nil || ok {
		t.Errorf("GetPosition() after full close: ok = %v, err = %v; want false, nil", ok, err)
	}
}

func TestRecordSellErrors(t *testing.T) {
	d := newTestDB(t)

	if _, _, err := d.RecordSell("AAPL", 1, 100, 0, "2026-07-01"); !errors.Is(err, ErrNoPosition) {
		t.Errorf("RecordSell() with no position error = %v, want ErrNoPosition", err)
	}

	if _, err := d.RecordBuy("AAPL", 5, 100, 0, "2026-07-01"); err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}
	if _, _, err := d.RecordSell("AAPL", 10, 100, 0, "2026-07-02"); !errors.Is(err, ErrInsufficientShares) {
		t.Errorf("RecordSell() oversized error = %v, want ErrInsufficientShares", err)
	}
}

func TestGetPositionsOrdering(t *testing.T) {
	d := newTestDB(t)

	if _, err := d.RecordBuy("MSFT", 1, 400, 0, "2026-07-01"); err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}
	if _, err := d.RecordBuy("AAPL", 1, 200, 0, "2026-07-01"); err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}

	got, err := d.GetPositions()
	if err != nil {
		t.Fatalf("GetPositions() error = %v", err)
	}
	if len(got) != 2 || got[0].Ticker != "AAPL" || got[1].Ticker != "MSFT" {
		t.Errorf("GetPositions() = %+v, want [AAPL, MSFT]", got)
	}
}

func TestSaveNetWorthSnapshotUpsert(t *testing.T) {
	d := newTestDB(t)

	if err := d.SaveNetWorthSnapshot("2026-07-05", 1000); err != nil {
		t.Fatalf("SaveNetWorthSnapshot() error = %v", err)
	}
	// Same date should replace, not conflict.
	if err := d.SaveNetWorthSnapshot("2026-07-05", 1200); err != nil {
		t.Fatalf("SaveNetWorthSnapshot() (upsert) error = %v", err)
	}
}

// TestMigrateIsRerunnable reopens the same database file, simulating a bot
// restart — migrate() must see the recorded user_version and skip already-
// applied steps (re-running the ALTER TABLE step would fail on duplicate
// columns).
func TestMigrateIsRerunnable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	d, err := New(path)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := d.AddTicker("AAPL"); err != nil {
		t.Fatalf("AddTicker() error = %v", err)
	}
	d.Close()

	d, err = New(path)
	if err != nil {
		t.Fatalf("New() (reopen) error = %v", err)
	}
	defer d.Close()
	got, err := d.GetWatchlist()
	if err != nil || len(got) != 1 || got[0] != "AAPL" {
		t.Errorf("GetWatchlist() after reopen = %v, %v; want [AAPL], nil", got, err)
	}
}
