import type { Candle } from "./api";

// Price-volume colouring (Phase 27 P4a, the design's "量價" chip): every volume
// bar is classed by price direction × volume direction against the previous
// bar. A flat close or flat volume counts as up, as in the design.
export type PvState = "upVu" | "upVd" | "dnVu" | "dnVd";

export const PV_STATES: PvState[] = ["upVu", "upVd", "dnVu", "dnVd"];

export const PV_COLORS: Record<PvState, string> = {
  upVu: "rgba(16,185,129,.9)",
  upVd: "rgba(16,185,129,.25)",
  dnVu: "rgba(239,68,68,.9)",
  dnVd: "rgba(239,68,68,.25)",
};

// null for the first bar, which has nothing to compare against.
export function pvState(candles: Candle[], i: number): PvState | null {
  if (i <= 0 || i >= candles.length) return null;
  const c = candles[i];
  const p = candles[i - 1];
  return `${c.close >= p.close ? "up" : "dn"}${c.volume >= p.volume ? "Vu" : "Vd"}` as PvState;
}

export interface PvLabels {
  states: Record<PvState, string>;
  price: string; // prefix before the price change, e.g. "價 " / "px "
  vol: string;
}

const signed = (v: number, digits: number) => `${v >= 0 ? "+" : ""}${v.toFixed(digits)}%`;

// The crosshair line under the chart: "2026-06-10 · 價漲量增 · 價 +1.23% · 量 +45%".
// The first bar is just its date; a previous bar with no volume (a halt) drops
// the volume change instead of printing Infinity.
export function pvReadout(candles: Candle[], i: number, labels: PvLabels): string {
  const state = pvState(candles, i);
  if (!state) return candles[i]?.date ?? "";
  const c = candles[i];
  const p = candles[i - 1];
  const parts = [c.date, labels.states[state]];
  if (p.close > 0) parts.push(labels.price + signed((c.close / p.close - 1) * 100, 2));
  if (p.volume > 0) parts.push(labels.vol + signed((c.volume / p.volume - 1) * 100, 0));
  return parts.join(" · ");
}
