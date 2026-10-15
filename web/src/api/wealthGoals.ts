import type { TradeResponse } from "./common";
import { getJSON, postJSON } from "./http";

// GoalKind/GoalAssetItem/Goal mirror wealth_goals.go's response shapes
// (Phase 9 波次3 PR6, §8.8/§9.4) — the retirement row (kind "retirement") is
// the same table, PR7 owns filling in its targetAmount from the retirement
// calc.
export type GoalKind = "retirement" | "general";
export type GoalStatus = "ahead" | "onTrack" | "behind" | "";

export interface GoalAssetItem {
  assetId: number;
  name: string;
  venue?: string;
  ratio: number;
  value: number | null;
}

// Goal mirrors wealth_goals.go's goalItem — saved/progressPct are null
// together when any earmarked asset's currency couldn't be priced to TWD
// (§8.17.1's "don't fabricate a number" rule, same as /w/cash's totals).
export interface Goal {
  id: number;
  name: string;
  kind: GoalKind;
  targetAmount: number;
  currency: string;
  targetDate?: string;
  note?: string;
  saved: number | null;
  progressPct: number | null;
  status?: GoalStatus;
  // markPct is where the "expected progress" tick renders on the progress
  // bar (§8.9 point 5's 應有進度標記) — the same straight-line expectation
  // status was classified against, unset together with status when there's
  // no targetDate.
  markPct?: number;
  // monthlyContribution is set only for the kind="retirement" row (read from
  // the same profile.retirement_monthly_contribution setting /w/retire
  // uses) — general goals have no monthly-contribution column (§8.8/§9.2).
  monthlyContribution?: number | null;
  // startYear is the drawer's 起始年 (0/absent = never typed).
  startYear?: number;
  // projectedAtRetirement/retirementAge: retirement row only (birth year set)
  // — the card subtitle's "60 歲屆退推估 …" and its 預計達成 age.
  projectedAtRetirement?: number;
  retirementAge?: number;
  assets: GoalAssetItem[];
}

export interface WealthGoals {
  asOf: string;
  goals: Goal[];
}

// fetchWealthFX returns TWD per one unit of each display currency the server
// could price; a currency missing from the map is unavailable.
export function fetchWealthFX(): Promise<{ rates: Record<string, number> }> {
  return getJSON("/api/wealth/fx");
}

export function fetchWealthGoals(): Promise<WealthGoals> {
  return getJSON("/api/wealth/goals");
}

export interface NewGoal {
  name: string;
  kind?: GoalKind;
  targetAmount: number;
  currency?: string;
  targetDate?: string;
  note?: string;
  // The drawer's hand-typed fields (migration 34); omit to leave unset.
  savedAmount?: number;
  monthlyContribution?: number;
  startYear?: number;
}

export function createWealthGoal(g: NewGoal): Promise<{ id: number }> {
  return postJSON("/api/wealth/goals", g);
}

export function updateWealthGoal(id: number, g: NewGoal): Promise<TradeResponse> {
  return postJSON("/api/wealth/goals/update", { id, ...g });
}

export function deleteWealthGoal(id: number): Promise<TradeResponse> {
  return postJSON("/api/wealth/goals/delete", { id });
}

export function setWealthGoalEarmark(goalId: number, assetId: number, ratio: number): Promise<TradeResponse> {
  return postJSON("/api/wealth/goals/earmark", { goalId, assetId, ratio });
}
