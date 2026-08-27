package service

import (
	"math"
	"sort"

	"argus/internal/data"
	"argus/internal/signals"
)

// RankAndTruncateCandidates keeps only the top n of tickers by a rule score
// blending relative strength vs. benchTicker, 20-day average dollar volume,
// and proximity to the nearest support level below the close — Phase 24 Stage 1's
// first Recommendation Pipeline Service slice, moved verbatim from
// bot.rankAndTruncateCandidates except benchTicker replaces the market.
// MarketID param: resolving "which ticker is the benchmark" (SPY vs. 0050)
// is a bot-layer policy choice (see bot.benchmarkFor, still used by several
// other callers), not something this service needs to know.
//
// # What has actually been measured (2026-08-26)
//
// This layer went unbacktested until then. cmd/strategyscan's -dump-trades
// now carries each replayed trade's factor values, so the ranking can be
// replayed offline over the random-entry control: 246 US cross-sections,
// ~465 names each, 2016-11..2026-08, split at 2021-11 (the earlier slice is
// the holdout — the screens were designed against the later one). Excess is
// the kept names' mean exit return minus the whole pool's, with a
// date-clustered bootstrap.
//
// Three results, and none of them says to add a factor:
//
//   - The support-side fix is return-NEUTRAL. Paired on identical pools,
//     old (both-sides distance) -> new (support only) is +0.00% in the
//     holdout (0.0 sigma), -0.07% from 2021-11 on (0.7 sigma), -0.03% full
//     sample (0.6 sigma). It was made because the factor did not measure
//     what its name said, not because it pays; don't let a future study
//     "restore" the old behavior on a stray positive number either.
//   - Nothing replicates across the two slices. Ranking on 63-day relative
//     strength is worth +0.98% at 3.9 sigma in the holdout (top 10% kept)
//     and +0.15% at 0.4 sigma after 2021-11. Swapping in 252-day RS or
//     12-1 momentum gives the exact mirror image: 0.9/1.1 sigma in the
//     holdout, 3.1/2.8 sigma after. Two factor sets that each work in only
//     one half are one factor set that works in neither, so 12-1 momentum
//     was NOT adopted despite the literature behind it.
//   - Dollar volume as a liquidity gate (drop the thinnest 20-50% of the
//     pool, then rank on RS alone) improves excess-vs-pool but not the
//     portfolio: holdout, top 20% kept, the gated book returns +2.78%
//     against +2.86% ungated. The excess only widened because gating
//     lowered the pool it is measured against. Left as a score.
//
// The one pattern that does hold everywhere is selectivity: excess grows
// monotonically as the kept fraction shrinks (top 50% beats nothing at all
// in any slice; top 10% is where every scheme's edge appears). n is
// therefore the knob most likely to matter here, not the factor list — but
// the study's pools are ~465 names deep and the live one is dozens, so that
// has to be measured against a real candidate pool before it is touched.
// See cmd/strategyscan/factors.go for the dumped columns.
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
		row.distToLevel, row.hasDistToLevel = nearestSupportDist(candles)
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

// nearestSupportDist returns the distance from the latest close down to the
// nearest support level, as a fraction of the close, and whether one was
// found at all.
//
// The support/resistance classification has to happen here rather than in
// signals.PriceLevels: a PriceLevel deliberately carries no side, because a
// level's role flips when price crosses it (old resistance becomes
// support), so that package's doc comment tells callers to compare against
// the latest close themselves. RankAndTruncateCandidates did not, until
// 2026-08-26: it scored math.Abs over every level, which gave "sitting on
// support" and "capped by overhead supply" an identical score. For a
// long-only screen those are opposite readings, so the two halves cancelled
// and the factor contributed noise at best.
//
// ok=false when there is no level below the close — the caller scores that
// neutral rather than penalizing it, since "no level found" is missing
// information, not a bad level.
func nearestSupportDist(candles []data.Candle) (dist float64, ok bool) {
	if len(candles) == 0 {
		return 0, false
	}
	lastClose := candles[len(candles)-1].Close
	if lastClose <= 0 {
		return 0, false
	}
	best := math.MaxFloat64
	for _, lvl := range signals.PriceLevels(candles) {
		if lvl.Price >= lastClose {
			continue
		}
		if d := (lastClose - lvl.Price) / lastClose; d < best {
			best = d
		}
	}
	if best == math.MaxFloat64 {
		return 0, false
	}
	return best, true
}
