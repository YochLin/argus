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
}

export interface Dashboard {
  kpis: KPIs;
  curve: DateValue[];
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

export interface RoundDetail {
  ticker: string;
  start: string;
  end: string; // "" while still open
  candles: Candle[];
  trades: Transaction[];
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
