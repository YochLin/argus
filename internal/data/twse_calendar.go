package data

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// TWTradingDayProvider is the authoritative TW trading-calendar counterpart
// to internal/market's IsTradingDay — that package is deliberately
// dependency-free (no network, NYSE-only per its doc comment), so this one
// lives here alongside TWSE's other live endpoints (twse_movers.go,
// institutional_tw.go). Backs PLAN.md's 台股盤前晨報交易日判斷 follow-up:
// replaces RunTWMorningBriefing's 0050-quote-staleness heuristic, which
// can't tell "single-day holiday" from "first trading day after a long
// break" apart, with TWSE's own published schedule.
type TWTradingDayProvider interface {
	IsTWTradingDay(date time.Time) (bool, error)
}

type twseHolidayResponse struct {
	Data [][]string `json:"data"`
}

// IsTWTradingDay reports whether date is a TWSE trading day, from TWSE's own
// published market-open/close schedule (free/keyless, same rwdBaseURL host
// as institutional_tw.go's T86 report).
func (t *TWSE) IsTWTradingDay(date time.Time) (bool, error) {
	if wd := date.Weekday(); wd == time.Saturday || wd == time.Sunday {
		return false, nil
	}
	holidays, err := t.twHolidaysForYear(date.Year())
	if err != nil {
		return false, err
	}
	return !holidays[date.Format("2006-01-02")], nil
}

// twHolidaysForYear fetches and caches (process-lifetime, one request per
// year ever queried) the set of non-trading dates in year. The `date` query
// param only selects which year's schedule comes back — any date within
// that year returns the same full-year list (live-verified against
// 2025/2026), so Jan 1 is used unconditionally.
func (t *TWSE) twHolidaysForYear(year int) (map[string]bool, error) {
	t.holidayMu.Lock()
	if cached, ok := t.holidayCache[year]; ok {
		t.holidayMu.Unlock()
		return cached, nil
	}
	t.holidayMu.Unlock()

	url := fmt.Sprintf("%s/rwd/zh/holidaySchedule/holidaySchedule?response=json&date=%04d0101", t.rwdBaseURL, year)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twse holiday schedule: status %d", resp.StatusCode)
	}

	var result twseHolidayResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	holidays := make(map[string]bool)
	for _, row := range result.Data {
		if len(row) < 2 {
			continue
		}
		date, name := row[0], row[1]
		// TWSE's schedule also lists informational markers for the first/
		// last trading day around a multi-day break (e.g. "農曆春節後開始
		// 交易日", "農曆春節前最後交易日", "國曆新年開始交易日") — those are
		// themselves real trading days, not holidays. A plain "交易"
		// substring check is too broad (a settlement-only closure is named
		// "市場無交易，僅辦理結算交割作業" — a real non-trading day whose
		// name also contains "交易"); "開始交易"/"最後交易" is the specific
		// phrase TWSE's marker names use, live-verified against the 2026
		// schedule.
		if strings.Contains(name, "開始交易") || strings.Contains(name, "最後交易") {
			continue
		}
		holidays[date] = true
	}

	t.holidayMu.Lock()
	if t.holidayCache == nil {
		t.holidayCache = make(map[int]map[string]bool)
	}
	t.holidayCache[year] = holidays
	t.holidayMu.Unlock()
	return holidays, nil
}
