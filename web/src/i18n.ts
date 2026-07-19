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
};

const dictionaries: Record<string, Dictionary> = { en, zh };

export function getDictionary(lang: string): Dictionary {
  return dictionaries[lang] ?? dictionaries.zh;
}
