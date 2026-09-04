package signals

import (
	"fmt"

	"argus/internal/data"
	"argus/internal/i18n"
	"argus/internal/market"
)

// FamilyStrategyMTF is this screen's signal_states family key — see
// strategies.go's FamilyStrategy* block for the convention.
const FamilyStrategyMTF = "strategy_mtf"

// TypeMTFCross is 【日週共振穿越】's Signal.Type. Exported for the same
// reason as TypeBoxBottom/TypeTrustFollow: service.DecorateStrategyHits
// matches on it to attach the §4.4 notice below.
const TypeMTFCross = "strategy_mtf_cross"

// mtfMinBars is what CheckMTFCrossExact needs before it can answer: 30 daily
// bars for MA30 plus one prior bar for its slope, and enough calendar span
// for 8 completed weeks (7 for wMA7 plus one to compare against). 60 trading
// days covers both with margin.
const mtfMinBars = 60

// weeklyCloses returns the closing price of every COMPLETED week in candles,
// oldest first. The week the LAST bar sits in is deliberately excluded: its
// close is not known until that week ends, and including it is the lookahead
// bug this screen was originally measured with (a mid-week bar would carry
// up to 4 days of future data, which inflated the measured excess from ~0 to
// +3-4pp at 6-9 sigma before it was caught). Weeks are keyed by ISO year+week
// so a year boundary mid-week doesn't split one week into two.
func weeklyCloses(candles []data.Candle) []float64 {
	var out []float64
	cur := ""
	for i, c := range candles {
		y, w := c.Date.ISOWeek()
		key := fmt.Sprintf("%d-%02d", y, w)
		if i > 0 && key != cur {
			out = append(out, candles[i-1].Close)
		}
		cur = key
	}
	return out
}

// wma is the period-week moving average as of the weekly bar back bars ago
// (0 = the last completed week, 1 = the week before it). Returns 0 when
// there aren't enough completed weeks — the same "0 = not enough data"
// sentinel MA already uses.
func wma(weekly []float64, period, back int) float64 {
	end := len(weekly) - back
	if end < period {
		return 0
	}
	var sum float64
	for _, c := range weekly[end-period : end] {
		sum += c
	}
	return sum / float64(period)
}

