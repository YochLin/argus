import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, act, cleanup } from "@testing-library/react";
import { FlashProvider, useFlash } from "./flash";

function Trigger({ message, tone }: { message: string; tone?: "ok" | "error" }) {
  const flash = useFlash();
  return <button onClick={() => flash(message, tone)}>go</button>;
}

beforeEach(() => vi.useFakeTimers());
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

function show(message: string, tone?: "ok" | "error") {
  render(
    <FlashProvider>
      <Trigger message={message} tone={tone} />
    </FlashProvider>,
  );
  act(() => screen.getByRole("button", { name: "go" }).click());
}

describe("flash toast", () => {
  it("shows a success as a status and clears it after 2.6s", () => {
    show("已封存「活存」");
    expect(screen.getByRole("status").textContent).toBe("已封存「活存」");
    act(() => vi.advanceTimersByTime(2599));
    expect(screen.queryByRole("status")).not.toBeNull();
    act(() => vi.advanceTimersByTime(1));
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("shows an error as an alert that stays longer", () => {
    show("failed to archive", "error");
    expect(screen.getByRole("alert").textContent).toBe("failed to archive");
    act(() => vi.advanceTimersByTime(2600));
    expect(screen.queryByRole("alert")).not.toBeNull();
    act(() => vi.advanceTimersByTime(2400));
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("restarts the timer when a second toast replaces the first", () => {
    show("first");
    act(() => vi.advanceTimersByTime(2000));
    act(() => screen.getByRole("button", { name: "go" }).click());
    act(() => vi.advanceTimersByTime(2000)); // 4s after the first, 2s after the second
    expect(screen.queryByRole("status")).not.toBeNull();
  });

  it("is a no-op without a provider, so a bare view can still call it", () => {
    render(<Trigger message="x" />);
    expect(() => act(() => screen.getByRole("button", { name: "go" }).click())).not.toThrow();
    expect(screen.queryByRole("status")).toBeNull();
  });
});
