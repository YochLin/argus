import type { TradeResponse } from "./common";
import { getJSON, postJSON } from "./http";

// CashflowDirection/CashflowItem/CashEvent mirror wealth_cash.go's
// recurring_cashflows response shapes (Phase 9 波次2 PR5, §8.4/§9.4) — a
// hand-maintained monthly amount, not a transaction ledger.
export type CashflowDirection = "in" | "out";

export interface CashflowItem {
  id: number;
  direction: CashflowDirection;
  name: string;
  amount: number;
  currency: string;
  dayOfMonth?: number;
  category?: string;
  assetId?: number;
  venue?: string;
  active: boolean;
  // pausedAt is the day a paused flow was paused — absent while active, and
  // for one paused before the server started recording it.
  pausedAt?: string;
  // valueTwd is amount converted to TWD as of today — absent only when the
  // currency couldn't be priced (a paused item has one too). Backs the in/out
  // breakdown's per-item bar and every amount the page shows.
  valueTwd?: number;
}

export interface CashEvent {
  date: string;
  item: string;
  venue?: string;
  amount: number | null;
  currency?: string;
  // valueTwd is the amount's TWD magnitude (direction carries the sign) —
  // absent for an event with no amount or an unpriceable currency.
  valueTwd?: number;
  direction: CashflowDirection | "event";
}

// CashForecastMonth is one bar of the 12-month forecast chart — net is
// monthlyNet projected flat (no calendar-lump fabrication, see
// wealth_cash.go's buildCashForecast doc comment).
export interface CashForecastMonth {
  month: string; // "2026-01"
  net: number;
}

// WealthCash mirrors wealth_cash.go's cashResponse — monthlyIn/Out/Net are
// null when an active flow's currency couldn't be priced to TWD today
// (§8.17.1's "don't fabricate a number" rule, same as /w/alloc's totals).
export interface WealthCash {
  asOf: string;
  items: CashflowItem[];
  monthlyIn: number | null;
  monthlyOut: number | null;
  monthlyNet: number | null;
  // saveRatePct/dcaSharePct/fixedSharePct are all percentages of monthlyIn,
  // null together with it when FX can't be resolved.
  saveRatePct: number | null;
  dcaSharePct: number | null;
  fixedSharePct: number | null;
  // annualNet is a flat monthlyNet*12 projection — see wealth_cash.go's
  // doc comment for why it doesn't attempt the template's one-off
  // Taiwan-calendar lumps (annual premium, tax season, etc).
  annualNet: number | null;
  eventsNet: number | null;
  events: CashEvent[];
  forecast: CashForecastMonth[];
}

export function fetchWealthCash(): Promise<WealthCash> {
  return getJSON("/api/wealth/cash");
}

export interface NewCashflow {
  direction: CashflowDirection;
  name: string;
  amount: number;
  currency?: string;
  dayOfMonth?: number;
  category?: string;
  assetId?: number;
}

export function createWealthCashflow(c: NewCashflow): Promise<{ id: number }> {
  return postJSON("/api/wealth/cash", c);
}

export function deactivateWealthCashflow(id: number): Promise<TradeResponse> {
  return postJSON("/api/wealth/cash/deactivate", { id });
}

export function resumeWealthCashflow(id: number): Promise<TradeResponse> {
  return postJSON("/api/wealth/cash/resume", { id });
}

// Permanent, and only for a paused flow — the server answers 409 otherwise.
export function deleteWealthCashflow(id: number): Promise<TradeResponse> {
  return postJSON("/api/wealth/cash/delete", { id });
}
