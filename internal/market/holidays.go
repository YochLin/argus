// Package market provides pure, dependency-free NYSE trading-calendar
// logic (currently just IsTradingDay) so callers like RunDailyReport can
// tell a genuine market holiday apart from an ordinary trading day without
// an external API call or a loaded timezone database — see IsTradingDay's
// doc comment for why that matters in this project's Docker/VPS setup.
package market

import "time"

// IsTradingDay reports whether t's calendar date (Year/Month/Day, read
// directly off t — see below for what zone t must already be in) is a
// normal NYSE trading day: a weekday that isn't a recognized market
// holiday.
//
// t is assumed to already carry the correct US Eastern calendar date, not
// converted from another zone here — loading a real America/New_York
// time.Location isn't safe in this project's deployment (see
// scheduler.New's own comment: the alpine Docker image has no tzdata
// package installed), and a fixed UTC offset would be wrong across the
// DST boundary since US Eastern alternates between UTC-5 and UTC-4. The
// one caller today (RunDailyReport) sidesteps both problems by relying on
// its cron firing at a fixed 23:30 CST: Taiwan never observes DST, and at
// that specific hour Taiwan's UTC+8 date, UTC's date, and US Eastern's
// date (10:30 EST / 11:30 EDT into the same calendar day, never having
// crossed midnight) all agree — so time.Now().In(cst) already carries the
// right Y/M/D with no timezone database involved. A caller running at a
// different hour would need to actually resolve the US Eastern date first;
// this function does not do that for you.
func IsTradingDay(t time.Time) bool {
	if wd := t.Weekday(); wd == time.Saturday || wd == time.Sunday {
		return false
	}
	return !IsHoliday(t)
}

// IsHoliday reports whether t's calendar date is a recognized NYSE
// full-day market holiday, independent of weekday (a holiday landing on a
// weekend is already excluded by IsTradingDay's own weekend check; this
// function only answers the holiday question).
//
// Known limitation, accepted as-is: this only covers the fixed annual
// holiday set below. Ad-hoc NYSE closures (a national day of mourning, a
// weather emergency) aren't predictable by calculation and won't be
// caught — same "no API dependency" tradeoff the design deliberately
// chose over e.g. Finnhub's holiday endpoint, whose free-tier
// availability was never confirmed.
func IsHoliday(t time.Time) bool {
	for _, h := range holidaysForYear(t.Year()) {
		if sameDate(h, t) {
			return true
		}
	}
	return false
}

func sameDate(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// holidaysForYear returns the NYSE's recognized full-day market holidays
// for year, with the observed-date shift already applied where NYSE
// applies one: a holiday falling on Saturday is observed the preceding
// Friday, one on Sunday the following Monday — except New Year's Day
// specifically does NOT shift to the preceding Friday when Jan 1 falls on
// a Saturday. That Friday is the year's last trading day, and NYSE keeps
// it open for year-end settlement rather than closing it — verified
// against NYSE's actual 2005 and 2022 calendars, the two most recent years
// New Year's fell on a Saturday; both show Dec 31 trading normally.
func holidaysForYear(year int) []time.Time {
	var hs []time.Time

	if d, ok := observedFixed(year, time.January, 1, false); ok {
		hs = append(hs, d)
	}
	hs = append(hs, nthWeekday(year, time.January, time.Monday, 3))  // MLK Day
	hs = append(hs, nthWeekday(year, time.February, time.Monday, 3)) // Washington's Birthday
	hs = append(hs, easterSunday(year).AddDate(0, 0, -2))            // Good Friday
	hs = append(hs, lastWeekday(year, time.May, time.Monday))        // Memorial Day
	if year >= 2022 {                                                // NYSE added Juneteenth starting 2022
		if d, ok := observedFixed(year, time.June, 19, true); ok {
			hs = append(hs, d)
		}
	}
	if d, ok := observedFixed(year, time.July, 4, true); ok {
		hs = append(hs, d)
	}
	hs = append(hs, nthWeekday(year, time.September, time.Monday, 1))  // Labor Day
	hs = append(hs, nthWeekday(year, time.November, time.Thursday, 4)) // Thanksgiving
	if d, ok := observedFixed(year, time.December, 25, true); ok {
		hs = append(hs, d)
	}

	return hs
}

// observedFixed resolves a fixed-date holiday (New Year's/Juneteenth/July
// 4th/Christmas) to its NYSE-observed date. The Saturday->Friday shift only
// applies when allowSaturdayShift is true (see holidaysForYear's New
// Year's exception); ok is false only for the New Year's Saturday case,
// meaning that year has no observed New Year's holiday at all.
func observedFixed(year int, month time.Month, day int, allowSaturdayShift bool) (time.Time, bool) {
	d := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	switch d.Weekday() {
	case time.Saturday:
		if !allowSaturdayShift {
			return time.Time{}, false
		}
		return d.AddDate(0, 0, -1), true
	case time.Sunday:
		return d.AddDate(0, 0, 1), true
	default:
		return d, true
	}
}

// nthWeekday returns the date of the nth occurrence of weekday in
// year/month (n=1 for the first — e.g. nthWeekday(2026, January, Monday, 3)
// is the third Monday of January 2026).
func nthWeekday(year int, month time.Month, weekday time.Weekday, n int) time.Time {
	first := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	offset := (int(weekday) - int(first.Weekday()) + 7) % 7
	return first.AddDate(0, 0, offset+7*(n-1))
}

// lastWeekday returns the date of the last occurrence of weekday in
// year/month.
func lastWeekday(year int, month time.Month, weekday time.Weekday) time.Time {
	nextMonth := time.Date(year, month+1, 1, 0, 0, 0, 0, time.UTC)
	offset := (int(nextMonth.Weekday()) - int(weekday) + 7) % 7
	if offset == 0 {
		offset = 7
	}
	return nextMonth.AddDate(0, 0, -offset)
}

// easterSunday computes the date of Easter Sunday for year using the
// Anonymous Gregorian algorithm (Meeus/Jones/Butcher) — valid for any
// Gregorian-calendar year, not just a hand-tuned recent range.
func easterSunday(year int) time.Time {
	a := year % 19
	b := year / 100
	c := year % 100
	d := b / 4
	e := b % 4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i := c / 4
	k := c % 4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	month := (h + l - 7*m + 114) / 31
	day := (h+l-7*m+114)%31 + 1
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}
