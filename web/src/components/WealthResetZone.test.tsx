import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, fireEvent, cleanup, waitFor } from "@testing-library/react";
import * as api from "../api";
import { getDictionary } from "../i18n";
import { WealthResetZone } from "./WealthResetZone";
import { SettingsView } from "./SettingsView";

vi.mock("../api", async (importOriginal) => {
  const real = await importOriginal<typeof import("../api")>();
  return { ...real, fetchWealthReset: vi.fn(), resetWealth: vi.fn(), fetchSettings: vi.fn() };
});
const forget = vi.fn();
vi.mock("../assistant", () => ({ useAssistant: () => ({ forget }) }));

afterEach(cleanup);
const m = vi.mocked(api);
const dict = getDictionary("zh");

beforeEach(() => {
  vi.clearAllMocks();
  m.fetchWealthReset.mockResolvedValue({ lastResetAt: null, backupDir: "/srv/argus/data/backups" });
  m.fetchSettings.mockResolvedValue({ settings: [] });
});

describe("WealthResetZone", () => {
  it("unlocks 永久重設 only once the checkbox is ticked, with focus on Cancel", async () => {
    render(<WealthResetZone dict={dict} onUnauthorized={vi.fn()} />);
    await screen.findByText("從未重設");
    fireEvent.click(screen.getByRole("button", { name: "重設管理帳戶…" }));

    const ok = screen.getByRole("button", { name: "永久重設" }) as HTMLButtonElement;
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "取消" }));
    expect(ok.disabled).toBe(true);
    // The backup goes where the server says, and the lists say what goes.
    expect(screen.getByText("/srv/argus/data/backups")).not.toBeNull();
    expect(screen.getByText("AI 建檔助理草稿與對話")).not.toBeNull();
    expect(screen.getByText("台股 / 美股交易帳戶")).not.toBeNull();

    fireEvent.click(screen.getByRole("checkbox"));
    expect(ok.disabled).toBe(false);
    fireEvent.click(screen.getByRole("button", { name: "取消" }));
    expect(screen.queryByRole("alertdialog")).toBeNull();
    expect(m.resetWealth).not.toHaveBeenCalled();

    // Reopened, the tick doesn't carry over.
    fireEvent.click(screen.getByRole("button", { name: "重設管理帳戶…" }));
    expect((screen.getByRole("button", { name: "永久重設" }) as HTMLButtonElement).disabled).toBe(true);
  });

  it("resets, shows when, and has the assistant forget its conversation", async () => {
    m.resetWealth.mockResolvedValue({ lastResetAt: "2026-10-14 23:40", backup: "/x.db" });
    render(<WealthResetZone dict={dict} onUnauthorized={vi.fn()} />);
    fireEvent.click(await screen.findByRole("button", { name: "重設管理帳戶…" }));
    fireEvent.click(screen.getByRole("checkbox"));
    fireEvent.click(screen.getByRole("button", { name: "永久重設" }));

    await screen.findByText("上次重設：2026-10-14 23:40");
    expect(m.resetWealth).toHaveBeenCalledTimes(1);
    expect(forget).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("alertdialog")).toBeNull();
  });

  it("keeps the dialog open when the reset fails", async () => {
    m.resetWealth.mockRejectedValue(new api.ApiError(500, "failed to back up the database; nothing was deleted"));
    render(<WealthResetZone dict={dict} onUnauthorized={vi.fn()} />);
    fireEvent.click(await screen.findByRole("button", { name: "重設管理帳戶…" }));
    fireEvent.click(screen.getByRole("checkbox"));
    fireEvent.click(screen.getByRole("button", { name: "永久重設" }));

    await waitFor(() => expect(m.resetWealth).toHaveBeenCalled());
    expect(screen.getByRole("alertdialog")).not.toBeNull();
    expect(forget).not.toHaveBeenCalled();
  });
});

describe("SettingsView danger zone", () => {
  it("shows only in the wealth account", async () => {
    const { unmount } = render(<SettingsView dict={dict} onUnauthorized={vi.fn()} />);
    await screen.findByText(dict.settingsRulesTitle);
    expect(screen.queryByText("危險區域")).toBeNull();
    unmount();

    render(<SettingsView dict={dict} wealth onUnauthorized={vi.fn()} />);
    expect(await screen.findByText("危險區域")).not.toBeNull();
  });
});
