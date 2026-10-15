import type { Market } from "./common";
import { getJSON } from "./http";

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
  winRate: number;
  profitFactor: number;
  expectancy: number;
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
  source: string;
  entryPrice: number;
  excessReturnPct: number;
}

// collapsed + scorable + unscorable + holdKept === total (see
// internal/web/recperf.go's recPerfCounts). `hold` is the pre-collapse raw
// HOLD count and is not part of that sum.
export interface RecPerfCounts {
  total: number;
  hold: number;
  holdKept: number;
  scorable: number;
  unscorable: number;
  collapsed: number;
}

// RecPerfActiveSignal mirrors internal/web/recperf.go's recPerfActiveSignal
// — one still-open (unmatured shortest-horizon window) BUY/SELL rec, with
// excessReturnPct recomputed entry-to-today rather than entry-to-a-fixed
// horizon.
export interface RecPerfActiveSignal {
  ticker: string;
  action: string;
  source: string;
  entryDate: string;
  entryPrice: number;
  daysHeld: number;
  excessReturnPct: number;
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
  overall: RecPerfStatsCell[];
  evalHorizon: number;
  actedVsSkipped: RecPerfGroup[];
  activeSignals: RecPerfActiveSignal[];
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

export function fetchReports(market: Market = "us"): Promise<Reports> {
  return getJSON<Reports>(`/api/reports?market=${market}`);
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
