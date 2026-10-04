import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, fireEvent, cleanup, waitFor } from "@testing-library/react";
import { WEALTH_IMPORT_HEADER, WealthImportView } from "./WealthImportView";
import { getDictionary } from "../i18n";
import * as api from "../api";
import type { WealthImportRow } from "../api";

vi.mock("../api", async (importOriginal) => {
  const real = await importOriginal<typeof import("../api")>();
  return { ...real, importWealthCSV: vi.fn() };
});

const m = vi.mocked(api);
const dict = getDictionary("zh");

const row = (over: Partial<WealthImportRow> & Pick<WealthImportRow, "line" | "status">): WealthImportRow => ({
  side: "asset",
  type: "deposit",
  name: "台銀活存",
  group: "liquid",
  currency: "TWD",
  value: 350000,
  date: "2026-10-04",
  message: "",
  ...over,
});

const preview = [
  row({ line: 2, status: "ok" }),
  row({ line: 3, status: "ok", name: "美元活存", currency: "USD", value: 1200 }),
  row({ line: 4, status: "duplicate", name: "黃金存摺", type: "gold", value: 128000, message: "matches an existing asset (same name/type) — skipped" }),
  // The parser stops at the first bad cell, so a row can come back with no value or date.
  row({ line: 5, status: "error", side: "", type: "", name: "", group: "", value: 0, date: "", message: "side must be \"asset\" or \"liability\": house" }),
];

const textarea = () => screen.getByRole("textbox") as HTMLTextAreaElement;
const button = (name: string) => screen.getByRole("button", { name });

function setup(readOnly?: { howOpen: boolean; onToggleHow: () => void }) {
  const onUnauthorized = vi.fn();
  const onSuccess = vi.fn();
  render(<WealthImportView dict={dict} onUnauthorized={onUnauthorized} onSuccess={onSuccess} readOnly={readOnly} />);
  return { onUnauthorized, onSuccess };
}

const type = (csv: string) => fireEvent.change(textarea(), { target: { value: csv } });

