import { getJSON } from "./http";
import type { AssetGroup } from "./wealthAssets";

// --- Phase 9 PR2: balance sheet / health metrics / debt payoff ---
// Mirrors internal/web/wealth_balance.go.

export interface BalanceSheetItem {
  assetId?: number;
  name: string;
  venue?: string;
  currency: string;
  valueTwd: number;
  type: string; // the asset's own type, or "equity_us"/"equity_tw" for the virtual row
  category: string; // AllocCategory — the item's dot color, wealthCategory.ts
  source: "manual" | "import" | "sync";
  staleDays?: number; // set only past the 90-day threshold (manual/import only)
}

export interface AssetGroupRow {
  group: AssetGroup;
  marketValue: number;
  pctOfAssets: number | null;
  assets: BalanceSheetItem[];
}

export interface LiabilityDetail {
  assetId: number;
  name: string;
  venue?: string;
  currency: string;
  valueTwd: number;
  ratePct: number | null;
  remainingMonths: number | null;
  minPayment: number | null;
  source: "manual" | "import" | "sync";
  type: string; // the liability's own type, e.g. "loan" / "credit_card"
  staleDays?: number;
}

export interface LiabGroupRow {
  kind: "short" | "long";
  marketValue: number;
  pctOfLiabilities: number | null;
  liabilities: LiabilityDetail[];
}

export interface QuarterPoint {
  quarter: string;
  netWorth: number | null;
}

// BalanceSheet mirrors wealth_balance.go's balanceSheetResponse — every
// ratio/total is null until there's enough data (fresh install, salary not
// set, or an FX rate missing for today); render "—", never 0.
export interface BalanceSheet {
  asOf: string;
  totalAssets: number | null;
  totalLiabilities: number | null;
  netWorth: number | null;
  debtRatioPct: number | null;
  liquidityMonths: number | null;
  savingsRatePct: number | null;
  expenseRatioPct: number | null;
  monthlySalary: number | null;
  assetGroups: AssetGroupRow[];
  liabilities: LiabilityDetail[];
  liabGroups: LiabGroupRow[];
  quarterlyTrend: QuarterPoint[];
}

export function fetchWealthBalance(): Promise<BalanceSheet> {
  return getJSON("/api/wealth/balance");
}
