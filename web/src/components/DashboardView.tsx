import { useEffect, useState } from "react";
import { currencySymbol, fetchDashboard, type Dashboard, type Market } from "../api";
import type { Dictionary } from "../i18n";
import { DrawdownChart } from "./DrawdownChart";
import { formatValue, KpiCard } from "./KpiCard";
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
      <div className="kpi-grid">
        <div className="card kpi-hero">
          <div>
            <div className="eyebrow">{dict.netPnL}</div>
            <div
              className={`kpi-hero-value ${
                kpis.netPnL > 0 ? "profit" : kpis.netPnL < 0 ? "loss" : ""
              }`}
            >
              {formatValue(kpis.netPnL, "currency", currency)}
            </div>
          </div>
          <div className="kpi-hero-sub">
            <div>
              <span className="eyebrow" style={{ display: "inline", marginRight: 6 }}>
                {dict.winRate}:
              </span>
              <span className="mono" style={{ color: "var(--ink)", fontWeight: 600 }}>
                {formatValue(kpis.winRate, "percent", currency)}
              </span>
            </div>
            <div>
              <span className="eyebrow" style={{ display: "inline", marginRight: 6 }}>
                {dict.profitFactor}:
              </span>
              <span className="mono" style={{ color: "var(--ink)", fontWeight: 600 }}>
                {formatValue(kpis.profitFactor, "ratio", currency)}
              </span>
            </div>
          </div>
        </div>
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
        <KpiCard label={dict.ytdReturn} value={kpis.ytdReturnPct} format="percentValue" colorMode="pnl" />
        <KpiCard label={dict.qtdReturn} value={kpis.qtdReturnPct} format="percentValue" colorMode="pnl" />
        <KpiCard label={dict.htdReturn} value={kpis.htdReturnPct} format="percentValue" colorMode="pnl" />
        <KpiCard
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
