package data

import (
	"testing"
	"time"
)

func TestNextTWDisclosureDeadline(t *testing.T) {
	mustDate := func(s string) time.Time {
		d, err := time.Parse("2006-01-02", s)
		if err != nil {
			t.Fatalf("parse %q: %v", s, err)
		}
		return d
	}

	cases := []struct {
		name string
		now  string
		days int
		want string // "" means not found
	}{
		{"monthly revenue deadline found before the later quarterly one", "2026-08-01", 20, "2026-08-10"},
		{"nothing in a window that lands between deadlines", "2026-06-15", 10, ""},
		{"today itself counts as within window", "2026-03-31", 5, "2026-03-31"},
		{"crosses into next year", "2026-12-20", 25, "2027-01-10"},
		{"nothing within a too-short window", "2026-06-11", 1, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := nextTWDisclosureDeadline(mustDate(c.now), c.days)
			if c.want == "" {
				if ok {
					t.Errorf("nextTWDisclosureDeadline(%s, %d) = %s, want not found", c.now, c.days, got.Format("2006-01-02"))
				}
				return
			}
			if !ok {
				t.Fatalf("nextTWDisclosureDeadline(%s, %d) not found, want %s", c.now, c.days, c.want)
			}
			if got.Format("2006-01-02") != c.want {
				t.Errorf("nextTWDisclosureDeadline(%s, %d) = %s, want %s", c.now, c.days, got.Format("2006-01-02"), c.want)
			}
		})
	}
}

// withYahooEarningsCallFetcher stubs the real-earnings-call fetch (see
// tw_earnings_call.go) for the duration of one test, so GetTWUpcomingEarnings/
// GetTWEarningsInRange stay hermetic instead of hitting the live Yahoo page —
// and resets the shared process-lifetime cache so tests don't leak into each
// other.
func withYahooEarningsCallFetcher(t *testing.T, fn func() (map[string][]EarningsEvent, error)) {
	t.Helper()
	oldFetcher, oldCache, oldAt := yahooEarningsCallFetcher, yahooEarningsCallCache, yahooEarningsCallAt
	yahooEarningsCallFetcher = fn
	yahooEarningsCallCache, yahooEarningsCallAt = nil, time.Time{}
	t.Cleanup(func() {
		yahooEarningsCallFetcher = oldFetcher
		yahooEarningsCallCache, yahooEarningsCallAt = oldCache, oldAt
	})
}

func noRealEarningsCalls() (map[string][]EarningsEvent, error) { return nil, nil }

func TestGetTWUpcomingEarnings(t *testing.T) {
	now, _ := time.Parse("2006-01-02", "2026-08-01")

	t.Run("only TW tickers get an entry", func(t *testing.T) {
		withYahooEarningsCallFetcher(t, noRealEarningsCalls)
		got := GetTWUpcomingEarnings([]string{"2330", "AAPL"}, 20, now)
		if _, ok := got["AAPL"]; ok {
			t.Error("GetTWUpcomingEarnings included AAPL, a US ticker")
		}
		e, ok := got["2330"]
		if !ok {
			t.Fatal("GetTWUpcomingEarnings missing 2330")
		}
		if !e.Estimated {
			t.Error("2330's EarningsEvent.Estimated = false, want true")
		}
		if e.Date != "2026-08-10" {
			t.Errorf("2330's Date = %q, want 2026-08-10 (monthly revenue deadline, soonest in window)", e.Date)
		}
	})

	t.Run("no TW tickers returns nil", func(t *testing.T) {
		withYahooEarningsCallFetcher(t, noRealEarningsCalls)
		if got := GetTWUpcomingEarnings([]string{"AAPL"}, 20, now); got != nil {
			t.Errorf("GetTWUpcomingEarnings(no TW tickers) = %+v, want nil", got)
		}
	})

	t.Run("a real scraped date wins over the statutory-deadline proxy", func(t *testing.T) {
		withYahooEarningsCallFetcher(t, func() (map[string][]EarningsEvent, error) {
			return map[string][]EarningsEvent{
				"2330": {{Ticker: "2330", Date: "2026-08-05"}},
			}, nil
		})
		got := GetTWUpcomingEarnings([]string{"2330", "2317"}, 20, now)

		e, ok := got["2330"]
		if !ok {
			t.Fatal("missing 2330")
		}
		if e.Estimated {
			t.Error("2330's Estimated = true, want false for a real scraped date")
		}
		if e.Date != "2026-08-05" {
			t.Errorf("2330's Date = %q, want 2026-08-05", e.Date)
		}

		if e2, ok := got["2317"]; !ok || !e2.Estimated || e2.Date != "2026-08-10" {
			t.Errorf("2317 = %+v, want the statutory-deadline proxy (2026-08-10, Estimated)", e2)
		}
	})
}

func TestGetTWEarningsInRange_RealDateSuppressesDeadlineForThatTicker(t *testing.T) {
	from, _ := time.Parse("2006-01-02", "2026-08-01")
	to, _ := time.Parse("2006-01-02", "2026-08-20")

	withYahooEarningsCallFetcher(t, func() (map[string][]EarningsEvent, error) {
		return map[string][]EarningsEvent{
			"2330": {{Ticker: "2330", Date: "2026-08-05"}},
		}, nil
	})
	got := GetTWEarningsInRange([]string{"2330", "2317"}, from, to)

	var have2330Real, have2330Deadline, have2317Deadline bool
	for _, e := range got {
		switch {
		case e.Ticker == "2330" && e.Date == "2026-08-05" && !e.Estimated:
			have2330Real = true
		case e.Ticker == "2330" && e.Estimated:
			have2330Deadline = true
		case e.Ticker == "2317" && e.Estimated:
			have2317Deadline = true
		}
	}
	if !have2330Real {
		t.Error("missing 2330's real scraped event")
	}
	if have2330Deadline {
		t.Error("2330 also got the statutory-deadline proxy despite having a real event in range")
	}
	if !have2317Deadline {
		t.Error("2317 (no real event) missing the statutory-deadline proxy fallback")
	}
}
