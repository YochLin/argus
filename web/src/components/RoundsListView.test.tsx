import { describe, it, expect } from "vitest";
import type { RoundSummary } from "../api";
import { groupRounds, holdDays, tally } from "./RoundsListView";

const r = (ticker: string, start: string, end: string, realizedPnL: number): RoundSummary => ({
  ticker,
  start,
  end,
  open: end === "",
  shares: 100,
  realizedPnL,
  maePct: 0,
  mfePct: 0,
  hasMaeMfe: false,
});

const rounds = [
  r("OPENA", "2026-02-10", "", 0),
  r("OPENB", "2026-03-01", "", 0),
  r("A", "2026-05-04", "2026-06-26", 100),
  r("B", "2026-04-13", "2026-06-26", -40),
  r("C", "2026-03-02", "2026-04-24", 60),
  r("D", "2025-10-06", "2025-12-19", -10),
];
const label = (t: string) => t;
const all = { year: "all", status: "all" as const, q: "" };

describe("groupRounds", () => {
  it("files closed rounds under close year → close month, newest first", () => {
    const g = groupRounds(rounds, all, label);
    expect(g.groups.map((y) => y.year)).toEqual(["2026", "2025"]);
    expect(g.groups[0].months.map((m) => m.month)).toEqual(["2026-06", "2026-04"]);
    // same close day → ticker order, not input order
    expect(g.groups[0].months[0].rounds.map((x) => x.ticker)).toEqual(["A", "B"]);
  });

  it("tallies each level on its own rounds", () => {
    const g = groupRounds(rounds, all, label);
    expect(g.groups[0].months[0].tally).toEqual({ n: 2, won: 1, net: 60 });
    expect(g.groups[0].tally).toEqual({ n: 3, won: 2, net: 120 });
    expect(g.summary).toEqual({ n: 4, won: 2, net: 110 });
  });

  it("keeps open rounds apart, newest start first, out of every tally", () => {
    const g = groupRounds(rounds, all, label);
    expect(g.open.map((x) => x.ticker)).toEqual(["OPENB", "OPENA"]);
    expect(g.summary.n).toBe(4);
  });

  it("year and status filters narrow the groups but not the year chips", () => {
    const y = groupRounds(rounds, { ...all, year: "2025" }, label);
    expect(y.groups.map((x) => x.year)).toEqual(["2025"]);
    expect(y.years.map((x) => x.year)).toEqual(["2026", "2025"]);
    expect(y.open).toHaveLength(2); // a year filter is about closed rounds only

    expect(groupRounds(rounds, { ...all, status: "open" }, label).groups).toEqual([]);
    expect(groupRounds(rounds, { ...all, status: "closed" }, label).open).toEqual([]);
  });

  it("search matches the displayed label, case-insensitively", () => {
    const named = (t: string) => (t === "A" ? "Apple(A)" : t);
    const g = groupRounds(rounds, { ...all, q: " apple " }, named);
    expect(g.groups).toHaveLength(1);
    expect(g.groups[0].months[0].rounds.map((x) => x.ticker)).toEqual(["A"]);
    expect(g.open).toEqual([]);
  });
});

describe("tally", () => {
  it("counts only a strictly positive round as won", () => {
    expect(tally([r("A", "2026-01-01", "2026-01-02", 0), r("B", "2026-01-01", "2026-01-02", 5)]).won).toBe(1);
  });
});

describe("holdDays", () => {
  it("counts calendar days and floors a same-day round at 1", () => {
    expect(holdDays("2026-05-04", "2026-06-26", "2026-09-01")).toBe(53);
    expect(holdDays("2026-05-04", "2026-05-04", "2026-09-01")).toBe(1);
  });

  it("runs an open round up to today", () => {
    expect(holdDays("2026-08-22", "", "2026-09-01")).toBe(10);
  });
});
