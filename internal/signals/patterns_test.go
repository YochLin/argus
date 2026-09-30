package signals

import (
	"testing"
	"time"

	"argus/internal/data"
)

// patBase is 30 alternating green/red bars (body 1, range 2, volume 1000, closes
// 101/100/...), so every prior-20 average is body 1 / range 2 / volume 1000 and
// none of the 21 patterns fires on it. Tests append the bars under test after
// index 29 (a red bar closing at 100).
func patBase() []data.Candle {
	var cs []data.Candle
	for k := 0; k < 30; k++ {
		o, c := 100.0, 101.0
		if k%2 == 1 {
			o, c = 101, 100
		}
		cs = append(cs, data.Candle{Open: o, High: 101.5, Low: 99.5, Close: c, Volume: 1000})
	}
	return withDates(cs)
}

func withDates(cs []data.Candle) []data.Candle {
	d := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range cs {
		cs[i].Date = d.AddDate(0, 0, i)
	}
	return cs
}

func bar(o, h, l, c float64, v int64) data.Candle {
	return data.Candle{Open: o, High: h, Low: l, Close: c, Volume: v}
}

// slide is n small bars stepping the close by step each, to set the prior-5-day
// direction ahead of a reversal pattern (red bars when falling, green when rising).
func slide(from float64, step float64, n int) []data.Candle {
	var cs []data.Candle
	for k := 0; k < n; k++ {
		o, c := from+float64(k)*step, from+float64(k+1)*step
		lo, hi := o, c
		if lo > hi {
			lo, hi = hi, lo
		}
		cs = append(cs, bar(o, hi+0.2, lo-0.2, c, 1000))
	}
	return cs
}

func find(ps []Pattern, t PatternType, end int) *Pattern {
	for i := range ps {
		if ps[i].Type == t && ps[i].End == end {
			return &ps[i]
		}
	}
	return nil
}

func TestDetectPatterns_BaseIsQuiet(t *testing.T) {
	if got := DetectPatterns(patBase()); len(got) != 0 {
		t.Errorf("DetectPatterns(base) = %+v, want none", got)
	}
	if got := DetectPatterns(nil); len(got) != 0 {
		t.Errorf("DetectPatterns(nil) = %+v, want none", got)
	}
}

