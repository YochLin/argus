package data

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"argus/internal/market"
)

// twseT86FullResponse is T86's whole-market response, kept separate from
// institutional_tw.go's twseT86Response because this one also needs the
// `fields` header text — see resolveT86Columns below for why a fixed index
// isn't safe across TWSE's own history.
type twseT86FullResponse struct {
	Date   string     `json:"date"`
	Fields []string   `json:"fields"`
	Data   [][]string `json:"data"`
}

// t86ForeignNetLabels/t86TrustNetLabel are T86's own column header text for
// the two fields 網5 needs, across every schema TWSE has used back to (at
// least) 2015 — live-verified 2026-08-27 by fetching T86 at 2015-01-05,
// 2016-01-04, 2017-01-03/2017-07-03, 2018-01-02, 2020-01-02, 2022-01-04,
// 2024-01-02, and today (2026-08-25): sometime between 2016-01 and
// 2017-01-03 the report went from 16 columns to 19, splitting what had been
// a single "外資買賣超股數" column into "外陸資買賣超股數(不含外資自營商)"
// plus a new "外資自營商買賣超股數" column, and moving 投信's three columns
// over accordingly. "投信買賣超股數" itself never changed label or moved
// relative meaning across that break.
//
// This is why columns are resolved by the response's own `fields` header
// text on every call instead of a fixed index (institutional_tw.go's
// findInstitutionalFlowRow can get away with fixed indices because it only
// ever reads *today's* report, which is always post-break) — an index-based
// reader silently reads the wrong column for any date before the break
// instead of erroring, which is worse than the extra string compare here.
// Matching is exact-equality, not prefix/substring: "自營商買賣超股數" is
// itself a strict prefix of two OTHER real column names
// ("自營商買賣超股數(自行買賣)"/"(避險)"), so a substring match would pick
// the wrong one.
var (
	t86ForeignNetLabels = []string{"外資買賣超股數", "外陸資買賣超股數(不含外資自營商)"}
	t86TrustNetLabel    = "投信買賣超股數"
)

// resolveT86Columns finds the column indices this client needs inside one
// T86 response's `fields` header. Ticker is always column 0 (true across
// every era tested above).
func resolveT86Columns(fields []string) (tickerIdx, foreignIdx, trustIdx int, err error) {
	foreignIdx, trustIdx = -1, -1
	for i, f := range fields {
		f = strings.TrimSpace(f)
		if foreignIdx == -1 {
			for _, label := range t86ForeignNetLabels {
				if f == label {
					foreignIdx = i
					break
				}
			}
		}
		if f == t86TrustNetLabel {
			trustIdx = i
		}
	}
	if foreignIdx == -1 || trustIdx == -1 {
		return 0, 0, 0, fmt.Errorf("twse t86: unrecognized column layout (fields=%v)", fields)
	}
	return 0, foreignIdx, trustIdx, nil
}

// fetchT86TrustForeignDay fetches TWSE's whole-market T86 report for date
// and returns every listed ticker's 投信/外資 net that day, reduced to
// data.TrustNetDay — the same shape FinMind's GetTrustNetSeries already
// returns, so internal/signals' AlignTrustNet/AlignForeignNet and 網5 work
// unmodified against either source. A non-trading day (weekend/holiday) has
// an empty data.data and comes back as a nil map, not an error — same
// "missing means genuinely nothing happened" contract findInstitutionalFlowRow
// already relies on.
func (t *TWSE) fetchT86TrustForeignDay(date time.Time) (map[string]TrustNetDay, error) {
	dateKey := date.Format("20060102")

	t.t86Mu.Lock()
	cached, hit := t.t86DayCache[dateKey]
	t.t86Mu.Unlock()
	if hit {
		return cached, nil
	}

	dayMap, err := t.doFetchT86TrustForeignDay(date)
	if err != nil {
		return nil, err
	}
	// A nil dayMap is ambiguous — TWSE answers a genuine non-trading day AND
	// a transient block/outage with the exact same "no data" shape (see
	// doFetchT86TrustForeignDay's doc comment). Caching it as confirmed
	// absence is only safe for a weekend, which is a real non-trading day
	// unconditionally regardless of market; a weekday nil is left
	// uncached so a transient block doesn't get memoized as "this day never
	// has data" for the rest of the process's lifetime. internal/market's
	// trading calendar isn't used here — it's NYSE-specific (see its doc
	// comment), not TW's.
	if dayMap != nil || date.Weekday() == time.Saturday || date.Weekday() == time.Sunday {
		t.t86Mu.Lock()
		t.t86DayCache[dateKey] = dayMap
		t.t86Mu.Unlock()
	}
	return dayMap, nil
}

