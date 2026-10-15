import type { DateValue, Market } from "./common";
import { getJSON } from "./http";

// Paper mirrors internal/web/paper.go's paperResponse (Phase 11 PR4,
// docs/phase-11-paper-account.md §7.1) — the live paper account's own KPIs/
// positions/closed-rounds/curves. curve/benchmark are equity-level (not
// cumulative-P&L-from-zero like Dashboard.curve/benchmark) — they start
// near kpis.initialCash, so they read naturally on their own axis without
// needing that baseline threaded through separately.
export interface PaperKPIs {
  cash: number;
  positionsValue: number;
  equity: number;
  initialCash: number;
  sinceDate: string; // "" before the account's first trade
  realizedPnL: number;
  unrealizedPnL: number;
  totalReturnPct: number;
  benchmarkReturnPct: number;
  alphaPct: number;
  maxDrawdownPct: number;
  winRate: number;
  profitFactor: number;
  expectancy: number;
}

export interface PaperPosition {
  ticker: string;
  entryDate: string;
  avgCost: number;
  shares: number;
  stopPrice: number;
  price: number;
  unrealizedPnL: number;
  unrealizedPct: number;
  distToStopPct: number;
}

// PaperClosedPosition.exitReason is "stop" | "llm_sell" — see
// internal/web/paper.go's closedPositionFromRound for how it's derived
// (zero schema changes: the SELL leg's own stop_price snapshot).
export interface PaperClosedPosition {
  ticker: string;
  entryDate: string;
  exitDate: string;
  avgCost: number;
  exitPrice: number;
  shares: number;
  realizedPnL: number;
  realizedPct: number;
  exitReason: string;
}

export interface Paper {
  kpis: PaperKPIs;
  positions: PaperPosition[];
  closed: PaperClosedPosition[];
  curve: DateValue[];
  benchmark: DateValue[];
  drawdown: DateValue[];
}

export function fetchPaper(market: Market = "us"): Promise<Paper> {
  return getJSON<Paper>(`/api/paper?market=${market}`);
}