beforeEach(() => {
  m.importWealthCSV.mockResolvedValue({ rows: preview, applied: 0, dryRun: true });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("the format on the page", () => {
  it("shows the header line, what is required, and a guide entry for every column", () => {
    setup();
    expect(screen.getByText(new RegExp(`^${WEALTH_IMPORT_HEADER}`))).not.toBeNull();
    expect(screen.getByText(new RegExp(dict.wealthImportHintNote.slice(0, 12)))).not.toBeNull();
    const codes = [...document.querySelectorAll(".wimp-guide-code")].map((e) => e.textContent);
    // Every column the server reads has an entry, the fund ones included: /w/funds has no add form, so this is the only place to learn them.
    for (const column of WEALTH_IMPORT_HEADER.split(",")) {
      if (["side", "type", "group"].includes(column)) continue; // explained by their values instead
      expect(codes).toContain(column);
    }
    expect(codes).toEqual(dict.wealthImportGuide.map(([code]) => code));
  });

  it.each(["zh", "en"] as const)("%s: the sample rows have exactly the columns of the header", (lang) => {
    const d = getDictionary(lang);
    const width = WEALTH_IMPORT_HEADER.split(",").length;
    expect(width).toBe(18);
    const rows = d.wealthImportSample.split("\n");
    expect(rows.length).toBeGreaterThan(1);
    for (const r of rows) expect(r.split(",")).toHaveLength(width);
  });
});

describe("the template", () => {
  function captureDownload() {
    const blobs: Blob[] = [];
    const names: string[] = [];
    vi.stubGlobal("URL", { createObjectURL: (b: Blob) => (blobs.push(b), "blob:x"), revokeObjectURL: vi.fn() });
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (this: HTMLAnchorElement) {
      names.push(this.download);
    });
    return { blobs, names };
  }

  it("downloads the header and the sample rows as a UTF-8 CSV, with a BOM for Excel", async () => {
    const { blobs, names } = captureDownload();
    setup();
    fireEvent.click(button(dict.wealthImportDownload));
    expect(names).toEqual([dict.wealthImportTemplateName]);
    expect(blobs[0].type).toContain("text/csv");
    const bytes = new Uint8Array(await blobs[0].arrayBuffer());
    expect([...bytes.slice(0, 3)]).toEqual([0xef, 0xbb, 0xbf]);
    expect(new TextDecoder().decode(bytes.slice(3))).toBe(`${WEALTH_IMPORT_HEADER}\n${dict.wealthImportSample}\n`);
  });

  it("is still offered, in the read-only panel", () => {
    const { names } = captureDownload();
    setup({ howOpen: false, onToggleHow: vi.fn() });
    fireEvent.click(button(dict.wealthImportDownload));
    expect(names).toEqual([dict.wealthImportTemplateName]);
  });
});

describe("the editor", () => {
  it("loads the sample under a header line, and clears", () => {
    setup();
    fireEvent.click(button(dict.wealthImportLoadSample));
    expect(textarea().value).toBe(`${WEALTH_IMPORT_HEADER}\n${dict.wealthImportSample}\n`);
    fireEvent.click(button(dict.wealthImportClear));
    expect(textarea().value).toBe("");
  });

  it("can't preview or write an empty box", () => {
    setup();
    expect((button(dict.importPreview) as HTMLButtonElement).disabled).toBe(true);
    expect((button(dict.importConfirm) as HTMLButtonElement).disabled).toBe(true);
    type("x");
    expect((button(dict.importPreview) as HTMLButtonElement).disabled).toBe(false);
  });

  it("warns when the first line is data, because the server drops line 1", () => {
    setup();
    expect(screen.queryByText(dict.wealthImportNoHeader)).toBeNull();
    type("asset,deposit,活存,liquid,,TWD,100,,,,,,,,,,,");
    expect(screen.getByText(dict.wealthImportNoHeader)).not.toBeNull();
    type("\n\n  Liability,loan,貸款,hard,,TWD,100,,,,,,,,,,,");
    expect(screen.getByText(dict.wealthImportNoHeader)).not.toBeNull();
    type(`${WEALTH_IMPORT_HEADER}\nasset,deposit,活存,liquid,,TWD,100,,,,,,,,,,,`);
    expect(screen.queryByText(dict.wealthImportNoHeader)).toBeNull();
    type("");
    expect(screen.queryByText(dict.wealthImportNoHeader)).toBeNull();
  });
});

describe("the row-by-row result", () => {
  async function previewIt() {
    const handlers = setup();
    type("csv text");
    fireEvent.click(button(dict.importPreview));
    await screen.findByText(dict.wealthImportSummary);
    return handlers;
  }

  it("previews without writing and shows one tagged row per line", async () => {
    await previewIt();
    expect(m.importWealthCSV).toHaveBeenCalledWith("csv text", true);
    const tags = [...document.querySelectorAll(".wimp-tag")].map((t) => [t.className, t.textContent]);
    expect(tags).toEqual([
      ["wimp-tag wimp-tag-ok", dict.importStatusOk],
      ["wimp-tag wimp-tag-ok", dict.importStatusOk],
      ["wimp-tag wimp-tag-duplicate", dict.importStatusDuplicate],
      ["wimp-tag wimp-tag-error", dict.importStatusError],
    ]);
    expect(screen.getByText("TWD 350,000")).not.toBeNull();
    expect(screen.getByText("USD 1,200")).not.toBeNull();
    expect(screen.getByText(preview[3].message)).not.toBeNull(); // an error says what the server said
    expect(screen.getByText(dict.wealthImportDuplicateMsg)).not.toBeNull(); // a duplicate says it in the page's language
    expect(screen.queryByText(preview[2].message)).toBeNull();
  });

  it("counts them: ok and error always, duplicates and the rest only when there are some", async () => {
    await previewIt();
    const counts = [...document.querySelectorAll(".wimp-count")].map((c) => c.textContent);
    expect(counts).toEqual([`${dict.importStatusOk} 2`, `${dict.importStatusDuplicate} 1`, `${dict.importStatusError} 1`]);
  });

  it("shows — rather than 0 for a row that failed before its value was read", async () => {
    await previewIt();
    const cells = [...document.querySelectorAll("tbody tr")[3].querySelectorAll("td")].map((c) => c.textContent);
    expect(cells.slice(0, 7)).toEqual(["5", "—", "—", "—", "—", "—", "—"]);
  });

  it("keeps errors quiet at zero", async () => {
    m.importWealthCSV.mockResolvedValue({ rows: [preview[0]], applied: 0, dryRun: true });
    setup();
    type("csv text");
    fireEvent.click(button(dict.importPreview));
    await screen.findByText(dict.wealthImportSummary);
    const errors = document.querySelector(".wimp-count-error")!;
    expect(errors.textContent).toBe(`${dict.importStatusError} 0`);
    expect(errors.className).toContain("none");
  });

  it("writes only after a preview with something to write, then shows what was applied", async () => {
    const { onSuccess } = setup();
    type("csv text");
    expect((button(dict.importConfirm) as HTMLButtonElement).disabled).toBe(true); // nothing previewed yet
    fireEvent.click(button(dict.importPreview));
    await waitFor(() => expect((button(dict.importConfirm) as HTMLButtonElement).disabled).toBe(false));

    m.importWealthCSV.mockResolvedValue({
      rows: [row({ line: 2, status: "applied" }), row({ line: 3, status: "applied" }), preview[2], preview[3]],
      applied: 2,
      dryRun: false,
    });
    fireEvent.click(button(dict.importConfirm));
    await waitFor(() => expect(document.querySelector(".success-message")?.textContent).toBe(`${dict.importAppliedPrefix}2${dict.importAppliedSuffix}`));
    expect(m.importWealthCSV).toHaveBeenLastCalledWith("csv text", false);
    expect(onSuccess).toHaveBeenCalledTimes(1);
    expect(document.querySelectorAll(".wimp-tag-applied")).toHaveLength(2);
    expect((button(dict.importConfirm) as HTMLButtonElement).disabled).toBe(true); // nothing left to write
  });

  it("has nothing to write when every row is a duplicate or an error", async () => {
    m.importWealthCSV.mockResolvedValue({ rows: [preview[2], preview[3]], applied: 0, dryRun: true });
    setup();
    type("csv text");
    fireEvent.click(button(dict.importPreview));
    await screen.findByText(dict.wealthImportSummary);
    expect((button(dict.importConfirm) as HTMLButtonElement).disabled).toBe(true);
  });

  it("drops a stale result once the text changes", async () => {
    await previewIt();
    type("csv text, edited");
    expect(screen.queryByText(dict.wealthImportSummary)).toBeNull();
  });

  it("drops it when a sample is loaded over the text too", async () => {
    await previewIt();
    fireEvent.click(button(dict.wealthImportLoadSample));
    expect(screen.queryByText(dict.wealthImportSummary)).toBeNull();
  });

  it("can write a row that only drew a warning", async () => {
    m.importWealthCSV.mockResolvedValue({ rows: [row({ line: 2, status: "warning" })], applied: 0, dryRun: true });
    setup();
    type("csv text");
    fireEvent.click(button(dict.importPreview));
    await screen.findByText(dict.wealthImportSummary);
    expect((button(dict.importConfirm) as HTMLButtonElement).disabled).toBe(false);
    expect(document.querySelector(".wimp-count-warning")?.textContent).toBe(`${dict.importStatusWarning} 1`);
  });

  it("says so when no rows came back", async () => {
    m.importWealthCSV.mockResolvedValue({ rows: [], applied: 0, dryRun: true });
    setup();
    type(WEALTH_IMPORT_HEADER);
    fireEvent.click(button(dict.importPreview));
    expect(await screen.findByText(dict.importNoRows)).not.toBeNull();
  });

  it("signs in and retries when the write is refused with 401", async () => {
    m.importWealthCSV.mockRejectedValueOnce(new api.ApiError(401, "login"));
    const { onUnauthorized } = setup();
    type("csv text");
    fireEvent.click(button(dict.importPreview));
    await waitFor(() => expect(onUnauthorized).toHaveBeenCalledTimes(1));
    onUnauthorized.mock.calls[0][0]();
    await screen.findByText(dict.wealthImportSummary);
    expect(m.importWealthCSV).toHaveBeenCalledTimes(2);
  });

  it("shows another failure's message", async () => {
    m.importWealthCSV.mockRejectedValueOnce(new api.ApiError(400, "parse csv: bad quote"));
    setup();
    type("csv text");
    fireEvent.click(button(dict.importPreview));
    expect(await screen.findByText("parse csv: bad quote")).not.toBeNull();
  });
});

describe("read-only", () => {
  it("keeps the format and the guide, swaps the editor for the explanation, and shares the how-to toggle", () => {
    const onToggleHow = vi.fn();
    setup({ howOpen: false, onToggleHow });
    expect(screen.getByText(dict.wealthImportTitle)).not.toBeNull();
    expect(document.querySelectorAll(".wimp-guide-item").length).toBe(dict.wealthImportGuide.length);
    expect(screen.getByText(dict.roImportBodyWealth)).not.toBeNull();
    expect(screen.queryByRole("textbox")).toBeNull();
    expect(screen.queryByRole("button", { name: dict.importPreview })).toBeNull();
    fireEvent.click(button(dict.roHow));
    expect(onToggleHow).toHaveBeenCalledTimes(1);
  });
});
