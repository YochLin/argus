import { describe, expect, it } from "vitest";
import type { Fill, FillSnapshotBody, RoundSummary } from "./api";
import { alignment, defaultFill, fillRows, hindsightRows, newsTime, readings, safeHref, snapshotJson } from "./fillSnapshot";

const fill = (over: Partial<Fill>): Fill => ({
  id: 1,
  date: "2026-03-02",
  ticker: "AAPL",
  side: "BUY",
  shares: 10,
  price: 100,
  fee: 0,
  realizedPnL: 0,
  roundStart: "2026-03-02",
  ...over,
});

const round = (start: string): RoundSummary => ({
  ticker: "AAPL",
  start,
  end: "",
  open: false,
  shares: 10,
  realizedPnL: 0,
  maePct: 0,
  mfePct: 0,
  hasMaeMfe: false,
});

const snap = (over: Partial<FillSnapshotBody> = {}): FillSnapshotBody => ({
  date: "2026-03-02",
  close: 100,
  high: 101,
  low: 99,
  dayChangePct: 1,
  rsi14: 55,
  rsi14Prev5: 50,
  rsiZone: "mid",
  macdDif: 1,
  macdDea: 0.5,
  macdHist: 0.5,
  macdCrossDays: 3,
  macdWidening: true,
  trend: "bull",
  closeVsMa20Pct: 2,
  ma20Slope5dPct: 1,
  volRatio20: 1.5,
  volRatio5v20: 1.1,
  volState: "up",
  patterns: [],
  ...over,
});

describe("fillRows / defaultFill", () => {
  // Rounds arrive newest first, like /api/chart sends them.
  const rounds = [round("2026-03-02"), round("2026-01-05")];
  const fills = [
    fill({ id: 1, date: "2026-01-05", roundStart: "2026-01-05" }),
    fill({ id: 2, date: "2026-02-01", side: "SELL", roundStart: "2026-01-05" }),
    fill({ id: 3, date: "2026-02-01", roundStart: "2026-02-01" }), // re-bought the day it sold
    fill({ id: 4, date: "2026-03-02", roundStart: "2026-03-02" }),
  ];
  const rows = fillRows(fills, rounds);

  it("lists newest first, numbering rounds oldest-first", () => {
    expect(rows.map((r) => r.fill.id)).toEqual([4, 3, 2, 1]);
    expect(rows.map((r) => r.roundNo)).toEqual([2, 0, 1, 1]); // 0 = a round the list doesn't know
  });

  it("falls back to the newest fill", () => {
    expect(defaultFill(rows, null, null)?.fill.id).toBe(4);
  });
  it("a picked fill wins; a stale pick is ignored", () => {
    expect(defaultFill(rows, 2, null)?.fill.id).toBe(2);
    expect(defaultFill(rows, 99, null)?.fill.id).toBe(4);
  });
  it("a selected round opens on its entry, not its exit", () => {
    expect(defaultFill(rows, null, "2026-01-05")?.fill.id).toBe(1);
  });
  it("nothing to show without fills", () => {
    expect(defaultFill([], null, null)).toBeNull();
  });
});

describe("readings", () => {
  it("a buy is toned against the trade", () => {
    const rs = readings(snap(), "BUY");
    expect(rs.map((r) => r.tone)).toEqual(["ok", "ok", "ok", "ok"]);
    expect(alignment(rs)).toEqual({ ok: 4, bad: 0 });
  });
  it("buying into overbought, a falling MACD and a downtrend count against it", () => {
    const rs = readings(snap({ rsiZone: "hot", macdHist: -1, trend: "bear", volState: "flat" }), "BUY");
    expect(rs.map((r) => r.tone)).toEqual(["bad", "bad", "bad", "neu"]);
    expect(alignment(rs)).toEqual({ ok: 0, bad: 3 });
  });
  it("a range trend is neutral", () => {
    expect(readings(snap({ trend: "range" }), "BUY")[2].tone).toBe("neu");
  });
  // The case that made the design mislead: a profit-taking sell into strength.
  it("a sell gets no verdict at all", () => {
    const rs = readings(snap({ rsiZone: "hot", macdHist: 2, trend: "bull" }), "SELL");
    expect(rs.every((r) => r.tone === "neu")).toBe(true);
    expect(alignment(rs)).toEqual({ ok: 0, bad: 0 });
  });
});

describe("hindsightRows", () => {
  const h = { fwd5Pct: 4, fwd20Pct: -6, maxUpPct: 9, maxDownPct: -8 };
  const by = (rows: ReturnType<typeof hindsightRows>) => Object.fromEntries(rows.map((r) => [r.id, r]));

  it("after a buy, up is good", () => {
    const r = by(hindsightRows(h, "BUY"));
    expect(r.fwd5).toEqual({ id: "fwd5", value: 4, tone: "ok" });
    expect(r.fwd20).toEqual({ id: "fwd20", value: -6, tone: "bad" });
    expect(r.best.value).toBe(9);
    expect(r.worst.value).toBe(-8);
  });
  it("after a sell, down is good, and best/worst are from the sell's side", () => {
    const r = by(hindsightRows(h, "SELL"));
    // The price figures stay the plain move; the colour flips.
    expect(r.fwd5).toEqual({ id: "fwd5", value: 4, tone: "bad" });
    expect(r.fwd20).toEqual({ id: "fwd20", value: -6, tone: "ok" });
    expect(r.best.value).toBe(8); // the deepest drop
    expect(r.worst.value).toBe(-9); // the highest rise
    expect(r.best.tone).toBe("ok");
    expect(r.worst.tone).toBe("bad");
  });
  it("a move the data doesn't reach yet stays null and neutral", () => {
    const r = by(hindsightRows({ fwd5Pct: null, fwd20Pct: null, maxUpPct: null, maxDownPct: null }, "SELL"));
    for (const id of ["fwd5", "fwd20", "best", "worst"]) expect(r[id]).toEqual({ id, value: null, tone: "neu" });
  });
});

describe("safeHref", () => {
  it("passes web links and nothing else", () => {
    expect(safeHref("https://example.com/a?b=1")).toBe("https://example.com/a?b=1");
    expect(safeHref("http://example.com")).toBe("http://example.com");
    expect(safeHref("javascript:alert(1)")).toBeNull();
    expect(safeHref("data:text/html,<script>1</script>")).toBeNull();
    expect(safeHref("//example.com")).toBeNull();
    expect(safeHref("")).toBeNull();
  });
});

describe("newsTime", () => {
  it("is a dash without a usable timestamp", () => {
    expect(newsTime("")).toBe("—");
    expect(newsTime("nope")).toBe("—");
  });
  it("is HH:MM otherwise", () => {
    expect(newsTime("2026-03-02T08:30:00Z")).toMatch(/^\d\d:\d\d$/);
  });
});

describe("snapshotJson", () => {
  const news = [
    { headline: "h", source: "s", url: "https://x", publishedAt: "", sentiment: "" as const, tag: "", major: false },
  ];
  const out = JSON.parse(snapshotJson("AAPL", fill({}), snap({ patterns: [{ type: "hammer", dir: "bull" }] }), news));

  it("carries the snapshot and the news with their links", () => {
    expect(out.ticker).toBe("AAPL");
    expect(out.indicators.rsi14).toBe(55);
    expect(out.candle_events).toEqual(["hammer"]);
    expect(out.news[0]).toMatchObject({ headline: "h", url: "https://x", time: null, tag: null, sentiment: null });
  });
  it("leaves out what happened afterwards and the verdicts", () => {
    const text = JSON.stringify(out);
    expect(text).not.toMatch(/hindsight|fwd|alignment/);
  });
});
