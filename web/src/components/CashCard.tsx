import { useState, type FormEvent } from "react";
import { ApiError, currencySymbol, saveCash, type Market, type Risk } from "../api";
import type { Dictionary } from "../i18n";

interface Props {
  dict: Dictionary;
  market: Market;
  risk: Risk;
  writable: boolean;
  onSaved: () => void;
  onUnauthorized: (retry: () => void) => void;
}

function fmtAmount(v: number, currency: string): string {
  return currency + v.toLocaleString("en-US", { maximumFractionDigits: 2 });
}

// Digits and one decimal point — the design's input filter, so a pasted
// "12,400" becomes 12400 instead of failing the save.
function cleanAmount(raw: string): string {
  const v = raw.replace(/[^0-9.]/g, "");
  const i = v.indexOf(".");
  return i < 0 ? v : v.slice(0, i + 1) + v.slice(i + 1).replace(/\./g, "");
}

// The risk page's 現金水位 card: the declared cash balance for this market,
// edited in place. Without a declared balance the account value is
// positions only (see Risk.hasCash), so the unset state is a warning, not
// a 0%.
export function CashCard({ dict, market, risk, writable, onSaved, onUnauthorized }: Props) {
  const [editing, setEditing] = useState(false);
  const [value, setValue] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const currency = currencySymbol(market);
  const ratio = risk.accountValue !== 0 ? `${((risk.cash / risk.accountValue) * 100).toFixed(1)}%` : "0.0%";
  const amount = fmtAmount(risk.cash, currency);

  function startEdit() {
    setValue(risk.hasCash ? String(risk.cash) : "");
    setError(null);
    setEditing(true);
  }

  async function save() {
    const n = parseFloat(value);
    if (value === "" || Number.isNaN(n)) {
      setError(dict.cashErrEmpty);
      return;
    }
    setSaving(true);
    setError(null);
    try {
      await saveCash(market, n);
      setEditing(false);
      onSaved();
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        onUnauthorized(save);
      } else {
        setError(dict.cashSaveFailed.replace("%s", e instanceof ApiError ? e.message : dict.error));
      }
    } finally {
      setSaving(false);
    }
  }

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (!saving) void save();
  }

  const inputId = `cash-input-${market}`;
  const unset = !risk.hasCash;

  return (
    <section className={`card cash-card${unset && !editing && writable ? " cash-card-unset" : ""}`} aria-label={dict.cashLevel}>
      <div className="cash-card-head">
        <span className="eyebrow">{dict.cashLevel}</span>
        <span className="cash-chip">{market === "tw" ? dict.cashChipTW : dict.cashChipUS}</span>
        {writable && !editing && !unset && (
          <button type="button" className="cash-edit-btn" onClick={startEdit}>
            {dict.cashEdit}
          </button>
        )}
      </div>

      {!editing && unset && (
        <div className="cash-card-body">
          <div className="kpi-value cash-dash">—</div>
          <div className="cash-warn" role="status">
            <strong>{dict.cashUnsetTitle}</strong>
            <span>{dict.cashUnsetDesc}</span>
          </div>
          {writable && (
            <div className="modal-actions cash-actions-start">
              <button type="button" className="btn-primary" onClick={startEdit}>
                {dict.cashSetBtn}
              </button>
            </div>
          )}
        </div>
      )}

      {!editing && !unset && (
        <div className="cash-card-body">
          <div className="cash-ratio">
            <span className="kpi-value">{ratio}</span>
            <span className="cash-sub">{dict.cashOfTotal}</span>
          </div>
          <div className="cash-amount mono">
            {dict.cashAmount} {amount}
          </div>
          {risk.cashSyncedFromBroker && <div className="cash-sub">{dict.cashSinoShort}</div>}
        </div>
      )}

      {editing && (
        <form className="cash-card-body" onSubmit={onSubmit}>
          <div className="cash-sub mono">
            {risk.hasCash ? `${dict.cashCurrent} ${ratio} · ${amount}` : dict.cashNotSetYet}
          </div>
          <label htmlFor={inputId} className="cash-label">
            {market === "tw" ? dict.cashInputTW : dict.cashInputUS}
          </label>
          <div className={`cash-input${error ? " cash-input-error" : ""}`}>
            <span aria-hidden="true">{currency}</span>
            <input
              id={inputId}
              type="text"
              inputMode="decimal"
              autoComplete="off"
              autoFocus
              value={value}
              placeholder={dict.cashPlaceholder}
              disabled={saving}
              aria-invalid={!!error}
              aria-describedby={error ? `${inputId}-err` : undefined}
              onChange={(e) => setValue(cleanAmount(e.target.value))}
            />
          </div>
          {error && (
            <div id={`${inputId}-err`} className="cash-error" role="alert">
              {error}
            </div>
          )}
          {risk.cashSyncedFromBroker && <div className="cash-sino">{dict.cashSinoNote}</div>}
          <p className="cash-note">{dict.cashNote}</p>
          <div className="modal-actions">
            <button type="button" disabled={saving} onClick={() => setEditing(false)}>
              {dict.cashCancel}
            </button>
            <button type="submit" className="btn-primary" disabled={saving}>
              {saving ? dict.cashSaving : dict.cashSave}
            </button>
          </div>
        </form>
      )}
    </section>
  );
}
