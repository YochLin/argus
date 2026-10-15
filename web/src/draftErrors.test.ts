import { describe, expect, it } from "vitest";
import type { AssistantDraft } from "./api";
import { draftError } from "./components/AssistantDrafts";
import { getDictionary } from "./i18n";

// That every server code has a string is checked from the Go side
// (internal/service's TestDraftErrorCodesTranslated).

const draft = (over: Partial<AssistantDraft>) =>
  ({ id: 1, kind: "goal", payload: {}, status: "error", missing: [], createdAt: "", ...over }) as AssistantDraft;

describe("draftError", () => {
  it("translates codes and fills args", () => {
    const d = draft({ error: "x", errors: [{ code: "TargetYearRange", args: ["2026", "2126"] }, { code: "AmountPositive" }] });
    expect(draftError(getDictionary("zh"), d)).toBe("目標年份要在 2026～2126 之間 / 金額要大於 0");
  });

  it("falls back to the server's text", () => {
    expect(draftError(getDictionary("zh"), draft({ error: "boom", failed: true }))).toBe("boom");
    expect(draftError(getDictionary("zh"), draft({ error: "raw", errors: [{ code: "Nope" }] }))).toBe("raw");
  });
});
