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
  llmRunPriceEvent: string;
  llmEventGap: string;
  llmEventChange: string;
  llmEventCumulative: string;
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
  recHoldKept: string;
  recCollapsed: string;
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
  wealthDisplayCurrency: string;
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
  // Research-notes card (chart page): one upserted note per ticker per day,
  // same add/edit-toggle shape as the thesisXxx keys above, plus a fixed
  // tag taxonomy and a pinned-notes sub-list.
  notesLabel: string;
  notesAddToggle: string;
  notesEditToggle: string;
  notesFieldPlaceholder: string;
  notesEmptyNote: string;
  notesSearchPlaceholder: string;
  notesClearSearch: string;
  notesPinnedLabel: string;
  notesPinLabel: string;
  notesUnpinLabel: string;
  notesDeleteLabel: string;
  notesDeleteConfirm: string;
  notesLoadMore: string;
  notesFilterAllLabel: string;
  notesTagTechnical: string;
  notesTagFlow: string;
  notesTagNews: string;
  notesTagOther: string;
  // Phase 9 wealth platform (docs/phase-9-asset-platform.md) — the /w net
  // worth home page (variant B: hero number + YTD/MoM) and its quick-add
  // form. Keyed off asset_group (liquid/growth/income/hard), the schema's
  // own four-bucket taxonomy, not the design mock's finer 9-category split.
  navWealth: string;
  acctTrading: string;
  acctWealth: string;
  wealthNetWorth: string;
  wealthYTD: string;
  wealthMoM: string;
  wealthAllocation: string;
  wealthModelConserv: string;
  wealthModelBalanced: string;
  wealthModelGrowth: string;
  wealthGroupLiquid: string;
  wealthGroupGrowth: string;
  wealthGroupIncome: string;
  wealthGroupHard: string;
  // wealthCategory* back /w/alloc's own nine-category taxonomy
  // (assets.AllocCategories, §8.5) — distinct from the four wealthGroup*
  // keys above, which the home page and balance sheet still use.
  wealthCategoryCash: string;
  wealthCategoryEquity: string;
  wealthCategoryFund: string;
  wealthCategoryBond: string;
  wealthCategoryInsurance: string;
  wealthCategoryEstate: string;
  wealthCategoryGold: string;
  wealthCategoryCrypto: string;
  wealthCategoryPension: string;
  wealthHomeVariantLabel: string;
  wealthHomeVariantBoard: string;
  wealthHomeVariantStory: string;
  wealthHomeAddNew: string;
  wealthHomeNetWorth: string;
  wealthHomeYtd: string;
  wealthHomeMom: string;
  wealthHomeDriftLabel: string;
  wealthHomeRebalanceLabel: string;
  wealthHomeDriftOnTarget: string;
  wealthHomeDriftNoRebal: string;
  wealthHomeDriftCount: string;
  wealthHomeDriftOff: string;
  wealthHomeDriftHead: string;
  wealthHomeHeadOk: string;
  wealthHomeThinShort: string;
  wealthHomeNeedAssets: string;
  wealthHomeAllocTitle: string;
  wealthHomeClass: string;
  wealthHomeCurrent: string;
  wealthHomeTarget: string;
  wealthHomeDrift: string;
  wealthHomeAction: string;
  wealthHomeActionTrim: string;
  wealthHomeActionAdd: string;
  wealthHomeActionOk: string;
  wealthHomeOffTitle: string;
  wealthHomeGroupTitle: string;
  wealthHomeLiabTitle: string;
  wealthHomeRate: string;
  wealthHomeLiquidLabel: string;
  wealthHomeMonths: string;
  wealthHomeStaleBanner: string;
  wealthHomeStaleGo: string;
  wealthHomeTipDrift: string;
  wealthHomeTipRebal: string;
  wealthHomeTipDebt: string;
  wealthHomeTipLiquid: string;
  wealthHomeTipYtd: string;
  wealthHomeYearsLeft: string;
  wealthHomePayFirst: string;
  wealthHomeMonthlyPay: string;
  wealthCurrentPct: string;
  wealthTargetPct: string;
  wealthDeviation: string;
  wealthMarketValue: string;
  wealthAssetsLabel: string;
  wealthAddAsset: string;
  wealthEmpty: string;
  wealthType: string;
  wealthName: string;
  wealthGroupLabel: string;
  wealthVenue: string;
  wealthVenueUnset: string;
  wealthCurrency: string;
  wealthValue: string;
  wealthArchive: string;
  wealthArchiveConfirm: string;
  wealthKindDeposit: string;
  wealthKindLoan: string;
  wealthKindInsurance: string;
  wealthKindFund: string;
  wealthKindBond: string;
  wealthKindEstate: string;
  wealthKindGold: string;
  wealthKindCrypto: string;
  wealthKindPension: string;
  wealthKindOther: string;
  wealthBank: string;
  wealthAccountNote: string;
  wealthLender: string;
  wealthRatePct: string;
  wealthOriginalPrincipal: string;
  wealthRemainingMonths: string;
  wealthAddTitle: string;
  wealthAddChange: string;
  wealthInitialValue: string;
  wealthEditValueTitle: string;
  // Phase 9 PR2 (partial): balance sheet page (/w/balance), health-metric
  // ratios, and the debt-payoff (snowball/avalanche) calculator. Mirrors
  // internal/web/wealth_balance.go.
  navWealthBalance: string;
  wealthTotalAssets: string;
  wealthTotalLiabilities: string;
  wealthDebtRatio: string;
  wealthLiquidityMonths: string;
  wealthSavingsRate: string;
  wealthExpenseRatio: string;
  wealthMonthlySalary: string;
  wealthAnnualSalary: string;
  wealthSetSalary: string;
  wealthSalarySave: string;
  wealthNoSalarySet: string;
  wealthPctOfAssets: string;
  wealthLiabilitiesLabel: string;
  wealthQuarterlyTrend: string;
  wealthEquityUS: string;
  wealthEquityTW: string;
  wealthMinPayment: string;
  wealthDebtPayoffTitle: string;
  wealthExtraPayment: string;
  wealthCalculate: string;
  wealthSnowball: string;
  wealthAvalanche: string;
  wealthPayoffOrder: string;
  wealthPayoffMonths: string;
  wealthPayoffMonthsSaved: string;
  wealthPayoffTotalInterest: string;
  wealthPayoffInterestDiff: string;
  wealthPayoffNoLoans: string;
  wealthOffTargetTitle: string;
  wealthSrcManual: string;
  wealthSrcImport: string;
  wealthSrcSync: string;
  // Phase 9 波次1 PR3' (§8.15.1): CSV paste import for initial data entry
  // (/w/import), replacing the dropped PDF-statement-parsing plan. Mirrors
  // internal/web/wealth_import.go.
  navWealthImport: string;
  wealthImportTitle: string;
  wealthImportInstructions: string;
  wealthImportTemplateHint: string;
  wealthImportTextareaPlaceholder: string;
  wealthImportColSide: string;
  wealthImportColType: string;
  wealthImportColName: string;
  wealthImportColGroup: string;
  wealthImportColValue: string;
  // Phase 9 §9.4 PR4: allocation & rebalance page (/w/alloc). Mirrors
  // internal/web/wealth_alloc.go.
  navWealthAlloc: string;
  wealthTargetModelLabel: string;
  wealthMixTitle: string;
  wealthOrdersTitle: string;
  wealthOrdersHint: string;
  wealthOrderBuy: string;
  wealthOrderSell: string;
  wealthNoOrders: string;
  wealthLockedTitle: string;
  wealthVenueLabel: string;
  // Generic disclosure label for a wealth page's collapsed write-affordance
  // section (a <details>/<summary>, not shown in the design template) — see
  // [[verify-ui-against-design-template]] for why write UI lives behind
  // this rather than always on screen.
  wealthManageLabel: string;
  wealthRiskTitle: string;
  wealthRiskBand: string;
  wealthFxExposure: string;
  wealthConcentrationTitle: string;
  wealthConcentrationHint: string;
  wealthNoConcentration: string;
  wealthRebalTotal: string;
  wealthAllocVsTarget: string;
  wealthAllocClass: string;
  wealthAllocCurrent: string;
  wealthAllocTarget: string;
  wealthAllocVenue: string;
  wealthRiskShare: string;
  wealthTipRisk: string;
  wealthTipFx: string;
  wealthRiskAggressive: string;
  wealthRiskBalanced: string;
  wealthRiskConservative: string;
  wealthGeoExposure: string;
  wealthNeedGeo: string;
  // Phase 9 波次2 PR5: cash flow page (/w/cash) — recurring income/expense
  // lines (hand-maintained monthly amounts, not a transaction ledger) and
  // the 90-day cash event table derived from them. Mirrors
  // internal/web/wealth_cash.go.
  navWealthCash: string;
  wealthCashMonthlyIn: string;
  wealthCashMonthlyOut: string;
  wealthCashMonthlyNet: string;
  wealthCashSaveRateLabel: string;
  wealthCashDcaShareLabel: string;
  wealthCashFixedShareLabel: string;
  wealthCashAnnualNetLabel: string;
  wealthCashNet90Label: string;
  wealthCashInBreakdownTitle: string;
  wealthCashOutBreakdownTitle: string;
  wealthCashItemsTitle: string;
  wealthCashEventsTitle: string;
  wealthCashNoItems: string;
  wealthCashNoEvents: string;
  wealthCashAddTitle: string;
  wealthCashDirectionIn: string;
  wealthCashDirectionOut: string;
  wealthCashNameLabel: string;
  wealthCashAmountLabel: string;
  wealthCashDayOfMonthLabel: string;
  wealthCashCategoryLabel: string;
  wealthCashAdd: string;
  wealthCashPause: string;
  wealthCashPaused: string;
  wealthCashDateLabel: string;
  wealthCashCatSalary: string;
  wealthCashCatRent: string;
  wealthCashCatDividend: string;
  wealthCashCatBondInterest: string;
  wealthCashCatFundDividend: string;
  wealthCashCatLiving: string;
  wealthCashCatMortgage: string;
  wealthCashCatSip: string;
  wealthCashCatLoan: string;
  wealthCashCatInsurance: string;
  wealthCashCatTax: string;
  // Phase 9 波次3 PR6: goal tracking page (/w/goals) — goals table +
  // goal_assets earmark. Retirement (kind "retirement") is the same table
  // (§8.8); PR7 owns computing its targetAmount, this page just renders it
  // like any other goal. Read-only, matching the design template's
  // isWGoals section — no add/earmark/delete keys here since the page has
  // no write UI. Mirrors internal/web/wealth_goals.go.
  navWealthGoals: string;
  wealthGoalsNoGoals: string;
  wealthGoalsSavedLabel: string;
  wealthGoalsTargetLabel: string;
  wealthGoalsStatusAhead: string;
  wealthGoalsStatusOnTrack: string;
  wealthGoalsStatusBehind: string;
  wealthGoalsTotalProgress: string;
  wealthGoalsMonthlyLabel: string;
  wealthGoalsBehindCountLabel: string;
  wealthGoalsEtaLabel: string;
  wealthGoalsExpectedLabel: string;
  wealthGoalsSeeRetireLink: string;
  wealthGoalsAddBtn: string;
  wealthGoalsAddTitle: string;
  wealthGoalsEditTitle: string;
  wealthGoalsEditBtn: string;
  wealthGoalsKindLabel: string;
  wealthGoalsNameLabel: string;
  wealthGoalsNamePlaceholder: string;
  wealthGoalsNoteLabel: string;
  wealthGoalsStartYearLabel: string;
  wealthGoalsEtaYearLabel: string;
  wealthGoalsSave: string;
  wealthGoalsDelete: string;
  wealthGoalsProgressLabel: string;
  wealthGoalsRemainLabel: string;
  wealthGoalsPaceLabel: string;
  wealthGoalsPaceNone: string;
  wealthGoalsPaceDone: string;
  wealthGoalsPaceAt: string;
  wealthGoalsRetireNote: string;
  wealthGoalsAgeSuffix: string;
  wealthGoalsRetireLocked: string;
  wealthGoalsKindEdu: string;
  wealthGoalsKindEduNote: string;
  wealthGoalsKindHome: string;
  wealthGoalsKindHomeNote: string;
  wealthGoalsKindEmg: string;
  wealthGoalsKindEmgNote: string;
  wealthGoalsKindTravel: string;
  wealthGoalsKindTravelNote: string;
  wealthGoalsKindCar: string;
  wealthGoalsKindCarNote: string;
  wealthGoalsKindStudy: string;
  wealthGoalsKindStudyNote: string;
  wealthGoalsKindWed: string;
  wealthGoalsKindWedNote: string;
  wealthGoalsKindCustom: string;

  // Retirement page (`/w/retire`, Phase 9 波次3 PR7, §8.7) — real-return
  // projection off the retirement-earmarked pool, plus three named §10.2②
  // scenarios. Copy lifted verbatim from the design template's t.wRet* keys.
  // Mirrors internal/web/wealth_retire.go.
  navWealthRetire: string;
  wealthRetireTargetAgeLabel: string;
  wealthRetireSpendLabel: string;
  wealthRetireNeedLabel: string;
  wealthRetireProjLabel: string;
  wealthRetireGapLabel: string;
  wealthRetireRateLabel: string;
  wealthRetireChartLabel: string;
  wealthRetireDepleteLabel: string;
  wealthRetireDepleteNever: string;
  wealthRetireFundedLabel: string;
  wealthRetireAssumption: string;
  wealthRetirePoolLabel: string;
  wealthRetireContribLabel: string;
  wealthRetireScenarioBaseline: string;
  wealthRetireScenarioCrash: string;
  wealthRetireScenarioLowReturn: string;
  wealthRetireSetupTitle: string;
  wealthRetireBirthYearLabel: string;
  wealthRetireContribInputLabel: string;
  wealthRetireSetupSave: string;
  wealthRetireSeeGoalsLink: string;
  wealthRetireGoalProgressLabel: string;
  // Settings drawer ("設定我的假設") — a live, unpersisted what-if preview
  // over every assumption the projection above depends on (design template
  // lines 1073-1139/4585-4619). wealthRetireChartLabelWithAge/
  // wealthRetireDepleteNeverWithAge carry a literal "{age}" token the
  // component substitutes with the live horizon age, mirroring the
  // template's own string concatenation for the same dynamic text.
  wealthRetireSettingsButton: string;
  wealthRetireCfgTitle: string;
  wealthRetireCfgNote: string;
  wealthRetireCfgEdited: string;
  wealthRetireCfgReset: string;
  wealthRetireCfgDone: string;
  wealthRetireCfgSecTime: string;
  wealthRetireCfgSecFlow: string;
  wealthRetireCfgSecAssume: string;
  wealthRetireCurrentAgeLabel: string;
  wealthRetireLifeLabel: string;
  wealthRetireOtherIncomeLabel: string;
  wealthRetireOtherIncomeHint: string;
  wealthRetireCfgContribLabel: string;
  wealthRetirePreRLabel: string;
  wealthRetirePostRLabel: string;
  wealthRetireSwrLabel: string;
  wealthRetireSwrHint: string;
  wealthRetireNetSpendLabel: string;
  wealthRetireChartLabelWithAge: string;
  wealthRetireDepleteNeverWithAge: string;

  // Insurance gap-analysis page (`/w/insure`, Phase 9 波次3 PR8, §8.6/
  // §8.16.1) — six-coverage-kind have/need table (a pure-function survivor-
  // needs estimate, never LLM-derived) plus the policy list. Mirrors
  // internal/web/wealth_insure.go.
  navWealthInsure: string;
  wealthInsureAdd: string;
  wealthInsureAddTitle: string;
  wealthInsureInsurerLabel: string;
  wealthInsurePolicyNameLabel: string;
  wealthInsureKindLabel: string;
  wealthInsureAmountLabel: string;
  wealthInsuredLabel: string;
  wealthInsureAnnualPremiumLabel: string;
  wealthInsurePremiumYearsLabel: string;
  wealthInsureBiggestGapLabel: string;
  wealthInsureCoverageLabel: string;
  wealthInsureTotalGapLabel: string;
  wealthInsurePremiumLabel: string;
  wealthInsurePremShareLabel: string;
  wealthInsureCountLabel: string;
  wealthInsureGapTitle: string;
  wealthInsureHaveLabel: string;
  wealthInsureNeedLabel: string;
  wealthInsureGapLabel: string;
  wealthInsureCoveredLabel: string;
  wealthInsureKindLife: string;
  wealthInsureKindAccident: string;
  wealthInsureKindCi: string;
  wealthInsureKindCancer: string;
  wealthInsureKindDisability: string;
  wealthInsureKindHospital: string;
  wealthInsurePerMonthSuffix: string;
  wealthInsurePerDaySuffix: string;
  wealthInsurePoliciesTitle: string;
  wealthInsureNoPolicies: string;
  wealthInsureSetupTitle: string;
  wealthInsureDependentsLabel: string;
  wealthInsureYoungestChildAgeLabel: string;
  wealthInsureSpouseIncomeLabel: string;
  wealthInsureSpouseIncomeYes: string;
  wealthInsureSpouseIncomeNo: string;
  wealthInsureSetupSave: string;
  wealthInsureNeedPendingNote: string;

  navWealthFunds: string;
  wealthFundsMvLabel: string;
  wealthFundsCostLabel: string;
  wealthFundsPnlLabel: string;
  wealthFundsMonthlyLabel: string;
  wealthFundsChartTitle: string;
  wealthFundsCostLeg: string;
  wealthFundsMvLeg: string;
  wealthFundsChartAxis24: string;
  wealthFundsChartAxis12: string;
  wealthFundsChartAxisNow: string;
  wealthFundsScheduleTitle: string;
  wealthFundsScheduleEmpty: string;
  wealthFundsTableTitle: string;
  wealthFundsColClass: string;
  wealthFundsColCode: string;
  wealthFundsColPlatform: string;
  wealthFundsColMonthly: string;
  wealthFundsColCost: string;
  wealthFundsColMv: string;
  wealthFundsColPnl: string;
  wealthFundsColReturn: string;
  wealthFundsColOneYear: string;
  wealthFundsColNext: string;
  wealthFundsStopped: string;
  wealthFundsClassName: string;
  wealthFundsNoFunds: string;
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
  llmRunPriceEvent: "Price event",
  llmEventGap: "Gap",
  llmEventChange: "Change",
  llmEventCumulative: "Cumulative",
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
  recHoldKept: "HOLD kept",
  recCollapsed: "repeats merged",
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
  recFollowedVsSkipped: "BUY/SELL calls vs HOLD",
  recFollowed: "BUY/SELL",
  recSkipped: "HOLD",
  recExcessByHorizon: "Excess Return by Holding Days",
  recSignalHitRate: "Signal Hit Rate",
  recRandomBaseline: "Random baseline",
  recBestHoldingWindow: "Evaluation Window",
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
  recNarrativePeakPrefix: "Over ",
  recNarrativePeakMid: " trading days, signals averaged ",
  recNarrativePeakSuffix: " excess over the market. ",
  recNarrativeSkipBetterPrefix: "Names it said to HOLD earned ",
  recNarrativeSkipBetterSuffix:
    " more than acting on its BUY/SELL calls — the calls aren't adding value.",
  recNarrativeFollowBetterPrefix: "Acting on its BUY/SELL calls earned ",
  recNarrativeFollowBetterSuffix:
    " more than the names it said to HOLD — the calls are adding value.",
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
  wealthDisplayCurrency: "DISPLAY CURRENCY",
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
  notesLabel: "RESEARCH NOTES",
  notesAddToggle: "+ Add Today's Note",
  notesEditToggle: "Edit Today's Note",
  notesFieldPlaceholder: "What did you see on the chart today?",
  notesEmptyNote: "No research notes yet for this ticker.",
  notesSearchPlaceholder: "Search notes...",
  notesClearSearch: "Clear",
  notesPinnedLabel: "PINNED",
  notesPinLabel: "Pin",
  notesUnpinLabel: "Unpin",
  notesDeleteLabel: "Delete",
  notesDeleteConfirm: "Delete this note? This can't be undone.",
  notesLoadMore: "Load more",
  notesFilterAllLabel: "All",
  notesTagTechnical: "Technical",
  notesTagFlow: "Flow",
  notesTagNews: "News",
  notesTagOther: "Other",
  navWealth: "Net Worth",
  acctTrading: "Trading",
  acctWealth: "Wealth",
  wealthNetWorth: "NET WORTH",
  wealthYTD: "YTD",
  wealthMoM: "MOM",
  wealthAllocation: "ALLOCATION",
  wealthModelConserv: "Conservative",
  wealthModelBalanced: "Balanced",
  wealthModelGrowth: "Growth",
  wealthGroupLiquid: "Liquid",
  wealthGroupGrowth: "Growth",
  wealthGroupIncome: "Income",
  wealthGroupHard: "Hard Assets",
  wealthCategoryCash: "Cash & FX deposits",
  wealthCategoryEquity: "Equities (trading account)",
  wealthCategoryFund: "ETFs & mutual funds",
  wealthCategoryBond: "Bonds & fixed income",
  wealthCategoryInsurance: "Insurance policies",
  wealthCategoryEstate: "Real estate",
  wealthCategoryGold: "Gold & physical",
  wealthCategoryCrypto: "Crypto",
  wealthCategoryPension: "Labor pension & insurance",
  wealthHomeVariantLabel: "HOME VARIANT",
  wealthHomeVariantBoard: "Allocation board",
  wealthHomeVariantStory: "Net-worth story",
  wealthHomeAddNew: "ADD",
  wealthHomeNetWorth: "NET WORTH",
  wealthHomeYtd: "YTD NET WORTH",
  wealthHomeMom: "MOM",
  wealthHomeDriftLabel: "ALLOCATION DRIFT",
  wealthHomeRebalanceLabel: "REBALANCE AMOUNT",
  wealthHomeDriftOnTarget: "On target",
  wealthHomeDriftNoRebal: "no rebalance needed",
  wealthHomeDriftCount: "%s",
  wealthHomeDriftOff: "classes off target",
  wealthHomeDriftHead: "%s classes off target",
  wealthHomeHeadOk: "On target",
  wealthHomeThinShort: "not enough data",
  wealthHomeNeedAssets: "only %s assets — need at least 3",
  wealthHomeAllocTitle: "ALLOCATION VS TARGET",
  wealthHomeClass: "Asset class",
  wealthHomeCurrent: "Current",
  wealthHomeTarget: "Target",
  wealthHomeDrift: "Drift",
  wealthHomeAction: "Action",
  wealthHomeActionTrim: "trim %s",
  wealthHomeActionAdd: "add %s",
  wealthHomeActionOk: "on target",
  wealthHomeOffTitle: "MOST OFF TARGET",
  wealthHomeGroupTitle: "BY GROUP",
  wealthHomeLiabTitle: "LIABILITIES",
  wealthHomeRate: "Rate",
  wealthHomeLiquidLabel: "EMERGENCY FUND",
  wealthHomeMonths: "%s months",
  wealthHomeStaleBanner: "%s records haven't been updated in 90+ days — net worth and ratios may be out of date.",
  wealthHomeStaleGo: "Update on balance sheet →",
  wealthHomeTipDrift: "How far current weights are from target. Within 2 points is normal.",
  wealthHomeTipRebal: "Roughly how much you would need to buy or sell to get back to target.",
  wealthHomeTipDebt: "Total liabilities ÷ total assets. Under 40% is healthy; lower means less reliance on debt.",
  wealthHomeTipLiquid: "How many months of spending your liquid assets cover (liquid ÷ monthly spending). 3–6 months is the usual guide.",
  wealthHomeTipYtd: "Change in net worth since January 1 this year.",
  wealthHomeYearsLeft: "%s yrs left",
  wealthHomePayFirst: "pay off first",
  wealthHomeMonthlyPay: "%s/mo",
  wealthCurrentPct: "Current %",
  wealthTargetPct: "Target %",
  wealthDeviation: "Deviation",
  wealthMarketValue: "Market Value",
  wealthAssetsLabel: "ASSETS",
  wealthAddAsset: "+ Add",
  wealthEmpty: "No assets yet — add your first one.",
  wealthType: "Type",
  wealthName: "Name",
  wealthGroupLabel: "Group",
  wealthVenue: "Venue",
  wealthVenueUnset: "Manual entry",
  wealthCurrency: "Currency",
  wealthValue: "Value",
  wealthArchive: "Archive",
  wealthArchiveConfirm: "Archive this asset? Its history stays on record, it just leaves the active list.",
  wealthKindDeposit: "Deposit",
  wealthKindLoan: "Loan",
  wealthKindInsurance: "Insurance",
  wealthKindFund: "Fund/ETF",
  wealthKindBond: "Bond",
  wealthKindEstate: "Real Estate",
  wealthKindGold: "Gold/Physical",
  wealthKindCrypto: "Crypto",
  wealthKindPension: "Pension",
  wealthKindOther: "Other",
  wealthBank: "Bank",
  wealthAccountNote: "Account note",
  wealthLender: "Lender",
  wealthRatePct: "Rate %",
  wealthOriginalPrincipal: "Original principal",
  wealthRemainingMonths: "Remaining months",
  wealthAddTitle: "Add Asset",
  wealthAddChange: "‹ Change",
  wealthInitialValue: "Starting value",
  wealthEditValueTitle: "Update value",
  navWealthBalance: "Balance Sheet",
  wealthTotalAssets: "Total Assets",
  wealthTotalLiabilities: "Total Liabilities",
  wealthDebtRatio: "Debt Ratio",
  wealthLiquidityMonths: "Liquidity (months)",
  wealthSavingsRate: "Savings Rate",
  wealthExpenseRatio: "Expense Ratio",
  wealthMonthlySalary: "Monthly Salary",
  wealthAnnualSalary: "Annual Salary",
  wealthSetSalary: "Set annual salary",
  wealthSalarySave: "Save",
  wealthNoSalarySet: "Set your annual salary to see savings rate, expense ratio, and liquidity months.",
  wealthPctOfAssets: "of assets",
  wealthLiabilitiesLabel: "LIABILITIES",
  wealthQuarterlyTrend: "QUARTERLY NET WORTH",
  wealthEquityUS: "US Trading Account",
  wealthEquityTW: "TW Trading Account",
  wealthMinPayment: "Min. Payment",
  wealthDebtPayoffTitle: "DEBT PAYOFF STRATEGY",
  wealthExtraPayment: "Extra monthly payment",
  wealthCalculate: "Calculate",
  wealthSnowball: "Snowball (smallest balance first)",
  wealthAvalanche: "Avalanche (highest rate first)",
  wealthPayoffOrder: "Payoff order",
  wealthPayoffMonths: "Months to payoff",
  wealthPayoffMonthsSaved: "Months saved",
  wealthPayoffTotalInterest: "Total interest",
  wealthPayoffInterestDiff: "Avalanche saves vs. snowball",
  wealthPayoffNoLoans: "No loans with both a rate and a remaining term yet.",
  wealthOffTargetTitle: "MOST OFF-TARGET",
  wealthSrcManual: "manual",
  wealthSrcImport: "import",
  wealthSrcSync: "sync",
  navWealthImport: "Import",
  wealthImportTitle: "Import Wealth Assets",
  wealthImportInstructions:
    "Paste or upload a CSV to bulk-create deposits, loans, and other assets/liabilities — the fastest way to enter your first batch of accounts.",
  wealthImportTemplateHint:
    "Columns: side,type,name,group,venue,currency,value,date,bank,accountNote,lender,ratePct,originalPrincipal,remainingMonths " +
    "(side: asset/liability; group: liquid/growth/income/hard; bank/accountNote apply to type=deposit; lender/ratePct/originalPrincipal/remainingMonths apply to type=loan/credit_card; header row required)",
  wealthImportTextareaPlaceholder:
    "side,type,name,group,venue,currency,value,date\nasset,deposit,Checking,liquid,Bank,TWD,100000,2026-09-01",
  wealthImportColSide: "Side",
  wealthImportColType: "Type",
  wealthImportColName: "Name",
  wealthImportColGroup: "Group",
  wealthImportColValue: "Value",
  navWealthAlloc: "Asset Allocation",
  wealthTargetModelLabel: "TARGET MODEL",
  wealthMixTitle: "CURRENT MIX",
  wealthOrdersTitle: "REBALANCE ORDERS",
  wealthOrdersHint: "Orders fire above 5pt of drift. Real estate, gold and statutory pension count as unadjustable.",
  wealthOrderBuy: "Add",
  wealthOrderSell: "Trim",
  wealthNoOrders: "Every adjustable class sits inside the tolerance band.",
  wealthLockedTitle: "OFF TARGET BUT LOCKED",
  wealthVenueLabel: "via",
  wealthManageLabel: "Manage",
  wealthRiskTitle: "RISK EXPOSURE",
  wealthRiskBand: "target band",
  wealthFxExposure: "CURRENCY EXPOSURE",
  wealthConcentrationTitle: "CONCENTRATION WARNINGS",
  wealthConcentrationHint: "A single position over a large share of your open equity book — informational only, no sell suggestion.",
  wealthNoConcentration: "No single position dominates your equity book.",
  wealthRebalTotal: "rebalance total",
  wealthAllocVsTarget: "ALLOCATION VS TARGET",
  wealthAllocClass: "Asset class",
  wealthAllocCurrent: "Current",
  wealthAllocTarget: "Target",
  wealthAllocVenue: "Venue",
  wealthRiskShare: "RISK ASSET SHARE",
  wealthTipRisk: "Share of stocks, funds and crypto in total assets. Higher means more volatility; 25–40% is balanced.",
  wealthTipFx: "Share of assets by currency. A large single foreign currency means more FX risk.",
  wealthRiskAggressive: "Aggressive",
  wealthRiskBalanced: "Balanced",
  wealthRiskConservative: "Conservative",
  wealthGeoExposure: "GEOGRAPHIC EXPOSURE",
  wealthNeedGeo: "Region exposure needs holdings detail; manual entries can't be broken down.",
  navWealthCash: "Cash Flow",
  wealthCashMonthlyIn: "Monthly income",
  wealthCashMonthlyOut: "Monthly expense",
  wealthCashMonthlyNet: "Monthly net",
  wealthCashSaveRateLabel: "Savings rate",
  wealthCashDcaShareLabel: "DCA / income",
  wealthCashFixedShareLabel: "Fixed / income",
  wealthCashAnnualNetLabel: "Full-year surplus",
  wealthCashNet90Label: "Net over period",
  wealthCashInBreakdownTitle: "Income sources",
  wealthCashOutBreakdownTitle: "Expense breakdown",
  wealthCashItemsTitle: "Recurring cash flows",
  wealthCashEventsTitle: "Next 90 days",
  wealthCashNoItems: "No recurring cash flows yet.",
  wealthCashNoEvents: "No cash events in the next 90 days.",
  wealthCashAddTitle: "Add recurring flow",
  wealthCashDirectionIn: "Income",
  wealthCashDirectionOut: "Expense",
  wealthCashNameLabel: "Name",
  wealthCashAmountLabel: "Monthly amount",
  wealthCashDayOfMonthLabel: "Day of month",
  wealthCashCategoryLabel: "Category",
  wealthCashAdd: "Add",
  wealthCashPause: "Pause",
  wealthCashPaused: "Paused",
  wealthCashDateLabel: "Date",
  wealthCashCatSalary: "Salary",
  wealthCashCatRent: "Rental income",
  wealthCashCatDividend: "Stock dividends",
  wealthCashCatBondInterest: "Bond interest",
  wealthCashCatFundDividend: "Fund dividends",
  wealthCashCatLiving: "Living expenses",
  wealthCashCatMortgage: "Mortgage",
  wealthCashCatSip: "Recurring investment",
  wealthCashCatLoan: "Loan / car payment",
  wealthCashCatInsurance: "Insurance premium",
  wealthCashCatTax: "Tax reserve",
  navWealthGoals: "Goals",
  wealthGoalsNoGoals: "No goals yet.",
  wealthGoalsSavedLabel: "Saved",
  wealthGoalsTargetLabel: "Target",
  wealthGoalsStatusAhead: "Ahead",
  wealthGoalsStatusOnTrack: "On track",
  wealthGoalsStatusBehind: "Behind",
  wealthGoalsTotalProgress: "ALL GOALS",
  wealthGoalsMonthlyLabel: "Monthly",
  wealthGoalsBehindCountLabel: "BEHIND",
  wealthGoalsEtaLabel: "ETA",
  wealthGoalsExpectedLabel: "expected",
  wealthGoalsSeeRetireLink: "Retirement plan ›",
  wealthGoalsAddBtn: "Add goal",
  wealthGoalsAddTitle: "NEW GOAL",
  wealthGoalsEditTitle: "EDIT GOAL",
  wealthGoalsEditBtn: "Edit",
  wealthGoalsKindLabel: "GOAL TYPE",
  wealthGoalsNameLabel: "Goal name",
  wealthGoalsNamePlaceholder: "e.g. Parents' care fund",
  wealthGoalsNoteLabel: "Description",
  wealthGoalsStartYearLabel: "Start year",
  wealthGoalsEtaYearLabel: "Target year",
  wealthGoalsSave: "Save",
  wealthGoalsDelete: "Delete goal",
  wealthGoalsProgressLabel: "PROGRESS",
  wealthGoalsRemainLabel: "REMAINING",
  wealthGoalsPaceLabel: "AT CURRENT PACE",
  wealthGoalsPaceNone: "Set a monthly amount",
  wealthGoalsPaceDone: "Funded",
  wealthGoalsPaceAt: "%s at this pace",
  wealthGoalsRetireNote: "Earmarked today vs required · projected %1 at %2",
  wealthGoalsAgeSuffix: "",
  wealthGoalsRetireLocked: "Driven by the retirement plan's assumptions",
  wealthGoalsKindEdu: "Children's education",
  wealthGoalsKindEduNote: "two children · overseas undergrad",
  wealthGoalsKindHome: "Home down payment",
  wealthGoalsKindHomeNote: "upgrade or first home",
  wealthGoalsKindEmg: "Emergency fund",
  wealthGoalsKindEmgNote: "6–12 months of outflow",
  wealthGoalsKindTravel: "Travel fund",
  wealthGoalsKindTravelNote: "long trip or sabbatical",
  wealthGoalsKindCar: "Car",
  wealthGoalsKindCarNote: "replacement or first car",
  wealthGoalsKindStudy: "Further study",
  wealthGoalsKindStudyNote: "degree or certification",
  wealthGoalsKindWed: "Wedding",
  wealthGoalsKindWedNote: "ceremony and honeymoon",
  wealthGoalsKindCustom: "Custom",

  navWealthRetire: "Retirement",
  wealthRetireTargetAgeLabel: "RETIREMENT AGE",
  wealthRetireSpendLabel: "MONTHLY SPEND",
  wealthRetireNeedLabel: "ASSETS REQUIRED",
  wealthRetireProjLabel: "PROJECTED AT RETIREMENT",
  wealthRetireGapLabel: "GAP",
  wealthRetireRateLabel: "FUNDED",
  wealthRetireChartLabel: "PROJECTION TO AGE 92",
  wealthRetireDepleteLabel: "DEPLETES AT",
  wealthRetireDepleteNever: "Never (92+)",
  wealthRetireFundedLabel: "Funded",
  wealthRetireAssumption:
    "Real returns after inflation: 3.0% before retirement, 1.0% after. Spending in today's dollars, assets required per the 4% rule. Counts retirement-earmarked assets only (other invested balances back the other goals); excludes primary residence and mortgage.",
  wealthRetirePoolLabel: "EARMARKED TODAY",
  wealthRetireContribLabel: "MONTHLY CONTRIBUTION",
  wealthRetireScenarioBaseline: "Baseline",
  wealthRetireScenarioCrash: "Crash 5y before retirement",
  wealthRetireScenarioLowReturn: "Long-term return -1.5pt",
  wealthRetireSetupTitle: "Set up retirement inputs",
  wealthRetireBirthYearLabel: "Birth year",
  wealthRetireContribInputLabel: "Monthly contribution",
  wealthRetireSetupSave: "Save",
  wealthRetireSeeGoalsLink: "See in goal tracker ›",
  wealthRetireGoalProgressLabel: "GOAL PROGRESS",
  wealthRetireSettingsButton: "Edit assumptions",
  wealthRetireCfgTitle: "RETIREMENT SETTINGS",
  wealthRetireCfgNote: "These inputs only drive this projection — your recorded assets stay untouched.",
  wealthRetireCfgEdited: "CUSTOM",
  wealthRetireCfgReset: "Reset",
  wealthRetireCfgDone: "Done",
  wealthRetireCfgSecTime: "TIMELINE",
  wealthRetireCfgSecFlow: "CASH FLOW & PRINCIPAL",
  wealthRetireCfgSecAssume: "RETURN & WITHDRAWAL",
  wealthRetireCurrentAgeLabel: "Current age",
  wealthRetireLifeLabel: "Life expectancy",
  wealthRetireOtherIncomeLabel: "Other income after retirement",
  wealthRetireOtherIncomeHint: "Labor pension, rent, etc. — subtracted from monthly spend",
  wealthRetireCfgContribLabel: "Monthly contribution",
  wealthRetirePreRLabel: "Pre-retirement real return",
  wealthRetirePostRLabel: "Post-retirement real return",
  wealthRetireSwrLabel: "WITHDRAWAL RATE",
  wealthRetireSwrHint: "4% ≈ 25× annual spend",
  wealthRetireNetSpendLabel: "NET MONTHLY WITHDRAWAL",
  wealthRetireChartLabelWithAge: "PROJECTION TO AGE {age}",
  wealthRetireDepleteNeverWithAge: "Never ({age}+)",

  navWealthInsure: "Insurance",
  wealthInsureAdd: "Add policy",
  wealthInsureAddTitle: "Add a policy",
  wealthInsureInsurerLabel: "Insurer",
  wealthInsurePolicyNameLabel: "Policy name",
  wealthInsureKindLabel: "Coverage type",
  wealthInsureAmountLabel: "Coverage amount",
  wealthInsuredLabel: "Insured",
  wealthInsureAnnualPremiumLabel: "Annual premium",
  wealthInsurePremiumYearsLabel: "Premium years",
  wealthInsureBiggestGapLabel: "BIGGEST GAP",
  wealthInsureCoverageLabel: "coverage",
  wealthInsureTotalGapLabel: "TOTAL LUMP-SUM GAP",
  wealthInsurePremiumLabel: "ANNUAL PREMIUM",
  wealthInsurePremShareLabel: "share of annual income",
  wealthInsureCountLabel: "POLICIES",
  wealthInsureGapTitle: "Coverage gap & ratio",
  wealthInsureHaveLabel: "Have",
  wealthInsureNeedLabel: "Need",
  wealthInsureGapLabel: "Gap",
  wealthInsureCoveredLabel: "Covered",
  wealthInsureKindLife: "Life",
  wealthInsureKindAccident: "Accidental death",
  wealthInsureKindCi: "Critical illness",
  wealthInsureKindCancer: "Cancer lump sum",
  wealthInsureKindDisability: "Disability support",
  wealthInsureKindHospital: "Hospital daily",
  wealthInsurePerMonthSuffix: " /mo",
  wealthInsurePerDaySuffix: " /day",
  wealthInsurePoliciesTitle: "Policy list",
  wealthInsureNoPolicies: "No policies recorded yet.",
  wealthInsureSetupTitle: "Set up household details",
  wealthInsureDependentsLabel: "Dependents",
  wealthInsureYoungestChildAgeLabel: "Youngest child's age",
  wealthInsureSpouseIncomeLabel: "Spouse has independent income",
  wealthInsureSpouseIncomeYes: "Yes",
  wealthInsureSpouseIncomeNo: "No",
  wealthInsureSetupSave: "Save",
  wealthInsureNeedPendingNote: "Set up household details below to see life/accidental-death/disability need estimates.",

  navWealthFunds: "Funds & DCA",
  wealthFundsMvLabel: "MARKET VALUE",
  wealthFundsCostLabel: "TOTAL INVESTED",
  wealthFundsPnlLabel: "UNREALIZED P&L",
  wealthFundsMonthlyLabel: "MONTHLY CONTRIBUTION",
  wealthFundsChartTitle: "INVESTED VS MARKET VALUE (24 MO)",
  wealthFundsCostLeg: "invested",
  wealthFundsMvLeg: "market value",
  wealthFundsChartAxis24: "24 mo ago",
  wealthFundsChartAxis12: "12 mo ago",
  wealthFundsChartAxisNow: "now",
  wealthFundsScheduleTitle: "THIS MONTH'S SCHEDULE",
  wealthFundsScheduleEmpty: "No upcoming contributions",
  wealthFundsTableTitle: "HOLDINGS & PERFORMANCE",
  wealthFundsColClass: "Class",
  wealthFundsColCode: "Code",
  wealthFundsColPlatform: "Platform",
  wealthFundsColMonthly: "Monthly",
  wealthFundsColCost: "Invested",
  wealthFundsColMv: "Market value",
  wealthFundsColPnl: "P&L",
  wealthFundsColReturn: "Return",
  wealthFundsColOneYear: "1-yr",
  wealthFundsColNext: "Next",
  wealthFundsStopped: "lump sum",
  wealthFundsClassName: "Fund",
  wealthFundsNoFunds: "No funds yet — import via CSV on the Import page.",
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
  llmRunPriceEvent: "價格事件",
  llmEventGap: "跳空",
  llmEventChange: "當日漲跌",
  llmEventCumulative: "累計漲跌",
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
  recHoldKept: "保留 HOLD",
  recCollapsed: "合併重複",
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
  recFollowedVsSkipped: "BUY/SELL 訊號 VS HOLD",
  recFollowed: "BUY/SELL",
  recSkipped: "HOLD",
  recExcessByHorizon: "超額報酬隨持有天數變化",
  recSignalHitRate: "訊號命中率",
  recRandomBaseline: "隨機基準",
  recBestHoldingWindow: "評估視窗",
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
  recNarrativePeakPrefix: "訊號 ",
  recNarrativePeakMid: " 個交易日的平均超額報酬為 ",
  recNarrativePeakSuffix: "。",
  recNarrativeSkipBetterPrefix: "它說 HOLD 的股票比照 BUY/SELL 操作多賺 ",
  recNarrativeSkipBetterSuffix: " ——BUY/SELL 訊號沒有加分。",
  recNarrativeFollowBetterPrefix: "照 BUY/SELL 操作比它說 HOLD 的股票多賺 ",
  recNarrativeFollowBetterSuffix: " ——BUY/SELL 訊號有加分。",
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
  wealthDisplayCurrency: "顯示幣別",
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
  notesLabel: "研究筆記",
  notesAddToggle: "+ 新增今日筆記",
  notesEditToggle: "編輯今日筆記",
  notesFieldPlaceholder: "今天在圖上看到了什麼？",
  notesEmptyNote: "這支標的還沒有研究筆記。",
  notesSearchPlaceholder: "搜尋筆記...",
  notesClearSearch: "清除",
  notesPinnedLabel: "已釘選",
  notesPinLabel: "釘選",
  notesUnpinLabel: "取消釘選",
  notesDeleteLabel: "刪除",
  notesDeleteConfirm: "確定要刪除這則筆記嗎？此動作無法復原。",
  notesLoadMore: "載入更多",
  notesFilterAllLabel: "全部",
  notesTagTechnical: "技術面",
  notesTagFlow: "籌碼面",
  notesTagNews: "消息面",
  notesTagOther: "其他",
  navWealth: "淨值總覽",
  acctTrading: "交易帳戶",
  acctWealth: "資產帳戶",
  wealthNetWorth: "淨資產",
  wealthYTD: "今年以來",
  wealthMoM: "月變動",
  wealthAllocation: "資產配置",
  wealthModelConserv: "穩健",
  wealthModelBalanced: "平衡",
  wealthModelGrowth: "成長",
  wealthGroupLiquid: "流動",
  wealthGroupGrowth: "成長",
  wealthGroupIncome: "收益",
  wealthGroupHard: "實體",
  wealthCategoryCash: "活存 · 定存 · 外幣",
  wealthCategoryEquity: "股票（交易帳戶）",
  wealthCategoryFund: "ETF 與共同基金",
  wealthCategoryBond: "債券與固定收益",
  wealthCategoryInsurance: "保單（儲蓄＋投資型）",
  wealthCategoryEstate: "不動產",
  wealthCategoryGold: "黃金與實體資產",
  wealthCategoryCrypto: "加密貨幣",
  wealthCategoryPension: "勞保 · 勞退",
  wealthHomeVariantLabel: "首頁版本",
  wealthHomeVariantBoard: "配置盤面",
  wealthHomeVariantStory: "淨值敘事",
  wealthHomeAddNew: "新增",
  wealthHomeNetWorth: "資產淨值",
  wealthHomeYtd: "YTD 淨值成長",
  wealthHomeMom: "月變化",
  wealthHomeDriftLabel: "配置偏離",
  wealthHomeRebalanceLabel: "建議再平衡金額",
  wealthHomeDriftOnTarget: "符合",
  wealthHomeDriftNoRebal: "無須再平衡",
  wealthHomeDriftCount: "%s 項",
  wealthHomeDriftOff: "偏離目標",
  wealthHomeDriftHead: "%s 項偏離目標",
  wealthHomeHeadOk: "配置符合目標",
  wealthHomeThinShort: "資料不足",
  wealthHomeNeedAssets: "資產僅 %s 筆，至少需 3 筆",
  wealthHomeAllocTitle: "資產配置 vs 目標",
  wealthHomeClass: "資產類別",
  wealthHomeCurrent: "現在",
  wealthHomeTarget: "目標",
  wealthHomeDrift: "偏離",
  wealthHomeAction: "建議",
  wealthHomeActionTrim: "減碼 %s",
  wealthHomeActionAdd: "加碼 %s",
  wealthHomeActionOk: "符合",
  wealthHomeOffTitle: "最需要處理的偏離",
  wealthHomeGroupTitle: "大類配置",
  wealthHomeLiabTitle: "負債",
  wealthHomeRate: "利率",
  wealthHomeLiquidLabel: "緊急預備金",
  wealthHomeMonths: "%s 個月",
  wealthHomeStaleBanner: "有 %s 筆資料超過 90 天未更新，淨值與比率可能已過時。",
  wealthHomeStaleGo: "到資產負債表更新 →",
  wealthHomeTipDrift: "目前比例和目標比例差多少。差 2 個百分點以內算正常。",
  wealthHomeTipRebal: "要回到目標比例，大約需要買賣的金額。",
  wealthHomeTipDebt: "總負債 ÷ 總資產。40% 以下算健康，越低代表越不依賴借貸。",
  wealthHomeTipLiquid: "流動資產夠支付幾個月的生活支出（流動資產 ÷ 每月支出）。一般建議 3–6 個月。",
  wealthHomeTipYtd: "今年 1 月 1 日到現在，淨值增加或減少的比例。",
  wealthHomeYearsLeft: "剩 %s 年",
  wealthHomePayFirst: "建議優先清償",
  wealthHomeMonthlyPay: "月付 %s",
  wealthCurrentPct: "現值佔比",
  wealthTargetPct: "目標佔比",
  wealthDeviation: "偏離",
  wealthMarketValue: "市值",
  wealthAssetsLabel: "資產列表",
  wealthAddAsset: "＋ 新增",
  wealthEmpty: "尚未新增任何資產，點擊上方按鈕新增第一筆。",
  wealthType: "型別",
  wealthName: "名稱",
  wealthGroupLabel: "分組",
  wealthVenue: "機構",
  wealthVenueUnset: "手動登錄",
  wealthCurrency: "幣別",
  wealthValue: "現值",
  wealthArchive: "封存",
  wealthArchiveConfirm: "封存這筆資產？歷史紀錄會保留，只是從清單中隱藏。",
  wealthKindDeposit: "存款",
  wealthKindLoan: "貸款",
  wealthKindInsurance: "保單",
  wealthKindFund: "基金/ETF",
  wealthKindBond: "債券",
  wealthKindEstate: "不動產",
  wealthKindGold: "黃金/實體",
  wealthKindCrypto: "加密貨幣",
  wealthKindPension: "勞保勞退",
  wealthKindOther: "其他",
  wealthBank: "銀行",
  wealthAccountNote: "帳戶備註",
  wealthLender: "貸款機構",
  wealthRatePct: "利率 %",
  wealthOriginalPrincipal: "原始本金",
  wealthRemainingMonths: "剩餘月數",
  wealthAddTitle: "新增資產",
  wealthAddChange: "‹ 更改",
  wealthInitialValue: "起始金額",
  wealthEditValueTitle: "更新現值",
  navWealthBalance: "資產負債表",
  wealthTotalAssets: "總資產",
  wealthTotalLiabilities: "總負債",
  wealthDebtRatio: "負債比",
  wealthLiquidityMonths: "流動性月數",
  wealthSavingsRate: "儲蓄率",
  wealthExpenseRatio: "收支比",
  wealthMonthlySalary: "月薪",
  wealthAnnualSalary: "年薪",
  wealthSetSalary: "設定年薪",
  wealthSalarySave: "儲存",
  wealthNoSalarySet: "設定年薪後即可看到儲蓄率、收支比與流動性月數。",
  wealthPctOfAssets: "佔總資產",
  wealthLiabilitiesLabel: "負債列表",
  wealthQuarterlyTrend: "季度淨值趨勢",
  wealthEquityUS: "美股交易帳戶",
  wealthEquityTW: "台股交易帳戶",
  wealthMinPayment: "最低月付",
  wealthDebtPayoffTitle: "債務償還策略",
  wealthExtraPayment: "每月可額外還款",
  wealthCalculate: "計算",
  wealthSnowball: "雪球法（先清最小筆）",
  wealthAvalanche: "雪崩法（先清最高利率）",
  wealthPayoffOrder: "清償順序",
  wealthPayoffMonths: "所需月數",
  wealthPayoffMonthsSaved: "縮短月數",
  wealthPayoffTotalInterest: "總利息",
  wealthPayoffInterestDiff: "雪崩比雪球省下利息",
  wealthPayoffNoLoans: "尚無同時填有利率與剩餘期數的貸款可計算。",
  wealthOffTargetTitle: "偏離最大項",
  wealthSrcManual: "手動",
  wealthSrcImport: "匯入",
  wealthSrcSync: "同步",
  navWealthImport: "匯入",
  wealthImportTitle: "匯入資產資料",
  wealthImportInstructions: "貼上或上傳 CSV 批次建立存款/貸款/其他資產負債——第一批帳戶最快的建檔方式。",
  wealthImportTemplateHint:
    "欄位：side,type,name,group,venue,currency,value,date,bank,accountNote,lender,ratePct,originalPrincipal,remainingMonths" +
    "（side：asset/liability；group：liquid/growth/income/hard；bank/accountNote 僅 type=deposit 時填寫；" +
    "lender/ratePct/originalPrincipal/remainingMonths 僅 type=loan/credit_card 時填寫；需保留標題列）",
  wealthImportTextareaPlaceholder:
    "side,type,name,group,venue,currency,value,date\nasset,deposit,活存,liquid,銀行,TWD,100000,2026-09-01",
  wealthImportColSide: "類別",
  wealthImportColType: "類型",
  wealthImportColName: "名稱",
  wealthImportColGroup: "分組",
  wealthImportColValue: "金額",
  navWealthAlloc: "資產配置",
  wealthTargetModelLabel: "目標模型",
  wealthMixTitle: "現有配置",
  wealthOrdersTitle: "再平衡指令",
  wealthOrdersHint: "偏離超過 5pt 才產生指令。不動產、黃金與勞退視為不可調整。",
  wealthOrderBuy: "加碼",
  wealthOrderSell: "減碼",
  wealthNoOrders: "目前所有可調整項目都在容忍區間內。",
  wealthLockedTitle: "偏離但不可調整",
  wealthVenueLabel: "經由",
  wealthManageLabel: "管理",
  wealthRiskTitle: "風險曝險",
  wealthRiskBand: "目標區間",
  wealthFxExposure: "幣別曝險",
  wealthConcentrationTitle: "個股集中度警示",
  wealthConcentrationHint: "單一持股占你交易帳戶部位過高比重——僅供參考，不建議賣出哪一檔。",
  wealthNoConcentration: "沒有單一持股占比過高。",
  wealthRebalTotal: "再平衡總額",
  wealthAllocVsTarget: "資產配置 vs 目標",
  wealthAllocClass: "資產類別",
  wealthAllocCurrent: "現在",
  wealthAllocTarget: "目標",
  wealthAllocVenue: "執行處",
  wealthRiskShare: "風險性資產佔比",
  wealthTipRisk: "股票、基金、加密貨幣佔總資產的比例。越高波動越大，一般 25–40% 算穩健。",
  wealthTipFx: "資產以各種貨幣計價的比例。單一外幣過高時，匯率變動影響較大。",
  wealthRiskAggressive: "積極",
  wealthRiskBalanced: "穩健成長",
  wealthRiskConservative: "保守",
  wealthGeoExposure: "地區曝險",
  wealthNeedGeo: "地區曝險需要持股明細，手動建檔的資產無法推算。",
  navWealthCash: "現金流",
  wealthCashMonthlyIn: "月收入",
  wealthCashMonthlyOut: "月支出",
  wealthCashMonthlyNet: "月結餘",
  wealthCashSaveRateLabel: "儲蓄率",
  wealthCashDcaShareLabel: "定期定額佔收入",
  wealthCashFixedShareLabel: "固定支出佔收入",
  wealthCashAnnualNetLabel: "全年累計結餘",
  wealthCashNet90Label: "期間淨額",
  wealthCashInBreakdownTitle: "收入來源",
  wealthCashOutBreakdownTitle: "支出分佈",
  wealthCashItemsTitle: "經常性收支",
  wealthCashEventsTitle: "未來 90 天",
  wealthCashNoItems: "尚無經常性收支項目。",
  wealthCashNoEvents: "未來 90 天沒有現金事件。",
  wealthCashAddTitle: "新增經常性收支",
  wealthCashDirectionIn: "收入",
  wealthCashDirectionOut: "支出",
  wealthCashNameLabel: "名稱",
  wealthCashAmountLabel: "月額",
  wealthCashDayOfMonthLabel: "每月幾號",
  wealthCashCategoryLabel: "類別",
  wealthCashAdd: "新增",
  wealthCashPause: "暫停",
  wealthCashPaused: "已暫停",
  wealthCashDateLabel: "日期",
  wealthCashCatSalary: "薪資",
  wealthCashCatRent: "租金收入",
  wealthCashCatDividend: "股息",
  wealthCashCatBondInterest: "債息",
  wealthCashCatFundDividend: "基金配息",
  wealthCashCatLiving: "生活支出",
  wealthCashCatMortgage: "房貸",
  wealthCashCatSip: "定期定額",
  wealthCashCatLoan: "信貸車貸",
  wealthCashCatInsurance: "保費",
  wealthCashCatTax: "稅務預留",
  navWealthGoals: "目標追蹤",
  wealthGoalsNoGoals: "尚無目標。",
  wealthGoalsSavedLabel: "已累積",
  wealthGoalsTargetLabel: "目標",
  wealthGoalsStatusAhead: "超前",
  wealthGoalsStatusOnTrack: "進度正常",
  wealthGoalsStatusBehind: "落後",
  wealthGoalsTotalProgress: "全部目標合計",
  wealthGoalsMonthlyLabel: "每月投入",
  wealthGoalsBehindCountLabel: "落後項目",
  wealthGoalsEtaLabel: "預計達成",
  wealthGoalsExpectedLabel: "應有進度",
  wealthGoalsSeeRetireLink: "退休規劃 ›",
  wealthGoalsAddBtn: "新增目標",
  wealthGoalsAddTitle: "新增目標",
  wealthGoalsEditTitle: "編輯目標",
  wealthGoalsEditBtn: "編輯",
  wealthGoalsKindLabel: "目標類型",
  wealthGoalsNameLabel: "目標名稱",
  wealthGoalsNamePlaceholder: "例如：父母安養金",
  wealthGoalsNoteLabel: "說明",
  wealthGoalsStartYearLabel: "開始年份",
  wealthGoalsEtaYearLabel: "目標年份",
  wealthGoalsSave: "儲存",
  wealthGoalsDelete: "刪除目標",
  wealthGoalsProgressLabel: "目前進度",
  wealthGoalsRemainLabel: "還差",
  wealthGoalsPaceLabel: "依目前速度",
  wealthGoalsPaceNone: "尚未設定每月投入",
  wealthGoalsPaceDone: "已達標",
  wealthGoalsPaceAt: "依此速度約 %s 年達成",
  wealthGoalsRetireNote: "退休專用資產對比所需資產 · %2 歲屆退推估 %1",
  wealthGoalsAgeSuffix: " 歲",
  wealthGoalsRetireLocked: "退休金由「退休規劃」的假設自動計算",
  wealthGoalsKindEdu: "子女教育金",
  wealthGoalsKindEduNote: "兩名子女 · 海外大學",
  wealthGoalsKindHome: "購屋頭期款",
  wealthGoalsKindHomeNote: "換屋或首購",
  wealthGoalsKindEmg: "緊急預備金",
  wealthGoalsKindEmgNote: "6–12 個月支出",
  wealthGoalsKindTravel: "旅行基金",
  wealthGoalsKindTravelNote: "長假或長途旅行",
  wealthGoalsKindCar: "購車基金",
  wealthGoalsKindCarNote: "換車或首購",
  wealthGoalsKindStudy: "進修基金",
  wealthGoalsKindStudyNote: "學位或証照",
  wealthGoalsKindWed: "結婚基金",
  wealthGoalsKindWedNote: "婚禮與蜜月",
  wealthGoalsKindCustom: "自訂",

  navWealthRetire: "退休規劃",
  wealthRetireTargetAgeLabel: "退休年齡",
  wealthRetireSpendLabel: "退休後月支出",
  wealthRetireNeedLabel: "所需資產",
  wealthRetireProjLabel: "預估退休資產",
  wealthRetireGapLabel: "缺口",
  wealthRetireRateLabel: "達成率",
  wealthRetireChartLabel: "資產推估（至 92 歲）",
  wealthRetireDepleteLabel: "資產耗盡",
  wealthRetireDepleteNever: "92 歲後仍有結餘",
  wealthRetireFundedLabel: "已達標",
  wealthRetireAssumption:
    "假設：以實質報酬計算（退休前 3.0%、退休後 1.0%，已扣通膨），支出以今日幣值表示，四%法則推估所需資產。僅計入退休專用資產（其餘投資部位分屬其他目標），不含自住不動產與房貸。",
  wealthRetirePoolLabel: "退休專用資產",
  wealthRetireContribLabel: "每月提撥",
  wealthRetireScenarioBaseline: "基準情境",
  wealthRetireScenarioCrash: "退休前五年遇大跌",
  wealthRetireScenarioLowReturn: "長期報酬低 1.5pt",
  wealthRetireSetupTitle: "設定退休規劃參數",
  wealthRetireBirthYearLabel: "出生年",
  wealthRetireContribInputLabel: "每月投入退休資產",
  wealthRetireSetupSave: "儲存",
  wealthRetireSeeGoalsLink: "在目標追蹤中檢視 ›",
  wealthRetireGoalProgressLabel: "目標進度",
  wealthRetireSettingsButton: "設定我的假設",
  wealthRetireCfgTitle: "退休規劃設定",
  wealthRetireCfgNote: "這些數字只影響退休頁的試算，不會改動你的實際資產紀錄。",
  wealthRetireCfgEdited: "已自訂",
  wealthRetireCfgReset: "恢復預設",
  wealthRetireCfgDone: "完成",
  wealthRetireCfgSecTime: "時程",
  wealthRetireCfgSecFlow: "現金流與本金",
  wealthRetireCfgSecAssume: "報酬與提領假設",
  wealthRetireCurrentAgeLabel: "目前年齡",
  wealthRetireLifeLabel: "預期壽命",
  wealthRetireOtherIncomeLabel: "退休後其他月收入",
  wealthRetireOtherIncomeHint: "勞保年金、勞退月領、租金等，會從月支出中扣除",
  wealthRetireCfgContribLabel: "每月投入",
  wealthRetirePreRLabel: "退休前實質報酬",
  wealthRetirePostRLabel: "退休後實質報酬",
  wealthRetireSwrLabel: "年提領率",
  wealthRetireSwrHint: "4% 約等於 25 倍年支出",
  wealthRetireNetSpendLabel: "淨提領月支出",
  wealthRetireChartLabelWithAge: "資產推估（至 {age} 歲）",
  wealthRetireDepleteNeverWithAge: "{age} 歲後仍有結餘",

  navWealthInsure: "保單健檢",
  wealthInsureAdd: "新增保單",
  wealthInsureAddTitle: "新增保單",
  wealthInsureInsurerLabel: "保險公司",
  wealthInsurePolicyNameLabel: "保單名稱",
  wealthInsureKindLabel: "保障類型",
  wealthInsureAmountLabel: "保額",
  wealthInsuredLabel: "被保人",
  wealthInsureAnnualPremiumLabel: "年繳保費",
  wealthInsurePremiumYearsLabel: "繳費年期",
  wealthInsureBiggestGapLabel: "最大缺口",
  wealthInsureCoverageLabel: "覆蓋率",
  wealthInsureTotalGapLabel: "一次金缺口合計",
  wealthInsurePremiumLabel: "年繳保費",
  wealthInsurePremShareLabel: "保費佔年收入",
  wealthInsureCountLabel: "保單件數",
  wealthInsureGapTitle: "保障缺口與覆蓋率",
  wealthInsureHaveLabel: "現有保額",
  wealthInsureNeedLabel: "需求保額",
  wealthInsureGapLabel: "缺口",
  wealthInsureCoveredLabel: "已足額",
  wealthInsureKindLife: "壽險",
  wealthInsureKindAccident: "意外身故",
  wealthInsureKindCi: "重大疾病",
  wealthInsureKindCancer: "癌症一次金",
  wealthInsureKindDisability: "失能扶助",
  wealthInsureKindHospital: "住院日額",
  wealthInsurePerMonthSuffix: "／月",
  wealthInsurePerDaySuffix: "／日",
  wealthInsurePoliciesTitle: "保單清冊",
  wealthInsureNoPolicies: "尚未登錄任何保單。",
  wealthInsureSetupTitle: "設定家庭參數",
  wealthInsureDependentsLabel: "扶養人數",
  wealthInsureYoungestChildAgeLabel: "最小孩子年齡",
  wealthInsureSpouseIncomeLabel: "配偶是否有收入",
  wealthInsureSpouseIncomeYes: "有",
  wealthInsureSpouseIncomeNo: "沒有",
  wealthInsureSetupSave: "儲存",
  wealthInsureNeedPendingNote: "設定下方家庭參數後，才能算出壽險／意外／失能扶助的需求保額。",

  navWealthFunds: "基金與定期定額",
  wealthFundsMvLabel: "投資市值",
  wealthFundsCostLabel: "累積投入",
  wealthFundsPnlLabel: "未實現損益",
  wealthFundsMonthlyLabel: "每月扣款",
  wealthFundsChartTitle: "累積投入 vs 市值（近 24 個月）",
  wealthFundsCostLeg: "累積投入",
  wealthFundsMvLeg: "市值",
  wealthFundsChartAxis24: "24 個月前",
  wealthFundsChartAxis12: "12 個月前",
  wealthFundsChartAxisNow: "現在",
  wealthFundsScheduleTitle: "本月扣款排程",
  wealthFundsScheduleEmpty: "近期沒有排定扣款",
  wealthFundsTableTitle: "扣款標的與績效",
  wealthFundsColClass: "類別",
  wealthFundsColCode: "代號",
  wealthFundsColPlatform: "平台",
  wealthFundsColMonthly: "每月扣款",
  wealthFundsColCost: "累積投入",
  wealthFundsColMv: "市值",
  wealthFundsColPnl: "損益",
  wealthFundsColReturn: "報酬率",
  wealthFundsColOneYear: "近一年",
  wealthFundsColNext: "下次扣款",
  wealthFundsStopped: "已停扣",
  wealthFundsClassName: "基金",
  wealthFundsNoFunds: "尚無基金資料 — 請至匯入頁用 CSV 匯入。",
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
