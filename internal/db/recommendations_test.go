package db

import "testing"

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
