import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";
import { AddAssetModal } from "./WealthHomeView";
import { FlashProvider } from "../flash";
import { getDictionary } from "../i18n";
import * as api from "../api";

vi.mock("../api", async (importOriginal) => {
  const real = await importOriginal<typeof import("../api")>();
  return { ...real, createWealthAsset: vi.fn(), fetchWealthAssets: vi.fn() };
});

const m = vi.mocked(api);
const dict = getDictionary("zh");

const onClose = vi.fn();
const onSuccess = vi.fn();
const onNavigate = vi.fn();

beforeEach(() => {
  m.fetchWealthAssets.mockResolvedValue({ assets: [] });
  m.createWealthAsset.mockResolvedValue({} as never);
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function open(withNavigate = true) {
  render(
    <FlashProvider>
      <AddAssetModal dict={dict} onClose={onClose} onSuccess={onSuccess} onUnauthorized={vi.fn()} onNavigate={withNavigate ? onNavigate : undefined} />
    </FlashProvider>,
  );
}

const pick = (label: string) => fireEvent.click(screen.getByText(label));
const type = (el: HTMLElement, value: string) => fireEvent.change(el, { target: { value } });
const save = () => fireEvent.click(screen.getByRole("button", { name: dict.wealthAddSave }));

describe("step 1", () => {
  it("says what each type is for, and that stocks don't go here", () => {
    open();
    expect(screen.getByText(dict.wealthAddPickHint)).not.toBeNull();
    expect(screen.getByText(dict.wealthKindNoteDeposit)).not.toBeNull();
    expect(screen.getByText(dict.wealthKindNoteLoan)).not.toBeNull();
    expect(screen.getByText(dict.wealthKindNotePension)).not.toBeNull();
  });

  it("hands cash flow, policies and CSV over to their own page", () => {
    for (const [label, path] of [
      [dict.wealthKindFlow, "/w/cash"],
      [dict.wealthKindInsurance, "/w/insure"],
      [dict.wealthKindImport, "/w/import"],
    ]) {
      open();
      pick(label);
      expect(onNavigate).toHaveBeenLastCalledWith(path);
      expect(onClose).toHaveBeenCalled();
      cleanup();
    }
  });

  it("doesn't offer the hand-offs when it can't navigate", () => {
    open(false);
    expect(screen.queryByText(dict.wealthKindImport)).toBeNull();
    expect(screen.queryByText(dict.wealthKindFlow)).toBeNull();
  });

  it("no longer creates a bare policy asset (no coverage rows) here", () => {
    open();
    pick(dict.wealthKindInsurance);
    expect(screen.queryByPlaceholderText(dict.wealthPhAssetName)).toBeNull();
  });
});

describe("an asset form", () => {
  it("has example text, and explains the chosen group", () => {
    open();
    pick(dict.wealthKindDeposit);
    expect(screen.getByPlaceholderText(dict.wealthPhAssetName)).not.toBeNull();
    expect(screen.getByText(/隨時可動用/)).not.toBeNull();
    fireEvent.click(screen.getByText(dict.wealthGroupGrowth));
    expect(screen.getByText(/0050 歸股票/)).not.toBeNull();
    expect(screen.queryByText(/隨時可動用/)).toBeNull();
  });

  it("has one institution field, filed as venue and, for a deposit, as bank", async () => {
    open();
    pick(dict.wealthKindDeposit);
    type(screen.getByPlaceholderText(dict.wealthPhAssetName), "玉山活存");
    type(screen.getByPlaceholderText("0"), "500000");
    type(screen.getByPlaceholderText(dict.wealthPhInst), "玉山銀行");
    expect(screen.queryByText(dict.wealthBank)).toBeNull();
    save();
    await waitFor(() => expect(m.createWealthAsset).toHaveBeenCalledTimes(1));
    expect(m.createWealthAsset).toHaveBeenCalledWith(
      expect.objectContaining({ side: "asset", type: "deposit", name: "玉山活存", assetGroup: "liquid", venue: "玉山銀行", initialValue: 500000, deposit: { bank: "玉山銀行", accountNote: undefined } }),
    );
  });

  it("confirms with a toast and closes", async () => {
    open();
    pick(dict.wealthKindDeposit);
    type(screen.getByPlaceholderText(dict.wealthPhAssetName), "玉山活存");
    type(screen.getByPlaceholderText("0"), "1");
    save();
    expect(await screen.findByText(`玉山活存${dict.wealthAddedToast}`)).not.toBeNull();
    expect(onSuccess).toHaveBeenCalledTimes(1);
  });
});

describe("a loan form", () => {
  it("has no group to pick, labels the amount as what's left, and says how short/long-term is told", () => {
    open();
    pick(dict.wealthKindLoan);
    expect(screen.queryByText(dict.wealthGroupLabel)).toBeNull();
    expect(screen.getByText(dict.wealthFBalance)).not.toBeNull();
    expect(screen.getByText(dict.wealthLoanHint)).not.toBeNull();
    expect(screen.getByPlaceholderText(dict.wealthPhLoanName)).not.toBeNull();
  });

  it("files the lender as venue and keeps its default group", async () => {
    open();
    pick(dict.wealthKindLoan);
    type(screen.getByPlaceholderText(dict.wealthPhLoanName), "房貸");
    type(screen.getByPlaceholderText("0"), "3000000");
    type(screen.getByPlaceholderText(dict.wealthPhInst), "台灣銀行");
    save();
    await waitFor(() => expect(m.createWealthAsset).toHaveBeenCalledTimes(1));
    expect(m.createWealthAsset).toHaveBeenCalledWith(
      expect.objectContaining({ side: "liability", type: "loan", assetGroup: "hard", venue: "台灣銀行", loan: expect.objectContaining({ lender: "台灣銀行" }) }),
    );
  });
});

describe("required fields", () => {
  it("shows what's missing on save instead of silently disabling it, and clears as it's fixed", () => {
    open();
    pick(dict.wealthKindDeposit);
    expect(screen.getByText(dict.wealthAddHintLive)).not.toBeNull();
    save();
    expect(m.createWealthAsset).not.toHaveBeenCalled();
    expect(screen.getByText(dict.wealthErrRequired)).not.toBeNull();
    expect(screen.getByText(dict.wealthErrAmount)).not.toBeNull();
    expect(screen.getByText(dict.wealthErrBlocked)).not.toBeNull();
    type(screen.getByPlaceholderText(dict.wealthPhAssetName), "活存");
    expect(screen.queryByText(dict.wealthErrRequired)).toBeNull();
    expect(screen.getByText(dict.wealthErrAmount)).not.toBeNull();
    type(screen.getByPlaceholderText("0"), "10");
    expect(screen.queryByText(dict.wealthErrBlocked)).toBeNull();
  });

  it("wants an amount above 0", () => {
    open();
    pick(dict.wealthKindDeposit);
    type(screen.getByPlaceholderText(dict.wealthPhAssetName), "活存");
    type(screen.getByPlaceholderText("0"), "-5");
    save();
    expect(m.createWealthAsset).not.toHaveBeenCalled();
    expect(screen.getByText(dict.wealthErrAmount)).not.toBeNull();
  });
});

describe("duplicate warning", () => {
  const rows = [
    { id: 1, side: "asset", name: "玉山活存" },
    { id: 2, side: "liability", name: "房貸" },
  ];

  it("warns on a similar name among the same side, and still lets you save", async () => {
    m.fetchWealthAssets.mockResolvedValue({ assets: rows as never });
    open();
    pick(dict.wealthKindDeposit);
    await waitFor(() => expect(m.fetchWealthAssets).toHaveBeenCalled());
    type(screen.getByPlaceholderText(dict.wealthPhAssetName), "玉山活存二號");
    expect(await screen.findByText(`${dict.wealthWarnDupMsg}玉山活存`)).not.toBeNull();
    type(screen.getByPlaceholderText("0"), "1");
    save();
    await waitFor(() => expect(m.createWealthAsset).toHaveBeenCalledTimes(1));
  });

  it("doesn't match across assets and liabilities, or on a 1-2 character overlap", async () => {
    m.fetchWealthAssets.mockResolvedValue({ assets: rows as never });
    open();
    pick(dict.wealthKindDeposit);
    await waitFor(() => expect(m.fetchWealthAssets).toHaveBeenCalled());
    const name = screen.getByPlaceholderText(dict.wealthPhAssetName);
    type(name, "房貸"); // a liability's name
    expect(screen.queryByText(dict.wealthWarnDup)).toBeNull();
    type(name, "玉山");
    expect(screen.queryByText(dict.wealthWarnDup)).toBeNull(); // 2 chars: too short to count as "contains"
  });
});
