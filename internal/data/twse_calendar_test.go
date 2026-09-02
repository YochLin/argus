package data

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// twseHolidayTestServer serves a trimmed version of TWSE's real 2026
// holidaySchedule response — a genuine holiday, a settlement-only no-trading
// day, and a "start/end of break" trading-day marker whose name contains
// "交易" — plus a request counter so the per-year cache can be asserted.
func twseHolidayTestServer(t *testing.T, requests *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*requests++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"stat":"ok","data":[
			["2026-01-01","中華民國開國紀念日","依規定放假1日。"],
			["2026-01-02","國曆新年開始交易日","國曆新年開始交易。"],
			["2026-02-12","市場無交易，僅辦理結算交割作業",""]
		]}`)
	}))
}

func TestTWSEIsTWTradingDay(t *testing.T) {
	var requests int
	srv := twseHolidayTestServer(t, &requests)
	defer srv.Close()

	tw := NewTWSE()
	tw.rwdBaseURL = srv.URL

	tests := []struct {
		name string
		date string // YYYY-MM-DD
		want bool
	}{
		{"real holiday", "2026-01-01", false},
		{"trading-resumption marker is a real trading day, not a holiday", "2026-01-02", true},
		{"settlement-only day", "2026-02-12", false},
		{"ordinary weekday not in the schedule", "2026-01-05", true},
		{"weekend, short-circuited before the schedule is even consulted", "2026-01-03", false}, // a Saturday
	}
	for _, tt := range tests {
		date, err := time.Parse("2006-01-02", tt.date)
		if err != nil {
			t.Fatalf("parse %q: %v", tt.date, err)
		}
		got, err := tw.IsTWTradingDay(date)
		if err != nil {
			t.Fatalf("IsTWTradingDay(%s): %v", tt.date, err)
		}
		if got != tt.want {
			t.Errorf("%s: IsTWTradingDay(%s) = %v, want %v", tt.name, tt.date, got, tt.want)
		}
	}

	if requests != 1 {
		t.Errorf("requests = %d, want 1 (one fetch for the whole year, cached across every date checked above)", requests)
	}
}
