import { useEffect, useState } from "react";
import { currencySymbol, fetchDashboard, type Dashboard, type Market } from "../api";
import type { Dictionary } from "../i18n";
import { KpiCard } from "./KpiCard";
import { PnlChart } from "./PnlChart";
import { PositionsTable } from "./PositionsTable";

interface Props {
  dict: Dictionary;
  market: Market;
  // onTickerClick is Phase 7's drill-down from a held position into its
  // /chart page (docs/phase-7-support-resistance.md §5.1) — threaded down
  // to PositionsTable, whose ticker cell becomes a link only when this is
  // provided.
  onTickerClick?: (ticker: string) => void;
}

// The dashboard screen's body, pulled out of App.tsx (Phase 5 PR2) so App
// can route between this and CalendarView — each view owns its own
// fetch/loading/error state rather than App prefetching everything upfront.
// Phase 6: refetches whenever the market toggle changes (see App.tsx),
// since /api/dashboard's numbers are market-scoped (buildDashboard).
export function DashboardView({ dict, market, onTickerClick }: Props) {
  const [dashboard, setDashboard] = useState<Dashboard | null>(null);
  const [error, setError] = useState(false);

  useEffect(() => {
    setDashboard(null);
    setError(false);
    fetchDashboard(market)
      .then(setDashboard)
      .catch(() => setError(true));
  }, [market]);

  if (error) {
    return <div className="error-message">{dict.error}</div>;
  }
  if (!dashboard) {
    return <div className="loading">{dict.loading}</div>;
  }

  const { kpis, curve, positions } = dashboard;
  const currency = currencySymbol(market);

  return (
    <>
      <div className="kpi-grid">
        <KpiCard label={dict.netPnL} value={kpis.netPnL} format="currency" colorMode="pnl" currency={currency} />
        <KpiCard label={dict.winRate} value={kpis.winRate} format="percent" />
        <KpiCard label={dict.profitFactor} value={kpis.profitFactor} format="ratio" />
        <KpiCard
          label={dict.expectancy}
          value={kpis.expectancy}
          format="currency"
          colorMode="pnl"
          currency={currency}
        />
        <KpiCard
          label={dict.maxDrawdown}
          value={-Math.abs(kpis.maxDrawdown)}
          format="currency"
          colorMode="loss"
          currency={currency}
        />
      </div>
      <PnlChart curve={curve} />
      <div className="card">
        <div className="eyebrow">{dict.positions}</div>
        <PositionsTable positions={positions} dict={dict} currency={currency} onTickerClick={onTickerClick} />
      </div>
    </>
  );
}
