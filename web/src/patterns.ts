import type { Candle, PatternHit } from "./api";
import type { Dictionary } from "./i18n";

// Phase 27 P4b — display logic for /api/chart's `patterns`. The backend sends
// numbers only; every label here comes from the dictionary.

export type PatCat = PatternHit["cat"];

// Chip order. The design's 策略 chip belongs to P5.
export const PAT_CATS: PatCat[] = ["rev", "cont", "gap", "vol", "indec"];

export type PatOn = Record<PatCat, boolean>;

// Doji fire constantly, so they start hidden (the design's default).
export const DEFAULT_PAT_ON: PatOn = { rev: true, cont: true, gap: true, vol: true, indec: false };

export const PAT_COLORS = { bull: "#34d399", bear: "#f87171", neu: "#cbd5e1" } as const;

// A pattern is shown when its category chip is on and, with 只看高信心 on, when
// it scored "high". Gaps have no score (conf ""), so they always pass.
export function visiblePatterns(hits: PatternHit[], on: PatOn, highOnly: boolean): PatternHit[] {
  return hits.filter((p) => on[p.cat] && (!highOnly || p.conf === "" || p.conf === "high"));
}

// A pattern type's short code, name and definition. The dictionary holds them
// as flat patCode_/patName_/patDef_<type> keys, like every other string.
export function patText(dict: Dictionary, type: string): { code: string; name: string; def: string } {
  const d = dict as unknown as Record<string, string>;
  return { code: d[`patCode_${type}`] ?? type, name: d[`patName_${type}`] ?? type, def: d[`patDef_${type}`] ?? "" };
}

export function patKey(p: PatternHit): string {
  return `${p.type}:${p.end}`;
}

export interface PatternHistory {
  n: number;
  avg5: number | null; // mean 5-day move after the pattern, %
  hitPct: number | null; // share that moved the way the pattern implies, %
  lowSample: boolean;
}

export const LOW_SAMPLE_N = 8;

// How the same pattern has played out on this ticker. Only occurrences with a
// full 5 bars after them count. "Went its way" is up for bullish, down for
// bearish, and plain "up" for neutral ones (the caller labels it).
export function patternHistory(hits: PatternHit[], of: PatternHit): PatternHistory {
  const same = hits.filter((p) => p.type === of.type && p.fwd5 !== null);
  const n = same.length;
  if (n === 0) return { n, avg5: null, hitPct: null, lowSample: true };
  const avg5 = same.reduce((s, p) => s + (p.fwd5 as number), 0) / n;
  const hit = same.filter((p) => (of.dir === "bear" ? (p.fwd5 as number) < 0 : (p.fwd5 as number) > 0)).length;
  return { n, avg5, hitPct: Math.round((hit / n) * 100), lowSample: n < LOW_SAMPLE_N };
}

// The labels patternConds needs, as a slice of the dictionary.
export interface PatCondText {
  condBody: string;
  condVol: string;
  condPrior5: string;
  condBodyPrev: string;
  condCloseInto: string;
  condLowerShadow: string;
  condUpperShadow: string;
  condPrior20High: string;
  condPrior20Low: string;
  condPriorHighDate: string;
  condVolVsHigh: string;
  condRange10: string;
  condGapRange: string;
  condGapSize: string;
  condGapStatus: string;
  gapStatusOpen: string; // %s = sessions open
  gapStatusFilled: string; // %s = day count, then date
}

const sgn = (v: number, digits: number) => `${v >= 0 ? "+" : ""}${v.toFixed(digits)}%`;

function fill(template: string, ...args: string[]): string {
  return args.reduce((t, a) => t.replace("%s", () => a), template);
}

// The "why it fired" rows: the measured values behind a hit, in the order the
// design lists them. Gaps get their own three rows instead of the common ones.
export function patternConds(p: PatternHit, candles: Candle[], t: PatCondText): { label: string; value: string }[] {
  if (p.cat === "gap" && p.gap) {
    const i = candles.findIndex((c) => c.date === p.end);
    const j = p.gap.fillDate ? candles.findIndex((c) => c.date === p.gap!.fillDate) : -1;
    const status =
      p.gap.fillDate && i >= 0 && j >= 0
        ? fill(t.gapStatusFilled, String(j - i), p.gap.fillDate)
        : fill(t.gapStatusOpen, String(i >= 0 ? candles.length - 1 - i : 0));
    return [
      { label: t.condGapRange, value: `${p.gap.lo.toFixed(2)} – ${p.gap.hi.toFixed(2)}` },
      { label: t.condGapSize, value: `${(((p.gap.hi - p.gap.lo) / p.gap.lo) * 100).toFixed(2)}%` },
      { label: t.condGapStatus, value: status },
    ];
  }

  const rows = [
    { label: t.condBody, value: `${p.bodyRatio.toFixed(1)}×` },
    { label: t.condVol, value: `${p.volRatio.toFixed(1)}×` },
    { label: t.condPrior5, value: sgn(p.prior5Pct, 1) },
  ];
  switch (p.type) {
    case "bullEngulf":
    case "bearEngulf":
      rows.push({ label: t.condBodyPrev, value: `${p.extra.toFixed(1)}×` });
      break;
    case "bullHarami":
    case "bearHarami":
      rows.push({ label: t.condBodyPrev, value: `${p.extra.toFixed(2)}×` });
      break;
    case "piercing":
    case "darkCloud":
      rows.push({ label: t.condCloseInto, value: `${Math.round(p.extra)}%` });
      break;
    case "hammer":
    case "hangingMan":
      rows.push({ label: t.condLowerShadow, value: `${p.extra.toFixed(1)}×` });
      break;
    case "shooting":
    case "invHammer":
      rows.push({ label: t.condUpperShadow, value: `${p.extra.toFixed(1)}×` });
      break;
    case "volBreak":
      rows.push({ label: t.condPrior20High, value: p.extra.toFixed(2) });
      break;
    case "volDump":
      rows.push({ label: t.condPrior20Low, value: p.extra.toFixed(2) });
      break;
    case "volDiverge":
      rows.push({ label: t.condPriorHighDate, value: p.refDate ?? "—" }, { label: t.condVolVsHigh, value: `${p.extra.toFixed(2)}×` });
      break;
    case "volDry":
      rows.push({ label: t.condRange10, value: `${p.extra.toFixed(1)}%` });
      break;
  }
  return rows;
}
