package web

import (
	"testing"

	"argus/internal/db"
	"argus/internal/market"
)

func TestSellReturnPct(t *testing.T) {
	t.Run("no fee", func(t *testing.T) {
		// BUY 10@100, SELL 10@120: realizedPnL = (120-100)*10 = 200.
		s := tx("AAPL", "SELL", 10, 120, "2026-06-10")
		s.RealizedPnL = 200
		pct, ok := sellReturnPct(s)
		if !ok {
			t.Fatal("sellReturnPct() ok = false, want true")
		}
		if want := 20.0; pct != want {
			t.Errorf("sellReturnPct() = %v, want %v", pct, want)
		}
	})

	t.Run("with fee", func(t *testing.T) {
		// avgCost 100.5 (100 + 5 buy-fee/10 shares), sell@120 fee 2:
		// realizedPnL = (120-100.5)*10 - 2 = 193. costBasis = 1200-2-193 = 1005.
		s := tx("AAPL", "SELL", 10, 120, "2026-06-10")
		s.Fee = 2
		s.RealizedPnL = 193
		pct, ok := sellReturnPct(s)
		if !ok {
			t.Fatal("sellReturnPct() ok = false, want true")
		}
		if want := 193.0 / 1005.0 * 100; pct != want {
			t.Errorf("sellReturnPct() = %v, want %v", pct, want)
		}
	})

	t.Run("zero cost basis guarded", func(t *testing.T) {
		s := tx("AAPL", "SELL", 10, 0, "2026-06-10")
		if _, ok := sellReturnPct(s); ok {
			t.Error("sellReturnPct() ok = true for non-positive cost basis, want false")
		}
	})
}

func TestHoldingDaysBucket(t *testing.T) {
	cases := []struct {
		days int
		want string
	}{
		{0, "0-5d"},
		{5, "0-5d"},
		{6, "6-20d"},
		{20, "6-20d"},
		{21, "21-60d"},
		{60, "21-60d"},
		{61, "60d+"},
		{400, "60d+"},
	}
	for _, c := range cases {
		if got := holdingDaysBucket(c.days); got != c.want {
			t.Errorf("holdingDaysBucket(%d) = %q, want %q", c.days, got, c.want)
		}
	}
}

func TestBuildSellRecords(t *testing.T) {
	t.Run("partial sells within one round get independent holding days off the same entry", func(t *testing.T) {
		txs := []db.Transaction{
			tx("AAPL", "BUY", 20, 100, "2026-06-01"), // a Monday
			tx("AAPL", "SELL", 10, 110, "2026-06-05"),
			tx("AAPL", "SELL", 10, 130, "2026-06-21"),
		}
		got, err := buildSellRecords(txs)
		if err != nil {
			t.Fatalf("buildSellRecords() error = %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("buildSellRecords() = %+v, want 2 records", got)
		}
		if got[0].HoldingDays != 4 {
			t.Errorf("first sell HoldingDays = %d, want 4 (06-01 to 06-05)", got[0].HoldingDays)
		}
		if got[1].HoldingDays != 20 {
			t.Errorf("second sell HoldingDays = %d, want 20 (06-01 to 06-21)", got[1].HoldingDays)
		}
		for _, r := range got {
			if r.EntryMonth.String() != "June" {
				t.Errorf("EntryMonth = %v, want June (round's own start month)", r.EntryMonth)
			}
		}
	})

	t.Run("open trailing round contributes no records", func(t *testing.T) {
		txs := []db.Transaction{tx("AAPL", "BUY", 10, 100, "2026-06-01")}
		got, err := buildSellRecords(txs)
		if err != nil {
			t.Fatalf("buildSellRecords() error = %v", err)
		}
		if len(got) != 0 {
			t.Errorf("buildSellRecords() = %+v, want empty (no SELL legs at all)", got)
		}
	})
}

func TestGroupReport_LowSample(t *testing.T) {
	var records []sellRecord
	for i := 0; i < 3; i++ {
		s := tx("AAPL", "SELL", 10, 110, "2026-06-05")
		s.RealizedPnL = 100
		records = append(records, sellRecord{Ticker: "AAPL", Sell: s})
	}
	got := groupReport(records, func(r sellRecord) string { return r.Ticker }, nil)
	if len(got) != 1 {
		t.Fatalf("groupReport() = %+v, want 1 group", got)
	}
	if !got[0].LowSample {
		t.Errorf("group with n=3 (< %d) LowSample = false, want true", minSampleSize)
	}
	if got[0].N != 3 {
		t.Errorf("group N = %d, want 3", got[0].N)
	}
}

func TestGroupReport_OrderedBuckets(t *testing.T) {
	records := []sellRecord{
		{Ticker: "AAPL", HoldingDays: 100, Sell: tx("AAPL", "SELL", 1, 1, "2026-01-01")}, // 60d+
		{Ticker: "AAPL", HoldingDays: 2, Sell: tx("AAPL", "SELL", 1, 1, "2026-01-01")},   // 0-5d
	}
	got := groupReport(records, func(r sellRecord) string { return holdingDaysBucket(r.HoldingDays) }, holdingDaysBucketOrder)
	if len(got) != 2 {
		t.Fatalf("groupReport() = %+v, want 2 buckets", got)
	}
	if got[0].Key != "0-5d" || got[1].Key != "60d+" {
		t.Errorf("bucket order = [%s, %s], want fixed order [0-5d, 60d+] not alphabetical/insertion", got[0].Key, got[1].Key)
	}
}

func TestBuildFeeSummary(t *testing.T) {
	txs := []db.Transaction{
		tx("AAPL", "BUY", 10, 100, "2026-06-01"),
		tx("AAPL", "SELL", 10, 120, "2026-06-10"),
	}
	txs[0].Fee = 5
	txs[1].Fee = 3
	txs[1].RealizedPnL = 192 // (120-100.5)*10 - 3, avgCost with buy fee folded in

	got := buildFeeSummary(txs)
	if got.TotalFees != 8 {
		t.Errorf("TotalFees = %v, want 8", got.TotalFees)
	}
	wantPct := 8.0 / 192.0 * 100
	if diff := got.PctOfRealizedPnL - wantPct; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("PctOfRealizedPnL = %v, want %v", got.PctOfRealizedPnL, wantPct)
	}
}

