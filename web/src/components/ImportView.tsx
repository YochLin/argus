import { useRef, useState } from "react";
import { ApiError, importCSV, type ImportResult, type ImportRow } from "../api";
import type { Dictionary } from "../i18n";

interface Props {
  dict: Dictionary;
  onUnauthorized: (retry: () => void) => void;
  onSuccess: () => void;
}

const statusLabelKey: Record<ImportRow["status"], keyof Dictionary> = {
  ok: "importStatusOk",
  warning: "importStatusWarning",
  duplicate: "importStatusDuplicate",
  error: "importStatusError",
  applied: "importStatusApplied",
};

// CSV import (Phase 5 §B, optional) — one page, two calls to the same
// /api/import endpoint: Preview (dryRun) shows what would happen, Confirm
// (dryRun=false) actually writes. No separate stateful preview session —
// the client just resends the same CSV text.
export function ImportView({ dict, onUnauthorized, onSuccess }: Props) {
  const [csv, setCsv] = useState("");
  const [result, setResult] = useState<ImportResult | null>(null);
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
      const res = await importCSV(csv, dryRun);
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
        <div className="eyebrow">{dict.importTitle}</div>
        <p>{dict.importInstructions}</p>
        <p className="mono">{dict.importTemplateHint}</p>
        <textarea
          className="mono import-textarea"
          rows={8}
          value={csv}
          placeholder={dict.importTextareaPlaceholder}
          onChange={(e) => {
            setCsv(e.target.value);
            setResult(null);
          }}
        />
        <div className="modal-actions">
          <input ref={fileInputRef} type="file" accept=".csv,text/csv" onChange={handleFile} />
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
                  <th>{dict.importDate}</th>
                  <th>{dict.ticker}</th>
                  <th>{dict.side}</th>
                  <th>{dict.shares}</th>
                  <th>{dict.price}</th>
                  <th>{dict.fee}</th>
                  <th>{dict.importStatus}</th>
                  <th>{dict.importMessage}</th>
                </tr>
              </thead>
              <tbody>
                {result.rows.map((row) => (
                  <tr key={row.line}>
                    <td>{row.line}</td>
                    <td>{row.date}</td>
                    <td>{row.ticker}</td>
                    <td>{row.action}</td>
                    <td>{row.shares}</td>
                    <td>{row.price}</td>
                    <td>{row.fee}</td>
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
