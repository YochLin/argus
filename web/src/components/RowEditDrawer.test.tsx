import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, fireEvent, cleanup, waitFor } from "@testing-library/react";
import { RowEditDrawer } from "./RowEditDrawer";
import { getDictionary } from "../i18n";
import * as api from "../api";
import type { WealthAsset } from "../api";

// The drawer talks to six endpoints; everything else in ../api (ApiError
// above all, which the drawer branches on) stays real.
vi.mock("../api", async (importOriginal) => {
  const real = await importOriginal<typeof import("../api")>();
  return {
    ...real,
    fetchWealthAssetHistory: vi.fn(),
    fetchWealthFX: vi.fn(),
    saveWealthAssetSnapshot: vi.fn(),
    deleteWealthAssetSnapshot: vi.fn(),
    updateWealthAsset: vi.fn(),
    archiveWealthAsset: vi.fn(),
    unarchiveWealthAsset: vi.fn(),
  };
});

const m = vi.mocked(api);
const dict = getDictionary("zh");

const TODAY = "2026-07-15";
const asset: WealthAsset = {
  id: 7,
  side: "asset",
  type: "deposit",
  name: "玉山活存",
  assetGroup: "liquid",
  venue: "玉山",
  currency: "TWD",
  source: "manual",
  createdAt: "2026-01-01",
  value: 120000,
  asOf: TODAY,
};

beforeEach(() => {
  m.fetchWealthAssetHistory.mockResolvedValue({
    today: TODAY,
    snapshots: [
      { date: TODAY, value: 120000, source: "manual" },
      { date: "2026-06-30", value: 100000, source: "import" },
    ],
  });
  m.fetchWealthFX.mockResolvedValue({ rates: { USD: 30 } });
  for (const f of [m.saveWealthAssetSnapshot, m.deleteWealthAssetSnapshot, m.updateWealthAsset, m.archiveWealthAsset, m.unarchiveWealthAsset]) {
    f.mockResolvedValue({ message: "ok" });
  }
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

async function setup(over: { asset?: WealthAsset; writable?: boolean } = {}) {
  const handlers = { onClose: vi.fn(), onChanged: vi.fn(), onUnauthorized: vi.fn() };
  render(<RowEditDrawer dict={dict} asset={over.asset ?? asset} writable={over.writable ?? true} {...handlers} />);
  await screen.findByText(dict.wealthEdLastRecord.replace("%s", TODAY)); // history arrived
  return handlers;
}

const amountInput = () => screen.getByLabelText(dict.wealthValue);
const dateInput = () => screen.getByLabelText("date");

describe("RowEditDrawer: what it shows", () => {
  it("shows the current value, the locked facts and the history, today's row editable", async () => {
    await setup();
    expect(screen.getByText("玉山活存", { selector: ".row-ed-name" })).not.toBeNull();
    expect(screen.getByText("NT$ 120,000", { selector: ".row-ed-current-val" })).not.toBeNull();
    expect(screen.getByText(`${dict.wealthSideAsset} · ${dict.wealthKindDeposit}`)).not.toBeNull();
    expect(screen.getByText(dict.wealthSrcManual)).not.toBeNull();
    // Only today's row carries 修正/刪除; the imported past one is plain history.
    expect(screen.getAllByRole("button", { name: dict.wealthEdFix })).toHaveLength(1);
    expect(screen.getAllByRole("button", { name: dict.wealthEdDel })).toHaveLength(1);
    expect(screen.getByText(dict.wealthEdTagToday)).not.toBeNull();
    expect(screen.getByText(dict.wealthEdTagHistory)).not.toBeNull();
  });

  it("is view-only when read-only: no form, no archive, a note instead", async () => {
    await setup({ writable: false });
    expect(screen.getByText(dict.wealthEdRoNote)).not.toBeNull();
    expect(screen.queryByRole("button", { name: dict.wealthEdAdd })).toBeNull();
    expect(screen.queryByRole("button", { name: dict.wealthEdArchive })).toBeNull();
    expect(screen.queryByRole("button", { name: dict.wealthEdSave })).toBeNull();
    expect(screen.queryByRole("button", { name: dict.wealthEdFix })).toBeNull();
    expect(screen.getByRole("heading", { name: dict.wealthEdTitle })).not.toBeNull();
  });

  it("an archived entry is read-only history with a way back", async () => {
    const archived = { ...asset, archivedAt: "2026-07-01 08:00:00" };
    const h = await setup({ asset: archived });
    expect(screen.getByRole("heading", { name: dict.wealthEdTitleArchived })).not.toBeNull();
    expect(screen.getByText(dict.wealthEdArchNote.replace("%s", "2026-07-01"))).not.toBeNull();
    expect(screen.queryByRole("button", { name: dict.wealthEdAdd })).toBeNull();
    expect(screen.queryByRole("button", { name: dict.wealthEdFix })).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: dict.wealthRestore }));
    await waitFor(() => expect(m.unarchiveWealthAsset).toHaveBeenCalledWith(7));
    await waitFor(() => expect(h.onClose).toHaveBeenCalled());
    expect(h.onChanged).toHaveBeenCalled();
  });
});

