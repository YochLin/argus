package web

import (
	"errors"
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/db"
)

func TestDailyCostBasis(t *testing.T) {
	legs := []db.Transaction{
		tx("AAPL", "BUY", 10, 100, "2026-06-01"),
		tx("AAPL", "BUY", 10, 120, "2026-06-05"), // avg becomes (10*100+10*120)/20 = 110
		tx("AAPL", "SELL", 5, 130, "2026-06-10"), // avg unchanged by a sell
	}
	got := dailyCostBasis(legs)
	if got["2026-06-01"] != 100 {
		t.Errorf("cost after first buy = %v, want 100", got["2026-06-01"])
	}
	if got["2026-06-05"] != 110 {
		t.Errorf("cost after second buy = %v, want 110", got["2026-06-05"])
	}
	if got["2026-06-10"] != 110 {
		t.Errorf("cost after sell = %v, want unchanged 110", got["2026-06-10"])
	}
}

func TestCostBasisOnOrBefore(t *testing.T) {
	checkpoints := map[string]float64{"2026-06-01": 100, "2026-06-10": 110}
	dates := []string{"2026-06-01", "2026-06-10"}

	if got, ok := costBasisOnOrBefore(dates, checkpoints, "2026-06-05"); !ok || got != 100 {
		t.Errorf("costBasisOnOrBefore(06-05) = (%v, %v), want (100, true)", got, ok)
	}
	if got, ok := costBasisOnOrBefore(dates, checkpoints, "2026-06-01"); !ok || got != 100 {
		t.Errorf("costBasisOnOrBefore(06-01, exact) = (%v, %v), want (100, true)", got, ok)
	}
	if _, ok := costBasisOnOrBefore(dates, checkpoints, "2026-05-01"); ok {
		t.Error("costBasisOnOrBefore before any checkpoint: ok = true, want false")
	}
}

func TestComputeMAEMFE(t *testing.T) {
	legs := []db.Transaction{tx("AAPL", "BUY", 10, 100, "2026-06-01")}
	candles := []data.Candle{
		{Date: parseDate("2026-06-01"), Low: 95, High: 105},
		{Date: parseDate("2026-06-05"), Low: 80, High: 130}, // -20%/+30% vs cost 100
		{Date: parseDate("2026-06-10"), Low: 90, High: 110},
		{Date: parseDate("2026-07-01"), Low: 1, High: 1000}, // outside window, must be excluded
	}
	got := computeMAEMFE(candles, legs, parseDate("2026-06-01"), parseDate("2026-06-10"))
	if !got.OK {
		t.Fatal("computeMAEMFE() OK = false, want true")
	}
	if got.MAEPct != -20 {
		t.Errorf("MAEPct = %v, want -20", got.MAEPct)
	}
	if got.MFEPct != 30 {
		t.Errorf("MFEPct = %v, want 30", got.MFEPct)
	}
}

func TestComputeMAEMFE_NoMatchingCandles(t *testing.T) {
	legs := []db.Transaction{tx("AAPL", "BUY", 10, 100, "2026-06-01")}
	got := computeMAEMFE(nil, legs, parseDate("2026-06-01"), parseDate("2026-06-10"))
	if got.OK {
		t.Error("computeMAEMFE() with no candles OK = true, want false")
	}
}

func TestRoundReturnPct(t *testing.T) {
	sell := tx("AAPL", "SELL", 10, 120, "2026-06-10")
	sell.RealizedPnL = 200
	legs := []db.Transaction{
		tx("AAPL", "BUY", 10, 100, "2026-06-01"),
		sell,
	}
	pct, ok := roundReturnPct(legs)
	if !ok {
		t.Fatal("roundReturnPct() ok = false, want true")
	}
	if want := 20.0; pct != want { // 200 realized / 1000 invested
		t.Errorf("roundReturnPct() = %v, want %v", pct, want)
	}
}

func TestRoundReturnPct_NoInvestmentGuarded(t *testing.T) {
	if _, ok := roundReturnPct(nil); ok {
		t.Error("roundReturnPct(nil) ok = true, want false")
	}
}

func TestRoundMAEMFE_OpenRoundUsesNow(t *testing.T) {
	legs := []db.Transaction{tx("AAPL", "BUY", 10, 100, "2026-06-01")}
	candles := []data.Candle{
		{Date: parseDate("2026-06-01"), Low: 90, High: 110},
		{Date: parseDate("2026-06-15"), Low: 80, High: 140},
	}
	now := parseDate("2026-06-20")
	got := roundMAEMFE(candles, legs, "2026-06-01", "", now)
	if !got.OK || got.MFEPct != 40 {
		t.Errorf("roundMAEMFE(open) = %+v, want OK with MFEPct 40 (both candles within [start, now])", got)
	}
}

func TestBuildMAEMFESummary(t *testing.T) {
	closedSell := tx("AAPL", "SELL", 10, 120, "2026-06-10")
	closedSell.RealizedPnL = 200 // 20% return
	txs := []db.Transaction{
		tx("AAPL", "BUY", 10, 100, "2026-06-01"),
		closedSell,
		tx("MSFT", "BUY", 5, 100, "2026-07-01"), // still open, must not contribute
	}
	hist := &fakeHistory{candles: map[string][]data.Candle{
		"AAPL": {
			{Date: parseDate("2026-06-01"), Low: 95, High: 105},
			{Date: parseDate("2026-06-10"), Low: 90, High: 140}, // MFE +40% vs cost 100
		},
	}}

	got := buildMAEMFESummary(txs, hist)
	if got.N != 1 {
		t.Fatalf("N = %d, want 1 (only the closed AAPL round)", got.N)
	}
	want := 20.0 / 40.0 * 100 // realized 20% captured of the 40% MFE
	if got.AvgCapturedPct != want {
		t.Errorf("AvgCapturedPct = %v, want %v", got.AvgCapturedPct, want)
	}
	if !got.LowSample {
		t.Error("LowSample = false with n=1, want true")
	}
}

func TestBuildMAEMFESummary_HistoryErrorDegradesGracefully(t *testing.T) {
	closedSell := tx("AAPL", "SELL", 10, 120, "2026-06-10")
	closedSell.RealizedPnL = 200
	txs := []db.Transaction{
		tx("AAPL", "BUY", 10, 100, "2026-06-01"),
		closedSell,
	}
	got := buildMAEMFESummary(txs, &fakeHistory{err: errors.New("yahoo down")})
	if got.N != 0 {
		t.Errorf("N = %d, want 0 when history fetch fails for the only ticker", got.N)
	}
}

func parseDate(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}
