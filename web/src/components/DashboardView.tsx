import { useEffect, useState } from "react";
import { fetchDashboard, type Dashboard } from "../api";
import type { Dictionary } from "../i18n";
import { StatusBar } from "./StatusBar";
import { KpiCard } from "./KpiCard";
import { PnlChart } from "./PnlChart";
import { PositionsTable } from "./PositionsTable";

interface Props {
  dict: Dictionary;
}

// The dashboard screen's body, pulled out of App.tsx (Phase 5 PR2) so App
// can route between this and CalendarView — each view owns its own
// fetch/loading/error state rather than App prefetching everything upfront.
export function DashboardView({ dict }: Props) {
  const [dashboard, setDashboard] = useState<Dashboard | null>(null);
  const [error, setError] = useState(false);

  useEffect(() => {
    fetchDashboard()
      .then(setDashboard)
      .catch(() => setError(true));
  }, []);

  if (error) {
    return <div className="error-message">{dict.error}</div>;
  }
  if (!dashboard) {
    return <div className="loading">{dict.loading}</div>;
  }

  const { kpis, curve, positions, status } = dashboard;

  return (
    <>
      <StatusBar status={status} dict={dict} />
      <div className="kpi-grid">
        <KpiCard label={dict.netPnL} value={kpis.netPnL} format="currency" colorMode="pnl" />
        <KpiCard label={dict.winRate} value={kpis.winRate} format="percent" />
        <KpiCard label={dict.profitFactor} value={kpis.profitFactor} format="ratio" />
        <KpiCard label={dict.expectancy} value={kpis.expectancy} format="currency" colorMode="pnl" />
        <KpiCard
          label={dict.maxDrawdown}
          value={-Math.abs(kpis.maxDrawdown)}
          format="currency"
          colorMode="loss"
        />
      </div>
      <PnlChart curve={curve} />
      <div className="card">
        <div className="eyebrow">{dict.positions}</div>
        <PositionsTable positions={positions} dict={dict} />
      </div>
    </>
  );
}