// doFetchT86TrustForeignDay is fetchT86TrustForeignDay's actual HTTP
// round-trip, split out so the cache check/store above wraps a single
// return point instead of every early-return inside the parse needing its
// own cache-store call.
func (t *TWSE) doFetchT86TrustForeignDay(date time.Time) (map[string]TrustNetDay, error) {
	url := fmt.Sprintf("%s/rwd/zh/fund/T86?date=%s&selectType=ALL&response=json", t.rwdBaseURL, date.Format("20060102"))
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
		// Live-verified 2026-08-27: TWSE answers a date with no T86 report
		// at all — weekends, national holidays, and at least one ad-hoc
		// closure found while building the historical cache (2016-09-23, a
		// Friday, almost certainly a typhoon closure — internal/market's
		// trading calendar doesn't cover those, see its doc comment) — with
		// an HTTP 307 to an HTML "security" page, not an empty JSON body.
		// Treating any non-200 here as "no data this day" (nil, nil) rather
		// than an error is what makes buildT86Cache's calendar-day walk
		// (t86_cache.go) match buildHistoryCache's existing "empty means
		// skip" contract instead of retrying and aborting the whole build
		// over a real, permanent non-trading day.
		return nil, nil
	}

	var result twseT86FullResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if len(result.Data) == 0 {
		return nil, nil
	}
	tickerIdx, foreignIdx, trustIdx, err := resolveT86Columns(result.Fields)
	if err != nil {
		return nil, err
	}

	day := date.Truncate(24 * time.Hour)
	out := make(map[string]TrustNetDay, len(result.Data))
	for _, row := range result.Data {
		if len(row) <= foreignIdx || len(row) <= trustIdx {
			continue
		}
		code := strings.TrimSpace(row[tickerIdx])
		if code == "" {
			continue
		}
		foreignNet, _ := parseTWSENetShares(row[foreignIdx])
		trustNet, _ := parseTWSENetShares(row[trustIdx])
		out[code] = TrustNetDay{Date: day, Net: trustNet, ForeignNet: foreignNet}
	}
	return out, nil
}

// GetT86Day exports fetchT86TrustForeignDay for cmd/strategyscan's bulk
// history-cache builder (t86_cache.go), which needs the whole-market map
// directly — one HTTP request serves every ticker for that day, versus
// GetTrustNetSeries below filtering the same request down to one ticker at
// a time.
func (t *TWSE) GetT86Day(date time.Time) (map[string]TrustNetDay, error) {
	return t.fetchT86TrustForeignDay(date)
}

// liveTrustNetLookbackDays bounds GetTrustNetSeries' live walk-back,
// independent of the days argument the caller passes. T86 has no ranged
// query — one HTTP request is one calendar day, whole market — and 網5's
// own logic (trustBuyWindow, TrustNetVolPct) only ever reads the trailing
// 3-5 TRADING days. The live caller (internal/service/scan.go) passes
// days=len(candles), which for its usual ~1y candle window is ~252 — walking
// that many individual TWSE requests per ticker on every scan would be both
// slow and pointless for data 網5 never looks at. 20 calendar days is
// generous headroom over the 5 trading days actually needed, covering any
// normal weekend/holiday gap. Backtesting a multi-year window instead goes
// through cmd/strategyscan's bulk day-major cache (GetT86Day +
// -build-t86-cache), which amortizes one request per trading day across the
// WHOLE universe instead of per ticker.
const liveTrustNetLookbackDays = 20

// GetTrustNetSeries implements data.TrustNetProvider directly off TWSE's own
// T86 report (Phase 25 §4.4) — the canonical source, not FinMind's secondhand
// copy, and the only free source with 外資 alongside 投信. See
// liveTrustNetLookbackDays for why days is capped rather than honored as-is.
func (t *TWSE) GetTrustNetSeries(ticker string, days int) ([]TrustNetDay, error) {
	if market.Of(ticker) != market.TW {
		return nil, errUSNotSupported
	}
	lookback := days
	if lookback <= 0 || lookback > liveTrustNetLookbackDays {
		lookback = liveTrustNetLookbackDays
	}

	var out []TrustNetDay
	failures := 0
	for i := 0; i < lookback; i++ {
		if i > 0 {
			time.Sleep(150 * time.Millisecond) // be a polite whole-market caller, same spirit as the Yahoo/FinMind rate limits elsewhere
		}
		dayMap, err := t.fetchT86TrustForeignDay(time.Now().AddDate(0, 0, -i))
		if err != nil {
			failures++
			continue
		}
		if row, ok := dayMap[ticker]; ok {
			out = append(out, row)
		}
	}
	if failures == lookback {
		return nil, fmt.Errorf("twse trust net series: all %d days failed to fetch", lookback)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out, nil
}
