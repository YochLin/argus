import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { ConfirmDialog } from "./ConfirmDialog";

// vitest.config.ts sets globals:false, so testing-library's automatic
// afterEach cleanup never registers (same note as LoginModal.test.tsx).
afterEach(cleanup);

function setup() {
  const onCancel = vi.fn();
  const onConfirm = vi.fn();
  render(
    <ConfirmDialog
      title="封存「活存」？"
      body="封存後不再計入淨值。"
      okLabel="封存"
      cancelLabel="取消"
      onCancel={onCancel}
      onConfirm={onConfirm}
    />,
  );
  return { onCancel, onConfirm };
}

describe("ConfirmDialog", () => {
  it("is an alertdialog named by its title and described by its body", () => {
    setup();
    const dialog = screen.getByRole("alertdialog", { name: "封存「活存」？" });
    expect(dialog.getAttribute("aria-describedby")).not.toBeNull();
    expect(screen.getByText("封存後不再計入淨值。")).not.toBeNull();
  });

  // The point of the component: Enter on a freshly opened dialog must not
  // destroy anything.
  it("opens with focus on Cancel, not on the destructive button", () => {
    setup();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "取消" }));
  });

  it("confirms only through the OK button", () => {
    const { onCancel, onConfirm } = setup();
    fireEvent.click(screen.getByRole("button", { name: "封存" }));
    expect(onConfirm).toHaveBeenCalledTimes(1);
    expect(onCancel).not.toHaveBeenCalled();
  });

  it("cancels on the Cancel button and on Escape", () => {
    const { onCancel, onConfirm } = setup();
    fireEvent.click(screen.getByRole("button", { name: "取消" }));
    fireEvent.keyDown(screen.getByRole("alertdialog"), { key: "Escape" });
    expect(onCancel).toHaveBeenCalledTimes(2);
    expect(onConfirm).not.toHaveBeenCalled();
  });
});
