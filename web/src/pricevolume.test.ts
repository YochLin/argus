import { describe, it, expect } from "vitest";
import type { Candle } from "./api";
import { pvReadout, pvState, type PvLabels } from "./pricevolume";

const bar = (date: string, close: number, volume: number): Candle => ({
  date,
  open: close,
  high: close,
  low: close,
  close,
  volume,
});

const labels: PvLabels = {
  states: { upVu: "UU", upVd: "UD", dnVu: "DU", dnVd: "DD" },
  price: "px ",
  vol: "vol ",
};

describe("pvState", () => {
  const cs = [bar("d0", 100, 1000), bar("d1", 101, 1500), bar("d2", 101, 900), bar("d3", 99, 900), bar("d4", 98, 2000)];

  it("has no state for the first bar", () => {
    expect(pvState(cs, 0)).toBeNull();
  });

  it("classes price × volume direction, flat counting as up", () => {
    expect(pvState(cs, 1)).toBe("upVu");
    expect(pvState(cs, 2)).toBe("upVd"); // flat close, lower volume
    expect(pvState(cs, 3)).toBe("dnVu"); // lower close, flat volume
    expect(pvState(cs, 4)).toBe("dnVu");
  });

  it("classes a falling close on falling volume", () => {
    expect(pvState([bar("a", 100, 1000), bar("b", 98, 400)], 1)).toBe("dnVd");
  });
});

describe("pvReadout", () => {
  it("prints the date only for the first bar", () => {
    expect(pvReadout([bar("2026-06-01", 100, 1000)], 0, labels)).toBe("2026-06-01");
  });

  it("prints the state and both changes, signed", () => {
    const cs = [bar("2026-06-01", 100, 1000), bar("2026-06-02", 101.234, 1450)];
    expect(pvReadout(cs, 1, labels)).toBe("2026-06-02 · UU · px +1.23% · vol +45%");
    const down = [bar("a", 100, 1000), bar("b", 98, 400)];
    expect(pvReadout(down, 1, labels)).toBe("b · DD · px -2.00% · vol -60%");
  });

  it("drops the volume change when the previous bar had no volume", () => {
    const cs = [bar("a", 100, 0), bar("b", 101, 500)];
    expect(pvReadout(cs, 1, labels)).toBe("b · UU · px +1.00%");
  });
});
