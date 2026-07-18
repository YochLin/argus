// Package i18n holds every user/LLM-facing string in the project as a
// translation table, split into exactly two files: zh.go (Traditional
// Chinese, the original/default language) and en.go (English). Every Key
// declared below must have an entry in both files — see T.
package i18n

import "fmt"

// Lang identifies an output language. The zero value behaves as ZH so a
// zero-valued Lang (e.g. a struct field left unset by mistake) degrades to
// the original default instead of silently emitting untranslated keys.
type Lang string

const (
	ZH Lang = "zh"
	EN Lang = "en"
)

// Parse resolves a BOT_LANGUAGE env value to a Lang, defaulting to ZH for
// anything unrecognized (including empty) so existing setups with no
// BOT_LANGUAGE configured keep behaving exactly as before this option
// existed.
func Parse(s string) Lang {
	if Lang(s) == EN {
		return EN
	}
	return ZH
}

// Key identifies a single translatable message.
type Key string

// T looks up key in the table for lang and formats it with args via
// fmt.Sprintf. Omit args for a static message with no placeholders. If a
// key is missing from a table (a translation was never added), T returns
// the key itself — a visible, greppable marker instead of a blank message.
func T(lang Lang, key Key, args ...any) string {
	table := zhMessages
	if lang == EN {
		table = enMessages
	}
	format, ok := table[key]
	if !ok {
		return string(key)
	}
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}

