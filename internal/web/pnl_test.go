package web

import (
	"testing"
	"time"

	"argus/internal/db"
)

func tx(ticker, side string, shares, price float64, date string) db.Transaction {
	return db.Transaction{Ticker: ticker, Side: side, Shares: shares, Price: price, Date: date}
}

func snap(ticker, date string, close float64) db.DailySnapshot {
	return db.DailySnapshot{Ticker: ticker, Date: date, Close: close}
}

func TestDailyPnL_Empty(t *testing.T) {
	if got := DailyPnL(nil, nil); got != nil {
		t.Errorf("DailyPnL(nil, nil) = %v, want nil", got)
	}
}

func TestDailyPnL_BuyThenNextDaySell(t *testing.T) {
	txs := []db.Transaction{
		tx("AAPL", "BUY", 10, 100, "2026-07-01"),
		tx("AAPL", "SELL", 10, 115, "2026-07-02"),
	}
	snaps := []db.DailySnapshot{
		snap("AAPL", "2026-07-01", 105),
		snap("AAPL", "2026-07-02", 110),
	}

	got := DailyPnL(txs, snaps)
	if len(got) != 2 {
		t.Fatalf("DailyPnL() len = %d, want 2", len(got))
	}
	// Day 1: no prior close to mark-to-market against (position opened
	// today) — only the buy correction: (close 105 - buy 100) * 10 = 50.
	if got[0].Date != "2026-07-01" || got[0].Value != 50 {
		t.Errorf("DailyPnL()[0] = %+v, want {2026-07-01 50}", got[0])
	}
	// Day 2: mark-to-market on the 10 shares held coming in, (110-105)*10=50,
	// plus the sell correction (115-110)*10=50 → 100.
	if got[1].Date != "2026-07-02" || got[1].Value != 100 {
		t.Errorf("DailyPnL()[1] = %+v, want {2026-07-02 100}", got[1])
	}

	// Sanity: the sum of daily P&L across a fully closed round trip must
	// equal db.RecordSell's own realized_pnl formula: (115-100)*10 = 150.
	var total float64
	for _, d := range got {
		total += d.Value
	}
	if total != 150 {
		t.Errorf("sum of DailyPnL = %v, want 150 (matches RecordSell's realized_pnl for this round trip)", total)
	}
}

func TestDailyPnL_SkipsGapWithoutSmearing(t *testing.T) {
	// MSFT has no snapshot on day2 (e.g. briefly /remove'd from the
	// watchlist); AAPL does, which pulls day2 into the shared date axis.
	// MSFT's own delta must land entirely on day3 against its own last
	// known close (day1), not get attributed to day2, and AAPL's presence
	// on day2 must not change that.
	txs := []db.Transaction{
		tx("MSFT", "BUY", 5, 200, "2026-07-01"),
		tx("AAPL", "BUY", 1, 50, "2026-07-01"),
	}
	snaps := []db.DailySnapshot{
		snap("MSFT", "2026-07-01", 200),
		snap("MSFT", "2026-07-03", 220), // gap on 07-02
		snap("AAPL", "2026-07-01", 50),
		snap("AAPL", "2026-07-02", 51),
		snap("AAPL", "2026-07-03", 52),
	}

	got := DailyPnL(txs, snaps)
	byDate := map[string]float64{}
	for _, d := range got {
		byDate[d.Date] = d.Value
	}
	if len(got) != 3 {
		t.Fatalf("DailyPnL() len = %d, want 3 (07-01/02/03)", len(got))
	}
	// Day 1: both buy corrections, both bought at exactly that day's close → 0.
	if byDate["2026-07-01"] != 0 {
		t.Errorf("day1 = %v, want 0 (bought at close price)", byDate["2026-07-01"])
	}
	// Day 2: only AAPL has a close; its delta is (51-50)*1=1. MSFT
	// contributes nothing (no snapshot that day).
	if byDate["2026-07-02"] != 1 {
		t.Errorf("day2 = %v, want 1 (AAPL-only delta, MSFT gap contributes 0)", byDate["2026-07-02"])
	}
	// Day 3: MSFT's full gap move (220-200)*5=100 lands here (against its
	// own last known close, day1 — not smeared across day2), plus AAPL's
	// (52-51)*1=1.
	if byDate["2026-07-03"] != 101 {
		t.Errorf("day3 = %v, want 101 (MSFT's gap delta 100 + AAPL's 1)", byDate["2026-07-03"])
	}
}

