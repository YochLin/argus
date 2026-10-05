import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, fireEvent, cleanup, waitFor } from "@testing-library/react";
import { WealthEmptyCard, WealthGuide, useGateAdd, useNoAssets } from "./WealthOnboarding";
import { FlashProvider } from "../flash";
import { getDictionary } from "../i18n";
import * as api from "../api";

vi.mock("../api", async (importOriginal) => {
  const real = await importOriginal<typeof import("../api")>();
  return {
    ...real,
    fetchWealthAssets: vi.fn(),
    fetchWealthCash: vi.fn(),
    fetchWealthProfile: vi.fn(),
    fetchWealthInsure: vi.fn(),
  };
});

const m = vi.mocked(api);
const dict = getDictionary("zh");

const flow = (direction: "in" | "out", active = true) => ({ id: 1, direction, name: "x", amount: 1, currency: "TWD", active });

// What the guide reads besides the asset list: expense flows, the birth year, policies.
function saved(over: { flows?: ReturnType<typeof flow>[]; birthYear?: number | null; policies?: number } = {}) {
  m.fetchWealthCash.mockResolvedValue({ items: over.flows ?? [] } as never);
  m.fetchWealthProfile.mockResolvedValue({ birthYear: over.birthYear ?? null } as never);
  m.fetchWealthInsure.mockResolvedValue({ policies: Array(over.policies ?? 0).fill({}) } as never);
}

// Node's own localStorage (no --localstorage-file) shadows the test DOM's, so
// give the code under test an in-memory one.
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
  saved();
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  vi.unstubAllGlobals();
});

async function guide(hasAssets: boolean, onNavigate = vi.fn()) {
  render(<WealthGuide dict={dict} hasAssets={hasAssets} onNavigate={onNavigate} />);
  await screen.findByText(dict.wealthGuideTitle);
  return { onNavigate };
}

const done = () => document.querySelectorAll(".wealth-guide-mark.done").length;

describe("WealthEmptyCard", () => {
  it("shows the line and an add button that fires", () => {
    const onAdd = vi.fn();
    render(<WealthEmptyCard dict={dict} line="還沒有資料" onAdd={onAdd} />);
    expect(screen.getByText("還沒有資料")).not.toBeNull();
    fireEvent.click(screen.getByRole("button", { name: new RegExp(dict.wealthHomeAddNew) }));
    expect(onAdd).toHaveBeenCalledTimes(1);
  });

});

describe("useGateAdd", () => {
  function Add({ writable, open }: { writable: boolean; open: () => void }) {
    const gateAdd = useGateAdd(dict, writable);
    return <button onClick={gateAdd(open)}>add</button>;
  }
  const click = (writable: boolean, open: () => void) => {
    render(
      <FlashProvider>
        <Add writable={writable} open={open} />
      </FlashProvider>,
    );
    fireEvent.click(screen.getByText("add"));
  };

  it("opens the form on a writable server", () => {
    const open = vi.fn();
    click(true, open);
    expect(open).toHaveBeenCalledTimes(1);
    expect(screen.queryByText(dict.wealthRoNoAdd)).toBeNull();
  });

  it("opens nothing read-only, and says why", () => {
    const open = vi.fn();
    click(false, open);
    expect(open).not.toHaveBeenCalled();
    expect(screen.getByText(dict.wealthRoNoAdd)).not.toBeNull();
  });
});

