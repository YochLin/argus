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
	KeyUnknownCommand         Key = "unknown_command"
	KeyAddUsage               Key = "add_usage"
	KeyAddFailed              Key = "add_failed"
	KeyAddSuccess             Key = "add_success"
	KeyRemoveUsage            Key = "remove_usage"
	KeyRemoveFailed           Key = "remove_failed"
	KeyRemoveSuccess          Key = "remove_success"
	KeyQueryFailed            Key = "query_failed"
	KeyWatchlistEmptyHint     Key = "watchlist_empty_hint"
	KeyWatchlistTitle         Key = "watchlist_title"
	KeyWatchlistEmpty         Key = "watchlist_empty"
	KeyMarketStatusTitle      Key = "market_status_title"
	KeyQuoteUnavailable       Key = "quote_unavailable"
	KeyQuoteFailed            Key = "quote_failed"
	KeyAnalyzing              Key = "analyzing"
	KeyWatchlistQueryFailed   Key = "watchlist_query_failed"
	KeyLLMFailed              Key = "llm_failed"
	KeyNoRecommendations      Key = "no_recommendations"
	KeyRecommendationsTitle   Key = "recommendations_title"
	KeyMarketNewsSummaryTitle Key = "market_news_summary_title"
	KeyCheckUsage             Key = "check_usage"
	KeyAnalyzingTicker        Key = "analyzing_ticker"
	KeyCheckResultTitle       Key = "check_result_title"
	KeyFundamentalsUsage      Key = "fundamentals_usage"
	KeyFundamentalsNoKey      Key = "fundamentals_no_key"
	KeyFundamentalsFailed     Key = "fundamentals_failed"
	KeyFundamentalsTitle      Key = "fundamentals_title"
	KeyThinking               Key = "thinking"
	KeyChatFailed             Key = "chat_failed"
	KeyResetDone              Key = "reset_done"
	KeySignalAlertTitle       Key = "signal_alert_title"
	KeyDailyReportStart       Key = "daily_report_start"
	KeyNoRecommendationsToday Key = "no_recommendations_today"
	KeyTrackUsage             Key = "track_usage"
	KeyTrackTitle             Key = "track_title"
	KeyTrackEmpty             Key = "track_empty"
	KeyTrackLine              Key = "track_line"
	KeyTrackLineNoPrice       Key = "track_line_no_price"
	KeyTrackSummary           Key = "track_summary"
	KeyTrackLineVsSPY         Key = "track_line_vs_spy"
	KeyTrackAvgReturnLine     Key = "track_avg_return_line"
	KeyTrackBySourceHeader    Key = "track_by_source_header"
	KeyTrackBySourceLine      Key = "track_by_source_line"

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

	KeyCashUsage      Key = "cash_usage"
	KeyCashCurrent    Key = "cash_current"
	KeyCashNotSet     Key = "cash_not_set"
	KeyCashSetSuccess Key = "cash_set_success"
	KeyCashSetFailed  Key = "cash_set_failed"

	KeyStopLossAlertTitle     Key = "stop_loss_alert_title"
	KeyStopLossAlertLine      Key = "stop_loss_alert_line"
	KeyTrailingStopAlertTitle Key = "trailing_stop_alert_title"
	KeyTrailingStopAlertLine  Key = "trailing_stop_alert_line"

	KeyChatContextHeader       Key = "chat_context_header"
	KeyChatContextTickerNoData Key = "chat_context_ticker_no_data"
	KeyChatContextWatchLine    Key = "chat_context_watch_line"
	KeyChatContextPositionLine Key = "chat_context_position_line"
	KeyChatContextFooter       Key = "chat_context_footer"

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

	KeyCheckPromptIntro Key = "check_prompt_intro"
	KeyCheckPromptTask  Key = "check_prompt_task"

	KeyInsightPromptIntro       Key = "insight_prompt_intro"
	KeyInsightPositionValueLine Key = "insight_position_value_line"
	KeyInsightCashLine          Key = "insight_cash_line"
	KeyInsightPromptTask        Key = "insight_prompt_task"

	KeyStockHeader             Key = "stock_header"
	KeyPriceLine               Key = "price_line"
	KeyOHLLine                 Key = "ohl_line"
	KeyVolumeLine              Key = "volume_line"
	KeyQuoteTimeLine           Key = "quote_time_line"
	KeyNewsHeader              Key = "news_header"
	KeyNewsItem                Key = "news_item"
	KeyFundamentalsSummaryLine Key = "fundamentals_summary_line"
	KeyStatementSummaryLine    Key = "statement_summary_line"
	KeyPositionLine            Key = "position_line"
	KeyPrevRecLine             Key = "prev_rec_line"
	KeyEarningsLine            Key = "earnings_line"
	KeyScanHitLine             Key = "scan_hit_line"
	KeyTechnicalsSummaryLine   Key = "technicals_summary_line"
	KeyTechnicalsMALine        Key = "technicals_ma_line"
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
)
