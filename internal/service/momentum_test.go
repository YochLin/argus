package service

import (
	"context"
	"math"
	"testing"
	"time"

	"argus/internal/data"
)

type momentumHistory map[string][]data.Candle

func (h momentumHistory) GetHistory(t, _ string) ([]data.Candle, error) { return h[t], nil }

type momentumQuotes map[string]*data.Quote

func (q momentumQuotes) GetQuote(t string) (*data.Quote, error) { return q[t], nil }

type momentumEarnings map[string]data.EarningsEvent

func (e momentumEarnings) GetUpcomingEarnings([]string, int) (map[string]data.EarningsEvent, error) {
	return e, nil
}

func momentumBars(closes, amps []float64) []data.Candle {
	c := make([]data.Candle, len(closes))
	for i, x := range closes {
		c[i] = data.Candle{Date: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i),
			Open: x, High: x + amps[i], Low: x - amps[i], Close: x, Volume: 1e6}
	}
	return c
}

func TestScanMomentumBreakout(t *testing.T) {
	var spy, flat, base, spyAmp, flatAmp, baseAmp []float64
	for i := range 130 {
		spy, spyAmp = append(spy, 100+float64(i)), append(spyAmp, 1)
		flat, flatAmp = append(flat, 50), append(flatAmp, 2)
	}
	for range 20 {
		base, baseAmp = append(base, 50), append(baseAmp, 2)
	}
	for i := range 99 { // the run-up
		base, baseAmp = append(base, 50+float64(i)*0.5), append(baseAmp, 2)
	}
	base, baseAmp = append(base, 100), append(baseAmp, 2) // the base's high: 102
	for j := range 10 {
		base, baseAmp = append(base, 98), append(baseAmp, 2.5-float64(j)*0.2)
	}
	hist := momentumHistory{"SPY": momentumBars(spy, spyAmp), "FLAT": momentumBars(flat, flatAmp), "BASE": momentumBars(base, baseAmp)}
	quotes := momentumQuotes{
		"SPY":  {Ticker: "SPY", Price: 240},
		"BASE": {Ticker: "BASE", Price: 102.5, Open: 99, High: 103, Low: 98},
		"FLAT": {Ticker: "FLAT", Price: 50, Open: 50, High: 51, Low: 49},
	}
	now := time.Date(2026, 10, 27, 15, 30, 0, 0, time.UTC)
	tickers := []string{"BASE", "FLAT"}

	w, err := ScanMomentumBreakout(context.Background(), hist, quotes, momentumEarnings{}, tickers, now, 0)
	if err != nil || !w.Regime.On || len(w.Picks) != 1 {
		t.Fatalf("got %+v err %v, want the regime on and BASE picked", w, err)
	}
	p := w.Picks[0]
	if p.Ticker != "BASE" || p.Pivot != 102 || !p.Broke || p.Entry != 102 || math.Abs(p.StopPct-100*4.0/102) > 1e-9 {
		t.Fatalf("pick %+v, want a break over 102 with the stop at today's low 98", p)
	}

	w, _ = ScanMomentumBreakout(context.Background(), hist, quotes, momentumEarnings{"BASE": {Ticker: "BASE"}}, tickers, now, 0)
	if len(w.Picks) != 0 {
		t.Fatalf("earnings within 10 days must drop the pick, got %+v", w.Picks)
	}

	quotes["SPY"] = &data.Quote{Ticker: "SPY", Price: 100} // today's price drags MA10/MA20 down
	w, _ = ScanMomentumBreakout(context.Background(), hist, quotes, nil, tickers, now, 0)
	if w.Regime.On || len(w.Picks) != 0 {
		t.Fatalf("regime %+v picks %d, want off with nothing fetched", w.Regime, len(w.Picks))
	}
}
