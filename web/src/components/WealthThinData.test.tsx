import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { WealthAllocView } from "./WealthAllocView";
import { wealthReadiness } from "./WealthOnboarding";
import { FlashProvider } from "../flash";
import { getDictionary } from "../i18n";
import { migrateLegacyModel } from "../wealthCategory";
import * as api from "../api";

vi.mock("../api", async (importOriginal) => {
  const real = await importOriginal<typeof import("../api")>();
  return {
    ...real,
    fetchWealthHome: vi.fn(),
    fetchWealthAlloc: vi.fn(),
    fetchWealthBalance: vi.fn(),
    fetchWealthAssets: vi.fn(),
    fetchWealthCash: vi.fn(),
    fetchWealthProfile: vi.fn(),
    fetchWealthInsure: vi.fn(),
    saveWealthProfile: vi.fn(),
  };
});

const m = vi.mocked(api);
const dict = getDictionary("zh");
const noop = vi.fn();

const asset = (id: number) => ({ id, side: "asset", name: `a${id}`, value: 1, currency: "TWD", assetGroup: "liquid" });
const assets = (n: number) => ({ assets: Array.from({ length: n }, (_, i) => asset(i + 1)) });
const alloc = {
  model: "balanced",
  modelPicked: false,
  totalAssets: 100,
  allocation: [{ category: "cash", currentPct: 100, targetPct: 15, deviationPt: 85, marketValue: 100, venue: "" }],
  orders: [{ category: "cash", side: "sell", amount: 85, assetName: "玉山活存", venue: "", deviationPt: 85 }],
  locked: [],
  rebalTotal: 85,
  riskPct: 0,
  currencyExposure: [],
  concentration: [],
};

// Node's own localStorage (no --localstorage-file) shadows the test DOM's, so
// give the code under test an in-memory one (same as WealthOnboarding.test).
function memoryStorage(): Storage {
  const store = new Map<string, string>();
  return {
    getItem: (k) => store.get(k) ?? null,
    setItem: (k, v) => void store.set(k, String(v)),
    removeItem: (k) => void store.delete(k),
    clear: () => store.clear(),
    key: (i) => [...store.keys()][i] ?? null,
    get length() {
      return store.size;
    },
  };
}

beforeEach(() => {
  vi.stubGlobal("localStorage", memoryStorage());
  m.fetchWealthAlloc.mockResolvedValue(alloc as never);
  m.saveWealthProfile.mockResolvedValue({ message: "saved" } as never);
  m.fetchWealthHome.mockResolvedValue({ staleCount: 0, allocation: [], ytdPct: null, momPct: null } as never);
  m.fetchWealthBalance.mockResolvedValue({ assetGroups: [], liabilities: [], quarterlyTrend: [], debtRatioPct: 600, liquidityMonths: null } as never);
  m.fetchWealthCash.mockResolvedValue({ items: [], events: [], forecast: [] } as never);
  m.fetchWealthProfile.mockResolvedValue({ birthYear: null } as never);
  m.fetchWealthInsure.mockResolvedValue({ hasProfile: false, policies: [], rows: [] } as never);
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  vi.unstubAllGlobals();
});

const alloc$ = () =>
  render(
    <FlashProvider>
      <WealthAllocView dict={dict} writable onUnauthorized={noop} />
    </FlashProvider>,
  );

describe("wealthReadiness", () => {
  it("blames the missing model before the asset count, and the count once a model is picked", () => {
    expect(wealthReadiness(dict, 5, true, false).whyDrift).toBe(dict.wealthNeedModel);
    expect(wealthReadiness(dict, 5, true, true).whyDrift).toBe("");
    expect(wealthReadiness(dict, 2, true, true).whyDrift).toBe("資產僅 2 筆，至少需 3 筆");
  });

  it("only asks for spending when it isn't known", () => {
    expect(wealthReadiness(dict, 3, true, true).whySpend).toBe("");
    expect(wealthReadiness(dict, 3, false, true).whySpend).toBe(dict.wealthNeedSpend);
  });
});

describe("/w/alloc with too little data", () => {
  it("withholds the rebalance orders and says why", async () => {
    m.fetchWealthAlloc.mockResolvedValue({ ...alloc, modelPicked: true } as never);
    m.fetchWealthAssets.mockResolvedValue(assets(1) as never);
    alloc$();
    await screen.findByText(/暫不產生再平衡指令：資產僅 1 筆，至少需 3 筆/);
    expect(screen.queryByText("玉山活存")).toBeNull();
    // the risk card and the FX card both carry the "not enough data" note
    expect(screen.getAllByText("資料不足 · 資產僅 1 筆，至少需 3 筆")).toHaveLength(2);
  });

  it("withholds them while no model has been picked, even with enough assets", async () => {
    m.fetchWealthAssets.mockResolvedValue(assets(3) as never);
    alloc$();
    await screen.findByText(/暫不產生再平衡指令：尚未選目標模型/);
    expect(screen.queryByText("玉山活存")).toBeNull();
  });

  it("swaps the reason as soon as a model is picked, even the one already showing", async () => {
    m.fetchWealthAssets.mockResolvedValue(assets(1) as never);
    alloc$();
    await screen.findByText(/暫不產生再平衡指令：尚未選目標模型/);
    m.fetchWealthAlloc.mockResolvedValue({ ...alloc, modelPicked: true } as never);
    fireEvent.click(screen.getByText(dict.wealthModelBalanced));
    expect(m.saveWealthProfile).toHaveBeenCalledWith({ allocModel: "balanced" });
    await screen.findByText(/暫不產生再平衡指令：資產僅 1 筆，至少需 3 筆/);
  });

  it("shows the orders once there are 3 assets and a model", async () => {
    m.fetchWealthAlloc.mockResolvedValue({ ...alloc, modelPicked: true } as never);
    m.fetchWealthAssets.mockResolvedValue(assets(3) as never);
    alloc$();
    await screen.findByText("玉山活存");
    expect(screen.queryByText(/暫不產生再平衡指令/)).toBeNull();
  });
});

describe("migrateLegacyModel", () => {
  it("moves a model saved in this browser to the server, then forgets it", async () => {
    localStorage.setItem("argus.wealth.allocModel", "growth");
    m.fetchWealthProfile.mockResolvedValue({ allocModel: null } as never);
    await migrateLegacyModel();
    expect(m.saveWealthProfile).toHaveBeenCalledWith({ allocModel: "growth" });
    expect(localStorage.getItem("argus.wealth.allocModel")).toBeNull();
  });

  it("keeps the server's model over the browser's, and keeps the browser's when the save fails", async () => {
    localStorage.setItem("argus.wealth.allocModel", "growth");
    m.fetchWealthProfile.mockResolvedValue({ allocModel: "conserv" } as never);
    await migrateLegacyModel();
    expect(m.saveWealthProfile).not.toHaveBeenCalled();
    expect(localStorage.getItem("argus.wealth.allocModel")).toBeNull();

    localStorage.setItem("argus.wealth.allocModel", "growth");
    m.fetchWealthProfile.mockResolvedValue({ allocModel: null } as never);
    m.saveWealthProfile.mockRejectedValue(new Error("401"));
    await migrateLegacyModel();
    expect(localStorage.getItem("argus.wealth.allocModel")).toBe("growth");
  });
});
