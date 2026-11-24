import { describe, it, expect } from "vitest";
import { preRHint, returnMixTip } from "./WealthRetireView";
import { getDictionary } from "../i18n";
import type { WealthRetire } from "../api";

const dict = getDictionary("zh");
const plan = (p: Partial<WealthRetire>) => ({ preReturnPct: 3, ...p }) as WealthRetire;

describe("pre-retirement return source", () => {
  it("lists the blended mix in the chip's tip and says so in the drawer", () => {
    const p = plan({
      preReturnPct: 2.8,
      preReturnSource: "mix",
      preReturnMix: [
        { category: "cash", sharePct: 20, returnPct: 0 },
        { category: "fund", sharePct: 80, returnPct: 3.5 },
      ],
    });
    expect(returnMixTip(dict, p)).toBe(`退休前報酬依退休資產組成推算：${dict.wealthCategoryCash} 20%（0%）、${dict.wealthCategoryFund} 80%（3.5%）`);
    expect(preRHint(dict, p)).toContain("2.8%");
  });

  it("has no tip for the default or an override, and a default hint", () => {
    expect(returnMixTip(dict, plan({ preReturnSource: "default" }))).toBeUndefined();
    expect(returnMixTip(dict, plan({ preReturnSource: "override" }))).toBeUndefined();
    expect(preRHint(dict, plan({ preReturnSource: "default" }))).toBe(dict.wealthRetirePreRHintDefault.replace("{v}", "3"));
    expect(preRHint(dict, null)).toBeUndefined();
  });
});
