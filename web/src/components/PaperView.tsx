import { useEffect, useState } from "react";
import { currencySymbol, fetchPaper, tickerLabel, type Market, type Paper } from "../api";
import type { Dictionary } from "../i18n";
import { KpiCard } from "./KpiCard";
import { PnlChart } from "./PnlChart";

interface Props {
  dict: Dictionary;
  market: Market;
  names?: Record<string, string>;
  onTickerClick?: (ticker: string) => void;
}

function fmtMoney(v: number, currency: string): string {
  return `${currency}${v.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
}

function fmtSignedMoney(v: number, currency: string): string {
  const sign = v > 0 ? "+" : v < 0 ? "-" : "";
  return `${sign}${fmtMoney(Math.abs(v), currency)}`;
}

function fmtPct(v: number): string {
  const sign = v > 0 ? "+" : "";
  return `${sign}${v.toFixed(2)}%`;
}

// The Paper Account page (Phase 11 PR4, docs/phase-11-paper-account.md §7.2):
// a read-only view of the live paper account — no trade buttons, no account
// switcher (one market at a time via the shared toggle, same as every other
// market-scoped page here), and no CLI-backtest reach into this page (that
// stays terminal-only, same as `argus eval`).
export function PaperView({ dict, market, names = {}, onTickerClick }: Props) {
  const [paper, setPaper] = useState<Paper | null>(null);
  const [error, setError] = useState(false);

  useEffect(() => {
    setPaper(null);
    setError(false);
    fetchPaper(market)
      .then(setPaper)
      .catch(() => setError(true));
  }, [market]);

  if (error) {
    return <div className="error-message">{dict.error}</div>;
  }
  if (!paper) {
    return <div className="loading">{dict.loading}</div>;
  }

  const { kpis, curve, benchmark, positions, closed } = paper;
  const currency = currencySymbol(market);

  return (
    <>
      <div className="readonly-notice">
        <span className="readonly-notice-badge">{dict.paperReadOnlyBadge}</span>
        {dict.paperReadOnlyNotice}
      </div>

      <div className="stat-grid">
        <KpiCard label={dict.paperEquity} value={kpis.equity} format="currency" currency={currency} />
        <KpiCard label={dict.paperTotalReturn} value={kpis.totalReturnPct} format="percentValue" colorMode="pnl" />
        <KpiCard label={dict.paperBenchmarkReturn} value={kpis.benchmarkReturnPct} format="percentValue" />
        <KpiCard label={dict.paperAlpha} value={kpis.alphaPct} format="percentValue" colorMode="pnl" />
        <KpiCard label={dict.maxDrawdown} value={-Math.abs(kpis.maxDrawdownPct)} format="percentValue" colorMode="loss" />
        <KpiCard label={dict.winRate} value={kpis.winRate} format="percent" />
        <KpiCard label={dict.profitFactor} value={kpis.profitFactor} format="ratio" />
        <KpiCard label={dict.expectancy} value={kpis.expectancy} format="currency" colorMode="pnl" currency={currency} />
      </div>

      <PnlChart curve={curve} benchmark={benchmark} dict={dict} title={dict.paperEquity} />

      <div className="card">
        <div className="eyebrow">{dict.positions}</div>
        {positions.length === 0 ? (
          <div className="empty-message">{dict.noPositions}</div>
        ) : (
          <table className="mono">
            <thead>
              <tr>
                <th>{dict.ticker}</th>
                <th>{dict.entryDate}</th>
                <th>{dict.shares}</th>
                <th>{dict.avgCost}</th>
                <th>{dict.price}</th>
                <th>{dict.unrealizedPnL}</th>
                <th>{dict.distToStop}</th>
              </tr>
            </thead>
            <tbody>
              {positions.map((p) => (
                <tr key={p.ticker}>
                  <td>
                    {onTickerClick ? (
                      <a
                        href={`/chart?ticker=${encodeURIComponent(p.ticker)}`}
                        onClick={(e) => {
                          e.preventDefault();
                          onTickerClick(p.ticker);
                        }}
                      >
                        {tickerLabel(p.ticker, names)}
                      </a>
                    ) : (
                      tickerLabel(p.ticker, names)
                    )}
                  </td>
                  <td>{p.entryDate}</td>
                  <td>{p.shares}</td>
                  <td>{fmtMoney(p.avgCost, currency)}</td>
                  <td>{fmtMoney(p.price, currency)}</td>
                  <td className={p.unrealizedPnL >= 0 ? "profit" : "loss"}>
                    {fmtSignedMoney(p.unrealizedPnL, currency)} ({fmtPct(p.unrealizedPct)})
                  </td>
                  <td>{p.stopPrice > 0 ? fmtPct(-p.distToStopPct) : "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <div className="card">
        <div className="eyebrow">{dict.closedPositions}</div>
        {closed.length === 0 ? (
          <div className="empty-message">{dict.noClosedPositions}</div>
        ) : (
          <table className="mono">
            <thead>
              <tr>
                <th>{dict.ticker}</th>
                <th>{dict.entryDate}</th>
                <th>{dict.exitDate}</th>
                <th>{dict.avgCost}</th>
                <th>{dict.exitPrice}</th>
                <th>{dict.realizedPnL}</th>
                <th>{dict.exitReason}</th>
              </tr>
            </thead>
            <tbody>
              {closed.map((c, i) => (
                <tr key={`${c.ticker}-${c.entryDate}-${i}`}>
                  <td>{tickerLabel(c.ticker, names)}</td>
                  <td>{c.entryDate}</td>
                  <td>{c.exitDate}</td>
                  <td>{fmtMoney(c.avgCost, currency)}</td>
                  <td>{fmtMoney(c.exitPrice, currency)}</td>
                  <td className={c.realizedPnL >= 0 ? "profit" : "loss"}>
                    {fmtSignedMoney(c.realizedPnL, currency)} ({fmtPct(c.realizedPct)})
                  </td>
                  <td>
                    <span className={`tag${c.exitReason === "stop" ? " warn" : ""}`}>
                      {c.exitReason === "stop" ? dict.exitReasonStop : dict.exitReasonLlmSell}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </>
  );
}
