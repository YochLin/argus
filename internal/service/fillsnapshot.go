package service

import (
	"math"
	"time"

	"argus/internal/data"
	"argus/internal/signals"
)

// Phase 27 P6 — what a stock looked like on the day a trade was filled. All of
// it is computed on demand from daily candles (nothing but the news is stored,
// D2), and the indicators only ever see bars up to and including the fill day:
// the hindsight half is a separate function so a caller that must not see the
// future (the at-close trade review) can't pick it up by accident.

const (
	// fillMinBars is the history a snapshot needs before the fill bar: MA60 is
	// the longest reading in it.
	fillMinBars = 60
	// fillMaxGapDays is how far the fill date may sit past the bar it maps to —
	// a weekend plus a holiday. A US fill stamped with the next Taiwan date
	// before that session has traded maps back to the previous bar the same way.
	fillMaxGapDays = 5
	// fillCrossLookback caps the walk back for "days since the MACD histogram
	// changed sign".
	fillCrossLookback = 60
	// fillMACDMinBars is signals.MACD's warm-up: below it the function returns
	// zeros, which must not be read as a histogram of exactly zero.
	fillMACDMinBars = 26 + 9

	fillRSIHot, fillRSICold = 70.0, 30.0
	fillVolUp, fillVolDown  = 1.3, 0.7
)

const (
	TrendBull  = "bull"
	TrendBear  = "bear"
	TrendRange = "range"

	RSIHot  = "hot"
	RSICold = "cold"
	RSIMid  = "mid"

	VolUp   = "up"
	VolDown = "down"
	VolFlat = "flat"
)

// FillPattern is a candle pattern that ended on the fill bar. Descriptive only,
// like every pattern (see signals.DetectPatterns).
type FillPattern struct {
	Type string
	Dir  string
}

// FillSnapshot is the day's bar plus four indicator readings and the patterns
// that ended on it.
type FillSnapshot struct {
	Date                    string // the bar's own date
	Close, High, Low        float64
	DayChangePct            float64 // close vs the previous close
	RSI14, RSI14Prev5       float64 // Prev5 is the reading five bars earlier
	RSIZone                 string  // RSIHot / RSICold / RSIMid
	MACDDif, MACDDea, MACDH float64
	// MACDCrossDays counts bars since the histogram last changed sign, the fill
	// bar itself being day 1; it stops at fillCrossLookback.
	MACDCrossDays  int
	MACDWidening   bool // |histogram| grew since the previous bar
	Trend          string
	CloseVsMA20Pct float64
	MA20Slope5dPct float64 // MA20 now vs five bars ago
	VolRatio20     float64 // the bar's volume / the mean of the 20 bars before it
	VolRatio5v20   float64 // mean of the 5 bars before / the same 20
	VolState       string
	Patterns       []FillPattern
}

// FillHindsight is what the price did after the fill, as plain price moves from
// the fill price: nil where the candles don't reach. Whether a move was good
// depends on the side, which is the caller's to colour.
type FillHindsight struct {
	Fwd5Pct, Fwd20Pct *float64 // close N bars later
	MaxUpPct          *float64 // highest high in the next 20 bars
	MaxDownPct        *float64 // lowest low in the next 20 bars
}

// FillBar maps a fill date to the candle it traded in: the last bar on or
// before it. ok is false when there is no such bar, it is more than
// fillMaxGapDays older than the date, or the history before it is too short to
// read indicators from.
func FillBar(candles []data.Candle, fillDate string) (int, bool) {
	t, err := time.Parse("2006-01-02", fillDate)
	if err != nil {
		return 0, false
	}
	i := -1
	for j := range candles {
		if candles[j].Date.Format("2006-01-02") > fillDate {
			break
		}
		i = j
	}
	if i < fillMinBars-1 {
		return 0, false
	}
	day, _ := time.Parse("2006-01-02", candles[i].Date.Format("2006-01-02"))
	if t.Sub(day) > fillMaxGapDays*24*time.Hour {
		return 0, false
	}
	return i, true
}

