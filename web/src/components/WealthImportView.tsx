import { useRef, useState } from "react";
import { ApiError, importWealthCSV, type WealthImportResult, type WealthImportRow } from "../api";
import type { Dictionary } from "../i18n";
import { ReadOnlyImportPanel } from "./ReadOnlyBanner";

interface Props {
  dict: Dictionary;
  onUnauthorized: (retry: () => void) => void;
  onSuccess: () => void;
  // Set while the server has no WEB_PASSWORD; howOpen/onToggleHow are the
  // shell banner's "how to enable editing" state, shared with this panel.
  readOnly?: { howOpen: boolean; onToggleHow: () => void };
}

const statusLabelKey: Record<WealthImportRow["status"], keyof Dictionary> = {
  ok: "importStatusOk",
  warning: "importStatusWarning",
  duplicate: "importStatusDuplicate",
  error: "importStatusError",
  applied: "importStatusApplied",
};

// The columns the server reads, in order (internal/web/wealth_import.go): the
// header line of the template file and of "load sample". The server always
// skips the first line, so a pasted CSV needs one.
export const WEALTH_IMPORT_HEADER =
  "side,type,name,group,venue,currency,value,date,bank,accountNote,lender,ratePct,originalPrincipal,remainingMonths,fundCode,fundPlatform,fundMonthlyAmount,fundNextContributionDate";

const templateText = (dict: Dictionary) => `${WEALTH_IMPORT_HEADER}\n${dict.wealthImportSample}\n`;

const BOM = String.fromCharCode(0xfeff);

