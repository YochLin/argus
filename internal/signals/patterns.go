package signals

import (
	"math"
	"sort"

	"argus/internal/data"
)

// Phase 27 P4b — the stock chart's candlestick / volume / gap pattern
// detector. Textbook heuristics ported one-to-one from the design's `tkPatterns`
// (docs/phase-27-stock-chart-redesign.md), with NO backtest behind them: this
// is descriptive only and must not feed alerts, the LLM prompt or the
// recommendation ranking (same rule as a screen measured negative — see
// service.ComputeTechnicals). Pure function over candles, like the rest of
// this package; display copy lives in the web frontend, this returns numbers.

type PatternType string

const (
	PatBullEngulf PatternType = "bullEngulf"
	PatBearEngulf PatternType = "bearEngulf"
	PatHammer     PatternType = "hammer"
	PatShooting   PatternType = "shooting"
	PatMorning    PatternType = "morning"
	PatEvening    PatternType = "evening"
	PatSoldiers   PatternType = "soldiers"
	PatCrows      PatternType = "crows"
	PatDoji       PatternType = "doji"
	PatVolBreak   PatternType = "volBreak"
	PatHangingMan PatternType = "hangingMan"
	PatInvHammer  PatternType = "invHammer"
	PatBullHarami PatternType = "bullHarami"
	PatBearHarami PatternType = "bearHarami"
	PatPiercing   PatternType = "piercing"
	PatDarkCloud  PatternType = "darkCloud"
	PatVolDry     PatternType = "volDry"
	PatVolDiverge PatternType = "volDiverge"
	PatGapUp      PatternType = "gapUp"
	PatGapDown    PatternType = "gapDown"
	PatVolDump    PatternType = "volDump"
)

// Cat/Dir/Conf are plain strings on the wire; the frontend keys its names and
// definitions off Type, and its filter chips off Cat.
const (
	PatCatReversal     = "rev"
	PatCatContinuation = "cont"
	PatCatIndecision   = "indec"
	PatCatVolume       = "vol"
	PatCatGap          = "gap"

	PatDirBull    = "bull"
	PatDirBear    = "bear"
	PatDirNeutral = "neu"

	PatConfHigh = "high"
	PatConfMid  = "mid"
	PatConfLow  = "low"
)

var patternMeta = map[PatternType]struct{ cat, dir string }{
	PatBullEngulf: {PatCatReversal, PatDirBull}, PatBearEngulf: {PatCatReversal, PatDirBear},
	PatHammer: {PatCatReversal, PatDirBull}, PatShooting: {PatCatReversal, PatDirBear},
	PatMorning: {PatCatReversal, PatDirBull}, PatEvening: {PatCatReversal, PatDirBear},
	PatHangingMan: {PatCatReversal, PatDirBear}, PatInvHammer: {PatCatReversal, PatDirBull},
	PatBullHarami: {PatCatReversal, PatDirBull}, PatBearHarami: {PatCatReversal, PatDirBear},
	PatPiercing: {PatCatReversal, PatDirBull}, PatDarkCloud: {PatCatReversal, PatDirBear},
	PatSoldiers: {PatCatContinuation, PatDirBull}, PatCrows: {PatCatContinuation, PatDirBear},
	PatDoji:     {PatCatIndecision, PatDirNeutral},
	PatVolBreak: {PatCatVolume, PatDirBull}, PatVolDump: {PatCatVolume, PatDirBear},
	PatVolDry: {PatCatVolume, PatDirNeutral}, PatVolDiverge: {PatCatVolume, PatDirBear},
	PatGapUp: {PatCatGap, PatDirBull}, PatGapDown: {PatCatGap, PatDirBear},
}

// Pattern is one detection. Start/End are indices into the candles passed to
// DetectPatterns (End is the bar that completes the pattern).
type Pattern struct {
	Type       PatternType
	Cat, Dir   string
	Start, End int
	// Conf is "high"/"mid"/"low", or "" for gaps (which have no such score).
	// Three one-point checks on the End bar: volume > 1.4x its prior-20 average,
	// body > 1.5x its prior-20 average body, and "context fits". The last only
	// means something for reversals (a bullish one needs the prior 5 days down,
	// a bearish one up); every other category always gets the point, so a
	// continuation / volume / doji pattern reaches mid on either of the other two
	// checks and high on both — not a bug.
	Conf string
	// BodyRatio/VolRatio: the End bar's body / volume over its prior-20 average.
	// Prior5Pct: the move into the pattern, Start's open vs the close 5 bars
	// before it. Not filled for gaps.
	BodyRatio, VolRatio, Prior5Pct float64
	// Extra is the pattern's own measure:
	//   engulfing / harami   body / previous body
	//   piercing / dark cloud  % of the previous body the close reached
	//   hammer / hanging man   lower shadow / body
	//   shooting star / inverted hammer  upper shadow / body
	//   volume breakout / breakdown  the prior 20-day high / low it cleared
	//   divergence   this bar's volume / the volume on the prior high (RefIdx)
	//   volume dry-up  10-day high-to-low range, %
	// Zero for the rest.
	Extra  float64
	RefIdx int // divergence only: the prior high's bar; -1 otherwise
	// Gaps only: the empty price band, and the first later bar that traded back
	// into it (FillIdx, -1 while still open).
	GapLo, GapHi float64
	FillIdx      int
	// Fwd5Pct is the close 5 bars after End, vs End's close; nil near the end.
	Fwd5Pct *float64
}

