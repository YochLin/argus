package db

import (
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
