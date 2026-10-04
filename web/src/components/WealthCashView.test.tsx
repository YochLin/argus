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
  // The server prices every item in TWD, a paused one included.
  return { currency: "TWD", active: true, valueTwd: over.amount, ...over };
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
  await screen.findByText(dict.wealthCashInBreakdownTitle, { selector: ".eyebrow" }); // the card title, not the net card's "月收入" label
  await screen.findByText(data.items.find((i) => i.active)!.name); // the list arrived
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
    expect(card.getAllByText(dict.wealthCashTagIn)).toHaveLength(1);
    expect(card.getAllByText(dict.wealthCashTagOut)).toHaveLength(2);
    // A flow paused before the date was recorded just has no date, not "undefined".
    expect(card.queryByText(/undefined/)).toBeNull();
    expect(card.getAllByText(/暫停於/)).toHaveLength(2); // 3 paused, one of them undated
    // And it's not counted in the breakdown above: each name appears once.
    expect(screen.getAllByText("健身房月費")).toHaveLength(1);
  });

  it("tags a paused flow In / Out in English, as the design does (not Income / Expense)", async () => {
    const en = getDictionary("en");
    render(
      <FlashProvider>
        <WealthCashView dict={en} writable onUnauthorized={vi.fn()} />
      </FlashProvider>,
    );
    const card = within((await screen.findByText(en.wealthCashPausedNote)).closest(".card") as HTMLElement);
    expect(card.getAllByText("In")).toHaveLength(1);
    expect(card.getAllByText("Out")).toHaveLength(2);
    expect(card.queryByText("Income")).toBeNull();
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

describe("WealthCashView: every amount in TWD", () => {
  const foreign: WealthCash = {
    ...cash,
    items: [
      item({ id: 1, name: "薪資", direction: "in", amount: 80000 }),
      item({ id: 2, name: "美股定期定額", direction: "out", amount: 1000, currency: "USD", valueTwd: 31800 }),
      item({ id: 3, name: "美元股息", direction: "in", amount: 200, currency: "USD", valueTwd: 6360, active: false, pausedAt: "2026-05-01" }),
    ],
    monthlyIn: 80000,
    monthlyOut: 31800,
    events: [
      { date: "2026-10-05", item: "美股定期定額", amount: 1000, currency: "USD", valueTwd: 31800, direction: "out" },
      { date: "2026-10-06", item: "歐元費用", amount: 20, currency: "EUR", direction: "out" }, // no rate for EUR
      { date: "2026-10-07", item: "美元股息", amount: 200, currency: "USD", valueTwd: 6360, direction: "in" },
      { date: "2026-10-09", item: "選擇權到期", amount: null, direction: "event" },
    ],
  };

  it("shows a foreign flow in TWD in the breakdown, the paused card and the events, the original in a tooltip", async () => {
    await setup(true, foreign);
    // The breakdown row and the event row both carry the original USD 1,000 as a tooltip.
    const [row, eventCell] = screen.getAllByTitle("USD 1,000");
    expect(row.textContent).toBe("NT$31,800");
    expect(eventCell.textContent).toBe("-NT$31,800");
    // The paused flow, which feeds no total, is priced too.
    expect(within(pausedCard()).getByTitle("USD 200").textContent).toBe(`NT$6,360${dict.wealthCashPerMonth}`);
    expect(screen.getByText("+NT$6,360")).not.toBeNull();
    // Nothing is shown as a bare USD amount.
    expect(screen.queryByText(/^USD /)).toBeNull();
  });

  it("falls back to the original amount, and says why, when there is no TWD value", async () => {
    await setup(true, foreign);
    const cell = screen.getByText("-EUR 20");
    expect(cell.getAttribute("title")).toBe(dict.wealthNoRate.replace("%s", "EUR"));
  });

  it("still lists an active flow it can't price, in its own currency, and blanks the totals", async () => {
    await setup(true, {
      ...cash,
      items: [
        item({ id: 1, name: "薪資", direction: "in", amount: 80000 }),
        item({ id: 2, name: "歐元租金", direction: "in", amount: 500, currency: "EUR", valueTwd: undefined }),
      ],
      monthlyIn: null,
      monthlyOut: null,
      monthlyNet: null,
      events: [],
    });
    const eur = screen.getByText("EUR 500");
    expect(eur.getAttribute("title")).toBe(dict.wealthNoRate.replace("%s", "EUR"));
    expect(screen.getByRole("button", { name: `${dict.wealthCashPause} 歐元租金` })).not.toBeNull(); // and it can be paused
    // No card total pretends to be NT$0, and the priced flow is listed before the unpriced one.
    const header = screen.getByText(dict.wealthCashInBreakdownTitle, { selector: ".eyebrow" }).parentElement as HTMLElement;
    expect(header.textContent).toContain("—");
    expect(header.textContent).not.toContain("NT$0");
    const names = screen.getAllByText(/^(薪資|歐元租金)$/).map((e) => e.textContent);
    expect(names).toEqual(["薪資", "歐元租金"]);
  });
});
