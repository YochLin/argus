package i18n

// enMessages is the English message table. Every format verb here must
// appear in the same order as its zh.go counterpart — see the note there.
var enMessages = map[Key]string{
	KeyUnknownCommand:                "Unknown command. Available commands:\n/add <ticker>\n/remove <ticker>\n/list\n/status [ticker]\n/recommend\n/check <ticker>\n/track [days]\n/buy <ticker> <shares> <price> [fee]\n/sell <ticker> <shares> <price> [fee]\n/stop <ticker> [price]\n/buyalert <ticker> [price|remove price]\n/portfolio\n/obuy /osell /oassign /oexercise\n/option <ticker> [call|put|csp|cc]\n/insight\n/cash [amount]\n/thesis <ticker> [text]\n/review <ticker>\n/events [ticker]\n/dailyreport\n/morningreport\n/monthlyreport\n/fundamentals <ticker>\n/universe [add|remove] <ticker>\n/reset\n/paper [reset]\n/podcast <url>\n\nSend a plain message (no leading /) to chat freely.",
	KeyAddUsage:                      "Usage: /add <ticker>, e.g. /add AAPL",
	KeyAddFailed:                     "Failed to add: %v",
	KeyAddSuccess:                    "Added *%s* to your watchlist.",
	KeyRemoveUsage:                   "Usage: /remove <ticker>, e.g. /remove AAPL",
	KeyRemoveFailed:                  "Failed to remove: %v",
	KeyRemoveSuccess:                 "Removed *%s* from your watchlist.",
	KeyQueryFailed:                   "Query failed: %v",
	KeyWatchlistEmptyHint:            "Your watchlist is empty. Use /add <ticker> to add one.",
	KeyWatchlistTitle:                "📋 *Watchlist*\n",
	KeyWatchlistEmpty:                "Your watchlist is empty.",
	KeyMarketStatusTitle:             "📊 *Market Status*\n\n",
	KeyQuoteUnavailable:              "*%s*: quote unavailable\n",
	KeyQuoteFailed:                   "Failed to get %s quote: %v",
	KeyAnalyzing:                     "Analyzing, please wait...",
	KeyWatchlistQueryFailed:          "Failed to query watchlist: %v",
	KeyLLMFailed:                     "LLM analysis failed: %v",
	KeyNoRecommendations:             "The LLM returned no recommendations — try again later.",
	KeyRecParseFailed:                "Could not parse the LLM's recommendation reply (unexpected format); raw reply has been logged: %v",
	KeyRecommendationsTitle:          "🤖 *Today's Recommendations*\n\n",
	KeyRecommendUsage:                "Usage: /recommend [tw|us] (no argument = run both US and TW reports in sequence)",
	KeyRecWatchlistSectionTitle:      "📋 *Watchlist & Holdings*\n",
	KeyRecCandidatesSectionTitle:     "🆕 *New Candidates*\n",
	KeyRecWatchlistNoneActionable:    "No action needed for the watchlist or holdings today.",
	KeyRecCandidatesAnalyzedNone:     "Analyzed %d candidate(s) — none worth recommending today.",
	KeyRecCandidatesUnavailable:      "No candidates to analyze today (market movers fetch failed or no strategy hits).",
	KeySizingLine:                    "💰 Suggested sizing: risk budget %s, stop ref %s (price−2×ATR), ~%d shares\n",
	KeyMarketNewsSummaryTitle:        "📰 *Market News Summary*\n\n",
	KeyCheckUsage:                    "Usage: /check <ticker>, e.g. /check TSLA",
	KeyAnalyzingTicker:               "Analyzing, please wait... (%s)",
	KeyCheckResultTitle:              "📈 *%s Instant Analysis*\n\n%s",
	KeyFundamentalsUsage:             "Usage: /fundamentals <ticker>, e.g. /fundamentals AAPL",
	KeyFundamentalsNoKey:             "FINNHUB_API_KEY isn't configured, so fundamentals data is unavailable.",
	KeyFundamentalsFailed:            "Failed to get %s fundamentals: %v",
	KeyFundamentalsTitle:             "📊 *%s Fundamentals*\n\n",
	KeyThinking:                      "Thinking, please wait...",
	KeyChatFailed:                    "Chat failed: %v",
	KeyResetDone:                     "Conversation memory has been reset.",
	KeySignalAlertTitle:              "🚨 *Market Signal Alert*\n\n",
	KeyDailyReportStart:              "📅 *Generating daily report...*",
	KeyMorningBriefingStart:          "📅 *Generating US market morning briefing...*",
	KeyMorningBriefingMarketClosed:   "📅 US markets were closed yesterday — skipping the morning briefing.",
	KeyTWMorningBriefingStart:        "📅 *Generating TW market pre-open briefing...*",
	KeyTWMorningBriefingMarketClosed: "📅 TW markets are closed — skipping the pre-open briefing.",
	KeyNoRecommendationsToday:        "The LLM returned no recommendations for today's report — try again later.",
	KeyTrackUsage:                    "Usage: /track [days], e.g. /track 14 (default 7 days, max 90)",
	KeyTrackTitle:                    "📊 *Recommendation Review (past %d days)*\n\n",
	KeyTrackEmpty:                    "No recommendations recorded in the past %d days.",
	KeyTrackLine:                     "• %s *%s* %s  %s → %s (%+.2f%%) %s\n",
	KeyTrackLineVsSPY:                "• %s *%s* %s  %s → %s (%+.2f%% vs %s %+.2f%%) %s\n",
	KeyTrackLineNoPrice:              "• %s *%s* %s — no price recorded to compare against\n",
	KeyTrackSummary:                  "\nHit rate: %d/%d (%.0f%%)\n(When same-period benchmark (SPY/0050) data is available, BUY must beat the benchmark and SELL must underperform it to count as a hit; without benchmark data it falls back to absolute direction; HOLD and rows without price data are excluded)",
	KeyTrackAvgReturnLine:            "Avg return: BUY %+.2f%% (%d calls) | SELL %+.2f%% (%d calls)\n",
	KeyTrackBySourceHeader:           "\nBy source:\n",
	KeyTrackBySourceLine:             "- %s: %d/%d (%.0f%%)\n",
	KeyTrackByMarketHeader:           "\nBy market:\n",
	KeyTrackByMarketLine:             "- %s: %d/%d (%.0f%%)\n",

	KeyBuyUsage:               "Usage: /buy <ticker> <shares> <price> [fee] [date], e.g. /buy AAPL 10 205.5 or /buy AAPL 10 205.5 1.5 2026-01-15 (backdate a historical trade)",
	KeyBuyFailed:              "Failed to record buy: %v",
	KeyBuySuccess:             "✅ Recorded BUY of *%s* %g shares @ %s (fee %s)\nPosition now: %g shares, avg cost %s",
	KeySellUsage:              "Usage: /sell <ticker> <shares> <price> [fee] [date], e.g. /sell AAPL 5 210 or /sell AAPL 5 210 1 2026-01-15 (backdate a historical trade)",
	KeySellFailed:             "Failed to record sell: %v",
	KeySellSuccess:            "✅ Recorded SELL of *%s* %g shares @ %s (fee %s)\nRealized P&L: %+.2f\nRemaining position: %g shares",
	KeyUndoUsage:              "Usage: /undo <ticker> — deletes that ticker's most recent transaction (last one only, for fixing a typo)",
	KeyUndoNoTx:               "%s has no transaction to delete",
	KeyUndoNotFound:           "Transaction not found (may already be deleted)",
	KeyUndoNotLatest:          "Only the most recent transaction for a ticker can be deleted — offset an older one with a matching buy/sell instead",
	KeyUndoFailed:             "Failed to delete transaction: %v",
	KeyUndoBuySuccess:         "✅ Deleted *%s* BUY of %g shares @ %s — position and cash restored",
	KeyUndoSellSuccess:        "✅ Deleted *%s* SELL of %g shares @ %s — position and cash restored",
	KeySellNoPosition:         "No position in *%s* to sell.",
	KeySellInsufficientShares: "Not enough shares of *%s* to sell that many.",
	KeyFeeAutoNote:            "\nFee auto-calculated from your broker discount; check your statement for the exact amount.",

	KeyStopUsage:            "Usage: /stop <ticker> [price] (omit the price to see the current setting and candidate stop prices), e.g. /stop AAPL 190",
	KeyStopNoPosition:       "No position in *%s* to set a stop for.",
	KeyStopInvalidPrice:     "Stop price %s must be below the latest close %s (a long stop sits below price).",
	KeyStopSet:              "✅ Set stop for *%s* at %s (%.1f%% below current price, risk %s/share)",
	KeyStopShow:             "*%s* current stop: %s\n",
	KeyStopNotSet:           "*%s* has no per-trade stop set — falls back to the global stop-loss threshold of %.1f%%.\n",
	KeyStopCandidatesHeader: "Candidate stop reference prices:\n",
	KeyStopCandidateLine:    "  • %s: %s\n",
	KeyStopLow10Label:       "10-day low",
	KeyStopLow20Label:       "20-day low",
	KeyStopATRLabel:         "price−2×ATR",
	KeyBuyStopSuggestion:    "\n💡 Stop-loss reference:\n",
	KeyBuyStopAddOnNote:     "*%s*'s current stop is %s — use /stop to reset it after adding to the position.\n",

	KeyBuyAlertUsage:       "Usage: /buyalert <ticker> [price | remove <price>] (omit price to list current alerts), e.g. /buyalert AAPL 190",
	KeyBuyAlertQueryFailed: "Query failed: %v",
	KeyBuyAlertSet:         "✅ Set a buy alert for *%s* at %s — will notify when price %s.",
	KeyBuyAlertListHeader:  "Buy alerts for *%s*:\n",
	KeyBuyAlertLine:        "  • %s — notify when price %s\n",
	KeyBuyAlertEmpty:       "No buy alerts set for *%s*.",
	KeyBuyAlertRemoved:     "Removed *%s* buy alert at %s.",
	KeyBuyAlertNotFound:    "No *%s* buy alert set at %s.",
	KeyBuyAlertDirBelow:    "drops to or below the target",
	KeyBuyAlertDirAbove:    "rises to or above the target",

	KeyPortfolioTitle:              "💼 *Portfolio Overview*\n\n",
	KeyPortfolioEmpty:              "No positions yet. Use /buy <ticker> <shares> <price> to record a purchase.",
	KeyPortfolioLine:               "*%s*  %g shares @ avg %s\nPrice %s  Value %s  Unrealized %+.2f (%+.2f%%)\n\n",
	KeyPortfolioSummary:            "—\nTotal market value: $%.2f\nCumulative realized P&L: %+.2f",
	KeyPortfolioSectionUS:          "*US (USD)*\n",
	KeyPortfolioSectionTW:          "*TW (TWD)*\n",
	KeyPortfolioSummaryTWD:         "—\nTotal market value: NT$%.2f\nCumulative realized P&L: %+.2f",
	KeyPortfolioSummaryWithCash:    "—\nTotal market value: $%.2f\nCumulative realized P&L: %+.2f\nTotal assets incl. cash: $%.2f (cash $%.2f)",
	KeyPortfolioSummaryWithCashTWD: "—\nTotal market value: NT$%.2f\nCumulative realized P&L: %+.2f\nTotal assets incl. cash: NT$%.2f (cash NT$%.2f)",
	KeyPortfolioLotSuffix:          " (= %d board lots)",
	KeyJobPanic:                    "⚠️ *%s* hit an unexpected error and aborted: %v",

	KeyInsightResultTitleUS: "💡 *Portfolio Insight | US*\n\n%s",
	KeyInsightResultTitleTW: "💡 *Portfolio Insight | TW*\n\n%s",

	KeyWeeklyReviewStart:                  "📅 Generating this week's portfolio review...",
	KeyWeeklyReviewResultTitleUS:          "📅 *Weekly Portfolio Review | US*\n\n%s",
	KeyWeeklyReviewResultTitleTW:          "📅 *Weekly Portfolio Review | TW*\n\n%s",
	KeyWeeklyNetWorthLine:                 "📈 Total value: $%.2f (%+.1f%% vs a week ago)\n",
	KeyWeeklyNetWorthLineWithCash:         "📈 Total value: $%.2f (%+.1f%% vs a week ago) | Total assets incl. cash: $%.2f\n",
	KeyWeeklyNetWorthLineTWD:              "📈 TW total value: NT$%.2f (%+.1f%% vs a week ago)\n",
	KeyWeeklyNetWorthLineWithCashTWD:      "📈 TW total value: NT$%.2f (%+.1f%% vs a week ago) | Total assets incl. cash: NT$%.2f\n",
	KeyWeeklyEarningsPreviewTitle:         "\n📅 *Next Week's Earnings*\n",
	KeyWeeklyEarningsPreviewLine:          "- %s: %s (in %d days)\n",
	KeyWeeklyEarningsPreviewLineEstimated: "- %s: est. statutory filing deadline %s (in %d days, not a confirmed company date)\n",

	KeyMonthlyReportTitle:           "📅 *%s Net Worth Monthly Report*\n\n",
	KeyMonthlyReportSparklineLine:   "Trend: %s\n",
	KeyMonthlyReportChangeLine:      "Month-end net worth: $%.2f (%+.2f%% vs. last month-end)\n",
	KeyMonthlyReportDrawdownLine:    "Max drawdown this month: %.2f%%\n",
	KeyMonthlyReportRealizedLine:    "Realized P&L this month: $%.2f\n",
	KeyMonthlyReportSPYLine:         "SPY over the same period: %+.2f%%\n",
	KeyMonthlyReportTWBenchmarkLine: "0050 over the same period: %+.2f%%\n",
	KeyMonthlyReportTxCountLine:     "Transactions this month: %d\n",
	KeyMonthlyReportCashLine:        "Total assets incl. cash: $%.2f (cash $%.2f)\n",

	KeyCashUsage:         "Usage: /cash <amount> = USD; /cash twd <amount> or /cash usd <amount> to specify a currency; omit the amount to check current values",
	KeyCashCurrent:       "Declared cash: $%.2f",
	KeyCashCurrentTWD:    "Declared cash (TWD): NT$%.2f",
	KeyCashNotSet:        "Cash balance not set yet. Use /cash <amount> to set it.",
	KeyCashSetSuccess:    "Cash balance set to $%.2f.",
	KeyCashSetSuccessTWD: "Cash balance (TWD) set to NT$%.2f.",
	KeyCashSetFailed:     "Failed to set cash balance: %v",

	KeyStopLossAlertTitle:       "🛑 *Stop-Loss Alert*\n\n",
	KeyStopLossAlertLine:        "• *%s* cost %s → price %s, unrealized loss %.1f%% — stop-loss threshold hit\n",
	KeyStopPriceHit:             "• *%s* stop %s, price %s — stop price breached\n",
	KeyTrailingStopAlertTitle:   "📉 *Trailing-Stop Alert*\n\n",
	KeyTrailingStopAlertLine:    "• *%s* peak close since entry %s → price %s, down %.1f%% from peak — trailing-stop threshold hit\n",
	KeyTrailingStopAlertLineATR: "• *%s* peak close since entry %s → price %s, down %.1f%% from peak — trailing-stop threshold hit (%.1f%% threshold, %.1f×ATR)\n",

	KeyTargetReached: "🎯 *%s* hit its %.1fR profit target! Target %s, price %s — consider taking half off and defending the rest with the 5MA.\n",
	KeyMA5Break:      "📉 *%s* broke below its 5-day MA %s (price %s) — the strong-trend line just failed, consider closing out the rest.\n",

	KeyBuyAlertTitle:   "📈 *Buy Alert*\n\n",
	KeyBuyAlertHitLine: "• *%s* target %s, price %s — buy alert triggered (price %s)\n",

	KeyChatContextHeader:       "[Background info, not real-time, for reference only]\n",
	KeyChatContextTickerNoData: "- %s: no closing data yet\n",
	KeyChatContextWatchLine:    "- %s: %s close %s (%+.2f%%)\n",
	KeyChatContextPositionLine: "- %s: %s close %s (%+.2f%%) | holding %g shares, avg cost %s, unrealized %+.2f%%\n",
	KeyChatContextFooter:       "[End of background info — reply to the message below]\n\n",

	KeyFetchingArticle:    "Fetching the article, please wait...",
	KeyArticleFetchFailed: "Couldn't fetch that URL: %v\n(it may be paywalled or blocking automated access — you can also just paste the article text directly)",
	KeyArticleTaskBlock:   "The user shared an article link in chat. First summarize the article in 3-5 sentences, then explain whether and how it's relevant to the user's watchlist/holdings — if there's no real connection, just say so rather than forcing one.\n\nTitle: %s\nURL: %s\n\nContent:\n%s\n\nUser's message: %s\n",

	KeyEarningsAlertTitle:         "📅 *Earnings Reminder*\n\n",
	KeyEarningsAlertLine:          "• *%s* reports earnings on %s (in %d days)\n",
	KeyEarningsAlertLineEstimated: "• *%s*'s statutory filing deadline is %s (in %d days, estimated, not a confirmed report date)\n",

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
	KeyPendingBuyConfirm:            "📝 *Trade proposal awaiting confirmation*\nBUY *%s* %g shares @ %s (fee %s, date %s)\nTap a button below to confirm or reject:",
	KeyPendingSellConfirm:           "📝 *Trade proposal awaiting confirmation*\nSELL *%s* %g shares @ %s (fee %s, date %s)\nTap a button below to confirm or reject:",
	KeyPendingActionFromSinopac:     "\n(source: synced from Sinopac Securities)",
	KeySinopacNotConfigured:         "Sinopac Shioaji isn't configured (SHIOAJI_ADDR is unset).",
	KeySinopacSyncFailed:            "Sinopac sync failed: %v",
	KeySinopacSyncNone:              "Sinopac sync: no new manual trades in the last 7 days.",
	KeySinopacSyncDryRunTitle:       "🔍 *Sinopac sync (dry run — nothing proposed)*",
	KeySinopacSyncTitle:             "✅ *Sinopac sync: %d trade proposal(s) created*",
	KeySinopacSyncNoDseq:            "(no dseq, deduped via a synthesized key)",
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

	KeyPriceUp:         "Up",
	KeyPriceDown:       "Down",
	KeyPriceSignal:     "%s %s %.2f%% (price $%.2f)",
	KeyRSIOverbought:   "%s RSI Overbought (%.1f), potential pullback",
	KeyRSIOversold:     "%s RSI Oversold (%.1f), potential rebound",
	KeyMACDBullish:     "%s MACD Bullish Momentum (MACD %.2f > Signal %.2f)",
	KeyMACDBearish:     "%s MACD Bearish Momentum (MACD %.2f < Signal %.2f)",
	KeyMACDGoldenCross: "%s MACD Golden Cross (MACD %.2f crossed above Signal %.2f), trend turning bullish",
	KeyMACDDeathCross:  "%s MACD Death Cross (MACD %.2f crossed below Signal %.2f), trend turning bearish",

	KeyStrategySqueezeBreakout:        "🚀 %s triggered [Squeeze Breakout] (%s): Bandwidth at 20-day low then breakout with volume, MACD turned positive",
	KeyStrategyBoxBottom:              "🎯 %s triggered [Box Bottom Rebound] (%s): 30-day box floor + KD low golden cross/hook + RSI rebound",
	KeyDaysAgoToday:                   "Today",
	KeyDaysAgoN:                       "%d days ago",
	KeyTechnicalsKDLine:               "- KD(9,3,3): K=%.1f, D=%.1f\n",
	KeyTechnicalsBandwidthLine:        "- Bollinger Bandwidth: %.1f%%\n",
	KeyTechnicalsMAAlignLine:          "- MA Alignment (5/20/60): %s\n",
	KeyTechnicalsVolumePriceLine:      "- Volume-Price: %s\n",
	KeyTechnicalsNewHighLine:          "- New High Record: %s\n",
	KeyTechnicalsMACDZeroLine:         "- MACD Zero-Line: %s (MACD=%.2f)\n",
	KeyTechnicalsRSLine:               "- Relative Strength RS63: %s (vs SPY %+.1f%%)\n",
	KeyStrategyHitLine:                "- ⚠️ Strategy Hit: %s (%s)\n",
	KeyStrategyBearRegimeWarning:      "⚠️ Warning: Broad market is in a bearish/weak regime. Counter-trend strategy signals carry lower win rates.",
	KeyStrategyUnvalidated:            "ℹ️ Screen 3 failed out-of-sample validation: negative excess return at ~4σ across four independent samples (cmd/strategyscan). Reference only — not an entry trigger on its own.",
	KeyStrategyUnvalidatedBoxBottom:   "ℹ️ Screen 2 failed out-of-sample validation: negative or insignificant excess return in both S&P 500 and S&P 400, both time splits (cmd/strategyscan). Reference only — not an entry trigger on its own.",
	KeyStrategyUnvalidatedTrustFollow: "ℹ️ Screen 5 failed out-of-sample validation: negative in one time split, insignificant in the other (cmd/strategyscan, retest against the TWSE T86 canonical data source). Reference only — not an entry trigger on its own.",
	KeyTechGuidanceBlock:              "\n[Technical & Volume-Price Guidance]\n1. Support & Resistance: Long wicks/double bottoms are strong support; breakout candle low is institutional support; broken resistance turns into support.\n2. Volume & Price: Vol Up + Price Up = Trend Start; Vol Down + Price Down = Healthy Correction; Vol Down + Price Up = Weak Rally; Vol Up + Price Down = Panic Dumping.\n3. MACD: Golden crosses above zero line are stronger; price higher high with lower MACD peak indicates bearish divergence.\n4. KD/RSI: Act when KD leaves oversold (>20/25); RSI 50 is bullish/bearish watershed; pullback to 50 is a strong entry.\n5. Moving Averages: Bullish alignment (MA5 > MA20 > MA60 all sloping up) makes 20MA pullback dynamic support.\n6. When giving a BUY, your reasoning must name a specific suggested stop level (a structural support — e.g. the 10/20-day low — or price minus 2x ATR) and why. This is a reference for the human executing the trade, not something you need to size a position around.\n",
	KeyVolUpPriceUp:                   "Vol Up Price Up (Bullish)",
	KeyVolDownPriceDown:               "Vol Down Price Down (Healthy Correction)",
	KeyVolDownPriceUp:                 "Vol Down Price Up (Weak Rally)",
	KeyVolUpPriceDown:                 "Vol Up Price Down (Panic Dumping)",
	KeyNewHigh20And52:                 "20-Day & 52-Week New High",
	KeyNewHigh20:                      "20-Day New High",
	KeyNewHigh52:                      "52-Week New High",
	KeyMACDAboveZero:                  "Above Zero Line (Bullish Zone)",
	KeyMACDBelowZero:                  "Below Zero Line (Bearish Zone)",
	KeyRSStronger:                     "Outperforming Market",
	KeyRSWeaker:                       "Underperforming Market",
	KeyStrategySqueezeName:            "Squeeze Breakout",
	KeyStrategyBoxName:                "Box Bottom Rebound",

	KeySystemPromptAnalyst: "You are a professional US equities analyst whose only job is to give a written analysis based on the market data the user provides. You have no tools available and need none; reply strictly according to the format the user's message requests, with no content unrelated to the analysis.",
	KeySystemPromptChat:    "You are the user's personal assistant. Talk naturally in English and remember what was said earlier in this conversation. You have no tools available and need none.",

	KeyRecPromptIntro:           "You are a US equities analyst. Based on the market data below, give today's action recommendations.\n\n",
	KeyRecPromptIntroTW:         "You are a Taiwan equities analyst. Based on the market data below, give today's action recommendations.\n\n",
	KeyRecTWMarketNote:          "This batch is Taiwan-listed (TWSE/TPEx) stocks, priced in TWD, with a daily move limit of ±10%.\n\n",
	KeyMarketRegimeHeader:       "## Market Regime\n\n",
	KeyMarketRegimeSPYLine:      "• %s price %s, MA200 %s, MA50 %s — %s\n",
	KeyMarketRegimeVIXLine:      "• VIX %.2f — %s\n",
	KeyMarketRegimeVolProxyLine: "• Volatility proxy (ATR14/close) %.1f%% — no official TW volatility index exists; reference only\n",
	KeyRiskOn:                   "risk-on (price above MA200)",
	KeyRiskOff:                  "risk-off (price below MA200)",
	KeyVIXCalm:                  "calm",
	KeyVIXNormal:                "normal",
	KeyVIXPanic:                 "high panic",
	KeyRecMarketNewsHeader:      "## Broad Market News\n\n",
	KeyRecWatchlistHeader:       "## My Watchlist\n\n",
	KeyRecNoWatchlist:           "(No watchlist tickers)\n\n",
	KeyRecMoversHeader:          "## Broad Market Movers\n\n",
	KeyRecNoCandidates:          "(No candidate tickers)\n\n",
	KeyRecMarketSummaryTask: `
Before the per-ticker analysis below, first output a block starting with the exact line
%s
followed by 3–5 bullet points summarizing the market news above, noting which sectors or
tickers might benefit or suffer (under 150 words total). Then continue with the per-ticker
blocks.
`,
	KeyRecTaskBlock: `
## Task

Review every ticker above, but output only items that genuinely need the user's attention: every section that
includes a "Position:" line must get an explicit call, and the action must be exactly one of BUY, SELL, or HOLD;
keep the full analysis for holdings. A watchlist ticker without a "Position:" line should be output only when you
believe it is worth buying now (action BUY); otherwise omit the entire block. From the broad market movers, only
list the ones you actually like as buys (at most 5, action BUY) — none is fine.

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

Strictly follow this output structure, one block per ticker you choose to output, with no extra text:

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

	KeyCheckPromptIntro:   "You are a US equities analyst. Give an instant market read on the ticker below.\n\n",
	KeyCheckPromptIntroTW: "You are a Taiwan equities analyst. Give an instant market read on the ticker below.\n\n",
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
	KeyInsightCashLineTWD:       "Cash balance (TWD): NT$%.2f\n",
	KeyInsightPromptTask: `
Please analyze the whole portfolio in English (don't repeat the per-ticker data already listed — focus on portfolio-level judgment):
1. Concentration risk: is any single position or sector too large a share of the portfolio (infer sector from your own
   knowledge of these tickers — don't wait for it to be supplied)
2. Thesis check: where a "holding thesis" is provided, challenge it directly — against the recent news/fundamentals
   shown, does it still hold, and what's changed; for holdings with no thesis, assess whether the likely original
   rationale still holds
3. Performance vs. the market: where "vs. market" data is provided, call out any holding that's actually
   underperforming its own market's benchmark (the benchmark is named in the data) — a position being up
   doesn't mean it was the right call
4. Add/reduce/stop-loss suggestions: which positions should be added to, trimmed, or given a stop-loss, and why
5. If a cash balance is provided, briefly comment on whether the current positions/cash split makes sense

Keep the reply concise — under 600 words total.
`,

	KeyWeeklyReviewPromptIntro: "You are an equities analyst. Give this week's portfolio review for the whole set of holdings below (not a repeat of each ticker's individual analysis).\n\n",
	KeyWeeklyReviewTrackHeader: "\nThis week's recommendation tracking:\n",
	KeyWeeklyReviewPromptTask: `
Please analyze the whole portfolio in English (don't repeat the per-ticker data already listed — focus on portfolio-level judgment):
1. Concentration risk: is any single position or sector too large a share of the portfolio (infer sector from your own
   knowledge of these tickers — don't wait for it to be supplied)
2. Thesis check: where a "holding thesis" is provided, challenge it directly — against the recent news/fundamentals
   shown, does it still hold, and what's changed; for holdings with no thesis, assess whether the likely original
   rationale still holds
3. Performance vs. the market: where "vs. market" data is provided, call out any holding that's actually
   underperforming its own market's benchmark (the benchmark is named in the data) — a position being up
   doesn't mean it was the right call
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
	KeyRMultipleLine:          "- R multiple: %.1fx\n",
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

	KeyPriceEventPromptIntro:    "You are a stock analyst. *%s* triggered a price event worth recording today — based on the data below, summarize what happened.\n\n",
	KeyPriceEventGapLine:        "Opening gap: %+.1f%% (vs prior close)%s\n",
	KeyPriceEventChangeLine:     "Day's change: %+.1f%% (close vs prior close)%s\n",
	KeyPriceEventCumulativeLine: "Recent cumulative change: %+.1f%%%s\n",
	KeyPriceEventTriggerSuffix:  "  (threshold crossed)",
	KeyPriceEventScaleLine:      "Scale: the day's move is about %.1fx ATR(14), on %.1fx the 20-day average volume\n",
	KeyPriceEventPromptTask: `
Write 3–5 sentences:
1. State the gap/change/cumulative numbers above as fact. Only the one marked "threshold crossed" is why this was recorded; the others are same-day context worth contrasting (a near-flat close after a violent gap means buyers absorbed it intraday).
2. If news is attached above, infer a likely cause from it (the date in parentheses is when it was published); if no news is attached, say plainly "no related news, cause unknown" — don't guess.
3. If a scale line is attached above, use it to say how unusual the move really is (e.g. "about 3x ATR on 2.4x volume") rather than restating the percentage.
4. This is a factual description, not trading advice — don't recommend any action.
`,
	KeyPriceEventResultTitle:        "📊 *Price Event: %s*\n\n%s",
	KeyPriceEventOverflowLine:       "The following tickers also triggered a price event today (over this run's cap, no writeup):\n%sSee /events for the full log",
	KeyPriceEventOverflowTickerLine: "  • %s: gap %+.1f%% / change %+.1f%% / cumulative %+.1f%%\n",
	KeyEventsListTitle:              "📊 *Recent Price Events*\n",
	KeyEventsTickerTitle:            "📊 *%s Price Event Log*\n",
	KeyEventsLine:                   "%s │ %s │ gap %+.1f%% / change %+.1f%% / cumulative %+.1f%% │ %s\n",
	KeyEventsNoSummary:              "(no summary)",
	KeyEventsEmpty:                  "No price events on record yet.",

	KeyMorningBriefingPromptIntro:     "You are a US equities analyst. Based on the data below, write a US market morning briefing in the style of a narrative digest — natural, flowing prose, not a bulleted list of trade calls.\n\n",
	KeyMorningBriefingIndicesHeader:   "## Index Close\n\n",
	KeyMorningBriefingIndexLine:       "%s: %.2f (%+.2f%%)\n",
	KeyMorningBriefingVIXLine:         "VIX: %.2f\n",
	KeyMorningBriefingNewsHeader:      "\n## Macro & Market News\n\n",
	KeyMorningBriefingWatchlistHeader: "\n## Watchlist\n\n",
	KeyMorningBriefingMoversHeader:    "\n## Broad Market Movers\n\n",
	KeyMorningBriefingTaskBlock: `
## Task

Write the US market morning briefing for %s, following this template for structure and tone:

📅 US Market Briefing | [date]

[One sentence naming the day's biggest storyline — an earnings surprise, a macro event, or a shift in market sentiment]

📊 Index Close
[One line per index from the data above, 🟢 for up, 🔴 for down]

🌐 Macro Backdrop
[A short paragraph on macro/Fed/economic-data context based on the news above; keep it brief or omit if there's no relevant news]

👀 Watchlist
[One line per watchlist ticker from the data above: an up/down emoji, the percent change, $TICKER price, with a short reason or news note]

🔥 Tech Spotlight / 💥 Cross-Sector Highlights
[Based on the movers/news above, call out any stock or sector move that's genuinely worth mentioning today; omit if nothing stands out]

🚀 Private Markets Watch
[Only include this section if the news above explicitly mentions a private company or IPO-related item; omit the whole section if there's no grounding for it — never invent content]

Feel free to reach out anytime with questions

Write only from the data actually provided above — omit any section you have no supporting data for rather than fabricating content.
`,

	KeyTWMorningBriefingPromptIntro: "You are a Taiwan equities analyst. It is 30 minutes before the TW market opens. Based on the data below (the prior session's TW close, overnight US performance, VIX, and market news), write a pre-open briefing in the style of a narrative digest — natural, flowing prose, not a bulleted list of trade calls. Remember TW stocks trade under a daily ±10%% price limit, and quote prices in TWD.\n\n",
	KeyTWMorningBriefingTaskBlock: `
## Task

Write the TW market pre-open briefing for %s, following this template for structure and tone:

📅 TW Pre-Open Briefing | [date]

[One sentence naming what matters most heading into today's open — overnight US performance, a VIX shift, or major news]

📊 Prior Close & Overnight US
[One line per index from the data above, 🟢 for up, 🔴 for down]

🌐 Market News
[A short paragraph on how the news above might affect today's TW open; keep it brief or omit if there's no relevant news]

👀 Watchlist
[One line per watchlist ticker from the data above: an up/down emoji, the percent change, $TICKER price, with a short reason or news note]

🔥 Movers to Watch
[Based on the movers/news above, call out any stock or sector move that's genuinely worth mentioning before today's open; omit if nothing stands out]

Feel free to reach out anytime with questions

Write only from the data actually provided above — omit any section you have no supporting data for rather than fabricating content.
`,

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
	KeyNewsItem:                "  %d. [%s] %s (%s)\n",
	KeyNewsSummaryLine:         "     %s\n",
	KeyFundamentalsSummaryLine: "- Fundamentals: P/E %.1f | P/B %.1f | ROE %.1f%% | Gross Margin %.1f%% | Operating Margin %.1f%% | Net Margin %.1f%% | Debt/Equity %.2f | Revenue YoY %.1f%% | EPS YoY %.1f%% | Dividend Yield %.2f%% | Beta %.2f | EPS(TTM) %.2f | Current Ratio %.2f | Market Cap $%.0fM | From 52W High %+.1f%% | From 52W Low %+.1f%%\n",
	KeyMonthRevenueYoYLine:     "- Month Revenue YoY: %+.1f%%\n",
	KeyAnalystRatingLine:       "- Analyst Ratings (%s): Strong Buy %d | Buy %d | Hold %d | Sell %d | Strong Sell %d\n",
	KeyAnalystRatingTrendLine:  "  vs. last month: Buy-side %+d | Sell-side %+d\n",
	KeyInsiderTxLine:           "- Insider Transactions (open-market filings, latest %s): Buys %d (%d sh) | Sells %d (%d sh)\n",
	KeyEarningsSurpriseLine:    "- Past %d quarters: %d beat / %d miss, avg surprise %+.1f%% (most recent %s: %+.1f%%)\n",
	KeyInstitutionalFlowLine:   "- Institutional Flow (%s): Foreign %+d | Trust %+d | Dealer %+d | Total %+d sh\n",
	KeyStatementSummaryLine:    "- Latest Filing (%s, FY%d, as of %s): Revenue $%.0fM | Gross Profit $%.0fM | Operating Income $%.0fM | Net Income $%.0fM | Total Assets $%.0fM | Total Liabilities $%.0fM | Total Equity $%.0fM | Operating Cash Flow $%.0fM | Free Cash Flow $%.0fM\n",
	KeyValuationEPSLine:        "- SEC annual EPS (fiscal year through %s): %.2f\n",
	KeyValuationPELine:         "- P/E at current price: %.1f\n",
	KeyValuationPercentileLine: "- Valuation percentile (vs. its own history only, not cross-market comparable): %.0f%%\n",
	KeyCashFlowQualityLine:     "- Cash-flow quality (operating cash flow / net income, same fiscal year): %.2f\n",
	KeyPositionLine:            "- Position: %g shares, avg cost $%.2f (unrealized %+.2f%%)\n",
	KeyPrevRecLine:             "- Previous call: %s @ $%.2f (%d days ago)\n",
	KeyEarningsLine:            "- ⚠️ Earnings date: %s (in %d days) — expect volatility\n",
	KeyEarningsLineEstimated:   "- ⚠️ Est. statutory filing deadline: %s (in %d days) — expect volatility\n",
	KeyScanHitLine:             "- 🔎 Scan hit: %s\n",
	KeyThesisLine:              "- 📝 Holding thesis: %s\n",
	KeyVsSPYLine:               "- vs. market: %+.1f%% over the holding period vs. %s %+.1f%% over the same period\n",
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

	KeyMCPNoInsiderTx:   "No insider transaction data available for %s.",
	KeyMCPInsiderTxItem: "- %s (%s): %+d sh, code %s, price %.2f\n",

	KeyMCPNoInstitutionalFlow:     "No institutional flow data available for %s.",
	KeyMCPInstitutionalFlowResult: "%s Institutional Flow (%s): Foreign %+d | Trust %+d | Dealer %+d | Total %+d sh",

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

	KeyCheckButton:         "🔍 Check",
	KeyBuyButton:           "🟢 Buy",
	KeySellButton:          "🔴 Sell",
	KeyBuyCommandTemplate:  "Tap to copy, then fill in shares and price:\n```\n/buy %s <shares> <price>\n```",
	KeySellCommandTemplate: "Tap to copy, then fill in shares and price:\n```\n/sell %s <shares> <price>\n```",

	KeyDailyReportMarketClosed:   "📅 US markets are closed today — skipping the daily report.",
	KeyTWDailyReportMarketClosed: "📅 TW markets are closed today — skipping the TW daily report.",

	KeyEvalTitle:                 "=== Offline Recommendation Evaluation ===\n",
	KeyEvalFetching:              "Fetching history... (%d/%d %s)\n",
	KeyEvalOverviewTitle:         "\n[Overview: action x horizon]\n",
	KeyEvalSourceTitle:           "\n[By source: source x horizon]\n",
	KeyEvalMarketTitle:           "\n[Market: %s]\n",
	KeyEvalStatsLine:             "  %s %dd (n=%d): hit rate %.1f%%, avg return %+.2f%%, median %+.2f%%, avg excess %+.2f%%\n",
	KeyEvalExtremesTitle:         "\n[%dd excess return extremes]\n",
	KeyEvalExtremesBest:          "Best %d:\n",
	KeyEvalExtremesWorst:         "Worst %d:\n",
	KeyEvalExtremeLine:           "  %s %s %s @ $%.2f -> %dd: %+.2f%% (excess %+.2f%%, MFE %+.2f%% / MAE %+.2f%%)\n",
	KeyEvalCountsTitle:           "\n[Honest counts]\n",
	KeyEvalCountHold:             "  Not scored (HOLD/no action): %d\n",
	KeyEvalCountUnscorable:       "  Unscorable: %d\n",
	KeyEvalCountUnscorableReason: "    - %s: %d\n",
	KeyEvalCountImmature:         "  Window not yet matured (%dd): %d\n",
	KeyEvalCSVSaved:              "\nDetail CSV written to: %s\n",
	KeyEvalNoData:                "No scorable recommendations found.\n",

	KeyBacktestTitle:           "=== Paper Account Backtest (%s) ===\n",
	KeyBacktestOverviewTitle:   "\n[Overview]\n",
	KeyBacktestOverviewLine:    "Initial cash $%.2f, final equity $%.2f, total return %+.2f%%, CAGR %+.2f%%, max drawdown %.2f%%\n%d trades, win rate %.1f%%, profit factor %.2f, expectancy $%.2f\n",
	KeyBacktestBenchmarkLine:   "Buy-and-hold %s over the same period: %+.2f%% (excess %+.2f%%)\n",
	KeyBacktestExitReasonTitle: "\n[Exit reason breakdown]\n",
	KeyBacktestExitReasonLine:  "  %s: %d, win rate %.1f%%, avg return %+.2f%%\n",
	KeyBacktestSourceTitle:     "\n[By source]\n",
	KeyBacktestSourceLine:      "  %s: %d\n",
	KeyBacktestCountsTitle:     "\n[Honest counts]\n",
	KeyBacktestCountHold:       "  Not actioned (HOLD/no action): %d\n",
	KeyBacktestCountNoHistory:  "  Skipped, no fetchable history: %d tickers\n",
	KeyBacktestCountSkippedBuy: "  BUYs skipped for insufficient funds: %d\n",

	KeyPaperDisabled:   "Paper account is disabled (PAPER_DB_PATH not set).",
	KeyPaperTitle:      "🧪 *Paper Account*\n\n",
	KeyPaperSectionUS:  "*US Stocks (USD)*\n",
	KeyPaperSectionTW:  "*TW Stocks (TWD)*\n",
	KeyPaperLine:       "*%s* %g sh @ avg $%.2f\nprice $%.2f stop $%.2f (%.1f%% away) unrealized %+.2f (%+.2f%%)\n\n",
	KeyPaperSummary:    "—\nCash: $%.2f\nEquity: $%.2f (started $%.2f, return %+.2f%%)\nRealized P&L to date: %+.2f",
	KeyPaperSummaryTWD: "—\nCash: NT$%.2f\nEquity: NT$%.2f (started NT$%.2f, return %+.2f%%)\nRealized P&L to date: %+.2f",
	KeyPaperBuyAlert:   "🧪 Paper account | bought %s %g sh @ %.2f (stop %.2f) | cash $%.2f\n",
	KeyPaperSellAlert:  "🧪 Paper account | sold %s %g sh @ %.2f (%s) realized %+.2f | cash $%.2f\n",
	KeyPaperResetUsage: "Usage: /paper reset confirm",
	KeyPaperResetDone:  "🧪 Paper account cleared and re-seeded.",

	KeyOptionUsage:                "Usage: /obuy <OCC symbol> <contracts> <premium> [fee] [date], or /osell with the same shape, e.g. /obuy AAPL260918C00320000 2 5.40",
	KeyOptionTradeFailed:          "Failed to record option trade: %v",
	KeyOptionCrossesZero:          "This order on %s would cross zero (flip long<->short) — close the position before reversing it",
	KeyOptionTradeSuccess:         "✅ Recorded %s of *%s* %g contract(s) @ $%.2f (fee $%.2f)\nRealized P&L: %+.2f\nPosition now: %g contract(s), avg premium $%.2f",
	KeyOptionNakedCallWarning:     "\n⚠️ Naked call warning: %s needs %g shares locked but you only hold %g — unlimited risk",
	KeyOptionResolutionUsage:      "Usage: /oassign <OCC symbol> [date] or /oexercise with the same shape",
	KeyOptionNoPosition:           "No option position found for %s",
	KeyOptionAssignRequiresShort:  "%s is a long position — /oassign only applies to a short (assignment happens to sellers); use /oexercise if you're exercising it",
	KeyOptionExerciseRequiresLong: "%s is a short position — /oexercise only applies to a long (exercise happens to buyers); use /oassign if it was assigned",
	KeyOptionResolveFailed:        "Failed to resolve option: %v",
	KeyOptionResolveSuccess:       "✅ %s *%s*\nRealized P&L: %+.2f",
	KeyPortfolioOptionsSection:    "\n📜 *Option Positions*",
	KeyPortfolioOptionLine:        "%s\nContracts: %g, avg premium $%.2f, mark $%.2f\nMarket value: $%.2f\nExpiry: %s (%d days left)",
	KeyPortfolioOptionUnavailable: "%s quote temporarily unavailable: %v",
	KeyOptionExpiryConfirm:        "%s has expired (underlying closed $%.2f vs strike $%.2f) — proposed resolution: %s\nPlease confirm",

	KeyOptionSelectUsage:        "Usage: /option <ticker> [call|put|csp|cc], defaults to call when omitted, e.g. /option AAPL or /option AAPL csp",
	KeyOptionSelectFailed:       "Option selection failed: %v",
	KeyOptionSelectNoCandidates: "No candidates for %s (%s) passed the liquidity/delta/DTE screen",
	KeyOptionSelectLine:         "%s\nMark $%.2f  Delta %.3f  IV %.1f%%  OI %d  Spread %.1f%%  DTE %d",

	KeyStrategyTrendBreakout: "🚀 %s triggered [Trend Breakout] (%s): New high + bullish MA alignment + attack volume + deviation within range",
	KeyStrategyTrendPullback: "🎯 %s triggered [Trend Pullback] (%s): MA60 sloping up + pullback to MA20 volume dry-up + KD oversold turn + reversal bar",
	KeyStrategyBreakoutName:  "Trend Breakout",
	KeyStrategyPullbackName:  "Trend Pullback",
	KeyStrategyTrustFollow:   "🐳 %s triggered [Trust Follow] (%s): %d-day trust buying at %.1f%% of volume + above rising MA60",

	KeyRestrictedStockAlertTitle: "⚠️ *Disposition/Attention Stock Alert*\n\n",
	KeyRestrictedStockAlertLine:  "• *%s* is now under disposition/attention trading: %s\n",

	KeyPodcastUsage:         "Usage: /podcast <podcast transcript url>",
	KeyPodcastFetching:      "Fetching the episode content...",
	KeyPodcastFetchFailed:   "Fetch failed: %v",
	KeyPodcastAnalyzing:     "Analyzing market views in the transcript...",
	KeyPodcastAnalyzeFailed: "Analysis failed: %v",
	KeyPodcastNoInsights:    "No market-relevant views were found in this episode.",
	KeyPodcastSavedHeader:   "Extracted and saved %d view(s) from this episode:\n\n",
	KeyPodcastSavedLine:     "• %s [%s] %s\n",
	KeyPodcastMacroLabel:    "Macro",
	KeyPodcastPromptIntro:   "The user shared a stock-market podcast/video transcript. It may contain sponsor reads, small talk, or listener Q&A unrelated to investing. Only extract points from market-analysis passages — ignore ads, sponsor reads, and unrelated personal chatter.\n\nTitle: %s\nURL: %s\n\nTranscript:\n%s\n\n",
	KeyPodcastTaskBlock: `
## Task

Find every per-stock view and macro view mentioned in the transcript (leave the ticker empty when there's no single stock attached). Output one block per view, with no extra text:

[TICKER: symbol or empty]
%s US or TW (empty for a macro-only view)
%s Stance must be exactly one of BULLISH, BEARISH, NEUTRAL, WATCH
%s One sentence close to the original wording — do not embellish or infer beyond it (under 100 words)

If the transcript mentions growing demand or a tailwind for some ticker, and based on your own
industry/supply-chain knowledge there's a clear, plausible downstream beneficiary (US or TW listed),
output an extra block for that company too, with one more line:
%s <the ticker actually mentioned>: <why it benefits, e.g. what demand is growing>
This line marks the block as your own supply-chain inference, not a company the transcript named
directly — only output it when the connection is clear, don't guess just to fill space.

If nothing in the transcript is worth extracting, output nothing.
`,
	KeyPodcastMarketMarker:     "Market:",
	KeyPodcastStanceMarker:     "Stance:",
	KeyPodcastDerivedMarker:    "Derived from:",
	KeyPodcastDerivedLine:      "   └ derived from: %s (not directly named in the transcript)\n",
	KeyPodcastDuplicateWarning: "⚠️ This link has already been analyzed before (%d record(s) on file) — proceeding to analyze it again.",
}
