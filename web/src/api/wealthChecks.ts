import { getJSON } from "./http";

// WealthChecks mirrors service.WealthChecks (wealth_checks.go): the home
// page's "本月該做的事". Items carry a code and figures, the text is ours.
export type CheckSeverity = "do" | "tip" | "data";

export type CheckCode =
  | "emergency"
  | "debtRatio"
  | "card"
  | "coverage"
  | "retirement"
  | "goals"
  | "alloc"
  | "savings"
  | "dependents"
  | "age"
  | "salary"
  | "stale";

export interface CheckItem {
  code: CheckCode;
  severity: CheckSeverity;
  link: string;
  value?: number;
  target?: number;
  gap?: number;
  count?: number;
  kind?: string;
  names?: string[];
}

export interface WealthChecks {
  asOf: string;
  new: boolean;
  rules: number;
  items: CheckItem[];
}

export function fetchWealthChecks(): Promise<WealthChecks> {
  return getJSON("/api/wealth/checks");
}
