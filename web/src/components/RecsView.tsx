import { useEffect, useState } from "react";
import { fetchRecPerformance, type Market, type RecPerfExtreme, type RecPerfGroup, type RecPerformance } from "../api";
import type { Dictionary } from "../i18n";

function fmtPct(v: number): string {
  const sign = v > 0 ? "+" : "";
  return `${sign}${v.toFixed(1)}%`;
}

function StatCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="card">
      <div className="eyebrow">{label}</div>
      <div className="kpi-value">{value}</div>
    </div>
  );
}

// StatsMatrix renders one group x horizon breakdown (source or action) —
// each cell shows hit rate and sample size (docs/phase-8-trader-analytics.md
// §5.2: "每格顯示 HitRate (n)"), greyed via the report page's existing
// low-sample convention (n < minSampleSize, reports.go's own threshold)
// rather than a page-specific one.
function StatsMatrix({ dict, title, groups }: { dict: Dictionary; title: string; groups: RecPerfGroup[] }) {
  if (groups.length === 0) {
    return null;
  }
  const horizons = groups[0].cells.map((c) => c.horizon);
  return (
    <div className="card report-section">
      <div className="eyebrow">{title}</div>
      <table className="mono">
        <thead>
          <tr>
            <th>{dict.group}</th>
            {horizons.map((h) => (
              <th key={h}>{h}d</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {groups.map((g) => (
            <tr key={g.key}>
              <td>{g.key}</td>
              {g.cells.map((c) => (
                <td key={c.horizon} className={c.n === 0 ? "" : c.avgExcessPct >= 0 ? "profit" : "loss"}>
                  {c.n === 0 ? (
                    dict.noData
                  ) : (
                    <>
                      {c.hitRatePct.toFixed(0)}% (n={c.n})
                      {c.lowSample && <span className="tag">{dict.lowSampleTag}</span>}
                    </>
                  )}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function ExtremesTable({ dict, title, rows }: { dict: Dictionary; title: string; rows: RecPerfExtreme[] }) {
  if (rows.length === 0) {
    return null;
  }
  return (
    <div className="card report-section">
      <div className="eyebrow">{title}</div>
      <table className="mono">
        <thead>
          <tr>
            <th>{dict.ticker}</th>
            <th>{dict.startDate}</th>
            <th>{dict.side}</th>
            <th>{dict.avgExcessReturn}</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r, i) => (
            <tr key={i}>
              <td>{r.ticker}</td>
              <td>{r.date}</td>
              <td>{r.action}</td>
              <td className={r.excessReturnPct >= 0 ? "profit" : "loss"}>{fmtPct(r.excessReturnPct)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// The recommendation-performance page (Phase 8 PR3, docs/phase-8-trader-
// analytics.md §5.2): scores the recommendations table's full history
// against actual subsequent price action via the same internal/receval
// functions `argus eval` uses server-side, so this page's numbers are
// guaranteed to match a CLI report run against the same database. Distinct
// from /track (bot.go's forward, single-snapshot tracker) per
// docs/offline-rec-eval.md §1 — the two are expected to disagree, since one
// is a live rolling window and this is an offline multi-horizon replay.
export function RecsView({ dict, market }: { dict: Dictionary; market: Market }) {
  const [perf, setPerf] = useState<RecPerformance | null>(null);
  const [error, setError] = useState(false);

  useEffect(() => {
    setPerf(null);
    setError(false);
    fetchRecPerformance(market)
      .then(setPerf)
      .catch(() => setError(true));
  }, [market]);

  if (error) {
    return <div className="error-message">{dict.error}</div>;
  }
  if (!perf) {
    return <div className="loading">{dict.loading}</div>;
  }
  if (perf.counts.scorable === 0) {
    return <div className="empty-message">{dict.noRecData}</div>;
  }

  return (
    <>
      <div className="kpi-grid">
        <StatCard label={dict.recTotal} value={String(perf.counts.total)} />
        <StatCard label={dict.recScorable} value={String(perf.counts.scorable)} />
        <StatCard label={dict.recUnscorable} value={String(perf.counts.unscorable)} />
        <StatCard label={dict.recHold} value={String(perf.counts.hold)} />
      </div>
      <StatsMatrix dict={dict} title={dict.recByAction} groups={perf.byAction} />
      <StatsMatrix dict={dict} title={dict.recBySource} groups={perf.bySource} />
      <ExtremesTable dict={dict} title={dict.recBest} rows={perf.best} />
      <ExtremesTable dict={dict} title={dict.recWorst} rows={perf.worst} />
    </>
  );
}
