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
	KeyUnknownCommand        Key = "unknown_command"
	KeyAddUsage              Key = "add_usage"
	KeyAddFailed             Key = "add_failed"
	KeyAddSuccess            Key = "add_success"
	KeyRemoveUsage           Key = "remove_usage"
	KeyRemoveFailed          Key = "remove_failed"
	KeyRemoveSuccess         Key = "remove_success"
	KeyQueryFailed           Key = "query_failed"
	KeyWatchlistEmptyHint    Key = "watchlist_empty_hint"
	KeyWatchlistTitle        Key = "watchlist_title"
	KeyWatchlistEmpty        Key = "watchlist_empty"
	KeyMarketStatusTitle     Key = "market_status_title"
	KeyQuoteUnavailable      Key = "quote_unavailable"
	KeyQuoteFailed           Key = "quote_failed"
	KeyAnalyzing             Key = "analyzing"
	KeyWatchlistQueryFailed  Key = "watchlist_query_failed"
	KeyLLMFailed             Key = "llm_failed"
	KeyNoRecommendations     Key = "no_recommendations"
	KeyRecommendationsTitle  Key = "recommendations_title"
	KeyCheckUsage            Key = "check_usage"
	KeyAnalyzingTicker       Key = "analyzing_ticker"
	KeyCheckResultTitle      Key = "check_result_title"
	KeyFundamentalsUsage     Key = "fundamentals_usage"
	KeyFundamentalsNoKey     Key = "fundamentals_no_key"
	KeyFundamentalsFailed    Key = "fundamentals_failed"
	KeyFundamentalsTitle     Key = "fundamentals_title"
	KeyThinking              Key = "thinking"
	KeyChatFailed            Key = "chat_failed"
	KeyResetDone             Key = "reset_done"
	KeySignalAlertTitle      Key = "signal_alert_title"
	KeyDailyReportStart      Key = "daily_report_start"
	KeyNoRecommendationsToday Key = "no_recommendations_today"
	KeyTrackUsage            Key = "track_usage"
	KeyTrackTitle            Key = "track_title"
	KeyTrackEmpty            Key = "track_empty"
	KeyTrackLine             Key = "track_line"
	KeyTrackLineNoPrice      Key = "track_line_no_price"
	KeyTrackSummary          Key = "track_summary"

	KeyBuyUsage                Key = "buy_usage"
	KeyBuyFailed               Key = "buy_failed"
	KeyBuySuccess              Key = "buy_success"
	KeySellUsage               Key = "sell_usage"
	KeySellFailed              Key = "sell_failed"
	KeySellSuccess             Key = "sell_success"
	KeySellNoPosition          Key = "sell_no_position"
	KeySellInsufficientShares  Key = "sell_insufficient_shares"
	KeyPortfolioTitle          Key = "portfolio_title"
	KeyPortfolioEmpty          Key = "portfolio_empty"
	KeyPortfolioLine           Key = "portfolio_line"
	KeyPortfolioSummary        Key = "portfolio_summary"
	KeyJobPanic                Key = "job_panic"

	KeyChatContextHeader       Key = "chat_context_header"
	KeyChatContextTickerNoData Key = "chat_context_ticker_no_data"
	KeyChatContextWatchLine    Key = "chat_context_watch_line"
	KeyChatContextPositionLine Key = "chat_context_position_line"
	KeyChatContextFooter       Key = "chat_context_footer"

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

	KeyStatementTitle       Key = "statement_title"
	KeyIncomeStatementHeader Key = "income_statement_header"
	KeyRevenue              Key = "revenue"
	KeyGrossProfit          Key = "gross_profit"
	KeyOperatingIncome      Key = "operating_income"
	KeyNetIncome            Key = "net_income"
	KeyDilutedEPS           Key = "diluted_eps"
	KeyBalanceSheetHeader   Key = "balance_sheet_header"
	KeyTotalAssets          Key = "total_assets"
	KeyTotalLiabilities     Key = "total_liabilities"
	KeyTotalEquity          Key = "total_equity"
	KeyCashFlowHeader       Key = "cash_flow_header"
	KeyOperatingCashFlow    Key = "operating_cash_flow"
	KeyCapEx                Key = "capex"
	KeyFreeCashFlow         Key = "free_cash_flow"
)

// Keys used by internal/signals.
const (
	KeyPriceUp      Key = "price_up"
	KeyPriceDown    Key = "price_down"
	KeyPriceSignal  Key = "price_signal"
	KeyRSIOverbought Key = "rsi_overbought"
	KeyRSIOversold  Key = "rsi_oversold"
	KeyMACDBullish  Key = "macd_bullish"
	KeyMACDBearish  Key = "macd_bearish"
	KeyMACDGoldenCross Key = "macd_golden_cross"
	KeyMACDDeathCross  Key = "macd_death_cross"
)

// Keys used by internal/llm (system prompts and prompt-template text sent
// to Claude, not just bot-facing copy — see CLAUDE.md's note on keeping the
// prompt language and parseRecommendations' marker in lockstep).
const (
	KeySystemPromptAnalyst Key = "system_prompt_analyst"
	KeySystemPromptChat    Key = "system_prompt_chat"

	KeyRecPromptIntro     Key = "rec_prompt_intro"
	KeyRecWatchlistHeader Key = "rec_watchlist_header"
	KeyRecNoWatchlist     Key = "rec_no_watchlist"
	KeyRecMoversHeader    Key = "rec_movers_header"
	KeyRecNoCandidates    Key = "rec_no_candidates"
	KeyRecTaskBlock       Key = "rec_task_block"
	KeyReasonMarker       Key = "reason_marker"
	KeyActionMarker       Key = "action_marker"

	KeyCheckPromptIntro Key = "check_prompt_intro"
	KeyCheckPromptTask  Key = "check_prompt_task"

	KeyStockHeader              Key = "stock_header"
	KeyPriceLine                Key = "price_line"
	KeyOHLLine                  Key = "ohl_line"
	KeyVolumeLine               Key = "volume_line"
	KeyQuoteTimeLine             Key = "quote_time_line"
	KeyNewsHeader               Key = "news_header"
	KeyNewsItem                 Key = "news_item"
	KeyFundamentalsSummaryLine  Key = "fundamentals_summary_line"
	KeyStatementSummaryLine     Key = "statement_summary_line"
	KeyPositionLine             Key = "position_line"
)
