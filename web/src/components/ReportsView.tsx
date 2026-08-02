import { useEffect, useState } from "react";
import {
  currencySymbol,
  fetchMonthly,
  fetchReports,
  tickerLabel,
  type Market,
  type Monthly,
  type ReportGroup,
  type Reports,
} from "../api";
import type { Dictionary } from "../i18n";
import { DistributionsSection } from "./DistributionsSection";
import { formatValue } from "./KpiCard";
import { MonthlyGrid } from "./MonthlyGrid";

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

// dimensionTabs pairs each Reports.by* group slice with the tab label and
// row-label dimension GroupTable needs — the tab row iterates this instead
// of one <GroupTable> per dimension (mockup's "分組拆解" is one table with a
// tab switcher, not four stacked tables).
function dimensionTabs(dict: Dictionary, reports: Reports): { key: Dimension; label: string; groups: ReportGroup[] }[] {
  return [
    { key: "ticker", label: dict.reportsByTicker, groups: reports.byTicker },
    { key: "plain", label: dict.reportsByHoldingDays, groups: reports.byHoldingDays },
    { key: "month", label: dict.reportsByEntryMonth, groups: reports.byEntryMonth },
    { key: "weekday", label: dict.reportsByEntryWeekday, groups: reports.byEntryWeekday },
  ];
}

