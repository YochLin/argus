package market

import (
	"testing"
	"time"
)

func date(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func TestIsHoliday_KnownDates(t *testing.T) {
	cases := []struct {
		name string
		date time.Time
		want bool
	}{
		// 2023
		{"2023 New Year's observed (Jan1 was Sunday)", date(2023, time.January, 2), true},
		{"2023 Jan1 itself not the observed date", date(2023, time.January, 1), false},
		{"2023 MLK", date(2023, time.January, 16), true},
		{"2023 Presidents Day", date(2023, time.February, 20), true},
		{"2023 Good Friday", date(2023, time.April, 7), true},
		{"2023 Memorial Day", date(2023, time.May, 29), true},
		{"2023 Juneteenth", date(2023, time.June, 19), true},
		{"2023 Independence Day", date(2023, time.July, 4), true},
		{"2023 Labor Day", date(2023, time.September, 4), true},
		{"2023 Thanksgiving", date(2023, time.November, 23), true},
		{"2023 Christmas", date(2023, time.December, 25), true},
		{"2023 ordinary trading day", date(2023, time.March, 15), false},

		// 2024
		{"2024 New Year's (Jan1 itself, a Monday)", date(2024, time.January, 1), true},
		{"2024 MLK", date(2024, time.January, 15), true},
		{"2024 Presidents Day", date(2024, time.February, 19), true},
		{"2024 Good Friday", date(2024, time.March, 29), true},
		{"2024 Memorial Day", date(2024, time.May, 27), true},
		{"2024 Juneteenth", date(2024, time.June, 19), true},
		{"2024 Independence Day", date(2024, time.July, 4), true},
		{"2024 Labor Day", date(2024, time.September, 2), true},
		{"2024 Thanksgiving", date(2024, time.November, 28), true},
		{"2024 Christmas", date(2024, time.December, 25), true},

		// 2025
		{"2025 New Year's", date(2025, time.January, 1), true},
		{"2025 MLK", date(2025, time.January, 20), true},
		{"2025 Presidents Day", date(2025, time.February, 17), true},
		{"2025 Good Friday", date(2025, time.April, 18), true},
		{"2025 Memorial Day", date(2025, time.May, 26), true},
		{"2025 Juneteenth", date(2025, time.June, 19), true},
		{"2025 Independence Day", date(2025, time.July, 4), true},
		{"2025 Labor Day", date(2025, time.September, 1), true},
		{"2025 Thanksgiving", date(2025, time.November, 27), true},
		{"2025 Christmas", date(2025, time.December, 25), true},

		// New Year's-on-Saturday exception (2022: Jan1 was a Saturday, and
		// unlike every other fixed-date holiday, NYSE does not shift it to
		// the preceding Friday, which is the last trading day of 2021).
		{"2022 Jan1 (Saturday) not itself flagged", date(2022, time.January, 1), false},
		{"2021 Dec31 (would-be shift target) stays a trading day", date(2021, time.December, 31), false},
		{"2022 Juneteenth (first year NYSE observed it)", date(2022, time.June, 20), true}, // Jun19,2022 was a Sunday
		{"2021 Juneteenth not yet an NYSE holiday", date(2021, time.June, 18), false},      // Jun19,2021 was a Saturday; pre-2022 so not shifted either

		// Ordinary Saturday->Friday / Sunday->Monday shifts on other
		// fixed-date holidays.
		{"2021 Christmas (Dec25 was Saturday) observed Dec24", date(2021, time.December, 24), true},
		{"2021 Dec25 itself not flagged", date(2021, time.December, 25), false},
		{"2021 Independence Day (Jul4 was Sunday) observed Jul5", date(2021, time.July, 5), true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsHoliday(c.date); got != c.want {
				t.Errorf("IsHoliday(%s) = %v, want %v", c.date.Format("2006-01-02"), got, c.want)
			}
		})
	}
}

func TestIsTradingDay(t *testing.T) {
	cases := []struct {
		name string
		date time.Time
		want bool
	}{
		{"ordinary Wednesday", date(2026, time.March, 4), true},
		{"Saturday", date(2026, time.March, 7), false},
		{"Sunday", date(2026, time.March, 8), false},
		{"a weekday holiday (2025 Thanksgiving)", date(2025, time.November, 27), false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsTradingDay(c.date); got != c.want {
				t.Errorf("IsTradingDay(%s) = %v, want %v", c.date.Format("2006-01-02"), got, c.want)
			}
		})
	}
}
