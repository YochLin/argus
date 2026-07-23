package data

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"argus/internal/market"
)

type Finnhub struct {
	apiKey string
	client *http.Client
}

// errTWNotSupported is returned by every per-ticker Finnhub method for a TW
// ticker instead of making the request — Finnhub's free tier doesn't cover
// Taiwan listings, so without this guard every TW quote/news/fundamentals
// call would waste a doomed request before Multi falls through to Yahoo
// (same "doomed request" pattern CLAUDE.md already documents for a
// placeholder FINNHUB_API_KEY). GetUpcomingEarnings doesn't need this guard:
// it's one whole-market call filtered client-side, so a TW ticker just never
// matches rather than costing an extra request.
var errTWNotSupported = errors.New("finnhub: taiwan market not supported")

func NewFinnhub(apiKey string) *Finnhub {
	return &Finnhub{
		apiKey: apiKey,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (f *Finnhub) Name() string { return "finnhub" }

func (f *Finnhub) get(path string, out any) error {
	url := "https://finnhub.io/api/v1" + path
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Finnhub-Token", f.apiKey)

	resp, err := f.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("finnhub: status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (f *Finnhub) GetQuote(ticker string) (*Quote, error) {
	if market.Of(ticker) == market.TW {
		return nil, errTWNotSupported
	}
	var result struct {
		C  float64 `json:"c"`  // current price
		H  float64 `json:"h"`  // high
		L  float64 `json:"l"`  // low
		O  float64 `json:"o"`  // open
		PC float64 `json:"pc"` // previous close
		T  int64   `json:"t"`  // timestamp
	}
	if err := f.get(fmt.Sprintf("/quote?symbol=%s", ticker), &result); err != nil {
		return nil, err
	}
	if result.C == 0 {
		return nil, fmt.Errorf("finnhub: no data for %s", ticker)
	}

	changePercent := 0.0
	if result.PC != 0 {
		changePercent = (result.C - result.PC) / result.PC * 100
	}

	return &Quote{
		Ticker:        ticker,
		Price:         result.C,
		Open:          result.O,
		High:          result.H,
		Low:           result.L,
		PrevClose:     result.PC,
		ChangePercent: changePercent,
		Timestamp:     time.Unix(result.T, 0),
	}, nil
}

func (f *Finnhub) GetNews(ticker string, limit int) ([]NewsItem, error) {
	if market.Of(ticker) == market.TW {
		return nil, errTWNotSupported
	}
	from := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	to := time.Now().Format("2006-01-02")

	var results []struct {
		Headline string `json:"headline"`
		Summary  string `json:"summary"`
		Source   string `json:"source"`
		URL      string `json:"url"`
		Datetime int64  `json:"datetime"`
	}
	path := fmt.Sprintf("/company-news?symbol=%s&from=%s&to=%s", ticker, from, to)
	if err := f.get(path, &results); err != nil {
		return nil, err
	}

	var items []NewsItem
	for i, r := range results {
		if i >= limit {
			break
		}
		items = append(items, NewsItem{
			Headline:    r.Headline,
			Summary:     r.Summary,
			Source:      r.Source,
			URL:         r.URL,
			PublishedAt: time.Unix(r.Datetime, 0),
		})
	}
	return items, nil
}

func (f *Finnhub) GetMarketMovers() ([]string, error) {
	// Finnhub free tier doesn't have a market movers endpoint.
	// Return a curated list of high-volume NASDAQ 100 / S&P 500 names as fallback.
	return []string{
		"AAPL", "MSFT", "NVDA", "GOOGL", "AMZN", "META", "TSLA",
		"AVGO", "JPM", "V", "UNH", "XOM", "LLY", "MA", "HD",
		"PG", "COST", "MRK", "ABBV", "CVX",
	}, nil
}
