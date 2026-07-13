package data

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Yahoo struct {
	client *http.Client
}

func NewYahoo() *Yahoo {
	return &Yahoo{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (y *Yahoo) Name() string { return "yahoo" }

func (y *Yahoo) GetQuote(ticker string) (*Quote, error) {
	url := fmt.Sprintf("https://query1.finance.yahoo.com/v8/finance/chart/%s?interval=1d&range=1d", ticker)
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

// GetHistory returns ~1 year of daily closes and volumes, oldest first —
// enough closes for a 200-period moving average (MA200 needs ~200 trading
// days) and still plenty of room for a 14-period RSI or a 12/26/9 MACD,
// which only read the tail of the slice regardless of how much extra
// history is in front of it. Volumes are index-aligned with closes.
func (y *Yahoo) GetHistory(ticker string) ([]float64, []int64, error) {
	url := fmt.Sprintf("https://query1.finance.yahoo.com/v8/finance/chart/%s?interval=1d&range=1y", ticker)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := y.client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("yahoo history: status %d for %s", resp.StatusCode, ticker)
	}

	var result struct {
		Chart struct {
			Result []struct {
				Indicators struct {
					Quote []struct {
						Close  []float64 `json:"close"`
						Volume []int64   `json:"volume"`
					} `json:"quote"`
				} `json:"indicators"`
			} `json:"result"`
		} `json:"chart"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, nil, err
	}
	if len(result.Chart.Result) == 0 || len(result.Chart.Result[0].Indicators.Quote) == 0 {
		return nil, nil, fmt.Errorf("yahoo history: no data for %s", ticker)
	}

	// Yahoo leaves a null (decoded as 0) hole for days without a trade
	// (e.g. halts); drop them rather than feeding a false price into RSI/MACD.
	// volumes is kept index-aligned with closes by dropping the same days.
	quote := result.Chart.Result[0].Indicators.Quote[0]
	var closes []float64
	var volumes []int64
	for i, c := range quote.Close {
		if c == 0 {
			continue
		}
		closes = append(closes, c)
		var v int64
		if i < len(quote.Volume) {
			v = quote.Volume[i]
		}
		volumes = append(volumes, v)
	}
	if len(closes) == 0 {
		return nil, nil, fmt.Errorf("yahoo history: no valid closes for %s", ticker)
	}
	return closes, volumes, nil
}

func (y *Yahoo) GetNews(ticker string, limit int) ([]NewsItem, error) {
	// Yahoo Finance news scraping via query2 endpoint
	url := fmt.Sprintf("https://query2.finance.yahoo.com/v1/finance/search?q=%s&newsCount=%d&quotesCount=0", ticker, limit)
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
			if isUSEquitySymbol(q.Symbol) {
				tickers = append(tickers, q.Symbol)
			}
		}
	}
	return tickers, nil
}

// isUSEquitySymbol filters Yahoo's /trending endpoint down to plain US
// equity tickers. Yahoo's trending API doesn't expose an asset-class field,
// so this relies on symbol shape: crypto pairs look like "BTC-USD" and
// foreign listings look like "SHOP.TO" — both contain characters a plain US
// ticker never does.
func isUSEquitySymbol(symbol string) bool {
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
