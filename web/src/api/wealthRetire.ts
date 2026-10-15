import { getJSON, postJSON } from "./http";
import type { Goal } from "./wealthGoals";

// RetirementScenario/WealthRetire mirror wealth_retire.go's response shapes
// (Phase 9 波次3 PR7, §8.7/§10.2②) — the real-return projection off the
// retirement-earmarked pool (the wealthGoals.ts row with kind "retirement"),
// plus three named scenarios instead of a Monte Carlo distribution.
export interface RetirementScenario {
  projectedAtRetirement: number;
  achievementPct: number;
  gapAmount: number;
  funded: boolean;
  depletionAge?: number;
}

export interface RetirementPathPoint {
  year: number;
  balance: number;
}

export interface WealthRetire {
  asOf: string;
  hasBirthYear: boolean;
  isSample: boolean;
  currentAge: number;
  retirementAge: number;
  retirementAgeOptions: number[];
  monthlySpend: number;
  monthlySpendOptions: number[];
  monthlyContribution: number;
  otherIncome: number;
  pool: number | null;
  horizonAge: number;
  preReturnPct: number;
  postReturnPct: number;
  withdrawalRatePct: number;
  retirementYear?: number;
  need: number | null;
  baseline: RetirementScenario | null;
  crash: RetirementScenario | null;
  lowReturn: RetirementScenario | null;
  path: RetirementPathPoint[];
  goal?: Goal;
}

export function fetchWealthRetire(): Promise<WealthRetire> {
  return getJSON("/api/wealth/retire");
}

// fetchWealthRetireLive is the settings drawer's what-if preview — the same
// GET endpoint with the drawer's overrides as a query string, never
// persisted server-side (see wealth_retire.go's retirementLiveOverrides).
export function fetchWealthRetireLive(params: URLSearchParams): Promise<WealthRetire> {
  return getJSON(`/api/wealth/retire?${params.toString()}`);
}

export function saveWealthRetire(name: string, retirementAge: number, monthlySpend: number): Promise<WealthRetire> {
  return postJSON("/api/wealth/retire", { name, retirementAge, monthlySpend });
}

// assignRetirementAssets saves the 指定退休資產 drawer: `assets` is the whole
// list set aside for retirement (anything left out is un-assigned). name/age/
// spend are only used when there is no retirement goal yet, so one gets created
// from the page's current settings. Returns the recomputed plan.
export function assignRetirementAssets(
  name: string,
  retirementAge: number,
  monthlySpend: number,
  assets: { assetId: number; ratio: number }[],
): Promise<WealthRetire> {
  return postJSON("/api/wealth/retire/assign", { name, retirementAge, monthlySpend, assets });
}