func TestDetectPatterns_Each(t *testing.T) {
	fall := slide(99, -0.5, 5) // bars 30..34, closes 98.5 .. 96.5
	rise := slide(101, 0.5, 5) // bars 30..34, closes 101.5 .. 103.5
	tests := []struct {
		name string
		add  []data.Candle // appended after patBase
		want PatternType
		end  int // index of the completing bar
	}{
		{"bullEngulf", []data.Candle{bar(100, 100.2, 98.6, 98.8, 1000), bar(98.5, 101.2, 98.3, 101, 2000)}, PatBullEngulf, 31},
		{"bearEngulf", []data.Candle{bar(100, 101.8, 99.8, 101.5, 1000), bar(101.8, 102, 98.8, 99.2, 2000)}, PatBearEngulf, 31},
		{"piercing", []data.Candle{bar(104, 104.2, 99.8, 100, 1000), bar(99.5, 103, 99.3, 102.5, 1000)}, PatPiercing, 31},
		{"darkCloud", []data.Candle{bar(100, 104.2, 99.8, 104, 1000), bar(104.5, 104.7, 101.5, 101.8, 1000)}, PatDarkCloud, 31},
		{"bullHarami", []data.Candle{bar(104, 104.2, 99.8, 100, 1000), bar(101, 102.4, 100.8, 102, 1000)}, PatBullHarami, 31},
		{"bearHarami", []data.Candle{bar(100, 104.2, 99.8, 104, 1000), bar(103, 103.4, 101.6, 102, 1000)}, PatBearHarami, 31},
		{"hammer", append(append([]data.Candle{}, fall...), bar(97.2, 97.7, 95.6, 97.6, 1000)), PatHammer, 35},
		{"hangingMan", append(append([]data.Candle{}, rise...), bar(103.4, 103.9, 101.8, 103.8, 1000)), PatHangingMan, 35},
		{"shooting", append(append([]data.Candle{}, rise...), bar(103.4, 105.1, 102.9, 103, 1000)), PatShooting, 35},
		{"invHammer", append(append([]data.Candle{}, fall...), bar(97, 98.7, 96.8, 96.6, 1000)), PatInvHammer, 35},
		{"morning", []data.Candle{bar(100, 100.2, 96.8, 97, 1000), bar(96.8, 97.2, 96.6, 97, 1000), bar(97.2, 99.2, 97, 99, 1000)}, PatMorning, 32},
		{"evening", []data.Candle{bar(97, 100.2, 96.8, 100, 1000), bar(100, 100.4, 99.8, 100.2, 1000), bar(99.8, 100, 97.6, 98, 1000)}, PatEvening, 32},
		{"soldiers", []data.Candle{bar(100, 101.7, 99.9, 101.5, 1000), bar(101, 102.7, 100.8, 102.5, 1000), bar(102, 103.7, 101.8, 103.5, 1000)}, PatSoldiers, 32},
		{"crows", []data.Candle{bar(101, 101.1, 99.3, 99.5, 1000), bar(100, 100.1, 98.3, 98.5, 1000), bar(99, 99.1, 97.3, 97.5, 1000)}, PatCrows, 31}, // bar 29 (red, close 100) already starts the run
		{"doji", []data.Candle{bar(100, 102, 98, 100.05, 1000)}, PatDoji, 30},
		{"volBreak", []data.Candle{bar(101, 103.5, 100.9, 103, 2500)}, PatVolBreak, 30},
		{"volDump", []data.Candle{bar(99.5, 99.6, 97, 97.2, 2500)}, PatVolDump, 30},
		{"volDry", []data.Candle{bar(100, 101, 100, 100.5, 300)}, PatVolDry, 30},
		{"gapUp", []data.Candle{bar(103, 104, 102.5, 103.5, 1000)}, PatGapUp, 30},
		{"gapDown", []data.Candle{bar(98, 98.5, 97, 97.5, 1000)}, PatGapDown, 30},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cs := withDates(append(patBase(), tc.add...))
			p := find(DetectPatterns(cs), tc.want, tc.end)
			if p == nil {
				t.Fatalf("no %s ending at %d in %+v", tc.want, tc.end, DetectPatterns(cs))
			}
			if p.Cat != patternMeta[tc.want].cat || p.Dir != patternMeta[tc.want].dir {
				t.Errorf("%s: cat/dir = %s/%s, want %v", tc.want, p.Cat, p.Dir, patternMeta[tc.want])
			}
		})
	}
}

func TestDetectPatterns_Divergence(t *testing.T) {
	cs := patBase()
	cs[25].High, cs[25].Volume = 102.5, 1600 // the prior 20-day high, on more volume
	cs = withDates(append(cs, bar(101, 103.2, 100.9, 103, 1200)))
	p := find(DetectPatterns(cs), PatVolDiverge, 30)
	if p == nil {
		t.Fatalf("no divergence in %+v", DetectPatterns(cs))
	}
	if p.RefIdx != 25 || p.Extra < 0.74 || p.Extra > 0.76 {
		t.Errorf("RefIdx/Extra = %d/%v, want 25/0.75", p.RefIdx, p.Extra)
	}
}

func TestDetectPatterns_Confidence(t *testing.T) {
	// A bullish engulfing after a decline, on big volume with a big body: 3 of 3.
	cs := withDates(append(patBase(), slide(100, -1, 5)...))
	cs = append(cs, bar(95.5, 95.7, 93.9, 94.3, 1000), bar(94, 97, 93.8, 96.5, 2500))
	cs = withDates(cs)
	p := find(DetectPatterns(cs), PatBullEngulf, 36)
	if p == nil {
		t.Fatalf("no engulfing in %+v", DetectPatterns(cs))
	}
	if p.Conf != PatConfHigh || p.Prior5Pct >= 0 {
		t.Errorf("Conf/Prior5Pct = %s/%v, want high and a negative prior move", p.Conf, p.Prior5Pct)
	}

	// The same shape out of a flat market: context doesn't fit, so 2 of 3.
	flat := withDates(append(patBase(), bar(100, 100.2, 98.6, 98.8, 1000), bar(98.5, 101.2, 98.3, 101, 2000)))
	if p := find(DetectPatterns(flat), PatBullEngulf, 31); p == nil || p.Conf != PatConfMid {
		t.Errorf("flat-market engulfing = %+v, want conf mid", p)
	}

	// A volume breakout is not a reversal, so context always scores: big volume alone reaches mid.
	brk := withDates(append(patBase(), bar(101, 103.5, 100.9, 102.9, 2500)))
	if p := find(DetectPatterns(brk), PatVolBreak, 30); p == nil || p.Conf == PatConfLow {
		t.Errorf("volume breakout = %+v, want at least mid", p)
	}
}

