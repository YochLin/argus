package llm

import (
	"strings"

	"argus/internal/data"
	"argus/internal/i18n"
	"argus/internal/market"
)

type Recommendation struct {
	Ticker string
	Action string // BUY / SELL / HOLD ("" if the model omitted the action line)
	Reason string
}

func buildRecommendationPrompt(lang i18n.Lang, watchlist []StockData, candidates []StockData, marketNews []data.NewsItem, market *MarketContext, recentLessons []PastLesson, isTW bool) string {
	var sb strings.Builder

	if isTW {
		sb.WriteString(i18n.T(lang, i18n.KeyRecPromptIntroTW))
		sb.WriteString(i18n.T(lang, i18n.KeyRecTWMarketNote))
	} else {
		sb.WriteString(i18n.T(lang, i18n.KeyRecPromptIntro))
	}
	writeMarketContext(&sb, lang, market)
	writeRecentLessons(&sb, lang, recentLessons)

	if len(marketNews) > 0 {
		sb.WriteString(i18n.T(lang, i18n.KeyRecMarketNewsHeader))
		for i, n := range marketNews {
			writeNewsItem(&sb, lang, i+1, n)
		}
		sb.WriteString("\n")
	}

	sb.WriteString(i18n.T(lang, i18n.KeyRecWatchlistHeader))

	if len(watchlist) == 0 {
		sb.WriteString(i18n.T(lang, i18n.KeyRecNoWatchlist))
	} else {
		for _, s := range watchlist {
			writeStockSection(&sb, lang, s)
		}
	}

	sb.WriteString(i18n.T(lang, i18n.KeyRecMoversHeader))
	if len(candidates) == 0 {
		sb.WriteString(i18n.T(lang, i18n.KeyRecNoCandidates))
	} else {
		for _, s := range candidates {
			writeStockSection(&sb, lang, s)
		}
	}

	if len(marketNews) > 0 {
		sb.WriteString(i18n.T(lang, i18n.KeyRecMarketSummaryTask, i18n.T(lang, i18n.KeyMarketSummaryMarker)))
	}

	action := i18n.T(lang, i18n.KeyActionMarker)
	reason := i18n.T(lang, i18n.KeyReasonMarker)
	sb.WriteString(i18n.T(lang, i18n.KeyRecTaskBlock, action, reason, action, reason))
	sb.WriteString(i18n.T(lang, i18n.KeyTechGuidanceBlock))
	return sb.String()
}

func buildCheckPrompt(lang i18n.Lang, s StockData) string {
	var sb strings.Builder
	if s.Quote != nil && market.Of(s.Quote.Ticker) == market.TW {
		sb.WriteString(i18n.T(lang, i18n.KeyCheckPromptIntroTW))
	} else {
		sb.WriteString(i18n.T(lang, i18n.KeyCheckPromptIntro))
	}
	writeStockSection(&sb, lang, s)
	sb.WriteString(i18n.T(lang, i18n.KeyCheckPromptTask))
	sb.WriteString(i18n.T(lang, i18n.KeyTechGuidanceBlock))
	return sb.String()
}
