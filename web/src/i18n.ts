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
  months: [string, string, string, string, string, string, string, string, string, string, string, string];
}

const en: Dictionary = {
  netPnL: "NET P&L",
  winRate: "WIN RATE",
  profitFactor: "PROFIT FACTOR",
  expectancy: "EXPECTANCY",
  maxDrawdown: "MAX DRAWDOWN",
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
  months: ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"],
};

const zh: Dictionary = {
  netPnL: "NET P&L",
  winRate: "WIN RATE",
  profitFactor: "PROFIT FACTOR",
  expectancy: "EXPECTANCY",
  maxDrawdown: "MAX DRAWDOWN",
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
  buy: "BUY",
  sell: "SELL",
  fee: "手續費",
  realizedPnL: "已實現損益",
  noTransactions: "當天沒有交易紀錄。",
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
  months: ["1月", "2月", "3月", "4月", "5月", "6月", "7月", "8月", "9月", "10月", "11月", "12月"],
};

const dictionaries: Record<string, Dictionary> = { en, zh };

export function getDictionary(lang: string): Dictionary {
  return dictionaries[lang] ?? dictionaries.zh;
}
