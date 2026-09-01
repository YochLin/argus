package data

import (
	"sort"
	"time"

	"argus/internal/market"
)

// twStatutoryDeadline is one fixed month/day in Taiwan's securities-law
// financial-disclosure calendar (docs/... TW data-gap investigation,
// 2026-07-28: FinMind has no earnings-calendar dataset at all, and a
// per-company date would require scraping MOPS — out of scope for now).
// Every TW-listed company shares the same deadline for the same period, so
// this is a filing-law due date, not a per-company announcement — see
// GetTWUpcomingEarnings.
type twStatutoryDeadline struct{ month, day int }

// twQuarterlyDeadlines is Taiwan's statutory filing deadlines for the
// annual report (prior year, due 3/31) and Q1/Q2/Q3 (5/15, 8/14, 11/14).
// Monthly revenue (due the 10th of every month) recurs too often to fit
// this once-a-year shape and is generated separately in
// nextTWDisclosureDeadline.
var twQuarterlyDeadlines = []twStatutoryDeadline{
	{3, 31}, {5, 15}, {8, 14}, {11, 14},
}

// GetTWUpcomingEarnings fills EarningsProvider's TW gap with, per ticker,
// the soonest of: a real announced 法說會 date scraped from Yahoo TW (see
// tw_earnings_call.go), or — when no real one is scraped for that ticker
// within [now, now+days] — the next statutory financial-disclosure deadline
// (quarterly/annual report or monthly revenue), which is the same date for
// every TW ticker since it's a filing-law date rather than a company-
// specific announcement. EarningsEvent.Estimated distinguishes the two so
// callers can word a deadline as such rather than implying a confirmed
// per-company date (see writeStockSection's KeyEarningsLineEstimated).
// Non-TW tickers in the input are ignored.
func GetTWUpcomingEarnings(tickers []string, days int, now time.Time) map[string]EarningsEvent {
	var twTickers []string
	for _, t := range tickers {
		if market.Of(t) == market.TW {
			twTickers = append(twTickers, t)
		}
	}
	if len(twTickers) == 0 {
		return nil
	}

	windowEnd := now.AddDate(0, 0, days)
	real := cachedYahooEarningsCalls()
	out := make(map[string]EarningsEvent, len(twTickers))
	var remaining []string
	for _, t := range twTickers {
		if ev, ok := earliestInRange(real[t], now, windowEnd); ok {
			out[t] = ev
			continue
		}
		remaining = append(remaining, t)
	}
	if len(remaining) == 0 {
		return out
	}

	deadline, ok := nextTWDisclosureDeadline(now, days)
	if !ok {
		return out
	}
	dateStr := deadline.Format("2006-01-02")
	for _, t := range remaining {
		out[t] = EarningsEvent{Ticker: t, Date: dateStr, Estimated: true}
	}
	return out
}

// GetTWEarningsInRange is GetTWUpcomingEarnings generalized to an arbitrary
// [from, to] range — the web dashboard's Calendar view needs this to browse
// a past or future month directly. Every real event scraped for a ticker
// within range is included; the statutory-deadline proxy fills in for a
// ticker only when it has no real event in that same range (a single month
// can otherwise still contain more than one deadline, e.g. the 8/14 Q2
// deadline and the 8/10 monthly-revenue deadline both falling in August).
func GetTWEarningsInRange(tickers []string, from, to time.Time) []EarningsEvent {
	var twTickers []string
	for _, t := range tickers {
		if market.Of(t) == market.TW {
			twTickers = append(twTickers, t)
		}
	}
	if len(twTickers) == 0 {
		return nil
	}

	real := cachedYahooEarningsCalls()
	var out []EarningsEvent
	haveReal := make(map[string]bool, len(twTickers))
	for _, t := range twTickers {
		for _, ev := range real[t] {
			d, err := time.Parse("2006-01-02", ev.Date)
			if err != nil || d.Before(from) || d.After(to) {
				continue
			}
			out = append(out, ev)
			haveReal[t] = true
		}
	}

	deadlines := twDisclosureDeadlinesInRange(from, to)
	for _, d := range deadlines {
		dateStr := d.Format("2006-01-02")
		for _, t := range twTickers {
			if haveReal[t] {
				continue
			}
			out = append(out, EarningsEvent{Ticker: t, Date: dateStr, Estimated: true})
		}
	}
	return out
}

// nextTWDisclosureDeadline finds the soonest of the fixed quarterly/annual
// deadlines and the recurring monthly-revenue deadline (10th of every
// month) that falls within [now, now+days]. now is truncated to its own
// date at midnight first — same date-only comparison bot.daysUntil already
// uses — so a deadline falling on today itself isn't excluded just because
// "now" carries a later time-of-day.
func nextTWDisclosureDeadline(now time.Time, days int) (time.Time, bool) {
	now = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	deadlines := twDisclosureDeadlinesInRange(now, now.AddDate(0, 0, days))
	if len(deadlines) == 0 {
		return time.Time{}, false
	}
	return deadlines[0], true
}

// twDisclosureDeadlinesInRange returns every statutory deadline (quarterly/
// annual report plus monthly revenue) falling within [from, to], sorted
// ascending. Scans this year and next so a deadline early in the following
// January is still found from a December `from`, and 15 months of
// monthly-revenue dates so an unusually wide range still finds every one.
func twDisclosureDeadlinesInRange(from, to time.Time) []time.Time {
	var candidates []time.Time
	for _, y := range []int{from.Year(), from.Year() + 1} {
		for _, d := range twQuarterlyDeadlines {
			candidates = append(candidates, time.Date(y, time.Month(d.month), d.day, 0, 0, 0, 0, from.Location()))
		}
	}
	cur := time.Date(from.Year(), from.Month(), 10, 0, 0, 0, 0, from.Location())
	for i := 0; i < 15; i++ {
		candidates = append(candidates, cur)
		cur = cur.AddDate(0, 1, 0)
	}

	var out []time.Time
	for _, c := range candidates {
		if c.Before(from) || c.After(to) {
			continue
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out
}
