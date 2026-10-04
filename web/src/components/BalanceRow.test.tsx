import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { BalanceRow } from "./BalanceRow";
import { getDictionary } from "../i18n";
import type { WealthAsset } from "../api";

afterEach(cleanup);

const dict = getDictionary("zh");

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
  value: 100000,
  asOf: "2026-07-15",
};

function setup(over: Partial<React.ComponentProps<typeof BalanceRow>> = {}) {
  const handlers = { onOpen: vi.fn(), onGoTrade: vi.fn(), onInlineSave: vi.fn() };
  render(
    <BalanceRow
      dict={dict}
      name="玉山活存"
      tag={<span>手動</span>}
      valueText="NT$100,000"
      asset={asset}
      link={false}
      writable
      open={false}
      {...handlers}
      {...over}
    />,
  );
  return handlers;
}

const valueButton = () => screen.getByRole("button", { name: "NT$100,000" });

describe("BalanceRow value cell", () => {
  it("types over a manual TWD value in place: Enter saves the number", () => {
    const h = setup();
    fireEvent.click(valueButton());
    const input = screen.getByLabelText(dict.wealthValue) as HTMLInputElement;
    expect(input.value).toBe("100000");
    fireEvent.change(input, { target: { value: "125000.5" } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(h.onInlineSave).toHaveBeenCalledWith(asset, 125000.5);
    expect(screen.queryByLabelText(dict.wealthValue)).toBeNull();
    expect(h.onOpen).not.toHaveBeenCalled();
  });

  it("saves on blur too, and keeps only digits, a dot and a minus", () => {
    const h = setup();
    fireEvent.click(valueButton());
    const input = screen.getByLabelText(dict.wealthValue);
    fireEvent.change(input, { target: { value: "1a2,3" } });
    fireEvent.blur(input);
    expect(h.onInlineSave).toHaveBeenCalledWith(asset, 123);
  });

  it("does not save an unchanged, empty or unparseable value", () => {
    const h = setup();
    fireEvent.click(valueButton());
    fireEvent.keyDown(screen.getByLabelText(dict.wealthValue), { key: "Enter" });
    fireEvent.click(valueButton());
    fireEvent.change(screen.getByLabelText(dict.wealthValue), { target: { value: "" } });
    fireEvent.keyDown(screen.getByLabelText(dict.wealthValue), { key: "Enter" });
    fireEvent.click(valueButton());
    fireEvent.change(screen.getByLabelText(dict.wealthValue), { target: { value: "." } });
    fireEvent.keyDown(screen.getByLabelText(dict.wealthValue), { key: "Enter" });
    expect(h.onInlineSave).not.toHaveBeenCalled();
  });

  it("Escape abandons the edit without saving", () => {
    const h = setup();
    fireEvent.click(valueButton());
    const input = screen.getByLabelText(dict.wealthValue);
    fireEvent.change(input, { target: { value: "5" } });
    fireEvent.keyDown(input, { key: "Escape" });
    expect(h.onInlineSave).not.toHaveBeenCalled();
    expect(screen.queryByLabelText(dict.wealthValue)).toBeNull();
    // and the next edit is not poisoned by the cancel
    fireEvent.click(valueButton());
    const again = screen.getByLabelText(dict.wealthValue);
    fireEvent.change(again, { target: { value: "6" } });
    fireEvent.keyDown(again, { key: "Enter" });
    expect(h.onInlineSave).toHaveBeenCalledWith(asset, 6);
  });

  it.each([
    ["an imported value", { asset: { ...asset, source: "import" as const } }],
    ["a foreign-currency value", { asset: { ...asset, currency: "USD" } }],
    ["any value while read-only", { writable: false }],
  ])("opens the drawer instead for %s", (_name, over) => {
    const h = setup(over);
    fireEvent.click(valueButton());
    expect(h.onOpen).toHaveBeenCalledTimes(1);
    expect(screen.queryByLabelText(dict.wealthValue)).toBeNull();
    expect(h.onInlineSave).not.toHaveBeenCalled();
  });

  it("sends the equity row to the trading account, and gives it no editor", () => {
    const h = setup({ asset: undefined, link: true });
    fireEvent.click(valueButton());
    fireEvent.click(screen.getByRole("button", { name: dict.wealthRowLinkGo }));
    expect(h.onGoTrade).toHaveBeenCalledTimes(2);
    expect(h.onOpen).not.toHaveBeenCalled();
    expect(screen.queryByRole("button", { name: dict.wealthRowMore })).toBeNull();
  });

  it("keeps a liability's value red even as a button", () => {
    setup({ loss: true });
    expect(valueButton().className).toContain("loss");
  });
});

describe("BalanceRow ⋯ button", () => {
  it("opens the drawer, and stays visible while it is open", () => {
    const h = setup({ open: true });
    const more = screen.getByRole("button", { name: dict.wealthRowMore });
    expect(more.className).toContain("open");
    fireEvent.click(more);
    expect(h.onOpen).toHaveBeenCalledWith(asset);
  });

  it("is absent when there is no record to open", () => {
    setup({ asset: undefined, link: false });
    expect(screen.queryByRole("button", { name: dict.wealthRowMore })).toBeNull();
  });
});
