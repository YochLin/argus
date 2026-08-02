package web

import (
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/market"
)

// seqCandles generates n consecutive daily candles (no weekend gaps, kept
// simple like this package's other fixtures) starting at start, with each
// day's close = startPrice + i*dailyChange.
func seqCandles(start time.Time, n int, startPrice, dailyChange float64) []data.Candle {
	out := make([]data.Candle, n)
	for i := 0; i < n; i++ {
		close := startPrice + float64(i)*dailyChange
		out[i] = data.Candle{Date: start.AddDate(0, 0, i), Open: close, High: close, Low: close, Close: close, Volume: 1000}
	}
	return out
}

func TestBuildRecPerformance(t *testing.T) {
	recDate, _ := time.Parse("2006-01-02", "2026-01-05")

	fdb := &fakeDB{
		recs: []db.Recommendation{
			{Date: "2026-01-05", Ticker: "AAPL", Action: "BUY", Price: 100, Source: "watchlist", Market: "us"},
			{Date: "2026-01-05", Ticker: "MSFT", Action: "HOLD", Price: 300, Source: "watchlist", Market: "us"},
			{Date: "2026-01-05", Ticker: "2330", Action: "BUY", Price: 900, Source: "scan", Market: "tw"},
		},
	}
	fh := &fakeHistory{candles: map[string][]data.Candle{
		"AAPL": seqCandles(recDate, 100, 100, 1),   // steady gainer
		"SPY":  seqCandles(recDate, 100, 500, 0.2), // benchmark, weaker gainer -> AAPL BUY should be a hit
		"2330": seqCandles(recDate, 100, 900, -1),  // steady loser
		"0050": seqCandles(recDate, 100, 100, 0),   // flat benchmark
	}}

	us, err := buildRecPerformance(fdb, fh, market.US)
	if err != nil {
		t.Fatalf("buildRecPerformance(US) error = %v", err)
	}

	// MSFT's HOLD isn't scorable; AAPL's BUY is — Total counts every stored
	// row for the market (including HOLD), Scorable only the BUY/SELL rows.
	if us.Counts.Total != 2 || us.Counts.Hold != 1 || us.Counts.Scorable != 1 {
		t.Errorf("Counts = %+v, want Total 2 / Hold 1 / Scorable 1", us.Counts)
	}
	if len(us.ByAction) != 1 || us.ByAction[0].Key != "BUY" {
		t.Fatalf("ByAction = %+v, want one BUY group", us.ByAction)
	}
	cell1 := us.ByAction[0].Cells[0]
	if cell1.Horizon != 1 || cell1.N != 1 || cell1.HitRatePct != 100 {
		t.Errorf("ByAction[BUY].Cells[0] = %+v, want horizon 1 / n 1 / hitRate 100 (AAPL outpaced SPY)", cell1)
	}
	if len(us.Best) != 1 || us.Best[0].Ticker != "AAPL" {
		t.Errorf("Best = %+v, want one AAPL entry", us.Best)
	}

	tw, err := buildRecPerformance(fdb, fh, market.TW)
	if err != nil {
		t.Fatalf("buildRecPerformance(TW) error = %v", err)
	}
	if tw.Counts.Total != 1 || tw.Counts.Scorable != 1 {
		t.Errorf("TW Counts = %+v, want Total 1 / Scorable 1", tw.Counts)
	}
	if len(tw.BySource) != 1 || tw.BySource[0].Key != "scan" {
		t.Fatalf("TW BySource = %+v, want one 'scan' group", tw.BySource)
	}
	twCell := tw.BySource[0].Cells[0]
	if twCell.N != 1 || twCell.HitRatePct != 0 {
		t.Errorf("TW BySource[scan].Cells[0] = %+v, want n 1 / hitRate 0 (2330 lost value, BUY should miss)", twCell)
	}
}

func TestBuildRecPerformance_NoScorableRecs(t *testing.T) {
	fdb := &fakeDB{recs: []db.Recommendation{
		{Date: "2026-01-05", Ticker: "AAPL", Action: "HOLD", Price: 100, Market: "us"},
	}}
	got, err := buildRecPerformance(fdb, &fakeHistory{}, market.US)
	if err != nil {
		t.Fatalf("buildRecPerformance() error = %v", err)
	}
	if got.Counts.Total != 1 || got.Counts.Hold != 1 || got.Counts.Scorable != 0 {
		t.Errorf("Counts = %+v, want Total 1 / Hold 1 / Scorable 0", got.Counts)
	}
	if len(got.BySource) != 0 || len(got.Best) != 0 {
		t.Errorf("got = %+v, want empty groups/extremes", got)
	}
}

func TestRecPerfStore_CachesWithinTTL(t *testing.T) {
	fdb := &fakeDB{recs: []db.Recommendation{
		{Date: "2026-01-05", Ticker: "AAPL", Action: "BUY", Price: 100, Market: "us"},
	}}
	calls := 0
	fh := &countingHistory{fakeHistory: fakeHistory{candles: map[string][]data.Candle{
		"AAPL": seqCandles(time.Now(), 10, 100, 1),
		"SPY":  seqCandles(time.Now(), 10, 500, 1),
	}}, calls: &calls}

	store := newRecPerfStore(fdb, fh)
	if _, err := store.Get(market.US); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	firstCalls := calls
	if _, err := store.Get(market.US); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if calls != firstCalls {
		t.Errorf("second Get() made %d more history calls, want 0 (should be cache hit)", calls-firstCalls)
	}
}

// countingHistory wraps fakeHistory to count GetHistory calls, for pinning
// recPerfStore's cache-hit behavior.
type countingHistory struct {
	fakeHistory
	calls *int
}

func (c *countingHistory) GetHistory(ticker, rangeParam string) ([]data.Candle, error) {
	*c.calls++
	return c.fakeHistory.GetHistory(ticker, rangeParam)
}
