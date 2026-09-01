import { useEffect, useState } from "react";
import { fetchOptions, type Market, type OptionPosition, type Options } from "../api";
import type { Dictionary } from "../i18n";
import { OptionModal, type OptionModalMode } from "./OptionModal";

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

function fmtSignedMoney(v: number): string {
  const sign = v > 0 ? "+" : v < 0 ? "-" : "";
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

// calendarDotColor flags an expiry bucket the same way ChartView's other
// urgency cues do: red inside a week, amber inside a month, neutral beyond
// that — glanceable without opening each contract's own DTE column.
function calendarDotColor(dte: number): string {
  if (dte <= 7) return "var(--loss)";
  if (dte <= 30) return "var(--s1)";
  return "var(--ink-3)";
}

function daysUntil(date: string): number {
  const ms = new Date(`${date}T00:00:00`).getTime() - new Date().setHours(0, 0, 0, 0);
  return Math.round(ms / 86400000);
}

// The /options page (Phase 12 PR4, docs/phase-12-options.md §4): open
// contracts with live mark/greeks/DTE, closed trade history, an expiry
// calendar, and the same collateral summary /api/risk shows (CSP locked
// cash, covered-call locked shares, naked-call flag). No P&L curve here —
// deliberately: daily_snapshots/DailyPnL don't cover option market value (no
// free historical option price source), so this page only ever shows
// realized P&L in the closed-trades table, never a portfolio-value line
// that would silently omit open option exposure. Design-parity pass adds
// the KPI summary row, the Add/Close write modals (OptionModal, gated on
// `writable` exactly like PositionsTable's buy/sell/stop buttons), and
// restyles the collateral/calendar cards to match the rest of the
// dashboard's card system.
export function OptionsView({ dict, market, writable, onUnauthorized }: Props) {
  const [options, setOptions] = useState<Options | null>(null);
  const [error, setError] = useState(false);
  const [modal, setModal] = useState<{ mode: OptionModalMode; position?: OptionPosition } | null>(null);

  function reload() {
    fetchOptions(market)
      .then(setOptions)
      .catch(() => setError(true));
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

  const { positions, closed, calendar, collateral } = options;
  const totalMarketValue = positions.reduce((s, p) => s + p.marketValue, 0);
  const totalUnrealized = positions.reduce((s, p) => s + unrealizedPnL(p), 0);

  return (
    <>
      <div className="stat-grid">
        <div className="card">
          <div className="eyebrow">{dict.positions}</div>
          <div className="kpi-value">{positions.length}</div>
        </div>
        <div className="card">
          <div className="eyebrow">{dict.optionMarketValue}</div>
          <div className="kpi-value">{fmtMoney(totalMarketValue)}</div>
        </div>
        <div className="card">
          <div className="eyebrow">{dict.unrealizedPnL}</div>
          <div className={`kpi-value ${totalUnrealized > 0 ? "profit" : totalUnrealized < 0 ? "loss" : ""}`}>
            {fmtSignedMoney(totalUnrealized)}
          </div>
        </div>
      </div>

      <div className="card">
        <div className="opt-card-header">
          <div className="eyebrow">{dict.positions}</div>
          <span style={{ flex: 1 }} />
          {writable && (
            <button className="btn-tint" onClick={() => setModal({ mode: "open" })}>
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
                <th>{dict.optionRight}</th>
                <th>{dict.optionStrike}</th>
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
                return (
                  <tr key={p.contractSymbol}>
                    <td>{p.underlying}</td>
                    <td>{p.right === "C" ? dict.optCall : dict.optPut}</td>
                    <td>{p.strike}</td>
                    <td>{p.expiry}</td>
                    <td>{p.dte}</td>
                    <td className={p.contracts >= 0 ? "profit" : "loss"}>{fmtContracts(p.contracts)}</td>
                    <td>{fmtMoney(p.avgPremium)}</td>
                    <td>{p.mark > 0 ? fmtMoney(p.mark) : "—"}</td>
                    <td className={p.marketValue >= 0 ? "profit" : "loss"}>{fmtMoney(p.marketValue)}</td>
                    <td className={p.mark > 0 ? (pnl >= 0 ? "profit" : "loss") : ""}>
                      {p.mark > 0 ? fmtSignedMoney(pnl) : "—"}
                    </td>
                    <td>{p.mark > 0 ? p.delta.toFixed(3) : "—"}</td>
                    {writable && (
                      <td className="row-actions">
                        <button onClick={() => setModal({ mode: "close", position: p })}>{dict.optCloseBtn}</button>
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
          <div className="eyebrow">{dict.optionCollateral}</div>
          <div className="kpi-value" style={{ marginTop: 8, marginBottom: 4 }}>
            {fmtMoney(collateral.lockedCash)}
          </div>
          <div className="stat-note">{dict.optionLockedCash}</div>
          {collateral.positions.length > 0 && (
            <table className="mono" style={{ marginTop: "1rem" }}>
              <thead>
                <tr>
                  <th>{dict.ticker}</th>
                  <th>{dict.optionLockedShares}</th>
                  <th>{dict.optionHeldShares}</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {collateral.positions.map((c) => (
                  <tr key={c.underlying}>
                    <td>{c.underlying}</td>
                    <td>{c.lockedShares}</td>
                    <td>{c.heldShares}</td>
                    <td>{c.naked && <span className="tag warn">{dict.optionNaked}</span>}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
          <div className="stat-note">{dict.optionCollateralNote}</div>
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
                    <span className="opt-calendar-dte" style={{ color: calendarDotColor(dte) }}>
                      {dte}d
                    </span>
                    <span className="opt-calendar-symbols">{entry.symbols.join(", ")}</span>
                  </div>
                );
              })}
            </div>
          )}
        </div>
      </div>

      <div className="card">
        <div className="eyebrow">{dict.closedPositions}</div>
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
              </tr>
            </thead>
            <tbody>
              {closed.map((c, i) => (
                <tr key={`${c.contractSymbol}-${i}`}>
                  <td>
                    {c.underlying} {c.right} {c.strike}
                  </td>
                  <td>
                    <span className="tag">{c.action}</span>
                  </td>
                  <td>{c.contracts}</td>
                  <td>{c.date}</td>
                  <td className={c.realizedPnL >= 0 ? "profit" : "loss"}>{fmtMoney(c.realizedPnL)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        <div className="stat-note">{dict.optionNoPnlNote}</div>
      </div>

      {modal && (
        <OptionModal
          dict={dict}
          mode={modal.mode}
          position={modal.position}
          onClose={() => setModal(null)}
          onSuccess={reload}
          onUnauthorized={onUnauthorized}
        />
      )}
    </>
  );
}
