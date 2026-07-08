package data

import "time"

// MarketNewsProvider is implemented only by Finnhub — same reasoning as
// FundamentalsProvider/EarningsProvider: no Yahoo equivalent we're willing to
// depend on.
type MarketNewsProvider interface {
	// GetMarketNews returns up to limit of the most recent general (not
	// per-ticker) market/macro news items, newest first.
	GetMarketNews(limit int) ([]NewsItem, error)
}

// GetMarketNews fetches Finnhub's general market news category — unlike
// GetNews (per-ticker /company-news), this isn't scoped to any symbol, so it
// covers macro/market-wide stories for the daily-report/recommend news
// summary.
func (f *Finnhub) GetMarketNews(limit int) ([]NewsItem, error) {
	var results []struct {
		Headline string `json:"headline"`
		Summary  string `json:"summary"`
		Source   string `json:"source"`
		URL      string `json:"url"`
		Datetime int64  `json:"datetime"`
	}
	if err := f.get("/news?category=general", &results); err != nil {
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
