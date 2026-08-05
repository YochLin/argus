import { useEffect, useState } from "react";
import { currencySymbol, fetchDashboard, type Dashboard, type Market } from "../api";
import type { Dictionary } from "../i18n";
import { DrawdownChart } from "./DrawdownChart";
import { formatValue, type KpiColorMode, type KpiFormat } from "./KpiCard";
import { PnlChart } from "./PnlChart";
import { PositionsTable } from "./PositionsTable";
import type { TradeMode } from "./TradeModal";

interface Props {
  dict: Dictionary;
  market: Market;
  onTickerClick?: (ticker: string) => void;
  names?: Record<string, string>;
  onTrade?: (mode: TradeMode, ticker: string, prefillPrice?: number) => void;
  refreshSignal?: number;
}

interface StripItemProps {
  label: string;
  value: number | null;
  format: KpiFormat;
  colorMode?: KpiColorMode;
  currency?: string;
}

function DashKpiStripItem({ label, value, format, colorMode = "neutral", currency = "$" }: StripItemProps) {
  let colorClass = "";
  if (value !== null) {
    if (colorMode === "loss") {
      colorClass = "loss";
    } else if (colorMode === "pnl") {
      colorClass = value > 0 ? "profit" : value < 0 ? "loss" : "";
    }
  }
  return (
    <div className="dash-kpi-strip-item">
      <div className="dash-kpi-strip-label">{label}</div>
      <div className={`dash-kpi-strip-value ${colorClass}`}>
        {value === null ? "—" : formatValue(value, format, currency)}
      </div>
    </div>
  );
}

export function DashboardView({ dict, market, onTickerClick, names, onTrade, refreshSignal }: Props) {
  const [dashboard, setDashboard] = useState<Dashboard | null>(null);
  const [error, setError] = useState(false);

  useEffect(() => {
    setDashboard(null);
    setError(false);
    fetchDashboard(market)
      .then(setDashboard)
      .catch(() => setError(true));
  }, [market, refreshSignal]);

  if (error) {
    return <div className="error-message">{dict.error}</div>;
  }
  if (!dashboard) {
    return <div className="loading">{dict.loading}</div>;
  }

  const { kpis, curve, drawdown, benchmark, positions } = dashboard;
  const currency = currencySymbol(market);

  return (
    <>
      <div className="dash-kpi-strip">
        <DashKpiStripItem label={dict.netPnL} value={kpis.netPnL} format="currency" colorMode="pnl" currency={currency} />
        <DashKpiStripItem label={dict.winRate} value={kpis.winRate} format="percent" />
        <DashKpiStripItem label={dict.profitFactor} value={kpis.profitFactor} format="ratio" />
        <DashKpiStripItem
          label={dict.expectancy}
          value={kpis.expectancy}
          format="currency"
          colorMode="pnl"
          currency={currency}
        />
        <DashKpiStripItem
          label={dict.maxDrawdown}
          value={-Math.abs(kpis.maxDrawdown)}
          format="currency"
          colorMode="loss"
          currency={currency}
        />
        <DashKpiStripItem label={dict.ytdReturn} value={kpis.ytdReturnPct} format="percentValue" colorMode="pnl" />
        <DashKpiStripItem label={dict.qtdReturn} value={kpis.qtdReturnPct} format="percentValue" colorMode="pnl" />
        <DashKpiStripItem label={dict.htdReturn} value={kpis.htdReturnPct} format="percentValue" colorMode="pnl" />
        <DashKpiStripItem
          label={dict.benchmarkAlpha}
          value={kpis.benchmarkAlpha}
          format="currency"
          colorMode="pnl"
          currency={currency}
        />
      </div>

      <div className="dash-charts-grid">
        <PnlChart curve={curve} benchmark={benchmark} dict={dict} />
        {drawdown.length > 0 && <DrawdownChart drawdown={drawdown} dict={dict} />}
      </div>

      <div className="card">
        <div className="eyebrow">{dict.positions}</div>
        <PositionsTable
          positions={positions}
          dict={dict}
          currency={currency}
          onTickerClick={onTickerClick}
          names={names}
          onTrade={onTrade}
        />
      </div>
    </>
  );
}
