package data

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// yahooEarningsCallURL is Yahoo TW's real per-company 法說會 (earnings call)
// calendar — unlike tw_earnings.go's statutory-deadline proxy, this is an
// actual per-ticker announced date/time, scraped from the same
// "unofficial, could change without notice" risk class as cnyes.go/
// twse_movers.go. Live-verified 2026-09-01: the page only ever lists
// companies that have actually filed a notice, which in practice means
// roughly the current month — requesting a date far in the future (via
// ?date=) comes back with an empty calendar rather than an error, so this
// fetches "now onward" once and callers just get whatever real dates Yahoo
// already knows about.
const yahooEarningsCallURL = "https://tw.stock.yahoo.com/calendar/earnings-call"

// yahooEarningsCallCacheTTL bounds how often the page is refetched — real
// announcements trickle in during the trading day, so an hour catches
// same-day additions without hitting the page on every report/dashboard
// load.
const yahooEarningsCallCacheTTL = time.Hour

var (
	yahooEarningsCallMu    sync.Mutex
	yahooEarningsCallCache map[string][]EarningsEvent // bare ticker -> real events
	yahooEarningsCallAt    time.Time

	// yahooEarningsCallFetcher is overridden in tests to avoid a real network
	// call, same seam yahoo.go's overridable baseURL fields serve for its own
	// tests.
	yahooEarningsCallFetcher = fetchYahooEarningsCalls
)

// cachedYahooEarningsCalls returns the process-lifetime-cached real
// earnings-call events, refetching on a cache miss/expiry. A fetch error
// falls back to whatever's cached (possibly nil) rather than propagating —
// callers treat "no real event found" as normal and fall back to the
// statutory-deadline proxy themselves.
func cachedYahooEarningsCalls() map[string][]EarningsEvent {
	yahooEarningsCallMu.Lock()
	defer yahooEarningsCallMu.Unlock()
	if yahooEarningsCallCache != nil && time.Since(yahooEarningsCallAt) < yahooEarningsCallCacheTTL {
		return yahooEarningsCallCache
	}
	events, err := yahooEarningsCallFetcher()
	if err != nil {
		return yahooEarningsCallCache
	}
	yahooEarningsCallCache = events
	yahooEarningsCallAt = time.Now()
	return events
}

type yahooEarningsCallEvent struct {
	Symbol string `json:"symbol"`
	Date   string `json:"date"`
}

type yahooEarningsCallDay struct {
	EarningsCall []yahooEarningsCallEvent `json:"earningsCall"`
}

// fetchYahooEarningsCalls fetches and parses the Fusion.js state blob Yahoo
// TW embeds in the calendar page's first <script> tag (root.App.main = {...}),
// pulling out just its "calendars" object rather than the whole page state.
func fetchYahooEarningsCalls() (map[string][]EarningsEvent, error) {
	req, err := http.NewRequest("GET", yahooEarningsCallURL, nil)
	if err != nil {
		return nil, err
	}
	// A default Go User-Agent gets a 404 from Yahoo (live-verified
	// 2026-09-01); any browser-shaped one works.
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("yahoo earnings-call: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	const marker = `"calendars":`
	idx := bytes.Index(body, []byte(marker))
	if idx == -1 {
		return nil, fmt.Errorf("yahoo earnings-call: calendars block not found")
	}
	obj, ok := balancedJSONObject(body, idx+len(marker))
	if !ok {
		return nil, fmt.Errorf("yahoo earnings-call: malformed calendars block")
	}

	var days map[string]yahooEarningsCallDay
	if err := json.Unmarshal(obj, &days); err != nil {
		return nil, err
	}

	out := make(map[string][]EarningsEvent)
	for _, day := range days {
		for _, ev := range day.EarningsCall {
			t, err := time.Parse(time.RFC3339, ev.Date)
			if err != nil {
				continue
			}
			ticker := strings.TrimSuffix(strings.TrimSuffix(ev.Symbol, ".TWO"), ".TW")
			out[ticker] = append(out[ticker], EarningsEvent{Ticker: ticker, Date: t.Format("2006-01-02")})
		}
	}
	return out, nil
}

// balancedJSONObject returns the substring of body starting at the '{' at or
// after start and ending at its matching '}', respecting quoted strings so a
// literal brace inside a JSON string value doesn't miscount depth.
func balancedJSONObject(body []byte, start int) ([]byte, bool) {
	for start < len(body) && body[start] != '{' {
		start++
	}
	if start >= len(body) {
		return nil, false
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(body); i++ {
		c := body[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return body[start : i+1], true
			}
		}
	}
	return nil, false
}

// earliestInRange returns the earliest event in events whose Date falls
// within [from, to], if any.
func earliestInRange(events []EarningsEvent, from, to time.Time) (EarningsEvent, bool) {
	var best EarningsEvent
	found := false
	for _, ev := range events {
		d, err := time.Parse("2006-01-02", ev.Date)
		if err != nil || d.Before(from) || d.After(to) {
			continue
		}
		if !found || ev.Date < best.Date {
			best = ev
			found = true
		}
	}
	return best, found
}
