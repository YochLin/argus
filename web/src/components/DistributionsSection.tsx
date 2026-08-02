import { useEffect, useState } from "react";
import { currencySymbol, fetchDistributions, type Distributions, type Market } from "../api";
import type { Dictionary } from "../i18n";
import { Histogram } from "./Histogram";
import { ScatterChart } from "./ScatterChart";

interface Props {
  dict: Dictionary;
  market: Market;
}

// The Reports page's three distribution views (Phase 8 PR4, docs/phase-8-
// trader-analytics.md §6.1) — R-multiple histogram, holding-days-vs-P&L
// scatter, and MAE-vs-return scatter, all reduced from the same closed-round
// unit. Self-fetching (own loading/error state) like every other top-level
// section component in this app, so a /api/distributions failure doesn't
// take down the rest of the already-loaded Reports page.
export function DistributionsSection({ dict, market }: Props) {
  const [dist, setDist] = useState<Distributions | null>(null);

  useEffect(() => {
    setDist(null);
    fetchDistributions(market)
      .then(setDist)
      .catch(() => {}); // degrades independently — the rest of Reports still renders
  }, [market]);

  if (!dist) {
    return null;
  }
  const currency = currencySymbol(market);
  const hasAny = dist.rMultiples.length > 0 || dist.holdingReturns.length > 0 || dist.maeReturns.length > 0;
  if (!hasAny) {
    return null;
  }

  return (
    <div className="distributions-grid">
      {(dist.rMultiples.length > 0 || dist.noStopCount > 0) && (
        <div className="card report-section">
          <div className="eyebrow">{dict.rMultipleHistogram}</div>
          {dist.rMultiples.length > 0 ? (
            <Histogram values={dist.rMultiples.map((s) => s.r)} />
          ) : (
            <div className="empty-message">{dict.noDistributionData}</div>
          )}
          <div className="stat-note">
            {dist.earliestRDate && `${dict.rMultipleNote} ${dist.earliestRDate}`}
            {dist.noStopCount > 0 && ` — ${dist.noStopCount} ${dict.noStopSamples}`}
          </div>
        </div>
      )}

      {dist.holdingReturns.length > 0 && (
        <div className="card report-section">
          <div className="eyebrow">{dict.holdingDaysScatter}</div>
          <ScatterChart
            points={dist.holdingReturns.map((s) => ({
              x: s.holdingDays,
              y: s.realizedPnL,
              label: `${s.ticker}: ${s.holdingDays}d, ${currency}${s.realizedPnL.toFixed(0)}`,
            }))}
          />
        </div>
      )}

      {(dist.maeReturns.length > 0 || dist.skippedMaeCount > 0) && (
        <div className="card report-section span-2">
          <div className="eyebrow">{dict.maeReturnScatter}</div>
          {dist.maeReturns.length > 0 && (
            <ScatterChart
              points={dist.maeReturns.map((s) => ({
                x: s.maePct,
                y: s.returnPct,
                label: `${s.ticker}: MAE ${s.maePct.toFixed(1)}%, ${s.returnPct.toFixed(1)}%`,
              }))}
            />
          )}
          <div className="stat-note">
            {dict.maeReturnNote}
            {dist.skippedMaeCount > 0 && ` — ${dist.skippedMaeCount} ${dict.skippedSamples}`}
          </div>
        </div>
      )}
    </div>
  );
}
