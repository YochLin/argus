package data

import "testing"

func TestFilterEarningsCalendar(t *testing.T) {
	t.Run("keeps only wanted tickers", func(t *testing.T) {
		events := []finnhubEarningsEvent{
			{Symbol: "AAPL", Date: "2026-07-15", Hour: "amc"},
			{Symbol: "TSLA", Date: "2026-07-16", Hour: "bmo"},
			{Symbol: "NFLX", Date: "2026-07-17", Hour: ""},
		}
		got := filterEarningsCalendar([]string{"AAPL", "TSLA"}, events)
		if len(got) != 2 {
			t.Fatalf("filterEarningsCalendar() = %+v, want 2 entries", got)
		}
		if got["AAPL"] != (EarningsEvent{Ticker: "AAPL", Date: "2026-07-15", Hour: "amc"}) {
			t.Errorf("filterEarningsCalendar()[AAPL] = %+v, want AAPL 2026-07-15 amc", got["AAPL"])
		}
		if _, ok := got["NFLX"]; ok {
			t.Errorf("filterEarningsCalendar() included NFLX, which wasn't requested")
		}
	})

	t.Run("duplicate ticker keeps earliest date", func(t *testing.T) {
		events := []finnhubEarningsEvent{
			{Symbol: "AAPL", Date: "2026-07-20", Hour: "amc"},
			{Symbol: "AAPL", Date: "2026-07-15", Hour: "bmo"},
		}
		got := filterEarningsCalendar([]string{"AAPL"}, events)
		if got["AAPL"].Date != "2026-07-15" {
			t.Errorf("filterEarningsCalendar()[AAPL].Date = %q, want 2026-07-15 (earliest)", got["AAPL"].Date)
		}
	})

	t.Run("empty tickers returns nil", func(t *testing.T) {
		if got := filterEarningsCalendar(nil, []finnhubEarningsEvent{{Symbol: "AAPL", Date: "2026-07-15"}}); got != nil {
			t.Errorf("filterEarningsCalendar(nil tickers) = %+v, want nil", got)
		}
	})

	t.Run("no matches returns empty map, not nil", func(t *testing.T) {
		got := filterEarningsCalendar([]string{"AAPL"}, []finnhubEarningsEvent{{Symbol: "TSLA", Date: "2026-07-15"}})
		if got == nil || len(got) != 0 {
			t.Errorf("filterEarningsCalendar() = %+v, want empty non-nil map", got)
		}
	})
}
