package data

import (
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

// GetTWUpcomingEarnings is the free, zero-API proxy for EarningsProvider's
// TW gap: the next statutory financial-disclosure deadline (quarterly/
// annual report or monthly revenue) that falls within [now, now+days],
// same for every TW ticker in tickers since it's a filing-law date rather
// than a company-specific announcement. EarningsEvent.Estimated is always
// true on these results so callers can word it as a deadline rather than
// implying a confirmed per-company earnings date (see writeStockSection's
// KeyEarningsLineEstimated). Non-TW tickers in the input are ignored.
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

	deadline, ok := nextTWDisclosureDeadline(now, days)
	if !ok {
		return nil
	}

	out := make(map[string]EarningsEvent, len(twTickers))
	dateStr := deadline.Format("2006-01-02")
	for _, t := range twTickers {
		out[t] = EarningsEvent{Ticker: t, Date: dateStr, Estimated: true}
	}
	return out
}

// nextTWDisclosureDeadline finds the soonest of the fixed quarterly/annual
// deadlines and the recurring monthly-revenue deadline (10th of every
// month) that falls within [now, now+days]. Scans this year and next so a
// deadline early in the following January is still found from a December
// now, and 15 months of monthly-revenue dates so an unusually large days
// window still finds one. now is truncated to its own date at midnight
// first — same date-only comparison bot.daysUntil already uses — so a
// deadline falling on today itself isn't excluded just because "now"
// carries a later time-of-day.
func nextTWDisclosureDeadline(now time.Time, days int) (time.Time, bool) {
	now = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	from := now
	to := now.AddDate(0, 0, days)

	var candidates []time.Time
	for _, y := range []int{now.Year(), now.Year() + 1} {
		for _, d := range twQuarterlyDeadlines {
			candidates = append(candidates, time.Date(y, time.Month(d.month), d.day, 0, 0, 0, 0, now.Location()))
		}
	}
	cur := time.Date(now.Year(), now.Month(), 10, 0, 0, 0, 0, now.Location())
	for i := 0; i < 15; i++ {
		candidates = append(candidates, cur)
		cur = cur.AddDate(0, 1, 0)
	}

	var best time.Time
	found := false
	for _, c := range candidates {
		if c.Before(from) || c.After(to) {
			continue
		}
		if !found || c.Before(best) {
			best = c
			found = true
		}
	}
	return best, found
}
