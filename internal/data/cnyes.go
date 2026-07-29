package data

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Cnyes is the free, keyless 鉅亨網 (Anue) client backing TW's
// MarketNewsProvider (docs/... TW data-gap investigation, 2026-07-28) —
// Finnhub's /news?category=general has no TW equivalent, and this is an
// unofficial API with the same "could disappear without notice" risk
// CLAUDE.md already documents for Yahoo's chart API.
type Cnyes struct {
	client  *http.Client
	baseURL string
}

func NewCnyes() *Cnyes {
	return &Cnyes{
		client:  &http.Client{Timeout: 10 * time.Second},
		baseURL: "https://api.cnyes.com",
	}
}

type cnyesNewsItem struct {
	NewsID    int64  `json:"newsId"`
	Title     string `json:"title"`
	Summary   string `json:"summary"`
	Source    string `json:"source"`
	PublishAt int64  `json:"publishAt"`
}

// GetMarketNews returns up to limit of the most recent TW-market news
// items (category tw_stock), newest first — the whole-market/macro news
// source for the TW daily-report's news summary, mirroring
// Finnhub.GetMarketNews's role for US. Summary is sometimes empty in the
// raw feed (live-verified 2026-07-28: null on some items, a real 1-2
// sentence teaser on others) — left as "" exactly like Yahoo.GetNews, so
// the prompt renderer's existing "no summary" skip applies unchanged. URL
// is built from newsId via cnyes's canonical article path since the list
// endpoint carries no direct link field of its own (live-verified to
// resolve).
func (c *Cnyes) GetMarketNews(limit int) ([]NewsItem, error) {
	url := fmt.Sprintf("%s/media/api/v1/newslist/category/tw_stock?limit=%d", c.baseURL, limit)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cnyes news: status %d", resp.StatusCode)
	}

	var result struct {
		Items struct {
			Data []cnyesNewsItem `json:"data"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var out []NewsItem
	for i, it := range result.Items.Data {
		if i >= limit {
			break
		}
		out = append(out, NewsItem{
			Headline:    it.Title,
			Summary:     it.Summary,
			Source:      it.Source,
			URL:         fmt.Sprintf("https://news.cnyes.com/news/id/%d", it.NewsID),
			PublishedAt: time.Unix(it.PublishAt, 0),
		})
	}
	return out, nil
}
