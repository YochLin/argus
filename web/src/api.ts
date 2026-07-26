// Mirrors internal/web/handlers.go's JSON response shapes exactly — the Go
// side sends only raw numbers/dates/tickers (no display strings), so this
// file and internal/web/handlers.go must be kept in sync by hand the same
// way any other cross-language API contract in this project is (there's
// no shared schema generator here).

// Market mirrors internal/market.MarketID (Phase 6, see
// docs/phase-6-tw-market.md §4.4) — "us" is every endpoint's default on the
// Go side when the query param is absent, so every fetch* function below
// defaults to it too.
export type Market = "us" | "tw";

// marketOf mirrors internal/market.Of: a leading digit means a TW ticker.
// The one client-side reader of this (RoundDetailView) needs it because a
// round's own currency comes from the ticker it's displaying, not from the
// page-level Market toggle — a round detail page is reached by ticker, and
// its ticker alone determines which currency symbol is correct regardless
// of which toggle position led there.
export function marketOf(ticker: string): Market {
  return /^[0-9]/.test(ticker) ? "tw" : "us";
}

// currencySymbol is the display-side counterpart of internal/bot's
// KeyPortfolioSectionTW/KeyPortfolioSummaryTWD convention: TWD amounts get
// "NT$", everything else keeps the existing "$".
export function currencySymbol(market: Market): string {
  return market === "tw" ? "NT$" : "$";
}

export interface DateValue {
  date: string;
  value: number;
}

export interface KPIs {
  netPnL: number;
  winRate: number;
  profitFactor: number;
  expectancy: number;
  maxDrawdown: number;
  // ytd/qtd/htdReturnPct are already-scaled percentages (12.3 means 12.3%,
  // same convention as Position.unrealizedPnLPct/ReportGroup.avgReturnPct —
  // not a 0-1 fraction like winRate). null means "no net worth baseline yet"
  // (fresh account, or history postdates the period) — render that as "—",
  // never as 0%.
  ytdReturnPct: number | null;
  qtdReturnPct: number | null;
  htdReturnPct: number | null;
  // benchmarkAlpha is curve's terminal value minus benchmark's terminal
  // value (Phase 8 PR2) — null when there's no benchmark curve in range yet.
  benchmarkAlpha: number | null;
}

export interface Position {
  ticker: string;
  shares: number;
  avgCost: number;
  price: number;
  marketValue: number;
  unrealizedPnL: number;
  unrealizedPnLPct: number;
}

export interface Status {
  watchingCount: number;
  spyChangePct: number;
  lastCloseDate: string;
  // accountValue/netPnL/winRate/tradeCount back the sidebar's
  // account-overview card — see internal/web/dashboard.go's buildStatus for
  // each field's exact definition.
  accountValue: number;
  netPnL: number;
  winRate: number;
  tradeCount: number;
}

export interface Dashboard {
  kpis: KPIs;
  curve: DateValue[];
  // drawdown is curve's underwater series (Phase 8 PR2, always <= 0), same
  // date axis as curve.
  drawdown: DateValue[];
  // benchmark is the same-cash-flow SPY/0050 replay (Phase 8 PR2), in the
  // same cumulative-P&L-amount units as curve — its date axis may start
  // later than curve's own (see docs/phase-8-trader-analytics.md §7).
  benchmark: DateValue[];
  positions: Position[];
}

export interface Config {
  lang: string;
}

export interface Transaction {
  date: string;
  ticker: string;
  side: string; // "BUY" | "SELL"
  shares: number;
  price: number;
  fee: number;
  realizedPnL: number;
}

export interface Calendar {
  month: string; // YYYY-MM
  days: DateValue[];
  transactions: Transaction[];
}

export interface RoundSummary {
  ticker: string;
  start: string;
  end: string; // "" while still open
  open: boolean;
  shares: number;
  realizedPnL: number;
}

export interface Rounds {
  rounds: RoundSummary[];
}

export interface Candle {
  date: string;
  open: number;
  high: number;
  low: number;
  close: number;
  volume: number;
}

export interface Lesson {
  date: string;
  lesson: string;
}

export interface RoundDetail {
  ticker: string;
  start: string;
  end: string; // "" while still open
  candles: Candle[];
  trades: Transaction[];
  maePct: number;
  mfePct: number;
  hasMaeMfe: boolean;
  // thesis/lessons (Phase 8 PR4) are read-only attachments. thesis is the
  // *current* thesis on record (db.GetThesis), not necessarily what it was
  // when this round was open — there's no thesis history table. null means
  // neither exists nor loaded successfully; the frontend renders no block
  // either way.
  thesis: string | null;
  lessons: Lesson[];
}

