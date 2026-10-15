import type { DateValue, Market } from "./common";
import { getJSON } from "./http";
import type { Transaction } from "./rounds";

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
  // stopPrice (Phase 10) mirrors db.Position.StopPrice — 0 means unset,
  // same sentinel convention as RiskPosition.stopPrice.
  stopPrice: number;
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
  // writable (Phase 10) mirrors whether WEB_PASSWORD is configured
  // server-side — the frontend renders no trade-input UI at all (not just
  // disabled) when this is false. See internal/web/auth.go.
  writable: boolean;
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
  // paperEnabled (Phase 11 PR4) mirrors whether PAPER_DB_PATH is configured
  // server-side — the sidebar's Paper Account link is hidden entirely (not
  // just disabled) when this is false, same convention as Status.writable
  // gating the trade UI.
  paperEnabled: boolean;
  // llmAuditEnabled (Phase 19, WEB_LLM_AUDIT) gates the /llm nav link the
  // same way — hidden entirely when the backend never registered those
  // routes, rather than shown-but-404ing.
  llmAuditEnabled: boolean;
}

// CalendarEvent is a single earnings-date marker (Calendar's scope is
// earnings-only, see internal/web/calendar.go's calendarEvent). Note text is
// deliberately not sent — the frontend builds display copy from
// kind/hour/estimated via i18n/, same "backend never sends display
// strings" rule every other endpoint here follows.
export interface CalendarEvent {
  date: string;
  ticker: string;
  kind: string; // only "earnings" today, kept generic for future event types
  hour: string; // "bmo" | "amc" | "dmh" | "" — Finnhub-only, always "" for TW's estimated proxy
  estimated: boolean; // true for TW's statutory-deadline proxy
  held: boolean; // ticker is a currently open position
}

export interface Calendar {
  month: string; // YYYY-MM
  days: DateValue[];
  transactions: Transaction[];
  events: CalendarEvent[];
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