// Keys used by internal/bot.
const (
	KeyUnknownCommand            Key = "unknown_command"
	KeyAddUsage                  Key = "add_usage"
	KeyAddFailed                 Key = "add_failed"
	KeyAddSuccess                Key = "add_success"
	KeyRemoveUsage               Key = "remove_usage"
	KeyRemoveFailed              Key = "remove_failed"
	KeyRemoveSuccess             Key = "remove_success"
	KeyQueryFailed               Key = "query_failed"
	KeyWatchlistEmptyHint        Key = "watchlist_empty_hint"
	KeyWatchlistTitle            Key = "watchlist_title"
	KeyWatchlistEmpty            Key = "watchlist_empty"
	KeyMarketStatusTitle         Key = "market_status_title"
	KeyQuoteUnavailable          Key = "quote_unavailable"
	KeyQuoteFailed               Key = "quote_failed"
	KeyAnalyzing                 Key = "analyzing"
	KeyWatchlistQueryFailed      Key = "watchlist_query_failed"
	KeyLLMFailed                 Key = "llm_failed"
	KeyNoRecommendations         Key = "no_recommendations"
	KeyRecommendationsTitle      Key = "recommendations_title"
	KeyRecWatchlistSectionTitle  Key = "rec_watchlist_section_title"
	KeyRecCandidatesSectionTitle Key = "rec_candidates_section_title"
	KeyMarketNewsSummaryTitle    Key = "market_news_summary_title"
	KeyCheckUsage                Key = "check_usage"
	KeyAnalyzingTicker           Key = "analyzing_ticker"
	KeyCheckResultTitle          Key = "check_result_title"
	KeyFundamentalsUsage         Key = "fundamentals_usage"
	KeyFundamentalsNoKey         Key = "fundamentals_no_key"
	KeyFundamentalsFailed        Key = "fundamentals_failed"
	KeyFundamentalsTitle         Key = "fundamentals_title"
	KeyThinking                  Key = "thinking"
	KeyChatFailed                Key = "chat_failed"
	KeyResetDone                 Key = "reset_done"
	KeySignalAlertTitle          Key = "signal_alert_title"
	KeyDailyReportStart          Key = "daily_report_start"
	KeyNoRecommendationsToday    Key = "no_recommendations_today"
	KeyTrackUsage                Key = "track_usage"
	KeyTrackTitle                Key = "track_title"
	KeyTrackEmpty                Key = "track_empty"
	KeyTrackLine                 Key = "track_line"
	KeyTrackLineNoPrice          Key = "track_line_no_price"
	KeyTrackSummary              Key = "track_summary"
	KeyTrackLineVsSPY            Key = "track_line_vs_spy"
	KeyTrackAvgReturnLine        Key = "track_avg_return_line"
	KeyTrackBySourceHeader       Key = "track_by_source_header"
	KeyTrackBySourceLine         Key = "track_by_source_line"

	KeyBuyUsage               Key = "buy_usage"
	KeyBuyFailed              Key = "buy_failed"
	KeyBuySuccess             Key = "buy_success"
	KeySellUsage              Key = "sell_usage"
	KeySellFailed             Key = "sell_failed"
	KeySellSuccess            Key = "sell_success"
	KeySellNoPosition         Key = "sell_no_position"
	KeySellInsufficientShares Key = "sell_insufficient_shares"
	KeyPortfolioTitle         Key = "portfolio_title"
	KeyPortfolioEmpty         Key = "portfolio_empty"
	KeyPortfolioLine          Key = "portfolio_line"
	KeyPortfolioSummary       Key = "portfolio_summary"
	KeyJobPanic               Key = "job_panic"

	KeyInsightResultTitle Key = "insight_result_title"

	KeyWeeklyReviewStart          Key = "weekly_review_start"
	KeyWeeklyReviewResultTitle    Key = "weekly_review_result_title"
	KeyWeeklyNetWorthLine         Key = "weekly_net_worth_line"
	KeyWeeklyNetWorthLineWithCash Key = "weekly_net_worth_line_with_cash"
	KeyWeeklyEarningsPreviewTitle Key = "weekly_earnings_preview_title"
	KeyWeeklyEarningsPreviewLine  Key = "weekly_earnings_preview_line"

	KeyMonthlyReportTitle         Key = "monthly_report_title"
	KeyMonthlyReportSparklineLine Key = "monthly_report_sparkline_line"
	KeyMonthlyReportChangeLine    Key = "monthly_report_change_line"
	KeyMonthlyReportDrawdownLine  Key = "monthly_report_drawdown_line"
	KeyMonthlyReportRealizedLine  Key = "monthly_report_realized_line"
	KeyMonthlyReportSPYLine       Key = "monthly_report_spy_line"
	KeyMonthlyReportTxCountLine   Key = "monthly_report_tx_count_line"
	KeyMonthlyReportCashLine      Key = "monthly_report_cash_line"

	KeyReviewUsage            Key = "review_usage"
	KeyReviewNoClosedTrade    Key = "review_no_closed_trade"
	KeyTradeReviewResultTitle Key = "trade_review_result_title"

	KeyCashUsage      Key = "cash_usage"
	KeyCashCurrent    Key = "cash_current"
	KeyCashNotSet     Key = "cash_not_set"
	KeyCashSetSuccess Key = "cash_set_success"
	KeyCashSetFailed  Key = "cash_set_failed"

	KeyStopLossAlertTitle       Key = "stop_loss_alert_title"
	KeyStopLossAlertLine        Key = "stop_loss_alert_line"
	KeyTrailingStopAlertTitle   Key = "trailing_stop_alert_title"
	KeyTrailingStopAlertLine    Key = "trailing_stop_alert_line"
	KeyTrailingStopAlertLineATR Key = "trailing_stop_alert_line_atr"

	KeyChatContextHeader       Key = "chat_context_header"
	KeyChatContextTickerNoData Key = "chat_context_ticker_no_data"
	KeyChatContextWatchLine    Key = "chat_context_watch_line"
	KeyChatContextPositionLine Key = "chat_context_position_line"
	KeyChatContextFooter       Key = "chat_context_footer"

	KeyFetchingArticle    Key = "fetching_article"
	KeyArticleFetchFailed Key = "article_fetch_failed"
	KeyArticleTaskBlock   Key = "article_task_block"

	KeyEarningsAlertTitle Key = "earnings_alert_title"
	KeyEarningsAlertLine  Key = "earnings_alert_line"

	KeyUniverseUsage         Key = "universe_usage"
	KeyUniverseSummary       Key = "universe_summary"
	KeyUniverseSourceLine    Key = "universe_source_line"
	KeyUniverseAddUsage      Key = "universe_add_usage"
	KeyUniverseAddSuccess    Key = "universe_add_success"
	KeyUniverseAddFailed     Key = "universe_add_failed"
	KeyUniverseRemoveUsage   Key = "universe_remove_usage"
	KeyUniverseRemoveSuccess Key = "universe_remove_success"
	KeyUniverseRemoveFailed  Key = "universe_remove_failed"
	KeyUniverseDelistedTitle Key = "universe_delisted_title"
	KeyUniverseDelistedLine  Key = "universe_delisted_line"

	KeyThesisUsage      Key = "thesis_usage"
	KeyThesisSetFailed  Key = "thesis_set_failed"
	KeyThesisSetSuccess Key = "thesis_set_success"
	KeyThesisNotSet     Key = "thesis_not_set"
	KeyThesisCurrent    Key = "thesis_current"
	KeyBuyThesisNudge   Key = "buy_thesis_nudge"

	// Phase 4 write-gating (db.PendingAction): a chat tool call (record_buy/
	// record_sell) only creates a proposal; these render the Telegram
	// confirm/reject message and the outcome once the user taps a button.
	KeyConfirmButton                Key = "confirm_button"
	KeyRejectButton                 Key = "reject_button"
	KeyPendingBuyConfirm            Key = "pending_buy_confirm"
	KeyPendingSellConfirm           Key = "pending_sell_confirm"
	KeyPendingActionRejected        Key = "pending_action_rejected"
	KeyPendingActionAlreadyResolved Key = "pending_action_already_resolved"
	KeyPendingActionExecFailed      Key = "pending_action_exec_failed"

	KeyQuoteLine Key = "quote_line"

	KeyValuationHeader     Key = "valuation_header"
	KeyPE                  Key = "pe"
	KeyPB                  Key = "pb"
	KeyPS                  Key = "ps"
	KeyMarketCap           Key = "market_cap"
	KeyBeta                Key = "beta"
	Key52Week              Key = "week52"
	KeyProfitabilityHeader Key = "profitability_header"
	KeyROE                 Key = "roe"
	KeyROA                 Key = "roa"
	KeyGrossMargin         Key = "gross_margin"
	KeyOperatingMargin     Key = "operating_margin"
	KeyNetMargin           Key = "net_margin"
	KeyFinStructureHeader  Key = "fin_structure_header"
	KeyDebtToEquity        Key = "debt_to_equity"
	KeyCurrentRatio        Key = "current_ratio"
	KeyQuickRatio          Key = "quick_ratio"
	KeyGrowthHeader        Key = "growth_header"
	KeyRevenueGrowth       Key = "revenue_growth"
	KeyEPSGrowth           Key = "eps_growth"
	KeyEPS                 Key = "eps"
	KeyBookValue           Key = "book_value"
	KeyDividendYield       Key = "dividend_yield"

	KeyStatementTitle        Key = "statement_title"
	KeyIncomeStatementHeader Key = "income_statement_header"
	KeyRevenue               Key = "revenue"
	KeyGrossProfit           Key = "gross_profit"
	KeyOperatingIncome       Key = "operating_income"
	KeyNetIncome             Key = "net_income"
	KeyDilutedEPS            Key = "diluted_eps"
	KeyBalanceSheetHeader    Key = "balance_sheet_header"
	KeyTotalAssets           Key = "total_assets"
	KeyTotalLiabilities      Key = "total_liabilities"
	KeyTotalEquity           Key = "total_equity"
	KeyCashFlowHeader        Key = "cash_flow_header"
	KeyOperatingCashFlow     Key = "operating_cash_flow"
	KeyCapEx                 Key = "capex"
	KeyFreeCashFlow          Key = "free_cash_flow"
)

