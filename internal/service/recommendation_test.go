package service

import (
	"errors"
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/llm"
	"argus/internal/market"
)

type fakeRecommendationStore struct {
	recs      []db.Recommendation
	snapshots map[string]float64
	fromDate  string
}

func (f *fakeRecommendationStore) GetRecommendationsSince(fromDate string) ([]db.Recommendation, error) {
	f.fromDate = fromDate
	return f.recs, nil
}

func (f *fakeRecommendationStore) GetSnapshotClose(ticker, date string) (float64, bool, error) {
	value, ok := f.snapshots[ticker+":"+date]
	return value, ok, nil
}

type fakeRecommendationQuotes struct {
	quotes map[string]*data.Quote
	errors map[string]error
	calls  []string
}

func (f *fakeRecommendationQuotes) GetQuote(ticker string) (*data.Quote, error) {
	f.calls = append(f.calls, ticker)
	if err := f.errors[ticker]; err != nil {
		return nil, err
	}
	return f.quotes[ticker], nil
}

func TestRecommendationTrackingServiceUsesMarketBenchmarks(t *testing.T) {
	store := &fakeRecommendationStore{
		recs: []db.Recommendation{
			{Date: "2026-08-10", Ticker: "AAPL", Action: "BUY", Price: 100, Source: "watchlist"},
			{Date: "2026-08-10", Ticker: "2330", Action: "BUY", Price: 900, Source: "scan"},
		},
		snapshots: map[string]float64{
			"SPY:2026-08-10":  500,
			"0050:2026-08-10": 100,
		},
	}
	quotes := &fakeRecommendationQuotes{quotes: map[string]*data.Quote{
		"AAPL": {Ticker: "AAPL", Price: 110},
		"2330": {Ticker: "2330", Price: 950},
		"SPY":  {Ticker: "SPY", Price: 505},
		"0050": {Ticker: "0050", Price: 101},
	}}

	svc := NewRecommendationTrackingService(store, quotes)
	svc.now = func() time.Time { return time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC) }
	report, err := svc.Track(7)
	if err != nil {
		t.Fatalf("Track() error = %v", err)
	}
	if store.fromDate != "2026-08-08" {
		t.Errorf("GetRecommendationsSince fromDate = %q, want 2026-08-08", store.fromDate)
	}
	if len(report.Details) != 2 || len(report.Rows) != 2 {
		t.Fatalf("report has %d details and %d rows, want 2 each", len(report.Details), len(report.Rows))
	}

	if got := report.Details[0].BenchmarkTicker; got != "SPY" {
		t.Errorf("US benchmark = %q, want SPY", got)
	}
	if got := report.Details[1].BenchmarkTicker; got != "0050" {
		t.Errorf("TW benchmark = %q, want 0050", got)
	}
	if !report.Details[0].HasBenchmark || !report.Details[1].HasBenchmark {
		t.Errorf("benchmark availability = %+v, want both available", report.Details)
	}
	if report.Summary.Overall.Hits != 2 || report.Summary.Overall.Evaluated != 2 {
		t.Errorf("summary = %+v, want 2/2 hits", report.Summary.Overall)
	}
	if len(quotes.calls) != 4 {
		t.Errorf("GetQuote calls = %v, want one call per ticker and benchmark", quotes.calls)
	}
}