export interface ChartLevel {
  price: number;
  touches: number;
  firstDate: string;
  lastDate: string;
}

export interface Chart {
  ticker: string;
  candles: Candle[];
  levels: ChartLevel[];
}

export interface Tickers {
  tickers: string[];
}

// RiskPosition mirrors internal/web/handlers.go's riskPositionResponse
// (Phase 8 PR1) — a live-priced view of one open position's stop-loss risk.
// openRisk/openRiskPct are null when the position has no stop price set
// (never 0 — 0 would read as "no risk," the opposite of the truth); see
// internal/web/risk.go's buildRisk doc comment.
export interface RiskPosition {
  ticker: string;
  shares: number;
  avgCost: number;
  price: number;
  value: number;
  weightPct: number;
  stopPrice: number; // 0 = unset
  openRisk: number | null;
  openRiskPct: number | null;
  unrealizedPnLPct: number;
}

// Risk mirrors internal/web/handlers.go's riskResponse. heatThresholdPct
// <= 0 means the server's RISK_HEAT_PCT is disabled — don't draw a warning
// line or recolor the heat KPI.
export interface Risk {
  accountValue: number;
  cash: number;
  heatPct: number;
  heatThresholdPct: number;
  positions: RiskPosition[];
}

// CompanyNames mirrors internal/web/handlers.go's companyNamesResponse —
// TW ticker → Chinese short name ("2330" → "台積電"). Empty (never null)
// when FINMIND_TOKEN isn't configured server-side.
export interface CompanyNames {
  names: Record<string, string>;
}

// tickerLabel mirrors internal/data.TickerLabel's display format
// ("台積電(2330)"): a ticker with a resolved name renders as name(ticker),
// anything else — every US ticker, and a TW ticker whose lookup failed —
// stays the bare ticker.
export function tickerLabel(ticker: string, names: Record<string, string>): string {
  const name = names[ticker];
  return name ? `${name}(${ticker})` : ticker;
}

// ReportGroup mirrors internal/web/reports.go's ReportGroup — one row of a
// grouped performance report (Phase 5 PR4, docs/phase-5-web-dashboard.md
// §A1). winRate is a fraction 0-1 (same convention as KPIs.winRate);
// avgReturnPct is already scaled (12.3 means 12.3%), same convention as
// Position.unrealizedPnLPct.
export interface ReportGroup {
  key: string;
  n: number;
  winRate: number;
  profitFactor: number;
  avgReturnPct: number;
  totalRealizedPnL: number;
  avgHoldingDays: number;
  lowSample: boolean;
}

export interface FeeSummary {
  totalFees: number;
  pctOfRealizedPnL: number;
}

export interface StreakStats {
  bestTradePnL: number;
  worstTradePnL: number;
  avgWinPnL: number;
  avgLossPnL: number;
  longestWinStreak: number;
  longestLossStreak: number;
}

// MAEMFESummary mirrors internal/web/maefe.go's MAEMFESummary — the report
// page's "on average, how much of the available run-up did exits actually
// capture" aggregate over every closed round (§A2). avgCapturedPct is
// realized return over MFE, already scaled (100 = captured all of it).
export interface MAEMFESummary {
  avgCapturedPct: number;
  n: number;
  lowSample: boolean;
}

export interface Reports {
  byTicker: ReportGroup[];
  byHoldingDays: ReportGroup[];
  byEntryMonth: ReportGroup[];
  byEntryWeekday: ReportGroup[];
  fees: FeeSummary;
  streaks: StreakStats;
  maeMfe: MAEMFESummary;
}

// YearlyPnL mirrors internal/web/monthly.go's YearlyPnL (Phase 8 PR3) — one
// row of the Reports page's monthly P&L grid. months[i] is null for a month
// with no data day at all (not 0) — render as an empty cell, not "$0".
export interface YearlyPnL {
  year: number;
  months: (number | null)[]; // index 0 = January ... 11 = December
  total: number;
}

export interface Monthly {
  years: YearlyPnL[];
}

