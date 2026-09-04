package signals

import (
	"fmt"
	"math"

	"argus/internal/data"
	"argus/internal/i18n"
)

// FamilyStrategyMTF is this screen's signal_states family key — see
// strategies.go's FamilyStrategy* block for the convention.
const FamilyStrategyMTF = "strategy_mtf"

// TypeMTFCross is 【日週共振穿越】's Signal.Type. Exported for the same
// reason as TypeBoxBottom/TypeTrustFollow: service.DecorateStrategyHits
// matches on it to attach the notice below.
const TypeMTFCross = "strategy_mtf_cross"

// The screen's five constants. They are deliberately NOT ScreenParams
// fields: every other screen's thresholds are per-market calibration knobs,
// these are the rule as measured, and a knob here would invite re-tuning
// against the same slices the result came from.
const (
	mtfDailyAngle    = 20.0 // min(daily MA5, MA7) rise, in degrees
	mtfWeeklyAngle   = 30.0 // min(weekly MA5, MA7) rise, in degrees
	mtfRSIMax        = 80.0 // overheat cap, same RSI the rest of the bot uses
	mtfDailySDBars   = 60   // trailing daily bars defining one "unit" of daily move
	mtfWeeklySDWeeks = 26   // trailing weeks defining one "unit" of weekly move
)

// mtfMinBars is what CheckMTFCrossExact needs before it can answer. The
// binding constraint is the weekly volatility unit: mtfWeeklySDWeeks+1
// completed weekly closes, i.e. ~27 calendar weeks, plus the running week.
// 150 trading days covers that with margin and comfortably clears the daily
// side (mtfDailySDBars+1 closes) and RSI's 15.
const mtfMinBars = 150

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

// stepSD is the standard deviation of the last n bar-to-bar changes in xs,
// in PRICE units (not percent). 0 when there isn't enough history or the
// series never moves, which every caller treats as "cannot answer".
func stepSD(xs []float64, n int) float64 {
	if len(xs) < n+1 {
		return 0
	}
	d := xs[len(xs)-n:]
	prev := xs[len(xs)-n-1:]
	var mean float64
	for i := range d {
		mean += d[i] - prev[i]
	}
	mean /= float64(n)
	var s float64
	for i := range d {
		x := d[i] - prev[i] - mean
		s += x * x
	}
	sd := math.Sqrt(s / float64(n-1))
	if sd <= 0 {
		return 0
	}
	return sd
}

// maAngle converts one bar of a moving average's rise into the "angle" a
// chart reader sees, in degrees.
//
// An angle on a chart is not a property of the data — it depends on the
// y-axis scale, the window's aspect ratio and the stock's price level. What
// the eye reads as "steep" is the MA's rise measured against how far this
// stock normally moves in one bar, because charting software autoscales the
// y-axis to the recent range: a volatile name is squashed and a quiet one
// stretched. So the rise is divided by sd, that timeframe's own trailing
// standard deviation of bar-to-bar moves. Both are prices, so the ratio is
// dimensionless and a NT$30 stock and a NT$600 one are on the same scale.
//
// Under a random walk a 5-period MA's one-bar change has stdev sd/sqrt(5),
// so 30 degrees (tan = 0.577) is about 1.3 of those and 45 about 2.2 — the
// interesting thresholds land where a chart reader would put them, which is
// the whole point of the construction.
//
// ok is false when sd is unusable; callers must not read deg in that case.
func maAngle(rise, sd float64) (deg float64, ok bool) {
	if sd <= 0 {
		return 0, false
	}
	return math.Atan(rise/sd) * 180 / math.Pi, true
}

