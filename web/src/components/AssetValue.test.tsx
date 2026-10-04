import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { AssetValue, twdAmount } from "./WealthHomeView";
import { getDictionary } from "../i18n";
import type { WealthAsset } from "../api";

const dict = getDictionary("zh");

afterEach(cleanup);

const asset = (over: Partial<WealthAsset>): WealthAsset => ({
  id: 1,
  side: "asset",
  type: "deposit",
  name: "活存",
  assetGroup: "liquid",
  venue: "",
  currency: "TWD",
  source: "manual",
  createdAt: "2026-01-01",
  value: 50000,
  ...over,
});

describe("twdAmount", () => {
  it("shows a TWD amount as is", () => {
    expect(twdAmount(dict, undefined, 50000, "TWD")).toEqual({ text: "NT$50,000", title: undefined, priced: true });
    expect(twdAmount(dict, undefined, 50000, "")).toEqual({ text: "NT$50,000", title: undefined, priced: true });
  });

  it("shows a foreign amount in TWD with the original as the tooltip", () => {
    expect(twdAmount(dict, 31800, 1000, "USD")).toEqual({ text: "NT$31,800", title: "USD 1,000", priced: true });
  });

  it("falls back to the original amount, and says why, when it couldn't be converted", () => {
    expect(twdAmount(dict, undefined, 1000, "EUR")).toEqual({
      text: "EUR 1,000",
      title: dict.wealthNoRate.replace("%s", "EUR"),
      priced: false,
    });
  });
});

describe("AssetValue", () => {
  it("is a dash while the asset has no value yet — never a 0", () => {
    render(<AssetValue dict={dict} asset={asset({ value: null })} />);
    expect(screen.getByText("—")).not.toBeNull();
  });

  it("renders a foreign asset's TWD value with its own-currency tooltip", () => {
    render(<AssetValue dict={dict} asset={asset({ currency: "USD", value: 1000, valueTwd: 31800 })} />);
    const el = screen.getByText("NT$31,800");
    expect(el.getAttribute("title")).toBe("USD 1,000");
  });
});