describe("WealthGuide", () => {
  it("lists the five steps in order, none done on an empty account", async () => {
    await guide(false);
    const labels = [...document.querySelectorAll(".wealth-guide-label")].map((e) => e.textContent);
    expect(labels).toEqual([dict.navWealthBalance, dict.navWealthCash, dict.navWealthRetire, dict.navWealthAlloc, dict.navWealthInsure]);
    expect(done()).toBe(0);
    expect(screen.getByText("0 / 5 完成")).not.toBeNull();
    expect(screen.getByText(dict.wealthGuideWhyInsure)).not.toBeNull();
  });

  it("ticks a step once its data exists", async () => {
    saved({ flows: [flow("out")], birthYear: 1990 });
    await guide(true);
    const ticked = [...document.querySelectorAll(".wealth-guide-step")].map((s) => s.querySelector(".wealth-guide-mark.done") != null);
    // 資產負債表, 現金流, 退休 done; 配置 (never picked) and 保單 not.
    expect(ticked).toEqual([true, true, true, false, false]);
    expect(screen.getByText("3 / 5 完成")).not.toBeNull();
  });

  it("counts the cash step only for an active expense", async () => {
    saved({ flows: [flow("in"), flow("out", false)] });
    await guide(true);
    expect(document.querySelectorAll(".wealth-guide-step")[1].querySelector(".done")).toBeNull();
  });

  it("counts the allocation step once a model has been picked in this browser", async () => {
    localStorage.setItem("argus.wealth.allocModel", "balanced");
    await guide(false);
    expect(document.querySelectorAll(".wealth-guide-step")[3].querySelector(".wealth-guide-mark.done")).not.toBeNull();
    expect(done()).toBe(1);
  });

  it("counts the insurance step once there is a policy", async () => {
    saved({ policies: 1 });
    await guide(false);
    expect(document.querySelectorAll(".wealth-guide-step")[4].querySelector(".wealth-guide-mark.done")).not.toBeNull();
  });

  it("is gone once all five are done", async () => {
    localStorage.setItem("argus.wealth.allocModel", "growth");
    saved({ flows: [flow("out")], birthYear: 1990, policies: 2 });
    render(<WealthGuide dict={dict} hasAssets={true} onNavigate={vi.fn()} />);
    await waitFor(() => expect(m.fetchWealthInsure).toHaveBeenCalled());
    await new Promise((r) => setTimeout(r, 0));
    expect(screen.queryByText(dict.wealthGuideTitle)).toBeNull();
  });

  it("shows nothing when its data can't be loaded", async () => {
    m.fetchWealthCash.mockRejectedValue(new Error("boom"));
    render(<WealthGuide dict={dict} hasAssets={false} onNavigate={vi.fn()} />);
    await waitFor(() => expect(m.fetchWealthCash).toHaveBeenCalled());
    await new Promise((r) => setTimeout(r, 0));
    expect(screen.queryByText(dict.wealthGuideTitle)).toBeNull();
  });

  it("navigates to the step's page", async () => {
    const { onNavigate } = await guide(false);
    fireEvent.click(screen.getByText(dict.navWealthCash));
    expect(onNavigate).toHaveBeenCalledWith("/w/cash");
    fireEvent.click(screen.getByText(dict.navWealthInsure));
    expect(onNavigate).toHaveBeenLastCalledWith("/w/insure");
  });

  it("still works when the browser blocks storage", async () => {
    const blocked = () => {
      throw new Error("blocked");
    };
    vi.stubGlobal("localStorage", { getItem: blocked, setItem: blocked });
    await guide(false);
    expect(done()).toBe(0);
    fireEvent.click(screen.getByRole("button", { name: dict.wealthGuideHide }));
    expect(screen.queryByText(dict.wealthGuideTitle)).toBeNull(); // hidden for this visit
  });

  it("hides for good once hidden", async () => {
    await guide(false);
    fireEvent.click(screen.getByRole("button", { name: dict.wealthGuideHide }));
    expect(screen.queryByText(dict.wealthGuideTitle)).toBeNull();
    cleanup();
    render(<WealthGuide dict={dict} hasAssets={false} onNavigate={vi.fn()} />);
    await waitFor(() => expect(m.fetchWealthCash).toHaveBeenCalledTimes(2));
    await new Promise((r) => setTimeout(r, 0));
    expect(screen.queryByText(dict.wealthGuideTitle)).toBeNull();
  });
});

describe("useNoAssets", () => {
  function Probe() {
    return <span data-testid="p">{String(useNoAssets(0))}</span>;
  }
  const asset = (side: "asset" | "liability") => ({ id: 1, side });

  it("is true for an empty list and for liabilities alone", async () => {
    m.fetchWealthAssets.mockResolvedValue({ assets: [] });
    render(<Probe />);
    await waitFor(() => expect(screen.getByTestId("p").textContent).toBe("true"));
    cleanup();
    m.fetchWealthAssets.mockResolvedValue({ assets: [asset("liability")] as never });
    render(<Probe />);
    await waitFor(() => expect(screen.getByTestId("p").textContent).toBe("true"));
  });

  it("is false with an asset, and while the list hasn't loaded or failed to", async () => {
    m.fetchWealthAssets.mockResolvedValue({ assets: [asset("asset")] as never });
    render(<Probe />);
    expect(screen.getByTestId("p").textContent).toBe("false");
    await waitFor(() => expect(m.fetchWealthAssets).toHaveBeenCalled());
    await new Promise((r) => setTimeout(r, 0));
    expect(screen.getByTestId("p").textContent).toBe("false");
    cleanup();
    m.fetchWealthAssets.mockRejectedValue(new Error("boom"));
    render(<Probe />);
    await new Promise((r) => setTimeout(r, 0));
    expect(screen.getByTestId("p").textContent).toBe("false");
  });
});
