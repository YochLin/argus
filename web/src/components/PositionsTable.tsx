import { tickerLabel, type Position } from "../api";
import type { Dictionary } from "../i18n";
import type { TradeMode } from "./TradeModal";

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
  // onTrade (Phase 10, docs/phase-10-web-trade-input.md §4.3) opens
  // App.tsx's shared TradeModal for one row's ticker — omitted entirely
  // (no [Buy]/[Sell]/[Stop] buttons, no Stop column) when writes are off,
  // same "don't render, don't just disable" rule as every other Phase 10
  // affordance.
  onTrade?: (mode: TradeMode, ticker: string, prefillPrice?: number) => void;
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

export function PositionsTable({
  positions,
  dict,
  currency = "$",
  onTickerClick,
  names = {},
  onTrade,
}: Props) {
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
          {onTrade && <th>{dict.stopPriceCol}</th>}
          {onTrade && <th />}
        </tr>
      </thead>
      <tbody>
        {positions.map((p) => {
          const stopDistPct = p.stopPrice > 0 && p.price !== 0 ? ((p.price - p.stopPrice) / p.price) * 100 : null;
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
              </td>
              <td>{p.shares}</td>
              <td>{fmtMoney(p.avgCost, currency)}</td>
              <td>{fmtMoney(p.price, currency)}</td>
              <td>{fmtMoney(p.marketValue, currency)}</td>
              <td className={p.unrealizedPnL >= 0 ? "profit" : "loss"}>
                {fmtSignedMoney(p.unrealizedPnL, currency)} ({fmtPct(p.unrealizedPnLPct)})
              </td>
              {onTrade && (
                <td>
                  {p.stopPrice > 0 ? (
                    <>
                      {fmtMoney(p.stopPrice, currency)}
                      {stopDistPct !== null && <span className="stat-note"> ({fmtPct(-stopDistPct)})</span>}
                    </>
                  ) : (
                    "—"
                  )}
                </td>
              )}
              {onTrade && (
                <td className="row-actions">
                  <button onClick={() => onTrade("buy", p.ticker, p.price)}>{dict.buy}</button>
                  <button onClick={() => onTrade("sell", p.ticker, p.price)}>{dict.sell}</button>
                  <button onClick={() => onTrade("stop", p.ticker, p.stopPrice)}>{dict.stopPriceCol}</button>
                </td>
              )}
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}
