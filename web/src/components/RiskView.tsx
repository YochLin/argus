import { useEffect, useState } from "react";
import { currencySymbol, fetchRisk, tickerLabel, type Market, type Risk } from "../api";
import type { Dictionary } from "../i18n";
import { CashCard } from "./CashCard";

interface Props {
  dict: Dictionary;
  market: Market;
  names?: Record<string, string>;
  onTickerClick?: (ticker: string) => void;
  writable?: boolean;
  onUnauthorized?: (retry: () => void) => void;
}

function fmtMoney(v: number, currency: string): string {
  return `${currency}${v.toLocaleString(undefined, { maximumFractionDigits: 0 })}`;
}

function fmtPct(v: number): string {
  const sign = v > 0 ? "+" : "";
  return `${sign}${v.toFixed(1)}%`;
}

// The /risk page (Phase 8 PR1, docs/phase-8-trader-analytics.md §3.3):
// current open-risk exposure across the held portfolio, priced live rather
// than from the daily snapshot replay every other view here reads — this is
// the one page in the dashboard meant to change behavior (see the design
// doc's "誠實價值評估"), not just make existing history more visible.
export function RiskView({ dict, market, names = {}, onTickerClick, writable = false, onUnauthorized = () => {} }: Props) {
  const [risk, setRisk] = useState<Risk | null>(null);
  const [error, setError] = useState(false);
  // Bumped after the cash card saves: account value, heat and weights all
  // move with the cash balance, so the whole response is re-fetched.
  const [reload, setReload] = useState(0);

  useEffect(() => {
    setError(false);
    fetchRisk(market)
      .then(setRisk)
      .catch(() => setError(true));
  }, [market, reload]);

  // A market switch must not show the other market's numbers while the new
  // ones load; a reload after saving keeps the old ones on screen instead.
  useEffect(() => setRisk(null), [market]);

  if (error) {
    return <div className="error-message">{dict.error}</div>;
  }
  if (!risk) {
    return <div className="loading">{dict.loading}</div>;
  }

  const currency = currencySymbol(market);
  const heatBreached = risk.heatThresholdPct > 0 && risk.heatPct > risk.heatThresholdPct;
  // Concentration view per the design doc: no separate pie chart, the
  // weight-sorted table row order already makes the biggest position
  // obvious at a glance.
  const positions = [...risk.positions].sort((a, b) => b.weightPct - a.weightPct);

  return (
    <>
      <div className="kpi-grid">
        <div className="card">
          <div className="eyebrow">{dict.portfolioHeat}</div>
          <div className={`kpi-value ${heatBreached ? "loss" : ""}`}>{risk.heatPct.toFixed(1)}%</div>
          {risk.heatThresholdPct > 0 && (
            <div className="cash-sub">{dict.heatThresholdLine.replace("%s", String(risk.heatThresholdPct))}</div>
          )}
          {!risk.hasCash && <span className="cash-warn-chip">{dict.heatNoCash}</span>}
        </div>
        <div className="card">
          <div className="eyebrow">{dict.accountValue}</div>
          <div className="kpi-value">{fmtMoney(risk.accountValue, currency)}</div>
          {!risk.hasCash && <div className="cash-sub">{dict.accountValueNoCash}</div>}
        </div>
        <CashCard
          dict={dict}
          market={market}
          risk={risk}
          writable={writable}
          onSaved={() => setReload((n) => n + 1)}
          onUnauthorized={onUnauthorized}
        />
      </div>

      <div className="card">
        {positions.length === 0 ? (
          <div className="empty-message">{dict.noPositions}</div>
        ) : (
          <table className="mono">
            <thead>
              <tr>
                <th>{dict.ticker}</th>
                <th>{dict.marketValue}</th>
                <th>{dict.weight}</th>
                <th>{dict.stopPriceCol}</th>
                <th>{dict.openRisk}</th>
                <th>{dict.unrealizedPnL}</th>
              </tr>
            </thead>
            <tbody>
              {positions.map((p) => {
                const noStop = p.stopPrice <= 0;
                const belowStop = !noStop && p.price < p.stopPrice;
                return (
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
                      {noStop && <span className="tag warn">{dict.noStopSet}</span>}
                      {belowStop && <span className="tag warn">{dict.belowStop}</span>}
                    </td>
                    <td>{fmtMoney(p.value, currency)}</td>
                    <td>{p.weightPct.toFixed(1)}%</td>
                    <td>{p.stopPrice > 0 ? fmtMoney(p.stopPrice, currency) : "—"}</td>
                    <td>
                      {p.openRisk === null || p.openRiskPct === null
                        ? "—"
                        : `${fmtMoney(p.openRisk, currency)} (${p.openRiskPct.toFixed(1)}%)`}
                    </td>
                    <td className={p.unrealizedPnLPct >= 0 ? "profit" : "loss"}>{fmtPct(p.unrealizedPnLPct)}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>
    </>
  );
}
