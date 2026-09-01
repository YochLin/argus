// docs/phase-5-web-dashboard.md's UI language decision: the backend never
// sends display strings (see api.ts), so the frontend picks a dictionary
// itself from /api/config's lang. Both dictionaries independently declare
// every key as a full object literal (not one spread from the other) so
// TypeScript's excess/missing-property checking on a typed literal catches
// a forgotten translation at compile time — the same discipline
// i18n_test.go's TestTablesMatch enforces on the Go side's zh/en tables,
// just carried by tsc instead of a unit test.
//
// Per the design doc, metric names stay English in both dictionaries
// (WIN RATE / PROFIT FACTOR / EXPECTANCY) — only supporting copy is
// translated.
export interface Dictionary {
  netPnL: string;
  winRate: string;
  profitFactor: string;
  expectancy: string;
  maxDrawdown: string;
  ytdReturn: string;
  qtdReturn: string;
  htdReturn: string;
  positions: string;
  ticker: string;
  shares: string;
  avgCost: string;
  price: string;
  marketValue: string;
  unrealizedPnL: string;
  watching: string;
  lastClose: string;
  loading: string;
  error: string;
  noPositions: string;
  navDashboard: string;
  navCalendar: string;
  weekTotal: string;
  monthTotal: string;
  noData: string;
  side: string;
  buy: string;
  sell: string;
  fee: string;
  realizedPnL: string;
  noTransactions: string;
  deleteTransaction: string;
  confirmDeleteTransaction: string;
  today: string;
  weekdays: [string, string, string, string, string, string, string];
  navRounds: string;
  startDate: string;
  endDate: string;
  open: string;
  noRounds: string;
  back: string;
  navChart: string;
  support: string;
  resistance: string;
  touches: string;
  lastTouch: string;
  levelType: string;
  noLevels: string;
  pickTicker: string;
  navReports: string;
  reportsByTicker: string;
  reportsByHoldingDays: string;
  reportsByEntryMonth: string;
  reportsByEntryWeekday: string;
  group: string;
  trades: string;
  avgReturn: string;
  totalPnL: string;
  avgHold: string;
  lowSampleTag: string;
  feeSummary: string;
  totalFees: string;
  feePctOfPnL: string;
  mfeCaptured: string;
  mfeCapturedNote: string;
  maeMfeRoundNote: string;
  tradeStats: string;
  bestTrade: string;
  worstTrade: string;
  avgWin: string;
  avgLoss: string;
  longestWinStreak: string;
  longestLossStreak: string;
  noReportData: string;
  reportsEdgeTitle: string;
  reportsEdgeSubtitle: string;
  reportsEdgeBothGood: string;
  reportsEdgeWinRateOnly: string;
  reportsEdgePayoffOnly: string;
  reportsEdgeBothWeak: string;
  payoffRatio: string;
  groupBreakdown: string;
  months: [string, string, string, string, string, string, string, string, string, string, string, string];
  navRisk: string;
  portfolioHeat: string;
  accountValue: string;
  cashLevel: string;
  weight: string;
  stopPriceCol: string;
  buyAlertPriceCol: string;
  openRisk: string;
  noStopSet: string;
  belowStop: string;
  benchmarkAlpha: string;
  myPortfolio: string;
  benchmarkReplay: string;
  cumPnl: string;
  drawdownChart: string;
  monthlyPnl: string;
  yearTotal: string;
  navRecs: string;
  navLlm: string;
  llmDevTag: string;
  llmSubtitle: string;
  llmKind: string;
  llmModel: string;
  llmLatency: string;
  llmCreated: string;
  llmWatchlist: string;
  llmCandidates: string;
  llmNews: string;
  llmRunRecommend: string;
  llmRunDailyReport: string;
  llmNoRuns: string;
  llmNewsMarket: string;
  llmNewsPerTicker: string;
  llmSource: string;
  llmHeadline: string;
  llmPublishedAt: string;
  llmBlockSource: string;
  llmBlocked: string;
  llmDataQuality: string;
  llmLowQualitySource: string;
  llmStaleNews: string;
  llmDuplicateTitles: string;
  llmDuplicateTitlesSameTicker: string;
  llmDuplicateTitlesCrossTicker: string;
  llmCandleGaps: string;
  llmNoSummaryRate: string;
  llmBlockedSources: string;
  llmUnblock: string;
  llmNoBlockedSources: string;
  llmBlockedHint: string;
  llmCandlesSummary: string;
  llmMarketContext: string;
  llmCrossTickerLessons: string;
  llmPerTickerLessons: string;
  llmStrategyHits: string;
  llmScanReason: string;
  llmPrevRecommendation: string;
  llmRawReply: string;
  llmScope: string;
  llmInsiderTx: string;
  llmStocksTitle: string;
  recCounts: string;
  recTotal: string;
  recScorable: string;
  recUnscorable: string;
  recHold: string;
  recBySource: string;
  recByAction: string;
  recBest: string;
  recWorst: string;
  horizonDays: string;
  hitRate: string;
  avgExcessReturn: string;
  noRecData: string;
  // Recs hero (Task #6): signal-checkup narrative card (built from fragments
  // since embedded numbers can't go through a plain dict lookup, same
  // convention as CalendarView's eventNote), followed-vs-skipped comparison
  // bars, excess-by-horizon chart, new stat labels, and source-value labels
  // for the four real recPerfExtreme/recPerfActiveSignal source strings
  // (watchlist/movers/scan/explore — there is no "news" source in the real
  // system despite the mockup showing one).
  recSignalCheckup: string;
  recFollowedVsSkipped: string;
  recFollowed: string;
  recSkipped: string;
  recExcessByHorizon: string;
  recSignalHitRate: string;
  recRandomBaseline: string;
  recBestHoldingWindow: string;
  recExcessReturnNote: string;
  recInsufficientData: string;
  recActiveSignals: string;
  recIssuedTime: string;
  recSource: string;
  recSourceWatchlist: string;
  recSourceMovers: string;
  recSourceScan: string;
  recSourceExplore: string;
  recEntryPrice: string;
  recDaysAgo: string;
  recSinceSignal: string;
  recNarrativePeakPrefix: string;
  recNarrativePeakMid: string;
  recNarrativePeakSuffix: string;
  recNarrativeSkipBetterPrefix: string;
  recNarrativeSkipBetterSuffix: string;
  recNarrativeFollowBetterPrefix: string;
  recNarrativeFollowBetterSuffix: string;
  thesisLabel: string;
  thesisAddToggle: string;
  thesisEditToggle: string;
  thesisFieldPlaceholder: string;
  thesisSaveFailedNote: string;
  thesisEmptyNote: string;
  lessonsLabel: string;
  rMultipleHistogram: string;
  rMultipleNote: string;
  rMultipleInfo: string;
  rMultipleXAxis: string;
  rMultipleYAxis: string;
  noStopSamples: string;
  noStopExplanation: string;
  holdingDaysScatter: string;
  holdingDaysInfo: string;
  holdingDaysXAxis: string;
  holdingDaysYAxis: string;
  maeReturnScatter: string;
  maeReturnNote: string;
  maeReturnInfo: string;
  maeXAxis: string;
  maeYAxis: string;
  skippedSamples: string;
  // themeLight/themeDark are TopBar's toggle-button label — shown for the
  // *action* the button performs (switch to X), not the current state, same
  // convention as Sidebar's market/lang pills labeling the destination.
  themeLight: string;
  themeDark: string;
  // Phase 10 (docs/phase-10-web-trade-input.md §4.3): TradeModal/LoginModal
  // copy plus PositionsTable/ChartListView's new write-affordance labels.
  // Server-side confirmation/error text (e.g. TradeResponse.message) is
  // never re-translated here — these keys are only for chrome the backend
  // never sends a string for.
  addTrade: string;
  tradeBuyTitle: string;
  tradeSellTitle: string;
  tradeStopTitle: string;
  tradeBuyAlertTitle: string;
  advancedOptions: string;
  tradeDate: string;
  submit: string;
  cancel: string;
  close: string;
  loginTitle: string;
  password: string;
  login: string;
  addTickerPlaceholder: string;
  add: string;
  remove: string;
  addBuyAlert: string;
  searchPlaceholder: string;
  watchlistCount: string;
  noMatch: string;
  heldOnly: string;
  nearestSup: string;
  nearestRes: string;
  ma20: string;
  ma60: string;
  atr14: string;
  ret20: string;
  fromHigh: string;
  vsAvg20: string;
  volume: string;
  above: string;
  below: string;
  rangeHigh: string;
  rangeLow: string;
  rangeNote: string;
  thisPosition: string;
  noPositionHere: string;
  riskIfStopped: string;
  pctOfAccount: string;
  tickerRounds: string;
  noRoundsHere: string;
  roundPicker: string;
  tradesInRound: string;
  allTrades: string;
  // Calendar's earnings-event dots/legend/day-detail table (Task #9) — note
  // text is built client-side from kind/hour/estimated (see api.ts's
  // CalendarEvent), same "backend never sends display strings" rule.
  eventsTitle: string;
  eventType: string;
  eventNote: string;
  noEventsToday: string;
  heldLegend: string;
  eventKindEarnings: string;
  eventHeld: string;
  eventHourBmo: string;
  eventHourAmc: string;
  eventHourDmh: string;
  eventHourUnknown: string;
  eventEstimated: string;
  // Phase 17: the connection/credential settings page. Field labels are the
  // env var names themselves (rendered from /api/settings), so only the
  // section headings and the surrounding copy live here — adding a variable
  // server-side needs no new key unless it introduces a new group.
  navSettings: string;
  // setNavNote/acctDevMode/acctDevNote back the account-switcher dropdown's
  // two static entries (design-canvas reference's account menu — see
  // Sidebar.tsx's sidebar-account-menu). Real multi-account switching isn't
  // built yet, so the dropdown only ever shows these two rows. devMode
  // (App.tsx, persisted client-side) gates the /llm nav link + route —
  // App.tsx's own comment on devModeStorageKey has the full rationale.
  setNavNote: string;
  acctDevMode: string;
  acctDevNote: string;
  settingsTitle: string;
  settingsIntro: string;
  settingsGroupTelegram: string;
  settingsGroupData: string;
  settingsGroupSinopac: string;
  settingsSinopacDaemonNote: string;
  settingsSecretSet: string;
  settingsSecretUnset: string;
  settingsSave: string;
  settingsRevert: string;
  settingsDirty: string;
  settingsRulesTitle: string;
  settingsRule1: string;
  settingsRule2: string;
  settingsRestarting: string;
  settingsReload: string;
  // Phase 5 §B (optional): CSV transaction import page (nav link gated on
  // status.writable, same convention as every other write-only entry point).
  navImport: string;
  importTitle: string;
  importInstructions: string;
  importTemplateHint: string;
  importTextareaPlaceholder: string;
  importChooseFile: string;
  importPreview: string;
  importConfirm: string;
  importLine: string;
  importDate: string;
  importStatus: string;
  importMessage: string;
  importStatusOk: string;
  importStatusWarning: string;
  importStatusDuplicate: string;
  importStatusError: string;
  importStatusApplied: string;
  importNoRows: string;
  importAppliedPrefix: string;
  importAppliedSuffix: string;
  // Phase 11 PR4 (docs/phase-11-paper-account.md §7.2): the read-only Paper
  // Account page — sidebar nav link gated on /api/config's paperEnabled,
  // same "hidden entirely when the feature is off" convention as navImport.
  navPaper: string;
  paperReadOnlyBadge: string;
  paperReadOnlyNotice: string;
  paperEquity: string;
  paperInitialCash: string;
  paperTotalReturn: string;
  paperBenchmarkReturn: string;
  paperAlpha: string;
  paperSince: string;
  closedPositions: string;
  entryDate: string;
  exitDate: string;
  exitPrice: string;
  exitReason: string;
  exitReasonStop: string;
  exitReasonLlmSell: string;
  distToStop: string;
  noClosedPositions: string;
  // Phase 12 PR4: the read-only Options page — always in the sidebar (no
  // feature flag; the options ledger is always present, just usually
  // empty), unlike navPaper's paperEnabled gating.
  navOptions: string;
  optionContract: string;
  optionRight: string;
  optionStrike: string;
  optionExpiry: string;
  optionDTE: string;
  optionContracts: string;
  optionAvgPremium: string;
  optionMark: string;
  optionMarketValue: string;
  optionDelta: string;
  optionAction: string;
  optionCalendar: string;
  optionCollateral: string;
  optionLockedCash: string;
  optionLockedShares: string;
  optionHeldShares: string;
  optionNaked: string;
  noOptionPositions: string;
  noClosedOptions: string;
  noOptionCollateral: string;
  // Phase 12 PR4's write half (design-parity pass): Add/Close modals wired
  // to POST /api/options/open|close (internal/web/options.go), same
  // requireTrade/writable gate as PositionsTable's buy/sell/stop buttons.
  optAddBtn: string;
  optCloseBtn: string;
  optionCollateralNote: string;
  optionNoPnlNote: string;
  optAddTitle: string;
  optCloseTitle: string;
  optFieldPremium: string;
  optOutcomeHint: string;
  optBuyToClose: string;
  optSellToClose: string;
  optExpiredBtn: string;
  optAssignedBtn: string;
  optExercisedBtn: string;
  optCall: string;
  optPut: string;
  // Phase 18: the sector money-flow treemap page.
  navFlow: string;
  sectorFlowSubtitle: string;
  sectorFlowHeldHint: string;
  sectorFlowNotReady: string;
  sectorFlowSizeBy: string;
  sectorFlowSizeByCap: string;
  sectorFlowSizeByFlow: string;
  sectorFlowHeld: string;
  sectorFlowNetFlow: string;
  sectorFlowChange: string;
  sectorFlowTickerCount: string;
  sectorFlowRanking: string;
  sectorFlowTWCapNote: string;
  sectorFlowTotalNetFlow: string;
  sectorFlowBreadth: string;
  sectorFlowStrongest: string;
  sectorFlowWeakest: string;
  sectorFlowRefresh: string;
  sectorFlowRefreshing: string;
  sectorFlowRefreshError: string;
}

