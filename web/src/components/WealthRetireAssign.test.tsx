import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";
import { RetireAssignDrawer } from "./WealthRetireAssign";
import { WealthRetireView } from "./WealthRetireView";
import { FlashProvider } from "../flash";
import { getDictionary } from "../i18n";
import * as api from "../api";

vi.mock("../api", async (importOriginal) => {
  const real = await importOriginal<typeof import("../api")>();
  return {
    ...real,
    fetchWealthAssets: vi.fn(),
    fetchWealthGoals: vi.fn(),
    fetchWealthRetire: vi.fn(),
    fetchWealthRetireLive: vi.fn(),
    assignRetirementAssets: vi.fn(),
  };
});

const m = vi.mocked(api);
const dict = getDictionary("zh");

const asset = (id: number, name: string, valueTwd: number, type = "deposit") => ({ id, side: "asset", type, name, valueTwd, value: valueTwd, currency: "TWD" });
const retire = (over: object = {}) =>
  ({
    asOf: "2026-10-06", hasBirthYear: true, isSample: false, currentAge: 42, retirementAge: 60, retirementAgeOptions: [55, 60, 65],
    monthlySpend: 90000, monthlySpendOptions: [70000, 90000, 120000], monthlyContribution: 0, otherIncome: 0, pool: 0,
    horizonAge: 92, preReturnPct: 3, postReturnPct: 1, withdrawalRatePct: 4, need: 27000000, baseline: null, crash: null, lowReturn: null, path: [],
    ...over,
  }) as never;
const goalWith = (assets: { assetId: number; ratio: number }[]) =>
  ({ id: 5, name: "退休", kind: "retirement", targetAmount: 1, currency: "TWD", saved: 0, progressPct: 0, assets: assets.map((a) => ({ ...a, name: "x", value: 0 })) }) as never;

const onClose = vi.fn();
const onSaved = vi.fn();
const onNavigate = vi.fn();
const onUnauthorized = vi.fn();

