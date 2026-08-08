package data

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"

	"argus/internal/market"
)

type Yahoo struct {
	client *http.Client

	// chartBaseURL/searchBaseURL default to the real Yahoo endpoints;
	// overridable so tests can point at an httptest server instead (see
	// yahoo_test.go's TW suffix-fallback tests).
	chartBaseURL  string
	searchBaseURL string
	fcBaseURL     string // crumb cookie priming endpoint; overridable for tests, same reason as chartBaseURL

	// twSuffixCache remembers, per bare TW ticker, which Yahoo suffix
	// (".TW" or ".TWO") actually resolved last time — see resolveSymbol.
	twSuffixMu    sync.Mutex
	twSuffixCache map[string]string

	// optionClient carries a cookie jar for the crumb handshake the option
	// chain endpoint requires (see ensureCrumb) — quote/history/news need no
	// cookies, so this stays separate from client rather than growing a jar
	// those paths never use.
	optionClient *http.Client
	crumbMu      sync.Mutex
	crumb        string
}

func NewYahoo() *Yahoo {
	jar, _ := cookiejar.New(nil)
	return &Yahoo{
		client:        &http.Client{Timeout: 10 * time.Second},
		chartBaseURL:  "https://query1.finance.yahoo.com",
		searchBaseURL: "https://query2.finance.yahoo.com",
		fcBaseURL:     "https://fc.yahoo.com",
		twSuffixCache: make(map[string]string),
		optionClient:  &http.Client{Timeout: 10 * time.Second, Jar: jar},
	}
}

func (y *Yahoo) Name() string { return "yahoo" }

// resolveSymbol returns the ordered list of Yahoo chart-API symbols to try
// for ticker. A US ticker (market.Of) is used as-is. A TW ticker has no
// listed-exchange information in its bare form (2330 could be TWSE or
// TPEx) — Yahoo distinguishes them by suffix (.TW for TWSE-listed, .TWO
// for TPEx-listed), so an unresolved ticker gets both candidates in that
// order; once one succeeds, cacheTWSuffix remembers it so later calls for
// the same ticker only ever try the one that actually worked.
func (y *Yahoo) resolveSymbol(ticker string) []string {
	if market.Of(ticker) != market.TW {
		return []string{ticker}
	}
	y.twSuffixMu.Lock()
	suffix, ok := y.twSuffixCache[ticker]
	y.twSuffixMu.Unlock()
	if ok {
		return []string{ticker + suffix}
	}
	return []string{ticker + ".TW", ticker + ".TWO"}
}

// cacheTWSuffix records which suffix resolveSymbol should try first for
// ticker next time, based on the symbol that just succeeded. No-op for a
// US ticker (nothing to cache).
func (y *Yahoo) cacheTWSuffix(ticker, symbol string) {
	if market.Of(ticker) != market.TW {
		return
	}
	suffix := strings.TrimPrefix(symbol, ticker)
	y.twSuffixMu.Lock()
	y.twSuffixCache[ticker] = suffix
	y.twSuffixMu.Unlock()
}

func (y *Yahoo) GetQuote(ticker string) (*Quote, error) {
	var lastErr error
	for _, symbol := range y.resolveSymbol(ticker) {
		q, err := y.getQuote(symbol)
		if err != nil {
			lastErr = err
			continue
		}
		y.cacheTWSuffix(ticker, symbol)
		q.Ticker = ticker
		return q, nil
	}
	return nil, lastErr
}

