import { marketOf, type StrategyAlert } from "./api";
import type { Dictionary } from "./i18n";

// Phase 27 P5 — display logic for /api/chart's `strategies`: the strategy
// signals that were actually pushed. The backend sends the signal type, the
// date, the channel and the text that went out; the names, the validation
// state and every other label come from the dictionary.

export const STRAT_COLOR = "#38bdf8";

// signals.Type* → the suffix of its dictionary keys.
const STRAT_KEY: Record<string, string> = {
  strategy_squeeze_breakout: "squeeze",
  strategy_box_bottom: "box",
  strategy_trend_breakout: "breakout",
  strategy_trend_pullback: "pullback",
  strategy_trust_follow: "trust",
  strategy_mtf_cross: "mtf",
};

export function stratText(dict: Dictionary, type: string): { code: string; name: string } {
  const d = dict as unknown as Record<string, string>;
  const k = STRAT_KEY[type];
  return { code: (k && d[`stratCode_${k}`]) || type, name: (k && d[`stratName_${k}`]) || type };
}

// A strategy alert's identity on the chart: the same `type:date` shape as a
// pattern's, so the one selection state serves both. Strategy types never
// collide with pattern types, and one alert per (type, date) is guaranteed by
// the table.
export function stratKey(a: StrategyAlert): string {
  return `${a.type}:${a.date}`;
}

// 只看高信心 does not touch strategy alerts: they carry no score (like gaps),
// and the 策略 chip is their switch. They are also few — only what was really
// pushed — so there is no crowd to thin out.

export type StratTone = "ok" | "warn" | "bad";

// The three-state validation label, from what the backtests concluded
// (internal/signals; the same verdicts the Telegram notices carry): every
// screen but one failed the out-of-sample bar; the daily-weekly cross is
// positive on TW (though its thresholds were picked after seeing both splits,
// so "positive", not "passed") and measured negative — a contrarian reading —
// on US.
export function stratValid(dict: Dictionary, type: string, ticker: string): { tone: StratTone; label: string; note: string } {
  if (type === "strategy_mtf_cross") {
    return marketOf(ticker) === "tw"
      ? { tone: "ok", label: dict.stratValid_ok, note: dict.stratNote_mtfTw }
      : { tone: "bad", label: dict.stratValid_bad, note: dict.stratNote_mtfUs };
  }
  return {
    tone: "warn",
    label: dict.stratValid_warn,
    note: type === "strategy_trend_breakout" ? dict.stratNote_breakout : dict.stratNote_warn,
  };
}

export function stratChannelNote(dict: Dictionary, a: StrategyAlert): string {
  const t = a.channel === "scan" ? dict.stratPushedScan : dict.stratPushedWatchlist;
  return t.replace("%s", () => a.date);
}
