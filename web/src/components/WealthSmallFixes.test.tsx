import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { fmtMoney } from "./WealthHomeView";
import { QuarterlyBars } from "./WealthBalanceView";
import { setDisplayCurrency } from "../currency";

afterEach(() => {
  cleanup();
  setDisplayCurrency("TWD", 1);
});

describe("fmtMoney", () => {
  it("puts a negative sign before the symbol, not between it and the digits", () => {
    expect(fmtMoney(-2500000, "NT$")).toBe("−NT$2,500,000");
    expect(fmtMoney(2500000, "NT$")).toBe("NT$2,500,000");
    expect(fmtMoney(-5, "$")).toBe("−$5");
  });

  it("signs the converted amount, in the display currency", () => {
    setDisplayCurrency("USD", 30);
    expect(fmtMoney(-3000000, "NT$")).toBe("−$100,000");
  });

  it("shows no sign on an amount that rounds to zero", () => {
    expect(fmtMoney(-0.4, "NT$")).toBe("NT$0");
  });
});

describe("QuarterlyBars", () => {
  const q = (quarter: string, netWorth: number | null) => ({ quarter, netWorth });

  it("draws only the current quarter for a new account, not seven NT$0 bars", () => {
    render(
      <QuarterlyBars
        points={[q("2025-Q1", 0), q("2025-Q2", 0), q("2025-Q3", 0), q("2025-Q4", 0), q("2026-Q1", 0), q("2026-Q2", 0), q("2026-Q3", 0), q("2026-Q4", 120000)]}
      />,
    );
    expect(screen.getByText("2026-Q4")).not.toBeNull();
    expect(screen.queryByText("2026-Q3")).toBeNull();
    expect(screen.queryByText("2025-Q1")).toBeNull();
  });

  it("starts at the first quarter that has a value and keeps every one after it", () => {
    render(<QuarterlyBars points={[q("2026-Q1", 0), q("2026-Q2", 500), q("2026-Q3", 0), q("2026-Q4", 900)]} />);
    expect(screen.queryByText("2026-Q1")).toBeNull();
    expect(screen.getByText("2026-Q2")).not.toBeNull();
    expect(screen.getByText("2026-Q3")).not.toBeNull();
    expect(screen.getByText("2026-Q4")).not.toBeNull();
  });

  it("keeps the current quarter even when nothing has a value yet", () => {
    render(<QuarterlyBars points={[q("2026-Q3", null), q("2026-Q4", null)]} />);
    expect(screen.getByText("2026-Q4")).not.toBeNull();
    expect(screen.queryByText("2026-Q3")).toBeNull();
  });
});
