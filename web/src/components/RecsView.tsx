import { useEffect, useState } from "react";
import {
  currencySymbol,
  fetchRecPerformance,
  type Market,
  type RecPerfActiveSignal,
  type RecPerfExtreme,
  type RecPerfGroup,
  type RecPerformance,
} from "../api";
import type { Dictionary } from "../i18n";

function fmtPct(v: number): string {
  const sign = v > 0 ? "+" : "";
  return `${sign}${v.toFixed(1)}%`;
}

// sourceLabel maps the four real receval.DisplaySource values (there is no
// "news" source in the real system despite the mockup showing one) to a
// localized display string.
function sourceLabel(dict: Dictionary, source: string): string {
  switch (source) {
    case "watchlist":
      return dict.recSourceWatchlist;
    case "movers":
      return dict.recSourceMovers;
    case "scan":
      return dict.recSourceScan;
    case "explore":
      return dict.recSourceExplore;
    default:
      return source;
  }
}

const PROFIT_RGB = "16, 185, 129";
const LOSS_RGB = "239, 68, 68";

// heatColor scales background alpha by |pct| relative to the largest
// magnitude in the current set — same intensity-by-magnitude idea as the
// mockup's "依來源" matrix and horizon chart, computed client-side since
// avgExcessPct is already on every cell.
function heatColor(pct: number, maxAbs: number): string {
  const intensity = maxAbs > 0 ? Math.min(Math.abs(pct) / maxAbs, 1) : 0;
  const alpha = 0.12 + intensity * 0.62;
  const rgb = pct >= 0 ? PROFIT_RGB : LOSS_RGB;
  return `rgba(${rgb}, ${alpha.toFixed(2)})`;
}

function StatCard({
  label,
  value,
  note,
}: {
  label: string;
  value: string;
  note?: string;
}) {
  return (
    <div className="card">
      <div className="eyebrow">{label}</div>
      <div className="kpi-value">{value}</div>
      {note && <div className="stat-note">{note}</div>}
    </div>
  );
}

