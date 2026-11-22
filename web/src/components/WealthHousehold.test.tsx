import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";
import { WealthHousehold } from "./WealthHousehold";
import { liquidityIp } from "./WealthBalanceView";
import { FlashProvider } from "../flash";
import { getDictionary } from "../i18n";
import * as api from "../api";
import type { WealthProfile } from "../api";

vi.mock("../api", async (importOriginal) => {
  const real = await importOriginal<typeof import("../api")>();
  return { ...real, saveWealthProfile: vi.fn() };
});

const m = vi.mocked(api);
const dict = getDictionary("zh");
const year = new Date().getFullYear();

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const blank: WealthProfile = {
  annualSalary: null,
  birthYear: null,
  retirementMonthlyContribution: null,
  dependents: null,
  youngestChildAge: null,
  spouseHasIncome: null,
  allocModel: null,
};

function show(p: Partial<WealthProfile>) {
  const onSaved = vi.fn();
  render(
    <FlashProvider>
      <WealthHousehold dict={dict} writable profile={{ ...blank, ...p }} onSaved={onSaved} onUnauthorized={vi.fn()} />
    </FlashProvider>,
  );
  return onSaved;
}

describe("WealthHousehold", () => {
  it("opens the form while something is missing and saves the age as a birth year", async () => {
    m.saveWealthProfile.mockResolvedValue({} as never);
    const onSaved = show({});
    fireEvent.change(screen.getByLabelText(dict.hhAge), { target: { value: "38" } });
    fireEvent.click(screen.getByRole("button", { name: "2" }));
    fireEvent.change(screen.getByLabelText(dict.wealthInsureYoungestChildAgeLabel), { target: { value: "5" } });
    fireEvent.click(screen.getByRole("button", { name: dict.hhSpouseNo }));
    fireEvent.click(screen.getByRole("button", { name: dict.hhDone }));
    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    expect(m.saveWealthProfile).toHaveBeenCalledWith({ birthYear: year - 38, dependents: 2, youngestChildAge: 5, spouseHasIncome: false });
  });

  it("shows the saved facts when nothing is missing", () => {
    show({ birthYear: year - 40, dependents: 3, spouseHasIncome: true });
    expect(screen.getByText("40 歲")).not.toBeNull();
    expect(screen.getByText(dict.hhDeps3)).not.toBeNull();
    expect(screen.getByRole("button", { name: dict.hhEdit })).not.toBeNull();
  });
});

describe("liquidityIp", () => {
  it("raises the target to 9 months with dependents and turns red under 3", () => {
    expect(liquidityIp(dict, 7, 0)?.tone).toBe("ok");
    expect(liquidityIp(dict, 7, 1)).toEqual({ tone: "warn", text: dict.ipLiqWarn.replace("{n}", "9") });
    expect(liquidityIp(dict, 2.9, null)?.tone).toBe("bad");
    expect(liquidityIp(dict, null, 0)).toBeNull();
  });
});
