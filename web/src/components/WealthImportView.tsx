import { useRef, useState } from "react";
import { ApiError, importWealthCSV, type WealthImportResult, type WealthImportRow } from "../api";
import type { Dictionary } from "../i18n";

interface Props {
  dict: Dictionary;
  onUnauthorized: (retry: () => void) => void;
  onSuccess: () => void;
}

const statusLabelKey: Record<WealthImportRow["status"], keyof Dictionary> = {
  ok: "importStatusOk",
  warning: "importStatusWarning",
  duplicate: "importStatusDuplicate",
  error: "importStatusError",
  applied: "importStatusApplied",
};

// Phase 9 波次1 PR3' (§8.15.1) — the initial-data-entry CSV importer that
// replaced PDF statement parsing. Same "paste → preview (dryRun) → confirm"
// shape as ImportView.tsx's trade importer, against /api/wealth/import
// instead of /api/import.
export function WealthImportView({ dict, onUnauthorized, onSuccess }: Props) {
  const [csv, setCsv] = useState("");
  const [result, setResult] = useState<WealthImportResult | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  function handleFile(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file) return;
    file.text().then(setCsv);
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

  return (
    <div className="import-view">
      <div className="card">
        <div className="eyebrow">{dict.wealthImportTitle}</div>
        <p>{dict.wealthImportInstructions}</p>
        <p className="mono">{dict.wealthImportTemplateHint}</p>
        <textarea
          className="mono import-textarea"
          rows={8}
          value={csv}
          placeholder={dict.wealthImportTextareaPlaceholder}
          onChange={(e) => {
            setCsv(e.target.value);
            setResult(null);
          }}
        />
        <div className="modal-actions">
          <input
            ref={fileInputRef}
            type="file"
            accept=".csv,text/csv"
            onChange={handleFile}
            style={{ display: "none" }}
          />
          <button type="button" onClick={() => fileInputRef.current?.click()}>
            {dict.importChooseFile}
          </button>
          <button disabled={busy || !csv.trim()} onClick={() => run(true)}>
            {dict.importPreview}
          </button>
          <button className="btn-primary" disabled={busy || !canApply} onClick={() => run(false)}>
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
        <div className="card">
          {result.rows.length === 0 ? (
            <div className="empty-message">{dict.importNoRows}</div>
          ) : (
            <table className="mono">
              <thead>
                <tr>
                  <th>{dict.importLine}</th>
                  <th>{dict.wealthImportColSide}</th>
                  <th>{dict.wealthImportColType}</th>
                  <th>{dict.wealthImportColName}</th>
                  <th>{dict.wealthImportColGroup}</th>
                  <th>{dict.wealthImportColValue}</th>
                  <th>{dict.importDate}</th>
                  <th>{dict.importStatus}</th>
                  <th>{dict.importMessage}</th>
                </tr>
              </thead>
              <tbody>
                {result.rows.map((row) => (
                  <tr key={row.line}>
                    <td>{row.line}</td>
                    <td>{row.side}</td>
                    <td>{row.type}</td>
                    <td>{row.name}</td>
                    <td>{row.group}</td>
                    <td>{row.value}</td>
                    <td>{row.date}</td>
                    <td className={row.status === "error" ? "loss" : row.status === "applied" ? "profit" : ""}>
                      {dict[statusLabelKey[row.status]]}
                    </td>
                    <td>{row.message}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      )}
    </div>
  );
}
