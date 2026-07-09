package signals

import (
	"math"

	"argus/internal/data"
	"argus/internal/i18n"
)

type Signal struct {
	Ticker  string
	Type    string
	Message string
}

// Signal-state vocabulary shared with the caller's persistence layer
// (db.signal_states rows are keyed by ticker + family and hold one of the
// state values). The stateful checks below take the previously persisted
// state and return the new one, so this package stays free of any DB
// dependency.
const (
	FamilyRSI  = "rsi"
	FamilyMACD = "macd"

	StateBullish    = "bullish"
	StateBearish    = "bearish"
	StateOverbought = "overbought"
	StateOversold   = "oversold"
	StateNormal     = "normal"
)

type Detector struct {
	priceThresholdPct float64 // alert if abs(changePercent) >= this
	volumeMultiplier  float64 // alert if volume >= avgVolume * this
	rsiOverbought     float64
	rsiOversold       float64
	lang              i18n.Lang
}

func NewDetector(lang i18n.Lang) *Detector {
	return &Detector{
		priceThresholdPct: 3.0,
		volumeMultiplier:  2.0,
		rsiOverbought:     70,
		rsiOversold:       30,
		lang:              lang,
	}
}

func (d *Detector) CheckQuote(q *data.Quote) []Signal {
	var signals []Signal

	// Price movement signal
	if math.Abs(q.ChangePercent) >= d.priceThresholdPct {
		dirKey := i18n.KeyPriceUp
		if q.ChangePercent < 0 {
			dirKey = i18n.KeyPriceDown
		}
		dir := i18n.T(d.lang, dirKey)
		signals = append(signals, Signal{
			Ticker:  q.Ticker,
			Type:    "price",
			Message: i18n.T(d.lang, i18n.KeyPriceSignal, q.Ticker, dir, math.Abs(q.ChangePercent), q.Price),
		})
	}

	return signals
}

// RSI calculates the Relative Strength Index over the most recent period+1
// closes in the slice (closes is oldest-first, as returned by
// HistoryProvider.GetHistory) — not the oldest period+1, so a longer history
// slice still yields today's RSI rather than one from months ago.
func RSI(closes []float64, period int) float64 {
	if len(closes) < period+1 {
		return 50
	}
	closes = closes[len(closes)-period-1:]

	var gains, losses float64
	for i := 1; i <= period; i++ {
		diff := closes[i] - closes[i-1]
		if diff > 0 {
			gains += diff
		} else {
			losses += math.Abs(diff)
		}
	}

	avgGain := gains / float64(period)
	avgLoss := losses / float64(period)

	if avgLoss == 0 {
		return 100
	}
	rs := avgGain / avgLoss
	return 100 - (100 / (1 + rs))
}

// MA returns the simple moving average of the trailing period closes (closes
// is oldest-first, same convention as RSI/MACD), or 0 if there isn't enough
// history yet — e.g. a recent IPO won't have ~200 days of closes for MA200.
func MA(closes []float64, period int) float64 {
	if len(closes) < period {
		return 0
	}
	closes = closes[len(closes)-period:]
	var sum float64
	for _, c := range closes {
		sum += c
	}
	return sum / float64(period)
}

// CheckRSIState is the deduplicated version of CheckRSI: it only returns a
// signal when RSI newly enters overbought/oversold territory relative to
// prevState (the state persisted after the previous check; "" reads as
// normal). While RSI stays in the same zone on consecutive days, no repeat
// signal fires. newState is what the caller should persist for next time.
func (d *Detector) CheckRSIState(ticker string, closes []float64, prevState string) (sig *Signal, newState string) {
	rsi := RSI(closes, 14)
	newState = StateNormal
	if rsi >= d.rsiOverbought {
		newState = StateOverbought
	} else if rsi <= d.rsiOversold {
		newState = StateOversold
	}
	if newState == prevState || newState == StateNormal {
		return nil, newState
	}
	if newState == StateOverbought {
		return &Signal{
			Ticker:  ticker,
			Type:    "rsi_overbought",
			Message: i18n.T(d.lang, i18n.KeyRSIOverbought, ticker, rsi),
		}, newState
	}
	return &Signal{
		Ticker:  ticker,
		Type:    "rsi_oversold",
		Message: i18n.T(d.lang, i18n.KeyRSIOversold, ticker, rsi),
	}, newState
}

// CheckRSI returns a signal if RSI is overbought or oversold.
func (d *Detector) CheckRSI(ticker string, closes []float64) *Signal {
	rsi := RSI(closes, 14)
	if rsi >= d.rsiOverbought {
		return &Signal{
			Ticker:  ticker,
			Type:    "rsi_overbought",
			Message: i18n.T(d.lang, i18n.KeyRSIOverbought, ticker, rsi),
		}
	}
	if rsi <= d.rsiOversold {
		return &Signal{
			Ticker:  ticker,
			Type:    "rsi_oversold",
			Message: i18n.T(d.lang, i18n.KeyRSIOversold, ticker, rsi),
		}
	}
	return nil
}

