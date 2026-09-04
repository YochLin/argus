package signals

import (
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/i18n"
)

// mtfCandles builds count consecutive WEEKDAY bars (weekends skipped, so the
// ISO-week grouping weeklyCloses does is exercised for real) whose closes
// come from closeAt.
func mtfCandles(count int, closeAt func(i int) float64) []data.Candle {
	out := make([]data.Candle, 0, count)
	d := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC) // a Monday
	for i := 0; len(out) < count; i++ {
		if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
			c := closeAt(len(out))
			out = append(out, data.Candle{
				Date: d, Open: c, High: c + 1, Low: c - 1, Close: c, Volume: 1_000_000,
			})
		}
		d = d.AddDate(0, 0, 1)
	}
	return out
}

// tw2059Closes is 川湖 (2059) daily closes for the 170 sessions ending
// 2025-10-13 — a real trigger of the shipped rule (daily angles 42.0/36.0,
// weekly 36.2/37.7, RSI 65.5, so every leg clears its threshold with room).
// Pinning the implementation to a real firing day is worth more than a
// synthetic fixture here: BOTH bugs this screen went through (weekly
// lookahead, and comparing weekly MAs day-to-day instead of bar-to-bar)
// produced code that still passed hand-built V-shapes while being wrong on
// real data.
var tw2059Closes = []struct {
	date  string
	close float64
}{
	{"2025-02-05", 1410}, {"2025-02-06", 1425}, {"2025-02-07", 1565}, {"2025-02-10", 1570},
	{"2025-02-11", 1620}, {"2025-02-12", 1655}, {"2025-02-13", 1670}, {"2025-02-14", 1675},
	{"2025-02-17", 1755}, {"2025-02-18", 1855}, {"2025-02-19", 1870}, {"2025-02-20", 1875},
	{"2025-02-21", 1865}, {"2025-02-24", 1930}, {"2025-02-25", 1955}, {"2025-02-26", 2050},
	{"2025-02-27", 1990}, {"2025-03-03", 2000}, {"2025-03-04", 2005}, {"2025-03-05", 1960},
	{"2025-03-06", 1970}, {"2025-03-07", 1925}, {"2025-03-10", 1900}, {"2025-03-11", 1870},
	{"2025-03-12", 1840}, {"2025-03-13", 1880}, {"2025-03-14", 1895}, {"2025-03-17", 1915},
	{"2025-03-18", 1980}, {"2025-03-19", 1925}, {"2025-03-20", 1965}, {"2025-03-21", 1865},
	{"2025-03-24", 1840}, {"2025-03-25", 1845}, {"2025-03-26", 1895}, {"2025-03-27", 1900},
	{"2025-03-28", 1825}, {"2025-03-31", 1650}, {"2025-04-01", 1665}, {"2025-04-02", 1640},
	{"2025-04-07", 1480}, {"2025-04-08", 1335}, {"2025-04-09", 1205}, {"2025-04-10", 1325},
	{"2025-04-11", 1455}, {"2025-04-14", 1585}, {"2025-04-15", 1640}, {"2025-04-16", 1570},
	{"2025-04-17", 1615}, {"2025-04-18", 1640}, {"2025-04-21", 1645}, {"2025-04-22", 1570},
	{"2025-04-23", 1670}, {"2025-04-24", 1675}, {"2025-04-25", 1795}, {"2025-04-28", 1770},
	{"2025-04-29", 1815}, {"2025-04-30", 1745}, {"2025-05-02", 1840}, {"2025-05-05", 1815},
	{"2025-05-06", 1855}, {"2025-05-07", 1865}, {"2025-05-08", 1920}, {"2025-05-09", 1890},
	{"2025-05-12", 1880}, {"2025-05-13", 1925}, {"2025-05-14", 1940}, {"2025-05-15", 2000},
	{"2025-05-16", 2020}, {"2025-05-19", 1965}, {"2025-05-20", 2010}, {"2025-05-21", 2050},
	{"2025-05-22", 2130}, {"2025-05-23", 2095}, {"2025-05-26", 2090}, {"2025-05-27", 2050},
	{"2025-05-28", 2060}, {"2025-05-29", 2050}, {"2025-06-02", 2030}, {"2025-06-03", 2160},
	{"2025-06-04", 2175}, {"2025-06-05", 2200}, {"2025-06-06", 2135}, {"2025-06-09", 2145},
	{"2025-06-10", 2180}, {"2025-06-11", 2140}, {"2025-06-12", 2130}, {"2025-06-13", 2165},
	{"2025-06-16", 2185}, {"2025-06-17", 2130}, {"2025-06-18", 2150}, {"2025-06-19", 2150},
	{"2025-06-20", 2075}, {"2025-06-23", 2100}, {"2025-06-24", 2145}, {"2025-06-25", 2135},
	{"2025-06-26", 2050}, {"2025-06-27", 2050}, {"2025-06-30", 2035}, {"2025-07-01", 2010},
	{"2025-07-02", 2015}, {"2025-07-03", 1990}, {"2025-07-04", 2050}, {"2025-07-07", 2025},
	{"2025-07-08", 2145}, {"2025-07-09", 2150}, {"2025-07-10", 2160}, {"2025-07-11", 2120},
	{"2025-07-14", 2055}, {"2025-07-15", 2130}, {"2025-07-16", 2185}, {"2025-07-17", 2240},
	{"2025-07-18", 2415}, {"2025-07-21", 2500}, {"2025-07-22", 2495}, {"2025-07-23", 2500},
	{"2025-07-24", 2520}, {"2025-07-25", 2565}, {"2025-07-28", 2605}, {"2025-07-29", 2595},
	{"2025-07-30", 2605}, {"2025-07-31", 2665}, {"2025-08-04", 2600}, {"2025-08-05", 2725},
	{"2025-08-06", 2810}, {"2025-08-07", 2925}, {"2025-08-08", 2960}, {"2025-08-11", 3005},
	{"2025-08-12", 2990}, {"2025-08-13", 3055}, {"2025-08-14", 3105}, {"2025-08-15", 3130},
	{"2025-08-18", 3180}, {"2025-08-19", 3085}, {"2025-08-20", 2855}, {"2025-08-21", 2965},
	{"2025-08-22", 2955}, {"2025-08-25", 3035}, {"2025-08-26", 2980}, {"2025-08-27", 2940},
	{"2025-08-28", 2925}, {"2025-08-29", 3025}, {"2025-09-01", 2965}, {"2025-09-02", 2930},
	{"2025-09-03", 3130}, {"2025-09-04", 2995}, {"2025-09-05", 3210}, {"2025-09-08", 3220},
	{"2025-09-09", 3335}, {"2025-09-10", 3485}, {"2025-09-11", 3415}, {"2025-09-12", 3430},
	{"2025-09-15", 3410}, {"2025-09-16", 3370}, {"2025-09-17", 3370}, {"2025-09-18", 3415},
	{"2025-09-19", 3335}, {"2025-09-22", 3250}, {"2025-09-23", 3220}, {"2025-09-24", 3220},
	{"2025-09-25", 3170}, {"2025-09-26", 3100}, {"2025-09-30", 3290}, {"2025-10-01", 3310},
	{"2025-10-02", 3340}, {"2025-10-03", 3415}, {"2025-10-07", 3470}, {"2025-10-08", 3575},
	{"2025-10-09", 3745}, {"2025-10-13", 3720},
}

