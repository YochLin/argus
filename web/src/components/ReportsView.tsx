import { useEffect, useState } from "react";
import { currencySymbol, fetchReports, tickerLabel, type Market, type ReportGroup, type Reports } from "../api";
import type { Dictionary } from "../i18n";
import { KpiCard } from "./KpiCard";

interface Props {
  dict: Dictionary;
  market: Market;
  // names is /api/company-names' TW ticker → Chinese-name map (see App.tsx),
  // applied only to the "By Ticker" group table's row labels.
  names?: Record<string, string>;
}

function fmtPct(v: number): string {
  const sign = v > 0 ? "+" : "";
  return `${sign}${v.toFixed(2)}%`;
}

function fmtSignedMoney(v: number, currency: string): string {
  const sign = v > 0 ? "+" : v < 0 ? "-" : "";
  return `${sign}${currency}${Math.abs(v).toLocaleString(undefined, { maximumFractionDigits: 0 })}`;
}

// weekdayIndex maps Go's time.Weekday.String() (what ByEntryWeekday's keys
// are, see internal/web/reports.go's weekdayOrder) onto dict.weekdays'
// Sunday-first index (i18n.ts's existing convention, shared with
// CalendarView).
const weekdayIndex: Record<string, number> = {
  Sunday: 0,
  Monday: 1,
  Tuesday: 2,
  Wednesday: 3,
  Thursday: 4,
  Friday: 5,
  Saturday: 6,
};

type Dimension = "plain" | "ticker" | "month" | "weekday";

function groupLabel(dict: Dictionary, dimension: Dimension, key: string, names: Record<string, string>): string {
  if (dimension === "ticker") {
    return tickerLabel(key, names);
  }
  if (dimension === "month") {
    const idx = parseInt(key, 10) - 1;
    return dict.months[idx] ?? key;
  }
  if (dimension === "weekday") {
    const idx = weekdayIndex[key];
    return idx !== undefined ? dict.weekdays[idx] : key;
  }
  return key;
}

