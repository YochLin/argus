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

func TestGetTWUpcomingEarnings(t *testing.T) {
	now, _ := time.Parse("2006-01-02", "2026-08-01")

	t.Run("only TW tickers get an entry", func(t *testing.T) {
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
		if got := GetTWUpcomingEarnings([]string{"AAPL"}, 20, now); got != nil {
			t.Errorf("GetTWUpcomingEarnings(no TW tickers) = %+v, want nil", got)
		}
	})
}
