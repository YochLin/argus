import { tickerLabel, type Position } from "../api";
import type { Dictionary } from "../i18n";

interface Props {
  positions: Position[];
  dict: Dictionary;
  // currency is Phase 6's per-market symbol ("$"/"NT$", see api.ts's
  // currencySymbol) — defaults to "$" so every pre-Phase-6 caller is
  // unaffected.
  currency?: string;
  // onTickerClick is Phase 7's drill-down into /chart (docs/phase-7-
  // support-resistance.md §5.1) — optional so every pre-existing call site
  // that doesn't pass it keeps rendering a plain, unclickable ticker cell.
  onTickerClick?: (ticker: string) => void;
  // names is /api/company-names' TW ticker → Chinese-name map (see App.tsx)
  // — optional with an empty default so a caller without it renders bare
  // tickers, same degrade as the backend without FINMIND_TOKEN.
  names?: Record<string, string>;
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

export function PositionsTable({ positions, dict, currency = "$", onTickerClick, names = {} }: Props) {
  if (positions.length === 0) {
    return <div className="empty-message">{dict.noPositions}</div>;
  }
  return (
    <table className="mono">
      <thead>
        <tr>
          <th>{dict.ticker}</th>
          <th>{dict.shares}</th>
          <th>{dict.avgCost}</th>
          <th>{dict.price}</th>
          <th>{dict.marketValue}</th>
          <th>{dict.unrealizedPnL}</th>
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
            <td>{p.shares}</td>
            <td>{fmtMoney(p.avgCost, currency)}</td>
            <td>{fmtMoney(p.price, currency)}</td>
            <td>{fmtMoney(p.marketValue, currency)}</td>
            <td className={p.unrealizedPnL >= 0 ? "profit" : "loss"}>
              {fmtSignedMoney(p.unrealizedPnL, currency)} ({fmtPct(p.unrealizedPnLPct)})
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
