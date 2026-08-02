import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { LoginModal } from "./LoginModal";
import { getDictionary } from "../i18n";

// Guards the three things the hand-rolled <div className="modal-backdrop">
// version silently lacked and Radix now provides: a real dialog role, Esc to
// dismiss, and initial focus landing on the password field rather than the ×
// button (which is where Radix would put it if onOpenAutoFocus weren't
// prevented in LoginModal.tsx).
const dict = getDictionary("zh");

function setup() {
  const onClose = vi.fn();
  render(<LoginModal dict={dict} onClose={onClose} onSuccess={vi.fn()} />);
  return onClose;
}

// vitest.config.ts sets globals:false, so testing-library's automatic
// afterEach cleanup never registers — without this each test leaks its
// render into the next one's DOM query.
afterEach(cleanup);

describe("LoginModal accessibility", () => {
  it("exposes a dialog role with an accessible name", () => {
    setup();
    expect(screen.getByRole("dialog", { name: dict.loginTitle })).not.toBeNull();
  });

  it("closes on Escape", () => {
    const onClose = setup();
    fireEvent.keyDown(document, { key: "Escape" });
    expect(onClose).toHaveBeenCalled();
  });

  it("puts initial focus on the password field, not the close button", () => {
    setup();
    expect(document.activeElement).toBe(screen.getByLabelText(dict.password));
  });
});