func TestDetectPatterns_RunsFireOnce(t *testing.T) {
	// Five green steps up is three overlapping soldiers windows; only the first counts.
	add := []data.Candle{
		bar(100, 101.7, 99.9, 101.5, 1000), bar(101, 102.7, 100.8, 102.5, 1000), bar(102, 103.7, 101.8, 103.5, 1000),
		bar(103, 104.7, 102.8, 104.5, 1000), bar(104, 105.7, 103.8, 105.5, 1000),
	}
	n := 0
	for _, p := range DetectPatterns(withDates(append(patBase(), add...))) {
		if p.Type == PatSoldiers {
			n++
		}
	}
	if n != 1 {
		t.Errorf("soldiers fired %d times over a 5-bar run, want 1", n)
	}

	// Volume dry-up has a 3-bar cooldown.
	dry := []data.Candle{bar(100, 101, 100, 100.5, 300), bar(100.5, 101, 100, 100.4, 300), bar(100.4, 101, 100, 100.6, 300)}
	n = 0
	for _, p := range DetectPatterns(withDates(append(patBase(), dry...))) {
		if p.Type == PatVolDry {
			n++
		}
	}
	if n != 1 {
		t.Errorf("volDry fired %d times in 3 bars, want 1", n)
	}
}

func TestDetectPatterns_GapFill(t *testing.T) {
	up := withDates(append(patBase(),
		bar(103, 104, 102.5, 103.5, 1000),   // gap up over 101.5..102.5
		bar(103.5, 104, 102.6, 103, 1000),   // stays above
		bar(103, 103.2, 101.4, 101.6, 1000), // trades back to 101.4 <= 101.5: filled
	))
	g := find(DetectPatterns(up), PatGapUp, 30)
	if g == nil || g.GapLo != 101.5 || g.GapHi != 102.5 || g.FillIdx != 32 || g.Conf != "" {
		t.Errorf("filled gap up = %+v, want band 101.5..102.5 filled at 32, no conf", g)
	}

	open := withDates(append(patBase(), bar(98, 98.5, 97, 97.5, 1000), bar(97.5, 98.4, 97, 98, 1000)))
	g = find(DetectPatterns(open), PatGapDown, 30)
	if g == nil || g.GapLo != 98.5 || g.GapHi != 99.5 || g.FillIdx != -1 {
		t.Errorf("open gap down = %+v, want band 98.5..99.5 still open", g)
	}
}

func TestDetectPatterns_Fwd5AndOrder(t *testing.T) {
	cs := patBase()
	cs = append(cs, bar(103, 104, 102.5, 103.5, 1000)) // gap up at 30, close 103.5
	for k := 0; k < 6; k++ {
		cs = append(cs, bar(103.5, 104, 103, 103.5+float64(k), 1000))
	}
	ps := DetectPatterns(withDates(cs))
	g := find(ps, PatGapUp, 30)
	if g == nil || g.Fwd5Pct == nil {
		t.Fatalf("gap = %+v, want a 5-bar forward return", g)
	}
	want := (cs[35].Close/cs[30].Close - 1) * 100
	if *g.Fwd5Pct != want {
		t.Errorf("Fwd5Pct = %v, want %v", *g.Fwd5Pct, want)
	}
	for i := 1; i < len(ps); i++ {
		if ps[i].End < ps[i-1].End {
			t.Errorf("patterns not ordered by End: %+v", ps)
			break
		}
	}
	if last := DetectPatterns(withDates(append(patBase(), bar(103, 104, 102.5, 103.5, 1000)))); last[0].Fwd5Pct != nil {
		t.Errorf("Fwd5Pct = %v for a bar with no bar 5 ahead, want nil", *last[0].Fwd5Pct)
	}
}

func TestDetectPatterns_SkipsFlatHistory(t *testing.T) {
	// 25 zero-volume bars: no baseline to be "larger than", so no volume/candle
	// patterns — but a gap needs only the previous bar.
	var cs []data.Candle
	for k := 0; k < 25; k++ {
		cs = append(cs, bar(100, 100, 100, 100, 0))
	}
	cs = append(cs, bar(105, 106, 104, 105, 5000))
	ps := DetectPatterns(withDates(cs))
	if len(ps) != 1 || ps[0].Type != PatGapUp {
		t.Errorf("patterns on a flat zero-volume history = %+v, want only the gap", ps)
	}
}
