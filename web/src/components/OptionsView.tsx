import { useEffect, useRef, useState } from "react";
import { fetchOptions, type Market, type OptionClosedTrade, type OptionPosition, type Options } from "../api";
import type { Dictionary } from "../i18n";
import {
  contractLabel,
  daysUntil,
  fmtSignedWhole,
  fmtWhole,
  kindLabel,
  kindOf,
  OptionCloseModal,
  OptionClosedModal,
  OptionFormModal,
} from "./OptionModal";

interface Props {
  dict: Dictionary;
  market: Market;
  writable: boolean;
  onUnauthorized: (retry: () => void) => void;
}

function fmtMoney(v: number): string {
  const sign = v < 0 ? "-" : "";
  return `${sign}$${Math.abs(v).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
}

function fmtContracts(v: number): string {
  const sign = v > 0 ? "+" : "";
  return `${sign}${v}`;
}

// unrealizedPnL mirrors db.RecordOption's realized-P&L formula with the
// exit price swapped for the live mark — (mark − avgPremium) × contracts ×
// multiplier is sign-correct for both a long (positive contracts, gains as
// mark rises) and a short (negative contracts, gains as mark falls) without
// a separate branch. 0 when mark is unavailable (fetchOptionMarkAndGreeks
// degraded this row), matching the "—" the mark/delta columns already show.
function unrealizedPnL(p: OptionPosition): number {
  if (p.mark <= 0) return 0;
  return (p.mark - p.avgPremium) * p.contracts * p.multiplier;
}

// dteClass and calendarDotColor share the design's urgency bands: red inside
// a week, amber inside three weeks.
function dteClass(dte: number): string {
  return dte <= 7 ? "loss" : dte <= 21 ? "opt-amber" : "";
}

function calendarDotColor(dte: number): string {
  if (dte <= 7) return "var(--loss)";
  if (dte <= 21) return "var(--opt-amber)";
  return "var(--accent)";
}

function actionLabel(dict: Dictionary, action: string): string {
  const labels: Record<string, string> = {
    STC: dict.optSellToClose,
    BTC: dict.optBuyToClose,
    EXPIRED: dict.optExpiredBtn,
    ASSIGNED: dict.optAssignedBtn,
    EXERCISED: dict.optExercisedBtn,
  };
  return labels[action] ?? action;
}

type ModalState =
  | { kind: "form"; position?: OptionPosition }
  | { kind: "close"; position: OptionPosition }
  | { kind: "closed"; record: OptionClosedTrade };

// The /options page (Phase 12 PR4, docs/phase-12-options.md §4): open
// contracts with live mark/greeks/DTE, closed trade history, an expiry
// calendar, and the same collateral summary /api/risk shows (CSP locked
// cash, covered-call locked shares, naked-call flag). No P&L curve here —
// deliberately: daily_snapshots/DailyPnL don't cover option market value (no
// free historical option price source), so this page only ever shows
// realized P&L in the closed table, never a portfolio-value line that would
// silently omit open option exposure. The write modals (OptionModal.tsx) are
// gated on `writable` exactly like PositionsTable's buy/sell/stop buttons.
export function OptionsView({ dict, market, writable, onUnauthorized }: Props) {
  const [options, setOptions] = useState<Options | null>(null);
  const [error, setError] = useState(false);
  const [modal, setModal] = useState<ModalState | null>(null);
  const [toast, setToast] = useState<string | null>(null);
  const toastTimer = useRef<number | undefined>(undefined);

  function reload() {
    fetchOptions(market)
      .then(setOptions)
      .catch(() => setError(true));
  }

  function done(message: string) {
    setModal(null);
    reload();
    setToast(message);
    window.clearTimeout(toastTimer.current);
    toastTimer.current = window.setTimeout(() => setToast(null), 2600);
  }

  useEffect(() => {
    setOptions(null);
    setError(false);
    reload();
  }, [market]);

  if (error) {
    return <div className="error-message">{dict.error}</div>;
  }
  if (!options) {
    return <div className="loading">{dict.loading}</div>;
  }

  const { positions, closed, calendar, collateral, held } = options;
  const labelBySymbol = new Map(positions.map((p) => [p.contractSymbol, contractLabel(p)]));
  const openContracts = positions.reduce((s, p) => s + Math.abs(p.contracts), 0);
  const thisYear = String(new Date().getFullYear());
  const closedThisYear = closed.filter((c) => c.date.startsWith(thisYear));
  const premiumYtd = closedThisYear.reduce((s, c) => s + c.realizedPnL, 0);
  const modalProps = { dict, onClose: () => setModal(null), onDone: done, onUnauthorized };

  return (
    <>
      <div className="stat-grid opt-kpis">
        <div className="card">
          <div className="eyebrow">{dict.optionLockedCash}</div>
          <div className="kpi-value">{fmtWhole(collateral.lockedCash)}</div>
          <div className="stat-note">
            {collateral.positions.length} {dict.optNoteCovered}
          </div>
        </div>
        <div className="card">
          <div className="eyebrow">{dict.optionOpenContracts}</div>
          <div className="kpi-value">{openContracts}</div>
          <div className="stat-note">
            {calendar.length} {dict.optNoteExpiries}
          </div>
        </div>
        <div className="card">
          <div className="eyebrow">{dict.optionPremiumYtd}</div>
          <div className={`kpi-value ${premiumYtd > 0 ? "profit" : premiumYtd < 0 ? "loss" : ""}`}>
            {fmtSignedWhole(premiumYtd)}
          </div>
          <div className="stat-note">
            {closedThisYear.length} {dict.optNoteClosed}
          </div>
        </div>
      </div>

      <div className="card">
        <div className="opt-card-header">
          <div className="eyebrow">{dict.optionOpenTitle}</div>
          <span style={{ flex: 1 }} />
          {writable && (
            <button className="btn-tint" onClick={() => setModal({ kind: "form" })}>
              {dict.optAddBtn}
            </button>
          )}
        </div>
        {positions.length === 0 ? (
          <div className="empty-message">{dict.noOptionPositions}</div>
        ) : (
          <table className="mono">
            <thead>
              <tr>
                <th>{dict.optionContract}</th>
                <th>{dict.optionKind}</th>
                <th>{dict.optionExpiry}</th>
                <th>{dict.optionDTE}</th>
                <th>{dict.optionContracts}</th>
                <th>{dict.optionAvgPremium}</th>
                <th>{dict.optionMark}</th>
                <th>{dict.optionMarketValue}</th>
                <th>{dict.unrealizedPnL}</th>
                <th>{dict.optionDelta}</th>
                {writable && <th />}
              </tr>
            </thead>
            <tbody>
              {positions.map((p) => {
                const pnl = unrealizedPnL(p);
                const dte = daysUntil(p.expiry);
                return (
                  <tr key={p.contractSymbol}>
                    <td>
                      <span className="opt-contract-cell">
                        {contractLabel(p)}
                        {p.edited && <span className="opt-edited-tag">{dict.optEditedTag}</span>}
                      </span>
                    </td>
                    <td className="opt-kind-cell">{kindLabel(dict, kindOf(p))}</td>
                    <td>{p.expiry}</td>
                    <td className={dteClass(dte)}>{dte}d</td>
                    <td className={p.contracts >= 0 ? "profit" : "loss"}>{fmtContracts(p.contracts)}</td>
                    <td>{fmtMoney(p.avgPremium)}</td>
                    <td>{p.mark > 0 ? fmtMoney(p.mark) : "—"}</td>
                    <td className={p.mark > 0 ? (p.marketValue >= 0 ? "profit" : "loss") : ""}>
                      {p.mark > 0 ? fmtSignedWhole(p.marketValue) : "—"}
                    </td>
                    <td className={p.mark > 0 ? (pnl >= 0 ? "profit" : "loss") : ""}>
                      {p.mark > 0 ? fmtSignedWhole(pnl) : "—"}
                    </td>
                    <td>{p.mark > 0 ? p.delta.toFixed(3) : "—"}</td>
                    {writable && (
                      <td className="opt-row-actions">
                        <span>
                          <button className="opt-row-edit" onClick={() => setModal({ kind: "form", position: p })}>
                            {dict.optEditBtn}
                          </button>
                          <button onClick={() => setModal({ kind: "close", position: p })}>{dict.optCloseBtn}</button>
                        </span>
                      </td>
                    )}
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>

      <div className="opt-split-grid">
        <div className="card">
          <div className="eyebrow" style={{ marginBottom: 4 }}>
            {dict.optionCollateral}
          </div>
          {collateral.positions.length > 0 && (
            <table className="mono">
              <thead>
                <tr>
                  <th>{dict.optionUnderlying}</th>
                  <th>{dict.optionLockedShares}</th>
                  <th>{dict.optionHeldShares}</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {collateral.positions.map((c) => (
                  <tr key={c.underlying}>
                    <td>{c.underlying}</td>
                    <td>{c.lockedShares.toLocaleString()}</td>
                    <td>{c.heldShares.toLocaleString()}</td>
                    <td>
                      <span className={c.naked ? "opt-collateral-tag naked" : "opt-collateral-tag"}>
                        {c.naked ? dict.optionNaked : dict.optionCovered}
                      </span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
          <div className="stat-note opt-table-note">{dict.optionCollateralNote}</div>
        </div>

        <div className="card">
          <div className="eyebrow">{dict.optionCalendar}</div>
          {calendar.length === 0 ? (
            <div className="empty-message">{dict.noOptionPositions}</div>
          ) : (
            <div style={{ marginTop: 10 }}>
              {calendar.map((entry) => {
                const dte = daysUntil(entry.date);
                return (
                  <div className="opt-calendar-row" key={entry.date}>
                    <span className="opt-calendar-dot" style={{ background: calendarDotColor(dte) }} />
                    <span className="opt-calendar-date">{entry.date}</span>
                    <span className={`opt-calendar-dte ${dteClass(dte)}`}>{dte}d</span>
                    <span className="opt-calendar-symbols">
                      {entry.symbols.map((s) => labelBySymbol.get(s) ?? s).join(" · ")}
                    </span>
                  </div>
                );
              })}
            </div>
          )}
        </div>
      </div>

      <div className="card">
        <div className="eyebrow" style={{ marginBottom: 4 }}>
          {dict.optionClosedTitle}
        </div>
        {closed.length === 0 ? (
          <div className="empty-message">{dict.noClosedOptions}</div>
        ) : (
          <table className="mono">
            <thead>
              <tr>
                <th>{dict.optionContract}</th>
                <th>{dict.optionAction}</th>
                <th>{dict.optionContracts}</th>
                <th>{dict.exitDate}</th>
                <th>{dict.realizedPnL}</th>
                {writable && <th />}
              </tr>
            </thead>
            <tbody>
              {closed.map((c) => (
                <tr key={c.id}>
                  <td>
                    <span className="opt-contract-cell">
                      {contractLabel(c)}
                      {c.edited && <span className="opt-edited-tag">{dict.optEditedTag}</span>}
                    </span>
                  </td>
                  <td>
                    <span className="opt-action-tag">{actionLabel(dict, c.action)}</span>
                  </td>
                  <td>{c.contracts}</td>
                  <td className="opt-date-cell">{c.date}</td>
                  <td className={c.realizedPnL >= 0 ? "profit" : "loss"}>{fmtSignedWhole(c.realizedPnL)}</td>
                  {writable && (
                    <td className="opt-row-actions" style={{ width: "1%" }}>
                      <button className="opt-row-edit" onClick={() => setModal({ kind: "closed", record: c })}>
                        {dict.optEditBtn}
                      </button>
                    </td>
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        )}
        <div className="stat-note opt-table-note">{dict.optionNoPnlNote}</div>
      </div>

      {modal?.kind === "form" && <OptionFormModal {...modalProps} position={modal.position} held={held} />}
      {modal?.kind === "close" && <OptionCloseModal {...modalProps} position={modal.position} />}
      {modal?.kind === "closed" && <OptionClosedModal {...modalProps} record={modal.record} />}
      {toast && <div className="opt-toast">{toast}</div>}
    </>
  );
}