func TestCumulativeCurve(t *testing.T) {
	daily := []DateValue{{Date: "d1", Value: 10}, {Date: "d2", Value: -3}, {Date: "d3", Value: 5}}
	got := CumulativeCurve(daily)
	want := []DateValue{{Date: "d1", Value: 10}, {Date: "d2", Value: 7}, {Date: "d3", Value: 12}}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("CumulativeCurve()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestMaxDrawdownAbs(t *testing.T) {
	tests := []struct {
		name string
		vals []float64
		want float64
	}{
		{"empty", nil, 0},
		{"single point", []float64{5}, 0},
		{"monotonic up", []float64{0, 5, 10}, 0},
		{"peak then trough then recovery", []float64{0, 10, -5, 8}, 15}, // peak 10, trough -5
		{"negative peak", []float64{-2, -1, -8}, 7},                     // peak -1, trough -8
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			curve := make([]DateValue, len(tt.vals))
			for i, v := range tt.vals {
				curve[i] = DateValue{Date: "d", Value: v}
			}
			if got := MaxDrawdownAbs(curve); got != tt.want {
				t.Errorf("MaxDrawdownAbs(%v) = %v, want %v", tt.vals, got, tt.want)
			}
		})
	}
}

func TestFilterSells(t *testing.T) {
	txs := []db.Transaction{
		tx("AAPL", "BUY", 1, 100, "d1"),
		tx("AAPL", "SELL", 1, 110, "d2"),
		tx("MSFT", "SELL", 1, 90, "d3"),
	}
	got := FilterSells(txs)
	if len(got) != 2 || got[0].Ticker != "AAPL" || got[1].Ticker != "MSFT" {
		t.Errorf("FilterSells() = %+v, want the two SELL rows in order", got)
	}
}

func TestWinRate(t *testing.T) {
	if got := WinRate(nil); got != 0 {
		t.Errorf("WinRate(nil) = %v, want 0", got)
	}
	sells := []db.Transaction{
		{RealizedPnL: 10},
		{RealizedPnL: -5},
		{RealizedPnL: 3},
		{RealizedPnL: 0},
	}
	if got := WinRate(sells); got != 0.5 {
		t.Errorf("WinRate() = %v, want 0.5 (2 of 4 positive)", got)
	}
}

func TestProfitFactor(t *testing.T) {
	if got := ProfitFactor(nil); got != 0 {
		t.Errorf("ProfitFactor(nil) = %v, want 0", got)
	}
	allWins := []db.Transaction{{RealizedPnL: 10}, {RealizedPnL: 5}}
	if got := ProfitFactor(allWins); got != 0 {
		t.Errorf("ProfitFactor(all wins) = %v, want 0 (no losses to divide by)", got)
	}
	mixed := []db.Transaction{{RealizedPnL: 20}, {RealizedPnL: -10}}
	if got := ProfitFactor(mixed); got != 2 {
		t.Errorf("ProfitFactor(mixed) = %v, want 2 (20/10)", got)
	}
}

func TestExpectancy(t *testing.T) {
	if got := Expectancy(nil); got != 0 {
		t.Errorf("Expectancy(nil) = %v, want 0", got)
	}
	sells := []db.Transaction{{RealizedPnL: 10}, {RealizedPnL: -4}, {RealizedPnL: 6}}
	if got := Expectancy(sells); got != 4 {
		t.Errorf("Expectancy() = %v, want 4 ((10-4+6)/3)", got)
	}
}