describe("RowEditDrawer: logging a value", () => {
  it("logs the typed amount for the chosen date and tells the page", async () => {
    const h = await setup();
    fireEvent.change(amountInput(), { target: { value: "130,500" } }); // the comma is stripped
    fireEvent.click(screen.getByRole("button", { name: dict.wealthEdAdd }));
    await waitFor(() => expect(m.saveWealthAssetSnapshot).toHaveBeenCalledWith(7, 130500, TODAY));
    await waitFor(() => expect(h.onChanged).toHaveBeenCalled());
    expect(m.fetchWealthAssetHistory).toHaveBeenCalledTimes(2); // reloaded after the write
    expect((amountInput() as HTMLInputElement).value).toBe(""); // form reset
    expect(h.onClose).not.toHaveBeenCalled(); // logging keeps the drawer open
  });

  it("refuses an empty amount, a future date and an already-recorded past date without calling the server", async () => {
    await setup();
    const add = () => fireEvent.click(screen.getByRole("button", { name: dict.wealthEdAdd }));

    add();
    expect(screen.getByText(dict.wealthEdErrAmount)).not.toBeNull();

    fireEvent.change(amountInput(), { target: { value: "5" } });
    fireEvent.change(dateInput(), { target: { value: "2026-07-16" } });
    add();
    expect(screen.getByText(dict.wealthEdErrFuture)).not.toBeNull();

    fireEvent.change(dateInput(), { target: { value: "2026-06-30" } });
    add();
    expect(screen.getByText(dict.wealthEdErrLocked)).not.toBeNull();

    expect(m.saveWealthAssetSnapshot).not.toHaveBeenCalled();
  });

  it("back-fills a past date that has no record", async () => {
    await setup();
    fireEvent.change(amountInput(), { target: { value: "90000" } });
    fireEvent.change(dateInput(), { target: { value: "2026-05-31" } });
    fireEvent.click(screen.getByRole("button", { name: dict.wealthEdAdd }));
    await waitFor(() => expect(m.saveWealthAssetSnapshot).toHaveBeenCalledWith(7, 90000, "2026-05-31"));
  });

  it("corrects today's record: 修正 loads it, the button turns into 更新, cancel puts it back", async () => {
    await setup();
    fireEvent.click(screen.getByRole("button", { name: dict.wealthEdFix }));
    expect((amountInput() as HTMLInputElement).value).toBe("120000");
    expect((dateInput() as HTMLInputElement).value).toBe(TODAY);
    expect(screen.getByText(dict.wealthEdFixTitle)).not.toBeNull();

    fireEvent.change(amountInput(), { target: { value: "121000" } });
    fireEvent.click(screen.getByRole("button", { name: dict.wealthEdUpdate }));
    await waitFor(() => expect(m.saveWealthAssetSnapshot).toHaveBeenCalledWith(7, 121000, TODAY));
    await screen.findByText(dict.wealthEdLogTitle); // back to "log new value"

    fireEvent.click(screen.getByRole("button", { name: dict.wealthEdFix }));
    fireEvent.click(screen.getByRole("button", { name: dict.wealthEdCancelFix }));
    expect(screen.getByText(dict.wealthEdLogTitle)).not.toBeNull();
    expect((amountInput() as HTMLInputElement).value).toBe("");
  });

  it("deletes today's record", async () => {
    const h = await setup();
    fireEvent.click(screen.getByRole("button", { name: dict.wealthEdDel }));
    await waitFor(() => expect(m.deleteWealthAssetSnapshot).toHaveBeenCalledWith(7, TODAY));
    await waitFor(() => expect(h.onChanged).toHaveBeenCalled());
  });

  it("previews a foreign amount in NT$ at the latest rate", async () => {
    await setup({ asset: { ...asset, currency: "USD", value: 1000 } });
    await waitFor(() => expect(m.fetchWealthFX).toHaveBeenCalled());
    expect(screen.getByText(dict.wealthEdHintFx.replace("%s", "USD"))).not.toBeNull();
    fireEvent.change(amountInput(), { target: { value: "100" } });
    expect(await screen.findByText("≈ NT$ 3,000")).not.toBeNull();
  });

  it("explains a server refusal of a closed day, and asks to log in again on a 401", async () => {
    const h = await setup();
    fireEvent.change(amountInput(), { target: { value: "5" } });

    m.saveWealthAssetSnapshot.mockRejectedValueOnce(new api.ApiError(409, "only today's value record can be changed"));
    fireEvent.click(screen.getByRole("button", { name: dict.wealthEdAdd }));
    expect(await screen.findByText(dict.wealthEdErrLocked)).not.toBeNull();

    m.saveWealthAssetSnapshot.mockRejectedValueOnce(new api.ApiError(401, "unauthorized"));
    fireEvent.click(screen.getByRole("button", { name: dict.wealthEdAdd }));
    await waitFor(() => expect(h.onUnauthorized).toHaveBeenCalledTimes(1));
  });
});

