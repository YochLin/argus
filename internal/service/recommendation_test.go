package service

import (
	"errors"
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/db"
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
