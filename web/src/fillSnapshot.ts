import type { Fill, FillHindsight, FillNews, FillSnapshotBody, RoundSummary } from "./api";

// Phase 27 P6 — the snapshot tab's logic. The backend computes every number
// (internal/service/fillsnapshot.go); this file only decides how a reading is
// toned for the side of the trade, and shapes the list and the JSON view.

export type Side = "BUY" | "SELL";
export type Tone = "ok" | "bad" | "neu";

export const sideOf = (f: { side: string }): Side => (f.side === "SELL" ? "SELL" : "BUY");

export interface FillRow {
  fill: Fill;
  side: Side;
  roundNo: number; // 1 = the ticker's oldest round, as the 回合 tab numbers them; 0 if unknown
}

// The picker's rows, newest first. rounds come from /api/chart newest-first, so
// the oldest round is #1.
export function fillRows(fills: Fill[], rounds: RoundSummary[]): FillRow[] {
  const chrono = [...rounds].reverse();
  return fills
    .map((fill) => ({ fill, side: sideOf(fill), roundNo: chrono.findIndex((r) => r.start === fill.roundStart) + 1 }))
    .sort((a, b) => (a.fill.date === b.fill.date ? b.fill.id - a.fill.id : a.fill.date < b.fill.date ? 1 : -1));
}

// Which fill is shown: the picked one; else the entry of the selected round;
// else the newest.
export function defaultFill(rows: FillRow[], pickedId: number | null, selectedRoundStart: string | null): FillRow | null {
  const picked = rows.find((r) => r.fill.id === pickedId);
  if (picked) return picked;
  const entry = selectedRoundStart
    ? [...rows].reverse().find((r) => r.fill.roundStart === selectedRoundStart && r.side === "BUY")
    : undefined;
  return entry ?? rows[0] ?? null;
}

export type IndId = "rsi" | "macd" | "trend" | "vol";

export interface Reading {
  id: IndId;
  tone: Tone;
}

// Whether each reading went with the trade — for a BUY only. The rules are the
// design's own rules of thumb (an overbought RSI is a poor place to buy, a
// positive MACD histogram and an uptrend are good ones), not something this
// project backtested, and they say nothing useful about a sell: the design
// scored a profit-taking sell into an uptrend as "wrong". So a sell gets the
// numbers and no verdict, and every reading comes back neutral.
export function readings(s: FillSnapshotBody, side: Side): Reading[] {
  const buy = side === "BUY";
  const t = (cond: Tone): Tone => (buy ? cond : "neu");
  return [
    { id: "rsi", tone: t(s.rsiZone === "hot" ? "bad" : "ok") },
    { id: "macd", tone: t(s.macdHist > 0 ? "ok" : "bad") },
    { id: "trend", tone: t(s.trend === "bull" ? "ok" : s.trend === "bear" ? "bad" : "neu") },
    { id: "vol", tone: t(s.volState === "up" ? "ok" : "neu") },
  ];
}

export function alignment(rs: Reading[]): { ok: number; bad: number } {
  return { ok: rs.filter((r) => r.tone === "ok").length, bad: rs.filter((r) => r.tone === "bad").length };
}

export type HindId = "fwd5" | "fwd20" | "best" | "worst";

export interface HindRow {
  id: HindId;
  value: number | null;
  tone: Tone;
}

const toneOf = (v: number | null): Tone => (v === null || v === 0 ? "neu" : v > 0 ? "ok" : "bad");

// "What happened next". +5d / +20d are the plain price move; best / worst are
// from the trade's point of view (a sell's best is the deepest drop), and the
// colour is for or against the decision: up is good after a buy, down after a
// sell.
export function hindsightRows(h: FillHindsight, side: Side): HindRow[] {
  const dir = side === "BUY" ? 1 : -1;
  const forSide = (v: number | null) => (v === null ? null : v * dir);
  const best = side === "BUY" ? h.maxUpPct : h.maxDownPct === null ? null : -h.maxDownPct;
  const worst = side === "BUY" ? h.maxDownPct : h.maxUpPct === null ? null : -h.maxUpPct;
  return [
    { id: "fwd5", value: h.fwd5Pct, tone: toneOf(forSide(h.fwd5Pct)) },
    { id: "fwd20", value: h.fwd20Pct, tone: toneOf(forSide(h.fwd20Pct)) },
    { id: "best", value: best, tone: toneOf(best) },
    { id: "worst", value: worst, tone: toneOf(worst) },
  ];
}

// A story's link is only used when it is a plain web address: the URL comes from
// a news feed, and an href of any other scheme (javascript:, data:) must never
// reach the page.
export function safeHref(url: string): string | null {
  return /^https?:\/\//i.test(url) ? url : null;
}

export function newsTime(publishedAt: string): string {
  if (!publishedAt) return "—";
  const d = new Date(publishedAt);
  if (Number.isNaN(d.getTime())) return "—";
  return `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
}

// The view-and-copy JSON: the snapshot as of the fill day, without the
// after-the-fact moves and without the tone verdicts (those are a display
// convention, not data).
export function snapshotJson(ticker: string, fill: Fill, s: FillSnapshotBody, news: FillNews[]): string {
  const r = (v: number, d: number) => Number(v.toFixed(d));
  return JSON.stringify(
    {
      ticker,
      date: fill.date,
      side: fill.side,
      price: r(fill.price, 2),
      shares: fill.shares,
      bar: s.date,
      indicators: {
        rsi14: r(s.rsi14, 1),
        rsi14_5d_ago: r(s.rsi14Prev5, 1),
        macd: {
          dif: r(s.macdDif, 3),
          dea: r(s.macdDea, 3),
          hist: r(s.macdHist, 3),
          cross: s.macdHist > 0 ? "bull" : "bear",
          days_since_cross: s.macdCrossDays,
        },
        trend: { state: s.trend, close_vs_ma20_pct: r(s.closeVsMa20Pct, 2), ma20_slope_5d_pct: r(s.ma20Slope5dPct, 2) },
        volume: { ratio_20d: r(s.volRatio20, 2), ratio_5d_20d: r(s.volRatio5v20, 2), state: s.volState },
      },
      candle_events: s.patterns.map((p) => p.type),
      news: news.map((n) => ({
        time: n.publishedAt || null,
        source: n.source,
        tag: n.tag || null,
        sentiment: n.sentiment || null,
        major: n.major,
        headline: n.headline,
        url: n.url || null,
      })),
    },
    null,
    2,
  );
}
