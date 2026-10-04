import type { AllocCategory, AllocationModel } from "./api";
import type { Dictionary } from "./i18n";

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