// CheckMTFCrossExact evaluates candles' last bar against 【日週共振穿越】:
// daily MA5 and MA7 both sloping up, the close having JUST crossed above
// MA30 (below it on the prior bar), and the weekly MA5 and MA7 both sloping
// up as well.
//
// It takes no ScreenParams on purpose. Every other screen's thresholds are
// calibration knobs; these five periods ARE the rule as specified, and they
// are exactly what was measured below — a ScreenParams field here would
// invite re-tuning against the same slices the result came from, which is
// the failure mode §4.4 exists to prevent.
//
// The weekly slope compares WEEKLY BAR to WEEKLY BAR (last completed week vs
// the one before). The obvious-looking alternative — today's week-to-date
// weekly MA vs yesterday's — degenerates on 4 of 5 weekdays into "today
// closed higher than yesterday", since the completed weeks in both readings
// are identical and only the running close differs. That over-fires by ~3.2x
// (33,163 vs 10,282 triggers on the same universe) and is not what a weekly
// chart shows.
//
// # Result: measured (2026-09-04), TW only, NOT validated
//
// Measured as a WATCHLIST screen rather than an entry trigger — the question
// asked was "does it surface names that go on to move", so the statistic is
// P(20-day return > threshold) for trigger days vs every other trading day in
// the same market and split, date-clustered bootstrap (400 resamples of
// dates, split 2021-11-01, 613 tickers / 1,478,624 bars, S&P 500 + tw150):
//
//	market  threshold   early split        late split
//	TW      >  +5%      +4.13pp  1.9σ      -0.73pp  0.4σ
//	TW      >  +7%      +5.04pp  2.5σ      +0.60pp  0.3σ
//	TW      > +10%      +4.53pp  2.4σ      +0.01pp  0.0σ
//	TW      > +15%      +4.42pp  3.0σ      +0.76pp  0.6σ
//	US      >  +5%      +0.55pp  0.4σ      -3.44pp  2.7σ  (negative)
//	US      > +10%      -0.24pp  0.2σ      -2.23pp  2.7σ  (negative)
//	US      > +20%      -0.76pp  2.1σ      -1.24pp  4.4σ  (negative)
//
// TW fails §4.4 (the late split clears 1 SE at no threshold) but is never
// negative. US is the reverse and it is the stronger result: negative in
// BOTH splits past 1 SE at every threshold from 0% to +20% — this screen
// measurably LOWERS the odds of a big move on US names, so Detector.
// CheckMTFCross gates it to TW rather than annotating it.
//
// Also measured and rejected along the way, so they don't get retried: 20
// common add-on filters (MA30 slope, MA5>MA7 alignment, breakout size,
// same-day move, MA5 deviation, slope magnitude, cross-sectional crowding)
// were all negative in the late split without exception; delayed and
// confirmed entry (enter at T+5, or only if T+5 is higher) both did worse
// than entering on the trigger day; and a first trigger in 90 days performed
// no better than a repeat one, so "first fire = early warning" does not hold.
//
// Note on precedent: PostGapDrift's doc comment says a screen that misses
// §4.4 must NOT be wired into i18n/service.CheckStatefulSignals on the
// strength of its own numbers. This one IS wired, as an explicit
// user-directed exception — it ships as a TW-only watchlist tag carrying
// i18n.KeyStrategyUnvalidatedMTF, never as an entry trigger. Do not read it
// as the bar having moved.
func CheckMTFCrossExact(candles []data.Candle) bool {
	n := len(candles)
	if n < mtfMinBars {
		return false
	}
	closes := data.Closes(candles)

	prev := closes[:n-1]
	ma5, ma5Prev := MA(closes, 5), MA(prev, 5)
	ma7, ma7Prev := MA(closes, 7), MA(prev, 7)
	ma30, ma30Prev := MA(closes, 30), MA(prev, 30)
	if ma30Prev == 0 || ma5 <= ma5Prev || ma7 <= ma7Prev {
		return false
	}
	// 剛穿過：今天收在 MA30 之上，昨天還在（當時的）MA30 之下。
	if closes[n-1] <= ma30 || closes[n-2] > ma30Prev {
		return false
	}

	weekly := weeklyCloses(candles)
	w5, w5Prev := wma(weekly, 5, 0), wma(weekly, 5, 1)
	w7, w7Prev := wma(weekly, 7, 0), wma(weekly, 7, 1)
	if w5Prev == 0 || w7Prev == 0 {
		return false
	}
	return w5 > w5Prev && w7 > w7Prev
}

// MTFCross evaluates candles for 【日週共振穿越】 triggers within the last
// strategyLookbackDays. Returns the most recent hit (smallest DaysAgo) or
// nil if none triggered.
func MTFCross(candles []data.Candle) *StrategyHit {
	n := len(candles)
	for offset := 0; offset < strategyLookbackDays; offset++ {
		evalIdx := n - 1 - offset
		if evalIdx < mtfMinBars {
			break
		}
		if CheckMTFCrossExact(candles[:evalIdx+1]) {
			return &StrategyHit{Name: "mtf_cross", DaysAgo: offset}
		}
	}
	return nil
}

// CheckMTFCross adds alert-once-per-occurrence dedup on top of MTFCross, and
// gates the screen to TW — see CheckMTFCrossExact's measured US numbers for
// why that gate is a hard exclusion and not a warning.
func (d *Detector) CheckMTFCross(ticker string, candles []data.Candle, prevState string) (sig *Signal, newState string) {
	if market.Of(ticker) != market.TW {
		return nil, ""
	}
	hit := MTFCross(candles)
	if hit == nil {
		return nil, ""
	}
	newState = "hit"
	if prevState == "hit" {
		return nil, newState
	}

	daysAgoStr := i18n.T(d.lang, i18n.KeyDaysAgoToday)
	if hit.DaysAgo > 0 {
		daysAgoStr = i18n.T(d.lang, i18n.KeyDaysAgoN, hit.DaysAgo)
	}
	return &Signal{
		Ticker:  ticker,
		Type:    TypeMTFCross,
		Message: i18n.T(d.lang, i18n.KeyStrategyMTFCross, ticker, daysAgoStr),
	}, newState
}
