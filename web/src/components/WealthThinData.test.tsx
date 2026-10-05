import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";
import { WealthHomeView } from "./WealthHomeView";
import { WealthAllocView } from "./WealthAllocView";
import { wealthReadiness } from "./WealthOnboarding";
import { FlashProvider } from "../flash";
import { getDictionary } from "../i18n";
import { saveModel } from "../wealthCategory";
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
  };
});

const m = vi.mocked(api);
const dict = getDictionary("zh");
const noop = vi.fn();

const asset = (id: number) => ({ id, side: "asset", name: `a${id}`, value: 1, currency: "TWD", assetGroup: "liquid" });
const assets = (n: number) => ({ assets: Array.from({ length: n }, (_, i) => asset(i + 1)) });
const alloc = {
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
    expect(wealthReadiness(dict, 5, true).whyDrift).toBe(dict.wealthNeedModel);
    saveModel("balanced");
    expect(wealthReadiness(dict, 5, true).whyDrift).toBe("");
    expect(wealthReadiness(dict, 2, true).whyDrift).toBe("資產僅 2 筆，至少需 3 筆");
  });

  it("only asks for spending when it isn't known", () => {
    expect(wealthReadiness(dict, 3, true).whySpend).toBe("");
    expect(wealthReadiness(dict, 3, false).whySpend).toBe(dict.wealthNeedSpend);
  });
});

describe("/w/alloc with too little data", () => {
  it("withholds the rebalance orders and says why", async () => {
    saveModel("balanced");
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
    fireEvent.click(screen.getByText(dict.wealthModelBalanced));
    await screen.findByText(/暫不產生再平衡指令：資產僅 1 筆，至少需 3 筆/);
  });

  it("shows the orders once there are 3 assets and a model", async () => {
    saveModel("balanced");
    m.fetchWealthAssets.mockResolvedValue(assets(3) as never);
    alloc$();
    await screen.findByText("玉山活存");
    expect(screen.queryByText(/暫不產生再平衡指令/)).toBeNull();
  });
});

describe("/w with too little data", () => {
  it("annotates the debt ratio and liquidity months instead of leaving a bare number or dash", async () => {
    m.fetchWealthAssets.mockResolvedValue(assets(1) as never);
    render(
      <FlashProvider>
        <WealthHomeView dict={dict} writable onUnauthorized={noop} onNavigate={noop} />
      </FlashProvider>,
    );
    await waitFor(() => expect(screen.getByText("資料不足 · 資產僅 1 筆，至少需 3 筆")).toBeTruthy());
    expect(screen.getByText(`資料不足 · ${dict.wealthNeedSpend}`)).toBeTruthy();
    // the hero blames the model first
    expect(screen.getByText(dict.wealthNeedModel)).toBeTruthy();
  });
});
