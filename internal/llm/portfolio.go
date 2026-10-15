package llm

import (
	"strings"

	"argus/internal/i18n"
)

// buildInsightPrompt is Phase 3.6's portfolio-level analysis prompt: every
// held position (via writeStockSection, the same per-ticker rendering
// /recommend and /check already use — quote, technicals, fundamentals,
// earnings, cost basis) followed by a portfolio-wide summary line and a task
// block that explicitly asks for concentration/thesis/rebalancing judgment
// rather than a repeat of the per-ticker analysis above it. cash is only
// rendered when haveCash is true (the user has run /cash at least once) —
// see PLAN.md's Phase 3.6 "現金水位" item: an unset cash balance should read
// as "no data," not silently as $0, which would misleadingly suggest 100%
// invested.
//
// One market per call (isTW says which, reusing KeyRecTWMarketNote to declare
// the whole prompt TWD-denominated), same split as buildWeeklyReviewPrompt:
// a mixed prompt summed TWD and USD position values into one meaningless
// total value and had the model comparing position sizes across currencies.
func buildInsightPrompt(lang i18n.Lang, positions []StockData, cash float64, haveCash bool, isTW bool) string {
	var sb strings.Builder
	sb.WriteString(i18n.T(lang, i18n.KeyInsightPromptIntro))
	if isTW {
		sb.WriteString(i18n.T(lang, i18n.KeyRecTWMarketNote))
	}

	var totalValue float64
	for _, s := range positions {
		if s.Quote != nil && s.Position != nil {
			totalValue += s.Quote.Price * s.Position.Shares
		}
		writeStockSection(&sb, lang, s)
	}

	sb.WriteString(i18n.T(lang, i18n.KeyInsightPositionValueLine, totalValue))
	if haveCash {
		sb.WriteString(i18n.T(lang, i18n.KeyInsightCashLine, cash, totalValue+cash))
	}

	sb.WriteString(i18n.T(lang, i18n.KeyInsightPromptTask))
	return sb.String()
}

// buildWeeklyReviewPrompt is Phase 3.6 PR2's Sunday weekly review prompt:
// the same per-position rendering and portfolio-value/cash lines as
// buildInsightPrompt, plus this week's /track summary (trackSummary, empty
// when there's no recommendation history yet — see bot.computeTrackData)
// folded into the same prompt so the model's portfolio judgment and its
// comment on recommendation accuracy come from a single coherent call
// rather than two.
//
// One market per call (positions/cash/trackSummary are all that market's, and
// isTW says which — reusing KeyRecTWMarketNote to declare the whole prompt
// TWD-denominated, same as buildRecommendationPrompt): a mixed prompt summed
// TWD and USD position values into one number and left the model comparing
// concentration across two currencies, which is what made the combined review
// unreadable.
func buildWeeklyReviewPrompt(lang i18n.Lang, positions []StockData, cash float64, haveCash bool, trackSummary string, isTW bool) string {
	var sb strings.Builder
	sb.WriteString(i18n.T(lang, i18n.KeyWeeklyReviewPromptIntro))
	if isTW {
		sb.WriteString(i18n.T(lang, i18n.KeyRecTWMarketNote))
	}

	var totalValue float64
	for _, s := range positions {
		if s.Quote != nil && s.Position != nil {
			totalValue += s.Quote.Price * s.Position.Shares
		}
		writeStockSection(&sb, lang, s)
	}

	sb.WriteString(i18n.T(lang, i18n.KeyInsightPositionValueLine, totalValue))
	if haveCash {
		sb.WriteString(i18n.T(lang, i18n.KeyInsightCashLine, cash, totalValue+cash))
	}

	if trackSummary != "" {
		sb.WriteString(i18n.T(lang, i18n.KeyWeeklyReviewTrackHeader))
		sb.WriteString(trackSummary)
	}

	sb.WriteString(i18n.T(lang, i18n.KeyWeeklyReviewPromptTask))
	return sb.String()
}
