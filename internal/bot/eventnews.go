package bot

import (
	"time"

	"argus/internal/data"
)

// tickerNewsSlots is how many news items a per-ticker prompt section is
// given, and tickerNewsFetch how many are pulled to fill them. The gap
// between the two is the room the date filter and the dedupe need: asking
// for exactly five and then throwing three away leaves the model with two,
// so the fetch is widened rather than the filters loosened. Both filters run
// before the truncation, so the five that survive are the five freshest
// distinct stories. Shared by the event summary and the daily report /
// morning briefing (see service.NewsPicker) — a wasted slot costs the same
// wherever it happens.
const (
	tickerNewsSlots = 5
	tickerNewsFetch = 10
)

// tickerNewsMaxAge is how old a fetchStockData news item may be before it's
// dropped as stale — Phase 19 後續 PR3. Unlike a price event,
// fetchStockData's daily-report/recommend path has no event date to anchor
// service.FilterNewsNearDate's ±service.EventNewsWindow to, so filterStaleNews
// below just checks age against "now" instead, with a window widened to 3
// days (matching the /llm audit page's own isStale threshold,
// LlmRunsView.tsx) — tighter than that and today's genuinely slow-news
// tickers would come back empty rather than merely older. Live-verified
// against run id=8 (2026-08-26 US daily report): 50/239 items were older
// than this.
const tickerNewsMaxAge = 72 * time.Hour

// filterStaleNews drops items older than tickerNewsMaxAge as of asOf.
// Unlike service.FilterNewsNearDate's symmetric window, only a lower bound
// is needed here — news is never dated in the future. Same "undated item is
// dropped, not kept" rule.
func filterStaleNews(news []data.NewsItem, asOf time.Time) []data.NewsItem {
	var out []data.NewsItem
	for _, n := range news {
		if n.PublishedAt.IsZero() {
			continue
		}
		if asOf.Sub(n.PublishedAt) <= tickerNewsMaxAge {
			out = append(out, n)
		}
	}
	return out
}
