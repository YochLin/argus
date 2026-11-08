import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, fireEvent, cleanup, waitFor } from "@testing-library/react";
import * as api from "../api";
import { getDictionary } from "../i18n";
import { CashCard } from "./CashCard";

vi.mock("../api", async (importOriginal) => {
  const real = await importOriginal<typeof import("../api")>();
  return { ...real, saveCash: vi.fn() };
});

afterEach(cleanup);
const m = vi.mocked(api);
const dict = getDictionary("zh");

const base: api.Risk = {
  accountValue: 55730,
  cash: 0,
  hasCash: false,
  cashSyncedFromBroker: false,
  heatPct: 8.1,
  heatThresholdPct: 6,
  positions: [],
};

beforeEach(() => vi.clearAllMocks());

describe("CashCard", () => {
  it("warns when cash was never set and saves the typed amount for this market", async () => {
    m.saveCash.mockResolvedValue({ message: "saved" });
    const onSaved = vi.fn();
    render(<CashCard dict={dict} market="tw" risk={base} writable onSaved={onSaved} onUnauthorized={vi.fn()} />);
    expect(screen.getByText("尚未設定現金")).not.toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "設定現金" }));
    fireEvent.change(screen.getByLabelText("台幣現金（NT$）"), { target: { value: "386,000.5" } });
    fireEvent.click(screen.getByRole("button", { name: "儲存" }));

    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    expect(m.saveCash).toHaveBeenCalledWith("tw", 386000.5);
  });

  it("keeps the typed amount when the save fails", async () => {
    m.saveCash.mockRejectedValue(new api.ApiError(500, "failed to save cash"));
    render(
      <CashCard
        dict={dict}
        market="us"
        risk={{ ...base, cash: 12400, hasCash: true }}
        writable
        onSaved={vi.fn()}
        onUnauthorized={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "編輯" }));
    const input = screen.getByLabelText("美元現金（$）") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "15800" } });
    fireEvent.click(screen.getByRole("button", { name: "儲存" }));

    expect((await screen.findByRole("alert")).textContent).toContain("failed to save cash");
    expect(input.value).toBe("15800");
  });

  it("shows no edit controls when read-only", () => {
    render(<CashCard dict={dict} market="us" risk={base} writable={false} onSaved={vi.fn()} onUnauthorized={vi.fn()} />);
    expect(screen.getByText("尚未設定現金")).not.toBeNull();
    expect(screen.queryByRole("button")).toBeNull();
  });
});
