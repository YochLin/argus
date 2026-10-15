package bot

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/i18n"
)

// fakeCompanyNames is a minimal data.CompanyNameProvider stub for
// TestTickerLabel/TestCompanyName — a static map avoids pulling in FinMind's
// HTTP-mocking machinery for what's just Bot's TW-gating logic.
type fakeCompanyNames struct {
	names map[string]string
}

// fakeEarnings is a minimal data.EarningsProvider stub for TestLoadEarnings.
type fakeEarnings struct {
	events map[string]data.EarningsEvent
	err    error
}

func (f fakeCompanyNames) GetCompanyName(ticker string) (string, error) {
	if name, ok := f.names[ticker]; ok {
		return name, nil
	}
	return "", fmt.Errorf("no name for %s", ticker)
}

func (f fakeEarnings) GetUpcomingEarnings(tickers []string, days int) (map[string]data.EarningsEvent, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[string]data.EarningsEvent)
	for _, t := range tickers {
		if e, ok := f.events[t]; ok {
			out[t] = e
		}
	}
	return out, nil
}

func (f fakeEarnings) GetEarningsInRange([]string, time.Time, time.Time) ([]data.EarningsEvent, error) {
	if f.err != nil {
		return nil, f.err
	}
	events := make([]data.EarningsEvent, 0, len(f.events))
	for _, e := range f.events {
		events = append(events, e)
	}
	return events, nil
}

func TestLoadEarningsMergesUSAndTWSources(t *testing.T) {
	b := &Bot{
		earnings: fakeEarnings{events: map[string]data.EarningsEvent{
			"AAPL": {Ticker: "AAPL", Date: "2026-08-01"},
		}},
		now: func() time.Time { return time.Date(2026, time.August, 1, 12, 0, 0, 0, cst) },
	}

	got := b.loadEarnings([]string{"AAPL", "2330"})

	aapl, ok := got["AAPL"]
	if !ok || aapl.Estimated {
		t.Errorf("loadEarnings()[AAPL] = %+v, want Finnhub's real (non-estimated) event", aapl)
	}
	tw, ok := got["2330"]
	if !ok || !tw.Estimated {
		t.Errorf("loadEarnings()[2330] = %+v, want the TW statutory-deadline proxy (Estimated=true)", tw)
	}
}

func TestLoadEarningsWorksWithoutFinnhubConfigured(t *testing.T) {
	// b.earnings is nil, same as no FINNHUB_API_KEY.
	b := &Bot{now: func() time.Time { return time.Date(2026, time.August, 1, 12, 0, 0, 0, cst) }}

	got := b.loadEarnings([]string{"2330"})

	if _, ok := got["2330"]; !ok {
		t.Error("loadEarnings() with nil b.earnings should still return the TW proxy entry")
	}
}

func TestComputeVsSPY(t *testing.T) {
	tests := []struct {
		name                                      string
		currentPrice, avgCost, spyPrice, spyEntry float64
		wantTickerPct, wantSPYPct                 float64
	}{
		{"beats the market", 200, 150, 550, 500, 33.333333333333336, 10},
		{"underperforms while still up", 165, 150, 550, 500, 10, 10},
		{"down position", 120, 150, 480, 500, -20, -4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeVsSPY(tt.currentPrice, tt.avgCost, tt.spyPrice, tt.spyEntry, "SPY")
			if !floatsClose(got.TickerPct, tt.wantTickerPct) || !floatsClose(got.SPYPct, tt.wantSPYPct) || got.Bench != "SPY" {
				t.Errorf("computeVsSPY(%v, %v, %v, %v) = %+v, want {%v %v}",
					tt.currentPrice, tt.avgCost, tt.spyPrice, tt.spyEntry, got, tt.wantTickerPct, tt.wantSPYPct)
			}
		})
	}
}

func floatsClose(a, b float64) bool {
	d := a - b
	return d > -1e-9 && d < 1e-9
}

func TestRenderEarningsPreviewFiltersByWindowAndSorts(t *testing.T) {
	in3Days := time.Now().In(cst).AddDate(0, 0, 3).Format("2006-01-02")
	in1Day := time.Now().In(cst).AddDate(0, 0, 1).Format("2006-01-02")
	in30Days := time.Now().In(cst).AddDate(0, 0, 30).Format("2006-01-02")
	yesterday := time.Now().In(cst).AddDate(0, 0, -1).Format("2006-01-02")

	earnings := map[string]data.EarningsEvent{
		"AAPL": {Ticker: "AAPL", Date: in3Days},
		"MSFT": {Ticker: "MSFT", Date: in1Day},
		"NVDA": {Ticker: "NVDA", Date: in30Days}, // outside the 7-day window
		"OLD":  {Ticker: "OLD", Date: yesterday}, // already past
	}

	got := renderEarningsPreview(i18n.EN, earnings, 7)

	if strings.Contains(got, "NVDA") || strings.Contains(got, "OLD") {
		t.Errorf("renderEarningsPreview() should exclude out-of-window tickers, got:\n%s", got)
	}
	msftIdx := strings.Index(got, "MSFT")
	aaplIdx := strings.Index(got, "AAPL")
	if msftIdx == -1 || aaplIdx == -1 || msftIdx > aaplIdx {
		t.Errorf("renderEarningsPreview() should sort soonest-first (MSFT before AAPL), got:\n%s", got)
	}
}

func TestRenderEarningsPreviewEmptyWhenNothingInWindow(t *testing.T) {
	if got := renderEarningsPreview(i18n.EN, nil, 7); got != "" {
		t.Errorf("renderEarningsPreview() with no earnings = %q, want \"\"", got)
	}
}

func TestRenderEarningsPreviewEstimatedWording(t *testing.T) {
	in3Days := time.Now().In(cst).AddDate(0, 0, 3).Format("2006-01-02")
	earnings := map[string]data.EarningsEvent{
		"2330": {Ticker: "2330", Date: in3Days, Estimated: true},
	}
	got := renderEarningsPreview(i18n.EN, earnings, 7)
	if !strings.Contains(got, "statutory filing deadline") {
		t.Errorf("renderEarningsPreview() missing estimated wording for a TW proxy entry, got:\n%s", got)
	}
}
