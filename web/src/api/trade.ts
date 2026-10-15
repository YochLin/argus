import type { TradeResponse } from "./common";
import { postJSON } from "./http";
import type { NoteTag } from "./rounds";

// --- Phase 10 write endpoints (docs/phase-10-web-trade-input.md) ---
// Mirrors internal/web/trade.go/auth.go's request/response shapes. Every
// write endpoint is same-origin POST + JSON, gated server-side on the
// requireWritable/requireAuth cookie check — fetch's default
// credentials: "same-origin" already sends the session cookie, no extra
// option needed.

export interface TradeRequest {
  ticker: string;
  shares: number;
  price: number;
  fee?: number;
  date?: string;
}

export interface StopRequest {
  ticker: string;
  price: number;
}

export function login(password: string): Promise<{ ok: boolean }> {
  return postJSON("/api/login", { password });
}

export function executeBuy(req: TradeRequest): Promise<TradeResponse> {
  return postJSON("/api/trade/buy", req);
}

export function executeSell(req: TradeRequest): Promise<TradeResponse> {
  return postJSON("/api/trade/sell", req);
}

export function setStopPrice(req: StopRequest): Promise<TradeResponse> {
  return postJSON("/api/stop", req);
}

// deleteTransaction backs TradesTable's per-row delete button (typo
// recovery) — the server only allows deleting a ticker's most recent
// transaction (see internal/db's DeleteTransaction doc comment) and
// otherwise rejects with a message rendered verbatim, same TradeResponse
// convention as executeBuy/executeSell.
export function deleteTransaction(id: number): Promise<TradeResponse> {
  return postJSON("/api/trade/delete", { id });
}

export function addWatchlistTicker(ticker: string): Promise<TradeResponse> {
  return postJSON("/api/watchlist/add", { ticker });
}

export function removeWatchlistTicker(ticker: string): Promise<TradeResponse> {
  return postJSON("/api/watchlist/remove", { ticker });
}

export interface BuyAlertRequest {
  ticker: string;
  price: number;
}

export function addBuyAlert(req: BuyAlertRequest): Promise<TradeResponse> {
  return postJSON("/api/buy-alerts/add", req);
}

export function removeBuyAlert(id: number): Promise<TradeResponse> {
  return postJSON("/api/buy-alerts/remove", { id });
}

// setThesis backs the two thesis-writing entry points: TradeModal's buy-form
// textarea (no roundStart — the server takes the just-opened round) and
// ChartView's round editor, which names the round by its start date.
export function setThesis(ticker: string, text: string, roundStart?: string): Promise<TradeResponse> {
  return postJSON("/api/thesis", { ticker, text, roundStart });
}

// saveResearchNote upserts ticker's research note for today — the server
// overwrites whatever was already recorded today (one row per ticker per
// calendar day), so this doubles as both "add today's note" and "edit
// today's note."
export function saveResearchNote(ticker: string, tag: NoteTag, text: string): Promise<TradeResponse> {
  return postJSON("/api/research-notes", { ticker, tag, text });
}

export function setResearchNotePinned(id: number, pinned: boolean): Promise<TradeResponse> {
  return postJSON("/api/research-notes/pin", { id, pinned });
}

export function deleteResearchNote(id: number): Promise<TradeResponse> {
  return postJSON("/api/research-notes/delete", { id });
}
