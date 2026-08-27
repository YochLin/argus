package main

import (
	"math"

	"argus/internal/data"
	"argus/internal/signals"
)

// rankWindowBars is how much history the factors below see. It is 252 (~1
// trading year) because that is exactly what the live ranker sees:
// service.RankAndTruncateCandidates calls GetHistory(ticker, "1y"). Feeding
// these the tool's full 10-year slice would measure a ranker that doesn't
// exist.
const rankWindowBars = 252

// mom12_1Skip is the 21-bar (~1 month) tail that 12-1 momentum deliberately
// excludes. The one-month horizon is short-term reversal, not momentum, and
// including it is the classic way to cancel the signal out.
const mom12_1Skip = 21

// rankFactors are the per-(ticker, day) inputs to the candidate-ranking
// layer, dumped alongside each replayed trade so the ranking can be studied
// offline (group by date, sort by a candidate score, keep the top k, compare
// mean ExitRet) without re-running this tool once per weighting.
//
// A factor that cannot be computed on the available history is NaN, written
// as an empty CSV cell so pandas reads it as a genuine NaN rather than a 0
// that would silently rank as a real value.
//
// AbsLevelDist is here to measure a change already shipped, not a proposal:
// it is the pre-2026-08-26 both-sides distance the ranker used before
// service.nearestSupportDist started filtering to support. Dumping the old
// and new factor side by side is what lets that fix be checked against
// returns rather than argued from first principles.
type rankFactors struct {
	RS63         float64 // relative strength vs. the benchmark over 63 bars — what the live ranker scores today
	RS252        float64 // same over 252 bars
	Mom12_1      float64 // 12-1 momentum vs. the benchmark: return from t-252 to t-21
	DollarVol20  float64 // 20-bar mean of close x volume
	SupportDist  float64 // fraction from the close down to the nearest level below it
	AbsLevelDist float64 // fraction to the nearest level on EITHER side (the retired behavior)
}

// computeRankFactors evaluates the ranking factors on sub's last bar. sub and
// benchSub must both end on the same calendar date (the caller aligns them
// through benchDateIdx) — RelativeStrength compares their tails, so a
// misaligned benchmark silently produces a wrong number rather than an error.
func computeRankFactors(sub, benchSub []data.Candle) rankFactors {
	f := rankFactors{
		RS63: math.NaN(), RS252: math.NaN(), Mom12_1: math.NaN(),
		DollarVol20: math.NaN(), SupportDist: math.NaN(), AbsLevelDist: math.NaN(),
	}
	if len(sub) == 0 {
		return f
	}
	closes := data.Closes(sub)
	benchCloses := data.Closes(benchSub)

	if rs, ok := signals.RelativeStrength(closes, benchCloses, 63); ok {
		f.RS63 = rs
	}
	if rs, ok := signals.RelativeStrength(closes, benchCloses, rankWindowBars); ok {
		f.RS252 = rs
	}
	f.Mom12_1 = mom12_1(closes, benchCloses)

	win := sub
	if len(win) > rankWindowBars {
		win = win[len(win)-rankWindowBars:]
	}
	f.DollarVol20 = avgDollarVolume(win, 20)

	lastClose := closes[len(closes)-1]
	if lastClose > 0 {
		bestBelow, bestAny := math.MaxFloat64, math.MaxFloat64
		for _, lvl := range signals.PriceLevels(win) {
			d := math.Abs(lastClose-lvl.Price) / lastClose
			if d < bestAny {
				bestAny = d
			}
			if lvl.Price < lastClose && d < bestBelow {
				bestBelow = d
			}
		}
		if bestBelow != math.MaxFloat64 {
			f.SupportDist = bestBelow
		}
		if bestAny != math.MaxFloat64 {
			f.AbsLevelDist = bestAny
		}
	}
	return f
}

// mom12_1 returns the stock's return from t-252 to t-21 minus the
// benchmark's over the same span, in percent, or NaN if either series is too
// short. Both series must end on the same date.
func mom12_1(closes, benchCloses []float64) float64 {
	need := rankWindowBars + 1
	if len(closes) < need || len(benchCloses) < need {
		return math.NaN()
	}
	seg := func(s []float64) float64 {
		start := s[len(s)-need]
		end := s[len(s)-1-mom12_1Skip]
		if start <= 0 {
			return math.NaN()
		}
		return (end - start) / start * 100
	}
	return seg(closes) - seg(benchCloses)
}

// avgDollarVolume is the mean of close x volume over the trailing n bars —
// service.RankAndTruncateCandidates' liquidity factor, reimplemented here
// rather than exported from it because that function computes it inline
// while walking its own rows. The two are one arithmetic line; a shared
// helper across a service package and an offline tool would cost more than
// it saves.
func avgDollarVolume(candles []data.Candle, n int) float64 {
	if len(candles) == 0 || n <= 0 {
		return math.NaN()
	}
	if n > len(candles) {
		n = len(candles)
	}
	var sum float64
	for _, c := range candles[len(candles)-n:] {
		sum += c.Close * float64(c.Volume)
	}
	return sum / float64(n)
}