// Keys used by internal/signals.
const (
	KeyPriceUp         Key = "price_up"
	KeyPriceDown       Key = "price_down"
	KeyPriceSignal     Key = "price_signal"
	KeyRSIOverbought   Key = "rsi_overbought"
	KeyRSIOversold     Key = "rsi_oversold"
	KeyMACDBullish     Key = "macd_bullish"
	KeyMACDBearish     Key = "macd_bearish"
	KeyMACDGoldenCross Key = "macd_golden_cross"
	KeyMACDDeathCross  Key = "macd_death_cross"
)

// Keys used by internal/llm (system prompts and prompt-template text sent
// to Claude, not just bot-facing copy — see CLAUDE.md's note on keeping the
// prompt language and parseRecommendations' marker in lockstep).
const (
	KeySystemPromptAnalyst Key = "system_prompt_analyst"
	KeySystemPromptChat    Key = "system_prompt_chat"

	KeyRecPromptIntro       Key = "rec_prompt_intro"
	KeyMarketRegimeHeader   Key = "market_regime_header"
	KeyMarketRegimeSPYLine  Key = "market_regime_spy_line"
	KeyMarketRegimeVIXLine  Key = "market_regime_vix_line"
	KeyRiskOn               Key = "risk_on"
	KeyRiskOff              Key = "risk_off"
	KeyVIXCalm              Key = "vix_calm"
	KeyVIXNormal            Key = "vix_normal"
	KeyVIXPanic             Key = "vix_panic"
	KeyRecMarketNewsHeader  Key = "rec_market_news_header"
	KeyRecWatchlistHeader   Key = "rec_watchlist_header"
	KeyRecNoWatchlist       Key = "rec_no_watchlist"
	KeyRecMoversHeader      Key = "rec_movers_header"
	KeyRecNoCandidates      Key = "rec_no_candidates"
	KeyRecMarketSummaryTask Key = "rec_market_summary_task"
	KeyRecTaskBlock         Key = "rec_task_block"
	KeyReasonMarker         Key = "reason_marker"
	KeyActionMarker         Key = "action_marker"
	KeyMarketSummaryMarker  Key = "market_summary_marker"

	KeyExplorePromptIntro Key = "explore_prompt_intro"
	KeyExploreExcludeLine Key = "explore_exclude_line"
	KeyExploreTaskBlock   Key = "explore_task_block"
	KeyExploreMarker      Key = "explore_marker"
	KeyExploreReasonLabel Key = "explore_reason_label"

	KeyCheckPromptIntro Key = "check_prompt_intro"
	KeyCheckPromptTask  Key = "check_prompt_task"

	KeyInsightPromptIntro       Key = "insight_prompt_intro"
	KeyInsightPositionValueLine Key = "insight_position_value_line"
	KeyInsightCashLine          Key = "insight_cash_line"
	KeyInsightPromptTask        Key = "insight_prompt_task"

	KeyWeeklyReviewPromptIntro Key = "weekly_review_prompt_intro"
	KeyWeeklyReviewTrackHeader Key = "weekly_review_track_header"
	KeyWeeklyReviewPromptTask  Key = "weekly_review_prompt_task"

	KeyTradeReviewPromptIntro Key = "trade_review_prompt_intro"
	KeyTradeReviewLegLine     Key = "trade_review_leg_line"
	KeyTradeReviewPnLLine     Key = "trade_review_pnl_line"
	KeyTradeReviewRangeLine   Key = "trade_review_range_line"
	KeyTradeReviewRecsHeader  Key = "trade_review_recs_header"
	KeyTradeReviewRecLine     Key = "trade_review_rec_line"
	KeyTradeReviewPromptTask  Key = "trade_review_prompt_task"
	KeyLessonMarker           Key = "lesson_marker"

	KeyPastLessonsHeader   Key = "past_lessons_header"
	KeyPastLessonLine      Key = "past_lesson_line"
	KeyRecentLessonsHeader Key = "recent_lessons_header"
	KeyRecentLessonLine    Key = "recent_lesson_line"

	KeyStockHeader             Key = "stock_header"
	KeyPriceLine               Key = "price_line"
	KeyOHLLine                 Key = "ohl_line"
	KeyVolumeLine              Key = "volume_line"
	KeyQuoteTimeLine           Key = "quote_time_line"
	KeyNewsHeader              Key = "news_header"
	KeyNewsItem                Key = "news_item"
	KeyFundamentalsSummaryLine Key = "fundamentals_summary_line"
	KeyAnalystRatingLine       Key = "analyst_rating_line"
	KeyAnalystRatingTrendLine  Key = "analyst_rating_trend_line"
	KeyStatementSummaryLine    Key = "statement_summary_line"
	KeyPositionLine            Key = "position_line"
	KeyPrevRecLine             Key = "prev_rec_line"
	KeyEarningsLine            Key = "earnings_line"
	KeyScanHitLine             Key = "scan_hit_line"
	KeyThesisLine              Key = "thesis_line"
	KeyVsSPYLine               Key = "vs_spy_line"
	KeyTechnicalsSummaryLine   Key = "technicals_summary_line"
	KeyTechnicalsMALine        Key = "technicals_ma_line"
	KeyVolumeRatioLine         Key = "volume_ratio_line"
	KeyATRLine                 Key = "atr_line"
	KeyBollingerLine           Key = "bollinger_line"
	KeyCandlesHeader           Key = "candles_header"
	KeyCandleLine              Key = "candle_line"
	KeyTrendBullish            Key = "trend_bullish"
	KeyTrendBearish            Key = "trend_bearish"
	KeyTrendUnknown            Key = "trend_unknown"
	KeyAboveMA                 Key = "above_ma"
	KeyBelowMA                 Key = "below_ma"
)

