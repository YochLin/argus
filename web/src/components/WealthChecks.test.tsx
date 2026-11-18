import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";
import { WealthChecksCard, checkText } from "./WealthChecks";
import { FlashProvider } from "../flash";
import { getDictionary } from "../i18n";
import * as api from "../api";
import type { CheckItem } from "../api";

vi.mock("../api", async (importOriginal) => {
  const real = await importOriginal<typeof import("../api")>();
  return { ...real, fetchWealthChecks: vi.fn(), saveWealthProfile: vi.fn() };
});

const m = vi.mocked(api);
const dict = getDictionary("zh");
const nav = vi.fn();

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function show(items: CheckItem[], isNew = false) {
  m.fetchWealthChecks.mockResolvedValue({ asOf: "", new: isNew, rules: 9, items });
  render(
    <FlashProvider>
      <WealthChecksCard dict={dict} writable refresh={0} onUnauthorized={vi.fn()} onNavigate={nav} guide={<div>GUIDE</div>} />
    </FlashProvider>,
  );
}

const items: CheckItem[] = [
  { code: "emergency", severity: "do", link: "/w/balance", value: 2.14, target: 9, gap: 84000 },
  { code: "goals", severity: "tip", link: "/w/goals", count: 2 },
  { code: "savings", severity: "tip", link: "/w/cash", value: 12, target: 20 },
  { code: "dependents", severity: "data", link: "/w/balance" },
];

describe("checkText", () => {
  it("words an item from its figures", () => {
    expect(checkText(dict, items[0]).reason).toBe("流動資產可撐 2.1 個月，有扶養人建議至少 9 個月");
    expect(checkText(dict, { ...items[0], target: 6 }).reason).toBe("流動資產可撐 2.1 個月，一般建議至少 6 個月");
    expect(checkText(dict, { code: "coverage", severity: "do", link: "", kind: "life", value: 40, target: 80 }).title).toBe("補足壽險保障");
    expect(checkText(dict, { code: "stale", severity: "data", link: "", count: 4, names: ["a", "b", "c"] }).reason).toBe("a、b、c 超過 90 天沒更新，淨值可能不準");
  });
});

describe("WealthChecksCard", () => {
  it("shows three, then all on request, and links each to its page", async () => {
    show(items);
    await screen.findByText(dict.ckEmergencyTitle);
    expect(screen.queryByText(dict.ckDependentsTitle)).toBeNull();
    expect(screen.getByText("NT$84,000")).not.toBeNull();
    fireEvent.click(screen.getByText("查看全部 4 項"));
    expect(screen.getByText(dict.ckDependentsTitle)).not.toBeNull();
    fireEvent.click(screen.getByText(dict.ckEmergencyTitle));
    expect(nav).toHaveBeenCalledWith("/w/balance");
  });

  it("asks for dependents in place and reloads", async () => {
    show([items[3]]);
    fireEvent.click(await screen.findByText(dict.ckDependentsTitle));
    m.fetchWealthChecks.mockResolvedValue({ asOf: "", new: false, rules: 9, items: [] });
    m.saveWealthProfile.mockResolvedValue({ message: "saved" } as never);
    fireEvent.click(screen.getByText("2"));
    await waitFor(() => expect(m.saveWealthProfile).toHaveBeenCalledWith({ dependents: 2 }));
    expect(await screen.findByText(dict.ckClearTitle)).not.toBeNull();
    expect(screen.getByText("9 項規則全部通過")).not.toBeNull();
  });

  it("shows the setup guide on an empty account", async () => {
    show([], true);
    expect(await screen.findByText("GUIDE")).not.toBeNull();
    expect(screen.queryByText(dict.ckTitle)).toBeNull();
  });
});
