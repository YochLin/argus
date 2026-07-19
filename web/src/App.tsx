import { useEffect, useState } from "react";
import { fetchConfig, fetchDashboard, type Dashboard } from "./api";
import { getDictionary, type Dictionary } from "./i18n";
import { StatusBar } from "./components/StatusBar";
import { KpiCard } from "./components/KpiCard";
import { PnlChart } from "./components/PnlChart";
import { PositionsTable } from "./components/PositionsTable";

export default function App() {
  const [dict, setDict] = useState<Dictionary>(getDictionary("zh"));
  const [dashboard, setDashboard] = useState<Dashboard | null>(null);
  const [error, setError] = useState(false);

  useEffect(() => {
    // /api/config's failure isn't fatal — the page still works with the
    // zh default dictionary already in state, just possibly the wrong
    // language.
    fetchConfig()
      .then((cfg) => setDict(getDictionary(cfg.lang)))
      .catch(() => {});
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
    <div className="app">
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
    </div>
  );
}
