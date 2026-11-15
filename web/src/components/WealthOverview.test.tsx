import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";
import { WealthOverview, donutArcs, trendGeometry, trendSeries, DONUT_R } from "./WealthOverview";
import { getDictionary } from "../i18n";
import * as api from "../api";

vi.mock("../api", async (importOriginal) => {
  const real = await importOriginal<typeof import("../api")>();
  return { ...real, fetchWealthTrend: vi.fn() };
});

const m = vi.mocked(api);
const dict = getDictionary("zh");

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("trendSeries", () => {
  it("drops the empty days before the records start and bridges a gap inside", () => {
    const s = trendSeries([
      { date: "d1", netWorth: null },
      { date: "d2", netWorth: 10 },
      { date: "d3", netWorth: null },
      { date: "d4", netWorth: 12 },
    ]);
    expect(s).toEqual([
      { date: "d2", value: 10 },
      { date: "d4", value: 12 },
    ]);
  });
});

describe("trendGeometry", () => {
  it("keeps the highest value above the lowest, inside the padded box", () => {
    const g = trendGeometry([100, 200, 150]);
    expect(g.y(200)).toBeLessThan(g.y(100));
    expect(g.y(200)).toBeGreaterThan(0);
    expect(g.y(100)).toBeLessThan(280);
    expect(g.x(0)).toBe(0);
    expect(g.x(2)).toBe(600);
    expect(g.area.endsWith("L600,280 L0,280 Z")).toBe(true);
  });

  it("draws a flat series as a level line instead of dividing by zero", () => {
    const g = trendGeometry([5, 5, 5]);
    expect(Number.isFinite(g.y(5))).toBe(true);
    expect(g.line).not.toContain("NaN");
  });
});

describe("donutArcs", () => {
  it("lays slices end to end around the circle, less the gap", () => {
    const c = 2 * Math.PI * DONUT_R;
    const arcs = donutArcs([1, 1, 2]);
    const seg = (a: { dash: string }) => parseFloat(a.dash.split(" ")[0]);
    expect(seg(arcs[2])).toBeCloseTo(c / 2 - 2, 1);
    expect(parseFloat(arcs[1].off)).toBeCloseTo(-c / 4, 1);
    expect(arcs[0].off).toBe("0.00");
  });

  it("gives a lone slice the whole ring, with no gap to leave", () => {
    const c = 2 * Math.PI * DONUT_R;
    expect(parseFloat(donutArcs([7])[0].dash.split(" ")[0])).toBeCloseTo(c, 1);
  });
});

const home = { netWorth: 20_000_000, totalAssets: 29_000_000, totalLiabilities: 9_000_000 } as api.WealthHome;
const item = (category: string, valueTwd: number) => ({ name: category, currency: "TWD", valueTwd, type: "deposit", category, source: "manual" });
const balance = {
  debtRatioPct: 31.0,
  assetGroups: [{ group: "liquid", marketValue: 0, pctOfAssets: null, assets: [item("estate", 12_000_000), item("equity", 6_000_000), item("gold", 1_000_000)] }],
  liabilities: [
    { assetId: 1, name: "房貸", currency: "TWD", valueTwd: 8_000_000, type: "loan" },
    { assetId: 2, name: "卡", currency: "TWD", valueTwd: 100_000, type: "credit_card" },
  ],
  liabGroups: [{ kind: "long", liabilities: [{ assetId: 1 }] }],
} as unknown as api.BalanceSheet;

const trend = (period: api.TrendPeriod, ...v: (number | null)[]) => ({
  period,
  points: v.map((netWorth, i) => ({ date: `2026-11-${String(i + 1).padStart(2, "0")}`, netWorth })),
});

function show() {
  return render(<WealthOverview dict={dict} home={home} balance={balance} refresh={0} money={(v) => `NT$${v}`} />);
}

describe("WealthOverview", () => {
  it("shows the headline cards and the period's gain", async () => {
    m.fetchWealthTrend.mockResolvedValue(trend("ytd", 18_000_000, 19_000_000, 20_000_000));
    show();
    expect(screen.getByText("NT$20000000")).toBeTruthy();
    expect(screen.getByText("NT$29000000")).toBeTruthy();
    expect(screen.getByText("31.0%")).toBeTruthy();
    expect(await screen.findByText(/\+NT\$2000000（\+11\.1%）今年/)).toBeTruthy();
  });

  it("flags a loss over the period", async () => {
    m.fetchWealthTrend.mockResolvedValue(trend("ytd", 21_000_000, 20_000_000));
    show();
    const pill = await screen.findByText(/NT\$-1000000/);
    expect(pill.className).toContain("ov-pill--down");
  });

  it("says so, instead of drawing a chart, when there are fewer than two points", async () => {
    m.fetchWealthTrend.mockResolvedValue(trend("ytd", null, 20_000_000));
    show();
    expect(await screen.findByText(dict.wealthOvTrendEmpty)).toBeTruthy();
    expect(document.querySelector(".ov-line")).toBeNull();
  });

  it("asks for the other period when it is picked", async () => {
    m.fetchWealthTrend.mockResolvedValue(trend("ytd", 1, 2));
    show();
    fireEvent.click(screen.getByRole("button", { name: dict.wealthOvPeriod1y }));
    await waitFor(() => expect(m.fetchWealthTrend).toHaveBeenLastCalledWith("1y"));
  });

  it("folds the assets into the design's categories and swaps to liabilities", async () => {
    m.fetchWealthTrend.mockResolvedValue(trend("ytd", 1, 2));
    show();
    // estate, equity and the gold-in-other slice
    expect(screen.getByText(dict.wealthOvCatOther)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: dict.wealthOvModeLiabs }));
    expect(screen.getByText(dict.wealthOvLiabMortgage)).toBeTruthy();
    expect(screen.getByText(dict.wealthOvLiabCard)).toBeTruthy();
    expect(screen.queryByText(dict.wealthOvCatOther)).toBeNull();
  });

  it("puts the hovered legend row in the donut's centre", async () => {
    m.fetchWealthTrend.mockResolvedValue(trend("ytd", 1, 2));
    show();
    const row = screen.getByText(dict.wealthOvCatOther).closest("button")!;
    fireEvent.mouseEnter(row);
    expect(document.querySelector(".ov-donut-label")!.textContent).toBe(dict.wealthOvCatOther);
    fireEvent.mouseLeave(row.closest("ul")!);
    expect(document.querySelector(".ov-donut-label")!.textContent).toBe(dict.wealthTotalAssets);
  });
});
