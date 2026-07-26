import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { KpiCard } from "./KpiCard";

describe("KpiCard", () => {
  it("renders a dash when value is null", () => {
    render(<KpiCard label="Test Label" value={null} format="percent" />);
    expect(screen.getByText("Test Label")).not.toBeNull();
    expect(screen.getByText("—")).not.toBeNull();
  });

  it("formats currency correctly with positive value", () => {
    render(<KpiCard label="Total P&L" value={1250} format="currency" currency="$" />);
    expect(screen.getByText("+$1,250")).not.toBeNull();
  });

  it("formats currency correctly with negative value", () => {
    render(<KpiCard label="Total P&L" value={-1250} format="currency" currency="NT$" />);
    expect(screen.getByText("-NT$1,250")).not.toBeNull();
  });

  it("formats percent correctly", () => {
    render(<KpiCard label="Win Rate" value={0.654} format="percent" />);
    expect(screen.getByText("65.4%")).not.toBeNull();
  });

  it("formats percentValue correctly with positive value", () => {
    render(<KpiCard label="YTD Return" value={12.3} format="percentValue" />);
    expect(screen.getByText("+12.3%")).not.toBeNull();
  });

  it("formats percentValue correctly with negative value", () => {
    render(<KpiCard label="YTD Return" value={-5.6} format="percentValue" />);
    expect(screen.getByText("-5.6%")).not.toBeNull();
  });

  it("applies profit class for positive P&L colorMode", () => {
    const { container } = render(
      <KpiCard label="P&L" value={100} format="currency" colorMode="pnl" />
    );
    const kpiValue = container.querySelector(".kpi-value");
    expect(kpiValue?.classList.contains("profit")).toBe(true);
  });

  it("applies loss class for negative P&L colorMode", () => {
    const { container } = render(
      <KpiCard label="P&L" value={-100} format="currency" colorMode="pnl" />
    );
    const kpiValue = container.querySelector(".kpi-value");
    expect(kpiValue?.classList.contains("loss")).toBe(true);
  });

  it("applies loss class for loss colorMode", () => {
    const { container } = render(
      <KpiCard label="Max Drawdown" value={10} format="percent" colorMode="loss" />
    );
    const kpiValue = container.querySelector(".kpi-value");
    expect(kpiValue?.classList.contains("loss")).toBe(true);
  });
});
