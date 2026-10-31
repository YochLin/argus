import { describe, expect, it } from "vitest";
import type { DropEvent } from "../api";
import { sameCauseHistory } from "./DropPanel";

function drop(start: string, over: Partial<DropEvent>): DropEvent {
  return {
    kind: "streak", start, end: start, pct: -8, gapPct: null, preClose: 100, low: 90, recoverBars: null,
    fwd10Pct: null, indexPct: null, summary: "", cause: "earn", secondary: [], confidence: "mid", loggedOn: start, news: [],
    ...over,
  };
}

describe("sameCauseHistory", () => {
  it("averages the other drops of the same main cause", () => {
    const of = drop("2026-05-01", { recoverBars: 99, fwd10Pct: 99 });
    const h = sameCauseHistory(
      [
        drop("2026-01-01", { recoverBars: 4, fwd10Pct: 2 }),
        drop("2026-02-01", { recoverBars: 7, fwd10Pct: -5 }),
        drop("2026-03-01", {}),
        drop("2026-04-01", { cause: "macro", recoverBars: 1, fwd10Pct: 50 }),
        of,
      ],
      of,
    );
    expect(h).toEqual({ n: 3, unrecovered: 1, avgRecover: 6, avgFwd10: -1.5, lowSample: true });
  });

  it("has no averages with nothing to average", () => {
    const of = drop("2026-05-01", {});
    const h = sameCauseHistory([drop("2026-01-01", {}), of], of);
    expect(h).toEqual({ n: 1, unrecovered: 1, avgRecover: null, avgFwd10: null, lowSample: true });
  });

  it("stops flagging a low sample at four", () => {
    const of = drop("2026-05-01", {});
    const others = ["01", "02", "03", "04"].map((m) => drop(`2026-${m}-01`, {}));
    expect(sameCauseHistory([...others, of], of).lowSample).toBe(false);
  });
});