// getQuote fetches a single Yahoo chart-API symbol as-is (no TW suffix
// resolution — see GetQuote, which tries resolveSymbol's candidates
// against this).
func (y *Yahoo) getQuote(ticker string) (*Quote, error) {
	url := fmt.Sprintf("%s/v8/finance/chart/%s?interval=1d&range=1d", y.chartBaseURL, ticker)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := y.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("yahoo: status %d for %s", resp.StatusCode, ticker)
	}

	var result struct {
		Chart struct {
			Result []struct {
				Meta struct {
					Symbol              string  `json:"symbol"`
					RegularMarketPrice  float64 `json:"regularMarketPrice"`
					ChartPreviousClose  float64 `json:"chartPreviousClose"`
					RegularMarketVolume int64   `json:"regularMarketVolume"`
					RegularMarketTime   int64   `json:"regularMarketTime"`
				} `json:"meta"`
				Indicators struct {
					Quote []struct {
						Open []float64 `json:"open"`
						High []float64 `json:"high"`
						Low  []float64 `json:"low"`
					} `json:"quote"`
				} `json:"indicators"`
			} `json:"result"`
			Error any `json:"error"`
		} `json:"chart"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if len(result.Chart.Result) == 0 {
		return nil, fmt.Errorf("yahoo: no data for %s", ticker)
	}

	meta := result.Chart.Result[0].Meta
	if meta.RegularMarketPrice == 0 {
		return nil, fmt.Errorf("yahoo: no market data for %s (invalid or delisted ticker?)", ticker)
	}

	changePercent := 0.0
	if meta.ChartPreviousClose != 0 {
		changePercent = (meta.RegularMarketPrice - meta.ChartPreviousClose) / meta.ChartPreviousClose * 100
	}

	// Prefer the exchange's own quote time over time.Now() — consumers use
	// it to tell a live quote from a stale one (e.g. the post-close snapshot
	// job skipping US market holidays).
	ts := time.Now()
	if meta.RegularMarketTime > 0 {
		ts = time.Unix(meta.RegularMarketTime, 0)
	}

	q := &Quote{
		Ticker:        ticker,
		Price:         meta.RegularMarketPrice,
		PrevClose:     meta.ChartPreviousClose,
		Volume:        meta.RegularMarketVolume,
		ChangePercent: changePercent,
		Timestamp:     ts,
	}

	if quotes := result.Chart.Result[0].Indicators.Quote; len(quotes) > 0 {
		opens := quotes[0].Open
		highs := quotes[0].High
		lows := quotes[0].Low
		if len(opens) > 0 {
			q.Open = opens[len(opens)-1]
		}
		if len(highs) > 0 {
			q.High = highs[len(highs)-1]
		}
		if len(lows) > 0 {
			q.Low = lows[len(lows)-1]
		}
	}

	return q, nil
}

// yahooDefaultRange is GetHistory's fallback for an unspecified rangeParam —
// "1y" is enough closes for a 200-period moving average and plenty of room
// for a 14-period RSI/ATR or a 12/26/9 MACD, which only read the tail of the
// series regardless of how much extra history is in front of it.
//
// yahooMaxRange/yahooMaxRangeSubstitute are the two ends of the one
// rangeParam value GetHistory rewrites: Yahoo's chart API silently ignores
// interval=1d for range=max and returns quarterly bars instead
// (live-verified: AAPL "max" comes back as 168 bars, matching "3mo";
// "1y"/"2y"/"5y"/"10y" all honor interval=1d correctly) — 10y is effectively
// "max" for any real position's holding period and actually gives daily
// bars. Every other rangeParam value ("2y", "3mo", ...) is an arbitrary
// Yahoo chart-API range string passed straight through by callers, not a
// fixed enum this function knows about, so those aren't named here.
const (
	yahooDefaultRange       = "1y"
	yahooMaxRange           = "max"
	yahooMaxRangeSubstitute = "10y"
)

// GetHistory returns daily OHLCV candles, oldest first, over rangeParam's
// window (a Yahoo chart API range value). Open comes from
// indicators.quote[0].open, same as GetQuote — the chart meta has no usable
// open field. Date comes from the top-level timestamp array, index-aligned
// with the quote arrays.
func (y *Yahoo) GetHistory(ticker, rangeParam string) ([]Candle, error) {
	if rangeParam == "" {
		rangeParam = yahooDefaultRange
	}
	if rangeParam == yahooMaxRange {
		rangeParam = yahooMaxRangeSubstitute
	}
	var lastErr error
	for _, symbol := range y.resolveSymbol(ticker) {
		candles, err := y.getHistory(symbol, rangeParam)
		if err != nil {
			lastErr = err
			continue
		}
		y.cacheTWSuffix(ticker, symbol)
		return candles, nil
	}
	return nil, lastErr
}

// getHistory fetches a single Yahoo chart-API symbol as-is — see GetHistory.
func (y *Yahoo) getHistory(ticker, rangeParam string) ([]Candle, error) {
	url := fmt.Sprintf("%s/v8/finance/chart/%s?interval=1d&range=%s", y.chartBaseURL, ticker, rangeParam)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := y.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("yahoo history: status %d for %s", resp.StatusCode, ticker)
	}

	var result struct {
		Chart struct {
			Result []struct {
				Timestamp  []int64 `json:"timestamp"`
				Indicators struct {
					Quote []struct {
						Open   []float64 `json:"open"`
						Close  []float64 `json:"close"`
						High   []float64 `json:"high"`
						Low    []float64 `json:"low"`
						Volume []int64   `json:"volume"`
					} `json:"quote"`
				} `json:"indicators"`
			} `json:"result"`
		} `json:"chart"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if len(result.Chart.Result) == 0 || len(result.Chart.Result[0].Indicators.Quote) == 0 {
		return nil, fmt.Errorf("yahoo history: no data for %s", ticker)
	}

	// Yahoo leaves a null (decoded as 0) hole for days without a trade
	// (e.g. halts); drop them rather than feeding a false price into
	// RSI/MACD/ATR. The other fields stay aligned by being read at the same
	// index of the same day.
	timestamps := result.Chart.Result[0].Timestamp
	quote := result.Chart.Result[0].Indicators.Quote[0]
	var candles []Candle
	for i, c := range quote.Close {
		if c == 0 {
			continue
		}
		candle := Candle{Close: c}
		if i < len(timestamps) {
			candle.Date = time.Unix(timestamps[i], 0)
		}
		if i < len(quote.Open) {
			candle.Open = quote.Open[i]
		}
		if i < len(quote.High) {
			candle.High = quote.High[i]
		}
		if i < len(quote.Low) {
			candle.Low = quote.Low[i]
		}
		if i < len(quote.Volume) {
			candle.Volume = quote.Volume[i]
		}
		candles = append(candles, candle)
	}
	if len(candles) == 0 {
		return nil, fmt.Errorf("yahoo history: no valid closes for %s", ticker)
	}
	return candles, nil
}

