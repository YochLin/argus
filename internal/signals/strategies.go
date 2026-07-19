package signals

import (
	"math"

	"argus/internal/data"
	"argus/internal/i18n"
)

const (
	strategyLookbackDays = 5       // 「近 N 日內曾觸發」窗口,配合 universe scan 5 天輪掃
	minAvgVolume5d       = 500_000 // 網 1 流動性門檻(股)
	squeezeBWWindow      = 20      // 帶寬新低的回看窗
	breakoutVolMult      = 2.0     // 突破日量 >= 2x 前 5 日均量
	boxWindowDays        = 30      // 網 2 箱型窗口
	boxMaxRangePct       = 15.0    // 箱型高低差上限 %
	boxFloorPct          = 2.0     // 距箱底 %
	kdOversold           = 25.0    // K 低檔門檻
	rsiReboundLevel      = 30.0    // RSI 超賣線
	rsiReboundLookback   = 3       // RSI 近 N 日內曾 < 30

	FamilyStrategySqueeze = "strategy_squeeze"
	FamilyStrategyBox     = "strategy_box"
)

type StrategyHit struct {
	Name    string // "squeeze_breakout" / "box_bottom"
	DaysAgo int    // 0 = 今天觸發
}

// SqueezeBreakout evaluates candles for Squeeze Breakout triggers within the last strategyLookbackDays.
// Returns the most recent hit (smallest DaysAgo) or nil if none triggered.
func SqueezeBreakout(candles []data.Candle) *StrategyHit {
	n := len(candles)
	for offset := 0; offset < strategyLookbackDays; offset++ {
		evalIdx := n - 1 - offset
		if evalIdx < 60 {
			break
		}
		sub := candles[:evalIdx+1]
		if CheckSqueezeBreakoutExact(sub) {
			return &StrategyHit{
				Name:    "squeeze_breakout",
				DaysAgo: offset,
			}
		}
	}
	return nil
}

// BoxBottomRebound evaluates candles for Box Bottom Rebound triggers within the last strategyLookbackDays.
// Returns the most recent hit (smallest DaysAgo) or nil if none triggered.
func BoxBottomRebound(candles []data.Candle) *StrategyHit {
	n := len(candles)
	for offset := 0; offset < strategyLookbackDays; offset++ {
		evalIdx := n - 1 - offset
		if evalIdx < 60 {
			break
		}
		sub := candles[:evalIdx+1]
		if CheckBoxBottomReboundExact(sub) {
			return &StrategyHit{
				Name:    "box_bottom",
				DaysAgo: offset,
			}
		}
	}
	return nil
}

func CheckSqueezeBreakoutExact(candles []data.Candle) bool {
	n := len(candles)
	if n < 60 {
		return false
	}
	closes := data.Closes(candles)
	volumes := data.Volumes(candles)

	// 1. Liquidity: avg volume of preceding 5 days > minAvgVolume5d
	window5v := volumes[n-6 : n-1]
	var sumV int64
	for _, v := range window5v {
		sumV += v
	}
	avgV5 := float64(sumV) / 5.0
	if avgV5 < minAvgVolume5d {
		return false
	}

	// 2. Squeeze: min bandwidth in trailing 5 bars == min bandwidth in trailing 20 bars
	bwSeries := BollingerBandwidthSeries(closes, 20, 2.0)
	if len(bwSeries) < 20 {
		return false
	}
	window20bw := bwSeries[len(bwSeries)-squeezeBWWindow:]
	min20bw := window20bw[0]
	for _, bw := range window20bw {
		if bw < min20bw {
			min20bw = bw
		}
	}
	window5bw := bwSeries[len(bwSeries)-5:]
	min5bw := window5bw[0]
	for _, bw := range window5bw {
		if bw < min5bw {
			min5bw = bw
		}
	}
	if math.Abs(min5bw-min20bw) > 1e-9 {
		return false
	}

	// 3. Breakout: close > Bollinger Upper (20, 2.0) and today vol >= 2.0 * avgV5
	mean, upper, _, ok := bollingerBands(closes, 20, 2.0)
	if !ok || mean == 0 {
		return false
	}
	evalClose := closes[n-1]
	evalVol := float64(volumes[n-1])
	if evalClose <= upper || evalVol < breakoutVolMult*avgV5 {
		return false
	}

	// 4. Confirmation: MACD histogram today > 0 and yesterday <= 0
	_, _, histToday := MACD(closes)
	_, _, histPrev := MACD(closes[:n-1])
	if histToday <= 0 || histPrev > 0 {
		return false
	}

	return true
}

func CheckBoxBottomReboundExact(candles []data.Candle) bool {
	n := len(candles)
	if n < 60 {
		return false
	}
	highs := data.Highs(candles)
	lows := data.Lows(candles)
	closes := data.Closes(candles)

	// 1. Box shape: 30-day close range <= 15%
	boxCloses := closes[n-boxWindowDays:]
	minC := boxCloses[0]
	maxC := boxCloses[0]
	for _, c := range boxCloses {
		if c < minC {
			minC = c
		}
		if c > maxC {
			maxC = c
		}
	}
	if minC <= 0 || (maxC-minC)/minC*100.0 > boxMaxRangePct {
		return false
	}

	// 2. Floor: eval close distance to 30-day min close <= 2%
	evalClose := closes[n-1]
	if (evalClose-minC)/minC*100.0 > boxFloorPct {
		return false
	}

	// 3. KD: K < 25 AND (golden cross OR hook up)
	kSeries, dSeries := StochasticSeries(highs, lows, closes, 9, 3)
	if kSeries == nil || dSeries == nil || len(kSeries) < 2 {
		return false
	}
	kToday := kSeries[n-1]
	dToday := dSeries[n-1]
	kPrev := kSeries[n-2]
	dPrev := dSeries[n-2]

	if kToday >= kdOversold {
		return false
	}
	goldenCross := kPrev <= dPrev && kToday > dToday
	hookUp := kPrev < kdOversold && kToday > kPrev
	if !goldenCross && !hookUp {
		return false
	}

	// 4. RSI: within last rsiReboundLookback (3) days RSI was < 30 AND RSI today > RSI yesterday
	rsiToday := RSI(closes, 14)
	rsiPrev := RSI(closes[:n-1], 14)
	if rsiToday <= rsiPrev {
		return false
	}
	wasOversold := false
	for i := n - rsiReboundLookback; i < n; i++ {
		if RSI(closes[:i+1], 14) < rsiReboundLevel {
			wasOversold = true
			break
		}
	}
	if !wasOversold {
		return false
	}

	return true
}

func (d *Detector) CheckSqueezeBreakout(ticker string, candles []data.Candle, prevState string) (sig *Signal, newState string) {
	hit := SqueezeBreakout(candles)
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
		Type:    "strategy_squeeze_breakout",
		Message: i18n.T(d.lang, i18n.KeyStrategySqueezeBreakout, ticker, daysAgoStr),
	}, newState
}

func (d *Detector) CheckBoxBottom(ticker string, candles []data.Candle, prevState string) (sig *Signal, newState string) {
	hit := BoxBottomRebound(candles)
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
		Type:    "strategy_box_bottom",
		Message: i18n.T(d.lang, i18n.KeyStrategyBoxBottom, ticker, daysAgoStr),
	}, newState
}
