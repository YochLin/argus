import type { TradeResponse } from "./common";
import { getJSON, postJSON } from "./http";

export interface WealthProfile {
  annualSalary: number | null;
  // birthYear/retirementMonthlyContribution back the retirement page (Phase
  // 9 波次3 PR7) — two more "個人參數" the platform can't derive on its own.
  birthYear: number | null;
  retirementMonthlyContribution: number | null;
  // dependents/youngestChildAge/spouseHasIncome back the insurance page's
  // need formula (Phase 9 波次3 PR8, §8.16.1). dependents === 0 is a real,
  // explicitly-saved "no dependents" — check for null, not falsiness, to
  // tell "not set yet" apart from that.
  dependents: number | null;
  youngestChildAge: number | null;
  spouseHasIncome: boolean | null;
}

export function fetchWealthProfile(): Promise<WealthProfile> {
  return getJSON("/api/wealth/profile");
}

// saveWealthProfile takes a partial patch — every mini setup card on
// WealthProfile's fields (the balance sheet's salary form, the retirement
// page's birth-year/contribution form, the insurance page's dependents/
// spouse form) each save just their own field without clobbering the others.
export function saveWealthProfile(patch: {
  annualSalary?: number;
  birthYear?: number;
  retirementMonthlyContribution?: number;
  dependents?: number;
  youngestChildAge?: number;
  spouseHasIncome?: boolean;
}): Promise<TradeResponse> {
  return postJSON("/api/wealth/profile", patch);
}
