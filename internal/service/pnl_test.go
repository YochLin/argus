package service

import (
	"math"
	"testing"
	"time"

	"argus/internal/db"
)

func TestDailyPnLAndCumulative(t *testing.T) {
	txs := []db.Transaction{
		makeTx("AAPL", "BUY", 10, 100, 0, 0, "2026-06-01"),
		makeTx("AAPL", "SELL", 10, 120, 0, 200, "2026-06-03"),
	}
	snaps := []db.DailySnapshot{
		{Ticker: "AAPL", Date: "2026-06-01", Close: 105},
		{Ticker: "AAPL", Date: "2026-06-02", Close: 110},
		{Ticker: "AAPL", Date: "2026-06-03", Close: 115},
	}

	daily := DailyPnL(txs, snaps)
	if len(daily) != 3 {
		t.Fatalf("DailyPnL len = %d, want 3", len(daily))
	}
	// Day 1: buy at 100, close at 105 -> (105-100)*10 = 50
	if math.Abs(daily[0].Value-50) > 1e-6 {
		t.Errorf("daily[0] = %v, want 50", daily[0].Value)
	}
	// Day 2: holding 10 shares from 105 to 110 -> 50
	if math.Abs(daily[1].Value-50) > 1e-6 {
		t.Errorf("daily[1] = %v, want 50", daily[1].Value)
	}
	// Day 3: holding 10 shares from 110 to 115 (+50) and sold at 120 (correction: (120-115)*10 = +50) -> 100
	if math.Abs(daily[2].Value-100) > 1e-6 {
		t.Errorf("daily[2] = %v, want 100", daily[2].Value)
	}

	curve := CumulativeCurve(daily)
	if len(curve) != 3 || math.Abs(curve[2].Value-200) > 1e-6 {
		t.Errorf("CumulativeCurve final value = %v, want 200", curve[2].Value)
	}

	ddSeries := DrawdownSeries(curve)
	for _, pt := range ddSeries {
		if pt.Value > 0 {
			t.Errorf("underwater series point > 0: %v", pt)
		}
	}
}

func TestFilterSellsAndKPIs(t *testing.T) {
	txs := []db.Transaction{
		makeTx("AAPL", "BUY", 10, 100, 0, 0, "2026-06-01"),
		makeTx("AAPL", "SELL", 5, 120, 0, 100, "2026-06-02"),
		makeTx("AAPL", "SELL", 5, 90, 0, -50, "2026-06-03"),
	}

	sells := FilterSells(txs)
	if len(sells) != 2 {
		t.Fatalf("FilterSells len = %d, want 2", len(sells))
	}

	if wr := WinRate(sells); wr != 0.5 {
		t.Errorf("WinRate = %v, want 0.5", wr)
	}

	if pf := ProfitFactor(sells); pf != 2.0 {
		t.Errorf("ProfitFactor = %v, want 2.0", pf)
	}

	if exp := Expectancy(sells); exp != 25.0 {
		t.Errorf("Expectancy = %v, want 25.0", exp)
	}
}

func TestPeriodStarts(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	if ytd := YTDStart(now); ytd != "2026-01-01" {
		t.Errorf("YTDStart = %q, want 2026-01-01", ytd)
	}
	if qtd := QTDStart(now); qtd != "2026-07-01" {
		t.Errorf("QTDStart = %q, want 2026-07-01", qtd)
	}
	if htd := HTDStart(now); htd != "2026-07-01" {
		t.Errorf("HTDStart = %q, want 2026-07-01", htd)
	}
}
