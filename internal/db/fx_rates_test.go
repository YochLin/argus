package db

import "testing"

func TestGetFXRateOnOrBefore(t *testing.T) {
	d := newTestDB(t)
	for date, rate := range map[string]float64{"2026-09-01": 30, "2026-10-10": 31, "2026-10-20": 32} {
		if err := d.SaveFXRate(date, "USDTWD", rate); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.SaveFXRate("2026-10-11", "JPYTWD", 0.2); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		date string
		days int
		want float64
		ok   bool
	}{
		{"2026-10-20", 45, 32, true}, // exact day wins over older ones
		{"2026-10-15", 45, 31, true}, // newest one on or before
		{"2026-10-15", 5, 31, true},  // exactly at the edge of the window
		{"2026-10-16", 5, 0, false},  // window ends before the 10-10 rate
		{"2026-10-05", 45, 30, true}, // falls back across a month, within 45 days
		{"2026-08-31", 45, 0, false}, // nothing recorded yet
		{"2026-12-10", 45, 0, false}, // newest rate is more than 45 days old
	}
	for _, c := range cases {
		got, ok, err := d.GetFXRateOnOrBefore(c.date, "USDTWD", c.days)
		if err != nil || ok != c.ok || got != c.want {
			t.Errorf("%s/%dd: got %v ok=%v err=%v, want %v ok=%v", c.date, c.days, got, ok, err, c.want, c.ok)
		}
	}
}