// MACD returns the MACD line, its 9-period EMA signal line, and their
// difference (histogram) for the latest point. The signal line needs a
// genuine EMA over the MACD series itself, not just the two most recent
// EMA12/EMA26 values, so this requires enough history to let that series
// warm up (26 bars for MACD + 9 more for the signal EMA).
func MACD(closes []float64) (macd, signal, histogram float64) {
	if len(closes) < 26+9 {
		return 0, 0, 0
	}
	ema12 := emaSeries(closes, 12)
	ema26 := emaSeries(closes, 26)

	macdSeries := make([]float64, len(closes))
	for i := range closes {
		macdSeries[i] = ema12[i] - ema26[i]
	}

	// The MACD series is noisy before EMA26 has warmed up, so seed the
	// signal EMA starting at index 25 (the first bar EMA26 covers a full
	// window) instead of index 0.
	signalSeries := emaSeries(macdSeries[25:], 9)

	macd = macdSeries[len(macdSeries)-1]
	signal = signalSeries[len(signalSeries)-1]
	histogram = macd - signal
	return macd, signal, histogram
}

// emaSeries returns the exponential moving average at every point in values,
// seeded with the first value (the standard approach when no prior EMA is
// available).
func emaSeries(values []float64, period int) []float64 {
	if len(values) == 0 {
		return nil
	}
	k := 2.0 / float64(period+1)
	series := make([]float64, len(values))
	series[0] = values[0]
	for i := 1; i < len(values); i++ {
		series[i] = values[i]*k + series[i-1]*(1-k)
	}
	return series
}

// MACDTrend reduces the latest MACD reading to a trend state: StateBullish
// when the MACD line is above its signal line, StateBearish when below, and
// "" when there isn't enough history to compute MACD (or the two lines are
// exactly equal).
func MACDTrend(closes []float64) string {
	macdLine, signalLine, histogram := MACD(closes)
	if macdLine == 0 && signalLine == 0 && histogram == 0 {
		return ""
	}
	if histogram > 0 {
		return StateBullish
	}
	if histogram < 0 {
		return StateBearish
	}
	return ""
}

// CheckMACDCross detects an actual golden/death cross by comparing today's
// MACD trend against prevState (the trend persisted after the previous
// check). Unlike CheckMACD — which reflects the standing trend and fires
// every call while it holds — this only signals on the day the trend flips:
// bearish→bullish is a golden cross, bullish→bearish a death cross. A first
// observation (prevState == "") just establishes the baseline without
// signaling, since no flip can be seen yet. newState is what the caller
// should persist for next time; when there's no data it stays prevState so
// a transient history outage doesn't erase the baseline.
func (d *Detector) CheckMACDCross(ticker string, closes []float64, prevState string) (sig *Signal, newState string) {
	trend := MACDTrend(closes)
	if trend == "" {
		return nil, prevState
	}
	if prevState == "" || trend == prevState {
		return nil, trend
	}
	macdLine, signalLine, _ := MACD(closes)
	if trend == StateBullish {
		return &Signal{
			Ticker:  ticker,
			Type:    "macd_golden_cross",
			Message: i18n.T(d.lang, i18n.KeyMACDGoldenCross, ticker, macdLine, signalLine),
		}, trend
	}
	return &Signal{
		Ticker:  ticker,
		Type:    "macd_death_cross",
		Message: i18n.T(d.lang, i18n.KeyMACDDeathCross, ticker, macdLine, signalLine),
	}, trend
}

// CheckMACD returns a signal when the MACD line and its signal line disagree
// in direction with the histogram, indicating trending momentum. It reflects
// the latest bar's state, not a fresh crossover — it will keep firing every
// call while the trend holds, not just on the day it flips.
func (d *Detector) CheckMACD(ticker string, closes []float64) *Signal {
	macdLine, signalLine, histogram := MACD(closes)
	if macdLine == 0 && signalLine == 0 && histogram == 0 {
		return nil
	}
	if histogram > 0 && macdLine > signalLine {
		return &Signal{
			Ticker:  ticker,
			Type:    "macd_bullish",
			Message: i18n.T(d.lang, i18n.KeyMACDBullish, ticker, macdLine, signalLine),
		}
	}
	if histogram < 0 && macdLine < signalLine {
		return &Signal{
			Ticker:  ticker,
			Type:    "macd_bearish",
			Message: i18n.T(d.lang, i18n.KeyMACDBearish, ticker, macdLine, signalLine),
		}
	}
	return nil
}