func (y *Yahoo) GetNews(ticker string, limit int) ([]NewsItem, error) {
	var lastErr error
	for _, symbol := range y.resolveSymbol(ticker) {
		items, err := y.getNews(symbol, limit)
		if err != nil {
			lastErr = err
			continue
		}
		y.cacheTWSuffix(ticker, symbol)
		return items, nil
	}
	return nil, lastErr
}

// getNews fetches a single Yahoo search-API symbol as-is — see GetNews.
func (y *Yahoo) getNews(ticker string, limit int) ([]NewsItem, error) {
	// Yahoo Finance news scraping via query2 endpoint
	url := fmt.Sprintf("%s/v1/finance/search?q=%s&newsCount=%d&quotesCount=0", y.searchBaseURL, ticker, limit)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := y.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("yahoo news: status %d", resp.StatusCode)
	}

	var result struct {
		News []struct {
			Title               string `json:"title"`
			Publisher           string `json:"publisher"`
			Link                string `json:"link"`
			ProviderPublishTime int64  `json:"providerPublishTime"`
		} `json:"news"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var items []NewsItem
	for _, n := range result.News {
		items = append(items, NewsItem{
			Headline:    n.Title,
			Source:      n.Publisher,
			URL:         n.Link,
			PublishedAt: time.Unix(n.ProviderPublishTime, 0),
		})
	}
	return items, nil
}

func (y *Yahoo) GetMarketMovers() ([]string, error) {
	// Yahoo Finance trending tickers
	url := "https://query1.finance.yahoo.com/v1/finance/trending/US?count=20"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := y.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("yahoo trending: status %d", resp.StatusCode)
	}

	var result struct {
		Finance struct {
			Result []struct {
				Quotes []struct {
					Symbol string `json:"symbol"`
				} `json:"quotes"`
			} `json:"result"`
		} `json:"finance"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var tickers []string
	if len(result.Finance.Result) > 0 {
		for _, q := range result.Finance.Result[0].Quotes {
			if IsUSEquitySymbol(q.Symbol) {
				tickers = append(tickers, q.Symbol)
			}
		}
	}
	return tickers, nil
}

