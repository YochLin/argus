package signals

import (
	"sort"

	"argus/internal/data"
)

// The stock chart's 下跌事件 tab, ported from the design's `tkDrops`: a run of
// consecutive lower closes that adds up, or a gap down past the previous low.
// Same footing as DetectPatterns — no backtest behind these thresholds, so
// this is descriptive only and must not feed alerts, the LLM prompt or the
// recommendation ranking. It is a separate rule from CheckPriceEvent on
// purpose: that one decides what gets pushed to Telegram, this one what gets
// drawn on a chart.
const (
	DropKindStreak = "streak"
	DropKindGap    = "gap"

	dropStreakMinBars = 3
	dropStreakPct     = -6.0
	dropGapPct        = -2.5
	dropFwdBars       = 10
)

// DropEvent is one drop in the candles it was detected on. Start and End are
// candle indices of the falling bars, so PreClose is the close at Start-1.
type DropEvent struct {
	Kind        string
	Start, End  int
	Pct         float64  // close at End vs PreClose
	GapPct      *float64 // open vs previous close; on a streak, its deepest gap, nil if none
	PreClose    float64
	Low         float64  // lowest low from Start to End
	RecoverBars *int     // bars after End until a close back at PreClose; nil if not yet
	Fwd10Pct    *float64 // close 10 bars after End vs close at End; nil near the end of the data
}

// DetectDrops returns candles' drop events, oldest first. Callers must not
// pass a bar still in session: its close isn't one yet.
func DetectDrops(candles []data.Candle) []DropEvent {
	var out []DropEvent
	inStreak := make([]int, len(candles)) // candle index -> 1 + index into out
	for i := 1; i < len(candles); {
		if candles[i].Close >= candles[i-1].Close {
			i++
			continue
		}
		j := i
		for j+1 < len(candles) && candles[j+1].Close < candles[j].Close {
			j++
		}
		if pre := candles[i-1].Close; pre > 0 && j-i+1 >= dropStreakMinBars {
			if pct := (candles[j].Close/pre - 1) * 100; pct <= dropStreakPct {
				out = append(out, DropEvent{Kind: DropKindStreak, Start: i, End: j, Pct: pct})
				for k := i; k <= j; k++ {
					inStreak[k] = len(out)
				}
			}
		}
		i = j + 1
	}

	for i := 1; i < len(candles); i++ {
		prev := candles[i-1]
		if prev.Close <= 0 || candles[i].Open >= prev.Low {
			continue
		}
		g := (candles[i].Open/prev.Close - 1) * 100
		if g > dropGapPct {
			continue
		}
		if n := inStreak[i]; n > 0 {
			if e := &out[n-1]; e.GapPct == nil || g < *e.GapPct {
				e.GapPct = &g
			}
			continue
		}
		pct := (candles[i].Close/prev.Close - 1) * 100
		out = append(out, DropEvent{Kind: DropKindGap, Start: i, End: i, Pct: pct, GapPct: &g})
	}

	sort.Slice(out, func(a, b int) bool { return out[a].Start < out[b].Start })
	for i := range out {
		fillDropOutcome(&out[i], candles)
	}
	return out
}

func fillDropOutcome(e *DropEvent, candles []data.Candle) {
	e.PreClose = candles[e.Start-1].Close
	e.Low = candles[e.Start].Low
	for _, c := range candles[e.Start : e.End+1] {
		if c.Low < e.Low {
			e.Low = c.Low
		}
	}
	for k := e.End + 1; k < len(candles); k++ {
		if candles[k].Close >= e.PreClose {
			bars := k - e.End
			e.RecoverBars = &bars
			break
		}
	}
	if k := e.End + dropFwdBars; k < len(candles) && candles[e.End].Close > 0 {
		pct := (candles[k].Close/candles[e.End].Close - 1) * 100
		e.Fwd10Pct = &pct
	}
}
