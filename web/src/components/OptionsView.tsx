import { useEffect, useState } from "react";
import { fetchOptions, type Market, type Options } from "../api";
import type { Dictionary } from "../i18n";

interface Props {
  dict: Dictionary;
  market: Market;
}

function fmtMoney(v: number): string {
  const sign = v < 0 ? "-" : "";
  return `${sign}$${Math.abs(v).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
}

function fmtContracts(v: number): string {
  const sign = v > 0 ? "+" : "";
  return `${sign}${v}`;
}

// The /options page (Phase 12 PR4, docs/phase-12-options.md §4): open
// contracts with live mark/greeks/DTE, closed trade history, an expiry
// calendar, and the same collateral summary /api/risk shows (CSP locked
// cash, covered-call locked shares, naked-call flag). No P&L curve here —
// deliberately: daily_snapshots/DailyPnL don't include option market value
// (no free historical option price source), so this page only ever shows
// realized P&L in the closed-trades table, never a portfolio-value line
// that would silently omit open option exposure.
export function OptionsView({ dict, market }: Props) {
  const [options, setOptions] = useState<Options | null>(null);
  const [error, setError] = useState(false);

  useEffect(() => {
    setOptions(null);
    setError(false);
    fetchOptions(market)
      .then(setOptions)
      .catch(() => setError(true));
  }, [market]);

  if (error) {
    return <div className="error-message">{dict.error}</div>;
  }
  if (!options) {
    return <div className="loading">{dict.loading}</div>;
  }

  const { positions, closed, calendar, collateral } = options;

  return (
    <>
      <div className="card">
        <div className="eyebrow">{dict.optionCollateral}</div>
        <div className="kpi-value">{fmtMoney(collateral.lockedCash)}</div>
        <div className="kpi-label">{dict.optionLockedCash}</div>
        {collateral.positions.length > 0 && (
          <table className="mono" style={{ marginTop: "1rem" }}>
            <thead>
              <tr>
                <th>{dict.optionContract}</th>
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
      </div>

      <div className="card">
        <div className="eyebrow">{dict.positions}</div>
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
                <th>{dict.optionDelta}</th>
              </tr>
            </thead>
            <tbody>
              {positions.map((p) => (
                <tr key={p.contractSymbol}>
                  <td>{p.underlying}</td>
                  <td>{p.right}</td>
                  <td>{p.strike}</td>
                  <td>{p.expiry}</td>
                  <td>{p.dte}</td>
                  <td className={p.contracts >= 0 ? "profit" : "loss"}>{fmtContracts(p.contracts)}</td>
                  <td>{fmtMoney(p.avgPremium)}</td>
                  <td>{p.mark > 0 ? fmtMoney(p.mark) : "—"}</td>
                  <td className={p.marketValue >= 0 ? "profit" : "loss"}>{fmtMoney(p.marketValue)}</td>
                  <td>{p.mark > 0 ? p.delta.toFixed(3) : "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
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
                  <td>{c.underlying} {c.right} {c.strike}</td>
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
      </div>

      <div className="card">
        <div className="eyebrow">{dict.optionCalendar}</div>
        {calendar.length === 0 ? (
          <div className="empty-message">{dict.noOptionPositions}</div>
        ) : (
          <table className="mono">
            <thead>
              <tr>
                <th>{dict.optionExpiry}</th>
                <th>{dict.optionContract}</th>
              </tr>
            </thead>
            <tbody>
              {calendar.map((entry) => (
                <tr key={entry.date}>
                  <td>{entry.date}</td>
                  <td>{entry.symbols.join(", ")}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </>
  );
}
