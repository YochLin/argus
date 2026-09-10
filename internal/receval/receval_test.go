package receval

import (
	"testing"
	"time"

	"argus/internal/data"
)

// dailyCandles builds n consecutive daily candles starting at startDate,
// each day's close/high/low derived from closes[i] (open/volume unused by
// this package, left zero).
func dailyCandles(startDate string, closes []float64) []data.Candle {
	start, _ := time.Parse("2006-01-02", startDate)
	out := make([]data.Candle, len(closes))
	for i, c := range closes {
		out[i] = data.Candle{
			Date:  start.AddDate(0, 0, i),
			Close: c,
			High:  c * 1.02,
			Low:   c * 0.98,
		}
	}
	return out
}

func TestScoreBasicReturnAndFallbackEntryPrice(t *testing.T) {
	candles := dailyCandles("2026-01-01", []float64{100, 101, 102, 103, 104, 110, 120})
	rec := Recommendation{Date: "2026-01-01", Ticker: "AAPL", Action: "BUY", Price: 0}

	sr := Score(rec, candles, nil, []int{5})
	if sr.Unscorable {
		t.Fatalf("unexpected unscorable: %s", sr.Reason)
	}
	if sr.EntryPrice != 100 {
		t.Fatalf("EntryPrice = %v, want 100 (fallback to entry candle close)", sr.EntryPrice)
	}
	if len(sr.Windows) != 1 || !sr.Windows[0].Matured {
		t.Fatalf("Windows = %+v, want one matured window", sr.Windows)
	}
	want := (110.0 - 100.0) / 100.0 * 100
	if sr.Windows[0].TickerReturnPct != want {
		t.Errorf("TickerReturnPct = %v, want %v", sr.Windows[0].TickerReturnPct, want)
	}
	if sr.Windows[0].HaveBench {
		t.Errorf("HaveBench = true with no bench candles passed")
	}
	if !sr.Windows[0].Hit {
		t.Errorf("BUY with positive absolute return should hit when no benchmark is available")
	}
}

func TestScoreUsesStoredPriceOverCandleClose(t *testing.T) {
	candles := dailyCandles("2026-01-01", []float64{100, 101, 102, 103, 104, 105})
	rec := Recommendation{Date: "2026-01-01", Ticker: "AAPL", Action: "BUY", Price: 90}

	sr := Score(rec, candles, nil, []int{5})
	if sr.EntryPrice != 90 {
		t.Fatalf("EntryPrice = %v, want stored Price 90", sr.EntryPrice)
	}
}

func TestScoreImmatureWindow(t *testing.T) {
	candles := dailyCandles("2026-01-01", []float64{100, 101, 102})
	rec := Recommendation{Date: "2026-01-01", Ticker: "AAPL", Action: "BUY", Price: 100}

	sr := Score(rec, candles, nil, []int{5, 20})
	if sr.Unscorable {
		t.Fatalf("unexpected unscorable: %s", sr.Reason)
	}
	for _, w := range sr.Windows {
		if w.Matured {
			t.Errorf("horizon %d matured with only %d candles, want immature", w.Horizon, len(candles))
		}
	}
}

func TestScoreBenchmarkExcessAndFallback(t *testing.T) {
	candles := dailyCandles("2026-01-01", []float64{100, 100, 100, 100, 100, 110})
	bench := dailyCandles("2026-01-01", []float64{200, 200, 200, 200, 200, 202})
	rec := Recommendation{Date: "2026-01-01", Ticker: "AAPL", Action: "BUY", Price: 100}

	sr := Score(rec, candles, bench, []int{5})
	w := sr.Windows[0]
	if !w.HaveBench {
		t.Fatalf("HaveBench = false, want true with aligned benchmark data")
	}
	wantTicker := 10.0
	wantBench := 1.0
	if w.TickerReturnPct != wantTicker || w.BenchReturnPct != wantBench {
		t.Fatalf("returns = ticker %v bench %v, want %v/%v", w.TickerReturnPct, w.BenchReturnPct, wantTicker, wantBench)
	}
	if w.ExcessReturnPct != wantTicker-wantBench {
		t.Errorf("ExcessReturnPct = %v, want %v", w.ExcessReturnPct, wantTicker-wantBench)
	}

	// SELL should miss (fail to hit) when it beat the benchmark going up.
	rec.Action = "SELL"
	sr = Score(rec, candles, bench, []int{5})
	if sr.Windows[0].Hit {
		t.Errorf("SELL beating benchmark upward should not be a hit")
	}

	// No overlapping benchmark data at all -> fall back to absolute return.
	sr = Score(Recommendation{Date: "2026-01-01", Ticker: "AAPL", Action: "BUY", Price: 100}, candles, nil, []int{5})
	if sr.Windows[0].HaveBench {
		t.Fatalf("HaveBench = true with nil bench")
	}
	if sr.Windows[0].ExcessReturnPct != sr.Windows[0].TickerReturnPct {
		t.Errorf("ExcessReturnPct should fall back to TickerReturnPct when no benchmark available")
	}
}

