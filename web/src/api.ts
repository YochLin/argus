// Mirrors internal/web/handlers.go's JSON response shapes exactly — the Go
// side sends only raw numbers/dates/tickers (no display strings), so this
// file and internal/web/handlers.go must be kept in sync by hand the same
// way any other cross-language API contract in this project is (there's
// no shared schema generator here).

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
  status: Status;
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

export function fetchDashboard(): Promise<Dashboard> {
  return getJSON<Dashboard>("/api/dashboard");
}

export function fetchCalendar(month: string): Promise<Calendar> {
  return getJSON<Calendar>(`/api/calendar?month=${encodeURIComponent(month)}`);
}
