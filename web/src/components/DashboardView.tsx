import { useEffect, useState } from "react";
import { currencySymbol, fetchDashboard, type Dashboard, type Market } from "../api";
import type { Dictionary } from "../i18n";
import { DrawdownChart } from "./DrawdownChart";
import { KpiCard } from "./KpiCard";
import { PnlChart } from "./PnlChart";
import { PositionsTable } from "./PositionsTable";
import type { TradeMode } from "./TradeModal";

interface Props {
  dict: Dictionary;
  market: Market;
  // onTickerClick is Phase 7's drill-down from a held position into its
  // /chart page (docs/phase-7-support-resistance.md §5.1) — threaded down
  // to PositionsTable, whose ticker cell becomes a link only when this is
  // provided.
  onTickerClick?: (ticker: string) => void;
  // names is /api/company-names' TW ticker → Chinese-name map (see App.tsx),
  // forwarded to PositionsTable.
  names?: Record<string, string>;
  // onTrade/refreshSignal (Phase 10) forward to PositionsTable's row
  // buttons — onTrade opens App.tsx's shared TradeModal, refreshSignal is
  // bumped by App after a successful write so this view's own dashboard
  // fetch picks up the new position/stop-price state.
  onTrade?: (mode: TradeMode, ticker: string, prefillPrice?: number) => void;
  refreshSignal?: number;
}

// The dashboard screen's body, pulled out of App.tsx (Phase 5 PR2) so App
// can route between this and CalendarView — each view owns its own
// fetch/loading/error state rather than App prefetching everything upfront.
// Phase 6: refetches whenever the market toggle changes (see App.tsx),
// since /api/dashboard's numbers are market-scoped (buildDashboard).
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
      <PnlChart curve={curve} benchmark={benchmark} dict={dict} />
      {drawdown.length > 0 && (
        <>
          <div className="eyebrow">{dict.drawdownChart}</div>
          <DrawdownChart drawdown={drawdown} />
        </>
      )}
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