func tw2059Candles(t *testing.T) []data.Candle {
	t.Helper()
	out := make([]data.Candle, len(tw2059Closes))
	for i, r := range tw2059Closes {
		d, err := time.Parse("2006-01-02", r.date)
		if err != nil {
			t.Fatalf("bad fixture date %q: %v", r.date, err)
		}
		out[i] = data.Candle{
			Date: d, Open: r.close, High: r.close, Low: r.close,
			Close: r.close, Volume: 1_000_000,
		}
	}
	return out
}

// TestMTFCrossRealTrigger pins the screen to a day the measurement run
// actually flagged. A drift here means the shipped screen no longer matches
// the numbers in CheckMTFCrossExact's doc comment.
func TestMTFCrossRealTrigger(t *testing.T) {
	candles := tw2059Candles(t)

	if !CheckMTFCrossExact(candles) {
		t.Error("2025-10-13 is a measured trigger for 2059 but the screen did not fire")
	}
	// The state has to be new: the prior bar must not already satisfy it,
	// or Detector's dedup would have alerted a day early.
	if CheckMTFCrossExact(candles[:len(candles)-1]) {
		t.Error("fired on 2025-10-10 too: this is not the day the state turns true")
	}
	if CheckMTFCrossExact(candles[:mtfMinBars-1]) {
		t.Error("should return false below mtfMinBars rather than indexing short slices")
	}

	// MTFCross's lookback should surface the same hit as DaysAgo=0.
	if hit := MTFCross(candles); hit == nil || hit.DaysAgo != 0 {
		t.Errorf("MTFCross() = %v, want a hit with DaysAgo=0", hit)
	}
}

