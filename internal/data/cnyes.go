package data

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"argus/internal/market"
)

// Cnyes is the free, keyless 鉅亨網 (Anue) client backing TW's
// MarketNewsProvider (docs/... TW data-gap investigation, 2026-07-28) —
// Finnhub's /news?category=general has no TW equivalent, and this is an
// unofficial API with the same "could disappear without notice" risk
// CLAUDE.md already documents for Yahoo's chart API. Phase 19 後續 PR5-2
// additionally makes it a full Provider (GetNews) — see stockNews below.
type Cnyes struct {
	client  *http.Client
	baseURL string

	// stockNews* cache the per-ticker reverse index GetNews builds from a
	// few pages of the same category feed — see cachedStockNews. Guarded by
	// its own mutex since GetMarketNews (unrelated, single page, no cache)
	// shares the struct but not this state.
	stockNewsMu      sync.Mutex
	stockNewsCache   map[string][]NewsItem
	stockNewsCacheAt time.Time
}

func NewCnyes() *Cnyes {
	return &Cnyes{
		client:  &http.Client{Timeout: 10 * time.Second},
		baseURL: "https://api.cnyes.com",
	}
}

func (c *Cnyes) Name() string { return "cnyes" }

// errCnyesUnsupported covers the two Provider methods this source has no
// answer for — mirrors GoogleNews's errGoogleNewsUnsupported. Cnyes
// implements the full Provider interface only so it can sit in the Multi
// chain for GetNews.
var errCnyesUnsupported = errors.New("cnyes: news-only provider")

func (c *Cnyes) GetQuote(string) (*Quote, error) { return nil, errCnyesUnsupported }

func (c *Cnyes) GetMarketMovers() ([]string, error) { return nil, errCnyesUnsupported }

// errCnyesNotTW mirrors errGoogleNewsNotTW — Cnyes is wired in ahead of
// GoogleNews (Phase 19 後續 PR5-2: Finnhub → Cnyes → GoogleNews → Yahoo), so
// a US ticker must fail immediately and let Multi fall through.
var errCnyesNotTW = errors.New("cnyes: taiwan tickers only")

type cnyesNewsItem struct {
	NewsID    int64  `json:"newsId"`
	Title     string `json:"title"`
	Summary   string `json:"summary"`
	Content   string `json:"content"` // HTML entity-encoded article body
	Source    string `json:"source"`
	PublishAt int64  `json:"publishAt"`
	Market    []struct {
		Code string `json:"code"` // editor-assigned ticker, e.g. "2317" / "NVDA" — not full-text-matched
	} `json:"market"`
}

// cnyesSummaryFallbackMaxRunes bounds the Content-derived stand-in used when
// summary is empty (docs/phase-19-followup-news-quality.md §2.2) — most
// items already have a real summary, this only covers the ~7% that don't,
// and a prompt line should stay teaser-sized, not the full ~730-char body.
const cnyesSummaryFallbackMaxRunes = 150

var cnyesHTMLTagRe = regexp.MustCompile(`<[^>]*>`)

