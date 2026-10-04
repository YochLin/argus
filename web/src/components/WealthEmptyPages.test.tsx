import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, waitFor } from "@testing-library/react";
import { WealthHomeView } from "./WealthHomeView";
import { WealthBalanceView } from "./WealthBalanceView";
import { WealthCashView } from "./WealthCashView";
import { WealthAllocView } from "./WealthAllocView";
import { WealthRetireView } from "./WealthRetireView";
import { WealthInsureView } from "./WealthInsureView";
import { FlashProvider } from "../flash";
import { getDictionary } from "../i18n";
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
    fetchWealthRetire: vi.fn(),
    fetchWealthInsure: vi.fn(),
    fetchWealthProfile: vi.fn(),
  };
});

const m = vi.mocked(api);
const dict = getDictionary("zh");

const asset = (over: object = {}) => ({ id: 1, side: "asset", name: "活存", value: 1, currency: "TWD", ...over });

// Every endpoint answers "nothing recorded" unless a test says otherwise.
beforeEach(() => {
  m.fetchWealthHome.mockResolvedValue({ staleCount: 0, allocation: [] } as never);
  m.fetchWealthAlloc.mockResolvedValue({ totalAssets: null, allocation: [], orders: [] } as never);
  m.fetchWealthBalance.mockResolvedValue({ assetGroups: [], liabilities: [], quarterlyTrend: [] } as never);
  m.fetchWealthAssets.mockResolvedValue({ assets: [] });
  m.fetchWealthCash.mockResolvedValue({ items: [], events: [], forecast: [] } as never);
  m.fetchWealthRetire.mockResolvedValue({ hasBirthYear: false, isSample: true, retirementAgeOptions: [], monthlySpendOptions: [] } as never);
  m.fetchWealthInsure.mockResolvedValue({ hasProfile: false, policies: [], rows: [] } as never);
  m.fetchWealthProfile.mockResolvedValue({ birthYear: null } as never);
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const noop = vi.fn();
// The card's own button: most pages also have an add button in their header.
const addButton = () => document.querySelector(".wealth-empty-add");

// Mount a page and wait until the data it asked for has arrived.
async function show(view: "home" | "balance" | "cash" | "alloc" | "retire" | "insure", writable = true) {
  const props = { dict, writable, onUnauthorized: noop, onNavigate: noop };
  const ui = {
    home: <WealthHomeView {...props} />,
    balance: <WealthBalanceView {...props} />,
    cash: <WealthCashView dict={dict} writable={writable} onUnauthorized={noop} />,
    alloc: <WealthAllocView dict={dict} writable={writable} onUnauthorized={noop} />,
    retire: <WealthRetireView {...props} />,
    insure: <WealthInsureView dict={dict} writable={writable} onUnauthorized={noop} />,
  }[view];
  render(<FlashProvider>{ui}</FlashProvider>);
  const fetched = { home: m.fetchWealthAssets, balance: m.fetchWealthAssets, cash: m.fetchWealthCash, alloc: m.fetchWealthAssets, retire: m.fetchWealthAssets, insure: m.fetchWealthInsure }[view];
  await waitFor(() => expect(fetched).toHaveBeenCalled());
  await new Promise((r) => setTimeout(r, 0)); // let the responses land
}

const cases = [
  { view: "home", line: () => dict.wealthEmptyNet, give: () => m.fetchWealthAssets.mockResolvedValue({ assets: [asset()] as never }), body: () => dict.wealthTotalAssets },
  { view: "balance", line: () => dict.wealthEmptyBalance, give: () => m.fetchWealthAssets.mockResolvedValue({ assets: [asset()] as never }), body: () => dict.wealthNetWorth },
  { view: "cash", line: () => dict.wealthEmptyCash, give: () => m.fetchWealthCash.mockResolvedValue({ items: [{ id: 1, direction: "out", name: "房租", amount: 1, currency: "TWD", active: true, valueTwd: 1 }], events: [], forecast: [] } as never), body: () => dict.wealthCashMonthlyNet },
  { view: "alloc", line: () => dict.wealthEmptyAlloc, give: () => m.fetchWealthAssets.mockResolvedValue({ assets: [asset()] as never }), body: () => dict.wealthMixTitle },
  { view: "retire", line: () => dict.wealthEmptyRetire, give: () => m.fetchWealthAssets.mockResolvedValue({ assets: [asset()] as never }), body: () => dict.wealthRetireSetupTitle },
  { view: "insure", line: () => dict.wealthEmptyInsure, give: () => m.fetchWealthInsure.mockResolvedValue({ hasProfile: false, policies: [{}], rows: [] } as never), body: () => dict.wealthInsureBiggestGapLabel },
] as const;

describe.each(cases)("$view page", ({ view, line, give, body }) => {
  it("is one line and an add button, nothing else, while it has no data", async () => {
    await show(view);
    expect(screen.getByText(line())).not.toBeNull();
    expect(addButton()).not.toBeNull();
    expect(screen.queryByText(body())).toBeNull();
  });

  it("is the page itself once there is data", async () => {
    give();
    await show(view);
    expect(screen.queryByText(line())).toBeNull();
    expect(screen.getAllByText(body()).length).toBeGreaterThan(0);
  });

  it("offers no add button read-only", async () => {
    await show(view, false);
    expect(screen.getByText(line())).not.toBeNull();
    expect(addButton()).toBeNull();
  });
});

describe("what counts as empty", () => {
  it("/w/balance still shows the page when only archived records are left", async () => {
    m.fetchWealthAssets.mockResolvedValue({ assets: [asset({ archivedAt: "2026-01-01" })] as never });
    await show("balance");
    expect(screen.queryByText(dict.wealthEmptyBalance)).toBeNull();
  });

  it("/w shows the empty card, and the guide, while nothing is recorded", async () => {
    await show("home");
    expect(screen.getByText(dict.wealthEmptyNet)).not.toBeNull();
    expect(await screen.findByText(dict.wealthGuideTitle)).not.toBeNull();
    expect(screen.getByText("0 / 5 完成")).not.toBeNull(); // the first step isn't ticked with nothing recorded
  });

  it("/w keeps the guide up beside a page that has data", async () => {
    m.fetchWealthAssets.mockResolvedValue({ assets: [asset()] as never });
    await show("home");
    expect(await screen.findByText(dict.wealthGuideTitle)).not.toBeNull();
    expect(screen.getByText("1 / 5 完成")).not.toBeNull();
  });

  it("/w/cash still shows the page when every flow is paused", async () => {
    m.fetchWealthCash.mockResolvedValue({
      items: [{ id: 1, direction: "out", name: "舊訂閱", amount: 1, currency: "TWD", active: false, valueTwd: 1 }],
      events: [],
      forecast: [],
    } as never);
    await show("cash");
    expect(screen.queryByText(dict.wealthEmptyCash)).toBeNull();
  });

  it("/w/alloc and /w/retire count assets, not liabilities", async () => {
    m.fetchWealthAssets.mockResolvedValue({ assets: [asset({ side: "liability" })] as never });
    await show("alloc");
    expect(screen.getByText(dict.wealthEmptyAlloc)).not.toBeNull();
    cleanup();
    await show("retire");
    expect(screen.getByText(dict.wealthEmptyRetire)).not.toBeNull();
  });

  it("/w/alloc isn't called empty just because its total couldn't be priced", async () => {
    m.fetchWealthAssets.mockResolvedValue({ assets: [asset({ currency: "XXX" })] as never });
    await show("alloc"); // fetchWealthAlloc says totalAssets: null, as it does with no exchange rate
    expect(screen.queryByText(dict.wealthEmptyAlloc)).toBeNull();
  });
});
