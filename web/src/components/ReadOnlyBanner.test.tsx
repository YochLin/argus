import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { ReadOnlyBanner, ReadOnlyImportPanel } from "./ReadOnlyBanner";
import { getDictionary } from "../i18n";

afterEach(cleanup);

const dict = getDictionary("zh");

describe("ReadOnlyBanner", () => {
  it("says why editing is off and offers the how-to, collapsed", () => {
    render(<ReadOnlyBanner dict={dict} howOpen={false} onToggleHow={vi.fn()} />);
    expect(screen.getByText(dict.roTag)).not.toBeNull();
    expect(screen.getByText(dict.roMsg)).not.toBeNull();
    expect(screen.getByRole("button", { name: dict.roHow })).not.toBeNull();
    expect(screen.queryByText(dict.roStep1)).toBeNull();
  });

  it("lists the three steps, with the env var on the first, when open", () => {
    render(<ReadOnlyBanner dict={dict} howOpen onToggleHow={vi.fn()} />);
    expect(screen.getByText(dict.roStep1)).not.toBeNull();
    expect(screen.getByText(dict.roStep2)).not.toBeNull();
    expect(screen.getByText(dict.roStep3)).not.toBeNull();
    expect(screen.getByText("WEB_PASSWORD=…")).not.toBeNull();
    expect(screen.getByRole("button", { name: dict.roHide })).not.toBeNull();
  });

  it("toggles through the link", () => {
    const onToggleHow = vi.fn();
    render(<ReadOnlyBanner dict={dict} howOpen={false} onToggleHow={onToggleHow} />);
    fireEvent.click(screen.getByRole("button", { name: dict.roHow }));
    expect(onToggleHow).toHaveBeenCalledTimes(1);
  });
});

describe("ReadOnlyImportPanel", () => {
  it("explains itself with the page's own body and shares the toggle", () => {
    const onToggleHow = vi.fn();
    render(<ReadOnlyImportPanel dict={dict} body={dict.roImportBodyTrade} howOpen={false} onToggleHow={onToggleHow} />);
    expect(screen.getByText(dict.roImportTitle)).not.toBeNull();
    expect(screen.getByText(dict.roImportBodyTrade)).not.toBeNull();
    fireEvent.click(screen.getByRole("button", { name: dict.roHow }));
    expect(onToggleHow).toHaveBeenCalledTimes(1);
  });

  it("has no template button unless the page offers one", () => {
    render(<ReadOnlyImportPanel dict={dict} body={dict.roImportBodyTrade} howOpen={false} onToggleHow={vi.fn()} />);
    expect(screen.queryByRole("button", { name: dict.wealthImportDownload })).toBeNull();
  });

  it("offers the template beside the toggle when the page has one", () => {
    const onDownload = vi.fn();
    const onToggleHow = vi.fn();
    render(<ReadOnlyImportPanel dict={dict} body={dict.roImportBodyWealth} howOpen={false} onToggleHow={onToggleHow} onDownload={onDownload} />);
    fireEvent.click(screen.getByRole("button", { name: dict.wealthImportDownload }));
    expect(onDownload).toHaveBeenCalledTimes(1);
    expect(onToggleHow).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: dict.roHow }));
    expect(onToggleHow).toHaveBeenCalledTimes(1);
  });
});
