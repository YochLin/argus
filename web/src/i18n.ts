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
  roundsClosedSuffix: string;
  roundsWon: string;
  roundsNet: string;
  roundsAll: string;
  roundsStatusOpen: string;
  roundsStatusClosed: string;
  roundsExpandAll: string;
  roundsCollapseAll: string;
  roundsHeld: string;
  roundsDaySuffix: string;
  roundsCountSuffix: string;
  roundsNoMatch: string;
  roundsFootnote: string;
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
  thesisAdd: string;
  thesisEdit: string;
  thesisEdited: string;
  thesisSave: string;
  thesisPlaceholder: string;
  currentThesisNote: string;
  thesisFieldPlaceholder: string;
  thesisSaveFailedNote: string;
  thesisEmptyOpen: string;
  thesisEmptyClosed: string;
  patTitle: string;
  pvChip: string;
  pvChipTip: string;
  maChip: string;
  maChipTip: string;
  pvLegend: string;
  pvUpVu: string;
  pvUpVd: string;
  pvDnVu: string;
  pvDnVd: string;
  pvReadPrice: string;
  pvReadVol: string;
  patHighOnly: string;
  patHighTip: string;
  patEvents: string;
  patEventsNote: string;
  patNone: string;
  patPick: string;
  patConf: string;
  patWhy: string;
  patWhyGap: string;
  patHist: string;
  patHistN: string;
  patHistAvg: string;
  patHistHit: string;
  patHistUp: string;
  patThisFwd: string;
  patFwdPending: string;
  patLowSample: string;
  patDisclaimer: string;
  patGapOpen: string;
  patGapFilled: string;
  patBars: string;
  patLegBull: string;
  patLegBear: string;
  patLegNeu: string;
  patLegGap: string;
  stratChip: string;
  stratChipTip: string;
  stratLeg: string;
  stratPushedWatchlist: string;
  stratPushedScan: string;
  stratMsgTitle: string;
  stratVerdictTitle: string;
  stratVerdictNone: string;
  stratValid_ok: string;
  stratValid_warn: string;
  stratValid_bad: string;
  stratNote_warn: string;
  stratNote_breakout: string;
  stratNote_mtfTw: string;
  stratNote_mtfUs: string;
  stratDisclaimer: string;
  stratCode_squeeze: string;
  stratName_squeeze: string;
  stratCode_box: string;
  stratName_box: string;
  stratCode_breakout: string;
  stratName_breakout: string;
  stratCode_pullback: string;
  stratName_pullback: string;
  stratCode_trust: string;
  stratName_trust: string;
  stratCode_mtf: string;
  stratName_mtf: string;
  tkTabPat: string;
  condBody: string;
  condVol: string;
  condPrior5: string;
  condBodyPrev: string;
  condCloseInto: string;
  condLowerShadow: string;
  condUpperShadow: string;
  condPrior20High: string;
  condPrior20Low: string;
  condPriorHighDate: string;
  condVolVsHigh: string;
  condRange10: string;
  condGapRange: string;
  condGapSize: string;
  condGapStatus: string;
  gapStatusOpen: string;
  gapStatusFilled: string;
  patCode_bullEngulf: string;
  patName_bullEngulf: string;
  patDef_bullEngulf: string;
  patCode_bearEngulf: string;
  patName_bearEngulf: string;
  patDef_bearEngulf: string;
  patCode_hammer: string;
  patName_hammer: string;
  patDef_hammer: string;
  patCode_shooting: string;
  patName_shooting: string;
  patDef_shooting: string;
  patCode_morning: string;
  patName_morning: string;
  patDef_morning: string;
  patCode_evening: string;
  patName_evening: string;
  patDef_evening: string;
  patCode_soldiers: string;
  patName_soldiers: string;
  patDef_soldiers: string;
  patCode_crows: string;
  patName_crows: string;
  patDef_crows: string;
  patCode_doji: string;
  patName_doji: string;
  patDef_doji: string;
  patCode_volBreak: string;
  patName_volBreak: string;
  patDef_volBreak: string;
  patCode_hangingMan: string;
  patName_hangingMan: string;
  patDef_hangingMan: string;
  patCode_invHammer: string;
  patName_invHammer: string;
  patDef_invHammer: string;
  patCode_bullHarami: string;
  patName_bullHarami: string;
  patDef_bullHarami: string;
  patCode_bearHarami: string;
  patName_bearHarami: string;
  patDef_bearHarami: string;
  patCode_piercing: string;
  patName_piercing: string;
  patDef_piercing: string;
  patCode_darkCloud: string;
  patName_darkCloud: string;
  patDef_darkCloud: string;
  patCode_volDry: string;
  patName_volDry: string;
  patDef_volDry: string;
  patCode_volDiverge: string;
  patName_volDiverge: string;
  patDef_volDiverge: string;
  patCode_gapUp: string;
  patName_gapUp: string;
  patDef_gapUp: string;
  patCode_gapDown: string;
  patName_gapDown: string;
  patDef_gapDown: string;
  patCode_volDump: string;
  patName_volDump: string;
  patDef_volDump: string;
  patCat_rev: string;
  patCat_cont: string;
  patCat_gap: string;
  patCat_vol: string;
  patCat_indec: string;
  patDir_bull: string;
  patDir_bear: string;
  patDir_neu: string;
  patConfLabel_high: string;
  patConfLabel_mid: string;
  patConfLabel_low: string;
  patCatTip_rev: string;
  patCatTip_cont: string;
  patCatTip_gap: string;
  patCatTip_vol: string;
  patCatTip_indec: string;
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
  peerTitle: string;
  peerRel: string;
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
  notesTagObservation: string;
  notesTagValuation: string;
  notesTagRisk: string;
  notesTagCatalyst: string;
  notesNote: string;
  notesCount: string;
  notesMatched: string;
  notesToThesis: string;
  notesShowing: string;
  notesAllShown: string;
  notesCollapse: string;
  notesNoMatch: string;
  notesMonthFmt: string;
  tkRoundLabel: string;
  tkPickRound: string;
  roundOlderTip: string;
  roundNewerTip: string;
  roundHolding: string;
  roundDaysUnit: string;
  roundsSummary: string;
  roundSpan: string;
  tkTabLvl: string;
  tkTabRound: string;
  snapTab: string;
  snapEmpty: string;
  snapPrevTip: string;
  snapNextTip: string;
  snapPickTip: string;
  snapRound: string;
  snapFillLine: string;
  snapDayLine: string;
  snapDayLineLive: string;
  snapProvenance: string;
  snapProvenanceLive: string;
  snapIntradayTag: string;
  snapVolPending: string;
  snapBarNote: string;
  snapNoData: string;
  snapLoadFail: string;
  snapAligned: string;
  snapAgainst: string;
  snapReadAsBuy: string;
  snapSellNote: string;
  indRsi: string;
  indMacd: string;
  indTrend: string;
  indVol: string;
  rsiHot: string;
  rsiCold: string;
  rsiMid: string;
  rsiSub: string;
  macdGolden: string;
  macdDeath: string;
  macdSub: string;
  macdWiden: string;
  macdNarrow: string;
  trendBullVal: string;
  trendBearVal: string;
  trendRangeVal: string;
  trendBullTag: string;
  trendBearTag: string;
  trendRangeTag: string;
  trendSub: string;
  volUpTag: string;
  volDownTag: string;
  volFlatTag: string;
  volSub: string;
  snapPatTitle: string;
  snapPatNone: string;
  snapNewsTitle: string;
  snapNewsNote: string;
  snapNewsNone: string;
  newsTag_earn: string;
  newsTag_guide: string;
  newsTag_analyst: string;
  newsTag_sector: string;
  newsTag_macro: string;
  newsTag_flow: string;
  newsTag_other: string;
  newsSent_bull: string;
  newsSent_bear: string;
  newsSent_neutral: string;
  newsSent_none: string;
  newsMajor: string;
  hindTitle: string;
  hindNote: string;
  hind5: string;
  hind20: string;
  hindBest: string;
  hindWorst: string;
  hindPending: string;
  snapDataTitle: string;
  snapDataNote: string;
  snapJsonView: string;
  snapJsonHide: string;
  snapCopy: string;
  snapCopied: string;
  levelsNote: string;
  pctOfRange: string;
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
  // wealthSheet* are the balance sheet's own four headings (design balanceModel:
  // grouped by asset type, not asset_group) — see wealthCategory.ts sheetGroups.
  wealthSheetLiquid: string;
  wealthSheetInvestment: string;
  wealthSheetHard: string;
  wealthSheetRetirement: string;
  // wealthCategory* back /w/alloc's own nine-category taxonomy
  // (assets.AllocCategories, §8.5) — distinct from the four wealthGroup*
  // keys above, which the home page still uses.
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
  // %s = the asset's name in the title and in the two flash toasts.
  wealthArchiveTitle: string;
  wealthArchiveBody: string;
  wealthFlashArchived: string;
  wealthFlashPaused: string;
  // /w/balance's 使用中/已封存 tabs and the archived table.
  wealthTabActive: string;
  wealthTabArchived: string;
  wealthArcDesc: string;
  wealthArcColKind: string;
  wealthArcColDate: string;
  wealthArcColValue: string;
  wealthArcNone: string;
  wealthRestore: string;
  wealthFlashRestored: string; // %s = name
  wealthRoNoChange: string;
  wealthSideAsset: string;
  wealthSideLiability: string;
  // A balance-sheet row's ⋯ button and click-to-edit value.
  wealthRowMore: string;
  wealthRowLinkGo: string;
  wealthRowLinkTitle: string;
  wealthRowEditHint: string;
  // The edit drawer (RowEditDrawer). %s placeholders are noted per key.
  wealthEdTitle: string;
  wealthEdTitleArchived: string;
  wealthEdLastRecord: string; // %s = date
  wealthEdNoRecord: string;
  wealthEdArchNote: string; // %s = archive date
  wealthEdRoNote: string;
  wealthEdLockSide: string;
  wealthEdLockCurrency: string;
  wealthEdLockSource: string;
  wealthEdLockNote: string;
  wealthEdInstPhAsset: string;
  wealthEdInstPhLiab: string;
  wealthEdLogTitle: string;
  wealthEdFixTitle: string;
  wealthEdCancelFix: string;
  wealthEdHintTwd: string;
  wealthEdHintFx: string; // %s = currency code
  wealthEdAdd: string;
  wealthEdUpdate: string;
  wealthEdErrAmount: string;
  wealthEdErrFuture: string;
  wealthEdErrName: string;
  wealthEdErrLocked: string;
  wealthEdHistTitle: string;
  wealthEdHistEmpty: string;
  wealthEdTagToday: string;
  wealthEdTagHistory: string;
  wealthEdFix: string;
  wealthEdDel: string;
  wealthEdHistNote: string;
  wealthEdSave: string;
  wealthEdClose: string;
  wealthEdArchive: string;
  wealthEdFlashSaved: string;
  wealthEdFlashLogged: string;
  wealthEdFlashFixed: string;
  wealthEdFlashDeleted: string;
  // Read-only mode (WEB_PASSWORD unset): the shell banner, its "how to
  // enable editing" steps, and the import pages' stand-in panels.
  roTag: string;
  roMsg: string;
  roHow: string;
  roHide: string;
  roStep1: string;
  roStep2: string;
  roStep3: string;
  roImportTitle: string;
  roImportBodyWealth: string;
  roImportBodyTrade: string;
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
  wealthKindCreditCard: string;
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
  wealthImportHintNote: string; // beside the header line: what is required, and that line 1 is the header
  wealthImportGuide: [string, string][]; // [what goes in the CSV, what it means], one per column / value
  wealthImportSample: string; // example rows, no header line — the template file and "load sample" both put one on top
  wealthImportTemplateName: string;
  wealthImportDownload: string;
  wealthImportLoadSample: string;
  wealthImportClear: string;
  wealthImportSummary: string;
  wealthImportNoHeader: string;
  wealthImportDuplicateMsg: string; // the server's own text for this status is English only
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
  wealthRateLabel: string;
  wealthNetWorthFormula: string;
  wealthStaleDaysSuffix: string;
  wealthPctOfLiabilities: string;
  wealthLiabShortTerm: string;
  wealthLiabLongTerm: string;
  wealthDebtRatioNote: string;
  wealthLiquidityNote: string;
  wealthSavingsRateNote: string;
  wealthExpenseRatioNote: string;
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
  wealthCashPauseTitle: string; // tooltip on a row's pause button
  wealthCashPaused: string;
  wealthCashPausedNote: string;
  wealthCashPausedSince: string; // %s = date
  wealthCashPerMonth: string;
  wealthCashTagIn: string; // the paused card's per-row direction tag
  wealthCashTagOut: string;
  wealthNoRate: string; // %s = currency code; tooltip on an amount that couldn't be converted to TWD
  wealthCashResume: string;
  wealthCashDelete: string;
  wealthCashDeleteTitle: string; // %s = name
  wealthCashDeleteBody: string;
  wealthFlashResumed: string; // %s = name
  wealthFlashDeleted: string; // %s = name
  // Onboarding: the empty-page card (one line per page) and the /w setup guide.
  wealthEmptyNet: string;
  wealthEmptyAlloc: string;
  wealthEmptyBalance: string;
  wealthEmptyCash: string;
  wealthEmptyRetire: string;
  wealthEmptyInsure: string;
  wealthGuideTitle: string;
  wealthGuideDone: string; // %s = steps done, %s = total
  wealthGuideHide: string;
  wealthGuideWhyBalance: string;
  wealthGuideWhyCash: string;
  wealthGuideWhyRetire: string;
  wealthGuideWhyAlloc: string;
  wealthGuideWhyInsure: string;
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
  wealthCashForecastTitle: string;
  wealthCashForecastNote: string;
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
  // wealthRetireTipDeplete/wealthRetireTipSwr back the design's two
  // dotted-underline tooltips (native title attr, DOTTED style) on
  // "資產耗盡"/the settings drawer's 提領率 field's read-only summary chip.
  // wealthRetireChip* are the read-only assumption chips next to the
  // header's quick-switch buttons (design's wrm.summary) — short, distinct
  // wording from the KPI/drawer labels above, each carrying a "{v}"-style
  // token the component substitutes (same convention as
  // wealthRetireChartLabelWithAge above).
  wealthRetireTipDeplete: string;
  wealthRetireTipSwr: string;
  wealthRetireChipSpend: string;
  wealthRetireChipContrib: string;
  wealthRetireChipReturn: string;
  wealthRetireChipSwr: string;
  wealthRetireChipLifeAge: string;
  // Sample-mode banner ("範例") shown instead of a real projection when no
  // birth year is on file yet (design's wrm.isSample) — the projection
  // still renders (using a sample age), tagged so it never reads as real.
  wealthRetireSampleTag: string;
  wealthRetireSampleMsg: string;
  wealthRetireSampleBtn: string;

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
  // wealthInsureNeedHow is the gap card's subtitle explaining the need
  // figures are this app's own estimate, not an insurer's number.
  // wealthInsureTipCoverage/wealthInsureTipNeed back the design's two
  // dotted-underline hover tooltips (native title attr + DOTTED style) on
  // "覆蓋率"/"需求保額" — same pattern WealthHomeView/WealthAllocView
  // already use for their own dotted-underline labels.
  wealthInsureNeedHow: string;
  wealthInsureTipCoverage: string;
  wealthInsureTipNeed: string;
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
  // wealthInsureColType/wealthInsureColTerm are the policy table's own
  // column-header wording (design reuses shorter contextual text there —
  // "類型"/"繳費／保障期間" — distinct from wealthInsureKindLabel/
  // wealthInsurePremiumYearsLabel, which stay the add-form field labels).
  wealthInsureColType: string;
  wealthInsureColTerm: string;
  wealthInsureNoPolicies: string;
  wealthInsureSetupTitle: string;
  wealthInsureDependentsLabel: string;
  wealthInsureYoungestChildAgeLabel: string;
  wealthInsureSpouseIncomeLabel: string;
  wealthInsureSpouseIncomeYes: string;
  wealthInsureSpouseIncomeNo: string;
  wealthInsureSetupSave: string;
  wealthInsureNeedPendingNote: string;

  // wealthLastImport/wealthImportSourceCsv back the design's 最後匯入 hint —
  // shared across every wealth page with a CSV import path (funds today,
  // bonds/tax later), same "generic label, page supplies the date" shape
  // the design template itself reuses (t.wLastImport/t.wDataSource).
  wealthLastImport: string;
  wealthImportSourceCsv: string;

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
  wealthFundsColCode: string;
  wealthFundsColPlatform: string;
  wealthFundsColMonthly: string;
  wealthFundsColCost: string;
  wealthFundsColMv: string;
  wealthFundsColReturn: string;
  wealthFundsColOneYear: string;
  wealthFundsColNext: string;
  wealthFundsStopped: string;
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
  roundsClosedSuffix: "closed",
  roundsWon: "won",
  roundsNet: "net",
  roundsAll: "All",
  roundsStatusOpen: "Open",
  roundsStatusClosed: "Closed",
  roundsExpandAll: "Expand all",
  roundsCollapseAll: "Collapse all",
  roundsHeld: "Held",
  roundsDaySuffix: "d",
  roundsCountSuffix: "rounds",
  roundsNoMatch: "No rounds match the filters.",
  roundsFootnote:
    "Closed rounds are grouped by the month they closed, matching the realized-P&L month in the calendar and reports.",
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
  thesisAdd: "Add thesis",
  thesisEdit: "Edit",
  thesisEdited: "edited",
  thesisSave: "Save",
  thesisPlaceholder: "Entry reason / hold condition / invalidation",
  currentThesisNote: "current thesis on record — may have changed since this round closed",
  thesisFieldPlaceholder: "Your reasoning right now",
  thesisSaveFailedNote: "Trade went through, but the thesis note failed to save",
  thesisEmptyOpen:
    "No thesis was recorded when this round was opened. Write down the entry reason, the hold condition, and what would invalidate it.",
  thesisEmptyClosed: "No thesis was recorded for this round.",
  patTitle: "PATTERNS",
  pvChip: "Price · volume",
  pvChipTip: "Colours every volume bar by price and volume direction",
  maChip: "MA",
  maChipTip: "Draws the 5 / 20 / 60-day simple moving averages of the close",
  pvLegend: "Volume",
  pvUpVu: "Price up · vol up",
  pvUpVd: "Price up · vol down",
  pvDnVu: "Price down · vol up",
  pvDnVd: "Price down · vol down",
  pvReadPrice: "px ",
  pvReadVol: "vol ",
  patHighOnly: "High confidence only",
  patHighTip:
    "Confidence scores three checks: volume above 1.4× the 20-day average, body above 1.5× the 20-day average body, and the right context (reversals must follow the opposite trend). All three = high. On: only high-confidence patterns. Off: medium and low appear as small unlabelled dots.",
  patEvents: "PATTERN & STRATEGY EVENTS",
  patEventsNote: "newest first — click to locate on chart",
  patNone: "No patterns detected under the current filters.",
  patPick: "Click a pattern dot on the chart, or an event below, to see why it fired and how it has played out on this ticker.",
  patConf: "confidence",
  patWhy: "WHY IT FIRED",
  patWhyGap: "GAP DETAILS",
  patHist: "SAME PATTERN ON THIS TICKER",
  patHistN: "Occurrences",
  patHistAvg: "Avg 5-day move",
  patHistHit: "Went its way",
  patHistUp: "Up after 5 days",
  patThisFwd: "This one, 5 days on",
  patFwdPending: "under 5 days old",
  patLowSample: "small sample — indicative only",
  patDisclaimer: "Patterns are detected from bar shapes and describe price/volume behaviour only. Not a trade signal.",
  patGapOpen: "unfilled",
  patGapFilled: "filled",
  patBars: "%s-bar",
  patLegBull: "Bullish (below bar)",
  patLegBear: "Bearish (above bar)",
  patLegNeu: "Neutral",
  patLegGap: "Gap (dashed = unfilled)",
  stratChip: "Strategy",
  stratChipTip: "Strategy signals the bot pushed (Nets 1–5, daily-weekly cross) — only ones that were really sent; the record starts the day this shipped",
  stratLeg: "Strategy alert (square)",
  stratPushedWatchlist: "Signal on %s — caught in the daily report and pushed (watchlist / holding)",
  stratPushedScan: "Signal on %s — caught by the universe scan and pushed",
  stratMsgTitle: "WHAT WAS SENT",
  stratVerdictTitle: "LLM CALL AT THE TIME",
  stratVerdictNone: "The LLM made no call on this ticker within 4 days.",
  stratValid_ok: "POSITIVE ON TW",
  stratValid_warn: "NOT VALIDATED",
  stratValid_bad: "CONTRARIAN",
  stratNote_warn: "Its backtest has not passed the validation bar — treat it as a watchlist cue, not an entry.",
  stratNote_breakout: "All four samples showed negative excess return.",
  stratNote_mtfTw: "The only screen with a positive live test on TW — but its thresholds were picked after seeing both samples, so it stays a watchlist cue.",
  stratNote_mtfUs: "Negative in US tests, so effectively a contrarian indicator. Alerts continue only because you asked to keep watching it.",
  stratDisclaimer: "Markers are the alerts that were really pushed; the record starts when this shipped. A strategy that has not passed validation is a cue, not a trade signal.",
  stratCode_squeeze: "N1",
  stratName_squeeze: "Squeeze Breakout",
  stratCode_box: "N2",
  stratName_box: "Box Bottom Rebound",
  stratCode_breakout: "N3",
  stratName_breakout: "Trend Breakout",
  stratCode_pullback: "N4",
  stratName_pullback: "Trend Pullback",
  stratCode_trust: "N5",
  stratName_trust: "Trust Follow",
  stratCode_mtf: "D/W",
  stratName_mtf: "Daily-Weekly MA Cross",
  tkTabPat: "Patterns",
  condBody: "Body vs 20-day avg",
  condVol: "Volume vs 20-day avg",
  condPrior5: "Prior 5-day move",
  condBodyPrev: "Body vs prior bar",
  condCloseInto: "Close into prior body",
  condLowerShadow: "Lower shadow / body",
  condUpperShadow: "Upper shadow / body",
  condPrior20High: "Prior 20-day high",
  condPrior20Low: "Prior 20-day low",
  condPriorHighDate: "Prior high (date)",
  condVolVsHigh: "Volume vs prior high",
  condRange10: "10-day range",
  condGapRange: "Gap range",
  condGapSize: "Gap size",
  condGapStatus: "Status",
  gapStatusOpen: "Unfilled · %s sessions open",
  gapStatusFilled: "Filled on day %s (%s)",
  patCode_bullEngulf: "E",
  patName_bullEngulf: "Bullish engulfing",
  patDef_bullEngulf: "A red bar followed by a long green bar whose body fully covers it. After a decline, read as buyers taking over.",
  patCode_bearEngulf: "E",
  patName_bearEngulf: "Bearish engulfing",
  patDef_bearEngulf: "A green bar followed by a long red bar whose body fully covers it. After a rally, read as sellers stepping in.",
  patCode_hammer: "H",
  patName_hammer: "Hammer",
  patDef_hammer: "Lower shadow at least 2× the body, little upper shadow, after a decline. Sold off intraday, then bought back.",
  patCode_shooting: "S",
  patName_shooting: "Shooting star",
  patDef_shooting: "Upper shadow at least 2× the body, little lower shadow, after a rally. Pushed up intraday, then sold back.",
  patCode_morning: "M",
  patName_morning: "Morning star",
  patDef_morning: "Long red → small body → long green closing above the first body's midpoint. A three-bar bottom reversal.",
  patCode_evening: "V",
  patName_evening: "Evening star",
  patDef_evening: "Long green → small body → long red closing below the first body's midpoint. A three-bar top reversal.",
  patCode_soldiers: "3W",
  patName_soldiers: "Three white soldiers",
  patDef_soldiers: "Three green bars in a row, each closing higher with a solid body. Buyers keep pressing.",
  patCode_crows: "3C",
  patName_crows: "Three black crows",
  patDef_crows: "Three red bars in a row, each closing lower with a solid body. Sellers keep pressing.",
  patCode_doji: "D",
  patName_doji: "Doji",
  patDef_doji: "Open and close almost equal with visible shadows. A standoff — wait for the next bar to confirm.",
  patCode_volBreak: "BO",
  patName_volBreak: "Volume breakout",
  patDef_volBreak: "Close at a 20-day high on volume above 1.8× the 20-day average.",
  patCode_hangingMan: "HM",
  patName_hangingMan: "Hanging man",
  patDef_hangingMan: "Same shape as a hammer (long lower shadow, short upper) but after a rally. Heavy intraday selling — buyers are loosening their grip.",
  patCode_invHammer: "IH",
  patName_invHammer: "Inverted hammer",
  patDef_invHammer: "Same shape as a shooting star (long upper shadow, short lower) but after a decline. Buyers start probing higher.",
  patCode_bullHarami: "Hr",
  patName_bullHarami: "Bullish harami",
  patDef_bullHarami: "A long red bar followed by a small green bar whose body sits inside the prior body. The decline pauses; needs confirmation.",
  patCode_bearHarami: "Hr",
  patName_bearHarami: "Bearish harami",
  patDef_bearHarami: "A long green bar followed by a small red bar whose body sits inside the prior body. The rally pauses; needs confirmation.",
  patCode_piercing: "PL",
  patName_piercing: "Piercing line",
  patDef_piercing: "After a long red bar, a green bar opens at or below the prior close and closes above its midpoint without fully engulfing it. A weaker bullish engulfing.",
  patCode_darkCloud: "DC",
  patName_darkCloud: "Dark cloud cover",
  patDef_darkCloud: "After a long green bar, a red bar opens at or above the prior close and closes below its midpoint without fully engulfing it. A weaker bearish engulfing.",
  patCode_volDry: "DV",
  patName_volDry: "Volume dry-up",
  patDef_volDry: "Volume under half the 20-day average during a tight 10-day range. Common near the end of a consolidation, before a directional move.",
  patCode_volDiverge: "DIV",
  patName_volDiverge: "Price-volume divergence",
  patDef_volDiverge: "Close at a 20-day high on less volume than at the previous high. The new high lacks volume support.",
  patCode_gapUp: "GU",
  patName_gapUp: "Gap up",
  patDef_gapUp: "Today's low is above yesterday's high, leaving a price range with no trades. Filled once price trades back into it.",
  patCode_gapDown: "GD",
  patName_gapDown: "Gap down",
  patDef_gapDown: "Today's high is below yesterday's low, leaving a price range with no trades. Filled once price trades back into it.",
  patCode_volDump: "BD",
  patName_volDump: "Volume breakdown",
  patDef_volDump: "Close at a 20-day low on volume above 1.8× the 20-day average.",
  patCat_rev: "Reversal",
  patCat_cont: "Continuation",
  patCat_gap: "Gaps",
  patCat_vol: "Volume event",
  patCat_indec: "Indecision",
  patDir_bull: "Bullish",
  patDir_bear: "Bearish",
  patDir_neu: "Neutral",
  patConfLabel_high: "high",
  patConfLabel_mid: "med",
  patConfLabel_low: "low",
  patCatTip_rev: "Engulfing, piercing / dark cloud, harami, hammer / hanging man, shooting star / inverted hammer, morning / evening star",
  patCatTip_cont: "Three soldiers, three crows",
  patCatTip_gap: "Gap up / down, drawn as a price band; unfilled gaps extend to today",
  patCatTip_vol: "Volume breakout / breakdown, dry-up, price-volume divergence",
  patCatTip_indec: "Doji — frequent, off by default",
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
  searchPlaceholder: "Search ticker…",
  watchlistCount: "tracked",
  noMatch: "No matching tickers found.",
  heldOnly: "Holdings only",
  nearestSup: "Nearest Support",
  nearestRes: "Nearest Resistance",
  ma20: "MA20",
  ma60: "MA60",
  atr14: "ATR(14)",
  ret20: "20d return",
  fromHigh: "From High",
  vsAvg20: "vs 20d avg",
  volume: "Volume",
  above: "above",
  below: "below",
  rangeHigh: "Range high",
  rangeLow: "Range low",
  rangeNote: "last 120 sessions",
  thisPosition: "Current Position",
  noPositionHere: "No active position in this ticker.",
  riskIfStopped: "Risk if Stopped",
  pctOfAccount: "of account",
  tickerRounds: "Rounds in this name",
  noRoundsHere: "No closed rounds for this ticker.",
  peerTitle: "PEER COMPARISON",
  peerRel: "Rel. (60d)",
  roundPicker: "Select a round — the chart zooms to its span",
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
  notesAddToggle: "Add Today's Note",
  notesEditToggle: "Edit Today's Note",
  notesFieldPlaceholder: "What did you see on the chart today?",
  notesEmptyNote: "No notes yet. Write down what you observe while researching — you can pull it straight into a thesis when you enter.",
  notesSearchPlaceholder: "Search note text or date (e.g. 2026-05)",
  notesClearSearch: "Clear",
  notesPinnedLabel: "PINNED",
  notesPinLabel: "Pin",
  notesUnpinLabel: "Unpin",
  notesDeleteLabel: "Delete",
  notesDeleteConfirm: "Delete this note? This can't be undone.",
  notesLoadMore: "Load 20 more",
  notesFilterAllLabel: "All",
  notesTagTechnical: "Technical",
  notesTagFlow: "Flow",
  notesTagNews: "News",
  notesTagOther: "Other",
  notesTagObservation: "Observation",
  notesTagValuation: "Valuation",
  notesTagRisk: "Risk",
  notesTagCatalyst: "Catalyst",
  notesNote: "your own analysis log for this ticker — kept per ticker, independent of trading rounds",
  notesCount: "notes",
  notesMatched: "matched",
  notesToThesis: "Use as round thesis",
  notesShowing: "Showing",
  notesAllShown: "Reached the earliest note",
  notesCollapse: "Collapse",
  notesNoMatch: "Nothing matched — try another keyword or tag.",
  notesMonthFmt: "{m} {y}",
  tkRoundLabel: "ROUND",
  tkPickRound: "Pick a round",
  roundOlderTip: "Older round",
  roundNewerTip: "Newer round",
  roundHolding: "open",
  roundDaysUnit: "d",
  roundsSummary: "%s rounds · %s/%s won · net %s",
  roundSpan: "Round",
  tkTabLvl: "Levels",
  tkTabRound: "Rounds",
  snapTab: "Fills",
  snapEmpty: "No fills on this ticker yet. Each fill saves the news around it; indicators and candle events are worked out when you open it.",
  snapPrevTip: "Older fill",
  snapNextTip: "Newer fill",
  snapPickTip: "Pick a fill",
  snapRound: "Round #%s",
  snapFillLine: "%s sh @ %s",
  snapDayLine: "Day %s · close %s · H %s · L %s",
  snapDayLineLive: "Day so far %s · last %s · H %s · L %s",
  snapProvenance: "Indicators on that day's close · worked out each time you open it",
  snapProvenanceLive: "Session still open · readings use the latest price and are worked out again each time you open it",
  snapIntradayTag: "Intraday",
  snapVolPending: "Volume ratio not final — the session isn't over",
  snapBarNote: "Matched to the %s session",
  snapNoData: "No candle data for this fill — it is older than the price history available, or there is too little history before it.",
  snapLoadFail: "Couldn't load the snapshot.",
  snapAligned: "%s of 4 with the trade",
  snapAgainst: " · %s against",
  snapReadAsBuy: "read as a buy",
  snapSellNote: "A sell gets the numbers only — selling into strength and into weakness can each be right, so nothing is scored as with or against.",
  indRsi: "RSI(14)",
  indMacd: "MACD",
  indTrend: "Trend",
  indVol: "Volume",
  rsiHot: "Overbought",
  rsiCold: "Oversold",
  rsiMid: "Neutral",
  rsiSub: "5d ago %s %s",
  macdGolden: "Bull cross +%sd",
  macdDeath: "Bear cross +%sd",
  macdSub: "DIF %s · DEA %s · %s",
  macdWiden: "hist widening",
  macdNarrow: "hist narrowing",
  trendBullVal: "Up",
  trendBearVal: "Down",
  trendRangeVal: "Range",
  trendBullTag: "MA stack up",
  trendBearTag: "MA stack down",
  trendRangeTag: "No clean trend",
  trendSub: "Close vs MA20 %s · MA20 5d slope %s",
  volUpTag: "Expanding",
  volDownTag: "Drying up",
  volFlatTag: "Normal",
  volSub: "Day / 20d avg · 5d avg / 20d %s",
  snapPatTitle: "Candle events that day",
  snapPatNone: "No pattern ended on this bar.",
  snapNewsTitle: "News around the fill",
  snapNewsNote: "Headlines saved when the fill was recorded · labels are an LLM's reading of the headline alone, rough and for reference only",
  snapNewsNone: "No news was saved for this fill — none was found when it was recorded, or it was imported or added after the fact.",
  newsTag_earn: "Earnings",
  newsTag_guide: "Guidance",
  newsTag_analyst: "Analyst",
  newsTag_sector: "Sector",
  newsTag_macro: "Macro",
  newsTag_flow: "Flow",
  newsTag_other: "Other",
  newsSent_bull: "Positive",
  newsSent_bear: "Negative",
  newsSent_neutral: "Neutral",
  newsSent_none: "Not labelled",
  newsMajor: "Major",
  hindTitle: "What happened next",
  hindNote: "Known only after the fill · not part of the read above · colour = for / against this decision",
  hind5: "+5d",
  hind20: "+20d",
  hindBest: "Best in 20d",
  hindWorst: "Worst in 20d",
  hindPending: "n/a",
  snapDataTitle: "Snapshot data",
  snapDataNote: "Indicators and candle events as of the fill day's close, plus the saved news — without what happened afterwards. The AI review of a closed round reads the entry and exit days' indicators and headlines (not the candle events, the labels or what came after).",
  snapJsonView: "View",
  snapJsonHide: "Hide",
  snapCopy: "Copy",
  snapCopied: "Copied",
  levelsNote: "● drawn on chart (nearest three each side)",
  pctOfRange: "Range position",
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
  wealthSheetLiquid: "Liquid assets",
  wealthSheetInvestment: "Investments",
  wealthSheetHard: "Hard assets",
  wealthSheetRetirement: "Retirement",
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
  wealthArchiveTitle: "Archive “%s”?",
  wealthArchiveBody:
    "It will no longer count toward net worth, allocation or ratios. All value records are kept, and you can restore it from Archived on the balance sheet.",
  wealthFlashArchived: "Archived “%s”",
  wealthFlashPaused: "Paused “%s”",
  wealthTabActive: "Active",
  wealthTabArchived: "Archived",
  wealthArcDesc: "Not counted in net worth, allocation or ratios. Value records are kept in full.",
  wealthArcColKind: "Type",
  wealthArcColDate: "Archived on",
  wealthArcColValue: "Value when archived",
  wealthArcNone: "Nothing archived. Any entry can be archived from its “⋯” menu.",
  wealthRestore: "Restore",
  wealthFlashRestored: "Restored “%s”",
  wealthRoNoChange: "Read-only: set a password to make changes",
  wealthSideAsset: "Asset",
  wealthSideLiability: "Liability",
  wealthRowMore: "Edit · history · archive",
  wealthRowLinkGo: "Open trading account",
  wealthRowLinkTitle: "Synced from the trading account — click to open",
  wealthRowEditHint: "Click to edit (Enter saves, Esc cancels)",
  wealthEdTitle: "EDIT ENTRY",
  wealthEdTitleArchived: "ARCHIVED ENTRY",
  wealthEdLastRecord: "last record %s",
  wealthEdNoRecord: "no records yet",
  wealthEdArchNote: "Archived %s · excluded from net worth, allocation and ratios. History is kept read-only.",
  wealthEdRoNote: "Read-only: view only. Set WEB_PASSWORD to edit, log values or archive.",
  wealthEdLockSide: "Side",
  wealthEdLockCurrency: "Currency",
  wealthEdLockSource: "Source",
  wealthEdLockNote: "Side and currency are fixed once created. To change them, archive this entry and add a new one.",
  wealthEdInstPhAsset: "e.g. Bank of Taiwan",
  wealthEdInstPhLiab: "e.g. Cathay Bank",
  wealthEdLogTitle: "LOG NEW VALUE",
  wealthEdFixTitle: "CORRECT TODAY'S RECORD",
  wealthEdCancelFix: "Cancel correction",
  wealthEdHintTwd: "Enter the amount in TWD",
  wealthEdHintFx: "Enter the amount in %s; converted to TWD at that day's rate",
  wealthEdAdd: "Add record",
  wealthEdUpdate: "Update record",
  wealthEdErrAmount: "Enter an amount",
  wealthEdErrFuture: "Date can't be in the future",
  wealthEdErrName: "Name is required",
  wealthEdErrLocked: "That day already has a record, and past records can't be changed",
  wealthEdHistTitle: "VALUE HISTORY",
  wealthEdHistEmpty: "No value records yet",
  wealthEdTagToday: "Logged today · editable",
  wealthEdTagHistory: "History",
  wealthEdFix: "Correct",
  wealthEdDel: "Delete",
  wealthEdHistNote:
    "Records can be corrected or deleted on the day they're logged; after that they become history. To fix an older one, log a new value.",
  wealthEdSave: "Save",
  wealthEdClose: "Close",
  wealthEdArchive: "Archive this entry",
  wealthEdFlashSaved: "Saved",
  wealthEdFlashLogged: "Value logged",
  wealthEdFlashFixed: "Record corrected",
  wealthEdFlashDeleted: "Record deleted",
  roTag: "READ-ONLY",
  roMsg:
    "WEB_PASSWORD isn't set on the server, so this is view-only. Adding, editing, archiving and import are disabled.",
  roHow: "How to enable editing →",
  roHide: "Hide",
  roStep1: "Set a password in the server's environment",
  roStep2: "Restart the service",
  roStep3: "Come back and enter the same password the first time you edit",
  roImportTitle: "Import needs edit access",
  roImportBodyWealth:
    "This is read-only, so nothing can be written. You can still download the template and prepare your CSV using the column guide above, then paste it in once a password is set.",
  roImportBodyTrade:
    "This is read-only, so trades can't be imported. After setting WEB_PASSWORD and restarting, the import form appears here.",
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
  wealthKindCreditCard: "Credit card",
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
    "Paste or upload a CSV, preview the result row by row, then write it in — the fastest way to enter your first batch of accounts.",
  wealthImportHintNote:
    "side, type, name, group and value are required, the rest may be blank; every row needs at least the first 8 columns (pad with commas). The first line is the header and is not imported.",
  wealthImportGuide: [
    ["asset", "side: an asset"],
    ["liability", "side: a liability"],
    ["type", "Assets: deposit, fund, bond, estate, gold, crypto, pension, other. Liabilities: loan, credit_card"],
    ["name", "Required. The same name and type as an existing record is skipped"],
    ["liquid", "group: available any time, e.g. savings, deposits"],
    ["growth", "group: stocks, ETFs, equity funds"],
    ["income", "group: bonds, dividend funds, savings policies"],
    ["hard", "group: property, gold, pension"],
    ["venue", "Bank or platform; may be blank"],
    ["currency", "e.g. TWD, USD; blank means TWD"],
    ["value", "Current value in that currency; enter a liability as a positive number"],
    ["date", "Valuation date, YYYY-MM-DD; blank means today"],
    ["bank", "deposit: bank"],
    ["accountNote", "deposit: account note"],
    ["lender", "loan: lender"],
    ["ratePct", "loan: annual rate in %, e.g. 2.1"],
    ["originalPrincipal", "loan: original principal"],
    ["remainingMonths", "loan: months left"],
    ["fundCode", "fund: code, e.g. F00012"],
    ["fundPlatform", "fund: where it was bought"],
    ["fundMonthlyAmount", "fund: monthly auto-invest; blank for a lump sum"],
    ["fundNextContributionDate", "fund: next debit date, YYYY-MM-DD"],
  ],
  wealthImportSample:
    "asset,deposit,Time deposit - Bank X,liquid,Bank X,TWD,350000,,Bank X,Salary account,,,,,,,,\n" +
    "asset,fund,Taiwan Tech Fund,growth,Fund platform,TWD,186000,,,,,,,,F00012,Fund platform,5000,2026-11-06\n" +
    "asset,gold,Gold passbook,hard,Bank X,TWD,128000,,,,,,,,,,,\n" +
    "liability,loan,Renovation loan,hard,Bank X,TWD,280000,,,,Bank X,2.1,500000,48,,,,",
  wealthImportTemplateName: "argus-wealth-template.csv",
  wealthImportDownload: "Download template",
  wealthImportLoadSample: "Load sample",
  wealthImportClear: "Clear",
  wealthImportSummary: "ROW RESULTS",
  wealthImportNoHeader:
    "The first line looks like data, not a header — the first line is always skipped. Add a header row on top (Load sample shows the format).",
  wealthImportDuplicateMsg: "Same name and type as an existing record — skipped",
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
  wealthRateLabel: "Rate",
  wealthNetWorthFormula: "NET WORTH = ASSETS − LIABILITIES",
  wealthStaleDaysSuffix: "%sd not updated",
  wealthPctOfLiabilities: "of liabilities",
  wealthLiabShortTerm: "SHORT TERM",
  wealthLiabLongTerm: "LONG TERM",
  wealthDebtRatioNote: "target under 40%",
  wealthLiquidityNote: "liquid assets ÷ monthly spending",
  wealthSavingsRateNote: "(salary − spending) ÷ salary",
  wealthExpenseRatioNote: "monthly spending ÷ salary",
  navWealthCash: "Cash Flow",
  wealthCashMonthlyIn: "Monthly income",
  wealthCashMonthlyOut: "Monthly expense",
  wealthCashMonthlyNet: "Monthly net",
  wealthCashSaveRateLabel: "Savings rate",
  wealthCashDcaShareLabel: "DCA / income",
  wealthCashFixedShareLabel: "Fixed / income",
  wealthCashAnnualNetLabel: "Full-year surplus",
  wealthCashNet90Label: "Net over period",
  wealthCashInBreakdownTitle: "MONTHLY INCOME",
  wealthCashOutBreakdownTitle: "MONTHLY OUTFLOW",
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
  wealthCashPauseTitle: "Excluded from monthly totals and forecast until resumed",
  wealthCashPaused: "Paused",
  wealthCashPausedNote: "Not counted in monthly totals or the forecast. Counting starts again from the next cycle once resumed.",
  wealthCashPausedSince: "paused %s",
  wealthCashPerMonth: " /mo",
  wealthCashTagIn: "In",
  wealthCashTagOut: "Out",
  wealthNoRate: "No %s exchange rate available — can't convert to TWD",
  wealthCashResume: "Resume",
  wealthCashDelete: "Delete",
  wealthCashDeleteTitle: "Delete “%s”?",
  wealthCashDeleteBody: "It will no longer appear in cash flow or the forecast. Past actual transactions are not affected. This can't be undone.",
  wealthFlashResumed: "Resumed “%s”",
  wealthFlashDeleted: "Deleted “%s”",
  wealthEmptyNet: "No assets or liabilities yet. Add the first one from the top right and net worth starts filling in.",
  wealthEmptyAlloc: "Nothing to allocate yet. Add an asset and the current-vs-target view appears here.",
  wealthEmptyBalance: "The balance sheet is empty. Add your first asset or liability from the top right.",
  wealthEmptyCash: "No income or expense items yet. Add a cash-flow item from the top right.",
  wealthEmptyRetire: "Nothing to project yet. Add your first asset from the top right, then tune the assumptions.",
  wealthEmptyInsure: "No policies on file yet. Add a policy from the top right.",
  wealthGuideTitle: "SUGGESTED ORDER",
  wealthGuideDone: "%s of %s done",
  wealthGuideHide: "Hide",
  wealthGuideWhyBalance: "Record what you own and owe so net worth can be computed",
  wealthGuideWhyCash: "Monthly cash flow sets how much you can save — retirement and goals depend on it",
  wealthGuideWhyRetire: "Set your birth year so the projection has a starting point",
  wealthGuideWhyAlloc: "Pick conservative, balanced or growth so drift has a baseline",
  wealthGuideWhyInsure: "Optional — needed to see coverage gaps",
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
  wealthCashForecastTitle: "12-month cash flow forecast",
  wealthCashForecastNote: "A flat projection of this month's net across the next year — it doesn't try to guess annual premiums, tax bills, or bonus timing.",
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
  wealthRetireTipDeplete: "Age at which assets run out under these assumptions. Safe if later than life expectancy.",
  wealthRetireTipSwr: "Share of assets withdrawn each year in retirement. 4% is a common conservative baseline.",
  wealthRetireChipSpend: "Spend {v}",
  wealthRetireChipContrib: "Contrib {v}",
  wealthRetireChipReturn: "Real {pre}% / {post}%",
  wealthRetireChipSwr: "SWR {v}%",
  wealthRetireChipLifeAge: "To {v}",
  wealthRetireSampleTag: "SAMPLE",
  wealthRetireSampleMsg: "Your age isn't set yet. This is a sample projection (age {age}, retiring at {ret}, pool {pool}) — not your result.",
  wealthRetireSampleBtn: "Enter my age",

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
  wealthInsureNeedHow: "Needed cover is an estimate from your debts, household spending and income — not an insurer's figure. Hover “needed” to see how each line is derived.",
  wealthInsureTipCoverage: "Current cover ÷ needed cover. 100% means fully covered; under 50% is a clear gap.",
  wealthInsureTipNeed: "Rule-of-thumb estimates: life = debts + 10 yrs of household spending; accident = half of life; critical illness = 3–5 yrs of income; cancer = one course of treatment; disability = 60% of monthly income; hospital = daily income.",
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
  wealthInsureColType: "Type",
  wealthInsureColTerm: "Term",
  wealthInsureNoPolicies: "No policies recorded yet.",
  wealthInsureSetupTitle: "Set up household details",
  wealthInsureDependentsLabel: "Dependents",
  wealthInsureYoungestChildAgeLabel: "Youngest child's age",
  wealthInsureSpouseIncomeLabel: "Spouse has independent income",
  wealthInsureSpouseIncomeYes: "Yes",
  wealthInsureSpouseIncomeNo: "No",
  wealthInsureSetupSave: "Save",
  wealthInsureNeedPendingNote: "Set up household details below to see life/accidental-death/disability need estimates.",

  wealthLastImport: "LAST IMPORT",
  wealthImportSourceCsv: "Platform statement (CSV)",

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
  wealthFundsColCode: "Code",
  wealthFundsColPlatform: "Platform",
  wealthFundsColMonthly: "Monthly",
  wealthFundsColCost: "Invested",
  wealthFundsColMv: "Market value",
  wealthFundsColReturn: "Return",
  wealthFundsColOneYear: "1-yr",
  wealthFundsColNext: "Next",
  wealthFundsStopped: "lump sum",
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
  buy: "買入",
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
  roundsClosedSuffix: "筆已平倉",
  roundsWon: "勝",
  roundsNet: "淨",
  roundsAll: "全部",
  roundsStatusOpen: "持有中",
  roundsStatusClosed: "已平倉",
  roundsExpandAll: "全部展開",
  roundsCollapseAll: "全部收合",
  roundsHeld: "持有",
  roundsDaySuffix: " 天",
  roundsCountSuffix: "筆",
  roundsNoMatch: "沒有符合條件的回合",
  roundsFootnote: "已平倉回合依平倉月份歸組，與月曆、績效報表的已實現損益月份一致。",
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
  thesisAdd: "新增論點",
  thesisEdit: "編輯",
  thesisEdited: "已編輯",
  thesisSave: "儲存",
  thesisPlaceholder: "進場理由 / 續抱條件 / 論點失效條件",
  currentThesisNote: "目前記錄的論點——回合平倉後論點可能已改寫",
  thesisFieldPlaceholder: "現在的判斷是什麼",
  thesisSaveFailedNote: "買單已成立，但論點寫入失敗",
  thesisEmptyOpen: "此回合下單時未記錄論點。寫下進場理由、續抱條件與失效條件，之後覆盤才有東西可比對。",
  thesisEmptyClosed: "此回合沒有留下論點紀錄。",
  patTitle: "K 線型態",
  pvChip: "量價",
  pvChipTip: "每根量柱依「價漲跌 × 量增縮」著色",
  maChip: "均線",
  maChipTip: "畫出收盤價的 5／20／60 日簡單移動平均線",
  pvLegend: "量柱",
  pvUpVu: "價漲量增",
  pvUpVd: "價漲量縮",
  pvDnVu: "價跌量增",
  pvDnVd: "價跌量縮",
  pvReadPrice: "價 ",
  pvReadVol: "量 ",
  patHighOnly: "只看高信心",
  patHighTip:
    "信心依三項條件計分：量 > 1.4 倍 20 日均量、實體 > 1.5 倍 20 日平均實體、出現位置符合型態（反轉型要在相反趨勢之後）。三項都符合才算「高」。開啟時只顯示高信心型態；關閉後中、低信心也會以小圓點出現，不帶文字。",
  patEvents: "型態與策略事件",
  patEventsNote: "由新到舊，點一下在圖上定位",
  patNone: "目前篩選下沒有偵測到型態。",
  patPick: "點圖上的型態圓點，或下方事件，查看判斷依據與這檔的歷史表現。",
  patConf: "信心",
  patWhy: "判斷依據",
  patWhyGap: "缺口資料",
  patHist: "這檔過去同型態",
  patHistN: "次數",
  patHistAvg: "5 日後平均",
  patHistHit: "依方向走",
  patHistUp: "5 日後上漲",
  patThisFwd: "這次 5 日後",
  patFwdPending: "尚未滿 5 日",
  patLowSample: "樣本少，僅供參考",
  patDisclaimer: "型態由系統依 K 線形狀判讀，只描述價量現象，不構成買賣建議。",
  patGapOpen: "未回補",
  patGapFilled: "已回補",
  patBars: "%s 根",
  patLegBull: "看漲（K 棒下方）",
  patLegBear: "看跌（K 棒上方）",
  patLegNeu: "中性",
  patLegGap: "缺口（虛線＝未回補）",
  stratChip: "策略",
  stratChipTip: "系統推播過的策略訊號（網 1–5、日週共振）——只列真的發過的，從這功能上線起才有紀錄",
  stratLeg: "策略推播（方塊）",
  stratPushedWatchlist: "%s 的訊號——日報偵測到並已推播（自選股／持倉）",
  stratPushedScan: "%s 的訊號——掃描池偵測到並已推播",
  stratMsgTitle: "推播內容",
  stratVerdictTitle: "當時的 LLM 建議",
  stratVerdictNone: "4 天內 LLM 沒有對這檔給過建議。",
  stratValid_ok: "台股實測為正",
  stratValid_warn: "未通過驗證",
  stratValid_bad: "反指標",
  stratNote_warn: "回測尚未通過驗證門檻，當作觀察提示，不是進場訊號。",
  stratNote_breakout: "四個獨立樣本的超額報酬都是負的。",
  stratNote_mtfTw: "台股唯一實測為正的訊號——但門檻是看過兩個樣本後才選的，仍只當關注清單。",
  stratNote_mtfUs: "美股實測為負，等於反指標。依你的要求保留觀察，才繼續發通知。",
  stratDisclaimer: "標記是真的推播過的策略訊號，從功能上線起才有紀錄。未通過驗證的策略只是觀察提示，不構成買賣建議。",
  stratCode_squeeze: "網1",
  stratName_squeeze: "布林壓縮噴發",
  stratCode_box: "網2",
  stratName_box: "箱型地板抄底",
  stratCode_breakout: "網3",
  stratName_breakout: "趨勢突破",
  stratCode_pullback: "網4",
  stratName_pullback: "趨勢回檔",
  stratCode_trust: "網5",
  stratName_trust: "主力跟單",
  stratCode_mtf: "共振",
  stratName_mtf: "日週共振穿越",
  tkTabPat: "型態",
  condBody: "實體 / 20 日平均實體",
  condVol: "量 / 20 日均量",
  condPrior5: "型態前 5 日",
  condBodyPrev: "實體 / 前一根",
  condCloseInto: "收盤深入前一根實體",
  condLowerShadow: "下影線 / 實體",
  condUpperShadow: "上影線 / 實體",
  condPrior20High: "前 20 日高點",
  condPrior20Low: "前 20 日低點",
  condPriorHighDate: "前高日期",
  condVolVsHigh: "量 / 前高當日量",
  condRange10: "近 10 日振幅",
  condGapRange: "缺口區間",
  condGapSize: "缺口大小",
  condGapStatus: "回補狀態",
  gapStatusOpen: "尚未回補 · 已 %s 個交易日",
  gapStatusFilled: "第 %s 天回補（%s）",
  patCode_bullEngulf: "吞",
  patName_bullEngulf: "長紅吞噬",
  patDef_bullEngulf: "前一根收黑，這根長紅的實體把前一根實體完全包住。出現在下跌段時，常解讀為買盤接手。",
  patCode_bearEngulf: "吞",
  patName_bearEngulf: "長黑吞噬",
  patDef_bearEngulf: "前一根收紅，這根長黑的實體把前一根實體完全包住。出現在上漲段時，常解讀為賣壓湧現。",
  patCode_hammer: "錘",
  patName_hammer: "錘子線",
  patDef_hammer: "下影線至少是實體的 2 倍、上影線很短，出現在下跌段。盤中殺低後被買回。",
  patCode_shooting: "射",
  patName_shooting: "射擊之星",
  patDef_shooting: "上影線至少是實體的 2 倍、下影線很短，出現在上漲段。盤中衝高後被賣回。",
  patCode_morning: "晨",
  patName_morning: "晨星",
  patDef_morning: "長黑 → 小實體 → 長紅，且收盤過第一根實體中點。三根組成的底部反轉。",
  patCode_evening: "夜",
  patName_evening: "夜星",
  patDef_evening: "長紅 → 小實體 → 長黑，且收盤跌破第一根實體中點。三根組成的頭部反轉。",
  patCode_soldiers: "兵",
  patName_soldiers: "紅三兵",
  patDef_soldiers: "連續三根紅 K，每根收盤都比前一根高，實體不小。多方持續推進。",
  patCode_crows: "鴉",
  patName_crows: "三黑鴉",
  patDef_crows: "連續三根黑 K，每根收盤都比前一根低，實體不小。空方持續施壓。",
  patCode_doji: "十",
  patName_doji: "十字星",
  patDef_doji: "開盤與收盤幾乎相同、上下影線明顯。多空暫時平衡，要看下一根確認方向。",
  patCode_volBreak: "突",
  patName_volBreak: "爆量突破",
  patDef_volBreak: "收盤創 20 日新高，成交量超過 20 日均量 1.8 倍。",
  patCode_hangingMan: "吊",
  patName_hangingMan: "吊人線",
  patDef_hangingMan: "和錘子線同樣的形狀（長下影、短上影），但出現在上漲段。盤中曾被大量賣出，多方力道開始鬆動。",
  patCode_invHammer: "倒",
  patName_invHammer: "倒錘線",
  patDef_invHammer: "和射擊之星同樣的形狀（長上影、短下影），但出現在下跌段。買方開始試探向上。",
  patCode_bullHarami: "孕",
  patName_bullHarami: "看漲孕線",
  patDef_bullHarami: "長黑之後出現一根小紅 K，實體完全落在前一根實體之內。跌勢暫停，等待確認。",
  patCode_bearHarami: "孕",
  patName_bearHarami: "看跌孕線",
  patDef_bearHarami: "長紅之後出現一根小黑 K，實體完全落在前一根實體之內。漲勢暫停，等待確認。",
  patCode_piercing: "貫",
  patName_piercing: "貫穿線",
  patDef_piercing: "長黑之後的紅 K 開在前收附近或以下，收盤超過前一根實體中點，但未完全吞噬。長紅吞噬的弱化版。",
  patCode_darkCloud: "烏",
  patName_darkCloud: "烏雲罩頂",
  patDef_darkCloud: "長紅之後的黑 K 開在前收附近或以上，收盤跌破前一根實體中點，但未完全吞噬。長黑吞噬的弱化版。",
  patCode_volDry: "窒",
  patName_volDry: "窒息量",
  patDef_volDry: "成交量低於 20 日均量的一半，且近 10 日在窄幅整理。常見於整理末端，之後容易出現方向選擇。",
  patCode_volDiverge: "背",
  patName_volDiverge: "價量背離",
  patDef_volDiverge: "收盤創 20 日新高，但成交量比前一次高點時少。新高缺少量能支撐。",
  patCode_gapUp: "缺",
  patName_gapUp: "向上跳空",
  patDef_gapUp: "今日最低價高於前一日最高價，中間留下沒有成交的價格區間。回補＝之後價格回到區間內。",
  patCode_gapDown: "缺",
  patName_gapDown: "向下跳空",
  patDef_gapDown: "今日最高價低於前一日最低價，中間留下沒有成交的價格區間。回補＝之後價格回到區間內。",
  patCode_volDump: "破",
  patName_volDump: "爆量跌破",
  patDef_volDump: "收盤創 20 日新低，成交量超過 20 日均量 1.8 倍。",
  patCat_rev: "反轉",
  patCat_cont: "延續",
  patCat_gap: "缺口",
  patCat_vol: "量能",
  patCat_indec: "猶豫",
  patDir_bull: "看漲",
  patDir_bear: "看跌",
  patDir_neu: "中性",
  patConfLabel_high: "高",
  patConfLabel_mid: "中",
  patConfLabel_low: "低",
  patCatTip_rev: "吞噬、貫穿線／烏雲罩頂、孕線、錘子線／吊人線、射擊之星／倒錘線、晨星、夜星",
  patCatTip_cont: "紅三兵、三黑鴉",
  patCatTip_gap: "向上／向下跳空，以價格帶標示；未回補的缺口延伸到今天",
  patCatTip_vol: "爆量突破、爆量跌破、窒息量、價量背離",
  patCatTip_indec: "十字星，出現頻繁，預設關閉",
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
  searchPlaceholder: "搜尋標的…",
  watchlistCount: "檔追蹤中",
  noMatch: "找不到符合的標的。",
  heldOnly: "已持倉",
  nearestSup: "最近支撐",
  nearestRes: "最近壓力",
  ma20: "20 日均線",
  ma60: "60 日均線",
  atr14: "ATR(14)",
  ret20: "20 日報酬",
  fromHigh: "距高點",
  vsAvg20: "對 20 日均量",
  volume: "成交量",
  above: "站上",
  below: "跌破",
  rangeHigh: "區間高",
  rangeLow: "區間低",
  rangeNote: "近 120 個交易日",
  thisPosition: "此標的持倉",
  noPositionHere: "目前無此標的持倉。",
  riskIfStopped: "觸及停損風險",
  pctOfAccount: "佔帳戶",
  tickerRounds: "此標的交易回合",
  noRoundsHere: "此標的尚無已平倉回合。",
  peerTitle: "同產業比價",
  peerRel: "相對本檔(60日)",
  roundPicker: "選擇回合（圖表會縮放到該區間）",
  tradesInRound: "回合內交易記錄",
  allTrades: "全部進出",
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
  notesAddToggle: "新增今日筆記",
  notesEditToggle: "編輯今日筆記",
  notesFieldPlaceholder: "今天在圖上看到了什麼？",
  notesEmptyNote: "還沒有筆記。研究過程中把觀察寫下來——真的要進場時可以直接帶入論點。",
  notesSearchPlaceholder: "搜尋筆記內容或日期（如 2026-05）",
  notesClearSearch: "清除",
  notesPinnedLabel: "置頂",
  notesPinLabel: "置頂",
  notesUnpinLabel: "取消置頂",
  notesDeleteLabel: "刪除",
  notesDeleteConfirm: "確定要刪除這則筆記嗎？此動作無法復原。",
  notesLoadMore: "再顯示 20 筆",
  notesFilterAllLabel: "全部",
  notesTagTechnical: "技術面",
  notesTagFlow: "籌碼面",
  notesTagNews: "消息面",
  notesTagOther: "其他",
  notesTagObservation: "觀察",
  notesTagValuation: "估值",
  notesTagRisk: "風險",
  notesTagCatalyst: "催化",
  notesNote: "自己分析這檔標的時的紀錄，依標的累積，與交易回合無關",
  notesCount: "筆",
  notesMatched: "符合",
  notesToThesis: "帶入回合論點",
  notesShowing: "已顯示",
  notesAllShown: "已到最早一筆",
  notesCollapse: "收合",
  notesNoMatch: "沒有符合的筆記，試試換關鍵字或標籤。",
  notesMonthFmt: "{y} 年 {n} 月",
  tkRoundLabel: "回合",
  tkPickRound: "選擇回合",
  roundOlderTip: "上一個（較早）回合",
  roundNewerTip: "下一個（較新）回合",
  roundHolding: "持有中",
  roundDaysUnit: " 天",
  roundsSummary: "%s 個回合 · 勝 %s / %s · 淨 %s",
  roundSpan: "回合區間",
  tkTabLvl: "支撐壓力",
  tkTabRound: "回合",
  snapTab: "成交快照",
  snapEmpty: "此標的還沒有成交紀錄。每筆買賣成交時會記下當天前後的新聞；指標與 K 線事件在開啟時才依當日收盤計算。",
  snapPrevTip: "較早一筆",
  snapNextTip: "較新一筆",
  snapPickTip: "選擇成交",
  snapRound: "回合 #%s",
  snapFillLine: "%s 股 @ %s",
  snapDayLine: "當日 %s · 收 %s · 高 %s · 低 %s",
  snapDayLineLive: "當日至目前 %s · 現價 %s · 高 %s · 低 %s",
  snapProvenance: "指標以成交日收盤計算 · 每次開啟時重算",
  snapProvenanceLive: "盤中 · 指標以目前價格計算，每次開啟時重算",
  snapIntradayTag: "盤中",
  snapVolPending: "盤中，量比未定",
  snapBarNote: "對應 %s 的交易日",
  snapNoData: "這筆成交沒有可用的 K 線資料（比可取得的價格歷史更早，或之前的歷史不足）。",
  snapLoadFail: "快照載入失敗。",
  snapAligned: "4 項中 %s 項順向",
  snapAgainst: " · %s 項逆向",
  snapReadAsBuy: "以買進方向判讀",
  snapSellNote: "賣出只列數值，不判順逆——賣在強勢或弱勢都可能是對的，所以不打分數。",
  indRsi: "RSI(14)",
  indMacd: "MACD",
  indTrend: "趨勢",
  indVol: "量能",
  rsiHot: "超買",
  rsiCold: "超賣",
  rsiMid: "中性",
  rsiSub: "5 日前 %s %s",
  macdGolden: "金叉 +%s 天",
  macdDeath: "死叉 +%s 天",
  macdSub: "DIF %s · DEA %s · %s",
  macdWiden: "柱體放大",
  macdNarrow: "柱體收斂",
  trendBullVal: "多頭",
  trendBearVal: "空頭",
  trendRangeVal: "盤整",
  trendBullTag: "多頭排列",
  trendBearTag: "空頭排列",
  trendRangeTag: "未成趨勢",
  trendSub: "收盤距 MA20 %s · MA20 5 日斜率 %s",
  volUpTag: "量增",
  volDownTag: "量縮",
  volFlatTag: "持平",
  volSub: "當日 / 20 日均量 · 5 日均 / 20 日 %s",
  snapPatTitle: "當日 K 線事件",
  snapPatNone: "當天沒有偵測到型態。",
  snapNewsTitle: "成交當日新聞",
  snapNewsNote: "標題為成交當下記下 · 標籤是 LLM 只看標題的判讀，僅供參考",
  snapNewsNone: "這筆成交沒有新聞快照（成交當下沒抓到，或是匯入／事後補登的成交）。",
  newsTag_earn: "財報",
  newsTag_guide: "法說",
  newsTag_analyst: "分析師",
  newsTag_sector: "產業",
  newsTag_macro: "總經",
  newsTag_flow: "資金",
  newsTag_other: "其他",
  newsSent_bull: "偏多",
  newsSent_bear: "偏空",
  newsSent_neutral: "中性",
  newsSent_none: "尚未標註",
  newsMajor: "大事件",
  hindTitle: "事後表現",
  hindNote: "成交後才知道 · 不列入上方判讀 · 顏色＝對此筆決策有利／不利",
  hind5: "+5 日",
  hind20: "+20 日",
  hindBest: "20 日內最有利",
  hindWorst: "20 日內最不利",
  hindPending: "未滿",
  snapDataTitle: "快照資料",
  snapDataNote: "成交日收盤的指標與 K 線事件，加上記下的新聞；不含事後表現。平倉後的 AI 回顧會讀到該回合進出場兩天的指標與新聞標題（不含 K 線事件、標籤與事後表現）。",
  snapJsonView: "檢視內容",
  snapJsonHide: "收起",
  snapCopy: "複製",
  snapCopied: "已複製",
  levelsNote: "● 表示已畫在圖上（各取最近三檔）",
  pctOfRange: "區間位置",
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
  wealthSheetLiquid: "流動資產",
  wealthSheetInvestment: "投資資產",
  wealthSheetHard: "實體資產",
  wealthSheetRetirement: "退休資產",
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
  wealthArchiveTitle: "封存「%s」？",
  wealthArchiveBody: "封存後這筆不再計入淨值、配置與比率，所有現值紀錄都會保留。之後可以在資產負債表的「已封存」取消封存。",
  wealthFlashArchived: "已封存「%s」",
  wealthFlashPaused: "已暫停「%s」",
  wealthTabActive: "使用中",
  wealthTabArchived: "已封存",
  wealthArcDesc: "不計入淨值、配置與比率，現值紀錄完整保留。",
  wealthArcColKind: "類別",
  wealthArcColDate: "封存日",
  wealthArcColValue: "封存時現值",
  wealthArcNone: "沒有封存的項目。在任一筆資料的「⋯」選單可以封存。",
  wealthRestore: "取消封存",
  wealthFlashRestored: "已取消封存「%s」",
  wealthRoNoChange: "唯讀模式：設定密碼後才能變更",
  wealthSideAsset: "資產",
  wealthSideLiability: "負債",
  wealthRowMore: "編輯 · 紀錄 · 封存",
  wealthRowLinkGo: "前往交易帳戶",
  wealthRowLinkTitle: "由交易帳戶自動帶入，點擊前往",
  wealthRowEditHint: "點一下直接改（Enter 儲存、Esc 取消）",
  wealthEdTitle: "編輯項目",
  wealthEdTitleArchived: "已封存項目",
  wealthEdLastRecord: "最後紀錄 %s",
  wealthEdNoRecord: "尚無紀錄",
  wealthEdArchNote: "封存於 %s · 不計入淨值、配置與比率。紀錄保留，只能檢視。",
  wealthEdRoNote: "唯讀模式：只能檢視。設定 WEB_PASSWORD 後才能編輯、記錄現值或封存。",
  wealthEdLockSide: "類別",
  wealthEdLockCurrency: "幣別",
  wealthEdLockSource: "來源",
  wealthEdLockNote: "類別、幣別建立後不能更改。要改的話，封存這筆再重新新增。",
  wealthEdInstPhAsset: "例：台灣銀行",
  wealthEdInstPhLiab: "例：國泰世華",
  wealthEdLogTitle: "記錄新現值",
  wealthEdFixTitle: "修正今天的紀錄",
  wealthEdCancelFix: "取消修正",
  wealthEdHintTwd: "填新台幣金額",
  wealthEdHintFx: "填 %s 原幣金額，系統依當日匯率換算新台幣",
  wealthEdAdd: "新增紀錄",
  wealthEdUpdate: "更新這筆",
  wealthEdErrAmount: "請輸入金額",
  wealthEdErrFuture: "日期不能晚於今天",
  wealthEdErrName: "名稱不能空白",
  wealthEdErrLocked: "這天已經有紀錄，過去的紀錄不能再改",
  wealthEdHistTitle: "現值紀錄",
  wealthEdHistEmpty: "還沒有現值紀錄",
  wealthEdTagToday: "今天記錄 · 可修正",
  wealthEdTagHistory: "歷史",
  wealthEdFix: "修正",
  wealthEdDel: "刪除",
  wealthEdHistNote: "當天記的可以修正或刪除；過了當天就成為歷史紀錄，不能再改。記錯了請新增一筆正確的現值。",
  wealthEdSave: "儲存",
  wealthEdClose: "關閉",
  wealthEdArchive: "封存這筆",
  wealthEdFlashSaved: "已儲存",
  wealthEdFlashLogged: "已新增現值紀錄",
  wealthEdFlashFixed: "已修正紀錄",
  wealthEdFlashDeleted: "已刪除紀錄",
  roTag: "唯讀模式",
  roMsg: "伺服器尚未設定 WEB_PASSWORD，目前只能檢視。新增、編輯、封存與匯入都已停用。",
  roHow: "如何開啟編輯 →",
  roHide: "收起",
  roStep1: "在伺服器的環境變數設定密碼",
  roStep2: "重新啟動服務",
  roStep3: "回到這裡，第一次編輯時輸入同一組密碼登入",
  roImportTitle: "匯入需要編輯權限",
  roImportBodyWealth:
    "目前是唯讀模式，不能寫入資料。你仍然可以先下載範本、照上方欄位說明整理好 CSV，設定密碼後再回來貼上匯入。",
  roImportBodyTrade: "目前是唯讀模式，不能匯入交易紀錄。設定 WEB_PASSWORD 並重新啟動後，這裡會出現匯入表單。",
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
  wealthKindCreditCard: "信用卡",
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
  wealthImportInstructions: "貼上或上傳 CSV，先預覽逐列結果，確認沒問題再寫入——第一批帳戶最快的建檔方式。",
  wealthImportHintNote:
    "side、type、name、group、value 必填，其餘可留空；每列至少要有前 8 欄（用逗號補齊）。第一列是標題列，不會被匯入。",
  wealthImportGuide: [
    ["asset", "side：資產"],
    ["liability", "side：負債"],
    ["type", "資產：deposit 存款、fund 基金、bond 債券、estate 不動產、gold 黃金、crypto 加密貨幣、pension 退休帳戶、other 其他。負債：loan 貸款、credit_card 信用卡"],
    ["name", "必填。與現有資料同名同類型會被略過"],
    ["liquid", "group 流動：隨時可動用，如活存、定存"],
    ["growth", "group 成長：股票、ETF、股票型基金"],
    ["income", "group 收益：債券、配息型基金、儲蓄險"],
    ["hard", "group 實體：不動產、黃金、退休帳戶"],
    ["venue", "往來機構或平台，可留空"],
    ["currency", "幣別，如 TWD、USD；留空視為 TWD"],
    ["value", "目前金額（該幣別）；負債填正數"],
    ["date", "估值日 YYYY-MM-DD；留空為今天"],
    ["bank", "存款：銀行"],
    ["accountNote", "存款：帳戶備註"],
    ["lender", "貸款：放款機構"],
    ["ratePct", "貸款：年利率 %，如 2.1"],
    ["originalPrincipal", "貸款：原始本金"],
    ["remainingMonths", "貸款：剩餘月數"],
    ["fundCode", "基金：代碼，如 F00012"],
    ["fundPlatform", "基金：購買平台"],
    ["fundMonthlyAmount", "基金：每月定期定額；一次買入留空"],
    ["fundNextContributionDate", "基金：下次扣款日 YYYY-MM-DD"],
  ],
  wealthImportSample:
    "asset,deposit,台銀活存,liquid,台灣銀行,TWD,350000,,台灣銀行,薪轉帳戶,,,,,,,,\n" +
    "asset,fund,安聯台灣科技基金,growth,基富通,TWD,186000,,,,,,,,F00012,基富通,5000,2026-11-06\n" +
    "asset,gold,黃金存摺,hard,台灣銀行,TWD,128000,,,,,,,,,,,\n" +
    "liability,loan,裝修貸款,hard,台灣銀行,TWD,280000,,,,台灣銀行,2.1,500000,48,,,,",
  wealthImportTemplateName: "argus-資產匯入範本.csv",
  wealthImportDownload: "下載範本",
  wealthImportLoadSample: "載入範例",
  wealthImportClear: "清空",
  wealthImportSummary: "逐列結果",
  wealthImportNoHeader: "第一列看起來是資料，不是標題列——匯入時第一列一律略過。請在最上面加上標題列（按「載入範例」可看格式）。",
  wealthImportDuplicateMsg: "與現有資料同名同類型，已略過",
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
  wealthRateLabel: "利率",
  wealthNetWorthFormula: "淨值 ＝ 總資產 − 總負債",
  wealthStaleDaysSuffix: "%s 天未更新",
  wealthPctOfLiabilities: "佔總負債",
  wealthLiabShortTerm: "短期負債",
  wealthLiabLongTerm: "長期負債",
  wealthDebtRatioNote: "目標 40% 以下",
  wealthLiquidityNote: "流動資產 ÷ 每月支出",
  wealthSavingsRateNote: "（月薪－支出）÷ 月薪",
  wealthExpenseRatioNote: "月支出 ÷ 月薪",
  navWealthCash: "現金流",
  wealthCashMonthlyIn: "月收入",
  wealthCashMonthlyOut: "月支出",
  wealthCashMonthlyNet: "月結餘",
  wealthCashSaveRateLabel: "儲蓄率",
  wealthCashDcaShareLabel: "定期定額佔收入",
  wealthCashFixedShareLabel: "固定支出佔收入",
  wealthCashAnnualNetLabel: "全年累計結餘",
  wealthCashNet90Label: "期間淨額",
  wealthCashInBreakdownTitle: "月收入",
  wealthCashOutBreakdownTitle: "月支出",
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
  wealthCashPauseTitle: "暫停後不計入每月收支與預測，可隨時恢復",
  wealthCashPaused: "已暫停",
  wealthCashPausedNote: "不計入每月收支與預測。恢復後從下個週期開始計算。",
  wealthCashPausedSince: "暫停於 %s",
  wealthCashPerMonth: " / 月",
  wealthCashTagIn: "收入",
  wealthCashTagOut: "支出",
  wealthNoRate: "目前查不到 %s 匯率，無法換算成新台幣",
  wealthCashResume: "恢復",
  wealthCashDelete: "刪除",
  wealthCashDeleteTitle: "刪除「%s」？",
  wealthCashDeleteBody: "刪除後不再出現在現金流與預測。過去已發生的實際收支紀錄不受影響。這個動作無法復原。",
  wealthFlashResumed: "已恢復「%s」",
  wealthFlashDeleted: "已刪除「%s」",
  wealthEmptyNet: "還沒有任何資產或負債。從右上新增第一筆，淨值總覽就會開始長出來。",
  wealthEmptyAlloc: "還沒有資產可以配置。先新增一筆資產，這裡才會出現現況與目標的對照。",
  wealthEmptyBalance: "資產負債表還是空的。從右上新增第一筆資產或負債。",
  wealthEmptyCash: "還沒有任何收支項目。從右上新增一筆現金流。",
  wealthEmptyRetire: "還沒有可以試算的資產。從右上新增第一筆，再回來調整假設。",
  wealthEmptyInsure: "還沒有保單資料。從右上新增一張保單。",
  wealthGuideTitle: "建議的建檔順序",
  wealthGuideDone: "%s / %s 完成",
  wealthGuideHide: "隱藏",
  wealthGuideWhyBalance: "先記下有什麼、欠什麼，淨值才算得出來",
  wealthGuideWhyCash: "每月收支決定能存多少，退休與目標都靠它",
  wealthGuideWhyRetire: "填出生年，退休試算才有起點",
  wealthGuideWhyAlloc: "選保守、平衡或成長型，偏離度才有比較基準",
  wealthGuideWhyInsure: "選填：登錄後才看得到保障缺口",
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
  wealthCashForecastTitle: "12 個月現金流預測",
  wealthCashForecastNote: "以本月結餘為基準推算未來一年，不臆測年繳保費、稅款或年終發放時間。",
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
  wealthRetireTipDeplete: "照目前假設，資產會在幾歲用完。晚於預期壽命才算安全。",
  wealthRetireTipSwr: "退休後每年從資產中領出的比例。4% 是常見的保守基準。",
  wealthRetireChipSpend: "月支出 {v}",
  wealthRetireChipContrib: "月投入 {v}",
  wealthRetireChipReturn: "實質報酬 {pre}% / {post}%",
  wealthRetireChipSwr: "提領率 {v}%",
  wealthRetireChipLifeAge: "活到 {v} 歲",
  wealthRetireSampleTag: "範例",
  wealthRetireSampleMsg: "還沒填你的年齡。以下是範例試算（{age} 歲、{ret} 歲退休、資產池 {pool}），不是你的結果。",
  wealthRetireSampleBtn: "填入我的年齡",

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
  wealthInsureNeedHow: "需求保額是依你的負債、家庭支出與收入推估的建議值，不是保險公司給的數字。滑過「需求保額」可看各項算法。",
  wealthInsureTipCoverage: "現有保額 ÷ 需求保額。100% 代表足額，低於 50% 為明顯不足。",
  wealthInsureTipNeed: "依常見原則估算：壽險＝負債＋10 年家庭支出；意外＝壽險的一半；重大疾病＝3–5 年收入；癌症＝一次療程費用；失能＝月收入的 6 成；住院日額＝每日收入。",
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
  wealthInsureColType: "類型",
  wealthInsureColTerm: "繳費／保障期間",
  wealthInsureNoPolicies: "尚未登錄任何保單。",
  wealthInsureSetupTitle: "設定家庭參數",
  wealthInsureDependentsLabel: "扶養人數",
  wealthInsureYoungestChildAgeLabel: "最小孩子年齡",
  wealthInsureSpouseIncomeLabel: "配偶是否有收入",
  wealthInsureSpouseIncomeYes: "有",
  wealthInsureSpouseIncomeNo: "沒有",
  wealthInsureSetupSave: "儲存",
  wealthInsureNeedPendingNote: "設定下方家庭參數後，才能算出壽險／意外／失能扶助的需求保額。",

  wealthLastImport: "最後匯入",
  wealthImportSourceCsv: "平台對帳單（CSV）",

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
  wealthFundsColCode: "代號",
  wealthFundsColPlatform: "平台",
  wealthFundsColMonthly: "每月扣款",
  wealthFundsColCost: "累積投入",
  wealthFundsColMv: "市值",
  wealthFundsColReturn: "報酬率",
  wealthFundsColOneYear: "近一年",
  wealthFundsColNext: "下次扣款",
  wealthFundsStopped: "已停扣",
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