// Keys used by internal/mcptools (read-only MCP tool result/error text
// returned to the chat model — see PLAN.md's Phase 3.5). get_fundamentals/
// get_financial_statements deliberately reuse internal/bot's granular
// per-field keys (KeyValuationHeader, KeyPE, KeyStatementTitle, ... — see
// "Keys used by internal/bot" above) instead of inventing new ones: those
// already render the full struct field-by-field for /fundamentals, which is
// exactly what a full-fidelity on-demand tool result wants too. Only the
// genuinely new pieces (ticker header, no-data errors, history/news/
// earnings/movers formatting — none of which /fundamentals or the
// prompt-injection keys already cover) get their own keys here.
const (
	KeyMCPTickerHeader Key = "mcp_ticker_header"

	KeyMCPNoQuote Key = "mcp_no_quote"

	KeyMCPNoHistory     Key = "mcp_no_history"
	KeyMCPHistoryResult Key = "mcp_history_result"

	KeyMCPNoNews   Key = "mcp_no_news"
	KeyMCPNewsItem Key = "mcp_news_item"

	KeyMCPNoFundamentals        Key = "mcp_no_fundamentals"
	KeyMCPNoFinancialStatements Key = "mcp_no_financial_statements"

	KeyMCPNoEarnings   Key = "mcp_no_earnings"
	KeyMCPEarningsItem Key = "mcp_earnings_item"

	KeyMCPNoMovers     Key = "mcp_no_movers"
	KeyMCPMoversResult Key = "mcp_movers_result"

	KeyMCPRecentRecsTitle       Key = "mcp_recent_recs_title"
	KeyMCPRecentRecsTitleTicker Key = "mcp_recent_recs_title_ticker"
	KeyMCPRecentRecsTruncated   Key = "mcp_recent_recs_truncated"
	KeyMCPRecentRecLine         Key = "mcp_recent_rec_line"
	KeyMCPRecentRecLineNoPrice  Key = "mcp_recent_rec_line_no_price"
	KeyMCPRecentRecsEmptyTicker Key = "mcp_recent_recs_empty_ticker"

	// record_buy/record_sell (Phase 4 "寫入把關基建"'s first write tools
	// beyond the watchlist pilot) never write a transaction directly — they
	// create a db.PendingAction and report that back, since the confirm/
	// reject step happens over Telegram, which this MCP subprocess has no
	// access to (see internal/bot's pending-action handling).
	KeyMCPTradeInvalidInput   Key = "mcp_trade_invalid_input"
	KeyMCPTradeProposalFailed Key = "mcp_trade_proposal_failed"
	KeyMCPTradeProposalBuy    Key = "mcp_trade_proposal_buy"
	KeyMCPTradeProposalSell   Key = "mcp_trade_proposal_sell"
)
