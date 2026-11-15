import { describe, expect, it } from "vitest";
import type { AssetGroup, AssetGroupRow, BalanceSheet, BalanceSheetItem, LiabilityDetail } from "./api";
import { overviewAssetSlices, overviewLiabSlices, sheetGroups } from "./wealthCategory";

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

describe("overviewAssetSlices", () => {
  const cat = (category: string, valueTwd: number): BalanceSheetItem => ({ ...item("deposit", valueTwd), category });

  it("folds gold, crypto and pension into one other slice, in the design's order", () => {
    const s = overviewAssetSlices([
      row("liquid", cat("cash", 10), cat("estate", 50)),
      row("hard", cat("gold", 1), cat("crypto", 2), cat("pension", 3), cat("equity", 20)),
    ]);
    expect(s).toEqual([
      { key: "estate", value: 50 },
      { key: "equity", value: 20 },
      { key: "cash", value: 10 },
      { key: "other", value: 6 },
    ]);
  });

  it("sends a category it doesn't know to other instead of dropping the money", () => {
    expect(overviewAssetSlices([row("liquid", cat("mystery", 7))])).toEqual([{ key: "other", value: 7 }]);
  });

  it("is empty with nothing in it", () => {
    expect(overviewAssetSlices([row("liquid")])).toEqual([]);
  });
});

describe("overviewLiabSlices", () => {
  const liab = (assetId: number, type: string, valueTwd: number): LiabilityDetail => ({
    assetId,
    name: String(assetId),
    currency: "TWD",
    valueTwd,
    ratePct: null,
    remainingMonths: null,
    minPayment: null,
    source: "manual",
    type,
  });
  const sheet = (liabs: LiabilityDetail[], longIds: number[]) =>
    ({
      liabilities: liabs,
      liabGroups: [
        { kind: "short", marketValue: 0, pctOfLiabilities: null, liabilities: liabs.filter((l) => !longIds.includes(l.assetId)) },
        { kind: "long", marketValue: 0, pctOfLiabilities: null, liabilities: liabs.filter((l) => longIds.includes(l.assetId)) },
      ],
    }) as unknown as BalanceSheet;

  it("calls a long-term loan a mortgage, a short one a loan, and a card a card", () => {
    const b = sheet([liab(1, "loan", 800), liab(2, "loan", 70), liab(3, "credit_card", 5), liab(4, "loan", 30)], [1]);
    expect(overviewLiabSlices(b)).toEqual([
      { key: "mortgage", value: 800 },
      { key: "loan", value: 100 },
      { key: "card", value: 5 },
    ]);
  });

  it("leaves out a kind with nothing owed", () => {
    expect(overviewLiabSlices(sheet([liab(1, "credit_card", 5)], []))).toEqual([{ key: "card", value: 5 }]);
  });
});
