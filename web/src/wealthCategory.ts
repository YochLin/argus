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

export function saveModel(m: AllocationModel) {
  try {
    localStorage.setItem(MODEL_KEY, m);
  } catch {
    // per-browser convenience only
  }
}
