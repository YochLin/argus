package i18n

// enMessages is the English message table. Every format verb here must
// appear in the same order as its zh.go counterpart — see the note there.
var enMessages = map[Key]string{
	KeyUnknownCommand:         "Unknown command. Available commands:\n/add <ticker>\n/remove <ticker>\n/list\n/status [ticker]\n/recommend\n/check <ticker>\n/track [days]\n/buy <ticker> <shares> <price> [fee]\n/sell <ticker> <shares> <price> [fee]\n/portfolio\n/dailyreport\n/fundamentals <ticker>\n/universe [add|remove] <ticker>\n/reset\n\nSend a plain message (no leading /) to chat freely.",
	KeyAddUsage:               "Usage: /add <ticker>, e.g. /add AAPL",
	KeyAddFailed:              "Failed to add: %v",
	KeyAddSuccess:             "Added *%s* to your watchlist.",
	KeyRemoveUsage:            "Usage: /remove <ticker>, e.g. /remove AAPL",
	KeyRemoveFailed:           "Failed to remove: %v",
	KeyRemoveSuccess:          "Removed *%s* from your watchlist.",
	KeyQueryFailed:            "Query failed: %v",
	KeyWatchlistEmptyHint:     "Your watchlist is empty. Use /add <ticker> to add one.",
	KeyWatchlistTitle:         "📋 *Watchlist*\n",
	KeyWatchlistEmpty:         "Your watchlist is empty.",
	KeyMarketStatusTitle:      "📊 *Market Status*\n\n",
	KeyQuoteUnavailable:       "*%s*: quote unavailable\n",
	KeyQuoteFailed:            "Failed to get %s quote: %v",
	KeyAnalyzing:              "Analyzing, please wait...",
	KeyWatchlistQueryFailed:   "Failed to query watchlist: %v",
	KeyLLMFailed:              "LLM analysis failed: %v",
	KeyNoRecommendations:      "The LLM returned no recommendations — try again later.",
	KeyRecommendationsTitle:   "🤖 *Today's Recommendations*\n\n",
	KeyMarketNewsSummaryTitle: "📰 *Market News Summary*\n\n",
	KeyCheckUsage:             "Usage: /check <ticker>, e.g. /check TSLA",
	KeyAnalyzingTicker:        "Analyzing, please wait... (%s)",
	KeyCheckResultTitle:       "📈 *%s Instant Analysis*\n\n%s",
	KeyFundamentalsUsage:      "Usage: /fundamentals <ticker>, e.g. /fundamentals AAPL",
	KeyFundamentalsNoKey:      "FINNHUB_API_KEY isn't configured, so fundamentals data is unavailable.",
	KeyFundamentalsFailed:     "Failed to get %s fundamentals: %v",
	KeyFundamentalsTitle:      "📊 *%s Fundamentals*\n\n",
	KeyThinking:               "Thinking, please wait...",
	KeyChatFailed:             "Chat failed: %v",
	KeyResetDone:              "Conversation memory has been reset.",
	KeySignalAlertTitle:       "🚨 *Market Signal Alert*\n\n",
	KeyDailyReportStart:       "📅 *Generating daily report...*",
	KeyNoRecommendationsToday: "No notable recommendations today.",
	KeyTrackUsage:             "Usage: /track [days], e.g. /track 14 (default 7 days, max 90)",
	KeyTrackTitle:             "📊 *Recommendation Review (past %d days)*\n\n",
	KeyTrackEmpty:             "No recommendations recorded in the past %d days.",
	KeyTrackLine:              "• %s *%s* %s  $%.2f → $%.2f (%+.2f%%) %s\n",
	KeyTrackLineVsSPY:         "• %s *%s* %s  $%.2f → $%.2f (%+.2f%% vs SPY %+.2f%%) %s\n",
	KeyTrackLineNoPrice:       "• %s *%s* %s — no price recorded to compare against\n",
	KeyTrackSummary:           "\nHit rate: %d/%d (%.0f%%)\n(When same-period SPY data is available, BUY must beat SPY and SELL must underperform SPY to count as a hit; without SPY data it falls back to absolute direction; HOLD and rows without price data are excluded)",
	KeyTrackAvgReturnLine:     "Avg return: BUY %+.2f%% (%d calls) | SELL %+.2f%% (%d calls)\n",
	KeyTrackBySourceHeader:    "\nBy source:\n",
	KeyTrackBySourceLine:      "- %s: %d/%d (%.0f%%)\n",

	KeyBuyUsage:               "Usage: /buy <ticker> <shares> <price> [fee] [date], e.g. /buy AAPL 10 205.5 or /buy AAPL 10 205.5 1.5 2026-01-15 (backdate a historical trade)",
	KeyBuyFailed:              "Failed to record buy: %v",
	KeyBuySuccess:             "✅ Recorded BUY of *%s* %g shares @ $%.2f (fee $%.2f)\nPosition now: %g shares, avg cost $%.2f",
	KeySellUsage:              "Usage: /sell <ticker> <shares> <price> [fee] [date], e.g. /sell AAPL 5 210 or /sell AAPL 5 210 1 2026-01-15 (backdate a historical trade)",
	KeySellFailed:             "Failed to record sell: %v",
	KeySellSuccess:            "✅ Recorded SELL of *%s* %g shares @ $%.2f (fee $%.2f)\nRealized P&L: %+.2f\nRemaining position: %g shares",
	KeySellNoPosition:         "No position in *%s* to sell.",
	KeySellInsufficientShares: "Not enough shares of *%s* to sell that many.",
	KeyPortfolioTitle:         "💼 *Portfolio Overview*\n\n",
	KeyPortfolioEmpty:         "No positions yet. Use /buy <ticker> <shares> <price> to record a purchase.",
	KeyPortfolioLine:          "*%s*  %g shares @ avg $%.2f\nPrice $%.2f  Value $%.2f  Unrealized %+.2f (%+.2f%%)\n\n",
	KeyPortfolioSummary:       "—\nTotal market value: $%.2f\nCumulative realized P&L: %+.2f",
	KeyJobPanic:               "⚠️ *%s* hit an unexpected error and aborted: %v",

	KeyStopLossAlertTitle:     "🛑 *Stop-Loss Alert*\n\n",
	KeyStopLossAlertLine:      "• *%s* cost $%.2f → price $%.2f, unrealized loss %.1f%% — stop-loss threshold hit\n",
	KeyTrailingStopAlertTitle: "📉 *Trailing-Stop Alert*\n\n",
	KeyTrailingStopAlertLine:  "• *%s* peak close since entry $%.2f → price $%.2f, down %.1f%% from peak — trailing-stop threshold hit\n",

	KeyChatContextHeader:       "[Background info, not real-time, for reference only]\n",
	KeyChatContextTickerNoData: "- %s: no closing data yet\n",
	KeyChatContextWatchLine:    "- %s: %s close $%.2f (%+.2f%%)\n",
	KeyChatContextPositionLine: "- %s: %s close $%.2f (%+.2f%%) | holding %g shares, avg cost $%.2f, unrealized %+.2f%%\n",
	KeyChatContextFooter:       "[End of background info — reply to the message below]\n\n",

	KeyEarningsAlertTitle: "📅 *Earnings Reminder*\n\n",
	KeyEarningsAlertLine:  "• *%s* reports earnings on %s (in %d days)\n",

	KeyUniverseUsage:         "Usage: /universe [add|remove] TICKER\nNo arguments shows a summary of the scan pool.",
	KeyUniverseSummary:       "*Scan Universe*\nTotal: %d tickers\n",
	KeyUniverseSourceLine:    "- %s: %d\n",
	KeyUniverseAddUsage:      "Usage: /universe add TICKER",
	KeyUniverseAddSuccess:    "%s added to the scan universe.",
	KeyUniverseAddFailed:     "Failed to add %s: %v",
	KeyUniverseRemoveUsage:   "Usage: /universe remove TICKER",
	KeyUniverseRemoveSuccess: "%s removed from the scan universe.",
	KeyUniverseRemoveFailed:  "Failed to remove %s: %v",

	KeyQuoteLine: "*%s*  $%.2f  %s%.2f%%\nOpen: $%.2f  High: $%.2f  Low: $%.2f",

	KeyValuationHeader:     "*Valuation*\n",
	KeyPE:                  "• P/E: %.1f\n",
	KeyPB:                  "• P/B: %.1f\n",
	KeyPS:                  "• P/S: %.1f\n",
	KeyMarketCap:           "• Market Cap: $%sM\n",
	KeyBeta:                "• Beta: %.2f\n",
	Key52Week:              "• 52-Week High/Low: $%.2f / $%.2f\n\n",
	KeyProfitabilityHeader: "*Profitability*\n",
	KeyROE:                 "• ROE: %.1f%%\n",
	KeyROA:                 "• ROA: %.1f%%\n",
	KeyGrossMargin:         "• Gross Margin: %.1f%%\n",
	KeyOperatingMargin:     "• Operating Margin: %.1f%%\n",
	KeyNetMargin:           "• Net Margin: %.1f%%\n\n",
	KeyFinStructureHeader:  "*Financial Structure*\n",
	KeyDebtToEquity:        "• Debt/Equity: %.2f\n",
	KeyCurrentRatio:        "• Current Ratio: %.2f\n",
	KeyQuickRatio:          "• Quick Ratio: %.2f\n\n",
	KeyGrowthHeader:        "*Growth / Shareholder Returns*\n",
	KeyRevenueGrowth:       "• Revenue YoY: %.1f%%\n",
	KeyEPSGrowth:           "• EPS YoY: %.1f%%\n",
	KeyEPS:                 "• EPS (TTM): %.2f\n",
	KeyBookValue:           "• Book Value/Share: %.2f\n",
	KeyDividendYield:       "• Dividend Yield: %.2f%%",

	KeyStatementTitle:        "*Latest Filing (%s, FY%d, as of %s)*\n\n",
	KeyIncomeStatementHeader: "Income Statement\n",
	KeyRevenue:               "• Revenue: $%sM\n",
	KeyGrossProfit:           "• Gross Profit: $%sM\n",
	KeyOperatingIncome:       "• Operating Income: $%sM\n",
	KeyNetIncome:             "• Net Income: $%sM\n",
	KeyDilutedEPS:            "• Diluted EPS: $%.2f\n\n",
	KeyBalanceSheetHeader:    "Balance Sheet\n",
	KeyTotalAssets:           "• Total Assets: $%sM\n",
	KeyTotalLiabilities:      "• Total Liabilities: $%sM\n",
	KeyTotalEquity:           "• Total Equity: $%sM\n\n",
	KeyCashFlowHeader:        "Cash Flow Statement\n",
	KeyOperatingCashFlow:     "• Operating Cash Flow: $%sM\n",
	KeyCapEx:                 "• CapEx: $%sM\n",
	KeyFreeCashFlow:          "• Free Cash Flow: $%sM",

	KeyPriceUp:         "up",
	KeyPriceDown:       "down",
	KeyPriceSignal:     "%s %s %.2f%% (price $%.2f)",
	KeyRSIOverbought:   "%s RSI overbought (%.1f) — a pullback is possible",
	KeyRSIOversold:     "%s RSI oversold (%.1f) — a bounce is possible",
	KeyMACDBullish:     "%s MACD showing bullish momentum (MACD %.2f > signal %.2f)",
	KeyMACDBearish:     "%s MACD showing bearish momentum (MACD %.2f < signal %.2f)",
	KeyMACDGoldenCross: "%s MACD golden cross (MACD %.2f crossed above signal %.2f) — trend turning bullish",
	KeyMACDDeathCross:  "%s MACD death cross (MACD %.2f crossed below signal %.2f) — trend turning bearish",

	KeySystemPromptAnalyst: "You are a professional US equities analyst whose only job is to give a written analysis based on the market data the user provides. You have no tools available and need none; reply strictly according to the format the user's message requests, with no content unrelated to the analysis.",
	KeySystemPromptChat:    "You are the user's personal assistant. Talk naturally in English and remember what was said earlier in this conversation. You have no tools available and need none.",

	KeyRecPromptIntro:      "You are a US equities analyst. Based on the market data below, give today's action recommendations.\n\n",
	KeyRecMarketNewsHeader: "## Broad Market News\n\n",
	KeyRecWatchlistHeader:  "## My Watchlist\n\n",
	KeyRecNoWatchlist:      "(No watchlist tickers)\n\n",
	KeyRecMoversHeader:     "## Broad Market Movers\n\n",
	KeyRecNoCandidates:     "(No candidate tickers)\n\n",
	KeyRecMarketSummaryTask: `
Before the per-ticker analysis below, first output a block starting with the exact line
%s
followed by 3–5 bullet points summarizing the market news above, noting which sectors or
tickers might benefit or suffer (under 150 words total). Then continue with the per-ticker
blocks.
`,
	KeyRecTaskBlock: `
## Task

Every watchlist ticker must get an explicit call, and the action must be exactly one of BUY, SELL, or HOLD;
from the broad market movers, only list the ones you actually like as buys (at most 3, action BUY) — none is fine.

When weighing each ticker's news, note whether each headline reads as bullish or bearish and fold that into your reasoning.

If a ticker's section below shows a "Previous call" and today's action differs from it, your reasoning must
explicitly say what changed (new information, a technical signal reversal, or the original thesis no longer
holding) — don't just restate the same conclusion in different words.

Strictly follow this output structure, one block per ticker, with no extra text:

[TICKER: AAPL]
%s BUY
%s (Under 200 words, covering technical, fundamental, or news-driven reasoning)

[TICKER: MSFT]
%s HOLD
%s ...

Watchlist tickers come first; within each group, order by conviction with your highest-confidence pick first.
`,
	KeyReasonMarker:        "Reason:",
	KeyActionMarker:        "Action:",
	KeyMarketSummaryMarker: "[MARKET SUMMARY]",

	KeyCheckPromptIntro: "You are a US equities analyst. Give an instant market read on the ticker below.\n\n",
	KeyCheckPromptTask: `
Please analyze in English:
1. Today's price action (technical read)
2. Recent news impact (sentiment)
3. Fundamental health (if fundamentals/financials are provided, comment on valuation, profitability, and financial structure)
4. Short-term call (Buy / Hold / Reduce) with reasoning

Keep the reply concise — under 400 words total.
`,

	KeyStockHeader:             "### %s\n",
	KeyPriceLine:               "- Price: $%.2f  Change: %.2f%%\n",
	KeyOHLLine:                 "- Open: $%.2f  High: $%.2f  Low: $%.2f\n",
	KeyVolumeLine:              "- Volume: %d  Prev Close: $%.2f\n",
	KeyQuoteTimeLine:           "- Quote Time: %s\n",
	KeyNewsHeader:              "- Recent News:\n",
	KeyNewsItem:                "  %d. [%s] %s\n",
	KeyFundamentalsSummaryLine: "- Fundamentals: P/E %.1f | P/B %.1f | ROE %.1f%% | Gross Margin %.1f%% | Operating Margin %.1f%% | Net Margin %.1f%% | Debt/Equity %.2f | Revenue YoY %.1f%% | EPS YoY %.1f%% | Dividend Yield %.2f%% | Beta %.2f | EPS(TTM) %.2f | Current Ratio %.2f | Market Cap $%.0fM | From 52W High %+.1f%% | From 52W Low %+.1f%%\n",
	KeyStatementSummaryLine:    "- Latest Filing (%s, FY%d, as of %s): Revenue $%.0fM | Gross Profit $%.0fM | Operating Income $%.0fM | Net Income $%.0fM | Total Assets $%.0fM | Total Liabilities $%.0fM | Total Equity $%.0fM | Operating Cash Flow $%.0fM | Free Cash Flow $%.0fM\n",
	KeyPositionLine:            "- Position: %g shares, avg cost $%.2f (unrealized %+.2f%%)\n",
	KeyPrevRecLine:             "- Previous call: %s @ $%.2f (%d days ago)\n",
	KeyEarningsLine:            "- ⚠️ Earnings date: %s (in %d days) — expect volatility\n",
	KeyScanHitLine:             "- 🔎 Scan hit: %s\n",
	KeyTechnicalsSummaryLine:   "- Technicals: RSI(14) %.1f | MACD %s\n",
	KeyTechnicalsMALine:        "- %s MA%d ($%.2f)\n",
	KeyTrendBullish:            "Bullish",
	KeyTrendBearish:            "Bearish",
	KeyTrendUnknown:            "insufficient data",
	KeyAboveMA:                 "Above",
	KeyBelowMA:                 "Below",

	KeyMCPTickerHeader: "## %s\n",

	KeyMCPNoQuote: "No quote data available for %s.",

	KeyMCPNoHistory:     "No historical price data available for %s.",
	KeyMCPHistoryResult: "%s closing prices for the last %d trading days (oldest first): %s\n",

	KeyMCPNoNews:   "No recent news found for %s.",
	KeyMCPNewsItem: "%d. [%s] %s (%s)\n   %s\n",

	KeyMCPNoFundamentals:        "No fundamentals data available for %s.",
	KeyMCPNoFinancialStatements: "No financial statement data available for %s.",

	KeyMCPNoEarnings:   "None of the requested tickers have earnings scheduled within %d days.",
	KeyMCPEarningsItem: "- %s: %s (%s)\n",

	KeyMCPNoMovers:     "No market movers data available right now.",
	KeyMCPMoversResult: "Current market movers: %s\n",
}
