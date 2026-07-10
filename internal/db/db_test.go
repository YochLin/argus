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

func TestGetLatestSnapshot(t *testing.T) {
	d := newTestDB(t)

	if _, ok, err := d.GetLatestSnapshot("AAPL"); err != nil || ok {
		t.Fatalf("GetLatestSnapshot() before any snapshot: ok = %v, err = %v; want false, nil", ok, err)
	}

	if err := d.SaveSnapshot(DailySnapshot{Ticker: "AAPL", Date: "2026-07-01", Close: 200}); err != nil {
		t.Fatalf("SaveSnapshot() error = %v", err)
	}
	if err := d.SaveSnapshot(DailySnapshot{Ticker: "AAPL", Date: "2026-07-05", Close: 210}); err != nil {
		t.Fatalf("SaveSnapshot() error = %v", err)
	}

	got, ok, err := d.GetLatestSnapshot("AAPL")
	if err != nil || !ok || got.Date != "2026-07-05" || got.Close != 210 {
		t.Errorf("GetLatestSnapshot() = %+v, %v, %v; want Date=2026-07-05 Close=210, true, nil", got, ok, err)
	}
}

func TestBackup(t *testing.T) {
	d := newTestDB(t)
	if err := d.AddTicker("AAPL"); err != nil {
		t.Fatalf("AddTicker() error = %v", err)
	}

	dest := filepath.Join(t.TempDir(), "backup.db")
	if err := d.Backup(dest); err != nil {
		t.Fatalf("Backup() error = %v", err)
	}

	backup, err := New(dest)
	if err != nil {
		t.Fatalf("open backup file: %v", err)
	}
	defer backup.Close()
	got, err := backup.GetWatchlist()
	if err != nil || len(got) != 1 || got[0] != "AAPL" {
		t.Errorf("backup GetWatchlist() = %v, %v; want [AAPL], nil", got, err)
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

// TestSeedSP500 verifies New() seeds the universe table from the embedded
// ticker list on first creation, and that reopening the same database
// (simulating a restart) doesn't duplicate or re-seed rows the user may have
// since removed.
func TestSeedSP500(t *testing.T) {
	d := newTestDB(t)

	entries, err := d.GetUniverse()
	if err != nil {
		t.Fatalf("GetUniverse() error = %v", err)
	}
	if len(entries) < 400 {
		t.Fatalf("GetUniverse() len = %d, want >= 400 (S&P 500 seed)", len(entries))
	}
	for _, e := range entries {
		if e.Source != "sp500" {
			t.Fatalf("unexpected source %q for freshly seeded ticker %q", e.Source, e.Ticker)
		}
	}

	// A user removes a seeded ticker...
	if err := d.RemoveUniverseTicker(entries[0].Ticker); err != nil {
		t.Fatalf("RemoveUniverseTicker() error = %v", err)
	}
	afterRemove, err := d.GetUniverse()
	if err != nil {
		t.Fatalf("GetUniverse() error = %v", err)
	}
	if len(afterRemove) != len(entries)-1 {
		t.Fatalf("GetUniverse() len after remove = %d, want %d", len(afterRemove), len(entries)-1)
	}

	// ...and it must not come back on a later seedSP500() call (only re-seeds
	// when the sp500 source is entirely empty).
	if err := d.seedSP500(); err != nil {
		t.Fatalf("seedSP500() error = %v", err)
	}
	afterReseed, err := d.GetUniverse()
	if err != nil {
		t.Fatalf("GetUniverse() error = %v", err)
	}
	if len(afterReseed) != len(afterRemove) {
		t.Errorf("GetUniverse() len after reseed = %d, want %d (removed ticker should stay gone)", len(afterReseed), len(afterRemove))
	}
}

func TestUniverseAddRemove(t *testing.T) {
	d := newTestDB(t)

	if err := d.AddUniverseTicker("ZZZZ", "manual"); err != nil {
		t.Fatalf("AddUniverseTicker() error = %v", err)
	}
	entries, err := d.GetUniverse()
	if err != nil {
		t.Fatalf("GetUniverse() error = %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Ticker == "ZZZZ" {
			found = true
			if e.Source != "manual" {
				t.Errorf("ZZZZ source = %q, want manual", e.Source)
			}
		}
	}
	if !found {
		t.Fatalf("ZZZZ not found in universe after AddUniverseTicker()")
	}

	if err := d.RemoveUniverseTicker("ZZZZ"); err != nil {
		t.Fatalf("RemoveUniverseTicker() error = %v", err)
	}
	entries, err = d.GetUniverse()
	if err != nil {
		t.Fatalf("GetUniverse() error = %v", err)
	}
	for _, e := range entries {
		if e.Ticker == "ZZZZ" {
			t.Fatalf("ZZZZ still present after RemoveUniverseTicker()")
		}
	}
}

func TestGetLatestRecommendations(t *testing.T) {
	d := newTestDB(t)

	if err := d.SaveRecommendations("2026-07-01", []Recommendation{
		{Ticker: "AAPL", Action: "HOLD", Reason: "wait and see", Price: 200},
	}); err != nil {
		t.Fatalf("SaveRecommendations() error = %v", err)
	}
	if err := d.SaveRecommendations("2026-07-05", []Recommendation{
		{Ticker: "AAPL", Action: "BUY", Reason: "breakout", Price: 210},
		{Ticker: "MSFT", Action: "SELL", Reason: "overextended", Price: 430},
	}); err != nil {
		t.Fatalf("SaveRecommendations() error = %v", err)
	}

	got, err := d.GetLatestRecommendations([]string{"AAPL", "MSFT", "NVDA"})
	if err != nil {
		t.Fatalf("GetLatestRecommendations() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("GetLatestRecommendations() = %+v, want exactly AAPL/MSFT (NVDA has no history)", got)
	}
	// AAPL must resolve to the 07-05 row (latest), not the 07-01 HOLD.
	if got["AAPL"].Action != "BUY" || got["AAPL"].Date != "2026-07-05" || got["AAPL"].Price != 210 {
		t.Errorf("GetLatestRecommendations()[AAPL] = %+v, want BUY/2026-07-05/210", got["AAPL"])
	}
	if got["MSFT"].Action != "SELL" {
		t.Errorf("GetLatestRecommendations()[MSFT] = %+v, want SELL", got["MSFT"])
	}
	if _, ok := got["NVDA"]; ok {
		t.Errorf("GetLatestRecommendations()[NVDA] present, want absent (no recommendation history)")
	}
}

func TestGetLatestRecommendationsEmptyTickers(t *testing.T) {
	d := newTestDB(t)
	got, err := d.GetLatestRecommendations(nil)
	if err != nil || got != nil {
		t.Errorf("GetLatestRecommendations(nil) = %v, %v; want nil, nil", got, err)
	}
}

func TestGetEarliestBuyDate(t *testing.T) {
	d := newTestDB(t)

	if _, ok, err := d.GetEarliestBuyDate("AAPL"); err != nil || ok {
		t.Fatalf("GetEarliestBuyDate() before any buy: ok = %v, err = %v; want false, nil", ok, err)
	}

	if _, err := d.RecordBuy("AAPL", 5, 200, 0, "2026-07-05"); err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}
	if _, err := d.RecordBuy("AAPL", 5, 210, 0, "2026-07-01"); err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}

	date, ok, err := d.GetEarliestBuyDate("AAPL")
	if err != nil || !ok || date != "2026-07-01" {
		t.Errorf("GetEarliestBuyDate() = %q, %v, %v; want 2026-07-01, true, nil", date, ok, err)
	}
}

func TestGetPeakClose(t *testing.T) {
	d := newTestDB(t)

	if _, ok, err := d.GetPeakClose("AAPL", "2026-07-01"); err != nil || ok {
		t.Fatalf("GetPeakClose() before any snapshot: ok = %v, err = %v; want false, nil", ok, err)
	}

	for _, s := range []DailySnapshot{
		{Ticker: "AAPL", Date: "2026-06-30", Close: 300}, // before sinceDate, must be excluded
		{Ticker: "AAPL", Date: "2026-07-01", Close: 200},
		{Ticker: "AAPL", Date: "2026-07-03", Close: 220},
		{Ticker: "AAPL", Date: "2026-07-05", Close: 210},
	} {
		if err := d.SaveSnapshot(s); err != nil {
			t.Fatalf("SaveSnapshot() error = %v", err)
		}
	}

	peak, ok, err := d.GetPeakClose("AAPL", "2026-07-01")
	if err != nil || !ok || peak != 220 {
		t.Errorf("GetPeakClose() = %v, %v, %v; want 220, true, nil", peak, ok, err)
	}
}

func TestScanHitsGroupedByTicker(t *testing.T) {
	d := newTestDB(t)

	if err := d.SaveScanHit("ZZZZ", "2026-07-08", "RSI oversold (28.4)"); err != nil {
		t.Fatalf("SaveScanHit() error = %v", err)
	}
	if err := d.SaveScanHit("ZZZZ", "2026-07-08", "MACD golden cross"); err != nil {
		t.Fatalf("SaveScanHit() error = %v", err)
	}
	if err := d.SaveScanHit("YYYY", "2026-07-08", "RSI oversold (25.1)"); err != nil {
		t.Fatalf("SaveScanHit() error = %v", err)
	}
	// A different date must not leak into the same-day query.
	if err := d.SaveScanHit("ZZZZ", "2026-07-07", "RSI oversold (29.0)"); err != nil {
		t.Fatalf("SaveScanHit() error = %v", err)
	}

	got, err := d.GetScanHits("2026-07-08")
	if err != nil {
		t.Fatalf("GetScanHits() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("GetScanHits() len = %d, want 2", len(got))
	}
	want := "RSI oversold (28.4); MACD golden cross"
	if got["ZZZZ"] != want {
		t.Errorf("GetScanHits()[ZZZZ] = %q, want %q", got["ZZZZ"], want)
	}
	if got["YYYY"] != "RSI oversold (25.1)" {
		t.Errorf("GetScanHits()[YYYY] = %q, want %q", got["YYYY"], "RSI oversold (25.1)")
	}
}
