import { getJSON, postJSON } from "./http";

// InsuranceKind/InsurancePerPeriod/InsuranceCoverageRow/InsurancePolicyItem/
// WealthInsure mirror wealth_insure.go's response shapes (Phase 9 波次3 PR8,
// §8.6/§8.16.1) — the six-coverage-kind have/need gap table plus the policy
// list. perPeriod is server-derived from kind (never independently chosen),
// so it's always one of the fixed pairs below.
export type InsuranceKind = "life" | "accident" | "ci" | "cancer" | "disability" | "hospital";
export type InsurancePerPeriod = "" | "month" | "day";

export interface InsuranceCoverageRow {
  kind: InsuranceKind;
  perPeriod?: InsurancePerPeriod;
  have: number;
  need: number | null;
  gap: number | null;
  pctOfNeed: number | null;
}

export interface InsurancePolicyItem {
  assetId: number;
  name: string;
  insurer: string;
  kind: InsuranceKind;
  perPeriod?: InsurancePerPeriod;
  amount: number;
  annualPremium: number | null;
  premiumYears: number | null;
  insured?: string;
  source: string;
}

export interface WealthInsure {
  asOf: string;
  hasProfile: boolean;
  rows: InsuranceCoverageRow[];
  policies: InsurancePolicyItem[];
  count: number;
  premium: number;
  premSharePct: number | null;
  worstKind?: InsuranceKind;
  worstGap: number;
  worstPct: number;
  totalGapLumpSum: number;
}

export function fetchWealthInsure(): Promise<WealthInsure> {
  return getJSON("/api/wealth/insure");
}

export interface NewInsurancePolicy {
  insurer: string;
  name: string;
  kind: InsuranceKind;
  amount: number;
  annualPremium?: number;
  premiumYears?: number;
  insured?: string;
}

export function createInsurancePolicy(p: NewInsurancePolicy): Promise<{ id: number }> {
  return postJSON("/api/wealth/insure", p);
}