const en: Dictionary = {
  netPnL: "NET P&L",
  winRate: "WIN RATE",
  profitFactor: "PROFIT FACTOR",
  expectancy: "EXPECTANCY",
  maxDrawdown: "MAX DRAWDOWN",
  ytdReturn: "YTD RETURN",
  qtdReturn: "QTD RETURN",
  htdReturn: "HTD RETURN",
  positions: "Positions",
  ticker: "Ticker",
  shares: "Shares",
  avgCost: "Avg Cost",
  price: "Price",
  marketValue: "Market Value",
  unrealizedPnL: "Unrealized P&L",
  watching: "WATCHING",
  lastClose: "LAST CLOSE",
  loading: "Loading…",
  error: "Failed to load dashboard.",
  noPositions: "No open positions.",
  navDashboard: "Dashboard",
  navCalendar: "Calendar",
  weekTotal: "Week",
  monthTotal: "Month total",
  noData: "No data",
  side: "Side",
  buy: "BUY",
  sell: "SELL",
  fee: "Fee",
  realizedPnL: "Realized P&L",
  noTransactions: "No transactions this day.",
  deleteTransaction: "Delete (only the ticker's most recent transaction can be deleted)",
  confirmDeleteTransaction: "Delete this transaction? This can only be undone by re-entering the trade.",
  today: "Today",
  weekdays: ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"],
  navRounds: "Rounds",
  startDate: "Start",
  endDate: "End",
  open: "OPEN",
  noRounds: "No trade rounds yet.",
  back: "‹ Back",
  navChart: "Chart",
  support: "Support",
  resistance: "Resistance",
  touches: "Touches",
  lastTouch: "Last touch",
  levelType: "Type",
  noLevels: "No significant levels",
  pickTicker: "Pick a ticker",
  navReports: "Reports",
  reportsByTicker: "By Ticker",
  reportsByHoldingDays: "By Holding Period",
  reportsByEntryMonth: "By Entry Month",
  reportsByEntryWeekday: "By Entry Weekday",
  group: "Group",
  trades: "N",
  avgReturn: "Avg Return",
  totalPnL: "Total P&L",
  avgHold: "Avg Hold (d)",
  lowSampleTag: "low sample",
  feeSummary: "Fees",
  totalFees: "Total Fees",
  feePctOfPnL: "of Realized P&L",
  mfeCaptured: "Avg MFE Captured",
  mfeCapturedNote: "of the best paper gain during each closed round, on average, how much did the exit actually capture (day-high/low approximation)",
  maeMfeRoundNote: "Max adverse/favorable excursion during this round's holding period (day-high/low approximation)",
  tradeStats: "Trade Stats",
  bestTrade: "Best Trade",
  worstTrade: "Worst Trade",
  avgWin: "Avg Win",
  avgLoss: "Avg Loss",
  longestWinStreak: "Longest Win Streak",
  longestLossStreak: "Longest Loss Streak",
  noReportData: "No closed trades yet.",
  reportsEdgeTitle: "Performance Checkup",
  reportsEdgeSubtitle: "Avg Win vs Avg Loss",
  reportsEdgeBothGood: "Win rate and payoff both favor you — you win more often, and each win is bigger than each loss.",
  reportsEdgeWinRateOnly: "Win rate favors you, but losses are eating the gains — cut losses earlier.",
  reportsEdgePayoffOnly: "Most trades lose, but wins are much bigger than losses — a handful of big winners is carrying the results.",
  reportsEdgeBothWeak: "Neither win rate nor payoff favors you right now — exit discipline is the biggest gap.",
  payoffRatio: "PAYOFF RATIO (WIN/LOSS)",
  groupBreakdown: "Group Breakdown",
  months: ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"],
  navRisk: "Risk",
  portfolioHeat: "PORTFOLIO HEAT",
  accountValue: "ACCOUNT VALUE",
  cashLevel: "CASH LEVEL",
  weight: "Weight",
  stopPriceCol: "Stop",
  buyAlertPriceCol: "Buy Level",
  openRisk: "Open Risk",
  noStopSet: "no stop",
  belowStop: "below stop",
  benchmarkAlpha: "ALPHA VS BENCHMARK",
  myPortfolio: "My portfolio",
  benchmarkReplay: "Same cash flow into SPY/0050",
  cumPnl: "Cumulative P&L",
  drawdownChart: "DRAWDOWN",
  monthlyPnl: "MONTHLY P&L",
  yearTotal: "Total",
  navRecs: "Recs",
  navLlm: "LLM Audit",
  llmDevTag: "DEV",
  llmSubtitle: "Exactly what was sent to the model for each /recommend or daily report call.",
  llmKind: "Kind",
  llmModel: "Model",
  llmLatency: "Latency",
  llmCreated: "Created",
  llmWatchlist: "Watchlist",
  llmCandidates: "Candidates",
  llmNews: "News",
  llmRunRecommend: "/recommend",
  llmRunDailyReport: "Daily report",
  llmNoRuns: "No LLM runs recorded yet.",
  llmNewsMarket: "Market news",
  llmNewsPerTicker: "Per-ticker news",
  llmSource: "Source",
  llmHeadline: "Headline",
  llmPublishedAt: "Published",
  llmBlockSource: "Block",
  llmBlocked: "Blocked",
  llmDataQuality: "Data quality",
  llmLowQualitySource: "Low-quality source",
  llmStaleNews: "Stale (>72h)",
  llmDuplicateTitles: "Duplicate",
  llmDuplicateTitlesSameTicker: "Duplicate (same ticker)",
  llmDuplicateTitlesCrossTicker: "Duplicate (cross-ticker)",
  llmCandleGaps: "Candle gaps",
  llmNoSummaryRate: "No summary",
  llmBlockedSources: "Blocked news sources",
  llmUnblock: "Unblock",
  llmNoBlockedSources: "No sources blocked yet.",
  llmBlockedHint: "History records what was actually sent — blocking a source later never rewrites it.",
  llmCandlesSummary: "Candles",
  llmMarketContext: "Market context",
  llmCrossTickerLessons: "Cross-ticker lessons",
  llmPerTickerLessons: "Past lessons (this ticker)",
  llmStrategyHits: "Strategy hits",
  llmScanReason: "Scan reason",
  llmPrevRecommendation: "Previous recommendation",
  llmRawReply: "Raw model reply",
  llmScope: "Scope",
  llmInsiderTx: "Insider transactions",
  llmStocksTitle: "Stocks",
  recCounts: "RECOMMENDATION COVERAGE",
  recTotal: "Total",
  recScorable: "Scorable",
  recUnscorable: "Unscorable",
  recHold: "Hold",
  recBySource: "By Source",
  recByAction: "By Action",
  recBest: "Best",
  recWorst: "Worst",
  horizonDays: "Horizon (days)",
  hitRate: "Hit Rate",
  avgExcessReturn: "Avg Excess Return",
  noRecData: "No scorable recommendations yet.",
  recSignalCheckup: "Signal Checkup",
  recFollowedVsSkipped: "Followed vs Skipped",
  recFollowed: "Followed",
  recSkipped: "Skipped",
  recExcessByHorizon: "Excess Return by Holding Days",
  recSignalHitRate: "Signal Hit Rate",
  recRandomBaseline: "Random baseline",
  recBestHoldingWindow: "Best Holding Window",
  recExcessReturnNote: "excess return",
  recInsufficientData: "Insufficient data to score",
  recActiveSignals: "Active Signals",
  recIssuedTime: "Issued",
  recSource: "Source",
  recSourceWatchlist: "Watchlist",
  recSourceMovers: "Movers",
  recSourceScan: "Scan",
  recSourceExplore: "LLM nomination",
  recEntryPrice: "Entry Price",
  recDaysAgo: "days ago",
  recSinceSignal: "Since Signal",
  recNarrativePeakPrefix: "Signal peaks on trading day ",
  recNarrativePeakMid: " (beats market by ",
  recNarrativePeakSuffix: ").",
  recNarrativeSkipBetterPrefix: "Signals you skipped earned ",
  recNarrativeSkipBetterSuffix:
    " more than the ones you followed — your extra filtering is costing you.",
  recNarrativeFollowBetterPrefix: "Signals you followed earned ",
  recNarrativeFollowBetterSuffix:
    " more than the ones you skipped — your filtering is working.",
  thesisLabel: "THESIS",
  thesisAddToggle: "Record entry thesis (optional)",
  thesisEditToggle: "Add thesis entry",
  thesisFieldPlaceholder: "Your reasoning right now",
  thesisSaveFailedNote: "Trade went through, but the thesis note failed to save",
  thesisEmptyNote: "No thesis recorded for this round yet.",
  lessonsLabel: "TRADE LESSONS",
  rMultipleHistogram: "R-MULTIPLE DISTRIBUTION",
  rMultipleNote: "R data accumulating since",
  rMultipleInfo:
    "Each closed round's realized P&L divided by its initial risk (entry price minus stop price) — shows whether your winners are actually outsized relative to what you risked.",
  rMultipleXAxis: "R Multiple",
  rMultipleYAxis: "Count",
  noStopSamples: "no-stop samples",
  noStopExplanation: "These closed rounds were bought without a stop-loss set, so R can't be computed.",
  holdingDaysScatter: "HOLDING DAYS VS REALIZED P&L",
  holdingDaysInfo:
    "How long each closed round was held vs. what it realized — points near the top are fast winners, points to the right took a long time to pay off (or didn't).",
  holdingDaysXAxis: "Holding Days",
  holdingDaysYAxis: "Realized P&L",
  maeReturnScatter: "MAE VS RETURN (CLOSE-ONLY APPROXIMATION)",
  maeReturnNote: "x = deepest adverse close % from entry, y = final return % — close prices only, intraday extremes aren't available",
  maeReturnInfo:
    "How far a round drew down against you (MAE) vs. what it ultimately returned — upper-left had a rough ride to a good outcome, lower-right gave back gains after holding up well.",
  maeXAxis: "MAE %",
  maeYAxis: "Return %",
  skippedSamples: "skipped (no snapshot data)",
  themeLight: "Light",
  themeDark: "Dark",
  addTrade: "+ Trade",
  tradeBuyTitle: "Buy",
  tradeSellTitle: "Sell",
  tradeStopTitle: "Set Stop",
  tradeBuyAlertTitle: "Set Buy Level",
  advancedOptions: "Advanced",
  tradeDate: "Date",
  submit: "Submit",
  cancel: "Cancel",
  close: "Close",
  loginTitle: "Log In",
  password: "Password",
  login: "Log In",
  addTickerPlaceholder: "Add ticker…",
  add: "Add",
  remove: "Remove",
  addBuyAlert: "Buy Level",
  searchPlaceholder: "Search ticker or name…",
  watchlistCount: "Watchlist",
  noMatch: "No matching tickers found.",
  heldOnly: "Holdings only",
  nearestSup: "Nearest Support",
  nearestRes: "Nearest Resistance",
  ma20: "MA20",
  ma60: "MA60",
  atr14: "ATR(14)",
  ret20: "20D Return",
  fromHigh: "From High",
  vsAvg20: "vs 20D Avg Vol",
  volume: "Volume",
  above: "Above",
  below: "Below",
  rangeHigh: "120D High",
  rangeLow: "120D Low",
  rangeNote: "120D Range Position",
  thisPosition: "Current Position",
  noPositionHere: "No active position in this ticker.",
  riskIfStopped: "Risk if Stopped",
  pctOfAccount: "Account %",
  tickerRounds: "Historical Rounds",
  noRoundsHere: "No closed rounds for this ticker.",
  roundPicker: "Select Round",
  tradesInRound: "Trades in Round",
  allTrades: "All Trades",
  eventsTitle: "EVENTS",
  eventType: "Type",
  eventNote: "Detail",
  noEventsToday: "No events this day.",
  heldLegend: "ring = position held",
  eventKindEarnings: "Earnings",
  eventHeld: "HELD",
  eventHourBmo: "before open",
  eventHourAmc: "after close",
  eventHourDmh: "during hours",
  eventHourUnknown: "time TBD",
  eventEstimated: "estimated",
  navSettings: "Settings",
  setNavNote: "Connections & credentials",
  acctDevMode: "Dev mode",
  acctDevNote: "Show entries not meant for everyday use",
  settingsTitle: "Connections",
  settingsIntro:
    "Credentials for the services Argus talks to. Saving rewrites .env and restarts the bot, which takes a few seconds.",
  settingsGroupTelegram: "Telegram",
  settingsGroupData: "Data sources",
  settingsGroupSinopac: "Sinopac Securities",
  settingsSinopacDaemonNote:
    "SJ_API_KEY / SJ_SEC_KEY are read by the shioaji daemon, not by Argus. After saving, also run: systemctl --user restart shioaji",
  settingsSecretSet: "Configured — leave blank to keep it",
  settingsSecretUnset: "Not configured",
  settingsSave: "Save & restart",
  settingsRevert: "Revert",
  settingsDirty: "Unsaved changes",
  settingsRulesTitle: "Notes",
  settingsRule1: "A blank field keeps its current value — there's no way to clear one here.",
  settingsRule2: "Saving any field restarts Argus to apply it.",
  settingsRestarting: "Saved. The bot is restarting…",
  settingsReload: "Reload",
  navImport: "Import",
  importTitle: "Import Transactions",
  importInstructions: "Paste or upload CSV transactions to backfill your trade history.",
  importTemplateHint: "Columns: date,ticker,action,shares,price,fee (fee optional, header row required)",
  importTextareaPlaceholder: "date,ticker,action,shares,price,fee\n2026-01-05,AAPL,BUY,10,150,1.5",
  importChooseFile: "Upload CSV",
  importPreview: "Preview",
  importConfirm: "Confirm Import",
  importLine: "Line",
  importDate: "Date",
  importStatus: "Status",
  importMessage: "Message",
  importStatusOk: "OK",
  importStatusWarning: "Warning",
  importStatusDuplicate: "Duplicate",
  importStatusError: "Error",
  importStatusApplied: "Applied",
  importNoRows: "No rows to show yet — paste CSV and preview.",
  importAppliedPrefix: "Applied ",
  importAppliedSuffix: " row(s).",
  navPaper: "Paper Account",
  paperReadOnlyBadge: "READ-ONLY",
  paperReadOnlyNotice: "Executed automatically by the live strategy. No manual trading here.",
  paperEquity: "EQUITY",
  paperInitialCash: "Initial Cash",
  paperTotalReturn: "TOTAL RETURN",
  paperBenchmarkReturn: "BENCHMARK",
  paperAlpha: "ALPHA",
  paperSince: "Since",
  closedPositions: "Closed Positions",
  entryDate: "Entry",
  exitDate: "Exit",
  exitPrice: "Exit Price",
  exitReason: "Exit Reason",
  exitReasonStop: "Stop",
  exitReasonLlmSell: "LLM Sell",
  distToStop: "Dist. to Stop",
  noClosedPositions: "No closed positions yet.",
  navOptions: "Options",
  optionContract: "Contract",
  optionRight: "Right",
  optionStrike: "Strike",
  optionExpiry: "Expiry",
  optionDTE: "DTE",
  optionContracts: "Contracts",
  optionAvgPremium: "Avg Premium",
  optionMark: "Mark",
  optionMarketValue: "Market Value",
  optionDelta: "Delta",
  optionAction: "Action",
  optionCalendar: "Expiry Calendar",
  optionCollateral: "Collateral",
  optionLockedCash: "CSP Locked Cash",
  optionLockedShares: "Locked Shares",
  optionHeldShares: "Held Shares",
  optionNaked: "NAKED",
  noOptionPositions: "No open option positions.",
  noClosedOptions: "No closed option trades yet.",
  noOptionCollateral: "No collateral obligations.",
  optAddBtn: "+ Add",
  optCloseBtn: "Close",
  optionCollateralNote: "CSP locks cash; a covered call locks shares. Naked means locked shares exceed what you hold.",
  optionNoPnlNote: "Realized P&L only — options have no daily market-value history to chart.",
  optAddTitle: "Add Option Position",
  optCloseTitle: "Close Position",
  optFieldPremium: "Premium",
  optOutcomeHint: "What happened to this contract?",
  optBuyToClose: "Buy to Close",
  optSellToClose: "Sell to Close",
  optExpiredBtn: "Expired",
  optAssignedBtn: "Assigned",
  optExercisedBtn: "Exercised",
  optCall: "Call",
  optPut: "Put",
  navFlow: "Money Flow",
  sectorFlowSubtitle: "Sector × market-cap money-flow heatmap — block size is market cap, color is change.",
  sectorFlowHeldHint: "Outlined blocks = your positions. Click a block to view its chart.",
  sectorFlowNotReady: "Sector data isn't ready yet — check back after the next scan.",
  sectorFlowSizeBy: "Size by",
  sectorFlowSizeByCap: "Market Cap",
  sectorFlowSizeByFlow: "Money Flow",
  sectorFlowHeld: "Held",
  sectorFlowNetFlow: "Net Flow",
  sectorFlowChange: "Change",
  sectorFlowTickerCount: "Tickers",
  sectorFlowRanking: "Sector Money Flow Ranking",
  sectorFlowTWCapNote: "TW block size uses trading value (no free market-cap source), not real market cap.",
  sectorFlowTotalNetFlow: "Total Net Flow",
  sectorFlowBreadth: "Advancers / Decliners",
  sectorFlowStrongest: "Strongest Sector",
  sectorFlowWeakest: "Weakest Sector",
  sectorFlowRefresh: "Trigger Scan Now",
  sectorFlowRefreshing: "Scan started — this can take several minutes for US (~500 tickers). Reload this page to check.",
  sectorFlowRefreshError: "Failed to start the scan.",
};