// StatsMatrix renders one group x horizon breakdown (source or action),
// heatmap-shaded by avgExcessPct magnitude — mirrors the mockup's "依來源"
// card. keyLabel lets the source-grouped table localize its row labels
// (watchlist/movers/scan/explore) while the action-grouped table keeps
// BUY/SELL as-is.
function StatsMatrix({
  dict,
  title,
  groups,
  keyLabel = (k) => k,
}: {
  dict: Dictionary;
  title: string;
  groups: RecPerfGroup[];
  keyLabel?: (key: string) => string;
}) {
  if (groups.length === 0) {
    return null;
  }
  const horizons = groups[0].cells.map((c) => c.horizon);
  const maxAbs = Math.max(1, ...groups.flatMap((g) => g.cells.map((c) => Math.abs(c.avgExcessPct))));
  return (
    <div className="card report-section">
      <div className="eyebrow">{title}</div>
      <table className="mono">
        <thead>
          <tr>
            <th>{dict.horizonDays}</th>
            {horizons.map((h) => (
              <th key={h}>{h}D</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {groups.map((g) => (
            <tr key={g.key}>
              <td>{keyLabel(g.key)}</td>
              {g.cells.map((c) => (
                <td key={c.horizon} style={{ padding: 0 }}>
                  {c.n === 0 ? (
                    <div className="heat-cell">{dict.noData}</div>
                  ) : (
                    <div className="heat-cell" style={{ background: heatColor(c.avgExcessPct, maxAbs) }}>
                      <div className={`heat-cell-value ${c.avgExcessPct >= 0 ? "profit" : "loss"}`}>
                        {fmtPct(c.avgExcessPct)}
                      </div>
                      <div className="heat-cell-n">
                        n={c.n}
                        {c.lowSample && <span className="tag">{dict.lowSampleTag}</span>}
                      </div>
                    </div>
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
            <th>{dict.recSource}</th>
            <th>{dict.side}</th>
            <th>{dict.avgExcessReturn}</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r, i) => (
            <tr key={i}>
              <td>{r.ticker}</td>
              <td>{r.date}</td>
              <td className="rec-source-cell">{sourceLabel(dict, r.source)}</td>
              <td>{r.action}</td>
              <td className={r.excessReturnPct >= 0 ? "profit" : "loss"}>{fmtPct(r.excessReturnPct)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// ActiveSignalsTable renders still-open (unmatured shortest-horizon window)
// BUY/SELL recs — mirrors the mockup's "目前有效訊號" table, minus the "持倉中"
// badge (no backend plumbing ties a rec to a still-open position) and with
// entryPrice standing in for the mockup's live "現價" (no current-quote fetch
// wired into this page).
function ActiveSignalsTable({
  dict,
  title,
  rows,
  currency,
}: {
  dict: Dictionary;
  title: string;
  rows: RecPerfActiveSignal[];
  currency: string;
}) {
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
            <th>{dict.side}</th>
            <th>{dict.recSource}</th>
            <th>{dict.recIssuedTime}</th>
            <th>{dict.recEntryPrice}</th>
            <th>{dict.recSinceSignal}</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r, i) => (
            <tr key={i}>
              <td>{r.ticker}</td>
              <td>{r.action}</td>
              <td className="rec-source-cell">{sourceLabel(dict, r.source)}</td>
              <td>
                {r.daysHeld} {dict.recDaysAgo}
              </td>
              <td>
                {currency}
                {r.entryPrice.toFixed(2)}
              </td>
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

  const bestCell = perf.overall.find((c) => c.horizon === perf.bestHorizon);
  const bestExcess = bestCell?.avgExcessPct ?? 0;
  const bestHitRate = bestCell?.hitRatePct ?? 0;

  const acted = perf.actedVsSkipped.find((g) => g.key === "acted");
  const skipped = perf.actedVsSkipped.find((g) => g.key === "skipped");
  const actedCell = acted?.cells.find((c) => c.horizon === perf.bestHorizon);
  const skippedCell = skipped?.cells.find((c) => c.horizon === perf.bestHorizon);
  const followedPct = actedCell?.avgExcessPct ?? 0;
  const followedN = actedCell?.n ?? 0;
  const skippedPct = skippedCell?.avgExcessPct ?? 0;
  const skippedN = skippedCell?.n ?? 0;
  // followScale/followPct/skipPct: the comparison card's two bars share one
  // scale (the larger of the two magnitudes = 100%), same idea as Reports'
  // avg-win/avg-loss bars.
  const followScale = Math.max(Math.abs(followedPct), Math.abs(skippedPct), 1);
  const followBarPct = (Math.abs(followedPct) / followScale) * 100;
  const skipBarPct = (Math.abs(skippedPct) / followScale) * 100;

  const diff = followedPct - skippedPct;
  const narrative =
    `${dict.recNarrativePeakPrefix}${perf.bestHorizon}${dict.recNarrativePeakMid}${fmtPct(bestExcess)}${dict.recNarrativePeakSuffix}` +
    (diff >= 0
      ? `${dict.recNarrativeFollowBetterPrefix}${fmtPct(Math.abs(diff))}${dict.recNarrativeFollowBetterSuffix}`
      : `${dict.recNarrativeSkipBetterPrefix}${fmtPct(Math.abs(diff))}${dict.recNarrativeSkipBetterSuffix}`);

  const horizonMaxAbs = Math.max(1, ...perf.overall.map((c) => Math.abs(c.avgExcessPct)));

  return (
    <>
      <div className="detail-grid-2col reports-hero">
        <div className="card">
          <div className="eyebrow">{dict.recSignalCheckup}</div>
          <div className="edge-narrative">{narrative}</div>
          <div className="edge-bars">
            <div className="eyebrow edge-bars-title">{dict.recFollowedVsSkipped}</div>
            <div className="edge-bar-row">
              <span className="edge-bar-label" style={{ flexBasis: 130 }}>
                {dict.recFollowed} · {followedN}
              </span>
              <span className="edge-bar-track">
                <span className="edge-bar-fill followed" style={{ width: `${followBarPct}%` }} />
              </span>
              <span className={`edge-bar-value mono ${followedPct >= 0 ? "profit" : "loss"}`}>{fmtPct(followedPct)}</span>
            </div>
            <div className="edge-bar-row">
              <span className="edge-bar-label" style={{ flexBasis: 130 }}>
                {dict.recSkipped} · {skippedN}
              </span>
              <span className="edge-bar-track">
                <span className="edge-bar-fill skipped" style={{ width: `${skipBarPct}%` }} />
              </span>
              <span className={`edge-bar-value mono ${skippedPct >= 0 ? "profit" : "loss"}`}>{fmtPct(skippedPct)}</span>
            </div>
          </div>
        </div>
        <div className="card">
          <div className="eyebrow">{dict.recExcessByHorizon}</div>
          <div className="horizon-chart">
            {perf.overall.map((c) => {
              const heightPct = Math.max((Math.abs(c.avgExcessPct) / horizonMaxAbs) * 100, 4);
              return (
                <div className="horizon-bar-col" key={c.horizon}>
                  <div className={`horizon-bar-value ${c.avgExcessPct >= 0 ? "profit" : "loss"}`}>{fmtPct(c.avgExcessPct)}</div>
                  <div
                    className="horizon-bar"
                    style={{ height: `${heightPct}%`, background: heatColor(c.avgExcessPct, horizonMaxAbs) }}
                  />
                  <div className="horizon-bar-label">{c.horizon}d</div>
                  <div className="horizon-bar-n">n={c.n}</div>
                </div>
              );
            })}
          </div>
        </div>
      </div>

      <div className="stat-grid">
        <StatCard label={dict.recTotal} value={String(perf.counts.total)} note={`${dict.recScorable} ${perf.counts.scorable}`} />
        <StatCard
          label={dict.recSignalHitRate}
          value={`${bestHitRate.toFixed(1)}%`}
          note={`${dict.recRandomBaseline} 50%`}
        />
        <StatCard
          label={dict.recBestHoldingWindow}
          value={`${perf.bestHorizon}d`}
          note={`${fmtPct(bestExcess)} ${dict.recExcessReturnNote}`}
        />
        <StatCard label={dict.recUnscorable} value={String(perf.counts.unscorable)} note={dict.recInsufficientData} />
      </div>

      <StatsMatrix dict={dict} title={dict.recBySource} groups={perf.bySource} keyLabel={(k) => sourceLabel(dict, k)} />
      <StatsMatrix dict={dict} title={dict.recByAction} groups={perf.byAction} />

      <ActiveSignalsTable
        dict={dict}
        title={dict.recActiveSignals}
        rows={perf.activeSignals}
        currency={currencySymbol(market)}
      />

      <div className="detail-grid-2col">
        <ExtremesTable dict={dict} title={dict.recBest} rows={perf.best} />
        <ExtremesTable dict={dict} title={dict.recWorst} rows={perf.worst} />
      </div>
    </>
  );
}
