package llm

import (
	"fmt"
	"strings"
	"time"

	"argus/internal/data"
	"argus/internal/i18n"
)

// buildExplorePrompt is Phase 2.6 解凍's two-stage LLM exploration prompt
// (see docs/phase-2.6-two-stage-llm-exploration.md): market news only (no
// per-ticker StockData — the model is nominating tickers, not analyzing
// ones already in hand) plus the exclude list (watchlist ∪ candidates ∪
// positions) so the model doesn't waste a nomination on something already
// covered. Reuses KeyNewsItem/KeyReasonMarker verbatim rather than minting
// exploration-specific duplicates of the same content shape, same
// convention as buildTradeReviewPrompt reusing KeyVsSPYLine/KeyThesisLine.
func buildExplorePrompt(lang i18n.Lang, marketNews []data.NewsItem, exclude []string) string {
	var sb strings.Builder
	sb.WriteString(i18n.T(lang, i18n.KeyExplorePromptIntro))
	for i, n := range marketNews {
		writeNewsItem(&sb, lang, i+1, n)
	}
	sb.WriteString("\n")
	sb.WriteString(i18n.T(lang, i18n.KeyExploreExcludeLine, strings.Join(exclude, ", ")))
	sb.WriteString(i18n.T(lang, i18n.KeyExploreTaskBlock, maxExploreNominations, i18n.T(lang, i18n.KeyExploreMarker), i18n.T(lang, i18n.KeyReasonMarker)))
	return sb.String()
}

// buildPodcastPrompt is the /podcast command's prompt (internal/bot/podcast.go):
// a one-shot call over a fetched podcast/video transcript, asking the model
// to pull out per-stock and macro market views while ignoring sponsor reads
// and unrelated chatter — the transcript itself does the filtering job a
// pre-processing step would otherwise need to do, since the model can judge
// "is this passage market-relevant" better than a regex ever could. title
// and url are the fetched page's own (webfetch.Article.Title, the pasted
// URL); transcript is webfetch.Article.Text, already capped by
// maxArticleRunes.
func buildPodcastPrompt(lang i18n.Lang, title, url, transcript string) string {
	var sb strings.Builder
	sb.WriteString(i18n.T(lang, i18n.KeyPodcastPromptIntro, title, url, transcript))
	sb.WriteString(i18n.T(lang, i18n.KeyPodcastTaskBlock,
		i18n.T(lang, i18n.KeyPodcastMarketMarker),
		i18n.T(lang, i18n.KeyPodcastStanceMarker),
		i18n.T(lang, i18n.KeyReasonMarker),
		i18n.T(lang, i18n.KeyPodcastDerivedMarker)))
	return sb.String()
}

// writeNewsItem renders one news item into sb: the numbered
// "[source] headline (date)" line plus, when the provider supplied one, its
// summary line. Every prompt that shows news goes through here, so what a
// news line carries stays identical across all of them — Phase 20 後續 PR2
// added the publish date in one place instead of to five drifting call
// sites (see newsDate for why the date had to be there at all).
func writeNewsItem(sb *strings.Builder, lang i18n.Lang, idx int, n data.NewsItem) {
	fmt.Fprint(sb, i18n.T(lang, i18n.KeyNewsItem, idx, n.Source, n.Headline, newsDate(n)))
	if n.Summary != "" {
		fmt.Fprint(sb, i18n.T(lang, i18n.KeyNewsSummaryLine, n.Summary))
	}
	if len(n.RelatedTickers) > 0 {
		fmt.Fprint(sb, i18n.T(lang, i18n.KeyNewsTickersLine, strings.Join(n.RelatedTickers, ", ")))
	}
}

// newsDate formats n's publish timestamp for a prompt news line, in CST
// (same convention as internal/mcptools' get_news) — "?" when the provider
// didn't supply one, which is honest about the gap rather than printing
// Go's zero time as if it were a real date. Phase 20 後續 PR2's finding: the
// news feed is ordered newest-first but a third of a ticker's top-5 was
// over two days old, and the prompts ask the model to infer a cause from
// news it had no way to date — so it hung last week's story on today's move.
func newsDate(n data.NewsItem) string {
	if n.PublishedAt.IsZero() {
		return "?"
	}
	return n.PublishedAt.In(newsDateZone).Format("2006-01-02")
}

// newsDateZone is CST/Taiwan time — the user's own timezone, so "today" in
// a news line means the same thing as "today" everywhere else the bot
// speaks. A fixed zone rather than a tzdata lookup, same reasoning as
// internal/scheduler's.
var newsDateZone = time.FixedZone("CST", 8*3600)

// pctFrom returns the percentage difference of price from ref (e.g. a 52-week
// high/low), or 0 if ref is unavailable (0) to avoid a divide-by-zero.
func pctFrom(price, ref float64) float64 {
	if ref == 0 {
		return 0
	}
	return (price - ref) / ref * 100
}
