package data

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// TWMarketMoversProvider is TW's narrow analog of Provider.GetMarketMovers —
// a separate interface (not folded into Provider/Multi) because no existing
// US provider can serve it and TWSE's own client has no reason to also
// implement GetQuote/GetNews/GetHistory, same reasoning as
// MarketNewsProvider/EarningsProvider staying their own interfaces.
type TWMarketMoversProvider interface {
	GetMarketMovers() ([]string, error)
}

// TWSE is the free, keyless TWSE OpenAPI client backing TW market movers
// (docs/... TW data-gap investigation, 2026-07-28). No Yahoo/Finnhub
// equivalent exists for this — Yahoo's /trending endpoint is US-only and
// Finnhub's free tier has no TW coverage at all (see finnhub.go's
// errTWNotSupported guards).
type TWSE struct {
	client  *http.Client
	baseURL string
	// rwdBaseURL is a second, separate host (www.twse.com.tw, not
	// openapi.twse.com.tw) for GetInstitutionalFlow's T86 report —
	// institutional_tw.go's own doc comment has the endpoint rationale.
	// Kept overridable the same way baseURL is, for the same test-against-
	// httptest reason.
	rwdBaseURL string

	// t86DayCache/t86Mu (Phase 25 §4.4) memoize fetchT86TrustForeignDay by
	// calendar day (YYYYMMDD), process-lifetime, shared across every ticker
	// GetTrustNetSeries is asked about. This exists because of a live-
	// verified finding, not speculative caution: building a multi-year T86
	// history by walking calendar days (2026-08-27, cmd/strategyscan
	// -build-t86-cache) triggered a sustained (>20 min observed) IP-level
	// WAF block on www.twse.com.tw/rwd/* after roughly 50 requests inside a
	// ~20s burst — a smaller batch of 7 requests spread over ~15s did not
	// trigger it. Without this cache, GetTrustNetSeries' own per-ticker walk
	// (up to liveTrustNetLookbackDays requests) repeated across the handful
	// of TW tickers 網5 checks in one scan run could stack into that same
	// range and take GetInstitutionalFlow — a different, already-shipped
	// feature sharing this same rwd host — down with it for the cooldown
	// window. Caching by day means N tickers checked the same run cost AT
	// MOST liveTrustNetLookbackDays requests total (usually far fewer, since
	// most days get reused across tickers), not N times that.
	t86DayCache map[string]map[string]TrustNetDay
	t86Mu       sync.Mutex
}

func NewTWSE() *TWSE {
	return &TWSE{
		// TWSE's OpenAPI resets the connection during an HTTP/2 handshake
		// (live-verified 2026-07-28: curl only succeeds with --http1.1,
		// even though the same client reaches Yahoo/cnyes fine over
		// HTTP/2) — an empty, non-nil TLSNextProto is net/http's
		// documented way to force HTTP/1.1 instead of failing outright on
		// the default Transport's HTTP/2 auto-negotiation.
		client: &http.Client{
			Timeout:   10 * time.Second,
			Transport: &http.Transport{TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{}},
		},
		baseURL:     "https://openapi.twse.com.tw",
		rwdBaseURL:  "https://www.twse.com.tw",
		t86DayCache: make(map[string]map[string]TrustNetDay),
	}
}

type twseMoverRow struct {
	Code string `json:"Code"`
}

// GetMarketMovers returns today's TWSE top-20-by-volume tickers
// (MI_INDEX20, no query params — always the current session's top 20),
// filtered via isTWETF: live-verified 2026-07-29 against the real endpoint
// (not just the raw curl check from the original 2026-07-28 investigation),
// the top ranks are dominated by ETFs of every kind — leveraged/inverse
// ("L"/"R" suffix, e.g. "00685L"/"00632R") but just as often a plain
// (non-leveraged) ETF like "0050"/"0056"/"00919"/"00403A" — only 7 of 17
// tickers that day were actual equities. An L/R-suffix-only filter (this
// function's original, narrower scope) missed all of those. Excluding by
// TWSE's own "00"-prefix ETF-code convention instead of by suffix catches
// every ETF regardless of share class/leverage, which is what the
// recommendation pipeline actually wants — a BUY/SELL candidate list is a
// list of stocks, not funds. TPEx-listed movers aren't covered — TWSE's
// OpenAPI only reports its own exchange, and there's no equivalent
// investigation done yet for a TPEx source.
func (t *TWSE) GetMarketMovers() ([]string, error) {
	req, err := http.NewRequest("GET", t.baseURL+"/v1/exchangeReport/MI_INDEX20", nil)
	if err != nil {
		return nil, err
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twse movers: status %d", resp.StatusCode)
	}

	var rows []twseMoverRow
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return nil, err
	}

	var tickers []string
	for _, r := range rows {
		if r.Code == "" || isTWETF(r.Code) {
			continue
		}
		tickers = append(tickers, r.Code)
	}
	return tickers, nil
}

// isTWETF reports whether ticker is a TW ETF, by Taiwan's own ticker-coding
// convention: every ETF (including a leveraged "L"-suffix or inverse
// "R"-suffix one, e.g. "00685L"/"00632R") is assigned a code starting "00",
// while a plain equity's code never does — the same symbol-shape-not-name-
// text heuristic IsUSEquitySymbol already uses for Yahoo's trending feed.
func isTWETF(ticker string) bool {
	return strings.HasPrefix(ticker, "00")
}
