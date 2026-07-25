// Package receval scores Argus's historical recommendations table against
// actual subsequent price action — see docs/offline-rec-eval.md for the full
// design. Pure functions over plain structs only (same "no DB/network"
// discipline as internal/signals): callers (cmd/bot's "eval" subcommand)
// fetch db.Recommendation rows and data.Candle history and hand over plain
// values, so this package's tests need neither.
package receval

import (
	"time"

	"argus/internal/data"
)

// maxEntryGap bounds how far the first on-or-after-recDate candle is allowed
// to sit past recDate before that candle is rejected as a real entry point.
// A few calendar days covers an ordinary weekend/holiday gap; anything wider
// means the fetched -range window simply doesn't reach back to recDate (the
// recommendation is older than the fetched history), not that the market
// was closed a long time — see Score's "recommendation predates fetched
// history range" case.
const maxEntryGap = 7 * 24 * time.Hour

// Recommendation is one BUY/SELL call to be scored. A package-local
// mini-struct rather than db.Recommendation itself — same
// doesn't-import-internal/db convention internal/llm's Position/Earnings
// mini-structs use — so this package's tests don't need a database.
type Recommendation struct {
	Date   string // YYYY-MM-DD
	Ticker string
	Action string // "BUY" or "SELL" only; callers filter out HOLD/"" before scoring
	Price  float64
	Source string
	Market string
}

// WindowScore is one recommendation's outcome over one trading-day horizon.
// Matured is false when the horizon hasn't happened yet (the recommendation
// is too recent for the fetched history to reach that far) — every other
// field is meaningless in that case. HaveBench is false when no benchmark
// candle lines up with this window (missing benchmark data), in which case
// ExcessReturnPct falls back to the plain TickerReturnPct, mirroring
// bot.trackHit's haveSPY degrade.
type WindowScore struct {
	Horizon         int
	Matured         bool
	TickerReturnPct float64
	HaveBench       bool
	BenchReturnPct  float64
	ExcessReturnPct float64
	Hit             bool
}

// ScoredRec is one recommendation with its entry point resolved and every
// requested horizon evaluated, plus MFE/MAE across the widest horizon
// requested. Unscorable recs (no history, or the recommendation date falls
// outside the fetched range) carry a human-readable Reason and no Windows.
type ScoredRec struct {
	Rec        Recommendation
	Unscorable bool
	Reason     string
	EntryDate  string
	EntryPrice float64
	Windows    []WindowScore
	MFEPct     float64
	MAEPct     float64
}

// Score evaluates rec against candles (rec.Ticker's daily history, oldest
// first) and bench (the same-period benchmark history, e.g. SPY/0050) over
// each requested horizon (trading days). See docs/offline-rec-eval.md §5 for
// the full method this implements.
func Score(rec Recommendation, candles, bench []data.Candle, horizons []int) ScoredRec {
	sr := ScoredRec{Rec: rec}

	recDate, err := time.Parse("2006-01-02", rec.Date)
	if err != nil {
		sr.Unscorable = true
		sr.Reason = "invalid recommendation date"
		return sr
	}
	if len(candles) == 0 {
		sr.Unscorable = true
		sr.Reason = "no history data"
		return sr
	}

	i, ok, tooOld := entryIndex(candles, recDate)
	if tooOld {
		sr.Unscorable = true
		sr.Reason = "recommendation predates fetched history range"
		return sr
	}
	if !ok {
		sr.Unscorable = true
		sr.Reason = "no trading day on or after recommendation date"
		return sr
	}

	entryPrice := rec.Price
	if entryPrice == 0 {
		entryPrice = candles[i].Close
	}
	if entryPrice <= 0 {
		sr.Unscorable = true
		sr.Reason = "no usable entry price"
		return sr
	}
	sr.EntryDate = candles[i].Date.Format("2006-01-02")
	sr.EntryPrice = entryPrice

	benchIdx, haveBenchAnchor := 0, false
	if len(bench) > 0 {
		if bi, bok, _ := entryIndex(bench, recDate); bok {
			benchIdx, haveBenchAnchor = bi, true
		}
	}

	maxHorizon := 0
	for _, h := range horizons {
		if h > maxHorizon {
			maxHorizon = h
		}
	}

	sr.Windows = make([]WindowScore, 0, len(horizons))
	for _, h := range horizons {
		ws := WindowScore{Horizon: h}
		exitIdx := i + h
		if exitIdx < len(candles) {
			ws.Matured = true
			ws.TickerReturnPct = pctChange(entryPrice, candles[exitIdx].Close)
			if haveBenchAnchor {
				bExitIdx := benchIdx + h
				if bExitIdx < len(bench) && bench[benchIdx].Close > 0 {
					ws.HaveBench = true
					ws.BenchReturnPct = pctChange(bench[benchIdx].Close, bench[bExitIdx].Close)
					ws.ExcessReturnPct = ws.TickerReturnPct - ws.BenchReturnPct
				}
			}
			if !ws.HaveBench {
				ws.ExcessReturnPct = ws.TickerReturnPct
			}
			ws.Hit = hit(rec.Action, ws.ExcessReturnPct)
		}
		sr.Windows = append(sr.Windows, ws)
	}

	sr.MFEPct, sr.MAEPct = mfeMae(candles, i, maxHorizon, entryPrice)
	return sr
}

// entryIndex finds the first candle on or after recDate. tooOld is true when
// that candle exists but sits implausibly far past recDate (see
// maxEntryGap) — meaning the fetched series simply doesn't reach back that
// far, not that the market was closed for a week. ok is false when every
// candle in the series predates recDate (the recommendation is newer than
// the fetched data).
func entryIndex(candles []data.Candle, recDate time.Time) (idx int, ok, tooOld bool) {
	for i, c := range candles {
		if !c.Date.Before(recDate) {
			if i == 0 && c.Date.Sub(recDate) > maxEntryGap {
				return 0, false, true
			}
			return i, true, false
		}
	}
	return 0, false, false
}

func hit(action string, excessReturnPct float64) bool {
	switch action {
	case "BUY":
		return excessReturnPct > 0
	case "SELL":
		return excessReturnPct < 0
	default:
		return false
	}
}

func pctChange(from, to float64) float64 {
	if from == 0 {
		return 0
	}
	return (to - from) / from * 100
}

// mfeMae scans from entryIdx to entryIdx+maxHorizon (or the series end,
// whichever comes first) for the largest favorable/adverse excursion
// relative to entryPrice, using each candle's High/Low rather than just its
// Close — "how far this trade ran in its favor, and against it, at any
// point along the way," not just at the fixed horizon checkpoints.
func mfeMae(candles []data.Candle, entryIdx, maxHorizon int, entryPrice float64) (mfe, mae float64) {
	end := entryIdx + maxHorizon
	if end >= len(candles) {
		end = len(candles) - 1
	}
	for idx := entryIdx; idx <= end; idx++ {
		c := candles[idx]
		if c.High > 0 {
			if up := pctChange(entryPrice, c.High); up > mfe {
				mfe = up
			}
		}
		if c.Low > 0 {
			if down := pctChange(entryPrice, c.Low); down < mae {
				mae = down
			}
		}
	}
	return mfe, mae
}
