import type { Transaction } from "../api";
import type { Dictionary } from "../i18n";

interface Props {
  dict: Dictionary;
  transactions: Transaction[];
  emptyMessage?: string;
  // currency is Phase 6's per-market symbol ("$"/"NT$", see api.ts's
  // currencySymbol) — defaults to "$" so every pre-Phase-6 caller is
  // unaffected.
  currency?: string;
}

function fmtSigned(v: number, currency: string): string {
  const sign = v > 0 ? "+" : v < 0 ? "-" : "";
  return `${sign}${currency}${Math.abs(v).toLocaleString(undefined, { maximumFractionDigits: 0 })}`;
}

// Shared by CalendarView's click-a-day panel and RoundDetailView's leg list
// (Phase 5 PR2/PR3) — same six columns, same BUY/SELL/realizedPnL
// formatting, so this was pulled out rather than kept as two near-identical
// copies once a second caller showed up.
export function TradesTable({ dict, transactions, emptyMessage, currency = "$" }: Props) {
  if (transactions.length === 0) {
    return <div className="empty-message">{emptyMessage ?? dict.noTransactions}</div>;
  }
  return (
    <table className="mono">
      <thead>
        <tr>
          <th>{dict.ticker}</th>
          <th>{dict.side}</th>
          <th>{dict.shares}</th>
          <th>{dict.price}</th>
          <th>{dict.fee}</th>
          <th>{dict.realizedPnL}</th>
        </tr>
      </thead>
      <tbody>
        {transactions.map((t, i) => (
          <tr key={i}>
            <td>{t.ticker}</td>
            <td className={t.side === "BUY" ? "profit" : "loss"}>{t.side === "BUY" ? dict.buy : dict.sell}</td>
            <td>{t.shares}</td>
            <td>
              {currency}
              {t.price.toFixed(2)}
            </td>
            <td>
              {currency}
              {t.fee.toFixed(2)}
            </td>
            <td className={t.realizedPnL > 0 ? "profit" : t.realizedPnL < 0 ? "loss" : ""}>
              {t.side === "SELL" ? fmtSigned(t.realizedPnL, currency) : "—"}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
