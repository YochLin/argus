package i18n

// enMessages is the English message table. Every format verb here must
// appear in the same order as its zh.go counterpart — see the note there.
var enMessages = map[Key]string{
	KeyUnknownCommand:            "Unknown command. Available commands:\n/add <ticker>\n/remove <ticker>\n/list\n/status [ticker]\n/recommend\n/check <ticker>\n/track [days]\n/buy <ticker> <shares> <price> [fee]\n/sell <ticker> <shares> <price> [fee]\n/portfolio\n/insight\n/cash [amount]\n/thesis <ticker> [text]\n/review <ticker>\n/dailyreport\n/fundamentals <ticker>\n/universe [add|remove] <ticker>\n/reset\n\nSend a plain message (no leading /) to chat freely.",
	KeyAddUsage:                  "Usage: /add <ticker>, e.g. /add AAPL",
	KeyAddFailed:                 "Failed to add: %v",
	KeyAddSuccess:                "Added *%s* to your watchlist.",
	KeyRemoveUsage:               "Usage: /remove <ticker>, e.g. /remove AAPL",
	KeyRemoveFailed:              "Failed to remove: %v",
	KeyRemoveSuccess:             "Removed *%s* from your watchlist.",
	KeyQueryFailed:               "Query failed: %v",
	KeyWatchlistEmptyHint:        "Your watchlist is empty. Use /add <ticker> to add one.",
	KeyWatchlistTitle:            "📋 *Watchlist*\n",
	KeyWatchlistEmpty:            "Your watchlist is empty.",
	KeyMarketStatusTitle:         "📊 *Market Status*\n\n",
	KeyQuoteUnavailable:          "*%s*: quote unavailable\n",
	KeyQuoteFailed:               "Failed to get %s quote: %v",
	KeyAnalyzing:                 "Analyzing, please wait...",
	KeyWatchlistQueryFailed:      "Failed to query watchlist: %v",
	KeyLLMFailed:                 "LLM analysis failed: %v",
	KeyNoRecommendations:         "The LLM returned no recommendations — try again later.",
	KeyRecommendationsTitle:      "🤖 *Today's Recommendations*\n\n",
	KeyRecWatchlistSectionTitle:  "📋 *Watchlist & Holdings*\n",
	KeyRecCandidatesSectionTitle: "🆕 *New Candidates*\n",
	KeyMarketNewsSummaryTitle:    "📰 *Market News Summary*\n\n",
	KeyCheckUsage:                "Usage: /check <ticker>, e.g. /check TSLA",
	KeyAnalyzingTicker:           "Analyzing, please wait... (%s)",
	KeyCheckResultTitle:          "📈 *%s Instant Analysis*\n\n%s",
	KeyFundamentalsUsage:         "Usage: /fundamentals <ticker>, e.g. /fundamentals AAPL",
	KeyFundamentalsNoKey:         "FINNHUB_API_KEY isn't configured, so fundamentals data is unavailable.",
	KeyFundamentalsFailed:        "Failed to get %s fundamentals: %v",
	KeyFundamentalsTitle:         "📊 *%s Fundamentals*\n\n",
	KeyThinking:                  "Thinking, please wait...",
	KeyChatFailed:                "Chat failed: %v",
	KeyResetDone:                 "Conversation memory has been reset.",
	KeySignalAlertTitle:          "🚨 *Market Signal Alert*\n\n",
	KeyDailyReportStart:          "📅 *Generating daily report...*",
	KeyNoRecommendationsToday:    "No notable recommendations today.",
	KeyTrackUsage:                "Usage: /track [days], e.g. /track 14 (default 7 days, max 90)",
	KeyTrackTitle:                "📊 *Recommendation Review (past %d days)*\n\n",
	KeyTrackEmpty:                "No recommendations recorded in the past %d days.",
	KeyTrackLine:                 "• %s *%s* %s  $%.2f → $%.2f (%+.2f%%) %s\n",
	KeyTrackLineVsSPY:            "• %s *%s* %s  $%.2f → $%.2f (%+.2f%% vs SPY %+.2f%%) %s\n",
	KeyTrackLineNoPrice:          "• %s *%s* %s — no price recorded to compare against\n",
	KeyTrackSummary:              "\nHit rate: %d/%d (%.0f%%)\n(When same-period SPY data is available, BUY must beat SPY and SELL must underperform SPY to count as a hit; without SPY data it falls back to absolute direction; HOLD and rows without price data are excluded)",
	KeyTrackAvgReturnLine:        "Avg return: BUY %+.2f%% (%d calls) | SELL %+.2f%% (%d calls)\n",
	KeyTrackBySourceHeader:       "\nBy source:\n",
	KeyTrackBySourceLine:         "- %s: %d/%d (%.0f%%)\n",

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

	KeyInsightResultTitle: "💡 *Portfolio Insight*\n\n%s",

	KeyWeeklyReviewStart:          "📅 Generating this week's portfolio review...",
	KeyWeeklyReviewResultTitle:    "📅 *Weekly Portfolio Review*\n\n%s",
	KeyWeeklyNetWorthLine:         "📈 Total value: $%.2f (%+.1f%% vs a week ago)\n",
	KeyWeeklyNetWorthLineWithCash: "📈 Total value: $%.2f (%+.1f%% vs a week ago) | Total assets incl. cash: $%.2f\n",
	KeyWeeklyEarningsPreviewTitle: "\n📅 *Next Week's Earnings*\n",
	KeyWeeklyEarningsPreviewLine:  "- %s: %s (in %d days)\n",

	KeyMonthlyReportTitle:         "📅 *%s Net Worth Monthly Report*\n\n",
	KeyMonthlyReportSparklineLine: "Trend: %s\n",
	KeyMonthlyReportChangeLine:    "Month-end net worth: $%.2f (%+.2f%% vs. last month-end)\n",
	KeyMonthlyReportDrawdownLine:  "Max drawdown this month: %.2f%%\n",
	KeyMonthlyReportRealizedLine:  "Realized P&L this month: $%.2f\n",
	KeyMonthlyReportSPYLine:       "SPY over the same period: %+.2f%%\n",
	KeyMonthlyReportTxCountLine:   "Transactions this month: %d\n",
	KeyMonthlyReportCashLine:      "Total assets incl. cash: $%.2f (cash $%.2f)\n",

	KeyCashUsage:      "Usage: /cash <amount> (omit the amount to check the current value)",
	KeyCashCurrent:    "Declared cash: $%.2f",
	KeyCashNotSet:     "Cash balance not set yet. Use /cash <amount> to set it.",
	KeyCashSetSuccess: "Cash balance set to $%.2f.",
	KeyCashSetFailed:  "Failed to set cash balance: %v",

	KeyStopLossAlertTitle:       "🛑 *Stop-Loss Alert*\n\n",
	KeyStopLossAlertLine:        "• *%s* cost $%.2f → price $%.2f, unrealized loss %.1f%% — stop-loss threshold hit\n",
	KeyTrailingStopAlertTitle:   "📉 *Trailing-Stop Alert*\n\n",
	KeyTrailingStopAlertLine:    "• *%s* peak close since entry $%.2f → price $%.2f, down %.1f%% from peak — trailing-stop threshold hit\n",
	KeyTrailingStopAlertLineATR: "• *%s* peak close since entry $%.2f → price $%.2f, down %.1f%% from peak — trailing-stop threshold hit (%.1f%% threshold, %.1f×ATR)\n",

	KeyChatContextHeader:       "[Background info, not real-time, for reference only]\n",
	KeyChatContextTickerNoData: "- %s: no closing data yet\n",
	KeyChatContextWatchLine:    "- %s: %s close $%.2f (%+.2f%%)\n",
	KeyChatContextPositionLine: "- %s: %s close $%.2f (%+.2f%%) | holding %g shares, avg cost $%.2f, unrealized %+.2f%%\n",
	KeyChatContextFooter:       "[End of background info — reply to the message below]\n\n",

	KeyFetchingArticle:    "Fetching the article, please wait...",
	KeyArticleFetchFailed: "Couldn't fetch that URL: %v\n(it may be paywalled or blocking automated access — you can also just paste the article text directly)",
	KeyArticleTaskBlock:   "The user shared an article link in chat. First summarize the article in 3-5 sentences, then explain whether and how it's relevant to the user's watchlist/holdings — if there's no real connection, just say so rather than forcing one.\n\nTitle: %s\nURL: %s\n\nContent:\n%s\n\nUser's message: %s\n",

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
	KeyUniverseDelistedTitle: "📉 *S&P 500 Constituent Changes*\n\nThe following tickers are no longer in the latest S&P 500 list. Whether to remove them from the scan pool is up to you:\n\n",
	KeyUniverseDelistedLine:  "• %s (use /universe remove %s to remove it)\n",

	KeyThesisUsage:      "Usage: /thesis <ticker> [thesis text]\nTicker alone looks up the recorded thesis; adding text overwrites it.",
	KeyThesisSetFailed:  "Failed to record thesis for %s: %v",
	KeyThesisSetSuccess: "Recorded holding thesis for *%s*: %s",
	KeyThesisNotSet:     "No thesis recorded for *%s* yet — add one with /thesis %s <text>.",
	KeyThesisCurrent:    "Current thesis for *%s*: %s",
	KeyBuyThesisNudge:   "\n💭 No thesis recorded for *%s* yet — add one with /thesis %s <text>?",

	KeyReviewUsage:            "Usage: /review <ticker> — review the most recent fully closed trade for that ticker",
	KeyReviewNoClosedTrade:    "*%s* has no fully closed trade on record to review yet.",
	KeyTradeReviewResultTitle: "📓 *Trade Review: %s*\n\n%s",

	KeyConfirmButton:                "✅ Confirm",
	KeyRejectButton:                 "❌ Reject",
	KeyPendingBuyConfirm:            "📝 *Trade proposal awaiting confirmation*\nBUY *%s* %g shares @ $%.2f (fee $%.2f, date %s)\nTap a button below to confirm or reject:",
	KeyPendingSellConfirm:           "📝 *Trade proposal awaiting confirmation*\nSELL *%s* %g shares @ $%.2f (fee $%.2f, date %s)\nTap a button below to confirm or reject:",
	KeyPendingActionRejected:        "Rejected — nothing was recorded.",
	KeyPendingActionAlreadyResolved: "This proposal has already been resolved and won't be executed again.",
	KeyPendingActionExecFailed:      "Couldn't parse this proposal, so nothing was executed — use /buy or /sell to record it manually.",

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
	KeyMarketRegimeHeader:  "## Market Regime\n\n",
	KeyMarketRegimeSPYLine: "• SPY price $%.2f, MA200 $%.2f, MA50 $%.2f — %s\n",
	KeyMarketRegimeVIXLine: "• VIX %.2f — %s\n",
	KeyRiskOn:              "risk-on (price above MA200)",
	KeyRiskOff:             "risk-off (price below MA200)",
	KeyVIXCalm:             "calm",
	KeyVIXNormal:           "normal",
	KeyVIXPanic:            "high panic",
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

For any BUY call, also weigh whether the current price is a reasonable entry point right now — where it sits within
the 52-week range, whether it's above or below its moving averages, whether volume is running well above its recent
average (real buying conviction behind the move vs. a low-volume, unconvincing breakout), and whether analyst ratings
have recently turned more bullish — as a read on market momentum and appetite for the stock, rather than leaning on
valuation multiples or analyst price targets (which tend to lag price and are of limited use here). If momentum looks
poor (e.g. a pullback right after a fresh high, weak volume, or ratings turning more cautious), say so as an
entry-timing concern in your reasoning even when fundamentals or news are otherwise positive — don't just cite
improving fundamentals.

If a ticker's section below shows a "Previous call" and today's action differs from it, your reasoning must
explicitly say what changed (new information, a technical signal reversal, or the original thesis no longer
holding) — don't just restate the same conclusion in different words.

If a Market Regime block appears above, weigh it when calling new BUYs: hold new positions to a higher bar in a
risk-off regime or when VIX is elevated rather than applying the same standard regardless of market conditions.

Before calling a BUY, mentally list the strongest bearish case against it and briefly note in your reasoning why
that case doesn't hold or matters little; for a SELL, do the reverse — list the strongest bullish case first and
explain why it isn't enough to justify continuing to hold. This guards against only picking evidence that supports
the conclusion you already reached and ignoring the counter-argument.

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

	KeyExplorePromptIntro: "You are a US equities analyst. Based on the market news above, nominate US-listed tickers not already on the lists below that these news items make worth watching.\n\n",
	KeyExploreExcludeLine: "The following tickers are already on a list — do not nominate them again: %s\n\n",
	KeyExploreTaskBlock: `
## Task

Nominate up to %d US-equity tickers mentioned above that are not on the exclude list (real, existing US tickers only — do not invent one).
If nothing in the news is worth nominating, output nothing.

Strictly follow this output format, one block per ticker, with no extra text:

%s TICKER]
%s one-sentence reason this ticker is worth watching (under 50 words)
`,
	KeyExploreMarker:      "[EXPLORE:",
	KeyExploreReasonLabel: "LLM nomination: %s",

	KeyCheckPromptIntro: "You are a US equities analyst. Give an instant market read on the ticker below.\n\n",
	KeyCheckPromptTask: `
Please analyze in English:
1. Today's price action (technical read)
2. Recent news impact (sentiment)
3. Fundamental health (if fundamentals/financials are provided, comment on valuation, profitability, and financial structure)
4. Short-term call (Buy / Hold / Reduce) with reasoning

Keep the reply concise — under 400 words total.
`,

	KeyInsightPromptIntro:       "You are a US equities analyst. Give a portfolio-level read on the whole set of holdings below (not a repeat of each ticker's individual analysis).\n\n",
	KeyInsightPositionValueLine: "Total position value: $%.2f\n",
	KeyInsightCashLine:          "Cash balance: $%.2f\nTotal assets (positions + cash): $%.2f\n",
	KeyInsightPromptTask: `
Please analyze the whole portfolio in English (don't repeat the per-ticker data already listed — focus on portfolio-level judgment):
1. Concentration risk: is any single position or sector too large a share of the portfolio (infer sector from your own
   knowledge of these tickers — don't wait for it to be supplied)
2. Thesis check: where a "holding thesis" is provided, challenge it directly — against the recent news/fundamentals
   shown, does it still hold, and what's changed; for holdings with no thesis, assess whether the likely original
   rationale still holds
3. Performance vs. the market: where "vs. market" data is provided, call out any holding that's actually
   underperforming SPY — a position being up doesn't mean it was the right call
4. Add/reduce/stop-loss suggestions: which positions should be added to, trimmed, or given a stop-loss, and why
5. If a cash balance is provided, briefly comment on whether the current positions/cash split makes sense

Keep the reply concise — under 600 words total.
`,

	KeyWeeklyReviewPromptIntro: "You are a US equities analyst. Give this week's portfolio review for the whole set of holdings below (not a repeat of each ticker's individual analysis).\n\n",
	KeyWeeklyReviewTrackHeader: "\nThis week's recommendation tracking:\n",
	KeyWeeklyReviewPromptTask: `
Please analyze the whole portfolio in English (don't repeat the per-ticker data already listed — focus on portfolio-level judgment):
1. Concentration risk: is any single position or sector too large a share of the portfolio (infer sector from your own
   knowledge of these tickers — don't wait for it to be supplied)
2. Thesis check: where a "holding thesis" is provided, challenge it directly — against the recent news/fundamentals
   shown, does it still hold, and what's changed; for holdings with no thesis, assess whether the likely original
   rationale still holds
3. Performance vs. the market: where "vs. market" data is provided, call out any holding that's actually
   underperforming SPY — a position being up doesn't mean it was the right call
4. Add/reduce/stop-loss suggestions: which positions should be added to, trimmed, or given a stop-loss, and why
5. If a cash balance is provided, briefly comment on whether the current positions/cash split makes sense
6. If "this week's recommendation tracking" data is provided above, briefly comment on which kind of call (by
   action BUY/SELL, or by source watchlist/movers/scan) has been most/least accurate lately; skip this point if
   that data isn't present

Keep the reply concise — under 600 words total.
`,

	KeyTradeReviewPromptIntro: "You are a US equities analyst. Review this fully closed *%s* trade below.\n\n",
	KeyTradeReviewLegLine:     "• %s %g shares @ $%.2f (%s)\n",
	KeyTradeReviewPnLLine:     "Realized P&L: %+.2f | Holding period: %d days\n",
	KeyTradeReviewRangeLine:   "Price range over the holding period: high $%.2f / low $%.2f\n",
	KeyTradeReviewRecsHeader:  "\nRecommendations during the holding period:\n",
	KeyTradeReviewRecLine:     "- %s %s: %s\n",
	KeyTradeReviewPromptTask: `
Give an honest, unvarnished review in English — even if the trade made money, call out any judgment errors along the way:
1. Entry/exit timing: where did the buy and sell prices land relative to the holding period's own high/low? Did
   exiting too early leave money on the table, or too late give back gains?
2. Thesis check: if a "holding thesis" is shown above, did the actual outcome bear it out? Even a profitable trade
   should be called out if the reason it worked had nothing to do with the thesis; skip this point if no thesis was recorded.
3. Recommendation check: if recommendations from the holding period are shown above, did they match what the user
   actually did (e.g. the system said SELL but the position was held for two more weeks, or said HOLD but was sold)?
   Skip this point if no recommendations are shown.
4. Distill one concrete, actionable lesson — not a vague "be more careful."

End your reply with one final line starting with exactly:
%s <the lesson, 2-4 sentences max>

Keep the reply concise — under 300 words total.
`,
	KeyLessonMarker: "Lesson:",

	KeyPastLessonsHeader:   "- Past lessons from this ticker:\n",
	KeyPastLessonLine:      "  · %s: %s\n",
	KeyRecentLessonsHeader: "## Recent Trade Lessons (across tickers, for reference)\n\n",
	KeyRecentLessonLine:    "- %s (%s): %s\n",

	KeyStockHeader:             "### %s\n",
	KeyPriceLine:               "- Price: $%.2f  Change: %.2f%%\n",
	KeyOHLLine:                 "- Open: $%.2f  High: $%.2f  Low: $%.2f\n",
	KeyVolumeLine:              "- Volume: %d  Prev Close: $%.2f\n",
	KeyQuoteTimeLine:           "- Quote Time: %s\n",
	KeyNewsHeader:              "- Recent News:\n",
	KeyNewsItem:                "  %d. [%s] %s\n",
	KeyFundamentalsSummaryLine: "- Fundamentals: P/E %.1f | P/B %.1f | ROE %.1f%% | Gross Margin %.1f%% | Operating Margin %.1f%% | Net Margin %.1f%% | Debt/Equity %.2f | Revenue YoY %.1f%% | EPS YoY %.1f%% | Dividend Yield %.2f%% | Beta %.2f | EPS(TTM) %.2f | Current Ratio %.2f | Market Cap $%.0fM | From 52W High %+.1f%% | From 52W Low %+.1f%%\n",
	KeyAnalystRatingLine:       "- Analyst Ratings (%s): Strong Buy %d | Buy %d | Hold %d | Sell %d | Strong Sell %d\n",
	KeyAnalystRatingTrendLine:  "  vs. last month: Buy-side %+d | Sell-side %+d\n",
	KeyStatementSummaryLine:    "- Latest Filing (%s, FY%d, as of %s): Revenue $%.0fM | Gross Profit $%.0fM | Operating Income $%.0fM | Net Income $%.0fM | Total Assets $%.0fM | Total Liabilities $%.0fM | Total Equity $%.0fM | Operating Cash Flow $%.0fM | Free Cash Flow $%.0fM\n",
	KeyPositionLine:            "- Position: %g shares, avg cost $%.2f (unrealized %+.2f%%)\n",
	KeyPrevRecLine:             "- Previous call: %s @ $%.2f (%d days ago)\n",
	KeyEarningsLine:            "- ⚠️ Earnings date: %s (in %d days) — expect volatility\n",
	KeyScanHitLine:             "- 🔎 Scan hit: %s\n",
	KeyThesisLine:              "- 📝 Holding thesis: %s\n",
	KeyVsSPYLine:               "- vs. market: %+.1f%% over the holding period vs. SPY %+.1f%% over the same period\n",
	KeyTechnicalsSummaryLine:   "- Technicals: RSI(14) %.1f | MACD %s\n",
	KeyTechnicalsMALine:        "- %s MA%d ($%.2f)\n",
	KeyVolumeRatioLine:         "- Volume: %.1fx 20-day avg\n",
	KeyATRLine:                 "- ATR(14): $%.2f (%.1f%% of price)\n",
	KeyBollingerLine:           "- Bollinger %%B: %.0f%% (0%% = lower band, 100%% = upper band, can go below 0%% or above 100%% on a breakout)\n",
	KeyCandlesHeader:           "- Last %d daily candles (oldest first):\n",
	KeyCandleLine:              "  %s O %.2f H %.2f L %.2f C %.2f V %d\n",
	KeyTrendBullish:            "Bullish",
	KeyTrendBearish:            "Bearish",
	KeyTrendUnknown:            "insufficient data",
	KeyAboveMA:                 "Above",
	KeyBelowMA:                 "Below",

	KeyMCPTickerHeader: "## %s\n",

	KeyMCPNoQuote: "No quote data available for %s.",

	KeyMCPNoHistory:     "No historical price data available for %s.",
	KeyMCPHistoryResult: "%s daily candles for the last %d trading days (oldest first):\n%s",

	KeyMCPNoNews:   "No recent news found for %s.",
	KeyMCPNewsItem: "%d. [%s] %s (%s)\n   %s\n",

	KeyMCPNoFundamentals:        "No fundamentals data available for %s.",
	KeyMCPNoFinancialStatements: "No financial statement data available for %s.",

	KeyMCPNoEarnings:   "None of the requested tickers have earnings scheduled within %d days.",
	KeyMCPEarningsItem: "- %s: %s (%s)\n",

	KeyMCPNoMovers:     "No market movers data available right now.",
	KeyMCPMoversResult: "Current market movers: %s\n",

	KeyMCPRecentRecsTitle:       "🗒 *Past recommendations (last %d days, newest first)*\n\n",
	KeyMCPRecentRecsTitleTicker: "🗒 *Past recommendations for %s (last %d days, newest first)*\n\n",
	KeyMCPRecentRecsTruncated:   "(showing the most recent %d of %d)\n\n",
	KeyMCPRecentRecLine:         "• %s *%s* %s @ $%.2f (source: %s)\n  %s\n\n",
	KeyMCPRecentRecLineNoPrice:  "• %s *%s* %s (source: %s)\n  %s\n\n",
	KeyMCPRecentRecsEmptyTicker: "No recommendations recorded for %s in the past %d days.",

	KeyMCPTradeInvalidInput:   "Invalid trade proposal input: ticker, shares, and price must all be valid positive numbers, and date (if given) must be YYYY-MM-DD.",
	KeyMCPTradeProposalFailed: "Failed to create trade proposal: %v",
	KeyMCPTradeProposalBuy:    "Created a BUY proposal: *%s* %g shares @ $%.2f (proposal #%d). This is only a proposal — the user still has to tap Confirm on Telegram before it's actually recorded, so don't treat it as done yet.",
	KeyMCPTradeProposalSell:   "Created a SELL proposal: *%s* %g shares @ $%.2f (proposal #%d). This is only a proposal — the user still has to tap Confirm on Telegram before it's actually recorded, so don't treat it as done yet.",
}
