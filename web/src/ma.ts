import type { Candle } from "./api";

// The chart's 均線 chip: simple moving averages of the close, drawn from the
// candles the page already has. The periods are fixed on purpose — a settings
// panel would invite re-tuning them against the same chart.
export const MA_PERIODS = [5, 20, 60] as const;

// Clear of the candle greens/reds, the pattern colours and the strategy blue.
export const MA_COLORS: Record<(typeof MA_PERIODS)[number], string> = {
  5: "#fbbf24",
  20: "#a78bfa",
  60: "#f472b6",
};

export interface MaPoint {
  time: string;
  value: number;
}

// The first period-1 bars have no average, so the line starts at bar period.
export function sma(candles: Candle[], period: number): MaPoint[] {
  const out: MaPoint[] = [];
  let sum = 0;
  for (let i = 0; i < candles.length; i++) {
    sum += candles[i].close;
    if (i >= period) sum -= candles[i - period].close;
    if (i >= period - 1) out.push({ time: candles[i].date, value: sum / period });
  }
  return out;
}

// Per-browser convenience, like the language override: off by default (the chart
// is already busy) and remembered once switched on.
const MA_KEY = "argus.chart.ma";

export function loadMaOn(): boolean {
  try {
    return localStorage.getItem(MA_KEY) === "1";
  } catch {
    return false; // storage blocked: fall through to the default
  }
}

export function saveMaOn(on: boolean) {
  try {
    localStorage.setItem(MA_KEY, on ? "1" : "0");
  } catch {
    // per-browser convenience only
  }
}
