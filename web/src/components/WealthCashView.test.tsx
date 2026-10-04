import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, fireEvent, cleanup, waitFor, within } from "@testing-library/react";
import { WealthCashView } from "./WealthCashView";
import { FlashProvider } from "../flash";
import { getDictionary } from "../i18n";
import * as api from "../api";
import type { CashflowItem, WealthCash } from "../api";

vi.mock("../api", async (importOriginal) => {
  const real = await importOriginal<typeof import("../api")>();
  return {
    ...real,
    fetchWealthCash: vi.fn(),
    deactivateWealthCashflow: vi.fn(),
    resumeWealthCashflow: vi.fn(),
    deleteWealthCashflow: vi.fn(),
  };
});

const m = vi.mocked(api);
const dict = getDictionary("zh");

function item(over: Partial<CashflowItem> & Pick<CashflowItem, "id" | "name" | "direction" | "amount">): CashflowItem {
  return { currency: "TWD", active: true, valueTwd: over.active === false ? undefined : over.amount, ...over };
}

const cash: WealthCash = {
  asOf: "2026-07-15",
  items: [
    // The server lists newest first.
    item({ id: 5, name: "子女才藝課", direction: "out", amount: 12000, active: false, pausedAt: "2026-06-30" }),
    item({ id: 1, name: "薪資", direction: "in", amount: 80000 }),
    item({ id: 2, name: "房貸", direction: "out", amount: 35000 }),
    item({ id: 3, name: "健身房月費", direction: "out", amount: 1800, active: false, pausedAt: "2026-05-01" }),
    item({ id: 4, name: "舊訂閱", direction: "in", amount: 500, active: false }), // paused before the date was recorded
  ],
  monthlyIn: 80000,
  monthlyOut: 35000,
  monthlyNet: 45000,
  saveRatePct: 56.25,
  dcaSharePct: 0,
  fixedSharePct: 43.75,
  annualNet: 540000,
  eventsNet: 0,
  events: [],
  forecast: [],
};