// stripCnyesHTML strips the simple <p>/<br>/<a>-only markup cnyes's content
// field carries and unescapes HTML entities. A regexp is enough for this —
// internal/webfetch's extractText does a full document walk for arbitrary
// pages, overkill for one article field (see the followup doc §2.2 for why
// that private function isn't reused/exported here).
func stripCnyesHTML(s string) string {
	return strings.TrimSpace(cnyesHTMLTagRe.ReplaceAllString(html.UnescapeString(s), ""))
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// mapCnyesItem converts one raw feed item to NewsItem, shared by
// GetMarketNews and the per-ticker stock-news index below.
func mapCnyesItem(it cnyesNewsItem) NewsItem {
	summary := it.Summary
	if summary == "" && it.Content != "" {
		summary = truncateRunes(stripCnyesHTML(it.Content), cnyesSummaryFallbackMaxRunes)
	}
	var tickers []string
	for _, mk := range it.Market {
		if mk.Code != "" {
			tickers = append(tickers, mk.Code)
		}
	}
	return NewsItem{
		Headline:       it.Title,
		Summary:        summary,
		Source:         it.Source,
		URL:            fmt.Sprintf("https://news.cnyes.com/news/id/%d", it.NewsID),
		PublishedAt:    time.Unix(it.PublishAt, 0),
		RelatedTickers: tickers,
	}
}

// fetchCnyesPage fetches one page of the tw_stock category feed.
func (c *Cnyes) fetchCnyesPage(limit, page int) ([]cnyesNewsItem, error) {
	url := fmt.Sprintf("%s/media/api/v1/newslist/category/tw_stock?limit=%d&page=%d", c.baseURL, limit, page)
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
	return result.Items.Data, nil
}

// GetMarketNews returns up to limit of the most recent TW-market news
// items (category tw_stock), newest first — the whole-market/macro news
// source for the TW daily-report's news summary, mirroring
// Finnhub.GetMarketNews's role for US. Summary is sometimes empty in the
// raw feed (falls back to a truncated Content — see mapCnyesItem).
func (c *Cnyes) GetMarketNews(limit int) ([]NewsItem, error) {
	items, err := c.fetchCnyesPage(limit, 1)
	if err != nil {
		return nil, err
	}
	var out []NewsItem
	for i, it := range items {
		if i >= limit {
			break
		}
		out = append(out, mapCnyesItem(it))
	}
	return out, nil
}

// cnyesStockNewsPages/PerPage/CacheTTL are Phase 19 後續 PR5-2's live-verified
// values (docs/phase-19-followup-news-quality.md §1.1/§3.1): 3 pages of 30
// covers ~24h and tagged 121 distinct TW tickers in testing; 15 minutes is
// short enough that a daily-report/morning-briefing/recommend run within the
// same window shares one fetch, long enough that news isn't re-pulled on
// every call (it doesn't update minute-to-minute anyway).
const (
	cnyesStockNewsPages    = 3
	cnyesStockNewsPerPage  = 30
	cnyesStockNewsCacheTTL = 15 * time.Minute
)

// fetchStockNews pulls cnyesStockNewsPages pages of the category feed and
// builds a ticker -> items reverse index off each item's editor-assigned
// market[] tags — the "one endpoint, dispatch to many tickers" side of the
// Provider.GetNews per-ticker interface mismatch (see followup doc §3.1).
func (c *Cnyes) fetchStockNews() (map[string][]NewsItem, error) {
	out := make(map[string][]NewsItem)
	for page := 1; page <= cnyesStockNewsPages; page++ {
		items, err := c.fetchCnyesPage(cnyesStockNewsPerPage, page)
		if err != nil {
			return nil, err
		}
		for _, it := range items {
			news := mapCnyesItem(it)
			for _, ticker := range news.RelatedTickers {
				out[ticker] = append(out[ticker], news)
			}
		}
	}
	return out, nil
}

// cachedStockNews returns the process-lifetime cache, refetching on a miss/
// expiry. A fetch error serves stale data instead of propagating when
// something is already cached (same shape as tw_earnings_call.go's
// cachedYahooEarningsCalls) — a transient cnyes hiccup shouldn't blank out
// news that was fine a few minutes ago. On a true first-fetch failure (no
// cache to fall back on) the error propagates so Multi.GetNews falls through
// to GoogleNews/Yahoo instead of silently going quiet.
func (c *Cnyes) cachedStockNews() (map[string][]NewsItem, error) {
	c.stockNewsMu.Lock()
	defer c.stockNewsMu.Unlock()
	if c.stockNewsCache != nil && time.Since(c.stockNewsCacheAt) < cnyesStockNewsCacheTTL {
		return c.stockNewsCache, nil
	}
	m, err := c.fetchStockNews()
	if err != nil {
		if c.stockNewsCache != nil {
			return c.stockNewsCache, nil
		}
		return nil, err
	}
	c.stockNewsCache = m
	c.stockNewsCacheAt = time.Now()
	return m, nil
}

// GetNews returns up to limit recent TW-market news items tagged with
// ticker, newest first, via the category-feed reverse index (see
// fetchStockNews) — not a keyword search, so it doesn't have Google News's
// full-text-mismatch problem (docs/phase-19-followup-news-quality.md §3).
//
// A ticker absent from the cached pages returns (nil, nil), not an error:
// per docs/phase-19-followup-news-quality.md §5.1 (confirmed with the user
// 2026-09-02), an untagged ticker is treated as "no TW-market news today"
// rather than falling back to GoogleNews — that fallback's headlines are
// full-text matches on the company name, which a live audit found 40%
// unrelated to the ticker (PR4). Because Multi.GetNews stops at the first
// nil error, this deliberately ends the chain here for TW tickers.
func (c *Cnyes) GetNews(ticker string, limit int) ([]NewsItem, error) {
	if market.Of(ticker) != market.TW {
		return nil, errCnyesNotTW
	}
	m, err := c.cachedStockNews()
	if err != nil {
		return nil, err
	}
	items := m[ticker]
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}