const zh: Dictionary = {
  netPnL: "NET P&L",
  winRate: "WIN RATE",
  profitFactor: "PROFIT FACTOR",
  expectancy: "EXPECTANCY",
  maxDrawdown: "MAX DRAWDOWN",
  ytdReturn: "YTD RETURN",
  qtdReturn: "QTD RETURN",
  htdReturn: "HTD RETURN",
  positions: "持倉",
  ticker: "標的",
  shares: "股數",
  avgCost: "平均成本",
  price: "現價",
  marketValue: "市值",
  unrealizedPnL: "未實現損益",
  watching: "WATCHING",
  lastClose: "LAST CLOSE",
  loading: "載入中…",
  error: "儀表板載入失敗。",
  noPositions: "目前沒有持倉。",
  navDashboard: "儀表板",
  navCalendar: "月曆",
  weekTotal: "週合計",
  monthTotal: "當月合計",
  noData: "無資料",
  side: "動作",
  buy: "購買",
  sell: "賣出",
  fee: "手續費",
  realizedPnL: "已實現損益",
  noTransactions: "當天沒有交易紀錄。",
  deleteTransaction: "刪除（僅能刪除該股票最近一筆交易紀錄）",
  confirmDeleteTransaction: "確定要刪除這筆交易紀錄嗎？之後只能靠重新輸入交易來還原。",
  today: "今天",
  weekdays: ["日", "一", "二", "三", "四", "五", "六"],
  navRounds: "回合",
  startDate: "起始",
  endDate: "結束",
  open: "進行中",
  noRounds: "目前沒有交易回合。",
  back: "‹ 返回",
  navChart: "個股圖",
  support: "支撐",
  resistance: "壓力",
  touches: "觸碰次數",
  lastTouch: "最後觸碰",
  levelType: "類型",
  noLevels: "無明顯支撐/壓力位",
  pickTicker: "選擇標的",
  navReports: "績效報表",
  reportsByTicker: "依標的",
  reportsByHoldingDays: "依持有天數",
  reportsByEntryMonth: "依進場月份",
  reportsByEntryWeekday: "依進場星期幾",
  group: "分組",
  trades: "筆數",
  avgReturn: "平均報酬%",
  totalPnL: "總損益",
  avgHold: "平均持有(天)",
  lowSampleTag: "樣本不足",
  feeSummary: "手續費彙總",
  totalFees: "總手續費",
  feePctOfPnL: "佔已實現損益比例",
  mfeCaptured: "平均 MFE 實現比例",
  mfeCapturedNote: "每個已平倉回合期間帳面最大浮盈中，出場平均實際兌現了多少（以日高低近似）",
  maeMfeRoundNote: "此回合持有期間的最大帳面浮虧／浮盈（以日高低近似）",
  tradeStats: "交易統計",
  bestTrade: "最佳單筆",
  worstTrade: "最差單筆",
  avgWin: "平均獲利",
  avgLoss: "平均虧損",
  longestWinStreak: "最長連勝",
  longestLossStreak: "最長連敗",
  noReportData: "尚無已平倉交易。",
  reportsEdgeTitle: "績效體檢",
  reportsEdgeSubtitle: "平均獲利 VS 平均虧損",
  reportsEdgeBothGood: "勝率與賠率同時站在你這邊——獲利筆數多、單筆也賺得比虧得多。",
  reportsEdgeWinRateOnly: "勝率站在你這邊，但單筆虧損吃掉了獲利——出場停損應該再更早一點。",
  reportsEdgePayoffOnly: "多數交易是虧損的，但獲利單筆遠大於虧損單筆——目前績效靠少數大賺撐住。",
  reportsEdgeBothWeak: "勝率與賠率目前都不利——出場紀律是最大的漏洞。",
  payoffRatio: "賠率（獲利/虧損）",
  groupBreakdown: "分組拆解",
  months: ["1月", "2月", "3月", "4月", "5月", "6月", "7月", "8月", "9月", "10月", "11月", "12月"],
  navRisk: "風險",
  portfolioHeat: "PORTFOLIO HEAT",
  accountValue: "帳戶總值",
  cashLevel: "現金水位",
  weight: "占比",
  stopPriceCol: "停損價",
  buyAlertPriceCol: "購買點位",
  openRisk: "Open Risk",
  noStopSet: "未設停損",
  belowStop: "已跌破停損",
  benchmarkAlpha: "超額報酬 vs 大盤",
  myPortfolio: "我的組合",
  benchmarkReplay: "同金流買 SPY／0050",
  cumPnl: "累積損益",
  drawdownChart: "水下曲線",
  monthlyPnl: "月度損益",
  yearTotal: "年度合計",
  navRecs: "推薦成效",
  navLlm: "LLM 稽核",
  llmDevTag: "開發用",
  llmSubtitle: "每次 /recommend 或每日報告實際送給模型的完整輸入內容。",
  llmKind: "類型",
  llmModel: "模型",
  llmLatency: "耗時",
  llmCreated: "時間",
  llmWatchlist: "追蹤清單",
  llmCandidates: "候選標的",
  llmNews: "新聞",
  llmRunRecommend: "/recommend",
  llmRunDailyReport: "每日報告",
  llmNoRuns: "尚無 LLM 執行紀錄。",
  llmNewsMarket: "市場新聞",
  llmNewsPerTicker: "個股新聞",
  llmSource: "來源",
  llmHeadline: "標題",
  llmPublishedAt: "發布時間",
  llmBlockSource: "封鎖",
  llmBlocked: "已封鎖",
  llmDataQuality: "資料品質檢查",
  llmLowQualitySource: "低品質來源",
  llmStaleNews: "過期新聞（>72h）",
  llmDuplicateTitles: "重複",
  llmDuplicateTitlesSameTicker: "重複標題（同檔）",
  llmDuplicateTitlesCrossTicker: "重複標題（跨檔）",
  llmCandleGaps: "K 線缺漏",
  llmNoSummaryRate: "無摘要",
  llmBlockedSources: "已封鎖的新聞來源",
  llmUnblock: "解除封鎖",
  llmNoBlockedSources: "目前沒有封鎖任何來源。",
  llmBlockedHint: "歷史紀錄保留實際送出的內容，之後封鎖來源不會改寫歷史。",
  llmCandlesSummary: "K 線",
  llmMarketContext: "市場背景",
  llmCrossTickerLessons: "跨標的教訓",
  llmPerTickerLessons: "過去教訓（此標的）",
  llmStrategyHits: "策略命中",
  llmScanReason: "掃描原因",
  llmPrevRecommendation: "前次建議",
  llmRawReply: "模型原始回覆",
  llmScope: "標的",
  llmInsiderTx: "內部人交易",
  llmStocksTitle: "個股",
  recCounts: "推薦覆蓋率",
  recTotal: "總筆數",
  recScorable: "可評分",
  recUnscorable: "無法評分",
  recHold: "HOLD",
  recBySource: "依來源",
  recByAction: "依動作",
  recBest: "表現最佳",
  recWorst: "表現最差",
  horizonDays: "交易日視窗",
  hitRate: "命中率",
  avgExcessReturn: "平均超額報酬",
  noRecData: "尚無可評分的推薦紀錄。",
  recSignalCheckup: "訊號體檢",
  recFollowedVsSkipped: "跟進 VS 略過",
  recFollowed: "有跟進",
  recSkipped: "略過",
  recExcessByHorizon: "超額報酬隨持有天數變化",
  recSignalHitRate: "訊號命中率",
  recRandomBaseline: "隨機基準",
  recBestHoldingWindow: "最佳持有視窗",
  recExcessReturnNote: "超額報酬",
  recInsufficientData: "資料不足無法評分",
  recActiveSignals: "目前有效訊號",
  recIssuedTime: "發出時間",
  recSource: "來源",
  recSourceWatchlist: "追蹤清單",
  recSourceMovers: "漲跌幅榜",
  recSourceScan: "選股條件",
  recSourceExplore: "LLM 提名",
  recEntryPrice: "進場價",
  recDaysAgo: "天前",
  recSinceSignal: "訊號後至今",
  recNarrativePeakPrefix: "訊號在第 ",
  recNarrativePeakMid: " 個交易日達到最佳（超越大盤 ",
  recNarrativePeakSuffix: "）。",
  recNarrativeSkipBetterPrefix: "你略過的訊號比你實際跟進的多賺 ",
  recNarrativeSkipBetterSuffix: " ——你額外加的篩選反而在扣分。",
  recNarrativeFollowBetterPrefix: "你跟進的訊號比你略過的多賺 ",
  recNarrativeFollowBetterSuffix: " ——你的篩選確實有效。",
  thesisLabel: "持有論點",
  thesisAddToggle: "記錄進場論點（選填）",
  thesisEditToggle: "新增論點",
  thesisFieldPlaceholder: "現在的判斷是什麼",
  thesisSaveFailedNote: "買單已成立，但論點寫入失敗",
  thesisEmptyNote: "此回合尚未記錄任何論點。",
  lessonsLabel: "交易教訓",
  rMultipleHistogram: "R-MULTIPLE 分布",
  rMultipleNote: "R 資料自",
  rMultipleInfo: "每個已平倉回合的已實現損益除以初始風險（進場價−停損價）——看你的獲利是否真的相對於承擔的風險放大。",
  rMultipleXAxis: "R 值",
  rMultipleYAxis: "筆數",
  noStopSamples: "無停損樣本",
  noStopExplanation: "這些已平倉回合買進時沒有設定停損，因此無法計算 R 值。",
  holdingDaysScatter: "持有天數 vs 已實現損益",
  holdingDaysInfo: "每個已平倉回合的持有天數 vs 最終損益——點越靠上代表獲利越快，點越靠右代表拖了很久才有結果（或沒有結果）。",
  holdingDaysXAxis: "持有天數",
  holdingDaysYAxis: "已實現損益",
  maeReturnScatter: "MAE vs 報酬（以收盤價近似）",
  maeReturnNote: "x = 期間內對進場價最深不利收盤 %，y = 最終報酬 %——僅收盤價，無盤中極值",
  maeReturnInfo: "回合期間對你最不利的收盤價跌幅（MAE）vs 最終報酬——左上代表過程震盪但結果不錯，右下代表撐住了卻後來把獲利吐回去。",
  maeXAxis: "MAE %",
  maeYAxis: "報酬 %",
  skippedSamples: "略過（無快照資料）",
  themeLight: "日間",
  themeDark: "夜間",
  addTrade: "＋交易",
  tradeBuyTitle: "買入",
  tradeSellTitle: "賣出",
  tradeStopTitle: "設定停損",
  tradeBuyAlertTitle: "設定購買點位",
  advancedOptions: "進階選項",
  tradeDate: "日期",
  submit: "送出",
  cancel: "取消",
  close: "關閉",
  loginTitle: "登入",
  password: "密碼",
  login: "登入",
  addTickerPlaceholder: "新增標的…",
  add: "新增",
  remove: "移除",
  addBuyAlert: "購買點位",
  searchPlaceholder: "搜尋標的或名稱…",
  watchlistCount: "追蹤清單",
  noMatch: "找不到符合的標的。",
  heldOnly: "已持倉",
  nearestSup: "最近支撐",
  nearestRes: "最近壓力",
  ma20: "MA20",
  ma60: "MA60",
  atr14: "ATR(14)",
  ret20: "20日報酬",
  fromHigh: "距高點",
  vsAvg20: "較20日均量",
  volume: "成交量",
  above: "高於",
  below: "低於",
  rangeHigh: "120日高點",
  rangeLow: "120日低點",
  rangeNote: "120日區間位置",
  thisPosition: "目前持倉",
  noPositionHere: "目前無此標的持倉。",
  riskIfStopped: "觸及停損風險",
  pctOfAccount: "帳戶占比",
  tickerRounds: "歷史交易回合",
  noRoundsHere: "此標的尚無已平倉回合。",
  roundPicker: "選擇回合",
  tradesInRound: "回合內交易記錄",
  allTrades: "所有交易",
  eventsTitle: "重大事件",
  eventType: "類型",
  eventNote: "說明",
  noEventsToday: "當天沒有重大事件。",
  heldLegend: "外圈＝持倉標的",
  eventKindEarnings: "財報",
  eventHeld: "持倉中",
  eventHourBmo: "盤前公布",
  eventHourAmc: "盤後公布",
  eventHourDmh: "盤中公布",
  eventHourUnknown: "時間未定",
  eventEstimated: "推估",
  navSettings: "設定",
  setNavNote: "連線與憑證",
  acctDevMode: "開發者模式",
  acctDevNote: "顯示不對一般使用開放的項目",
  settingsTitle: "連線設定",
  settingsIntro: "Argus 對外連線用的憑證。存檔會改寫 .env 並重新啟動 bot，約需數秒。",
  settingsGroupTelegram: "Telegram",
  settingsGroupData: "資料來源",
  settingsGroupSinopac: "永豐證券",
  settingsSinopacDaemonNote:
    "SJ_API_KEY／SJ_SEC_KEY 是 shioaji daemon 讀的，不是 Argus。存檔後請另外執行：systemctl --user restart shioaji",
  settingsSecretSet: "已設定 — 留空即保留原值",
  settingsSecretUnset: "未設定",
  settingsSave: "儲存並重啟",
  settingsRevert: "還原",
  settingsDirty: "有未儲存的變更",
  settingsRulesTitle: "使用須知",
  settingsRule1: "留空的欄位會維持原值 — 這裡沒有清除功能。",
  settingsRule2: "任一欄位存檔後，Argus 都會重新啟動以套用設定。",
  settingsRestarting: "已儲存，bot 重新啟動中…",
  settingsReload: "重新整理",
  navImport: "匯入",
  importTitle: "匯入交易紀錄",
  importInstructions: "貼上或上傳 CSV 交易紀錄以補建歷史交易。",
  importTemplateHint: "欄位：date,ticker,action,shares,price,fee（fee 可省略，需保留標題列）",
  importTextareaPlaceholder: "date,ticker,action,shares,price,fee\n2026-01-05,AAPL,BUY,10,150,1.5",
  importChooseFile: "上傳 CSV",
  importPreview: "預覽",
  importConfirm: "確認匯入",
  importLine: "行號",
  importDate: "日期",
  importStatus: "狀態",
  importMessage: "訊息",
  importStatusOk: "正常",
  importStatusWarning: "警告",
  importStatusDuplicate: "重複",
  importStatusError: "錯誤",
  importStatusApplied: "已匯入",
  importNoRows: "尚無資料 — 請貼上 CSV 並預覽。",
  importAppliedPrefix: "已匯入 ",
  importAppliedSuffix: " 筆。",
  navPaper: "虛擬帳戶",
  paperReadOnlyBadge: "唯讀",
  paperReadOnlyNotice: "由自動策略即時執行，這裡只看不能下單。",
  paperEquity: "權益",
  paperInitialCash: "起始資金",
  paperTotalReturn: "總報酬",
  paperBenchmarkReturn: "大盤同期",
  paperAlpha: "超額報酬",
  paperSince: "起始於",
  closedPositions: "已平倉",
  entryDate: "進場日",
  exitDate: "出場日",
  exitPrice: "出場價",
  exitReason: "出場原因",
  exitReasonStop: "停損",
  exitReasonLlmSell: "LLM 賣出",
  distToStop: "距停損",
  noClosedPositions: "尚無已平倉紀錄。",
  navOptions: "期權",
  optionContract: "契約",
  optionRight: "權利",
  optionStrike: "履約價",
  optionExpiry: "到期日",
  optionDTE: "剩餘天數",
  optionContracts: "口數",
  optionAvgPremium: "平均權利金",
  optionMark: "現價",
  optionMarketValue: "市值",
  optionDelta: "Delta",
  optionAction: "動作",
  optionCalendar: "到期日曆",
  optionCollateral: "擔保品",
  optionLockedCash: "CSP 鎖定現金",
  optionLockedShares: "鎖定股數",
  optionHeldShares: "持有股數",
  optionNaked: "裸賣",
  noOptionPositions: "目前沒有期權部位。",
  noClosedOptions: "尚無已平倉期權交易。",
  noOptionCollateral: "沒有擔保品義務。",
  optAddBtn: "+ 新增",
  optCloseBtn: "平倉",
  optionCollateralNote: "CSP 鎖定現金；備兌賣權鎖定股數。裸賣代表鎖定股數超過實際持有股數。",
  optionNoPnlNote: "僅顯示已實現損益 — 選擇權沒有每日市值歷史可繪製曲線。",
  optAddTitle: "新增選擇權部位",
  optCloseTitle: "平倉",
  optFieldPremium: "權利金",
  optOutcomeHint: "這筆合約發生了什麼事？",
  optBuyToClose: "買進平倉",
  optSellToClose: "賣出平倉",
  optExpiredBtn: "到期歸零",
  optAssignedBtn: "被履約",
  optExercisedBtn: "履約",
  optCall: "買權",
  optPut: "賣權",
  navFlow: "資金流向",
  sectorFlowSubtitle: "資金流向產業 × 市值熱力圖，方格大小為市值，顏色為漲跌。",
  sectorFlowHeldHint: "外框標示 = 我的持倉，點方格可看個股圖。",
  sectorFlowNotReady: "類股資料尚未就緒，請等待下次排程掃描後再查看。",
  sectorFlowSizeBy: "方塊大小",
  sectorFlowSizeByCap: "市值",
  sectorFlowSizeByFlow: "資金流",
  sectorFlowHeld: "持有",
  sectorFlowNetFlow: "淨資金流",
  sectorFlowChange: "漲跌",
  sectorFlowTickerCount: "檔數",
  sectorFlowRanking: "類股資金流排行",
  sectorFlowTWCapNote: "台股方塊大小以成交金額計算（無免費市值資料源），非實際市值。",
  sectorFlowTotalNetFlow: "全市場淨資金流",
  sectorFlowBreadth: "上漲／下跌家數",
  sectorFlowStrongest: "資金流最強類股",
  sectorFlowWeakest: "資金流最弱類股",
  sectorFlowRefresh: "立即觸發掃描",
  sectorFlowRefreshing: "掃描已開始，美股約 500 檔可能需要數分鐘，稍後重新整理本頁查看結果。",
  sectorFlowRefreshError: "觸發掃描失敗。",
};