function GroupTable({
  dict,
  title,
  groups,
  currency,
  dimension,
  names = {},
}: {
  dict: Dictionary;
  title: string;
  groups: ReportGroup[];
  currency: string;
  dimension: Dimension;
  names?: Record<string, string>;
}) {
  if (groups.length === 0) {
    return null;
  }
  return (
    <div className="card report-section">
      <div className="eyebrow">{title}</div>
      <table className="mono">
        <thead>
          <tr>
            <th>{dict.group}</th>
            <th>{dict.trades}</th>
            <th>{dict.winRate}</th>
            <th>{dict.profitFactor}</th>
            <th>{dict.avgReturn}</th>
            <th>{dict.totalPnL}</th>
            <th>{dict.avgHold}</th>
          </tr>
        </thead>
        <tbody>
          {groups.map((g) => (
            <tr key={g.key} className={g.lowSample ? "low-sample-row" : ""}>
              <td>
                {groupLabel(dict, dimension, g.key, names)}
                {g.lowSample && <span className="tag">{dict.lowSampleTag}</span>}
              </td>
              <td>{g.n}</td>
              <td>{(g.winRate * 100).toFixed(1)}%</td>
              <td>{g.profitFactor.toFixed(2)}</td>
              <td className={g.avgReturnPct >= 0 ? "profit" : "loss"}>{fmtPct(g.avgReturnPct)}</td>
              <td className={g.totalRealizedPnL >= 0 ? "profit" : "loss"}>
                {fmtSignedMoney(g.totalRealizedPnL, currency)}
              </td>
              <td>{g.avgHoldingDays.toFixed(1)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// The report page's stat-tile row: best/worst trade and avg win/loss reuse
// KpiCard (same currency/pnl-color convention as the dashboard's own KPIs);
// streak counts and the fee/MFE-captured figures don't fit KpiCard's
// currency/percent/ratio formats cleanly, so they're small local
// card/eyebrow/kpi-value blocks instead of forcing a fourth KpiCard format
// variant for two call sites.
function StatCard({
  label,
  value,
  note,
  colorClass = "",
}: {
  label: string;
  value: string;
  note?: string;
  colorClass?: string;
}) {
  return (
    <div className="card">
      <div className="eyebrow">{label}</div>
      <div className={`kpi-value ${colorClass}`}>{value}</div>
      {note && <div className="stat-note">{note}</div>}
    </div>
  );
}

// Phase 5 PR4 (docs/phase-5-web-dashboard.md §A1/A2/A5): grouped performance
// report (ticker / holding-period / entry-month / entry-weekday), a fee
// rollup, cheap KPI completions (best/worst trade, avg win/loss, streaks),
// and the MAE/MFE "average captured %" aggregate. Every group table is
// independently omitted when empty (GroupTable returns null) rather than
// shown with a "no data" placeholder — with few trades, most dimensions
// legitimately have nothing yet.
export function ReportsView({ dict, market, names = {} }: Props) {
  const [reports, setReports] = useState<Reports | null>(null);
  const [error, setError] = useState(false);

  useEffect(() => {
    setReports(null);
    setError(false);
    fetchReports(market)
      .then(setReports)
      .catch(() => setError(true));
  }, [market]);

  if (error) {
    return <div className="error-message">{dict.error}</div>;
  }
  if (!reports) {
    return <div className="loading">{dict.loading}</div>;
  }
  if (reports.byTicker.length === 0) {
    return <div className="empty-message">{dict.noReportData}</div>;
  }

  const currency = currencySymbol(market);
  const { streaks, fees, maeMfe } = reports;

  return (
    <>
      <div className="kpi-grid">
        <KpiCard label={dict.bestTrade} value={streaks.bestTradePnL} format="currency" colorMode="pnl" currency={currency} />
        <KpiCard label={dict.worstTrade} value={streaks.worstTradePnL} format="currency" colorMode="pnl" currency={currency} />
        <KpiCard label={dict.avgWin} value={streaks.avgWinPnL} format="currency" colorMode="pnl" currency={currency} />
        <KpiCard label={dict.avgLoss} value={streaks.avgLossPnL} format="currency" colorMode="pnl" currency={currency} />
      </div>
      <div className="kpi-grid">
        <StatCard label={dict.longestWinStreak} value={String(streaks.longestWinStreak)} />
        <StatCard label={dict.longestLossStreak} value={String(streaks.longestLossStreak)} />
        <StatCard
          label={dict.totalFees}
          value={`${currency}${fees.totalFees.toLocaleString(undefined, { maximumFractionDigits: 0 })}`}
          note={fees.pctOfRealizedPnL !== 0 ? `${fees.pctOfRealizedPnL.toFixed(1)}% ${dict.feePctOfPnL}` : undefined}
        />
        {maeMfe.n > 0 && (
          <StatCard
            label={`${dict.mfeCaptured}${maeMfe.lowSample ? ` (${dict.lowSampleTag})` : ""}`}
            value={`${maeMfe.avgCapturedPct.toFixed(1)}%`}
            note={`${dict.mfeCapturedNote} (n=${maeMfe.n})`}
            colorClass={maeMfe.avgCapturedPct >= 0 ? "profit" : "loss"}
          />
        )}
      </div>

      <GroupTable
        dict={dict}
        title={dict.reportsByTicker}
        groups={reports.byTicker}
        currency={currency}
        dimension="ticker"
        names={names}
      />
      <GroupTable
        dict={dict}
        title={dict.reportsByHoldingDays}
        groups={reports.byHoldingDays}
        currency={currency}
        dimension="plain"
      />
      <GroupTable
        dict={dict}
        title={dict.reportsByEntryMonth}
        groups={reports.byEntryMonth}
        currency={currency}
        dimension="month"
      />
      <GroupTable
        dict={dict}
        title={dict.reportsByEntryWeekday}
        groups={reports.byEntryWeekday}
        currency={currency}
        dimension="weekday"
      />
    </>
  );
}
