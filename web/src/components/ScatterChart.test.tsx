import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { ScatterChart } from "./ScatterChart";

describe("ScatterChart", () => {
  it("renders nothing for an empty point list", () => {
    const { container } = render(<ScatterChart points={[]} />);
    expect(container.firstChild).toBeNull();
  });

  it("renders min/mid/max tick labels for the axis range", () => {
    render(<ScatterChart points={[{ x: 0, y: 10 }, { x: 8, y: -5 }]} />);
    // x ticks: min 0, mid 4, max 8 — y ticks: min -5, mid 2.5, max 10
    expect(screen.getByText("0")).not.toBeNull();
    expect(screen.getByText("4")).not.toBeNull();
    expect(screen.getByText("8")).not.toBeNull();
    expect(screen.getByText("-5")).not.toBeNull();
    expect(screen.getByText("2.5")).not.toBeNull();
    expect(screen.getByText("10")).not.toBeNull();
  });

  it("renders each point's tooltip label as hidden text, not a native title", () => {
    const { container } = render(
      <ScatterChart points={[{ x: 3, y: 8, label: "AAPL: 3d, $8" }]} />
    );
    expect(screen.getByText("AAPL: 3d, $8")).not.toBeNull();
    expect(container.querySelector("title")).toBeNull();
  });

  it("applies custom axis formatters and labels", () => {
    render(
      <ScatterChart
        points={[{ x: -2.5, y: 4.2 }]}
        xLabel="MAE %"
        yLabel="Return %"
        fmtX={(n) => `${n.toFixed(1)}%`}
        fmtY={(n) => `${n.toFixed(1)}%`}
      />
    );
    expect(screen.getByText("MAE %")).not.toBeNull();
    expect(screen.getByText("Return %")).not.toBeNull();
    expect(screen.getByText("-2.5%")).not.toBeNull();
    expect(screen.getByText("4.2%")).not.toBeNull();
  });
});
