import type { Market } from "./common";
import { getJSON } from "./http";

// RiskPosition mirrors internal/web/risk.go's riskPositionResponse
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

// Risk mirrors internal/web/risk.go's riskResponse. heatThresholdPct
// <= 0 means the server's RISK_HEAT_PCT is disabled — don't draw a warning
// line or recolor the heat KPI.
export interface Risk {
  accountValue: number;
  cash: number;
  heatPct: number;
  heatThresholdPct: number;
  positions: RiskPosition[];
}

export function fetchRisk(market: Market = "us"): Promise<Risk> {
  return getJSON<Risk>(`/api/risk?market=${market}`);
}