describe("RowEditDrawer: editing and archiving", () => {
  it("saves name, institution and group, then closes", async () => {
    const h = await setup();
    fireEvent.change(screen.getByDisplayValue("玉山活存"), { target: { value: "  新名稱 " } });
    fireEvent.change(screen.getByDisplayValue("玉山"), { target: { value: "台灣銀行" } });
    fireEvent.click(screen.getByRole("button", { name: dict.wealthGroupGrowth }));
    fireEvent.click(screen.getByRole("button", { name: dict.wealthEdSave }));
    await waitFor(() =>
      expect(m.updateWealthAsset).toHaveBeenCalledWith({ assetId: 7, name: "新名稱", assetGroup: "growth", venue: "台灣銀行" }),
    );
    await waitFor(() => expect(h.onClose).toHaveBeenCalled());
    expect(h.onChanged).toHaveBeenCalled();
  });

  it("won't save a blank name", async () => {
    await setup();
    fireEvent.change(screen.getByDisplayValue("玉山活存"), { target: { value: "   " } });
    fireEvent.click(screen.getByRole("button", { name: dict.wealthEdSave }));
    expect(screen.getByText(dict.wealthEdErrName)).not.toBeNull();
    expect(m.updateWealthAsset).not.toHaveBeenCalled();
  });

  it("a liability has no group to pick, and keeps its own on save", async () => {
    await setup({ asset: { ...asset, side: "liability", type: "loan", assetGroup: "hard" } });
    expect(screen.queryByRole("button", { name: dict.wealthGroupGrowth })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: dict.wealthEdSave }));
    await waitFor(() => expect(m.updateWealthAsset).toHaveBeenCalledWith(expect.objectContaining({ assetGroup: "hard" })));
  });

  it("archives only after the confirm dialog", async () => {
    const h = await setup();
    fireEvent.click(screen.getByRole("button", { name: dict.wealthEdArchive }));
    expect(m.archiveWealthAsset).not.toHaveBeenCalled();
    const dialog = screen.getByRole("alertdialog");
    expect(dialog).not.toBeNull();
    fireEvent.click(screen.getAllByRole("button", { name: dict.wealthArchive }).find((b) => dialog.contains(b))!);
    await waitFor(() => expect(m.archiveWealthAsset).toHaveBeenCalledWith(7));
    await waitFor(() => expect(h.onClose).toHaveBeenCalled());
  });
});
