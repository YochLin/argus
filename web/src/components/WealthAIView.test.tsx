import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { WealthAIViewCard } from "./WealthAIView";
import { getDictionary } from "../i18n";
import * as api from "../api";

vi.mock("../api", async (importOriginal) => {
  const real = await importOriginal<typeof import("../api")>();
  return { ...real, fetchWealthAIView: vi.fn() };
});

const m = vi.mocked(api);
const dict = getDictionary("zh");

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("WealthAIViewCard", () => {
  it("shows the summary collapsed and the points once expanded", async () => {
    m.fetchWealthAIView.mockResolvedValue({ generatedAt: "2026-11-01", summary: "保單重複", points: ["兩張醫療險都附日額", "不動產占 44%"] });
    render(<WealthAIViewCard dict={dict} refresh={0} />);
    expect(await screen.findByText("保單重複")).not.toBeNull();
    expect(screen.getByText("未經規則驗證・11/1 產生")).not.toBeNull();
    expect(screen.queryByText("不動產占 44%")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: dict.aivExpand }));
    expect(screen.getByText("不動產占 44%")).not.toBeNull();
    expect(screen.queryByText("保單重複")).toBeNull();
  });

  it("shows nothing before the first monthly report", async () => {
    m.fetchWealthAIView.mockResolvedValue(null);
    const { container } = render(<WealthAIViewCard dict={dict} refresh={0} />);
    await new Promise((r) => setTimeout(r, 0));
    expect(container.innerHTML).toBe("");
  });
});
