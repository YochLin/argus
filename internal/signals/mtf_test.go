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

// tw1101Closes is 台泥 (1101) daily closes for the 75 sessions ending
// 2023-01-05 — a real trigger from the measurement run behind
// CheckMTFCrossExact's doc comment (TW, WeeklyB reading). Pinning the
// implementation to a real firing day is worth more than a synthetic
// fixture here: BOTH bugs this screen went through (weekly lookahead, and
// comparing weekly MAs day-to-day instead of bar-to-bar) produced code that
// still passed hand-built V-shapes while being wrong on real data.
var tw1101Closes = []struct {
	date  string
	close float64
}{
	{"2022-09-21", 38.1000}, {"2022-09-22", 37.5500}, {"2022-09-23", 37.0000}, {"2022-09-26", 35.8500},
	{"2022-09-27", 35.8000}, {"2022-09-28", 34.9000}, {"2022-09-29", 34.1500}, {"2022-09-30", 33.8000},
	{"2022-10-03", 33.6500}, {"2022-10-04", 33.8500}, {"2022-10-05", 34.1500}, {"2022-10-06", 34.3000},
	{"2022-10-07", 34.2500}, {"2022-10-11", 33.8000}, {"2022-10-12", 33.7500}, {"2022-10-13", 32.6500},
	{"2022-10-14", 32.4000}, {"2022-10-17", 31.7000}, {"2022-10-18", 31.4500}, {"2022-10-19", 31.6000},
	{"2022-10-20", 30.9000}, {"2022-10-21", 31.0000}, {"2022-10-24", 30.6000}, {"2022-10-25", 29.9000},
	{"2022-10-26", 30.3000}, {"2022-10-27", 30.8500}, {"2022-10-28", 30.5000}, {"2022-10-31", 30.2500},
	{"2022-11-01", 30.6500}, {"2022-11-02", 30.7500}, {"2022-11-03", 30.3000}, {"2022-11-04", 30.4500},
	{"2022-11-07", 31.0000}, {"2022-11-08", 31.6500}, {"2022-11-09", 31.9000}, {"2022-11-10", 31.7000},
	{"2022-11-11", 32.1500}, {"2022-11-14", 34.1500}, {"2022-11-15", 33.8500}, {"2022-11-16", 32.9000},
	{"2022-11-17", 34.2500}, {"2022-11-18", 33.3000}, {"2022-11-21", 32.8500}, {"2022-11-22", 32.6500},
	{"2022-11-23", 32.7000}, {"2022-11-24", 33.2000}, {"2022-11-25", 33.5500}, {"2022-11-28", 33.0000},
	{"2022-11-29", 33.5500}, {"2022-11-30", 33.9000}, {"2022-12-01", 34.0500}, {"2022-12-02", 33.6500},
	{"2022-12-05", 33.7000}, {"2022-12-06", 33.3000}, {"2022-12-07", 33.3500}, {"2022-12-08", 33.2000},
	{"2022-12-09", 33.7000}, {"2022-12-12", 33.8500}, {"2022-12-13", 33.7000}, {"2022-12-14", 33.5000},
	{"2022-12-15", 33.5500}, {"2022-12-16", 34.6000}, {"2022-12-19", 33.8500}, {"2022-12-20", 33.7000},
	{"2022-12-21", 34.0500}, {"2022-12-22", 34.4500}, {"2022-12-23", 34.0500}, {"2022-12-26", 34.4000},
	{"2022-12-27", 34.3000}, {"2022-12-28", 34.3000}, {"2022-12-29", 33.7000}, {"2022-12-30", 33.6500},
	{"2023-01-03", 33.5000}, {"2023-01-04", 33.5000}, {"2023-01-05", 34.4500},
}

func tw1101Candles(t *testing.T) []data.Candle {
	t.Helper()
	out := make([]data.Candle, len(tw1101Closes))
	for i, r := range tw1101Closes {
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
	candles := tw1101Candles(t)

	if !CheckMTFCrossExact(candles) {
		t.Error("2023-01-05 is a measured trigger for 1101 but the screen did not fire")
	}
	// "Just crossed" — the day before the same cross must not fire.
	if CheckMTFCrossExact(candles[:len(candles)-1]) {
		t.Error("fired on 2023-01-04 too: the prevClose <= prevMA30 leg is not enforcing a cross")
	}
	if CheckMTFCrossExact(candles[:mtfMinBars-1]) {
		t.Error("should return false below mtfMinBars rather than indexing short slices")
	}

	// MTFCross's lookback should surface the same hit as DaysAgo=0, and as
	// DaysAgo=1 when evaluated one session later.
	if hit := MTFCross(candles); hit == nil || hit.DaysAgo != 0 {
		t.Errorf("MTFCross() = %v, want a hit with DaysAgo=0", hit)
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

// TestMTFCrossDetectorTWOnly pins the market gate. The US exclusion is a
// measured one (negative past 1 SE in both splits at every threshold, see
// CheckMTFCrossExact) — if this ever starts firing on US tickers, the screen
// is actively working against the user there.
func TestMTFCrossDetectorTWOnly(t *testing.T) {
	candles := tw1101Candles(t)
	d := NewDetector(i18n.ZH)

	sig, state := d.CheckMTFCross("1101", candles, "")
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
	if sig, _ := d.CheckMTFCross("1101", candles, "hit"); sig != nil {
		t.Error("fired again with prevState=hit; dedup is not working")
	}

	if sig, _ := d.CheckMTFCross("AAPL", candles, ""); sig != nil {
		t.Errorf("CheckMTFCross(US) = %v, want nil — the screen is TW-only", sig)
	}
}
