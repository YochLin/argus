import type { Candle } from "./common";
import { getJSON } from "./http";
import type { RiskPosition } from "./risk";
import type { RoundSummary, Transaction } from "./rounds";

export interface ChartLevel {
  price: number;
  touches: number;
  firstDate: string;
  lastDate: string;
}

// PatternHit is one candlestick/volume/gap pattern from /api/chart (Phase 27
// P4b). Numbers only — names, definitions and the "why" rows come from the
// frontend dictionary keyed by type. extra's meaning depends on type (see
// signals.Pattern in Go): engulfing/harami body÷previous body, piercing/dark
// cloud % of the previous body reached, hammer/hanging man lower shadow÷body,
// shooting star/inverted hammer upper shadow÷body, breakout/breakdown the prior
// 20-day high/low, divergence this volume÷the prior high's volume, dry-up the
// 10-day range %.
export interface PatternHit {
  type: string;
  cat: "rev" | "cont" | "indec" | "vol" | "gap";
  dir: "bull" | "bear" | "neu";
  start: string;
  end: string;
  conf: "high" | "mid" | "low" | ""; // "" for gaps
  bodyRatio: number;
  volRatio: number;
  prior5Pct: number;
  extra: number;
  refDate?: string;
  gap?: { lo: number; hi: number; fillDate: string }; // fillDate "" while open
  fwd5: number | null;
}

// Fill is one buy/sell of the viewed ticker, with the round it belongs to
// (roundStart matches a RoundSummary.start) — the snapshot tab's picker.
export interface Fill extends Transaction {
  roundStart: string;
}

export interface Chart {
  ticker: string;
  candles: Candle[];
  levels: ChartLevel[];
  patterns: PatternHit[];
  position: RiskPosition | null;
  rounds: RoundSummary[];
  fills: Fill[]; // oldest first
  strategies?: StrategyAlert[]; // oldest first; absent from a pre-P5 backend
  drops?: DropEvent[]; // oldest first; absent from an older backend
  benchmark?: string; // the index each drop's indexPct is measured on: ^GSPC or ^TWII
}

// DropEvent is one 下跌事件 from /api/chart: a run of lower closes or a gap
// down, computed from the candles. cause down to news were recorded when a
// watchlist drop was first seen; older drops and other tickers have none.
// summary falls back to the Telegram price event's writeup, "" if neither.
export interface DropEvent {
  kind: "streak" | "gap";
  start: string;
  end: string;
  pct: number;
  gapPct: number | null;
  preClose: number;
  low: number;
  recoverBars: number | null;
  fwd10Pct: number | null;
  indexPct: number | null;
  summary: string;
  cause: DropCause | "";
  secondary: DropCause[];
  confidence: "high" | "mid" | "";
  loggedOn: string; // the bar the cause was recorded on, "" if none was
  news: { headline: string; source: string; url: string; publishedAt: string }[];
}

export type DropCause = "earn" | "macro" | "sector" | "flow" | "corp" | "tech";

// StrategyAlert is one strategy signal that was pushed to the user (Phase 27
// P5). type is the signal type ("strategy_squeeze_breakout", ...), date the
// candle it was computed on, message the text that was sent. verdict is the
// LLM's call on that ticker around that date, null when it never weighed in.
export interface StrategyAlert {
  type: string;
  date: string;
  channel: "watchlist" | "scan" | "momentum";
  message: string;
  verdict: { date: string; action: "BUY" | "SELL" | "HOLD"; reason: string } | null;
}

// FillSnapshot is /api/fill-snapshot's body (Phase 27 P6): how the stock looked
// on the day of a fill. Numbers and codes only. snapshot is null when no candle
// maps to the fill (too old, or too little history before it); news is
// independent of that. hindsight's moves are plain price changes from the fill
// price, null where the candles don't reach yet.
export interface FillSnapshotBody {
  date: string; // the candle the fill maps to
  close: number;
  high: number;
  low: number;
  dayChangePct: number;
  rsi14: number;
  rsi14Prev5: number;
  rsiZone: "hot" | "cold" | "mid";
  macdDif: number;
  macdDea: number;
  macdHist: number;
  macdCrossDays: number;
  macdWidening: boolean;
  trend: "bull" | "bear" | "range";
  closeVsMa20Pct: number;
  ma20Slope5dPct: number;
  volRatio20: number;
  volRatio5v20: number;
  volState: "up" | "down" | "flat";
  patterns: { type: string; dir: "bull" | "bear" | "neu" }[];
  // The bar is a session still in progress: the price-based readings are as of
  // now and the volume ratios are of a day not yet traded out.
  provisional: boolean;
}

export interface FillHindsight {
  fwd5Pct: number | null;
  fwd20Pct: number | null;
  maxUpPct: number | null;
  maxDownPct: number | null;
}

export interface FillNews {
  headline: string;
  source: string;
  url: string;
  publishedAt: string; // RFC3339, "" when the source gave none
  sentiment: "bull" | "bear" | "neutral" | ""; // "" = not labelled yet
  tag: string; // earn | guide | analyst | sector | macro | flow | other | ""
  major: boolean;
}

export interface FillSnapshot {
  ticker: string;
  date: string;
  snapshot: FillSnapshotBody | null;
  hindsight: FillHindsight | null;
  news: FillNews[];
}

// Peers is /api/chart/peers' body (Phase 27 P3): the viewed ticker plus the
// watchlist/held tickers of its sector, strongest 60-day first. A change is
// null when the history is shorter than its window. peers is empty when there
// is nothing to compare, and the card is then hidden.
export interface PeerRow {
  ticker: string;
  self: boolean;
  price: number;
  chg20: number | null;
  chg60: number | null;
  chg120: number | null;
  rel: number | null;
}

export interface Peers {
  sector: string;
  peers: PeerRow[];
}

export function fetchChart(ticker: string): Promise<Chart> {
  return getJSON<Chart>(`/api/chart?ticker=${encodeURIComponent(ticker)}`);
}

export function fetchFillSnapshot(ticker: string, date: string, price: number): Promise<FillSnapshot> {
  return getJSON<FillSnapshot>(
    `/api/fill-snapshot?ticker=${encodeURIComponent(ticker)}&date=${encodeURIComponent(date)}&price=${price}`,
  );
}

export function fetchPeers(ticker: string): Promise<Peers> {
  return getJSON<Peers>(`/api/chart/peers?ticker=${encodeURIComponent(ticker)}`);
}
