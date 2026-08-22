package service

import (
	"math"
	"sort"

	"argus/internal/data"
	"argus/internal/signals"
)

// RankAndTruncateCandidates keeps only the top n of tickers by a rule score
// blending relative strength vs. benchTicker, 20-day average dollar volume,
// and proximity to a nearby support/resistance level — Phase 24 Stage 1's
// first Recommendation Pipeline Service slice, moved verbatim from
// bot.rankAndTruncateCandidates except benchTicker replaces the market.
// MarketID param: resolving "which ticker is the benchmark" (SPY vs. 0050)
// is a bot-layer policy choice (see bot.benchmarkFor, still used by several
// other callers), not something this service needs to know.
func RankAndTruncateCandidates(history RiskHistoryReader, tickers []string, benchTicker string, n int) []string {
	if len(tickers) <= n {
		return tickers
	}

	var benchCloses []float64
	if candles, err := history.GetHistory(benchTicker, "1y"); err == nil {
		benchCloses = data.Closes(candles)
	}

	type rawScore struct {
		ticker         string
		relStrength    float64
		hasRelStrength bool
		avgDollarVol   float64
		distToLevel    float64
		hasDistToLevel bool
	}
	rows := make([]rawScore, 0, len(tickers))
	for _, t := range tickers {
		row := rawScore{ticker: t}
		candles, err := history.GetHistory(t, "1y")
		if err != nil || len(candles) < 60 {
			rows = append(rows, row) // unscoreable — neutral score, not dropped outright
			continue
		}
		closes := data.Closes(candles)
		if rs, ok := signals.RelativeStrength(closes, benchCloses, 63); ok {
			row.relStrength, row.hasRelStrength = rs, true
		}
		volumes := data.Volumes(candles)
		tail := 20
		if len(candles) < tail {
			tail = len(candles)
		}
		var dollarSum float64
		for i := len(candles) - tail; i < len(candles); i++ {
			dollarSum += closes[i] * float64(volumes[i])
		}
		row.avgDollarVol = dollarSum / float64(tail)
		lastClose := closes[len(closes)-1]
		if levels := signals.PriceLevels(candles); len(levels) > 0 && lastClose > 0 {
			best := math.MaxFloat64
			for _, lvl := range levels {
				if d := math.Abs(lastClose-lvl.Price) / lastClose; d < best {
					best = d
				}
			}
			row.distToLevel, row.hasDistToLevel = best, true
		}
		rows = append(rows, row)
	}

	// Each factor is min-max normalized to [0,1] across this batch — a
	// batch-relative rank is all "粗篩到 20" needs, and sidesteps picking an
	// absolute scale across three unrelated units (%, $, %).
	rs := make([]float64, len(rows))
	vol := make([]float64, len(rows))
	dist := make([]float64, len(rows))
	for i, r := range rows {
		rs[i], vol[i], dist[i] = r.relStrength, r.avgDollarVol, r.distToLevel
	}
	rsNorm, volNorm, distNorm := Normalize01(rs), Normalize01(vol), Normalize01(dist)

	type ranked struct {
		ticker string
		score  float64
	}
	out := make([]ranked, len(rows))
	for i, r := range rows {
		score := volNorm[i]
		if r.hasRelStrength {
			score += rsNorm[i]
		} else {
			score += 0.5
		}
		if r.hasDistToLevel {
			score += 1 - distNorm[i] // closer to a level scores higher
		} else {
			score += 0.5
		}
		out[i] = ranked{r.ticker, score}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].score > out[j].score })
	if len(out) > n {
		out = out[:n]
	}
	kept := make([]string, len(out))
	for i, r := range out {
		kept[i] = r.ticker
	}
	return kept
}

// Normalize01 min-max scales vals to [0,1]. An empty batch or a batch where
// every value is equal returns 0.5 for each — a neutral score that
// contributes nothing to the ranking rather than a divide-by-zero.
func Normalize01(vals []float64) []float64 {
	out := make([]float64, len(vals))
	if len(vals) == 0 {
		return out
	}
	lo, hi := vals[0], vals[0]
	for _, v := range vals {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	if hi == lo {
		for i := range out {
			out[i] = 0.5
		}
		return out
	}
	for i, v := range vals {
		out[i] = (v - lo) / (hi - lo)
	}
	return out
}
