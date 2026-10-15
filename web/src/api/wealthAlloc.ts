import { getJSON } from "./http";
import type { AllocationModel } from "./wealthAssets";

// --- Phase 9 PR4: allocation & rebalance (/w/alloc) ---
// Mirrors internal/web/wealth_alloc.go. /w/alloc uses its own finer
// nine-category taxonomy (assets.AllocCategories, docs/phase-9-asset-
// platform.md §8.5) rather than the four-bucket AssetGroup the rest of the
// wealth pages (home/balance-sheet) key off — the two coexist on purpose,
// see allocation.go's doc comments.
export type AllocCategory =
  | "cash"
  | "equity"
  | "fund"
  | "bond"
  | "insurance"
  | "estate"
  | "gold"
  | "crypto"
  | "pension";

export interface WealthAllocRow {
  category: AllocCategory;
  marketValue: number;
  currentPct: number;
  targetPct: number;
  deviationPt: number;
  venue?: string;
}

export interface WealthAllocOrder {
  category: AllocCategory;
  side: "buy" | "sell";
  amount: number;
  deviationPt: number;
  assetName?: string;
  venue?: string;
}

export interface WealthAllocLockedRow {
  category: AllocCategory;
  marketValue: number;
  deviationPt: number;
}

export interface CurrencyExposureRow {
  currency: string;
  pct: number;
}

export interface ConcentrationWarning {
  ticker: string;
  market: string;
  pctOfEquity: number;
  pctOfAssets: number;
}

// WealthAlloc mirrors wealth_alloc.go's allocResponse — every total/list is
// nil/empty until there's at least one priced asset (§8.17.1).
export interface WealthAlloc {
  asOf: string;
  model: AllocationModel;
  totalAssets: number | null;
  allocation: WealthAllocRow[];
  orders: WealthAllocOrder[];
  rebalTotal: number;
  locked: WealthAllocLockedRow[];
  riskPct: number | null;
  riskTargetLow: number | null;
  riskTargetHigh: number | null;
  currencyExposure: CurrencyExposureRow[];
  concentration: ConcentrationWarning[];
}

export function fetchWealthAlloc(model: AllocationModel): Promise<WealthAlloc> {
  return getJSON(`/api/wealth/alloc?model=${model}`);
}

export interface DebtPayoffPlan {
  order: string[];
  months: number;
  totalInterest: number;
  monthsSaved: number;
}

export interface DebtPayoffResult {
  loans: string[];
  snowball: DebtPayoffPlan;
  avalanche: DebtPayoffPlan;
  interestDifference: number;
}

export function fetchWealthDebtPayoff(extraMonthly: number): Promise<DebtPayoffResult> {
  return getJSON(`/api/wealth/debt-payoff?extra=${extraMonthly}`);
}
