// Market mirrors internal/market.MarketID (Phase 6, see
// docs/phase-6-tw-market.md §4.4) — "us" is every endpoint's default on the
// Go side when the query param is absent, so every fetch* function in
// this directory defaults to it too.
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

export interface Candle {
  date: string;
  open: number;
  high: number;
  low: number;
  close: number;
  volume: number;
}

// TradeResponse's message is the exact Telegram-confirmation-text the
// server would have sent for the equivalent /buy, /sell, or /stop command
// (see docs/phase-10-web-trade-input.md §4.2) — render it verbatim, no
// frontend-side translation needed.
export interface TradeResponse {
  message: string;
}
