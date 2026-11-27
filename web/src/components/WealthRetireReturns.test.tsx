import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { RETURNS_DEFAULT, RetireReturns, returnOverrides, type ReturnsState } from "./WealthRetireReturns";
import { getDictionary } from "../i18n";
import type { RetireReturnRow, WealthRetire } from "../api";

vi.mock("../api", async (importOriginal) => {
  const real = await importOriginal<typeof import("../api")>();
  return { ...real, fetchWealthRetireLive: vi.fn().mockResolvedValue({ baseline: { achievementPct: 50 } }) };
});

const dict = getDictionary("zh");
afterEach(cleanup);

// The server's table: 600 ETF, 200 deposits, 200 pension.
const row = (c: RetireReturnRow["class"], amount: number, returnPct: number, movable: boolean): RetireReturnRow => ({ class: c, amount, returnPct, movable });
const mix: RetireReturnRow[] = [
  row("etf", 600, 4.5, true),
  row("bond", 0, 1.5, true),
  row("gold", 0, 0.5, true),
  row("cash", 200, -0.3, true),
  row("policy", 0, 0.3, false),
  row("pension", 200, 1.5, false),
  row("estate", 0, 1, false),
  row("crypto", 0, 0, false),
];

describe("returnOverrides", () => {
  it("sends nothing while the table and post mix are the server's own", () => {
    expect(returnOverrides(mix, RETURNS_DEFAULT)).toEqual({});
  });

  it("re-blends both returns once a class return is edited", () => {
    // pre: 0.6×6 + 0.2×−0.3 + 0.2×1.5 = 3.84 → 3.8.
    // post (conservative): 800 movable → 240 ETF, 400 bond, 40 gold, 120 cash; pension kept.
    // (240×6 + 400×1.5 + 40×0.5 − 120×0.3 + 200×1.5) / 1000 = 2.324 → 2.3.
    expect(returnOverrides(mix, { ...RETURNS_DEFAULT, edits: { etf: "6" } })).toEqual({ preR: "3.8", postR: "2.3" });
  });

  it("keeps the post return at the pre one for 維持不變", () => {
    // 0.6×4.5 − 0.06 + 0.3 = 2.94 → 2.9.
    expect(returnOverrides(mix, { ...RETURNS_DEFAULT, post: "keep" })).toEqual({ preR: "2.9", postR: "2.9" });
  });
});

describe("RetireReturns", () => {
  const plan = { preReturnPct: 2.9, postReturnPct: 2, returnMix: mix, baseline: { achievementPct: 40 } } as unknown as WealthRetire;
  function show(state: ReturnsState = RETURNS_DEFAULT) {
    const onChange = vi.fn();
    render(<RetireReturns dict={dict} plan={plan} current={plan} params={{}} state={state} onChange={onChange} onAskAI={vi.fn()} />);
    return onChange;
  }

  it("lists only the classes the pool holds, fixed ones tagged", () => {
    show();
    expect(screen.getByText(dict.rxClassPension)).not.toBeNull();
    expect(screen.getAllByText(dict.rxFixed)).toHaveLength(1); // pension
    expect(screen.queryByText(dict.rxClassPolicy)).toBeNull();
  });

  it("reports an edited return and switching mode", () => {
    const onChange = show();
    fireEvent.change(screen.getByLabelText(dict.rxReturnAria.replace("{name}", dict.rxClassEtf)), { target: { value: "5" } });
    expect(onChange).toHaveBeenLastCalledWith({ ...RETURNS_DEFAULT, edits: { etf: "5" } });
    fireEvent.click(screen.getByRole("button", { name: dict.rxModeManual }));
    expect(onChange).toHaveBeenLastCalledWith({ ...RETURNS_DEFAULT, mode: "manual" });
  });

  it("flags a trial mix that doesn't add up to 100%", () => {
    show();
    fireEvent.change(screen.getByLabelText(dict.rxTrialAria.replace("{name}", dict.rxClassEtf)), { target: { value: "90" } });
    expect(screen.getByRole("alert").textContent).toBe(dict.rxSumBad.replace("{n}", "115"));
  });

  it("hides the table and the trial in manual mode", () => {
    show({ ...RETURNS_DEFAULT, mode: "manual" });
    expect(screen.getByText(dict.rxManualNote)).not.toBeNull();
    expect(screen.queryByText(dict.rxWhatTitle)).toBeNull();
  });
});