const (
	patAvgWindow    = 20
	patVolDryWindow = 10
)

// DetectPatterns scans candles (oldest first) and returns every pattern found,
// ordered by End then Start. Bars with fewer than 20 predecessors get no
// candlestick/volume patterns (their averages don't exist yet); gaps need only
// the previous bar. Bars whose prior-20 average body, range or volume is zero
// (a halted or flat stretch) are skipped for the same reason — there's nothing
// to be "larger than".
func DetectPatterns(cs []data.Candle) []Pattern {
	var out []Pattern
	// lastMatch: last bar a run-suppressed pattern matched (even when not
	// emitted), so a 5-bar streak fires once, at its start. lastEmit: last bar a
	// cooldown pattern was emitted.
	lastMatch := map[PatternType]int{}
	lastEmit := map[PatternType]int{}

	body := func(c data.Candle) float64 { return math.Abs(c.Close - c.Open) }
	rng := func(c data.Candle) float64 { return c.High - c.Low }
	vol := func(c data.Candle) float64 { return float64(c.Volume) }
	green := func(c data.Candle) bool { return c.Close >= c.Open }
	avg := func(i int, f func(data.Candle) float64) float64 {
		s := 0.0
		for j := i - patAvgWindow; j < i; j++ {
			s += f(cs[j])
		}
		return s / patAvgWindow
	}
	nz := func(x float64) float64 { // a zero body would divide by zero below
		if x == 0 {
			return 0.0001
		}
		return x
	}

	add := func(t PatternType, start, end int, extra float64) {
		m := patternMeta[t]
		trend := 0.0
		if end >= 5 && start >= 5 {
			trend = (cs[start].Open/cs[start-5].Close - 1) * 100
		}
		bodyR := body(cs[end]) / avg(end, body)
		volR := vol(cs[end]) / avg(end, vol)
		aligned := m.dir == PatDirNeutral || m.cat != PatCatReversal ||
			(m.dir == PatDirBull && trend < 0) || (m.dir == PatDirBear && trend > 0)
		score := 0
		if volR > 1.4 {
			score++
		}
		if bodyR > 1.5 {
			score++
		}
		if aligned {
			score++
		}
		conf := PatConfLow
		switch {
		case score >= 3:
			conf = PatConfHigh
		case score == 2:
			conf = PatConfMid
		}
		out = append(out, Pattern{
			Type: t, Cat: m.cat, Dir: m.dir, Start: start, End: end, Conf: conf,
			BodyRatio: bodyR, VolRatio: volR, Prior5Pct: trend, Extra: extra, RefIdx: -1, FillIdx: -1,
		})
		lastEmit[t] = end
	}

	for i := 1; i < len(cs); i++ {
		c, p := cs[i], cs[i-1]

		if i >= patAvgWindow {
			aB, aR, aV := avg(i, body), avg(i, rng), avg(i, vol)
			if aB > 0 && aR > 0 && aV > 0 {
				q := cs[i-2]
				up5, dn5 := cs[i-1].Close > cs[i-6].Close, cs[i-1].Close < cs[i-6].Close
				b := nz(body(c))
				lo := math.Min(c.Open, c.Close) - c.Low
				hi := c.High - math.Max(c.Open, c.Close)

				// One two-bar pattern per bar, in this order.
				switch {
				case !green(p) && green(c) && c.Close > p.Open && c.Open <= p.Close && body(c) > 1.3*aB && body(c) > body(p)*1.1:
					add(PatBullEngulf, i-1, i, body(c)/nz(body(p)))
				case green(p) && !green(c) && c.Close < p.Open && c.Open >= p.Close && body(c) > 1.3*aB && body(c) > body(p)*1.1:
					add(PatBearEngulf, i-1, i, body(c)/nz(body(p)))
				case !green(p) && body(p) > 1.2*aB && green(c) && c.Open <= p.Close && c.Close > (p.Open+p.Close)/2 && c.Close < p.Open:
					add(PatPiercing, i-1, i, (c.Close-p.Close)/(p.Open-p.Close)*100)
				case green(p) && body(p) > 1.2*aB && !green(c) && c.Open >= p.Close && c.Close < (p.Open+p.Close)/2 && c.Close > p.Open:
					add(PatDarkCloud, i-1, i, (p.Close-c.Close)/(p.Close-p.Open)*100)
				case body(p) > 1.2*aB && body(c) < 0.5*body(p) &&
					math.Max(c.Open, c.Close) <= math.Max(p.Open, p.Close) &&
					math.Min(c.Open, c.Close) >= math.Min(p.Open, p.Close) && green(p) != green(c):
					t := PatBearHarami
					if green(c) {
						t = PatBullHarami
					}
					add(t, i-1, i, body(c)/nz(body(p)))
				}

				hamShape := lo >= 2*b && hi <= 0.5*b && rng(c) > 0.8*aR
				starShape := hi >= 2*b && lo <= 0.5*b && rng(c) > 0.8*aR
				switch {
				case hamShape && (dn5 || up5):
					t := PatHangingMan
					if dn5 {
						t = PatHammer
					}
					add(t, i, i, lo/b)
				case starShape && (dn5 || up5):
					t := PatInvHammer
					if up5 {
						t = PatShooting
					}
					add(t, i, i, hi/b)
				}

				if !green(q) && body(q) > 1.2*aB && body(p) < 0.45*aB && green(c) && c.Close > (q.Open+q.Close)/2 {
					add(PatMorning, i-2, i, 0)
				} else if green(q) && body(q) > 1.2*aB && body(p) < 0.45*aB && !green(c) && c.Close < (q.Open+q.Close)/2 {
					add(PatEvening, i-2, i, 0)
				}

				minBody := math.Min(body(q), math.Min(body(p), body(c)))
				switch {
				case green(q) && green(p) && green(c) && p.Close > q.Close && c.Close > p.Close && minBody > 0.7*aB:
					if last, ok := lastMatch[PatSoldiers]; !ok || last != i-1 {
						add(PatSoldiers, i-2, i, 0)
					}
					lastMatch[PatSoldiers] = i
				case !green(q) && !green(p) && !green(c) && p.Close < q.Close && c.Close < p.Close && minBody > 0.7*aB:
					if last, ok := lastMatch[PatCrows]; !ok || last != i-1 {
						add(PatCrows, i-2, i, 0)
					}
					lastMatch[PatCrows] = i
				}

				if body(c) <= 0.08*rng(c) && rng(c) > 0.7*aR {
					add(PatDoji, i, i, 0)
				}

				hh, ll := cs[i-patAvgWindow].High, cs[i-patAvgWindow].Low
				hiIdx := i - patAvgWindow
				for j := i - patAvgWindow; j < i; j++ {
					hh, ll = math.Max(hh, cs[j].High), math.Min(ll, cs[j].Low)
					if cs[j].High >= cs[hiIdx].High {
						hiIdx = j // the latest bar among equal highs
					}
				}
				switch {
				case c.Close > hh && vol(c) > 1.8*aV:
					add(PatVolBreak, i, i, hh)
				case c.Close < ll && vol(c) > 1.8*aV:
					add(PatVolDump, i, i, ll)
				case c.Close > hh && vol(cs[hiIdx]) > vol(c)*1.05:
					add(PatVolDiverge, i, i, vol(c)/vol(cs[hiIdx]))
					out[len(out)-1].RefIdx = hiIdx
				}

				hi10, lo10 := c.High, c.Low
				for j := i - patVolDryWindow + 1; j < i; j++ {
					hi10, lo10 = math.Max(hi10, cs[j].High), math.Min(lo10, cs[j].Low)
				}
				if lo10 > 0 && vol(c) < 0.5*aV {
					r10 := (hi10/lo10 - 1) * 100
					if last, ok := lastEmit[PatVolDry]; r10 < 9 && (!ok || last < i-3) {
						add(PatVolDry, i, i, r10)
					}
				}
			}
		}

		if c.Low > p.High || c.High < p.Low {
			up := c.Low > p.High
			g := Pattern{Type: PatGapDown, Start: i, End: i, RefIdx: -1, FillIdx: -1, GapLo: c.High, GapHi: p.Low}
			if up {
				g.Type, g.GapLo, g.GapHi = PatGapUp, p.High, c.Low
			}
			g.Cat, g.Dir = patternMeta[g.Type].cat, patternMeta[g.Type].dir
			for j := i + 1; j < len(cs); j++ {
				if (up && cs[j].Low <= g.GapLo) || (!up && cs[j].High >= g.GapHi) {
					g.FillIdx = j
					break
				}
			}
			out = append(out, g)
		}
	}

	sort.SliceStable(out, func(a, b int) bool {
		if out[a].End != out[b].End {
			return out[a].End < out[b].End
		}
		return out[a].Start < out[b].Start
	})
	for k := range out {
		if e := out[k].End; e+5 < len(cs) && cs[e].Close != 0 {
			f := (cs[e+5].Close/cs[e].Close - 1) * 100
			out[k].Fwd5Pct = &f
		}
	}
	return out
}
