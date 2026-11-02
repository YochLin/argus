import type { Market, TradeResponse } from "./common";
import { getJSON, postJSON } from "./http";

// Options mirrors internal/web/options.go's optionsResponse (Phase 12 PR4,
// docs/phase-12-options.md). US-only — a TW-market fetch returns all-empty
// arrays, same convention as buildOptionCollateral. Mark/greeks are 0 on a
// position whose live chain fetch failed (degraded row, not dropped).
export interface OptionPosition {
  contractSymbol: string;
  underlying: string;
  right: string; // "C" | "P"
  strike: number;
  expiry: string;
  dte: number;
  multiplier: number;
  contracts: number; // signed: + long, - short
  avgPremium: number;
  stopPremium: number;
  edited: boolean;
  mark: number;
  marketValue: number;
  delta: number;
  gamma: number;
  theta: number;
  vega: number;
}

export interface OptionClosedTrade {
  id: number;
  contractSymbol: string;
  underlying: string;
  right: string;
  strike: number;
  expiry: string;
  action: string; // STC | BTC | EXPIRED | ASSIGNED | EXERCISED
  contracts: number;
  premium: number;
  fee: number;
  date: string;
  realizedPnL: number;
  edited: boolean;
}

export interface OptionCalendarEntry {
  date: string;
  symbols: string[];
}

export interface OptionCollateralPosition {
  underlying: string;
  lockedShares: number;
  heldShares: number;
  naked: boolean;
}

export interface Options {
  positions: OptionPosition[];
  closed: OptionClosedTrade[];
  calendar: OptionCalendarEntry[];
  collateral: {
    lockedCash: number;
    positions: OptionCollateralPosition[];
  };
  held: Record<string, number>; // shares held per ticker
}

export function fetchOptions(market: Market = "us"): Promise<Options> {
  return getJSON<Options>(`/api/options?market=${market}`);
}

// OptionOpenRequest/OptionCloseRequest mirror internal/web/options.go's
// optionOpenRequest/optionCloseRequest — the options page's write half
// (OptionsView's Add/Close modals). action is one of the
// OPTION_CLOSE_ACTIONS below; a resolution action (EXPIRED/ASSIGNED/
// EXERCISED) ignores contracts/premium/fee server-side (resolveOption
// always closes the whole remaining position at premium 0).
export interface OptionOpenRequest {
  symbol: string;
  side: "BUY" | "SELL";
  contracts: number;
  premium: number;
  fee?: number;
  date?: string;
}

export interface OptionCloseRequest {
  symbol: string;
  action: string;
  contracts?: number;
  premium?: number;
  fee?: number;
  date?: string;
}

export function executeOptionOpen(req: OptionOpenRequest): Promise<TradeResponse> {
  return postJSON("/api/options/open", req);
}

export function executeOptionClose(req: OptionCloseRequest): Promise<TradeResponse> {
  return postJSON("/api/options/close", req);
}

// The corrections: an open position typed wrong, and a closed record.
export interface OptionEditRequest {
  symbol: string;
  newSymbol: string;
  side: "BUY" | "SELL";
  contracts: number;
  premium: number;
}

export interface OptionClosedRef {
  id: number;
  symbol: string;
}

export interface OptionClosedEditRequest extends OptionClosedRef {
  action: string; // CLOSED | EXPIRED, or the row's own ASSIGNED | EXERCISED
  contracts: number;
  date: string;
  realizedPnL: number;
}

export function editOptionPosition(req: OptionEditRequest): Promise<TradeResponse> {
  return postJSON("/api/options/edit", req);
}

export function deleteOptionPosition(symbol: string): Promise<TradeResponse> {
  return postJSON("/api/options/delete", { symbol });
}

export function editOptionClosed(req: OptionClosedEditRequest): Promise<TradeResponse> {
  return postJSON("/api/options/closed/edit", req);
}

export function deleteOptionClosed(ref: OptionClosedRef): Promise<TradeResponse> {
  return postJSON("/api/options/closed/delete", ref);
}

export function undoOptionClose(ref: OptionClosedRef): Promise<TradeResponse> {
  return postJSON("/api/options/closed/undo", ref);
}
