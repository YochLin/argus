import type { YearlyPnL } from "../api";
import type { Dictionary } from "../i18n";

function fmtSignedMoney(v: number, currency: string): string {
  const sign = v > 0 ? "+" : v < 0 ? "-" : "";
  return `${sign}${currency}${Math.abs(v).toLocaleString(undefined, { maximumFractionDigits: 0 })}`;
}

interface Props {
  dict: Dictionary;
  years: YearlyPnL[];
  currency: string;
}

// The Reports page's monthly P&L grid (Phase 8 PR3, docs/phase-8-trader-
// analytics.md §5.1) — a fund-factsheet-style year x month table, amount-
// based (not %, deliberately — see internal/web/monthly.go's doc comment)
// and sourced from the same DailyPnL replay engine the calendar heatmap
// already uses, so this grid's totals always match summing the calendar by
// hand. A month with no data (null, not 0) renders as an empty cell.
export function MonthlyGrid({ dict, years, currency }: Props) {
  if (years.length === 0) {
    return null;
  }
  return (
    <div className="card report-section">
      <div className="eyebrow">{dict.monthlyPnl}</div>
      <table className="mono">
        <thead>
          <tr>
            <th></th>
            {dict.months.map((m) => (
              <th key={m}>{m}</th>
            ))}
            <th>{dict.yearTotal}</th>
          </tr>
        </thead>
        <tbody>
          {years.map((y) => (
            <tr key={y.year}>
              <td>{y.year}</td>
              {y.months.map((v, i) => (
                <td key={i} className={v === null ? "" : v >= 0 ? "profit" : "loss"}>
                  {v === null ? "" : fmtSignedMoney(v, currency)}
                </td>
              ))}
              <td className={y.total >= 0 ? "profit" : "loss"}>{fmtSignedMoney(y.total, currency)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
