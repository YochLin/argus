import type { Market } from "./common";
import { getJSON } from "./http";

export interface Tickers {
  tickers: string[];
}

export interface WatchlistSummaryItem {
  ticker: string;
  price: number;
  changePct: number;
  sparkline: number[];
  support: number | null;
  resistance: number | null;
  heldShares: number;
}

export interface WatchlistSummary {
  tickers: WatchlistSummaryItem[];
}

// CompanyNames mirrors internal/web/companynames.go's companyNamesResponse —
// TW ticker → Chinese short name ("2330" → "台積電"). Empty (never null)
// when FINMIND_TOKEN isn't configured server-side.
export interface CompanyNames {
  names: Record<string, string>;
}

// tickerLabel mirrors internal/data.TickerLabel's display format
// ("台積電(2330)"): a ticker with a resolved name renders as name(ticker),
// anything else — every US ticker, and a TW ticker whose lookup failed —
// stays the bare ticker.
export function tickerLabel(ticker: string, names: Record<string, string>): string {
  const name = names[ticker];
  return name ? `${name}(${ticker})` : ticker;
}

export function fetchTickers(market: Market = "us"): Promise<Tickers> {
  return getJSON<Tickers>(`/api/tickers?market=${market}`);
}

export function fetchWatchlistSummary(market: Market = "us"): Promise<WatchlistSummary> {
  return getJSON<WatchlistSummary>(`/api/watchlist-summary?market=${market}`);
}

export function fetchCompanyNames(): Promise<CompanyNames> {
  return getJSON<CompanyNames>("/api/company-names");
}