func TestRecommendationTrackingServiceFallsBackToDirectionAndEntrySnapshot(t *testing.T) {
	store := &fakeRecommendationStore{
		recs: []db.Recommendation{
			{Date: "2026-08-10", Ticker: "AAPL", Action: "BUY", Price: 0},
			{Date: "2026-08-10", Ticker: "MSFT", Action: "SELL", Price: 100},
		},
		snapshots: map[string]float64{"AAPL:2026-08-10": 100},
	}
	quotes := &fakeRecommendationQuotes{
		quotes: map[string]*data.Quote{
			"AAPL": {Ticker: "AAPL", Price: 101},
			"MSFT": {Ticker: "MSFT", Price: 99},
		},
		errors: map[string]error{"SPY": errors.New("benchmark unavailable")},
	}

	svc := NewRecommendationTrackingService(store, quotes)
	svc.now = func() time.Time { return time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC) }
	report, err := svc.Track(7)
	if err != nil {
		t.Fatalf("Track() error = %v", err)
	}
	if len(report.Rows) != 2 || report.Summary.Overall.Hits != 2 {
		t.Fatalf("summary = %+v, want both direction-only rows to hit", report.Summary.Overall)
	}
	if report.Details[0].BasePrice != 100 || !report.Details[0].HasBasePrice {
		t.Errorf("snapshot fallback detail = %+v, want base price 100", report.Details[0])
	}
	if report.Details[0].HasBenchmark || report.Details[1].HasBenchmark {
		t.Errorf("benchmark should be unavailable: %+v", report.Details)
	}
}

func TestRecommendationTrackingServicePreservesMissingPricesAndQuotes(t *testing.T) {
	store := &fakeRecommendationStore{
		recs: []db.Recommendation{
			{Date: "2026-08-10", Ticker: "AAPL", Action: "HOLD"},
			{Date: "2026-08-10", Ticker: "MSFT", Action: "BUY", Price: 100},
		},
		snapshots: map[string]float64{},
	}
	quotes := &fakeRecommendationQuotes{errors: map[string]error{"MSFT": errors.New("quote unavailable")}}
	svc := NewRecommendationTrackingService(store, quotes)
	svc.now = func() time.Time { return time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC) }

	report, err := svc.Track(7)
	if err != nil {
		t.Fatalf("Track() error = %v", err)
	}
	if !report.HasRecommendations || len(report.Details) != 2 || len(report.Rows) != 0 {
		t.Fatalf("report = %+v, want details without scored rows", report)
	}
	if report.Details[0].HasBasePrice || report.Details[0].HasQuote {
		t.Errorf("missing-price detail = %+v, want no price or quote", report.Details[0])
	}
	if report.Details[1].HasQuote || report.Details[1].QuoteErr == nil {
		t.Errorf("missing-quote detail = %+v, want quote error", report.Details[1])
	}
	if report.Summary.Overall.Evaluated != 0 {
		t.Errorf("summary = %+v, want no evaluated rows", report.Summary.Overall)
	}
	if report.Details[0].Market != market.US {
		t.Errorf("AAPL market = %q, want US", report.Details[0].Market)
	}
}

func TestTrackHit(t *testing.T) {
	if !TrackHit("BUY", 5, 3, true) || TrackHit("BUY", 3, 5, true) {
		t.Error("BUY benchmark comparison did not match expected hit rule")
	}
	if !TrackHit("SELL", -5, -2, true) || TrackHit("SELL", -2, -5, true) {
		t.Error("SELL benchmark comparison did not match expected hit rule")
	}
	if !TrackHit("BUY", 1, 0, false) || !TrackHit("SELL", -1, 0, false) {
		t.Error("direction fallback did not match expected hit rule")
	}
}

func TestMergeCandidates(t *testing.T) {
	movers := []string{"AAPL", "MSFT"}
	scanHits := map[string]string{
		"MSFT": "RSI oversold (28.0)", // also a mover: must not duplicate
		"NVDA": "MACD golden cross",
	}
	watchlist := []string{"TSLA"} // excluded even if it somehow appears

	got := MergeCandidates(movers, scanHits, watchlist)

	want := map[string]bool{"AAPL": true, "MSFT": true, "NVDA": true}
	if len(got) != len(want) {
		t.Fatalf("MergeCandidates() = %v, want exactly %v", got, want)
	}
	seen := make(map[string]bool)
	for _, ticker := range got {
		if seen[ticker] {
			t.Errorf("MergeCandidates() contains duplicate %s", ticker)
		}
		seen[ticker] = true
		if !want[ticker] {
			t.Errorf("MergeCandidates() contains unexpected %s", ticker)
		}
	}
}