const dictionaries: Record<string, Dictionary> = { en, zh };

export type Lang = "zh" | "en";

export function getDictionary(lang: string): Dictionary {
  return dictionaries[lang] ?? dictionaries.zh;
}

// normalizeLang collapses whatever string /api/config or localStorage held
// into a valid Lang, defaulting to zh — same fallback getDictionary applies,
// but usable where the Lang itself (not just the dictionary) is needed,
// e.g. highlighting the active button in Sidebar's language toggle.
export function normalizeLang(lang: string | null): Lang {
  return lang === "en" ? "en" : "zh";
}

// sectorNamesZh translates Finnhub's US finnhubIndustry classification (an
// open-ended English string, e.g. "Technology", "Semiconductors") for the
// Money Flow page's sector groups. TW's own classification (FinMind's
// industry_category) already comes back in Chinese, so this only matters
// for the US market. Falls back to the original English string when
// unmapped — same "data-sourced proper noun that can't always translate"
// exception the app already makes for ticker company names.
const sectorNamesZh: Record<string, string> = {
  Technology: "科技",
  "Information Technology": "資訊科技",
  Semiconductors: "半導體",
  "Semiconductor Equipment & Materials": "半導體設備與材料",
  Software: "軟體",
  "Software - Infrastructure": "基礎軟體",
  "Software - Application": "應用軟體",
  Hardware: "硬體",
  "Computer Hardware": "電腦硬體",
  "Consumer Electronics": "消費性電子",
  "Electronic Equipment": "電子設備",
  Internet: "網路服務",
  "Internet Content & Information": "網路內容與資訊",
  "Health Care": "醫療保健",
  Healthcare: "醫療保健",
  "Health Care Providers": "醫療服務",
  "Medical Devices": "醫療器材",
  Biotechnology: "生技",
  Pharmaceuticals: "製藥",
  "Life Sciences Tools & Services": "生命科學工具與服務",
  "Financial Services": "金融服務",
  Financials: "金融",
  Banks: "銀行",
  "Diversified Financial Services": "多元化金融",
  "Consumer Finance": "消費金融",
  Insurance: "保險",
  "Capital Markets": "資本市場",
  "Consumer Cyclical": "非必需消費",
  "Consumer Defensive": "必需消費",
  Retail: "零售",
  "Internet Retail": "網路零售",
  "Specialty Retail": "專業零售",
  "Auto Manufacturers": "汽車製造",
  Automobiles: "汽車",
  "Auto Parts": "汽車零件",
  Apparel: "服飾",
  Beverages: "飲料",
  "Food Products": "食品",
  "Household Products": "家用產品",
  "Packaging & Containers": "包裝",
  Communication: "通訊",
  "Communication Services": "通訊服務",
  Telecommunication: "電信",
  Media: "媒體",
  Entertainment: "娛樂",
  "Hotels, Restaurants & Leisure": "飯店餐飲休閒",
  Airlines: "航空",
  Industrials: "工業",
  "Aerospace & Defense": "航太國防",
  Machinery: "機械",
  Transportation: "運輸",
  "Building Materials": "建材",
  Homebuilding: "營建",
  Chemicals: "化學",
  "Basic Materials": "原物料",
  "Metals & Mining": "金屬與礦業",
  Energy: "能源",
  "Oil & Gas": "石油天然氣",
  Utilities: "公用事業",
  "Real Estate": "不動產",
  REIT: "不動產投資信託",
};

// sectorLabel returns name translated for the given language, falling back
// to the original (English) string when no translation is known.
export function sectorLabel(name: string, lang: Lang): string {
  if (lang !== "zh") return name;
  return sectorNamesZh[name] ?? name;
}
