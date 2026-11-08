import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import * as api from "../api";
import { getDictionary } from "../i18n";
import { NewsSourceBlock } from "./NewsSourceBlock";

vi.mock("../api", async (importOriginal) => {
  const real = await importOriginal<typeof import("../api")>();
  return { ...real, fetchBlockedNewsSources: vi.fn(), blockNewsSource: vi.fn(), unblockNewsSource: vi.fn() };
});

afterEach(cleanup);
const m = vi.mocked(api);
const dict = getDictionary("zh");

beforeEach(() => {
  vi.clearAllMocks();
  m.fetchBlockedNewsSources.mockResolvedValue({
    sources: [{ source: "MarketPulse Wire", createdAt: "2026-10-03 08:00:00" }],
  });
});

describe("NewsSourceBlock", () => {
  it("lists blocked sources and says when one is already blocked", async () => {
    m.blockNewsSource.mockRejectedValue(new api.ApiError(409, "already blocked"));
    render(<NewsSourceBlock dict={dict} onUnauthorized={vi.fn()} />);
    expect(await screen.findByText("MarketPulse Wire")).not.toBeNull();
    expect(screen.getByText("2026-10-03")).not.toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "封鎖" }));
    expect((await screen.findByRole("alert")).textContent).toBe("請輸入來源名稱。");

    fireEvent.change(screen.getByLabelText("新增封鎖"), { target: { value: "marketpulse wire" } });
    fireEvent.click(screen.getByRole("button", { name: "封鎖" }));
    expect((await screen.findByRole("alert")).textContent).toBe("「marketpulse wire」已經在封鎖清單中。");
  });

  it("offers a retry when unblocking fails", async () => {
    m.unblockNewsSource.mockRejectedValueOnce(new api.ApiError(500, "failed")).mockResolvedValueOnce({ message: "ok" });
    render(<NewsSourceBlock dict={dict} onUnauthorized={vi.fn()} />);
    fireEvent.click(await screen.findByRole("button", { name: "解除封鎖 MarketPulse Wire" }));
    fireEvent.click(await screen.findByRole("button", { name: "重試" }));
    await vi.waitFor(() => expect(m.unblockNewsSource).toHaveBeenCalledTimes(2));
  });
});
