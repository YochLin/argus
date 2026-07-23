package db

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"argus/internal/market"
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
		{Ticker: "AAPL", Action: "BUY", Reason: "strong earnings", Price: 205.5, Source: "watchlist"},
		{Ticker: "MSFT", Action: "HOLD", Reason: "cloud growth", Price: 430, Source: "scan"},
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
	if got[0].Ticker != "AAPL" || got[0].Action != "BUY" || got[0].Price != 205.5 || got[0].Date != "2026-07-05" || got[0].Source != "watchlist" {
		t.Errorf("GetRecommendationsSince()[0] = %+v, want AAPL/BUY/205.5/2026-07-05/watchlist", got[0])
	}
	if got[1].Source != "scan" {
		t.Errorf("GetRecommendationsSince()[1].Source = %q, want scan", got[1].Source)
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

func TestSaveRecommendationsDefaultSourceIsEmpty(t *testing.T) {
	d := newTestDB(t)

	if err := d.SaveRecommendations("2026-07-05", []Recommendation{
		{Ticker: "AAPL", Action: "BUY", Reason: "no source given", Price: 200},
	}); err != nil {
		t.Fatalf("SaveRecommendations() error = %v", err)
	}

	got, err := d.GetRecommendationsSince("2026-07-01")
	if err != nil || len(got) != 1 {
		t.Fatalf("GetRecommendationsSince() = %v, %v; want exactly one row", got, err)
	}
	if got[0].Source != "" {
		t.Errorf("Source = %q, want \"\" when not set (displays as watchlist at the read path)", got[0].Source)
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

	total, err := d.GetRealizedPnL(market.US)
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

func TestSetStopPrice(t *testing.T) {
	d := newTestDB(t)

	if err := d.SetStopPrice("AAPL", 190); !errors.Is(err, ErrNoPosition) {
		t.Errorf("SetStopPrice() with no position error = %v, want ErrNoPosition", err)
	}

	if _, err := d.RecordBuy("AAPL", 10, 200, 0, "2026-07-01"); err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}
	if err := d.SetStopPrice("AAPL", 190); err != nil {
		t.Fatalf("SetStopPrice() error = %v", err)
	}

	pos, ok, err := d.GetPosition("AAPL")
	if err != nil {
		t.Fatalf("GetPosition() error = %v", err)
	}
	if !ok || pos.StopPrice != 190 {
		t.Errorf("GetPosition() = %+v, want StopPrice=190", pos)
	}

	// A full close deletes the positions row, so the stop price goes with
	// it rather than lingering for a later, unrelated round in the ticker.
	if _, _, err := d.RecordSell("AAPL", 10, 210, 0, "2026-07-05"); err != nil {
		t.Fatalf("RecordSell() error = %v", err)
	}
	if err := d.SetStopPrice("AAPL", 190); !errors.Is(err, ErrNoPosition) {
		t.Errorf("SetStopPrice() after full close error = %v, want ErrNoPosition", err)
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

	if err := d.SaveNetWorthSnapshot("2026-07-05", market.US, 1000); err != nil {
		t.Fatalf("SaveNetWorthSnapshot() error = %v", err)
	}
	// Same date should replace, not conflict.
	if err := d.SaveNetWorthSnapshot("2026-07-05", market.US, 1200); err != nil {
		t.Fatalf("SaveNetWorthSnapshot() (upsert) error = %v", err)
	}
}

func TestGetLatestNetWorthEmpty(t *testing.T) {
	d := newTestDB(t)

	_, _, ok, err := d.GetLatestNetWorth(market.US)
	if err != nil {
		t.Fatalf("GetLatestNetWorth() error = %v", err)
	}
	if ok {
		t.Error("GetLatestNetWorth() on an empty table should return ok=false")
	}
}

func TestGetLatestNetWorth(t *testing.T) {
	d := newTestDB(t)

	if err := d.SaveNetWorthSnapshot("2026-07-01", market.US, 1000); err != nil {
		t.Fatalf("SaveNetWorthSnapshot() error = %v", err)
	}
	if err := d.SaveNetWorthSnapshot("2026-07-08", market.US, 1100); err != nil {
		t.Fatalf("SaveNetWorthSnapshot() error = %v", err)
	}

	date, total, ok, err := d.GetLatestNetWorth(market.US)
	if err != nil || !ok || date != "2026-07-08" || total != 1100 {
		t.Errorf("GetLatestNetWorth() = %q, %v, %v, %v; want \"2026-07-08\", 1100, true, nil", date, total, ok, err)
	}
}

func TestGetNetWorthOnOrBefore(t *testing.T) {
	d := newTestDB(t)

	if err := d.SaveNetWorthSnapshot("2026-07-01", market.US, 1000); err != nil {
		t.Fatalf("SaveNetWorthSnapshot() error = %v", err)
	}
	if err := d.SaveNetWorthSnapshot("2026-07-03", market.US, 1050); err != nil {
		t.Fatalf("SaveNetWorthSnapshot() error = %v", err)
	}

	t.Run("exact date match", func(t *testing.T) {
		total, ok, err := d.GetNetWorthOnOrBefore("2026-07-03", market.US)
		if err != nil || !ok || total != 1050 {
			t.Errorf("GetNetWorthOnOrBefore(2026-07-03) = %v, %v, %v; want 1050, true, nil", total, ok, err)
		}
	})

	t.Run("falls back to most recent prior date", func(t *testing.T) {
		// 2026-07-02 has no snapshot (e.g. a weekend) — should fall back to 07-01.
		total, ok, err := d.GetNetWorthOnOrBefore("2026-07-02", market.US)
		if err != nil || !ok || total != 1000 {
			t.Errorf("GetNetWorthOnOrBefore(2026-07-02) = %v, %v, %v; want 1000, true, nil", total, ok, err)
		}
	})

	t.Run("no snapshot on or before date", func(t *testing.T) {
		_, ok, err := d.GetNetWorthOnOrBefore("2026-06-01", market.US)
		if err != nil {
			t.Fatalf("GetNetWorthOnOrBefore() error = %v", err)
		}
		if ok {
			t.Error("GetNetWorthOnOrBefore() before any snapshot should return ok=false")
		}
	})
}

func TestGetNetWorthRange(t *testing.T) {
	d := newTestDB(t)

	if err := d.SaveNetWorthSnapshot("2026-06-30", market.US, 900); err != nil { // outside range
		t.Fatalf("SaveNetWorthSnapshot() error = %v", err)
	}
	if err := d.SaveNetWorthSnapshot("2026-07-01", market.US, 1000); err != nil {
		t.Fatalf("SaveNetWorthSnapshot() error = %v", err)
	}
	if err := d.SaveNetWorthSnapshot("2026-07-15", market.US, 1100); err != nil {
		t.Fatalf("SaveNetWorthSnapshot() error = %v", err)
	}
	if err := d.SaveNetWorthSnapshot("2026-08-01", market.US, 1200); err != nil { // outside range
		t.Fatalf("SaveNetWorthSnapshot() error = %v", err)
	}

	got, err := d.GetNetWorthRange("2026-07-01", "2026-07-31", market.US)
	if err != nil {
		t.Fatalf("GetNetWorthRange() error = %v", err)
	}
	want := []NetWorthPoint{{Date: "2026-07-01", Total: 1000}, {Date: "2026-07-15", Total: 1100}}
	if len(got) != len(want) {
		t.Fatalf("GetNetWorthRange() = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("GetNetWorthRange()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestGetNetWorthRangeEmpty(t *testing.T) {
	d := newTestDB(t)
	got, err := d.GetNetWorthRange("2026-07-01", "2026-07-31", market.US)
	if err != nil {
		t.Fatalf("GetNetWorthRange() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("GetNetWorthRange() on empty table = %+v, want empty", got)
	}
}

func TestGetTransactionStats(t *testing.T) {
	d := newTestDB(t)

	if _, err := d.RecordBuy("AAPL", 10, 100, 0, "2026-06-15"); err != nil { // outside range
		t.Fatalf("RecordBuy() error = %v", err)
	}
	if _, err := d.RecordBuy("AAPL", 10, 100, 0, "2026-07-01"); err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}
	if _, _, err := d.RecordSell("AAPL", 5, 120, 0, "2026-07-10"); err != nil {
		t.Fatalf("RecordSell() error = %v", err)
	}

	t.Run("month with a buy and a sell", func(t *testing.T) {
		count, sellCount, realized, err := d.GetTransactionStats("2026-07-01", "2026-07-31")
		if err != nil {
			t.Fatalf("GetTransactionStats() error = %v", err)
		}
		wantRealized := (120.0 - 100.0) * 5
		if count != 2 || sellCount != 1 || realized != wantRealized {
			t.Errorf("GetTransactionStats() = %d, %d, %v; want 2, 1, %v", count, sellCount, realized, wantRealized)
		}
	})

	t.Run("month with no transactions", func(t *testing.T) {
		count, sellCount, realized, err := d.GetTransactionStats("2026-09-01", "2026-09-30")
		if err != nil {
			t.Fatalf("GetTransactionStats() error = %v", err)
		}
		if count != 0 || sellCount != 0 || realized != 0 {
			t.Errorf("GetTransactionStats() = %d, %d, %v; want 0, 0, 0", count, sellCount, realized)
		}
	})
}

func TestGetSnapshotCloseRange(t *testing.T) {
	d := newTestDB(t)

	snap := func(date string, close float64) {
		t.Helper()
		if err := d.SaveSnapshot(DailySnapshot{Ticker: "SPY", Date: date, Close: close}); err != nil {
			t.Fatalf("SaveSnapshot(%s) error = %v", date, err)
		}
	}
	snap("2026-06-30", 500) // outside range
	snap("2026-07-01", 510)
	snap("2026-07-15", 520)
	snap("2026-07-31", 530)

	t.Run("multiple snapshots in range", func(t *testing.T) {
		first, last, ok, err := d.GetSnapshotCloseRange("SPY", "2026-07-01", "2026-07-31")
		if err != nil || !ok || first != 510 || last != 530 {
			t.Errorf("GetSnapshotCloseRange() = %v, %v, %v, %v; want 510, 530, true, nil", first, last, ok, err)
		}
	})

	t.Run("fewer than two snapshots in range", func(t *testing.T) {
		_, _, ok, err := d.GetSnapshotCloseRange("SPY", "2026-08-01", "2026-08-31")
		if err != nil {
			t.Fatalf("GetSnapshotCloseRange() error = %v", err)
		}
		if ok {
			t.Error("GetSnapshotCloseRange() with < 2 rows in range should return ok=false")
		}
	})
}

func TestGetSettingUnsetKeyIsNotFound(t *testing.T) {
	d := newTestDB(t)

	_, ok, err := d.GetSetting("cash_balance")
	if err != nil {
		t.Fatalf("GetSetting() error = %v", err)
	}
	if ok {
		t.Error("GetSetting() on an unset key should return ok=false")
	}
}

func TestSettingRoundTripAndUpsert(t *testing.T) {
	d := newTestDB(t)

	if err := d.SetSetting("cash_balance", "1000.50"); err != nil {
		t.Fatalf("SetSetting() error = %v", err)
	}
	got, ok, err := d.GetSetting("cash_balance")
	if err != nil || !ok || got != "1000.50" {
		t.Errorf("GetSetting() = %q, %v, %v; want \"1000.50\", true, nil", got, ok, err)
	}

	// Same key should overwrite, not conflict.
	if err := d.SetSetting("cash_balance", "2000"); err != nil {
		t.Fatalf("SetSetting() (upsert) error = %v", err)
	}
	got, ok, err = d.GetSetting("cash_balance")
	if err != nil || !ok || got != "2000" {
		t.Errorf("GetSetting() after upsert = %q, %v, %v; want \"2000\", true, nil", got, ok, err)
	}
}

func TestGetThesisUnsetTickerIsNotFound(t *testing.T) {
	d := newTestDB(t)

	_, ok, err := d.GetThesis("AAPL")
	if err != nil {
		t.Fatalf("GetThesis() error = %v", err)
	}
	if ok {
		t.Error("GetThesis() on a ticker with no thesis should return ok=false")
	}
}

func TestThesisRoundTripAndOverwrite(t *testing.T) {
	d := newTestDB(t)

	if err := d.SetThesis("AAPL", "long-term compounder, services growth"); err != nil {
		t.Fatalf("SetThesis() error = %v", err)
	}
	got, ok, err := d.GetThesis("AAPL")
	if err != nil || !ok || got != "long-term compounder, services growth" {
		t.Errorf("GetThesis() = %q, %v, %v; want the set thesis, true, nil", got, ok, err)
	}

	// Same ticker should overwrite wholesale, not conflict or append.
	if err := d.SetThesis("AAPL", "changed my mind: AI capex risk"); err != nil {
		t.Fatalf("SetThesis() (overwrite) error = %v", err)
	}
	got, ok, err = d.GetThesis("AAPL")
	if err != nil || !ok || got != "changed my mind: AI capex risk" {
		t.Errorf("GetThesis() after overwrite = %q, %v, %v; want the new thesis, true, nil", got, ok, err)
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

// TestOpenReadOnly verifies both halves of the read-only connection's
// contract: it can see data committed by a separate read-write connection
// to the same file (proving it isn't just an empty in-memory stand-in), and
// SQLite's query_only pragma actually rejects a write attempted through it
// (proving "read-only" is enforced, not just a naming convention) — see
// OpenReadOnly's doc comment for why this can't rely on the DSN's
// conventional "mode=ro" instead.
func TestOpenReadOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	rw, err := New(path)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer rw.Close()
	if err := rw.AddTicker("AAPL"); err != nil {
		t.Fatalf("AddTicker() error = %v", err)
	}

	ro, err := OpenReadOnly(path)
	if err != nil {
		t.Fatalf("OpenReadOnly() error = %v", err)
	}
	defer ro.Close()

	got, err := ro.GetWatchlist()
	if err != nil {
		t.Fatalf("GetWatchlist() error = %v", err)
	}
	if len(got) != 1 || got[0] != "AAPL" {
		t.Errorf("GetWatchlist() = %v, want [AAPL]", got)
	}

	if err := ro.AddTicker("MSFT"); err == nil {
		t.Error("AddTicker() through a read-only connection should have failed, got nil error")
	}
}

// TestOpenForWrites verifies the write-pilot connection is genuinely
// writable (unlike OpenReadOnly) and that its writes are visible through a
// separate connection to the same file — the two-writer-process scenario
// add_to_watchlist/remove_from_watchlist actually run under.
func TestOpenForWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	rw, err := New(path)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer rw.Close()

	writer, err := OpenForWrites(path)
	if err != nil {
		t.Fatalf("OpenForWrites() error = %v", err)
	}
	defer writer.Close()

	if err := writer.AddTicker("TSLA"); err != nil {
		t.Fatalf("AddTicker() through the write connection error = %v", err)
	}

	got, err := rw.GetWatchlist()
	if err != nil {
		t.Fatalf("GetWatchlist() error = %v", err)
	}
	if len(got) != 1 || got[0] != "TSLA" {
		t.Errorf("GetWatchlist() via the main connection = %v, want [TSLA] (write from the second connection should be visible)", got)
	}

	if err := writer.RemoveTicker("TSLA"); err != nil {
		t.Fatalf("RemoveTicker() through the write connection error = %v", err)
	}
	if got, err := rw.GetWatchlist(); err != nil || len(got) != 0 {
		t.Errorf("GetWatchlist() after remove = %v, %v; want empty, nil", got, err)
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

// TestMigration12BackfillsMarketAndRebuildsNetWorth simulates a pre-Phase-6
// database (stopped at migration 11, no market columns, net_worth_snapshots
// keyed by date alone) picking up a TW position before ever running the
// Phase 6 code — the deploy-day scenario docs/phase-6-tw-market.md §8 flags
// as this migration's biggest risk, since it's the project's first
// rebuild-a-table migration rather than a plain append-only ALTER TABLE.
// Asserts every pre-existing row survives with the correct backfilled
// market, not just that the migration runs without error.
func TestMigration12BackfillsMarketAndRebuildsNetWorth(t *testing.T) {
	conn, err := sql.Open("sqlite", fmt.Sprintf("file:%s", filepath.Join(t.TempDir(), "legacy.db")))
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	d := &DB{conn: conn}
	defer d.Close()

	// Apply migrations 1..11 by hand (pre-Phase-6 schema) and stamp
	// user_version accordingly, exactly like migrate() would have at that
	// point in this database's history.
	for i, m := range migrations[:11] {
		if _, err := conn.Exec(m); err != nil {
			t.Fatalf("apply legacy migration %d: %v", i+1, err)
		}
	}
	if _, err := conn.Exec(`PRAGMA user_version = 11`); err != nil {
		t.Fatalf("set user_version: %v", err)
	}

	// Seed old-shaped rows: a US and a TW ticker on the watchlist/positions/
	// transactions/recommendations tables (none of which have a market
	// column yet), plus a net_worth_snapshots row under the old
	// single-column (date) primary key.
	legacyInserts := []string{
		`INSERT INTO watchlist (ticker) VALUES ('AAPL')`,
		`INSERT INTO watchlist (ticker) VALUES ('2330')`,
		`INSERT INTO positions (ticker, shares, avg_cost) VALUES ('AAPL', 10, 150)`,
		`INSERT INTO positions (ticker, shares, avg_cost) VALUES ('2330', 1000, 900)`,
		`INSERT INTO transactions (ticker, side, shares, price, fee, date) VALUES ('AAPL', 'BUY', 10, 150, 0, '2026-07-01')`,
		`INSERT INTO transactions (ticker, side, shares, price, fee, date) VALUES ('2330', 'BUY', 1000, 900, 0, '2026-07-01')`,
		`INSERT INTO recommendations (date, ticker, action, reason, price, source) VALUES ('2026-07-01', 'AAPL', 'BUY', 'test', 150, 'watchlist')`,
		`INSERT INTO recommendations (date, ticker, action, reason, price, source) VALUES ('2026-07-01', '2330', 'BUY', 'test', 900, 'watchlist')`,
		`INSERT INTO net_worth_snapshots (date, total_value) VALUES ('2026-07-01', 1500)`,
	}
	for _, ins := range legacyInserts {
		if _, err := conn.Exec(ins); err != nil {
			t.Fatalf("legacy insert %q: %v", ins, err)
		}
	}

	// Now apply the rest — migration 12, the market-column/net_worth rebuild.
	if err := d.migrate(); err != nil {
		t.Fatalf("migrate() error = %v", err)
	}

	watchlistUS, err := d.GetWatchlistByMarket(market.US)
	if err != nil || len(watchlistUS) != 1 || watchlistUS[0] != "AAPL" {
		t.Errorf("GetWatchlistByMarket(US) = %v, %v; want [AAPL], nil", watchlistUS, err)
	}
	watchlistTW, err := d.GetWatchlistByMarket(market.TW)
	if err != nil || len(watchlistTW) != 1 || watchlistTW[0] != "2330" {
		t.Errorf("GetWatchlistByMarket(TW) = %v, %v; want [2330], nil", watchlistTW, err)
	}

	posUS, ok, err := d.GetPosition("AAPL")
	if err != nil || !ok || posUS.Market != "us" {
		t.Errorf("GetPosition(AAPL) = %+v, %v, %v; want Market=us", posUS, ok, err)
	}
	posTW, ok, err := d.GetPosition("2330")
	if err != nil || !ok || posTW.Market != "tw" {
		t.Errorf("GetPosition(2330) = %+v, %v, %v; want Market=tw", posTW, ok, err)
	}

	txsUS, err := d.GetTransactions("AAPL")
	if err != nil || len(txsUS) != 1 || txsUS[0].Market != "us" {
		t.Errorf("GetTransactions(AAPL) = %+v, %v; want one row with Market=us", txsUS, err)
	}
	txsTW, err := d.GetTransactions("2330")
	if err != nil || len(txsTW) != 1 || txsTW[0].Market != "tw" {
		t.Errorf("GetTransactions(2330) = %+v, %v; want one row with Market=tw", txsTW, err)
	}

	recs, err := d.GetRecommendationsSince("2026-07-01")
	if err != nil || len(recs) != 2 {
		t.Fatalf("GetRecommendationsSince() = %v, %v; want 2 rows", recs, err)
	}
	for _, r := range recs {
		wantMarket := "us"
		if r.Ticker == "2330" {
			wantMarket = "tw"
		}
		if r.Market != wantMarket {
			t.Errorf("recommendation %s Market = %q, want %q", r.Ticker, r.Market, wantMarket)
		}
	}

	// The pre-migration net_worth_snapshots row must survive the table
	// rebuild, backfilled to market='us' (the only market that existed
	// before Phase 6).
	date, total, ok, err := d.GetLatestNetWorth(market.US)
	if err != nil || !ok || date != "2026-07-01" || total != 1500 {
		t.Errorf("GetLatestNetWorth(US) = %q, %v, %v, %v; want \"2026-07-01\", 1500, true, nil", date, total, ok, err)
	}
	if _, _, ok, err := d.GetLatestNetWorth(market.TW); err != nil || ok {
		t.Errorf("GetLatestNetWorth(TW) = _, _, %v, %v; want ok=false (no TW row existed pre-migration)", ok, err)
	}

	// A post-migration write for the other market must coexist at the same
	// date without clobbering the backfilled US row — the whole point of the
	// (date, market) composite PK.
	if err := d.SaveNetWorthSnapshot("2026-07-01", market.TW, 950000); err != nil {
		t.Fatalf("SaveNetWorthSnapshot(TW) error = %v", err)
	}
	if _, total, ok, err := d.GetLatestNetWorth(market.US); err != nil || !ok || total != 1500 {
		t.Errorf("GetLatestNetWorth(US) after a same-date TW write = %v, %v, %v; want unaffected 1500", total, ok, err)
	}
	if _, total, ok, err := d.GetLatestNetWorth(market.TW); err != nil || !ok || total != 950000 {
		t.Errorf("GetLatestNetWorth(TW) after write = %v, %v, %v; want 950000", total, ok, err)
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

// TestUniverseRemoveIsTombstoneNotDelete verifies migration 9's soft-delete:
// RemoveUniverseTicker must not erase the row (SyncSP500's stick guarantee
// depends on the tombstone still being there to check against).
func TestUniverseRemoveIsTombstoneNotDelete(t *testing.T) {
	d := newTestDB(t)

	if err := d.AddUniverseTicker("ZZZZ", "manual"); err != nil {
		t.Fatalf("AddUniverseTicker() error = %v", err)
	}
	if err := d.RemoveUniverseTicker("ZZZZ"); err != nil {
		t.Fatalf("RemoveUniverseTicker() error = %v", err)
	}

	var removed bool
	if err := d.conn.QueryRow(`SELECT removed FROM universe WHERE ticker = 'ZZZZ'`).Scan(&removed); err != nil {
		t.Fatalf("row for ZZZZ should still exist after RemoveUniverseTicker(): %v", err)
	}
	if !removed {
		t.Error("ZZZZ's removed flag should be 1 after RemoveUniverseTicker()")
	}

	// Adding it back must clear the tombstone, not silently no-op.
	if err := d.AddUniverseTicker("ZZZZ", "manual"); err != nil {
		t.Fatalf("AddUniverseTicker() (re-add) error = %v", err)
	}
	entries, err := d.GetUniverse()
	if err != nil {
		t.Fatalf("GetUniverse() error = %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Ticker == "ZZZZ" {
			found = true
		}
	}
	if !found {
		t.Error("ZZZZ should be active again in GetUniverse() after re-adding a tombstoned ticker")
	}
}

func TestSyncSP500NewTickerAutoAdded(t *testing.T) {
	d := newTestDB(t)

	entries, err := d.GetUniverse()
	if err != nil {
		t.Fatalf("GetUniverse() error = %v", err)
	}
	missingTicker := entries[0].Ticker
	if _, err := d.conn.Exec(`DELETE FROM universe WHERE ticker = ?`, missingTicker); err != nil {
		t.Fatalf("simulating a never-seeded ticker: %v", err)
	}

	added, delisted, err := d.SyncSP500()
	if err != nil {
		t.Fatalf("SyncSP500() error = %v", err)
	}
	if len(delisted) != 0 {
		t.Errorf("SyncSP500() delisted = %v, want empty", delisted)
	}
	found := false
	for _, ticker := range added {
		if ticker == missingTicker {
			found = true
		}
	}
	if !found {
		t.Fatalf("SyncSP500() added = %v, want it to contain %q", added, missingTicker)
	}

	after, err := d.GetUniverse()
	if err != nil {
		t.Fatalf("GetUniverse() error = %v", err)
	}
	if len(after) != len(entries) {
		t.Errorf("GetUniverse() len after sync = %d, want %d (missing ticker restored)", len(after), len(entries))
	}
}

func TestSyncSP500TombstonedTickerSticks(t *testing.T) {
	d := newTestDB(t)

	entries, err := d.GetUniverse()
	if err != nil {
		t.Fatalf("GetUniverse() error = %v", err)
	}
	removedTicker := entries[0].Ticker
	if err := d.RemoveUniverseTicker(removedTicker); err != nil {
		t.Fatalf("RemoveUniverseTicker() error = %v", err)
	}

	added, delisted, err := d.SyncSP500()
	if err != nil {
		t.Fatalf("SyncSP500() error = %v", err)
	}
	for _, ticker := range added {
		if ticker == removedTicker {
			t.Fatalf("SyncSP500() added = %v, must not re-add a tombstoned ticker (%q)", added, removedTicker)
		}
	}
	for _, ticker := range delisted {
		if ticker == removedTicker {
			t.Fatalf("SyncSP500() delisted = %v, a tombstoned ticker must be skipped by the diff entirely (%q)", delisted, removedTicker)
		}
	}

	after, err := d.GetUniverse()
	if err != nil {
		t.Fatalf("GetUniverse() error = %v", err)
	}
	for _, e := range after {
		if e.Ticker == removedTicker {
			t.Errorf("GetUniverse() after SyncSP500() should still exclude the tombstoned ticker %q", removedTicker)
		}
	}
}

func TestSyncSP500DelistedReportedButRowNotDeleted(t *testing.T) {
	d := newTestDB(t)

	if _, err := d.conn.Exec(`INSERT INTO universe (ticker, source) VALUES ('FAKEDELISTED', 'sp500')`); err != nil {
		t.Fatalf("inserting a fake sp500-sourced ticker: %v", err)
	}

	added, delisted, err := d.SyncSP500()
	if err != nil {
		t.Fatalf("SyncSP500() error = %v", err)
	}
	for _, ticker := range added {
		if ticker == "FAKEDELISTED" {
			t.Fatalf("SyncSP500() added = %v, should not include an already-present ticker", added)
		}
	}
	found := false
	for _, ticker := range delisted {
		if ticker == "FAKEDELISTED" {
			found = true
		}
	}
	if !found {
		t.Fatalf("SyncSP500() delisted = %v, want it to contain FAKEDELISTED", delisted)
	}

	var removed bool
	if err := d.conn.QueryRow(`SELECT removed FROM universe WHERE ticker = 'FAKEDELISTED'`).Scan(&removed); err != nil {
		t.Fatalf("FAKEDELISTED row should still exist after SyncSP500(): %v", err)
	}
	if removed {
		t.Error("SyncSP500() must never set removed on a delisted ticker — only the user's /universe remove does that")
	}
}

func TestSyncSP500Idempotent(t *testing.T) {
	d := newTestDB(t)

	// Fresh DB is seeded to exactly match the embedded list — no drift at
	// all — so the first call should already be a no-op, and a second call
	// must agree.
	added1, delisted1, err := d.SyncSP500()
	if err != nil {
		t.Fatalf("SyncSP500() (first run) error = %v", err)
	}
	if len(added1) != 0 || len(delisted1) != 0 {
		t.Fatalf("SyncSP500() (first run) = %v, %v, want both empty on a freshly seeded DB", added1, delisted1)
	}

	added2, delisted2, err := d.SyncSP500()
	if err != nil {
		t.Fatalf("SyncSP500() (second run) error = %v", err)
	}
	if len(added2) != 0 || len(delisted2) != 0 {
		t.Fatalf("SyncSP500() (second run) = %v, %v, want both empty (idempotent)", added2, delisted2)
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

func TestGetTransactions(t *testing.T) {
	d := newTestDB(t)

	if txs, err := d.GetTransactions("AAPL"); err != nil || len(txs) != 0 {
		t.Fatalf("GetTransactions() before any trade: txs = %v, err = %v; want empty, nil", txs, err)
	}

	if _, err := d.RecordBuy("AAPL", 10, 200, 1, "2026-07-01"); err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}
	if _, _, err := d.RecordSell("AAPL", 4, 220, 0.5, "2026-07-10"); err != nil {
		t.Fatalf("RecordSell() error = %v", err)
	}
	// A different ticker's trades must not leak in.
	if _, err := d.RecordBuy("MSFT", 5, 300, 0, "2026-07-05"); err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}

	txs, err := d.GetTransactions("AAPL")
	if err != nil {
		t.Fatalf("GetTransactions() error = %v", err)
	}
	if len(txs) != 2 {
		t.Fatalf("GetTransactions() len = %d, want 2", len(txs))
	}
	if txs[0].Side != "BUY" || txs[0].Shares != 10 || txs[0].Price != 200 {
		t.Errorf("GetTransactions()[0] = %+v, want the BUY leg first (oldest date)", txs[0])
	}
	if txs[1].Side != "SELL" || txs[1].Shares != 4 || txs[1].Price != 220 {
		t.Errorf("GetTransactions()[1] = %+v, want the SELL leg second", txs[1])
	}
}

func TestGetCloseExtremes(t *testing.T) {
	d := newTestDB(t)

	if _, _, ok, err := d.GetCloseExtremes("AAPL", "2026-07-01", "2026-07-10"); err != nil || ok {
		t.Fatalf("GetCloseExtremes() before any snapshot: ok = %v, err = %v; want false, nil", ok, err)
	}

	for _, s := range []DailySnapshot{
		{Ticker: "AAPL", Date: "2026-06-25", Close: 999}, // before the window, must be excluded
		{Ticker: "AAPL", Date: "2026-07-01", Close: 200},
		{Ticker: "AAPL", Date: "2026-07-03", Close: 230},
		{Ticker: "AAPL", Date: "2026-07-05", Close: 190},
		{Ticker: "AAPL", Date: "2026-07-15", Close: 999}, // after the window, must be excluded
	} {
		if err := d.SaveSnapshot(s); err != nil {
			t.Fatalf("SaveSnapshot() error = %v", err)
		}
	}

	high, low, ok, err := d.GetCloseExtremes("AAPL", "2026-07-01", "2026-07-10")
	if err != nil || !ok || high != 230 || low != 190 {
		t.Errorf("GetCloseExtremes() = %v, %v, %v, %v; want 230, 190, true, nil", high, low, ok, err)
	}
}

func TestGetRecommendationsForTicker(t *testing.T) {
	d := newTestDB(t)

	if recs, err := d.GetRecommendationsForTicker("AAPL", "2026-07-01", "2026-07-10"); err != nil || len(recs) != 0 {
		t.Fatalf("GetRecommendationsForTicker() before any rec: recs = %v, err = %v; want empty, nil", recs, err)
	}

	if err := d.SaveRecommendations("2026-06-20", []Recommendation{{Ticker: "AAPL", Action: "BUY", Reason: "early"}}); err != nil {
		t.Fatalf("SaveRecommendations() error = %v", err)
	}
	if err := d.SaveRecommendations("2026-07-05", []Recommendation{
		{Ticker: "AAPL", Action: "HOLD", Reason: "in range"},
		{Ticker: "MSFT", Action: "BUY", Reason: "other ticker"},
	}); err != nil {
		t.Fatalf("SaveRecommendations() error = %v", err)
	}
	if err := d.SaveRecommendations("2026-07-20", []Recommendation{{Ticker: "AAPL", Action: "SELL", Reason: "too late"}}); err != nil {
		t.Fatalf("SaveRecommendations() error = %v", err)
	}

	recs, err := d.GetRecommendationsForTicker("AAPL", "2026-07-01", "2026-07-10")
	if err != nil {
		t.Fatalf("GetRecommendationsForTicker() error = %v", err)
	}
	if len(recs) != 1 || recs[0].Action != "HOLD" || recs[0].Reason != "in range" {
		t.Errorf("GetRecommendationsForTicker() = %+v, want exactly the in-window AAPL HOLD row", recs)
	}
}

func TestGetAllTransactions(t *testing.T) {
	d := newTestDB(t)

	if txs, err := d.GetAllTransactions(); err != nil || len(txs) != 0 {
		t.Fatalf("GetAllTransactions() before any trade: txs = %v, err = %v; want empty, nil", txs, err)
	}

	if _, err := d.RecordBuy("MSFT", 5, 300, 0, "2026-07-05"); err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}
	if _, err := d.RecordBuy("AAPL", 10, 200, 1, "2026-07-01"); err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}
	if _, _, err := d.RecordSell("AAPL", 4, 220, 0.5, "2026-07-10"); err != nil {
		t.Fatalf("RecordSell() error = %v", err)
	}

	txs, err := d.GetAllTransactions()
	if err != nil {
		t.Fatalf("GetAllTransactions() error = %v", err)
	}
	if len(txs) != 3 {
		t.Fatalf("GetAllTransactions() len = %d, want 3 (across every ticker)", len(txs))
	}
	// Ordered by date across tickers, not grouped by ticker: AAPL BUY
	// (07-01) before MSFT BUY (07-05) before AAPL SELL (07-10).
	if txs[0].Ticker != "AAPL" || txs[0].Side != "BUY" || txs[0].Date != "2026-07-01" {
		t.Errorf("GetAllTransactions()[0] = %+v, want AAPL BUY 2026-07-01 (oldest date first)", txs[0])
	}
	if txs[1].Ticker != "MSFT" || txs[1].Date != "2026-07-05" {
		t.Errorf("GetAllTransactions()[1] = %+v, want MSFT 2026-07-05", txs[1])
	}
	if txs[2].Ticker != "AAPL" || txs[2].Side != "SELL" || txs[2].Date != "2026-07-10" {
		t.Errorf("GetAllTransactions()[2] = %+v, want AAPL SELL 2026-07-10", txs[2])
	}
}

func TestGetDailySnapshotsForTickers(t *testing.T) {
	d := newTestDB(t)

	if got, err := d.GetDailySnapshotsForTickers(nil, "2026-07-01", "2026-07-31"); err != nil || got != nil {
		t.Fatalf("GetDailySnapshotsForTickers(nil tickers) = %v, %v; want nil, nil", got, err)
	}

	snaps := []DailySnapshot{
		{Ticker: "AAPL", Date: "2026-07-01", Close: 200},
		{Ticker: "AAPL", Date: "2026-07-02", Close: 205},
		{Ticker: "MSFT", Date: "2026-07-01", Close: 300},
		{Ticker: "MSFT", Date: "2026-07-15", Close: 310}, // outside the queried window
		{Ticker: "NVDA", Date: "2026-07-01", Close: 900}, // not in the tickers filter
	}
	for _, s := range snaps {
		if err := d.SaveSnapshot(s); err != nil {
			t.Fatalf("SaveSnapshot(%+v) error = %v", s, err)
		}
	}

	got, err := d.GetDailySnapshotsForTickers([]string{"AAPL", "MSFT"}, "2026-07-01", "2026-07-05")
	if err != nil {
		t.Fatalf("GetDailySnapshotsForTickers() error = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("GetDailySnapshotsForTickers() len = %d, want 3 (AAPL x2, MSFT x1 in-window; NVDA excluded, MSFT 07-15 out of range)", len(got))
	}
	if got[0].Ticker != "AAPL" || got[0].Date != "2026-07-01" || got[0].Close != 200 {
		t.Errorf("GetDailySnapshotsForTickers()[0] = %+v, want AAPL 2026-07-01 close 200 (ordered by ticker, date)", got[0])
	}
	if got[1].Ticker != "AAPL" || got[1].Date != "2026-07-02" {
		t.Errorf("GetDailySnapshotsForTickers()[1] = %+v, want AAPL 2026-07-02", got[1])
	}
	if got[2].Ticker != "MSFT" || got[2].Date != "2026-07-01" {
		t.Errorf("GetDailySnapshotsForTickers()[2] = %+v, want MSFT 2026-07-01", got[2])
	}
}
