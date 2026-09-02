import { describe, it, expect } from "vitest";
import { getDictionary } from "../i18n";
import { runInput, runKindLabel } from "./LlmRunsView";

// A price_event run's input (internal/bot/pipeline.go recordPriceEventLLMRun)
// has none of recommendationInputs' three arrays; spreading them unguarded is
// what used to blow up the whole detail page with a TypeError.
const priceEvent = {
  ticker: "3037",
  gapPct: -1.65,
  changePct: 0.2,
  cumulativePct: -16.12,
  news: [{ Headline: "h", Summary: "", Source: "s", URL: "", PublishedAt: "2026-09-02" }],
};

describe("runInput", () => {
  it("maps a price_event payload onto the recommendation shape", () => {
    const got = runInput(priceEvent);
    expect(got.watchlist).toEqual([]);
    expect(got.candidates).toEqual([]);
    expect(got.marketNews).toHaveLength(1);
  });

  it("passes a recommendation payload through untouched", () => {
    const got = runInput({ watchlist: [{}], candidates: [{}, {}], marketNews: [] });
    expect(got.watchlist).toHaveLength(1);
    expect(got.candidates).toHaveLength(2);
    expect(got.marketNews).toEqual([]);
  });
});

describe("runKindLabel", () => {
  it("does not label a price_event run as a daily report", () => {
    const dict = getDictionary("zh");
    expect(runKindLabel("price_event", dict)).toBe(dict.llmRunPriceEvent);
    expect(runKindLabel("daily_report", dict)).toBe(dict.llmRunDailyReport);
    expect(runKindLabel("recommend", dict)).toBe(dict.llmRunRecommend);
  });
});
