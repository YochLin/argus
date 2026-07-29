package data

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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
		baseURL: "https://openapi.twse.com.tw",
	}
}

type twseMoverRow struct {
	Code string `json:"Code"`
}

// GetMarketMovers returns today's TWSE top-20-by-volume tickers
// (MI_INDEX20, no query params — always the current session's top 20),
// filtered via isTWLeveragedOrInverse: the raw feed's top ranks are
// dominated by leveraged/inverse ETFs (live-verified 2026-07-28: 3 of the
// top 5 rows) that aren't useful BUY/SELL candidates for the
// recommendation pipeline. TPEx-listed movers aren't covered — TWSE's
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
		if r.Code == "" || isTWLeveragedOrInverse(r.Code) {
			continue
		}
		tickers = append(tickers, r.Code)
	}
	return tickers, nil
}

// isTWLeveragedOrInverse reports whether ticker is a leveraged ("L"
// suffix, e.g. "00685L" = 元大台灣50正2-style) or inverse ("R" suffix,
// e.g. "00632R") ETF, per Taiwan's own ticker-naming convention — the same
// symbol-shape-not-name-text heuristic IsUSEquitySymbol already uses for
// Yahoo's trending feed. Plain equity tickers are all-digit and never
// trigger this.
func isTWLeveragedOrInverse(ticker string) bool {
	return strings.HasSuffix(ticker, "L") || strings.HasSuffix(ticker, "R")
}