// SnapshotAt reads the indicators off candles[:i+1] and nothing later. i must
// come from FillBar.
func SnapshotAt(candles []data.Candle, i int) FillSnapshot {
	cs := candles[:i+1]
	closes := data.Closes(cs)
	c := cs[i]
	s := FillSnapshot{Date: c.Date.Format("2006-01-02"), Close: c.Close, High: c.High, Low: c.Low}
	if i > 0 && closes[i-1] != 0 {
		s.DayChangePct = (c.Close/closes[i-1] - 1) * 100
	}

	s.RSI14 = signals.RSI(closes, 14)
	s.RSI14Prev5 = signals.RSI(closes[:i-4], 14)
	switch {
	case s.RSI14 >= fillRSIHot:
		s.RSIZone = RSIHot
	case s.RSI14 <= fillRSICold:
		s.RSIZone = RSICold
	default:
		s.RSIZone = RSIMid
	}

	s.MACDDif, s.MACDDea, s.MACDH = signals.MACD(closes)
	s.MACDCrossDays = 1
	for k := i - 1; k >= fillMACDMinBars-1 && s.MACDCrossDays < fillCrossLookback; k-- {
		_, _, h := signals.MACD(closes[:k+1])
		if (h > 0) != (s.MACDH > 0) {
			break
		}
		s.MACDCrossDays++
	}
	if i > 0 {
		_, _, prev := signals.MACD(closes[:i])
		s.MACDWidening = math.Abs(s.MACDH) > math.Abs(prev)
	}

	ma20, ma60 := signals.MA(closes, 20), signals.MA(closes, 60)
	ma20Prev := signals.MA(closes[:i-4], 20)
	s.CloseVsMA20Pct = (c.Close/ma20 - 1) * 100
	s.MA20Slope5dPct = (ma20/ma20Prev - 1) * 100
	switch {
	case c.Close > ma20 && ma20 > ma60 && s.MA20Slope5dPct > 0:
		s.Trend = TrendBull
	case c.Close < ma20 && ma20 < ma60 && s.MA20Slope5dPct < 0:
		s.Trend = TrendBear
	default:
		s.Trend = TrendRange
	}

	// Volume is measured against the bars *before* the fill bar, so a spike
	// isn't diluted by itself.
	if avg20 := meanVolume(cs[i-20 : i]); avg20 > 0 {
		s.VolRatio20 = float64(c.Volume) / avg20
		s.VolRatio5v20 = meanVolume(cs[i-5:i]) / avg20
	}
	switch {
	case s.VolRatio20 >= fillVolUp:
		s.VolState = VolUp
	case s.VolRatio20 > 0 && s.VolRatio20 <= fillVolDown:
		s.VolState = VolDown
	default:
		s.VolState = VolFlat
	}

	for _, p := range signals.DetectPatterns(cs) {
		if p.End == i {
			s.Patterns = append(s.Patterns, FillPattern{Type: string(p.Type), Dir: p.Dir})
		}
	}
	return s
}

// HindsightAfter reads what happened after the bar at i, measured from the
// price the fill actually got.
func HindsightAfter(candles []data.Candle, i int, fillPrice float64) FillHindsight {
	var h FillHindsight
	if fillPrice <= 0 {
		return h
	}
	move := func(v float64) *float64 { p := (v/fillPrice - 1) * 100; return &p }
	if i+5 < len(candles) {
		h.Fwd5Pct = move(candles[i+5].Close)
	}
	if i+20 < len(candles) {
		h.Fwd20Pct = move(candles[i+20].Close)
	}
	end := i + 20
	if end > len(candles)-1 {
		end = len(candles) - 1
	}
	if end > i {
		hi, lo := math.Inf(-1), math.Inf(1)
		for _, c := range candles[i+1 : end+1] {
			hi, lo = math.Max(hi, c.High), math.Min(lo, c.Low)
		}
		h.MaxUpPct, h.MaxDownPct = move(hi), move(lo)
	}
	return h
}

func meanVolume(cs []data.Candle) float64 {
	var sum float64
	for _, c := range cs {
		sum += float64(c.Volume)
	}
	return sum / float64(len(cs))
}