// RecPerfStatsCell/RecPerfGroup mirror internal/web/recperf.go's
// recPerfStatsCell/recPerfGroup (Phase 8 PR3) — hitRatePct/avgReturnPct/
// avgExcessPct are already-scaled percentages, same convention as
// ReportGroup's own fields.
export interface RecPerfStatsCell {
  horizon: number;
  n: number;
  hitRatePct: number;
  avgReturnPct: number;
  avgExcessPct: number;
  lowSample: boolean;
}

export interface RecPerfGroup {
  key: string;
  cells: RecPerfStatsCell[];
}

export interface RecPerfExtreme {
  ticker: string;
  date: string;
  action: string;
  entryPrice: number;
  excessReturnPct: number;
}

export interface RecPerfCounts {
  total: number;
  hold: number;
  scorable: number;
  unscorable: number;
}

// RecPerformance mirrors internal/web/recperf.go's recPerformanceResponse —
// /recs page's data, scored via the same internal/receval functions `argus
// eval` uses, so the two reports agree exactly off the same database.
export interface RecPerformance {
  counts: RecPerfCounts;
  horizons: number[];
  bySource: RecPerfGroup[];
  byAction: RecPerfGroup[];
  best: RecPerfExtreme[];
  worst: RecPerfExtreme[];
}
// RMultipleSample/HoldingReturnSample/MAEReturnSample/Distributions mirror
// internal/web/distributions.go's response types (Phase 8 PR4) — three
// independent per-closed-round datasets (one sample per round, not per
// SELL leg — see that file's doc comment on why the unit differs from
// ReportGroup's).
export interface RMultipleSample {
  ticker: string;
  r: number;
}

export interface HoldingReturnSample {
  ticker: string;
  holdingDays: number;
  realizedPnL: number;
}

export interface MAEReturnSample {
  ticker: string;
  maePct: number;
  returnPct: number;
}

export interface Distributions {
  rMultiples: RMultipleSample[];
  noStopCount: number;
  earliestRDate: string; // "" when no round has a computable R yet
  holdingReturns: HoldingReturnSample[];
  maeReturns: MAEReturnSample[];
  skippedMaeCount: number;
}

async function getJSON<T>(url: string): Promise<T> {
  const res = await fetch(url);
  if (!res.ok) {
    throw new Error(`${url}: HTTP ${res.status}`);
  }
  return res.json() as Promise<T>;
}

export function fetchConfig(): Promise<Config> {
  return getJSON<Config>("/api/config");
}

export function fetchDashboard(market: Market = "us"): Promise<Dashboard> {
  return getJSON<Dashboard>(`/api/dashboard?market=${market}`);
}

export function fetchStatus(market: Market = "us"): Promise<Status> {
  return getJSON<Status>(`/api/status?market=${market}`);
}

export function fetchCalendar(month: string, market: Market = "us"): Promise<Calendar> {
  return getJSON<Calendar>(`/api/calendar?month=${encodeURIComponent(month)}&market=${market}`);
}

export function fetchRounds(market: Market = "us"): Promise<Rounds> {
  return getJSON<Rounds>(`/api/rounds?market=${market}`);
}

export function fetchRoundDetail(ticker: string, start: string): Promise<RoundDetail> {
  return getJSON<RoundDetail>(
    `/api/round-detail?ticker=${encodeURIComponent(ticker)}&start=${encodeURIComponent(start)}`,
  );
}

export function fetchChart(ticker: string): Promise<Chart> {
  return getJSON<Chart>(`/api/chart?ticker=${encodeURIComponent(ticker)}`);
}

export function fetchTickers(market: Market = "us"): Promise<Tickers> {
  return getJSON<Tickers>(`/api/tickers?market=${market}`);
}

export function fetchReports(market: Market = "us"): Promise<Reports> {
  return getJSON<Reports>(`/api/reports?market=${market}`);
}

export function fetchCompanyNames(): Promise<CompanyNames> {
  return getJSON<CompanyNames>("/api/company-names");
}

export function fetchRisk(market: Market = "us"): Promise<Risk> {
  return getJSON<Risk>(`/api/risk?market=${market}`);
}

export function fetchMonthly(market: Market = "us"): Promise<Monthly> {
  return getJSON<Monthly>(`/api/monthly?market=${market}`);
}

export function fetchRecPerformance(market: Market = "us"): Promise<RecPerformance> {
  return getJSON<RecPerformance>(`/api/rec-performance?market=${market}`);
}
export function fetchDistributions(market: Market = "us"): Promise<Distributions> {
  return getJSON<Distributions>(`/api/distributions?market=${market}`);
}