beforeEach(() => {
  m.fetchWealthAssets.mockResolvedValue({ assets: [asset(1, "活存", 100000), asset(2, "勞退專戶", 500000, "pension")] as never });
  m.fetchWealthGoals.mockResolvedValue({ asOf: "", goals: [] });
  m.fetchWealthRetireLive.mockResolvedValue({ baseline: { achievementPct: 42.4 } } as never);
  m.assignRetirementAssets.mockResolvedValue(retire({ pool: 1 }));
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function open(over: { retire?: object; writable?: boolean } = {}) {
  render(
    <FlashProvider>
      <RetireAssignDrawer
        dict={dict}
        retire={over.retire ? (over.retire as never) : retire()}
        cfg={{}}
        writable={over.writable ?? true}
        onClose={onClose}
        onSaved={onSaved}
        onUnauthorized={onUnauthorized}
        onNavigate={onNavigate}
      />
    </FlashProvider>,
  );
}

const box = (name: RegExp) => screen.findByRole("checkbox", { name });
const save = () => screen.getByRole("button", { name: dict.wealthRetireAssignSave });

describe("the 指定退休資產 drawer", () => {
  it("suggests the pension account when nothing is assigned yet, and counts it", async () => {
    open();
    expect(await box(/勞退專戶/)).toHaveProperty("ariaChecked", "true");
    expect(screen.getByRole("checkbox", { name: /活存/ })).toHaveProperty("ariaChecked", "false");
    expect(screen.getByText(dict.wealthRetireAssignSuggested)).not.toBeNull();
    // its own value on the row, and the footer's earmarked total
    expect(screen.getAllByText("NT$500,000")).toHaveLength(2);
    expect(screen.getByText(dict.wealthRetireAssignSumCount.replace("{n}", "1"), { exact: false })).not.toBeNull();
  });

  it("says a retirement goal will be created, when there is none", async () => {
    open();
    await box(/活存/);
    expect(screen.getByText(dict.wealthRetireAssignNoGoal.replace("{age}", "60"))).not.toBeNull();
  });

  it("saves the whole ticked list, with the page's age and spend for a goal that doesn't exist yet", async () => {
    open();
    fireEvent.click(await box(/活存/));
    fireEvent.click(save());
    await waitFor(() => expect(m.assignRetirementAssets).toHaveBeenCalledTimes(1));
    expect(m.assignRetirementAssets).toHaveBeenCalledWith(dict.navWealthRetire, 60, 90000, [
      { assetId: 1, ratio: 1 },
      { assetId: 2, ratio: 1 },
    ]);
    await waitFor(() => expect(onSaved).toHaveBeenCalledTimes(1));
    expect(onClose).toHaveBeenCalled();
  });

  it("un-ticking removes: an empty list is saved as nothing set aside", async () => {
    open({ retire: retire({ pool: 500000, goal: goalWith([{ assetId: 2, ratio: 1 }]) }) });
    fireEvent.click(await box(/勞退專戶/));
    fireEvent.click(save());
    await waitFor(() => expect(m.assignRetirementAssets).toHaveBeenCalledWith(dict.navWealthRetire, 60, 90000, []));
  });

  it("only counts what another goal hasn't taken, and says so", async () => {
    m.fetchWealthGoals.mockResolvedValue({
      asOf: "",
      goals: [{ id: 9, name: "換屋頭期款", kind: "general", assets: [{ assetId: 1, ratio: 0.7, name: "活存", value: 70000 }] }] as never,
    });
    open();
    fireEvent.click(await box(/活存/));
    expect(screen.getByText(dict.wealthRetireAssignOtherPart.replace("{goal}", "換屋頭期款").replace("{pct}", "70%"))).not.toBeNull();
    expect(screen.getByText(dict.wealthRetireAssignCountsRemain.replace("{amount}", "NT$30,000").replace("{pct}", "30%"))).not.toBeNull();
    fireEvent.click(save());
    await waitFor(() => expect(m.assignRetirementAssets).toHaveBeenCalled());
    expect(m.assignRetirementAssets.mock.calls[0][3]).toContainEqual({ assetId: 1, ratio: 0.3 });
  });

  it("can't tick an asset another goal already holds in full", async () => {
    m.fetchWealthGoals.mockResolvedValue({
      asOf: "",
      goals: [{ id: 9, name: "子女教育金", kind: "general", assets: [{ assetId: 1, ratio: 1, name: "活存", value: 100000 }] }] as never,
    });
    open();
    const full = await box(/活存/);
    expect(full).toHaveProperty("disabled", true);
    expect(screen.getByText(dict.wealthRetireAssignOtherFull.replace("{goal}", "子女教育金"))).not.toBeNull();
  });

  it("with 依比例計入 on, a percentage over what's left blocks saving and says why", async () => {
    m.fetchWealthGoals.mockResolvedValue({
      asOf: "",
      goals: [{ id: 9, name: "換屋頭期款", kind: "general", assets: [{ assetId: 1, ratio: 0.7, name: "活存", value: 70000 }] }] as never,
    });
    open();
    fireEvent.click(await box(/活存/));
    fireEvent.click(screen.getByRole("switch", { name: dict.wealthRetireAssignAdvLabel }));
    const pct = screen.getByRole("textbox", { name: "活存 %" });
    fireEvent.change(pct, { target: { value: "40" } });
    expect(screen.getByText(dict.wealthRetireAssignOverWith.replace("{goal}", "換屋頭期款").replace("{total}", "110%").replace("{left}", "30%"))).not.toBeNull();
    expect(screen.getByText(dict.wealthRetireAssignOverMsg.replace("{n}", "1"))).not.toBeNull();
    expect(save()).toHaveProperty("disabled", true);

    fireEvent.change(pct, { target: { value: "30" } });
    expect(screen.queryByText(dict.wealthRetireAssignOverMsg.replace("{n}", "1"))).toBeNull();
    expect(save()).toHaveProperty("disabled", false);
    fireEvent.click(save());
    await waitFor(() => expect(m.assignRetirementAssets).toHaveBeenCalled());
    expect(m.assignRetirementAssets.mock.calls[0][3]).toContainEqual({ assetId: 1, ratio: 0.3 });
  });

  it("opens with the saved share, and starts in 依比例計入 when it's only part of what's left", async () => {
    open({ retire: retire({ pool: 50000, goal: goalWith([{ assetId: 1, ratio: 0.5 }]) }) });
    await box(/活存/);
    expect(screen.getByRole("switch", { name: dict.wealthRetireAssignAdvLabel })).toHaveProperty("ariaChecked", "true");
    expect((screen.getByRole("textbox", { name: "活存 %" }) as HTMLInputElement).value).toBe("50");
    // an assigned goal is neither re-created nor re-suggested
    expect(screen.queryByText(dict.wealthRetireAssignSuggested)).toBeNull();
    expect(screen.queryByText(dict.wealthRetireAssignNoGoal.replace("{age}", "60"))).toBeNull();
  });

  it("previews the funded % from the server's own projection for the ticked pool", async () => {
    open();
    await box(/勞退專戶/);
    await waitFor(() => expect(m.fetchWealthRetireLive).toHaveBeenCalled());
    expect(m.fetchWealthRetireLive.mock.calls[0][0].get("pool")).toBe("500000");
    expect(await screen.findByText("42%")).not.toBeNull();
  });

  it("is view-only on a read-only server", async () => {
    open({ writable: false });
    await box(/活存/);
    expect(screen.getByText(dict.wealthRetireAssignReadOnly)).not.toBeNull();
    expect(save()).toHaveProperty("disabled", true);
  });

  it("points an empty balance sheet at the page that adds assets", async () => {
    m.fetchWealthAssets.mockResolvedValue({ assets: [] });
    open();
    fireEvent.click(await screen.findByText(dict.wealthRetireAssignGoBalance));
    expect(onClose).toHaveBeenCalled();
    expect(onNavigate).toHaveBeenCalledWith("/w/balance");
  });

  it("asks to log in again on a 401 and retries the save", async () => {
    m.assignRetirementAssets.mockRejectedValueOnce(new api.ApiError(401, "unauthorized"));
    open();
    await box(/活存/);
    fireEvent.click(save());
    await waitFor(() => expect(onUnauthorized).toHaveBeenCalledTimes(1));
    expect(onSaved).not.toHaveBeenCalled();
  });
});

describe("the retirement page around it", () => {
  const view = (writable = true) =>
    render(
      <FlashProvider>
        <WealthRetireView dict={dict} writable={writable} onUnauthorized={vi.fn()} onNavigate={vi.fn()} />
      </FlashProvider>,
    );
  const base = { achievementPct: 0, funded: false, gapAmount: 27000000, projectedAtRetirement: 0, depletionAge: 61 };
  const withScenarios = { crash: base, lowReturn: base, path: [{ year: 2026, balance: 0 }, { year: 2027, balance: 0 }] };

  beforeEach(() => {
    m.fetchWealthAssets.mockResolvedValue({ assets: [asset(1, "活存", 100000)] as never });
  });

  it("with nothing assigned, says why and reads —, never a 0% funded rate", async () => {
    m.fetchWealthRetire.mockResolvedValue(retire({ pool: 0, baseline: base, ...withScenarios }));
    view();
    expect(await screen.findByText(dict.wealthRetireAssignEmptyTitle)).not.toBeNull();
    expect(screen.getByRole("button", { name: new RegExp(dict.wealthRetireAssignBtn) })).not.toBeNull();
    expect(screen.queryByText("0%")).toBeNull();
    expect(screen.queryByText("61 歲")).toBeNull(); // the depletion age rests on the same missing pool
    expect(screen.queryByText(dict.wealthRetireScenarioCrash)).toBeNull(); // so do the scenario rows under the chart
  });

  it("once assets are assigned, drops the card, shows the rate, and offers 調整指定", async () => {
    m.fetchWealthRetire.mockResolvedValue(
      retire({ pool: 5000000, baseline: { ...base, achievementPct: 35, gapAmount: 100, depletionAge: 78 }, goal: goalWith([{ assetId: 2, ratio: 1 }]) }),
    );
    view();
    expect(await screen.findByText("35%")).not.toBeNull();
    expect(screen.queryByText(dict.wealthRetireAssignEmptyTitle)).toBeNull();
    expect(screen.getByText("78 歲")).not.toBeNull();
    expect(screen.getByText(dict.wealthRetireAssignedCount.replace("{n}", "1"), { exact: false })).not.toBeNull();
    fireEvent.click(screen.getByText(`${dict.wealthRetireAdjustBtn} ›`));
    expect(await screen.findByRole("dialog", { name: dict.wealthRetireAssignTitle })).not.toBeNull();
  });

  it("opens the drawer from the empty card", async () => {
    m.fetchWealthRetire.mockResolvedValue(retire({ pool: 0, baseline: base }));
    view();
    fireEvent.click(await screen.findByRole("button", { name: new RegExp(dict.wealthRetireAssignBtn) }));
    expect(await screen.findByRole("dialog", { name: dict.wealthRetireAssignTitle })).not.toBeNull();
  });

  it("no longer offers a typed 退休專用資產 in the settings drawer — it's set by assigning", async () => {
    m.fetchWealthRetire.mockResolvedValue(retire({ pool: 0, baseline: base }));
    view();
    fireEvent.click(await screen.findByRole("button", { name: new RegExp(dict.wealthRetireSettingsButton) }));
    expect(screen.getByText(dict.wealthRetireAssignPoolHint)).not.toBeNull();
    expect(document.getElementById("retcfg-pool")).toBeNull();
  });
});