func TestScoreUnscorableTooOldAndNoHistory(t *testing.T) {
	candles := dailyCandles("2026-06-01", []float64{100, 101, 102})

	sr := Score(Recommendation{Date: "2026-01-01", Ticker: "AAPL", Action: "BUY"}, candles, nil, []int{5})
	if !sr.Unscorable {
		t.Fatalf("want unscorable when recommendation predates fetched range, got scored")
	}

	sr = Score(Recommendation{Date: "2026-01-01", Ticker: "AAPL", Action: "BUY"}, nil, nil, []int{5})
	if !sr.Unscorable {
		t.Fatalf("want unscorable with no history data")
	}
}

func TestScoreUnscorableRecommendationNewerThanHistory(t *testing.T) {
	candles := dailyCandles("2026-01-01", []float64{100, 101, 102})
	sr := Score(Recommendation{Date: "2026-06-01", Ticker: "AAPL", Action: "BUY"}, candles, nil, []int{5})
	if !sr.Unscorable {
		t.Fatalf("want unscorable when recommendation date is after every fetched candle")
	}
}

func TestMFEMAE(t *testing.T) {
	// entry at 100, then a spike to 130 (high 132.6) and a dip to 90 (low
	// 88.2) within the 5-day window.
	candles := dailyCandles("2026-01-01", []float64{100, 130, 95, 90, 105, 110})
	sr := Score(Recommendation{Date: "2026-01-01", Ticker: "AAPL", Action: "BUY", Price: 100}, candles, nil, []int{5})
	if sr.MFEPct <= 30 || sr.MFEPct >= 33 {
		t.Errorf("MFEPct = %v, want ~32.6 (high of day 2 vs entry 100)", sr.MFEPct)
	}
	if sr.MAEPct >= -10 || sr.MAEPct <= -12 {
		t.Errorf("MAEPct = %v, want ~-11.8 (low of day 4 vs entry 100)", sr.MAEPct)
	}
}

func TestCollapseRepeats(t *testing.T) {
	// Deliberately out of date order: CollapseRepeats must sort, not trust input order.
	recs := []Recommendation{
		{Date: "2026-07-03", Ticker: "AAPL", Action: "BUY"},
		{Date: "2026-07-01", Ticker: "AAPL", Action: "BUY"},
		{Date: "2026-07-02", Ticker: "AAPL", Action: "BUY"},
		{Date: "2026-07-02", Ticker: "MSFT", Action: "HOLD"},
		{Date: "2026-07-04", Ticker: "AAPL", Action: "HOLD"},
		{Date: "2026-07-05", Ticker: "MSFT", Action: "HOLD"},
		{Date: "2026-08-20", Ticker: "AAPL", Action: "HOLD"}, // gap doesn't break a run
		{Date: "2026-08-21", Ticker: "AAPL", Action: "BUY"},
	}
	kept, collapsed := CollapseRepeats(recs)
	var got []string
	for _, r := range kept {
		got = append(got, r.Date+" "+r.Ticker+" "+r.Action)
	}
	want := []string{
		"2026-07-01 AAPL BUY",
		"2026-07-02 MSFT HOLD",
		"2026-07-04 AAPL HOLD",
		"2026-08-21 AAPL BUY",
	}
	if len(got) != len(want) || collapsed != len(recs)-len(want) {
		t.Fatalf("kept = %v (collapsed %d), want %v (collapsed %d)", got, collapsed, want, len(recs)-len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("kept[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestScoreHoldHitIsBeatBenchmark(t *testing.T) {
	candles := dailyCandles("2026-01-01", []float64{100, 100, 100, 100, 100, 110})
	bench := dailyCandles("2026-01-01", []float64{100, 100, 100, 100, 100, 105})
	up := Score(Recommendation{Date: "2026-01-01", Ticker: "AAPL", Action: "HOLD", Price: 100}, candles, bench, []int{5})
	if !up.Windows[0].Hit {
		t.Errorf("HOLD that beat the benchmark (+10%% vs +5%%) should count as a hit")
	}
	down := Score(Recommendation{Date: "2026-01-01", Ticker: "AAPL", Action: "HOLD", Price: 100}, bench, candles, []int{5})
	if down.Windows[0].Hit {
		t.Errorf("HOLD that lagged the benchmark (+5%% vs +10%%) should not count as a hit")
	}
}
