package web

import (
	"net/http"
	"time"

	"argus/internal/service"
)

// fxLookbackDays bounds how stale a rate the trend may borrow for a day that
// has none of its own (see db.GetFXRateOnOrBefore).
const fxLookbackDays = 45

// lookbackFX is the trend's exception to "a missing rate degrades the whole
// metric to —": fx_rates only gains a row on a day someone opened a wealth
// page, so a strict per-day lookup would leave most of a month's points empty
// for anyone holding a foreign-currency asset or US stocks. A point priced
// with a rate a few days old is still an honest point on a trend line; the
// same borrowed rate on a single headline figure (YTD, MoM) would not be, so
// those keep the exact-date rule.
type lookbackFX struct{ fxRateStore }

func (l lookbackFX) GetFXRate(date, pair string) (float64, bool, error) {
	if r, ok, err := l.fxRateStore.GetFXRate(date, pair); err != nil || ok {
		return r, ok, err
	}
	return l.fxRateStore.GetFXRateOnOrBefore(date, pair, fxLookbackDays)
}

type trendPoint struct {
	Date     string   `json:"date"`
	NetWorth *float64 `json:"netWorth"` // nil = no usable figure that day, never 0
}

type trendResponse struct {
	Period string       `json:"period"`
	Points []trendPoint `json:"points"`
}

const dateLayout = "2006-01-02"

// trendDates returns the sample dates for a period, oldest first and ending
// today. Unknown periods fall back to "ytd" (the page's default), the same
// way handleWealthHome treats an unknown model.
func trendDates(period string, now time.Time) (string, []string) {
	y, m, d := now.Date()
	at := func(yy int, mm time.Month, dd int) time.Time {
		return time.Date(yy, mm, dd, 0, 0, 0, 0, now.Location())
	}
	monthEnd := func(yy int, mm time.Month) time.Time { return at(yy, mm+1, 1).AddDate(0, 0, -1) }
	today := at(y, m, d)

	var out []time.Time
	switch period {
	case "mtd":
		// the last day of last month is the baseline the month is measured from
		out = append(out, monthEnd(y, m-1))
		for day := 1; day <= d; day++ {
			out = append(out, at(y, m, day))
		}
	case "3m":
		for i := 13; i >= 0; i-- {
			out = append(out, today.AddDate(0, 0, -7*i))
		}
	case "1y":
		start := today.AddDate(-1, 0, 0)
		out = append(out, start)
		for mm := start.Month(); ; mm++ {
			e := monthEnd(start.Year(), mm)
			if !e.Before(at(y, m, 1)) {
				break
			}
			out = append(out, e)
		}
		out = append(out, today)
	default:
		period = "ytd"
		out = append(out, at(y, 1, 1))
		for mm := time.January; mm < m; mm++ {
			out = append(out, monthEnd(y, mm))
		}
		out = append(out, today)
	}

	dates := make([]string, 0, len(out))
	for _, t := range out {
		s := t.Format(dateLayout)
		if len(dates) == 0 || s > dates[len(dates)-1] {
			dates = append(dates, s)
		}
	}
	return period, dates
}

// handleWealthTrend backs GET /api/wealth/networth/trend?period=mtd|3m|ytd|1y
// — the 淨值總覽 page's line chart. Each point is service.WealthTotals as of
// that day, so it moves in steps: a hand-kept asset only changes on the day
// its value was recorded. Today's point is priced live, to match the page's
// headline figure exactly.
func (s *Server) handleWealthTrend(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	period, dates := trendDates(r.URL.Query().Get("period"), now)
	today := now.Format(dateLayout)

	resp := trendResponse{Period: period, Points: make([]trendPoint, 0, len(dates))}
	past := lookbackFX{s.fxDB}
	for _, date := range dates {
		pt := trendPoint{Date: date}
		var total float64
		var ok bool
		if date == today {
			total, _, ok = service.WealthTotals(s.db, s.fxDB, s.quotes, date, true)
		} else {
			total, _, ok = service.WealthTotals(s.db, past, s.quotes, date, false)
		}
		// A day with nothing recorded yet totals exactly 0; drawing that would
		// be a cliff from before the account existed, not a net worth of 0.
		if ok && total != 0 {
			nw := total
			pt.NetWorth = &nw
		}
		resp.Points = append(resp.Points, pt)
	}
	writeJSON(w, http.StatusOK, resp)
}
