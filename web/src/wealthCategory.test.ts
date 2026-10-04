import { describe, expect, it } from "vitest";
import type { AssetGroup, AssetGroupRow, BalanceSheetItem } from "./api";
import { sheetGroups } from "./wealthCategory";

const item = (type: string, valueTwd: number): BalanceSheetItem => ({
  name: type,
  currency: "TWD",
  valueTwd,
  type,
  category: "cash",
  source: "manual",
});
const row = (group: AssetGroup, ...assets: BalanceSheetItem[]): AssetGroupRow => ({
  group,
  marketValue: 0,
  pctOfAssets: null,
  assets,
});

describe("sheetGroups", () => {
  it("files by type, whatever asset_group the row carries", () => {
    const g = sheetGroups(
      [row("income", item("deposit", 100), item("bond", 200)), row("hard", item("pension", 300), item("gold", 400)), row("growth", item("equity_us", 500))],
      1500,
    );
    expect(g.map((x) => [x.group, x.marketValue])).toEqual([
      ["liquid", 100],
      ["investment", 700],
      ["hard", 400],
      ["retirement", 300],
    ]);
    expect(g[0].pctOfAssets).toBeCloseTo(6.67, 1);
  });

  it("falls back to the asset_group bucket for a type the design doesn't list", () => {
    const g = sheetGroups([row("income", item("other", 10)), row("hard", item("other", 20)), row("liquid", item("other", 30))], 60);
    expect(g.map((x) => x.marketValue)).toEqual([30, 10, 20, 0]);
  });

  it("has no percentages when the total is unknown", () => {
    expect(sheetGroups([row("liquid", item("deposit", 1))], null).every((x) => x.pctOfAssets == null)).toBe(true);
  });
});