func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return d
}

func TestYTDStart(t *testing.T) {
	tests := []struct{ now, want string }{
		{"2026-07-15", "2026-01-01"},
		{"2026-01-01", "2026-01-01"},
		{"2026-12-31", "2026-01-01"},
	}
	for _, tt := range tests {
		if got := YTDStart(mustDate(t, tt.now)); got != tt.want {
			t.Errorf("YTDStart(%s) = %s, want %s", tt.now, got, tt.want)
		}
	}
}

func TestQTDStart(t *testing.T) {
	tests := []struct{ now, want string }{
		{"2026-01-15", "2026-01-01"},
		{"2026-03-31", "2026-01-01"},
		{"2026-04-01", "2026-04-01"},
		{"2026-06-30", "2026-04-01"},
		{"2026-07-01", "2026-07-01"},
		{"2026-09-30", "2026-07-01"},
		{"2026-10-01", "2026-10-01"},
		{"2026-12-31", "2026-10-01"},
	}
	for _, tt := range tests {
		if got := QTDStart(mustDate(t, tt.now)); got != tt.want {
			t.Errorf("QTDStart(%s) = %s, want %s", tt.now, got, tt.want)
		}
	}
}

func TestHTDStart(t *testing.T) {
	tests := []struct{ now, want string }{
		{"2026-01-01", "2026-01-01"},
		{"2026-06-30", "2026-01-01"},
		{"2026-07-01", "2026-07-01"},
		{"2026-12-31", "2026-07-01"},
	}
	for _, tt := range tests {
		if got := HTDStart(mustDate(t, tt.now)); got != tt.want {
			t.Errorf("HTDStart(%s) = %s, want %s", tt.now, got, tt.want)
		}
	}
}

func TestCurveValueBefore(t *testing.T) {
	curve := []DateValue{
		{Date: "2026-01-05", Value: 10},
		{Date: "2026-02-10", Value: 25},
		{Date: "2026-03-20", Value: 15},
	}
	if got := curveValueBefore(curve, "2026-03-01"); got != 25 {
		t.Errorf("curveValueBefore(before 03-01) = %v, want 25 (last point strictly before)", got)
	}
	if got := curveValueBefore(curve, "2026-01-01"); got != 0 {
		t.Errorf("curveValueBefore(before everything) = %v, want 0", got)
	}
	if got := curveValueBefore(nil, "2026-01-01"); got != 0 {
		t.Errorf("curveValueBefore(nil) = %v, want 0", got)
	}
}

func TestPeriodReturnPct(t *testing.T) {
	curve := []DateValue{
		{Date: "2025-12-31", Value: 100},
		{Date: "2026-06-15", Value: 150},
	}

	// Baseline before the curve's history starts: period P&L is the full
	// curve (nothing to subtract), 50/1000*100 = 5%.
	if pct, ok := PeriodReturnPct(curve, "2026-01-01", 1000, true); !ok || pct != 5 {
		t.Errorf("PeriodReturnPct(baseline before curve) = (%v, %v), want (5, true)", pct, ok)
	}

	// haveBaseline false → not computable.
	if _, ok := PeriodReturnPct(curve, "2026-01-01", 1000, false); ok {
		t.Errorf("PeriodReturnPct(no baseline) ok = true, want false")
	}

	// baseline == 0 → not computable (division would be meaningless).
	if _, ok := PeriodReturnPct(curve, "2026-01-01", 0, true); ok {
		t.Errorf("PeriodReturnPct(zero baseline) ok = true, want false")
	}

	// periodStart predates the whole curve (every curve point postdates
	// it): curveValueBefore returns 0, so period P&L is the full curve
	// range (150), 150/500*100 = 30%.
	if pct, ok := PeriodReturnPct(curve, "2025-01-01", 500, true); !ok || pct != 30 {
		t.Errorf("PeriodReturnPct(period predates history) = (%v, %v), want (30, true)", pct, ok)
	}
}