func TestRecommendationSources(t *testing.T) {
	watchlist := []string{"AAPL", "MSFT"}
	// MSFT also appears as a candidate — shouldn't happen in practice since
	// MergeCandidates already excludes watchlist tickers, but RecommendationSources
	// guards it anyway: watchlist attribution must win regardless.
	candidates := []string{"MSFT", "NVDA", "TSLA", "SNOW"}
	scanHits := map[string]string{
		"NVDA": "RSI oversold (28.0)",
	}
	explore := map[string]string{
		"SNOW": "LLM nomination: named in a cloud-spend story",
	}

	got := RecommendationSources(watchlist, candidates, scanHits, explore)

	want := map[string]string{
		"AAPL": "watchlist",
		"MSFT": "watchlist",
		"NVDA": "scan",
		"TSLA": "movers",
		"SNOW": "explore",
	}
	for ticker, wantSource := range want {
		if got[ticker] != wantSource {
			t.Errorf("RecommendationSources()[%s] = %q, want %q", ticker, got[ticker], wantSource)
		}
	}
}

func TestRecommendationSourcesScanBeatsExplore(t *testing.T) {
	// Shouldn't happen in practice — exploreCandidates' dedup step already
	// excludes anything already a candidate — but scan must win over explore
	// defensively, same reasoning as watchlist winning over both.
	candidates := []string{"NVDA"}
	scanHits := map[string]string{"NVDA": "MACD golden cross"}
	explore := map[string]string{"NVDA": "LLM nomination: also mentioned in the news"}

	got := RecommendationSources(nil, candidates, scanHits, explore)

	if got["NVDA"] != "scan" {
		t.Errorf("RecommendationSources()[NVDA] = %q, want %q", got["NVDA"], "scan")
	}
}

func candle(date string) data.Candle {
	d, err := time.Parse("2006-01-02", date)
	if err != nil {
		panic(err)
	}
	return data.Candle{Date: d}
}

func TestCountCandleGaps(t *testing.T) {
	cases := []struct {
		name   string
		stocks []llm.StockData
		market market.MarketID
		want   int
	}{
		{
			name:   "consecutive trading days, no gap",
			stocks: []llm.StockData{{Candles: []data.Candle{candle("2026-08-17"), candle("2026-08-18")}}}, // Mon -> Tue
			market: market.US,
			want:   0,
		},
		{
			name:   "weekend between Friday and Monday is not a gap",
			stocks: []llm.StockData{{Candles: []data.Candle{candle("2026-08-14"), candle("2026-08-17")}}}, // Fri -> Mon
			market: market.US,
			want:   0,
		},
		{
			name:   "missing bar mid-week is a gap",
			stocks: []llm.StockData{{Candles: []data.Candle{candle("2026-08-17"), candle("2026-08-19")}}}, // Mon -> Wed, Tue missing
			market: market.US,
			want:   1,
		},
		{
			name: "US holiday (Labor Day) does not count as a gap",
			// 2026-09-04 Fri -> 2026-09-08 Tue, skipping weekend + Labor Day (Mon 2026-09-07).
			stocks: []llm.StockData{{Candles: []data.Candle{candle("2026-09-04"), candle("2026-09-08")}}},
			market: market.US,
			want:   0,
		},
		{
			name:   "TW has no holiday calendar, so it falls back to weekday-only",
			stocks: []llm.StockData{{Candles: []data.Candle{candle("2026-08-17"), candle("2026-08-19")}}},
			market: market.TW,
			want:   1,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := CountCandleGaps(c.stocks, c.market); got != c.want {
				t.Errorf("CountCandleGaps() = %d, want %d", got, c.want)
			}
		})
	}
}