// downloadWealthTemplate saves the header plus the sample rows as a CSV. The
// BOM is for Excel, which otherwise opens a UTF-8 file as Big5 and garbles the
// Chinese names; reading the file back through the upload button drops it.
export function downloadWealthTemplate(dict: Dictionary) {
  const url = URL.createObjectURL(new Blob([BOM + templateText(dict)], { type: "text/csv;charset=utf-8" }));
  const a = document.createElement("a");
  a.href = url;
  a.download = dict.wealthImportTemplateName;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

// Phase 9 波次1 PR3' (§8.15.1) — the initial-data-entry CSV importer that
// replaced PDF statement parsing. Same "paste → preview (dryRun) → confirm"
// shape as ImportView.tsx's trade importer, against /api/wealth/import
// instead of /api/import.
export function WealthImportView({ dict, onUnauthorized, onSuccess, readOnly }: Props) {
  const [csv, setCsv] = useState("");
  const [result, setResult] = useState<WealthImportResult | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  function handleFile(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file) return;
    file.text().then((text) => {
      setCsv(text);
      setResult(null);
    });
    e.target.value = "";
  }

  async function run(dryRun: boolean) {
    setBusy(true);
    setError(null);
    try {
      const res = await importWealthCSV(csv, dryRun);
      setResult(res);
      if (!dryRun) onSuccess();
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        onUnauthorized(() => run(dryRun));
      } else {
        setError(e instanceof ApiError ? e.message : dict.error);
      }
    } finally {
      setBusy(false);
    }
  }

  const canApply = result !== null && result.rows.some((r) => r.status === "ok" || r.status === "warning");
  // The server drops the first line whatever it is, so a CSV that starts with
  // a data row loses it without a word.
  const firstCell = csv.trimStart().split(/[,\r\n]/, 1)[0].trim().toLowerCase();
  const missingHeader = firstCell === "asset" || firstCell === "liability";

  const header = (
    <div style={{ display: "flex", alignItems: "center", gap: 12, margin: "16px 0", flexWrap: "wrap" }}>
      <span style={{ fontFamily: "var(--font-mono)", textTransform: "uppercase", letterSpacing: ".08em", fontSize: 11, color: "var(--ink)" }}>
        {dict.navWealthImport}
      </span>
    </div>
  );

  // What the page is about, whether or not it can write: the format, and the
  // column guide to prepare a file by.
  const format = (
    <>
      <div className="eyebrow">{dict.wealthImportTitle}</div>
      <div className="wimp-instructions">{dict.wealthImportInstructions}</div>
      <div className="wimp-hint">
        {WEALTH_IMPORT_HEADER}　│　{dict.wealthImportHintNote}
      </div>
      <div className="wimp-guide">
        {dict.wealthImportGuide.map(([code, desc]) => (
          <span key={code} className={`wimp-guide-item${code === "type" ? " wide" : ""}`}>
            <span className="wimp-guide-code">{code}</span>
            <span>{desc}</span>
          </span>
        ))}
      </div>
    </>
  );

  // Read-only: keep the format and column guide (a CSV can still be prepared)
  // and the template, but swap the form for the explanation.
  if (readOnly) {
    return (
      <>
        {header}
        <div className="import-view">
          <div className="card">
            {format}
            <ReadOnlyImportPanel
              dict={dict}
              body={dict.roImportBodyWealth}
              howOpen={readOnly.howOpen}
              onToggleHow={readOnly.onToggleHow}
              onDownload={() => downloadWealthTemplate(dict)}
            />
          </div>
        </div>
      </>
    );
  }

  const count = (status: WealthImportRow["status"]) => result?.rows.filter((r) => r.status === status).length ?? 0;
  // ok and error always show, the rest only when there are some.
  const counts = (["ok", "warning", "duplicate", "error", "applied"] as const).filter(
    (s) => s === "ok" || s === "error" || count(s) > 0,
  );

  return (
    <>
      {header}
      <div className="import-view">
        <div className="card">
          {format}
          <textarea
            className="wimp-textarea"
            rows={9}
            spellCheck={false}
            value={csv}
            placeholder={dict.wealthImportTextareaPlaceholder}
            onChange={(e) => {
              setCsv(e.target.value);
              setResult(null);
            }}
          />
          {missingHeader && <div className="wimp-note">{dict.wealthImportNoHeader}</div>}
          <div className="wimp-actions">
            <input ref={fileInputRef} type="file" accept=".csv,text/csv" onChange={handleFile} style={{ display: "none" }} />
            <button type="button" className="wimp-btn" onClick={() => downloadWealthTemplate(dict)}>
              {dict.wealthImportDownload}
            </button>
            <button
              type="button"
              className="wimp-btn"
              onClick={() => {
                setCsv(templateText(dict));
                setResult(null);
              }}
            >
              {dict.wealthImportLoadSample}
            </button>
            <button type="button" className="wimp-btn" onClick={() => fileInputRef.current?.click()}>
              {dict.importChooseFile}
            </button>
            <button
              type="button"
              className="wimp-btn"
              onClick={() => {
                setCsv("");
                setResult(null);
                setError(null);
              }}
            >
              {dict.wealthImportClear}
            </button>
            <span style={{ flex: 1 }} />
            <button type="button" className="wimp-btn wimp-btn-preview" disabled={busy || !csv.trim()} onClick={() => run(true)}>
              {dict.importPreview}
            </button>
            <button type="button" className="wimp-btn wimp-btn-confirm" disabled={busy || !canApply} onClick={() => run(false)}>
              {dict.importConfirm}
            </button>
          </div>
          {error && <div className="error-message">{error}</div>}
          {result && result.applied > 0 && (
            <div className="success-message">
              {dict.importAppliedPrefix}
              {result.applied}
              {dict.importAppliedSuffix}
            </div>
          )}
        </div>

        {result && (
          <div className="card" style={{ overflowX: "auto" }}>
            <div className="wimp-summary">
              <span className="eyebrow">{dict.wealthImportSummary}</span>
              {counts.map((s) => (
                <span key={s} className={`wimp-count wimp-count-${s}${count(s) === 0 ? " none" : ""}`}>
                  {dict[statusLabelKey[s]]} {count(s)}
                </span>
              ))}
            </div>
            {result.rows.length === 0 ? (
              <div className="empty-message">{dict.importNoRows}</div>
            ) : (
              <table className="mono wimp-table">
                <thead>
                  <tr>
                    <th>{dict.importLine}</th>
                    <th className="wimp-left">{dict.wealthImportColSide}</th>
                    <th className="wimp-left">{dict.wealthImportColType}</th>
                    <th className="wimp-left">{dict.wealthImportColName}</th>
                    <th className="wimp-left">{dict.wealthImportColGroup}</th>
                    <th>{dict.wealthImportColValue}</th>
                    <th>{dict.importDate}</th>
                    <th>{dict.importStatus}</th>
                    <th className="wimp-left">{dict.importMessage}</th>
                  </tr>
                </thead>
                <tbody>
                  {result.rows.map((row) => (
                    <tr key={row.line}>
                      <td className="wimp-line">{row.line}</td>
                      <td className="wimp-kind">{row.side || "—"}</td>
                      <td className="wimp-kind">{row.type || "—"}</td>
                      <td className="wimp-name">{row.name || "—"}</td>
                      <td className="wimp-group">{row.group || "—"}</td>
                      {/* date is only set once the value has parsed: before that value is just 0 */}
                      <td>{row.date ? `${row.currency || "TWD"} ${row.value.toLocaleString("en-US")}` : "—"}</td>
                      <td>{row.date || "—"}</td>
                      <td>
                        <span className={`wimp-tag wimp-tag-${row.status}`}>{dict[statusLabelKey[row.status]]}</span>
                      </td>
                      {/* a duplicate means one thing, so say it in the page's language; the other messages are the server's own */}
                      <td className="wimp-msg">{row.status === "duplicate" ? dict.wealthImportDuplicateMsg : row.message}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        )}
      </div>
    </>
  );
}