// IsUSEquitySymbol filters Yahoo's /trending endpoint down to plain US
// equity tickers. Yahoo's trending API doesn't expose an asset-class field,
// so this relies on symbol shape: crypto pairs look like "BTC-USD" and
// foreign listings look like "SHOP.TO" — both contain characters a plain US
// ticker never does. Exported (Phase 2.6 解凍's two-stage LLM exploration,
// docs/phase-2.6-two-stage-llm-exploration.md) for a second caller,
// bot.exploreCandidates' validation chain, which runs the same shape check
// against LLM-nominated tickers before trusting them enough to spend a
// GetQuote call on.
func IsUSEquitySymbol(symbol string) bool {
	if symbol == "" || len(symbol) > 5 {
		return false
	}
	for _, r := range symbol {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

// ensureCrumb returns a cached crumb, or performs the two-request handshake
// Yahoo's option chain endpoint requires (fc.yahoo.com primes a session
// cookie in optionClient's jar; /v1/test/getcrumb then returns the crumb as
// plain text) when force is true or nothing is cached yet.
func (y *Yahoo) ensureCrumb(force bool) (string, error) {
	y.crumbMu.Lock()
	defer y.crumbMu.Unlock()
	if y.crumb != "" && !force {
		return y.crumb, nil
	}

	primeReq, err := http.NewRequest("GET", y.fcBaseURL, nil)
	if err != nil {
		return "", err
	}
	primeReq.Header.Set("User-Agent", "Mozilla/5.0")
	primeResp, err := y.optionClient.Do(primeReq)
	if err != nil {
		return "", fmt.Errorf("yahoo: crumb cookie priming failed: %w", err)
	}
	primeResp.Body.Close()

	crumbReq, err := http.NewRequest("GET", y.chartBaseURL+"/v1/test/getcrumb", nil)
	if err != nil {
		return "", err
	}
	crumbReq.Header.Set("User-Agent", "Mozilla/5.0")
	crumbResp, err := y.optionClient.Do(crumbReq)
	if err != nil {
		return "", fmt.Errorf("yahoo: getcrumb request failed: %w", err)
	}
	defer crumbResp.Body.Close()
	if crumbResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("yahoo: getcrumb status %d", crumbResp.StatusCode)
	}
	body, err := io.ReadAll(crumbResp.Body)
	if err != nil {
		return "", err
	}
	crumb := strings.TrimSpace(string(body))
	if crumb == "" {
		return "", fmt.Errorf("yahoo: empty crumb")
	}
	y.crumb = crumb
	return crumb, nil
}

type yahooOptionContract struct {
	ContractSymbol    string  `json:"contractSymbol"`
	Strike            float64 `json:"strike"`
	LastPrice         float64 `json:"lastPrice"`
	Bid               float64 `json:"bid"`
	Ask               float64 `json:"ask"`
	Volume            int64   `json:"volume"`
	OpenInterest      int64   `json:"openInterest"`
	ImpliedVolatility float64 `json:"impliedVolatility"`
	InTheMoney        bool    `json:"inTheMoney"`
	Expiration        int64   `json:"expiration"`
	LastTradeDate     int64   `json:"lastTradeDate"`
}

type yahooOptionChainResp struct {
	OptionChain struct {
		Result []struct {
			ExpirationDates []int64 `json:"expirationDates"`
			Options         []struct {
				Calls []yahooOptionContract `json:"calls"`
				Puts  []yahooOptionContract `json:"puts"`
			} `json:"options"`
		} `json:"result"`
		Error any `json:"error"`
	} `json:"optionChain"`
}

// fetchOptionChain calls Yahoo's v7 option chain endpoint for symbol. A nil
// expiry fetches the nearest listed expiry (enough to read expirationDates
// off); a non-nil expiry is passed through as Yahoo's ?date= param, which
// snaps to the nearest listed date rather than erroring on a mismatch.
// Retries the crumb handshake exactly once on a 401, to avoid hammering the
// endpoint if Yahoo has changed the mechanism entirely.
func (y *Yahoo) fetchOptionChain(symbol string, expiry *time.Time) (*yahooOptionChainResp, error) {
	crumb, err := y.ensureCrumb(false)
	if err != nil {
		return nil, err
	}

	buildURL := func(crumb string) string {
		u := fmt.Sprintf("%s/v7/finance/options/%s?crumb=%s", y.chartBaseURL, symbol, url.QueryEscape(crumb))
		if expiry != nil {
			u += fmt.Sprintf("&date=%d", expiry.Unix())
		}
		return u
	}

	do := func(crumb string) (*http.Response, error) {
		req, err := http.NewRequest("GET", buildURL(crumb), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0")
		return y.optionClient.Do(req)
	}

	resp, err := do(crumb)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		crumb, err = y.ensureCrumb(true)
		if err != nil {
			return nil, err
		}
		resp, err = do(crumb)
		if err != nil {
			return nil, err
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("yahoo options: status %d for %s", resp.StatusCode, symbol)
	}

	var result yahooOptionChainResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if len(result.OptionChain.Result) == 0 {
		return nil, fmt.Errorf("yahoo options: no data for %s", symbol)
	}
	return &result, nil
}

// GetOptionExpirations returns the listed expiry dates for underlying.
// Returns (nil, nil) for a TW ticker — the OptionChainProvider contract
// (options.go) is US-only, same as the interface doc says.
func (y *Yahoo) GetOptionExpirations(underlying string) ([]time.Time, error) {
	if market.Of(underlying) != market.US {
		return nil, nil
	}
	result, err := y.fetchOptionChain(underlying, nil)
	if err != nil {
		return nil, err
	}
	dates := result.OptionChain.Result[0].ExpirationDates
	out := make([]time.Time, len(dates))
	for i, d := range dates {
		out[i] = time.Unix(d, 0).UTC()
	}
	return out, nil
}

// GetOptionChain returns every call and put contract at expiry. Returns
// (nil, nil) for a TW ticker, same as GetOptionExpirations.
func (y *Yahoo) GetOptionChain(underlying string, expiry time.Time) ([]OptionQuote, error) {
	if market.Of(underlying) != market.US {
		return nil, nil
	}
	result, err := y.fetchOptionChain(underlying, &expiry)
	if err != nil {
		return nil, err
	}
	options := result.OptionChain.Result[0].Options
	if len(options) == 0 {
		return nil, fmt.Errorf("yahoo options: no contracts for %s at %s", underlying, expiry.Format("2006-01-02"))
	}

	convert := func(right string, contracts []yahooOptionContract) []OptionQuote {
		out := make([]OptionQuote, len(contracts))
		for i, c := range contracts {
			out[i] = OptionQuote{
				ContractSymbol:    c.ContractSymbol,
				Right:             right,
				Strike:            c.Strike,
				LastPrice:         c.LastPrice,
				Bid:               c.Bid,
				Ask:               c.Ask,
				Volume:            c.Volume,
				OpenInterest:      c.OpenInterest,
				ImpliedVolatility: c.ImpliedVolatility,
				InTheMoney:        c.InTheMoney,
				Expiration:        time.Unix(c.Expiration, 0).UTC(),
				LastTradeDate:     time.Unix(c.LastTradeDate, 0).UTC(),
			}
		}
		return out
	}

	var out []OptionQuote
	out = append(out, convert("C", options[0].Calls)...)
	out = append(out, convert("P", options[0].Puts)...)
	return out, nil
}