// CheckMTFCrossExact evaluates candles' last bar against 【日週共振穿越】:
// daily MA5 and MA7 both rising at an angle of at least mtfDailyAngle,
// weekly MA5 and MA7 both rising at an angle of at least mtfWeeklyAngle, and
// RSI14 at or below mtfRSIMax.
//
// It is a STATE, not an event — there is no "just crossed" leg. The alert
// semantics come from the layers above: MTFCross's 5-day lookback plus
// Detector.CheckMTFCross's alert-once dedup together mean the alert fires on
// a day the state is true after being false for 5 bars, and that combination
// is what the numbers below were measured with.
//
// The weekly angle compares WEEKLY BAR to WEEKLY BAR (last completed week vs
// the one before). The obvious-looking alternative — today's week-to-date
// weekly MA vs yesterday's — degenerates on 4 of 5 weekdays into "today
// closed higher than yesterday", since the completed weeks in both readings
// are identical and only the running close differs. That over-fires by ~3.2x
// and is not what a weekly chart shows.
//
// # Result: measured 2026-09-04 — positive on TW, measurably NEGATIVE on US
//
// Measured as a WATCHLIST screen, not an entry trigger: the statistic is
// P(20-day return > +10%) on trigger days, demeaned by each TICKER's own
// base rate over the whole sample so what is left is timing rather than
// stock selection (the cross-sectional version of this same rule looks
// strongly positive on US, and that entire apparent edge is name selection —
// it inverts under the within-ticker control). Date-clustered bootstrap, 400
// resamples of dates, split 2021-11-01, 613 tickers / 1,497,014 bars
// (S&P 500 + tw150), re-arm 5 bars = the shipped semantics:
//
//	market  rule                        early split       late split
//	TW      angles only                 +3.66pp  2.2σ     +6.35pp  3.9σ
//	TW      + RSI<=80  (shipped)        +4.77pp  2.7σ     +5.67pp  3.2σ
//	US      angles only                 -7.16pp 12.2σ     -4.68pp  6.0σ
//	US      + RSI<=80  (shipped)        -6.93pp 10.9σ     -3.69pp  4.8σ
//
// TW clears §4.4's arithmetic (both splits past 1 SE, same sign) but the
// 20/30 degree thresholds were chosen with BOTH splits visible, so the split
// test is no longer out-of-sample for them — read it as "not refuted", not
// as validated. TW hit rate is 22.0% against a 14.6% base, but 60 of the 116
// tickers show a positive within-ticker excess and 56 negative, and removing
// the best 5% of triggers drops the mean 20-day return from +2.90% to
// +0.43%: the edge is a right tail, so the screen cannot be cherry-picked
// down to a few names without giving it back.
//
// US is negative past 1 SE in both splits and is NOT gated off, at the
// user's explicit direction (2026-09-04) — they want to watch it run. It
// carries i18n.KeyStrategyMTFCrossUS saying so. Do not read its presence as
// the numbers having changed, and do not feed it to the recommendation
// prompt: service.ComputeTechnicals stays TW-only.
//
// Also measured and rejected, so they don't get retried:
//   - MA20/MA30 price-cross legs (the previous shipped rule): structurally
//     near-mutually-exclusive with a steep weekly MA, 10/10 angle thresholds
//     fail with one.
//   - Slope expressed as %/week instead of an angle: non-monotone.
//   - A plain "last 5 weeks up >5%" momentum gate: its onset event fails
//     (-0.89 / +1.22).
//   - 平轉向上 (weekly MA flat last week, turning up this week), 16 flat-band
//     x turn-threshold cells plus 18 more with a daily-angle leg: on TW every
//     cell is negative early and positive late (sign flip, unstable); on US
//     every cell is negative in both splits. Early entry is measurably worse
//     than confirmation here.
//   - RSI band analysis on US: every bucket is negative including RSI<50, so
//     the US result is not an overheat problem and no cap fixes it.
//   - Market-breadth regime gate (% of the pool above its own MA200): 2018
//     6.9%->7.8%, 2022 8.6%->4.2%, full sample 22.0%->22.1%. Useless; the
//     per-stock angle gate already is the regime filter.
//   - Re-arm windows of 1, 3 and 7 bars: 1 scores best but the bot cannot
//     express it (MTFCross's lookback is 5), 7 is worse.
//
// Known limitation: the measurement universe is today's index membership
// back-filled 10 years — 613 of 613 tickers survive to the last bar, so
// delistings and index removals are absent entirely. That biases a
// strong-uptrend screen's TW result upward by an unquantified amount; it
// makes the US negative, if anything, an understatement.
func CheckMTFCrossExact(candles []data.Candle) bool {
	n := len(candles)
	if n < mtfMinBars {
		return false
	}
	closes := data.Closes(candles)

	prev := closes[:n-1]
	dSD := stepSD(closes, mtfDailySDBars)
	d5, ok5 := maAngle(MA(closes, 5)-MA(prev, 5), dSD)
	d7, ok7 := maAngle(MA(closes, 7)-MA(prev, 7), dSD)
	if !ok5 || !ok7 || math.Min(d5, d7) < mtfDailyAngle {
		return false
	}

	weekly := weeklyCloses(candles)
	wSD := stepSD(weekly, mtfWeeklySDWeeks)
	w5, okw5 := maAngle(wma(weekly, 5, 0)-wma(weekly, 5, 1), wSD)
	w7, okw7 := maAngle(wma(weekly, 7, 0)-wma(weekly, 7, 1), wSD)
	if !okw5 || !okw7 || wma(weekly, 7, 1) == 0 || math.Min(w5, w7) < mtfWeeklyAngle {
		return false
	}

	return RSI(closes, 14) <= mtfRSIMax
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

// CheckMTFCross adds alert-once-per-occurrence dedup on top of MTFCross.
// Together with MTFCross's 5-day lookback that is the re-arm-5 event the
// numbers in CheckMTFCrossExact were measured with: the state has to have
// been false for 5 bars before it can alert again.
//
// There is no market gate. US is measured negative in both splits and fires
// anyway because the user asked to observe it (2026-09-04); the warning
// rides on the message, not on a filter here — see
// service.DecorateStrategyHits.
func (d *Detector) CheckMTFCross(ticker string, candles []data.Candle, prevState string) (sig *Signal, newState string) {
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
