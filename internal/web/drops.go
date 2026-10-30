package web

import (
	"time"

	"argus/internal/data"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/service"
	"argus/internal/signals"
)

// dropResponse is one signals.DetectDrops hit, dates instead of candle
// indices. Everything but Summary is computed per request from the candles, so
// any ticker has its whole chart window. Summary is the Telegram price event's
// LLM writeup for a day inside the drop, "" when there is none — the usual case,
// because a past day's news can't be fetched (data.Provider.GetNews takes no
// date) and price_events runs on different thresholds.
type dropResponse struct {
	Kind        string   `json:"kind"` // "streak"/"gap"
	Start       string   `json:"start"`
	End         string   `json:"end"`
	Pct         float64  `json:"pct"`
	GapPct      *float64 `json:"gapPct"`
	PreClose    float64  `json:"preClose"`
	Low         float64  `json:"low"`
	RecoverBars *int     `json:"recoverBars"`
	Fwd10Pct    *float64 `json:"fwd10Pct"`
	IndexPct    *float64 `json:"indexPct"` // benchmark over the same span; null if it has no bars for it
	Summary     string   `json:"summary"`
}

// buildDrops assembles /api/chart's drops and the benchmark their IndexPct is
// measured on. A bar still in session is left out: the events say "after the
// close", and a part-day close could make one that's gone by the end of the day.
func buildDrops(database dbReader, history data.HistoryProvider, ticker string, candles []data.Candle) ([]dropResponse, string) {
	benchmark := service.BenchmarkFor(market.Of(ticker))
	if n := len(candles); n > 0 && service.BarForming(candles, n-1, ticker, time.Now()) {
		candles = candles[:n-1]
	}
	drops := signals.DetectDrops(candles)
	out := make([]dropResponse, 0, len(drops))
	if len(drops) == 0 {
		return out, benchmark
	}

	indexCloses := benchmarkCloses(history, benchmark, ticker, candles)
	summaries := priceEventSummaries(database, ticker)
	for _, d := range drops {
		r := dropResponse{
			Kind:        d.Kind,
			Start:       candles[d.Start].Date.Format("2006-01-02"),
			End:         candles[d.End].Date.Format("2006-01-02"),
			Pct:         d.Pct,
			GapPct:      d.GapPct,
			PreClose:    d.PreClose,
			Low:         d.Low,
			RecoverBars: d.RecoverBars,
			Fwd10Pct:    d.Fwd10Pct,
		}
		from, ok1 := indexCloses[candles[d.Start-1].Date.Format("2006-01-02")]
		to, ok2 := indexCloses[r.End]
		if ok1 && ok2 && from > 0 {
			pct := (to/from - 1) * 100
			r.IndexPct = &pct
		}
		// Latest day first, so a streak reads its last writeup.
		for i := d.End; i >= d.Start && r.Summary == ""; i-- {
			r.Summary = summaries[candles[i].Date.Format("2006-01-02")]
		}
		out = append(out, r)
	}
	return out, benchmark
}

// benchmarkCloses maps date to the benchmark's close, nil if it can't be had:
// the drops still stand without their market comparison.
func benchmarkCloses(history data.HistoryProvider, benchmark, ticker string, candles []data.Candle) map[string]float64 {
	bars := candles
	if ticker != benchmark {
		var err error
		if bars, err = history.GetHistory(benchmark, chartHistoryRange); err != nil {
			logger.Errorf("web: drops for %s: benchmark %s unavailable: %v", ticker, benchmark, err)
			return nil
		}
	}
	closes := make(map[string]float64, len(bars))
	for _, c := range bars {
		closes[c.Date.Format("2006-01-02")] = c.Close
	}
	return closes
}

func priceEventSummaries(database dbReader, ticker string) map[string]string {
	if database == nil {
		return nil
	}
	events, err := database.GetPriceEventsForTicker(ticker, eventsListLimit)
	if err != nil {
		logger.Errorf("web: drops for %s: price events: %v", ticker, err)
		return nil
	}
	out := make(map[string]string, len(events))
	for _, ev := range events {
		out[ev.Date] = ev.Summary
	}
	return out
}