function GroupTable({
  dict,
  groups,
  currency,
  dimension,
  names = {},
}: {
  dict: Dictionary;
  groups: ReportGroup[];
  currency: string;
  dimension: Dimension;
  names?: Record<string, string>;
}) {
  if (groups.length === 0) {
    return <div className="empty-message">{dict.noReportData}</div>;
  }
  // maxAbsPnL normalizes the total-P&L magnitude bar per currently-shown
  // group, same convention as the mockup — a $6k loss should look "bigger"
  // than a $1k win within the same table, not against every dimension at once.
  const maxAbsPnL = Math.max(...groups.map((g) => Math.abs(g.totalRealizedPnL)), 1);
  return (
    <table className="mono">
      <thead>
        <tr>
          <th>{dict.group}</th>
          <th>{dict.trades}</th>
          <th>{dict.winRate}</th>
          <th>{dict.profitFactor}</th>
          <th>{dict.avgReturn}</th>
          <th>{dict.avgHold}</th>
          <th>{dict.totalPnL}</th>
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
            <td>
              <span className="winrate-cell">
                <span className="winrate-bar-track">
                  <span className="winrate-bar-fill" style={{ width: `${Math.min(100, g.winRate * 100)}%` }} />
                </span>
                {(g.winRate * 100).toFixed(1)}%
              </span>
            </td>
            <td>{g.profitFactor.toFixed(2)}</td>
            <td className={g.avgReturnPct >= 0 ? "profit" : "loss"}>{fmtPct(g.avgReturnPct)}</td>
            <td>{g.avgHoldingDays.toFixed(1)}</td>
            <td className={g.totalRealizedPnL >= 0 ? "profit" : "loss"}>
              <span className="pnl-bar-cell">
                {fmtSignedMoney(g.totalRealizedPnL, currency)}
                <span className="pnl-bar-track">
                  <span
                    className={`pnl-bar-fill ${g.totalRealizedPnL >= 0 ? "profit" : "loss"}`}
                    style={{ width: `${(Math.abs(g.totalRealizedPnL) / maxAbsPnL) * 100}%` }}
                  />
                </span>
              </span>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
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

// edgeNarrative picks one of four canned verdicts from the win-rate/payoff
// quadrant the account currently sits in — mirrors the mockup's "績效體檢"
// card copy without needing an NLG backend field (both inputs are already
// on Reports).
function edgeNarrative(dict: Dictionary, winRate: number, payoffRatio: number): string {
  const goodWinRate = winRate >= 0.5;
  const goodPayoff = payoffRatio >= 1;
  if (goodWinRate && goodPayoff) return dict.reportsEdgeBothGood;
  if (goodWinRate) return dict.reportsEdgeWinRateOnly;
  if (goodPayoff) return dict.reportsEdgePayoffOnly;
  return dict.reportsEdgeBothWeak;
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
  const [monthly, setMonthly] = useState<Monthly | null>(null);
  const [error, setError] = useState(false);
  const [tab, setTab] = useState<Dimension>("ticker");

  useEffect(() => {
    setReports(null);
    setError(false);
    fetchReports(market)
      .then(setReports)
      .catch(() => setError(true));
    setMonthly(null);
    fetchMonthly(market)
      .then(setMonthly)
      .catch(() => {}); // the monthly grid degrades independently — a failure here shouldn't hide the rest of the page
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
  const { streaks, fees, maeMfe, winRate, profitFactor, expectancy } = reports;
  const payoffRatio = streaks.avgLossPnL !== 0 ? streaks.avgWinPnL / Math.abs(streaks.avgLossPnL) : 0;
  // winPct/lossPct: the edge card's two bars share one scale (the larger of
  // the two magnitudes = 100%) so "avg win vs avg loss" is visually
  // comparable at a glance, same idea as GroupTable's maxAbsPnL norm.
  const barScale = Math.max(streaks.avgWinPnL, Math.abs(streaks.avgLossPnL), 1);
  const winPct = (streaks.avgWinPnL / barScale) * 100;
  const lossPct = (Math.abs(streaks.avgLossPnL) / barScale) * 100;

  const tabs = dimensionTabs(dict, reports);
  const activeTab = tabs.find((t) => t.key === tab) ?? tabs[0];

  return (
    <>
      <div className="detail-grid-2col reports-hero">
        <div className="card">
          <div className="eyebrow">{dict.reportsEdgeTitle}</div>
          <div className="edge-narrative">{edgeNarrative(dict, winRate, payoffRatio)}</div>
          <div className="edge-bars">
            <div className="eyebrow edge-bars-title">{dict.reportsEdgeSubtitle}</div>
            <div className="edge-bar-row">
              <span className="edge-bar-label">{dict.avgWin}</span>
              <span className="edge-bar-track">
                <span className="edge-bar-fill profit" style={{ width: `${winPct}%` }} />
              </span>
              <span className="edge-bar-value profit mono">{fmtSignedMoney(streaks.avgWinPnL, currency)}</span>
            </div>
            <div className="edge-bar-row">
              <span className="edge-bar-label">{dict.avgLoss}</span>
              <span className="edge-bar-track">
                <span className="edge-bar-fill loss" style={{ width: `${lossPct}%` }} />
              </span>
              <span className="edge-bar-value loss mono">{fmtSignedMoney(streaks.avgLossPnL, currency)}</span>
            </div>
          </div>
        </div>
        <div className="card quality-grid">
          <div>
            <div className="eyebrow">{dict.winRate}</div>
            <div className="kpi-value">{formatValue(winRate, "percent", currency)}</div>
          </div>
          <div>
            <div className="eyebrow">{dict.payoffRatio}</div>
            <div className="kpi-value">{payoffRatio.toFixed(2)}×</div>
          </div>
          <div>
            <div className="eyebrow">{dict.profitFactor}</div>
            <div className="kpi-value">{profitFactor.toFixed(2)}</div>
          </div>
          <div>
            <div className="eyebrow">{dict.expectancy}</div>
            <div className={`kpi-value ${expectancy >= 0 ? "profit" : "loss"}`}>
              {formatValue(expectancy, "currency", currency)}
            </div>
          </div>
          <div>
            <div className="eyebrow">{dict.bestTrade}</div>
            <div className="kpi-value profit">{formatValue(streaks.bestTradePnL, "currency", currency)}</div>
          </div>
          <div>
            <div className="eyebrow">{dict.worstTrade}</div>
            <div className="kpi-value loss">{formatValue(streaks.worstTradePnL, "currency", currency)}</div>
          </div>
        </div>
      </div>

      {monthly && <MonthlyGrid dict={dict} years={monthly.years} currency={currency} />}

      <div className="card report-section">
        <div className="eyebrow">{dict.groupBreakdown}</div>
        <div className="group-tabs">
          {tabs.map((t) => (
            <button
              key={t.key}
              className={`group-tab-btn${t.key === activeTab.key ? " active" : ""}`}
              onClick={() => setTab(t.key)}
            >
              {t.label}
            </button>
          ))}
        </div>
        <GroupTable dict={dict} groups={activeTab.groups} currency={currency} dimension={activeTab.key} names={names} />
      </div>

      <div className="stat-grid">
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

      <DistributionsSection dict={dict} market={market} />
    </>
  );
}
