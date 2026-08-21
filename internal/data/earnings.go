package data

import (
	"fmt"
	"sort"
	"time"

	"argus/internal/market"
)

// EarningsEvent is a single scheduled earnings report date, from Finnhub's
// earnings calendar.
type EarningsEvent struct {
	Ticker    string
	Date      string // YYYY-MM-DD
	Hour      string // "bmo" (before open), "amc" (after close), "dmh" (during hours), or "" if unspecified
	Estimated bool   // true for GetTWUpcomingEarnings' statutory-deadline proxy — see its own doc comment
}

// EarningsProvider is implemented only by Finnhub — same reasoning as
// FundamentalsProvider (fundamentals.go): no Yahoo equivalent we're willing
// to depend on.
type EarningsProvider interface {
	// GetUpcomingEarnings returns the next scheduled earnings date within
	// the next `days` days for each ticker in tickers that has one, keyed
	// by ticker. A ticker with nothing scheduled in that window is simply
	// absent from the result rather than an error.
	GetUpcomingEarnings(tickers []string, days int) (map[string]EarningsEvent, error)
	// GetEarningsInRange is GetUpcomingEarnings generalized to an arbitrary
	// [from, to] range — backs the web dashboard's Calendar view (see its own
	// doc comment below).
	GetEarningsInRange(tickers []string, from, to time.Time) ([]EarningsEvent, error)
}

type finnhubEarningsEvent struct {
	Date   string `json:"date"`
	Symbol string `json:"symbol"`
	Hour   string `json:"hour"`
}

// GetUpcomingEarnings fetches Finnhub's whole-market earnings calendar for
// [today, today+days] in a single request and filters client-side — the
// endpoint's `symbol` query param only accepts one ticker at a time, so a
// per-ticker loop would cost one request each, while the unfiltered range
// query costs exactly one regardless of how many tickers we care about.
func (f *Finnhub) GetUpcomingEarnings(tickers []string, days int) (map[string]EarningsEvent, error) {
	if len(tickers) == 0 {
		return nil, nil
	}
	from := time.Now()
	to := from.AddDate(0, 0, days)
	events, err := f.GetEarningsInRange(tickers, from, to)
	if err != nil {
		return nil, err
	}
	out := make(map[string]EarningsEvent, len(events))
	for _, e := range events {
		if existing, ok := out[e.Ticker]; ok && existing.Date <= e.Date {
			continue
		}
		out[e.Ticker] = e
	}
	return out, nil
}

// GetEarningsInRange is GetUpcomingEarnings generalized to an arbitrary
// [from, to] range (Finnhub's underlying endpoint already accepts one) and
// returning every matching event rather than deduping to one-per-ticker —
// the web dashboard's Calendar view needs this for browsing a past or future
// month directly, not just "the next N days from now".
func (f *Finnhub) GetEarningsInRange(tickers []string, from, to time.Time) ([]EarningsEvent, error) {
	if len(tickers) == 0 {
		return nil, nil
	}
	want := make(map[string]bool, len(tickers))
	for _, t := range tickers {
		want[t] = true
	}

	var result struct {
		EarningsCalendar []finnhubEarningsEvent `json:"earningsCalendar"`
	}
	path := fmt.Sprintf("/calendar/earnings?from=%s&to=%s", from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err := f.get(path, &result); err != nil {
		return nil, err
	}
	var out []EarningsEvent
	for _, e := range result.EarningsCalendar {
		if want[e.Symbol] {
			out = append(out, EarningsEvent{Ticker: e.Symbol, Date: e.Date, Hour: e.Hour})
		}
	}
	return out, nil
}

// EarningsSurprise is one past quarter's actual-vs-estimate EPS (Phase 23
// PR8, docs/phase-23-strategy-data-uplift.md §5) — briefing material only
// (§4.2, same as valuation percentile/cash-flow quality), never a ranking
// factor. SurprisePct is Finnhub's own surprisePercent, not
// (Actual-Estimate)/Estimate recomputed here — Finnhub's own figure already
// handles a near-zero or negative Estimate sanely, which a naive
// recomputation wouldn't.
type EarningsSurprise struct {
	Period      string // fiscal quarter end, "2026-06-30"
	Actual      float64
	Estimate    float64
	SurprisePct float64
}

// EarningsSurpriseProvider is implemented only by Finnhub, US-only
// (live-verified 2026-08-20: Finnhub's free tier caps /stock/earnings at the
// trailing 4 quarters regardless of a from/to range — unlike the Yahoo
// fundamentals-timeseries case this project already rejected for being too
// shallow for a percentile, §3.1, a 4-quarter beat/miss streak is exactly
// the standard shape this signal is used in, not a crippled version of
// something that needed to be longer).
type EarningsSurpriseProvider interface {
	GetEarningsSurprises(ticker string) ([]EarningsSurprise, error)
}

type finnhubEarningsSurprise struct {
	Period          string  `json:"period"`
	Actual          float64 `json:"actual"`
	Estimate        float64 `json:"estimate"`
	SurprisePercent float64 `json:"surprisePercent"`
}

// GetEarningsSurprises fetches Finnhub's /stock/earnings, one ticker per
// call (like GetAnalystRating/GetFundamentals) — only called for a bounded
// ticker set to stay under the free-tier rate limit. Returned oldest-first
// (Finnhub documents newest-first, but this project sorts defensively
// rather than trust response ordering, same reasoning as GetAnalystRating).
func (f *Finnhub) GetEarningsSurprises(ticker string) ([]EarningsSurprise, error) {
	if market.Of(ticker) == market.TW {
		return nil, errTWNotSupported
	}
	var result []finnhubEarningsSurprise
	if err := f.get(fmt.Sprintf("/stock/earnings?symbol=%s", ticker), &result); err != nil {
		return nil, err
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Period < result[j].Period })
	out := make([]EarningsSurprise, len(result))
	for i, r := range result {
		out[i] = EarningsSurprise{Period: r.Period, Actual: r.Actual, Estimate: r.Estimate, SurprisePct: r.SurprisePercent}
	}
	return out, nil
}

// filterEarningsCalendar narrows Finnhub's whole-market earnings calendar
// response down to just the tickers we asked about, keyed by ticker. If a
// ticker somehow appears more than once (shouldn't normally happen for a
// single date range), the earliest date wins.
func filterEarningsCalendar(tickers []string, events []finnhubEarningsEvent) map[string]EarningsEvent {
	if len(tickers) == 0 {
		return nil
	}
	want := make(map[string]bool, len(tickers))
	for _, t := range tickers {
		want[t] = true
	}

	out := make(map[string]EarningsEvent)
	for _, e := range events {
		if !want[e.Symbol] {
			continue
		}
		if existing, ok := out[e.Symbol]; ok && existing.Date <= e.Date {
			continue
		}
		out[e.Symbol] = EarningsEvent{Ticker: e.Symbol, Date: e.Date, Hour: e.Hour}
	}
	return out
}