beforeEach(() => {
  m.fetchWealthCash.mockResolvedValue(cash);
  for (const f of [m.deactivateWealthCashflow, m.resumeWealthCashflow, m.deleteWealthCashflow]) f.mockResolvedValue({ message: "ok" });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

async function setup(writable = true, data: WealthCash = cash) {
  m.fetchWealthCash.mockResolvedValue(data);
  const onUnauthorized = vi.fn();
  render(
    <FlashProvider>
      <WealthCashView dict={dict} writable={writable} onUnauthorized={onUnauthorized} />
    </FlashProvider>,
  );
  await screen.findByText(dict.wealthCashInBreakdownTitle);
  await screen.findByText("房貸"); // the list arrived
  return { onUnauthorized };
}

const pausedCard = () => screen.getByText(dict.wealthCashPausedNote).closest(".card") as HTMLElement;

describe("WealthCashView: pausing", () => {
  it("offers 暫停 on every active row, and nowhere read-only", async () => {
    await setup(true);
    expect(screen.getByRole("button", { name: `${dict.wealthCashPause} 薪資` })).not.toBeNull();
    expect(screen.getByRole("button", { name: `${dict.wealthCashPause} 房貸` })).not.toBeNull();
    expect(screen.getAllByRole("button", { name: new RegExp(`^${dict.wealthCashPause} `) })).toHaveLength(2);
    cleanup();

    await setup(false);
    expect(screen.queryByRole("button", { name: new RegExp(`^${dict.wealthCashPause} `) })).toBeNull();
  });

  it("pauses the row, says so, and reloads", async () => {
    await setup();
    fireEvent.click(screen.getByRole("button", { name: `${dict.wealthCashPause} 房貸` }));
    await waitFor(() => expect(m.deactivateWealthCashflow).toHaveBeenCalledWith(2));
    expect(await screen.findByText(dict.wealthFlashPaused.replace("%s", "房貸"))).not.toBeNull();
    await waitFor(() => expect(m.fetchWealthCash).toHaveBeenCalledTimes(2));
  });

  it("signs in on a 401 and retries the same pause", async () => {
    m.deactivateWealthCashflow.mockRejectedValueOnce(new api.ApiError(401, "unauthorized"));
    const { onUnauthorized } = await setup();
    fireEvent.click(screen.getByRole("button", { name: `${dict.wealthCashPause} 房貸` }));
    await waitFor(() => expect(onUnauthorized).toHaveBeenCalledTimes(1));
    expect(m.fetchWealthCash).toHaveBeenCalledTimes(1); // nothing changed yet

    onUnauthorized.mock.calls[0][0]();
    await waitFor(() => expect(m.deactivateWealthCashflow).toHaveBeenCalledTimes(2));
    expect(m.deactivateWealthCashflow).toHaveBeenLastCalledWith(2);
  });
});

describe("WealthCashView: the paused card", () => {
  it("lists paused flows with the day they were paused, apart from the active breakdown", async () => {
    await setup();
    const card = within(pausedCard());
    expect(card.getByText(dict.wealthCashPaused)).not.toBeNull();
    expect(card.getByText("3")).not.toBeNull(); // the count
    // Income before expense, each oldest first — not the server's newest-first order.
    const order = card.getAllByText(/^(舊訂閱|健身房月費|子女才藝課)$/).map((e) => e.textContent);
    expect(order).toEqual(["舊訂閱", "健身房月費", "子女才藝課"]);
    expect(card.getByText("健身房月費")).not.toBeNull();
    expect(card.getByText(dict.wealthCashPausedSince.replace("%s", "2026-05-01"))).not.toBeNull();
    expect(card.getByText(`NT$1,800${dict.wealthCashPerMonth}`)).not.toBeNull();
    // Income vs expense tag per row.
    expect(card.getAllByText(dict.wealthCashDirectionIn)).toHaveLength(1);
    expect(card.getAllByText(dict.wealthCashDirectionOut)).toHaveLength(2);
    // A flow paused before the date was recorded just has no date, not "undefined".
    expect(card.queryByText(/undefined/)).toBeNull();
    expect(card.getAllByText(/暫停於/)).toHaveLength(2); // 3 paused, one of them undated
    // And it's not counted in the breakdown above: each name appears once.
    expect(screen.getAllByText("健身房月費")).toHaveLength(1);
  });

  it("is absent when nothing is paused", async () => {
    await setup(true, { ...cash, items: cash.items.filter((i) => i.active) });
    expect(screen.queryByText(dict.wealthCashPausedNote)).toBeNull();
  });

  it("resumes a flow, says so, and reloads", async () => {
    await setup();
    fireEvent.click(screen.getByRole("button", { name: `${dict.wealthCashResume} 健身房月費` }));
    await waitFor(() => expect(m.resumeWealthCashflow).toHaveBeenCalledWith(3));
    expect(await screen.findByText(dict.wealthFlashResumed.replace("%s", "健身房月費"))).not.toBeNull();
    await waitFor(() => expect(m.fetchWealthCash).toHaveBeenCalledTimes(2));
  });
});

describe("WealthCashView: deleting", () => {
  it("asks first, and cancelling deletes nothing", async () => {
    await setup();
    fireEvent.click(screen.getByRole("button", { name: `${dict.wealthCashDelete} 健身房月費` }));
    const dialog = await screen.findByRole("alertdialog");
    expect(within(dialog).getByText(dict.wealthCashDeleteTitle.replace("%s", "健身房月費"))).not.toBeNull();
    expect(within(dialog).getByText(dict.wealthCashDeleteBody)).not.toBeNull();
    expect(m.deleteWealthCashflow).not.toHaveBeenCalled();

    fireEvent.click(within(dialog).getByRole("button", { name: dict.cancel }));
    await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull());
    expect(m.deleteWealthCashflow).not.toHaveBeenCalled();
  });

  it("deletes the flow once confirmed, says so, and reloads", async () => {
    await setup();
    fireEvent.click(screen.getByRole("button", { name: `${dict.wealthCashDelete} 健身房月費` }));
    const dialog = await screen.findByRole("alertdialog");
    fireEvent.click(within(dialog).getByRole("button", { name: dict.wealthCashDelete }));
    await waitFor(() => expect(m.deleteWealthCashflow).toHaveBeenCalledWith(3));
    expect(await screen.findByText(dict.wealthFlashDeleted.replace("%s", "健身房月費"))).not.toBeNull();
    await waitFor(() => expect(m.fetchWealthCash).toHaveBeenCalledTimes(2));
    expect(screen.queryByRole("alertdialog")).toBeNull();
  });

  it("shows the server's reason and reloads when the flow changed under it", async () => {
    m.deleteWealthCashflow.mockRejectedValueOnce(new api.ApiError(409, "pause the cash flow before deleting it"));
    await setup();
    fireEvent.click(screen.getByRole("button", { name: `${dict.wealthCashDelete} 健身房月費` }));
    fireEvent.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: dict.wealthCashDelete }));
    expect(await screen.findByText("pause the cash flow before deleting it")).not.toBeNull();
    expect(screen.queryByText(dict.wealthFlashDeleted.replace("%s", "健身房月費"))).toBeNull();
    await waitFor(() => expect(m.fetchWealthCash).toHaveBeenCalledTimes(2)); // caught up with the server
  });
});

describe("WealthCashView: read-only", () => {
  it("still shows the paused card, but its buttons only explain why nothing happens", async () => {
    await setup(false);
    expect(pausedCard()).not.toBeNull();

    fireEvent.click(screen.getByRole("button", { name: `${dict.wealthCashResume} 健身房月費` }));
    expect(await screen.findByText(dict.wealthRoNoChange)).not.toBeNull();
    fireEvent.click(screen.getByRole("button", { name: `${dict.wealthCashDelete} 健身房月費` }));
    expect(screen.queryByRole("alertdialog")).toBeNull();
    expect(m.resumeWealthCashflow).not.toHaveBeenCalled();
    expect(m.deleteWealthCashflow).not.toHaveBeenCalled();
  });
});
