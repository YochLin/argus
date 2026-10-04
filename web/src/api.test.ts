import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import {
  ApiError,
  archiveWealthAsset,
  createWealthAsset,
  deleteWealthAssetSnapshot,
  importWealthCSV,
  onNetWorthChange,
  saveWealthAssetSnapshot,
  unarchiveWealthAsset,
  updateWealthAsset,
} from "./api";

function answer(status: number, body: object = {}) {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: status < 400, status, json: async () => body }));
}

let changed: ReturnType<typeof vi.fn<() => void>>;
let stop: () => void;

beforeEach(() => {
  changed = vi.fn<() => void>();
  stop = onNetWorthChange(changed);
  answer(200, { id: 1, message: "ok", rows: [], applied: 0, dryRun: false });
});

afterEach(() => {
  stop();
  vi.unstubAllGlobals();
});

describe("onNetWorthChange", () => {
  const writes: [string, () => Promise<unknown>][] = [
    ["adding an asset", () => createWealthAsset({ side: "asset", type: "deposit", name: "活存", assetGroup: "liquid", initialValue: 1 })],
    ["saving a value", () => saveWealthAssetSnapshot(1, 100)],
    ["archiving", () => archiveWealthAsset(1)],
    ["restoring", () => unarchiveWealthAsset(1)],
    ["undoing today's value", () => deleteWealthAssetSnapshot(1, "2026-10-04")],
    ["a real import", () => importWealthCSV("asset,活存,1,liquid", false)],
  ];

  it.each(writes)("fires after %s", async (_name, write) => {
    await write();
    expect(changed).toHaveBeenCalledTimes(1);
  });

  it("doesn't fire for an import preview", async () => {
    await importWealthCSV("asset,活存,1,liquid", true);
    expect(changed).not.toHaveBeenCalled();
  });

  it("doesn't fire for a rename, which moves no number", async () => {
    await updateWealthAsset({ assetId: 1, name: "新名字", assetGroup: "liquid", venue: "" });
    expect(changed).not.toHaveBeenCalled();
  });

  it("doesn't fire when the write is refused", async () => {
    answer(403, { error: "read-only" });
    await expect(saveWealthAssetSnapshot(1, 100)).rejects.toBeInstanceOf(ApiError);
    expect(changed).not.toHaveBeenCalled();
  });

  it("stops firing once unsubscribed", async () => {
    stop();
    await archiveWealthAsset(1);
    expect(changed).not.toHaveBeenCalled();
  });
});
