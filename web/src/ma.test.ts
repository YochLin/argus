import { describe, it, expect } from "vitest";
import type { Candle } from "./api";
import { sma } from "./ma";

const bars = (closes: number[]): Candle[] =>
  closes.map((close, i) => ({ date: `d${i}`, open: close, high: close, low: close, close, volume: 1 }));

describe("sma", () => {
  it("starts at bar `period` and averages the last `period` closes", () => {
    const got = sma(bars([1, 2, 3, 4, 5, 6]), 3);
    expect(got).toEqual([
      { time: "d2", value: 2 },
      { time: "d3", value: 3 },
      { time: "d4", value: 4 },
      { time: "d5", value: 5 },
    ]);
  });

  it("matches a direct average well past the first window (the running sum must not drift)", () => {
    const closes = Array.from({ length: 300 }, (_, i) => 100 + Math.sin(i / 7) * 10 + i * 0.01);
    const got = sma(bars(closes), 60);
    const i = 250;
    const direct = closes.slice(i - 59, i + 1).reduce((a, b) => a + b, 0) / 60;
    expect(got).toHaveLength(300 - 59);
    expect(got[i - 59].time).toBe(`d${i}`);
    expect(got[i - 59].value).toBeCloseTo(direct, 9);
  });

  it("is empty when there are fewer bars than the period", () => {
    expect(sma(bars([1, 2]), 5)).toEqual([]);
  });
});