func TestBuildFeeSummary_NoRealizedPnLGuardsDivideByZero(t *testing.T) {
	txs := []db.Transaction{tx("AAPL", "BUY", 10, 100, "2026-06-01")}
	txs[0].Fee = 5
	got := buildFeeSummary(txs)
	if got.PctOfRealizedPnL != 0 {
		t.Errorf("PctOfRealizedPnL = %v, want 0 with no SELLs yet", got.PctOfRealizedPnL)
	}
}

func TestBuildStreakStats(t *testing.T) {
	mk := func(pnl float64) db.Transaction {
		s := tx("AAPL", "SELL", 1, 1, "2026-01-01")
		s.RealizedPnL = pnl
		return s
	}
	sells := []db.Transaction{mk(100), mk(50), mk(-20), mk(-30), mk(-10), mk(200)}
	got := buildStreakStats(sells)

	if got.BestTradePnL != 200 {
		t.Errorf("BestTradePnL = %v, want 200", got.BestTradePnL)
	}
	if got.WorstTradePnL != -30 {
		t.Errorf("WorstTradePnL = %v, want -30", got.WorstTradePnL)
	}
	if got.LongestWinStreak != 2 {
		t.Errorf("LongestWinStreak = %d, want 2 (first two trades)", got.LongestWinStreak)
	}
	if got.LongestLossStreak != 3 {
		t.Errorf("LongestLossStreak = %d, want 3 (middle three trades)", got.LongestLossStreak)
	}
	if want := (100.0 + 50.0 + 200.0) / 3; got.AvgWinPnL != want {
		t.Errorf("AvgWinPnL = %v, want %v", got.AvgWinPnL, want)
	}
	if want := (-20.0 - 30.0 - 10.0) / 3; got.AvgLossPnL != want {
		t.Errorf("AvgLossPnL = %v, want %v", got.AvgLossPnL, want)
	}
}

func TestBuildStreakStats_Empty(t *testing.T) {
	got := buildStreakStats(nil)
	if got.BestTradePnL != 0 || got.WorstTradePnL != 0 {
		t.Errorf("buildStreakStats(nil) = %+v, want all zero", got)
	}
}

func TestBuildReports_MarketFiltering(t *testing.T) {
	usSell := tx("AAPL", "SELL", 10, 120, "2026-06-10")
	usSell.RealizedPnL = 200
	twSell := tx("2330", "SELL", 10, 120, "2026-06-10")
	twSell.RealizedPnL = 50

	fdb := &fakeDB{
		txs: []db.Transaction{
			tx("AAPL", "BUY", 10, 100, "2026-06-01"),
			usSell,
			tx("2330", "BUY", 10, 100, "2026-06-01"),
			twSell,
		},
	}
	got, err := buildReports(fdb, &fakeHistory{}, market.US)
	if err != nil {
		t.Fatalf("buildReports() error = %v", err)
	}
	if len(got.ByTicker) != 1 || got.ByTicker[0].Key != "AAPL" {
		t.Errorf("ByTicker = %+v, want only AAPL (TW filtered out)", got.ByTicker)
	}
}

func TestBuildReports_ByTickerSortedByPnLDesc(t *testing.T) {
	loseSell := tx("MSFT", "SELL", 10, 90, "2026-06-10")
	loseSell.RealizedPnL = -100
	winSell := tx("AAPL", "SELL", 10, 120, "2026-06-10")
	winSell.RealizedPnL = 200

	fdb := &fakeDB{
		txs: []db.Transaction{
			tx("MSFT", "BUY", 10, 100, "2026-06-01"),
			loseSell,
			tx("AAPL", "BUY", 10, 100, "2026-06-01"),
			winSell,
		},
	}
	got, err := buildReports(fdb, &fakeHistory{}, market.US)
	if err != nil {
		t.Fatalf("buildReports() error = %v", err)
	}
	if len(got.ByTicker) != 2 || got.ByTicker[0].Key != "AAPL" || got.ByTicker[1].Key != "MSFT" {
		t.Errorf("ByTicker = %+v, want [AAPL(+200), MSFT(-100)]", got.ByTicker)
	}
}

func TestBuildReports_Empty(t *testing.T) {
	got, err := buildReports(&fakeDB{}, &fakeHistory{}, market.US)
	if err != nil {
		t.Fatalf("buildReports() error = %v", err)
	}
	if len(got.ByTicker) != 0 || len(got.ByHoldingDays) != 0 || len(got.ByEntryMonth) != 0 || len(got.ByEntryWeekday) != 0 {
		t.Errorf("buildReports() with no transactions = %+v, want all-empty groups", got)
	}
}
