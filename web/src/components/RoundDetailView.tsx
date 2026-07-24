import { useCallback, useEffect, useRef, useState } from "react";
import {
  createChart,
  type IChartApi,
  type ISeriesApi,
  type SeriesMarker,
  type Time,
} from "lightweight-charts";
import { currencySymbol, fetchRoundDetail, marketOf, type RoundDetail } from "../api";
import type { Dictionary } from "../i18n";
import { TradesTable } from "./TradesTable";

interface Props {
  dict: Dictionary;
  ticker: string;
  start: string;
  onBack: () => void;
}

// Phase 5 PR3's round detail page: lightweight-charts daily candlesticks
// (reusing the same library PR1's PnlChart introduced, per that component's
// own doc comment anticipating this) plus buy/sell markers for every leg in
// the round — green arrow-up below the bar for a BUY, red arrow-down above
// for a SELL, the same profit/loss color convention as everywhere else in
// the dashboard.
// MAEMFEBar is Phase 5 PR4's approximate MAE/MFE visualization (design doc
// §A2: "回合明細頁畫 MAE/MFE 長條" — the closest available substitute for
// an R-Multiple bar until a per-trade stop price exists to compute one).
// A plain two-sided bar off a shared zero line rather than a chart library
// addition: this is one number pair, not a series.
function MAEMFEBar({ dict, maePct, mfePct }: { dict: Dictionary; maePct: number; mfePct: number }) {
  const range = Math.max(Math.abs(maePct), Math.abs(mfePct), 1) * 1.15;
  const losePct = (Math.abs(Math.min(maePct, 0)) / range) * 100;
  const winPct = (Math.max(mfePct, 0) / range) * 100;

  return (
    <div className="card report-section">
      <div className="eyebrow">MAE / MFE</div>
      <div className="mae-mfe-track">
        <div className="mae-mfe-half mae-mfe-half-loss">
          <div className="mae-mfe-loss" style={{ width: `${losePct}%` }} />
        </div>
        <div className="mae-mfe-zero" />
        <div className="mae-mfe-half mae-mfe-half-win">
          <div className="mae-mfe-win" style={{ width: `${winPct}%` }} />
        </div>
      </div>
      <div className="mae-mfe-labels">
        <span className="loss">MAE {maePct.toFixed(1)}%</span>
        <span className="profit">MFE +{mfePct.toFixed(1)}%</span>
      </div>
      <div className="stat-note">{dict.maeMfeRoundNote}</div>
    </div>
  );
}

export function RoundDetailView({ dict, ticker, start, onBack }: Props) {
  const [detail, setDetail] = useState<RoundDetail | null>(null);
  const [error, setError] = useState(false);
  const chartRef = useRef<IChartApi | null>(null);
  const seriesRef = useRef<ISeriesApi<"Candlestick"> | null>(null);

  useEffect(() => {
    setDetail(null);
    setError(false);
    if (!ticker || !start) {
      setError(true);
      return;
    }
    fetchRoundDetail(ticker, start)
      .then(setDetail)
      .catch(() => setError(true));
  }, [ticker, start]);

  // Callback ref rather than useRef+mount-effect: the container div only
  // enters the DOM once `detail` has loaded (see the `{detail && (...)}`
  // below), so a `useEffect(..., [])` mount effect would already have run
  // — and bailed on a null containerRef.current — before the div ever
  // existed, permanently skipping chart creation. A callback ref fires
  // exactly when the node is actually attached/detached, whenever that is.
  const containerRef = useCallback((node: HTMLDivElement | null) => {
    if (chartRef.current) {
      chartRef.current.remove();
      chartRef.current = null;
      seriesRef.current = null;
    }
    if (!node) return;
    const chart = createChart(node, {
      layout: { background: { color: "transparent" }, textColor: "#9AA6BC" },
      grid: { vertLines: { color: "#26314B" }, horzLines: { color: "#26314B" } },
      rightPriceScale: { borderColor: "#26314B" },
      timeScale: { borderColor: "#26314B" },
      autoSize: true,
    });
    const series = chart.addCandlestickSeries({
      upColor: "#2FBF71",
      downColor: "#E5484D",
      borderVisible: false,
      wickUpColor: "#2FBF71",
      wickDownColor: "#E5484D",
    });
    chartRef.current = chart;
    seriesRef.current = series;
  }, []);

  useEffect(() => {
    if (!seriesRef.current || !detail) return;
    seriesRef.current.setData(
      detail.candles.map((c) => ({
        time: c.date as Time,
        open: c.open,
        high: c.high,
        low: c.low,
        close: c.close,
      })),
    );

    // setMarkers requires ascending time order; trades come back in
    // db.GetAllTransactions' date order already, but sort defensively since
    // that's an implementation detail of the API, not a documented contract.
    const trades = [...detail.trades].sort((a, b) => (a.date < b.date ? -1 : a.date > b.date ? 1 : 0));
    const currency = currencySymbol(marketOf(detail.ticker));
    const markers: SeriesMarker<Time>[] = trades.map((t) => ({
      time: t.date as Time,
      position: t.side === "BUY" ? "belowBar" : "aboveBar",
      color: t.side === "BUY" ? "#2FBF71" : "#E5484D",
      shape: t.side === "BUY" ? "arrowUp" : "arrowDown",
      text: `${t.side} ${t.shares}@${currency}${t.price.toFixed(2)}`,
    }));
    seriesRef.current.setMarkers(markers);
    chartRef.current?.timeScale().fitContent();
  }, [detail]);

  return (
    <>
      <button className="back-link" onClick={onBack}>
        {dict.back}
      </button>
      {error && <div className="error-message">{dict.error}</div>}
      {!error && !detail && <div className="loading">{dict.loading}</div>}
      {detail && (
        <>
          <div className="eyebrow round-detail-title">
            {detail.ticker} · {detail.start} → {detail.end || dict.open}
          </div>
          <div className="card chart-card" ref={containerRef} />
          {detail.hasMaeMfe && <MAEMFEBar dict={dict} maePct={detail.maePct} mfePct={detail.mfePct} />}
          <div className="card">
            <TradesTable dict={dict} transactions={detail.trades} currency={currencySymbol(marketOf(ticker))} />
          </div>
        </>
      )}
    </>
  );
}