// TestMTFCrossRSICap isolates the RSI leg. Changing only the LAST close
// cannot touch the weekly legs at all — weeklyCloses excludes the week the
// last bar sits in — and a jump only makes the daily angles steeper, so RSI
// is the one thing that can turn the screen off.
func TestMTFCrossRSICap(t *testing.T) {
	candles := tw2059Candles(t)
	candles[len(candles)-1].Close *= 1.30

	if got := RSI(data.Closes(candles), 14); got <= mtfRSIMax {
		t.Fatalf("RSI after the spike = %.1f, want > %.0f for this test to mean anything", got, mtfRSIMax)
	}
	if CheckMTFCrossExact(candles) {
		t.Error("fired with RSI above the cap; the overheat leg is not being enforced")
	}
}

// TestWeeklyClosesExcludesCurrentWeek pins the lookahead bug: the week the
// LAST bar sits in must not appear in the completed-week series, or a
// mid-week evaluation silently reads that week's future closes.
func TestWeeklyClosesExcludesCurrentWeek(t *testing.T) {
	// 8 weekdays = all of week 1 (Mon-Fri) plus Mon/Tue/Wed of week 2.
	c := mtfCandles(8, func(i int) float64 { return float64(i) })
	weekly := weeklyCloses(c)
	if len(weekly) != 1 {
		t.Fatalf("weeklyCloses() = %v, want exactly 1 completed week", weekly)
	}
	if weekly[0] != 4 {
		t.Errorf("completed week close = %v, want 4 (Friday's close, index 4)", weekly[0])
	}
}

// TestWMAComparesWeeklyBars pins the second bug: wma(back=0) vs wma(back=1)
// must differ by a whole weekly bar, not by one daily close. Two consecutive
// mid-week evaluations therefore see the SAME weekly MAs.
func TestWMAComparesWeeklyBars(t *testing.T) {
	c := mtfCandles(60, func(i int) float64 { return 100 + float64(i) })
	w := weeklyCloses(c)

	last := c[len(c)-1].Date
	if last.Weekday() == time.Monday {
		t.Skip("fixture ends on a Monday; this check needs two bars in the same week")
	}
	wPrevDay := weeklyCloses(c[:len(c)-1])
	if got, want := wma(w, 5, 0), wma(wPrevDay, 5, 0); got != want {
		t.Errorf("wMA5 changed mid-week: %v -> %v; it must only move when a week closes", want, got)
	}

	// And it must actually move once a week does close.
	if wma(w, 5, 0) == wma(w, 5, 1) {
		t.Error("wma(back=0) == wma(back=1) on a rising series; the two are not reading different bars")
	}
}

// TestMTFCrossDetectorBothMarkets pins the market policy: it USED to be
// TW-only, and now fires on US too at the user's explicit request even
// though US is measured negative (see CheckMTFCrossExact). If this ever goes
// back to returning nil for US, that is a policy change, not a fix.
func TestMTFCrossDetectorBothMarkets(t *testing.T) {
	candles := tw2059Candles(t)
	d := NewDetector(i18n.ZH)

	sig, state := d.CheckMTFCross("2059", candles, "")
	if sig == nil {
		t.Fatal("CheckMTFCross(TW) = nil, want a signal on a measured trigger day")
	}
	if sig.Type != TypeMTFCross {
		t.Errorf("Type = %q, want %q", sig.Type, TypeMTFCross)
	}
	if state != "hit" {
		t.Errorf("newState = %q, want \"hit\"", state)
	}
	// Dedup: the same occurrence must not alert twice.
	if sig, _ := d.CheckMTFCross("2059", candles, "hit"); sig != nil {
		t.Error("fired again with prevState=hit; dedup is not working")
	}

	if sig, _ := d.CheckMTFCross("AAPL", candles, ""); sig == nil {
		t.Error("CheckMTFCross(US) = nil; US is expected to fire (with a warning notice attached downstream)")
	}
}
