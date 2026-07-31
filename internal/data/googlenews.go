package data

import (
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"argus/internal/market"
)

// GoogleNews is the free, keyless Google News RSS search client backing TW
// per-ticker news. Finnhub's /company-news is US-only (errTWNotSupported)
// and Yahoo's search API answers a .TW symbol with mostly English/wire
// coverage, so before this the TW half of GetNews had no Chinese-language
// source at all. Same "unofficial endpoint, could disappear without notice"
// risk CLAUDE.md already documents for Yahoo's chart API and cnyes.
type GoogleNews struct {
	client  *http.Client
	baseURL string // overridable in tests, defaults to the real endpoint

	// names resolves a TW ticker's Chinese short name for the search query
	// — see GetNews. Optional: nil (no FINMIND_TOKEN) degrades to searching
	// the bare ticker number rather than disabling the provider.
	names CompanyNameProvider
}

func NewGoogleNews(names CompanyNameProvider) *GoogleNews {
	return &GoogleNews{
		client:  &http.Client{Timeout: 10 * time.Second},
		baseURL: "https://news.google.com",
		names:   names,
	}
}

func (g *GoogleNews) Name() string { return "googlenews" }

// errGoogleNewsUnsupported covers the two Provider methods this source has
// no answer for. GoogleNews implements the full Provider interface only so
// it can sit in the Multi chain for GetNews; Multi's fallback loop moves
// past these two without a network round trip.
var errGoogleNewsUnsupported = errors.New("google news: news-only provider")

func (g *GoogleNews) GetQuote(string) (*Quote, error) { return nil, errGoogleNewsUnsupported }

func (g *GoogleNews) GetMarketMovers() ([]string, error) { return nil, errGoogleNewsUnsupported }

// errGoogleNewsNotTW mirrors Finnhub's errTWNotSupported with the markets
// swapped: this provider is wired in ahead of Yahoo, so a US ticker must
// fail immediately and let Multi fall through, leaving the pre-existing
// Finnhub-then-Yahoo US path untouched. Google News does carry US coverage,
// but its headlines-only items are strictly worse than Finnhub's
// summary-bearing /company-news, so there's nothing to gain by answering.
var errGoogleNewsNotTW = errors.New("google news: taiwan tickers only")

// googleNewsWindow is the search-query time window, matching Finnhub
// GetNews's 7-day /company-news lookback so both markets' per-ticker news
// covers the same span. Expressed as a Google News query *operator*
// (when:7d) rather than a request parameter — that's the only way the RSS
// search endpoint takes a window.
const googleNewsWindow = "when:7d"

type googleNewsFeed struct {
	Items []struct {
		Title   string `xml:"title"`
		Link    string `xml:"link"`
		PubDate string `xml:"pubDate"`
		Source  string `xml:"source"`
	} `xml:"channel>item"`
}

// GetNews returns up to limit recent Chinese-language news items for a TW
// ticker, newest first.
//
// The query is the company's Chinese short name when g.names resolves one
// ("台積電"), falling back to the bare ticker number. That fallback is
// materially worse, not just less pretty: a bare "2330" search matches
// price mentions ("台積電跌20元至2330") and social-media noise
// (facebook.com, 股市爆料同學會) alongside real coverage, which the name
// query doesn't.
//
// Summary is always left empty. Google News search items carry a
// <description> holding only an HTML <a> wrapper around the same headline
// — no teaser text — so filling Summary would just duplicate Headline. The
// existing "no summary" skip in the prompt renderer (same path Yahoo.GetNews
// and cnyes already rely on) applies unchanged.
func (g *GoogleNews) GetNews(ticker string, limit int) ([]NewsItem, error) {
	if market.Of(ticker) != market.TW {
		return nil, errGoogleNewsNotTW
	}

	items, err := g.fetch(g.query(ticker))
	if err != nil {
		return nil, err
	}

	var out []NewsItem
	for _, it := range items.Items {
		headline, source := splitGoogleNewsTitle(it.Title, it.Source)
		if headline == "" || it.Link == "" {
			continue
		}
		out = append(out, NewsItem{
			Headline:    headline,
			Source:      source,
			URL:         it.Link,
			PublishedAt: parseGoogleNewsDate(it.PubDate),
		})
	}

	// Sort before truncating, not after: this endpoint orders by relevance,
	// not recency (live-verified 2026-08-01 — a same-day story came back
	// last of five), so a plain head-of-list cut at limit can silently drop
	// today's news. Every other GetNews implementation hands callers
	// newest-first, and the prompt renderer's "recent news" framing assumes
	// it. Items with an unparseable pubDate sort last on their zero time
	// rather than jumping to the front.
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].PublishedAt.After(out[j].PublishedAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// query builds the search term for ticker — see GetNews for why the name
// lookup matters. A CompanyNameProvider error is not propagated: a missing
// name degrades the result quality but still returns real news, which beats
// failing the whole call over.
func (g *GoogleNews) query(ticker string) string {
	if g.names == nil {
		return ticker
	}
	name, err := g.names.GetCompanyName(ticker)
	if err != nil || name == "" {
		return ticker
	}
	return name
}

func (g *GoogleNews) fetch(query string) (*googleNewsFeed, error) {
	// hl/gl/ceid are what make this the Traditional-Chinese Taiwan edition
	// of Google News rather than the US English one — dropping them turns
	// the results back into the English coverage Yahoo already provides.
	params := url.Values{
		"q":    {query + " " + googleNewsWindow},
		"hl":   {"zh-TW"},
		"gl":   {"TW"},
		"ceid": {"TW:zh-Hant"},
	}
	req, err := http.NewRequest("GET", g.baseURL+"/rss/search?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google news: status %d", resp.StatusCode)
	}

	var feed googleNewsFeed
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		return nil, err
	}
	return &feed, nil
}

// splitGoogleNewsTitle strips the " - Publisher" suffix Google News appends
// to every headline, since the publisher is also carried in the item's own
// <source> element and repeating it inside Headline would waste prompt
// tokens on every single item. The suffix is only removed when it actually
// matches <source>; a headline that legitimately ends in " - something else"
// (or an item with no <source> at all) is left intact.
func splitGoogleNewsTitle(title, source string) (headline, publisher string) {
	title = strings.TrimSpace(title)
	source = strings.TrimSpace(source)
	if source != "" {
		if trimmed, ok := strings.CutSuffix(title, " - "+source); ok {
			title = strings.TrimSpace(trimmed)
		}
	}
	return title, source
}

// parseGoogleNewsDate returns the zero time for an unparseable pubDate
// instead of failing the item — same "0 fields mean no data" sentinel
// convention as Candle/Quote, and a news item with a missing timestamp is
// still worth showing.
func parseGoogleNewsDate(s string) time.Time {
	t, err := time.Parse(time.RFC1123, strings.TrimSpace(s))
	if err != nil {
		return time.Time{}
	}
	return t
}
