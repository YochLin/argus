import type { AllocCategory, AllocationModel, AssetGroup, AssetGroupRow, BalanceSheet, BalanceSheetItem } from "./api";
import type { Dictionary } from "./i18n";

export type SheetGroup = "liquid" | "investment" | "hard" | "retirement";

export interface SheetGroupRow {
  group: SheetGroup;
  marketValue: number;
  pctOfAssets: number | null;
  assets: BalanceSheetItem[];
}

// The balance sheet files an asset by its type (design balanceModel's mk()),
// not by asset_group — that stays the home page's four buckets.
const SHEET_GROUP_OF_TYPE: Record<string, SheetGroup> = {
  deposit: "liquid",
  equity_tw: "investment",
  equity_us: "investment",
  fund: "investment",
  bond: "investment",
  insurance: "investment",
  crypto: "investment",
  estate: "hard",
  gold: "hard",
  pension: "retirement",
};

// Only a type the design doesn't list (e.g. "other") falls back to its bucket.
const SHEET_GROUP_OF_BUCKET: Record<AssetGroup, SheetGroup> = {
  liquid: "liquid",
  growth: "investment",
  income: "investment",
  hard: "hard",
};

const SHEET_GROUPS: SheetGroup[] = ["liquid", "investment", "hard", "retirement"];

export function sheetGroups(rows: AssetGroupRow[], totalAssets: number | null): SheetGroupRow[] {
  const out = SHEET_GROUPS.map((group): SheetGroupRow => ({ group, marketValue: 0, pctOfAssets: null, assets: [] }));
  for (const row of rows) {
    for (const item of row.assets) {
      const g = out.find((o) => o.group === (SHEET_GROUP_OF_TYPE[item.type] ?? SHEET_GROUP_OF_BUCKET[row.group]))!;
      g.assets.push(item);
      g.marketValue += item.valueTwd;
    }
  }
  if (totalAssets != null && totalAssets > 0) {
    for (const g of out) g.pctOfAssets = (g.marketValue / totalAssets) * 100;
  }
  return out;
}

export function sheetGroupLabel(dict: Dictionary, g: SheetGroup): string {
  switch (g) {
    case "liquid":
      return dict.wealthSheetLiquid;
    case "investment":
      return dict.wealthSheetInvestment;
    case "hard":
      return dict.wealthSheetHard;
    case "retirement":
      return dict.wealthSheetRetirement;
  }
}

// Design template's nine asset-type colours (wColors()), shared by home and /w/alloc.
export const CATEGORY_COLOR: Record<AllocCategory, string> = {
  cash: "#38bdf8",
  equity: "var(--accent)",
  fund: "#a78bfa",
  bond: "#34d399",
  insurance: "#f59e0b",
  estate: "#c08457",
  gold: "#eab308",
  crypto: "#f472b6",
  pension: "#94a3b8",
};

// 淨值總覽's donut folds the nine categories into seven: gold, crypto and
// pension share one "other" slice (the design's 其他（黃金・加密・勞退）). It is a
// display fold of AllocCategory, not a third taxonomy — /w/alloc keeps the nine.
export type OverviewCategory = "estate" | "equity" | "cash" | "fund" | "insurance" | "bond" | "other";

export const OVERVIEW_ORDER: OverviewCategory[] = ["estate", "equity", "cash", "fund", "insurance", "bond", "other"];

export const OVERVIEW_COLOR: Record<OverviewCategory, string> = {
  estate: CATEGORY_COLOR.estate,
  equity: CATEGORY_COLOR.equity,
  cash: CATEGORY_COLOR.cash,
  fund: CATEGORY_COLOR.fund,
  insurance: CATEGORY_COLOR.insurance,
  bond: CATEGORY_COLOR.bond,
  other: "#64748b",
};

export interface OverviewSlice<K extends string> {
  key: K;
  value: number;
}

// overviewAssetSlices sums the balance sheet's assets by folded category, in
// OVERVIEW_ORDER, leaving out categories with nothing in them.
export function overviewAssetSlices(rows: AssetGroupRow[]): OverviewSlice<OverviewCategory>[] {
  const sum = new Map<OverviewCategory, number>();
  for (const row of rows) {
    for (const a of row.assets) {
      const k: OverviewCategory = (OVERVIEW_ORDER as string[]).includes(a.category) ? (a.category as OverviewCategory) : "other";
      sum.set(k, (sum.get(k) ?? 0) + a.valueTwd);
    }
  }
  return OVERVIEW_ORDER.filter((k) => (sum.get(k) ?? 0) > 0).map((key) => ({ key, value: sum.get(key)! }));
}

export type OverviewLiab = "mortgage" | "loan" | "card";

// overviewLiabSlices splits liabilities three ways. The backend has no
// mortgage type, only the long/short split by remaining term (liabGroups), so
// a loan is a 房貸 exactly when that split files it as long-term. A long car
// loan, or a mortgage with no term on file, lands on the wrong side.
export function overviewLiabSlices(b: BalanceSheet): OverviewSlice<OverviewLiab>[] {
  const long = new Set(b.liabGroups.find((g) => g.kind === "long")?.liabilities.map((l) => l.assetId));
  const sum: Record<OverviewLiab, number> = { mortgage: 0, loan: 0, card: 0 };
  for (const l of b.liabilities) {
    sum[l.type === "credit_card" ? "card" : long.has(l.assetId) ? "mortgage" : "loan"] += l.valueTwd;
  }
  return (["mortgage", "loan", "card"] as OverviewLiab[]).filter((k) => sum[k] > 0).map((key) => ({ key, value: sum[key] }));
}

export function categoryLabel(dict: Dictionary, c: AllocCategory): string {
  switch (c) {
    case "cash":
      return dict.wealthCategoryCash;
    case "equity":
      return dict.wealthCategoryEquity;
    case "fund":
      return dict.wealthCategoryFund;
    case "bond":
      return dict.wealthCategoryBond;
    case "insurance":
      return dict.wealthCategoryInsurance;
    case "estate":
      return dict.wealthCategoryEstate;
    case "gold":
      return dict.wealthCategoryGold;
    case "crypto":
      return dict.wealthCategoryCrypto;
    case "pension":
      return dict.wealthCategoryPension;
  }
}

// The target model picked on /w/alloc, shared with the home page so both
// show the same targets (the template keeps one wAllocModel for both).
const MODEL_KEY = "argus.wealth.allocModel";

export function loadModel(): AllocationModel {
  try {
    const v = localStorage.getItem(MODEL_KEY);
    if (v === "conserv" || v === "growth" || v === "balanced") return v;
  } catch {
    // storage blocked: fall through to the default
  }
  return "balanced";
}

// hasPickedModel tells "chose 平衡" from "never chose": loadModel defaults to
// balanced either way, and the choice only lives in this browser. The setup
// guide's allocation step counts as done once a model has been saved here.
export function hasPickedModel(): boolean {
  try {
    return localStorage.getItem(MODEL_KEY) !== null;
  } catch {
    return false;
  }
}

export function saveModel(m: AllocationModel) {
  try {
    localStorage.setItem(MODEL_KEY, m);
  } catch {
    // per-browser convenience only
  }
}
