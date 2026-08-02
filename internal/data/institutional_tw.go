package data

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"argus/internal/market"
)

// errUSNotSupported mirrors errTWNotSupported (finnhub.go) in the other
// direction — TWSE's T86 report only ever lists TW-listed codes, so a US
// ticker would otherwise cost maxInstitutionalFlowLookback wasted whole-
// market requests walking back through days that were never going to
// contain it.
var errUSNotSupported = errors.New("twse: us market not supported")

// InstitutionalFlow is one ticker's daily net buy/sell from Taiwan's three
// major institutional investor categories (三大法人), from TWSE's own T86
// report (www.twse.com.tw/rwd/zh/fund/T86, free/keyless, live-verified
// 2026-08-02) — the same classification TWSE's own website uses, not a
// custom grouping. Foreign dealers get their own column separate from
// ordinary foreign investment because that's how TWSE reports it.
type InstitutionalFlow struct {
	Ticker           string
	Date             string // YYYY-MM-DD, the trading day T86 actually has data for (see GetInstitutionalFlow's lookback)
	ForeignNet       int64  // 外資及陸資買賣超股數 (excludes foreign dealers)
	ForeignDealerNet int64  // 外資自營商買賣超股數
	TrustNet         int64  // 投信買賣超股數
	DealerNet        int64  // 自營商買賣超股數 (自行買賣 + 避險 combined)
	TotalNet         int64  // 三大法人買賣超股數合計
}

// InstitutionalFlowProvider is TW-only — no US equivalent (SEC has no daily
// per-stock institutional-flow report; the closest analog, 13F, is quarterly
// and aggregated per-filer, not per-day-per-stock), same one-market-only
// reasoning as TWMarketMoversProvider.
type InstitutionalFlowProvider interface {
	GetInstitutionalFlow(ticker string) (*InstitutionalFlow, error)
}

// maxInstitutionalFlowLookback caps how many days GetInstitutionalFlow walks
// backward looking for the most recent session with a row for ticker —
// T86 has no row at all for a non-trading day (weekend/holiday), so a Monday
// call needs to skip back past the gap. 7 days covers any real TW holiday
// stretch (Lunar New Year, the longest, runs under 10 calendar days) without
// walking back indefinitely on a genuine outage.
const maxInstitutionalFlowLookback = 7

type twseT86Response struct {
	Date string     `json:"date"`
	Data [][]string `json:"data"`
}

// GetInstitutionalFlow fetches TWSE's T86 report (whole market, one request
// per day tried) and returns ticker's row, walking backward day by day (up
// to maxInstitutionalFlowLookback) until a session actually has one — same
// "unfiltered whole-market query, filter client-side" shape GetMarketMovers
// already uses, since T86 has no single-ticker query param.
func (t *TWSE) GetInstitutionalFlow(ticker string) (*InstitutionalFlow, error) {
	if market.Of(ticker) != market.TW {
		return nil, errUSNotSupported
	}
	for i := 0; i < maxInstitutionalFlowLookback; i++ {
		flow, err := t.fetchInstitutionalFlow(ticker, time.Now().AddDate(0, 0, -i))
		if err != nil {
			return nil, err
		}
		if flow != nil {
			return flow, nil
		}
	}
	return nil, fmt.Errorf("twse institutional flow: no data for %s in the last %d days", ticker, maxInstitutionalFlowLookback)
}

func (t *TWSE) fetchInstitutionalFlow(ticker string, date time.Time) (*InstitutionalFlow, error) {
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
		return nil, fmt.Errorf("twse institutional flow: status %d", resp.StatusCode)
	}

	var result twseT86Response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return findInstitutionalFlowRow(ticker, result), nil
}

// findInstitutionalFlowRow picks ticker's row out of a T86 whole-market
// response, split out from fetchInstitutionalFlow so the column-index
// parsing is testable without a live HTTP round-trip. Returns nil (not an
// error) if ticker has no row this session — the caller's walk-back loop is
// what turns "not found on any tried day" into an actual error.
func findInstitutionalFlowRow(ticker string, result twseT86Response) *InstitutionalFlow {
	// Column indices are T86's fixed field order (verified against a live
	// response, 2026-08-02): 0=code, 4=外資及陸資買賣超, 7=外資自營商買賣超,
	// 10=投信買賣超, 11=自營商買賣超(合計), 18=三大法人買賣超合計.
	for _, row := range result.Data {
		if len(row) < 19 || strings.TrimSpace(row[0]) != ticker {
			continue
		}
		reportDate := result.Date
		if len(reportDate) == 8 {
			reportDate = reportDate[:4] + "-" + reportDate[4:6] + "-" + reportDate[6:]
		}
		foreignNet, _ := parseTWSENetShares(row[4])
		foreignDealerNet, _ := parseTWSENetShares(row[7])
		trustNet, _ := parseTWSENetShares(row[10])
		dealerNet, _ := parseTWSENetShares(row[11])
		totalNet, _ := parseTWSENetShares(row[18])
		return &InstitutionalFlow{
			Ticker:           ticker,
			Date:             reportDate,
			ForeignNet:       foreignNet,
			ForeignDealerNet: foreignDealerNet,
			TrustNet:         trustNet,
			DealerNet:        dealerNet,
			TotalNet:         totalNet,
		}
	}
	return nil
}

func parseTWSENetShares(s string) (int64, error) {
	return strconv.ParseInt(strings.ReplaceAll(s, ",", ""), 10, 64)
}
