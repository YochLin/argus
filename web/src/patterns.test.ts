import { describe, expect, it } from "vitest";
import type { Candle, PatternHit } from "./api";
import { DEFAULT_PAT_ON, patternConds, patternHistory, visiblePatterns, type PatCondText } from "./patterns";

function hit(over: Partial<PatternHit>): PatternHit {
  return {
    type: "bullEngulf",
    cat: "rev",
    dir: "bull",
    start: "2026-01-01",
    end: "2026-01-02",
    conf: "high",
    bodyRatio: 2.4,
    volRatio: 1.8,
    prior5Pct: -3.2,
    extra: 2.1,
    fwd5: null,
    ...over,
  };
}

const T: PatCondText = {
  condBody: "body",
  condVol: "vol",
  condPrior5: "prior5",
  condBodyPrev: "bodyPrev",
  condCloseInto: "closeInto",
  condLowerShadow: "lower",
  condUpperShadow: "upper",
  condPrior20High: "hi20",
  condPrior20Low: "lo20",
  condPriorHighDate: "hiDate",
  condVolVsHigh: "volVsHigh",
  condRange10: "range10",
  condGapRange: "gapRange",
  condGapSize: "gapSize",
  condGapStatus: "gapStatus",
  gapStatusOpen: "open %s",
  gapStatusFilled: "filled day %s (%s)",
};

const candle = (date: string): Candle => ({ date, open: 1, high: 1, low: 1, close: 1, volume: 1 });

describe("visiblePatterns", () => {
  const hits = [
    hit({ conf: "high" }),
    hit({ conf: "low", type: "hammer" }),
    hit({ cat: "gap", type: "gapUp", conf: "" }),
    hit({ cat: "indec", type: "doji", conf: "high" }),
  ];
  it("high-only keeps high and unscored gaps, and hides categories that are off", () => {
    const got = visiblePatterns(hits, DEFAULT_PAT_ON, true);
    expect(got.map((p) => p.type)).toEqual(["bullEngulf", "gapUp"]); // low hammer out, doji chip off
  });
  it("without high-only, low and mid come back", () => {
    expect(visiblePatterns(hits, DEFAULT_PAT_ON, false).map((p) => p.type)).toEqual(["bullEngulf", "hammer", "gapUp"]);
  });
  it("a category chip turned on shows its patterns", () => {
    expect(visiblePatterns(hits, { ...DEFAULT_PAT_ON, indec: true }, true).map((p) => p.type)).toContain("doji");
  });
});

describe("patternHistory", () => {
  it("counts only occurrences with a 5-day result, averaging and scoring by direction", () => {
    const of = hit({ type: "bullEngulf", dir: "bull" });
    const hits = [of, hit({ fwd5: 4 }), hit({ fwd5: -2 }), hit({ fwd5: 6 }), hit({ fwd5: null }), hit({ type: "hammer", fwd5: 100 })];
    const h = patternHistory(hits, of);
    expect(h.n).toBe(3);
    expect(h.avg5).toBeCloseTo(8 / 3);
    expect(h.hitPct).toBe(67); // 2 of 3 rose
    expect(h.lowSample).toBe(true);
  });
  it("a bearish pattern went its way when it fell", () => {
    const of = hit({ type: "bearEngulf", dir: "bear" });
    const h = patternHistory([hit({ type: "bearEngulf", dir: "bear", fwd5: -3 }), hit({ type: "bearEngulf", dir: "bear", fwd5: 2 })], of);
    expect(h.hitPct).toBe(50);
  });
  it("no history is null, not zero", () => {
    expect(patternHistory([], hit({}))).toEqual({ n: 0, avg5: null, hitPct: null, lowSample: true });
  });
  it("eight occurrences is no longer a small sample", () => {
    const hits = Array.from({ length: 8 }, () => hit({ fwd5: 1 }));
    expect(patternHistory(hits, hits[0]).lowSample).toBe(false);
  });
});

describe("patternConds", () => {
  it("lists the three common rows plus the pattern's own", () => {
    const rows = patternConds(hit({ type: "piercing", extra: 62.4 }), [], T);
    expect(rows).toEqual([
      { label: "body", value: "2.4×" },
      { label: "vol", value: "1.8×" },
      { label: "prior5", value: "-3.2%" },
      { label: "closeInto", value: "62%" },
    ]);
  });
  it("divergence adds the prior high's date and the volume ratio", () => {
    const rows = patternConds(hit({ type: "volDiverge", cat: "vol", extra: 0.75, refDate: "2026-01-10" }), [], T);
    expect(rows.slice(3)).toEqual([
      { label: "hiDate", value: "2026-01-10" },
      { label: "volVsHigh", value: "0.75×" },
    ]);
  });
  it("an open gap reports the sessions since it opened", () => {
    const cs = ["2026-01-01", "2026-01-02", "2026-01-03", "2026-01-04"].map(candle);
    const rows = patternConds(hit({ type: "gapUp", cat: "gap", conf: "", end: "2026-01-02", gap: { lo: 100, hi: 102, fillDate: "" } }), cs, T);
    expect(rows).toEqual([
      { label: "gapRange", value: "100.00 – 102.00" },
      { label: "gapSize", value: "2.00%" },
      { label: "gapStatus", value: "open 2" },
    ]);
  });
  it("a filled gap says on which day", () => {
    const cs = ["2026-01-01", "2026-01-02", "2026-01-03", "2026-01-04"].map(candle);
    const rows = patternConds(hit({ type: "gapDown", cat: "gap", conf: "", end: "2026-01-02", gap: { lo: 98, hi: 100, fillDate: "2026-01-04" } }), cs, T);
    expect(rows[2]).toEqual({ label: "gapStatus", value: "filled day 2 (2026-01-04)" });
  });
});
