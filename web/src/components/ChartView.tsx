import { useCallback, useEffect, useRef, useState } from "react";
import {
  createChart,
  type IChartApi,
  type IPriceLine,
  type ISeriesApi,
  type SeriesMarker,
  type Time,
} from "lightweight-charts";
import {
  ApiError,
  currencySymbol,
  fetchChart,
  fetchRoundDetail,
  marketOf,
  setThesis,
  tickerLabel,
  type Candle,
  type Chart,
  type ChartLevel,
  type RoundDetail,
  type RoundSummary,
} from "../api";
import type { Dictionary } from "../i18n";
import { TradesTable } from "./TradesTable";
import type { TradeMode } from "./TradeModal";

interface Props {
  dict: Dictionary;
  ticker: string;
  initialRoundStart?: string;
  onBack: () => void;
  names?: Record<string, string>;
  writable?: boolean;
  onTrade?: (mode: TradeMode, ticker: string, prefillPrice?: number) => void;
  // onUnauthorized backs the round-detail thesis edit form's 401 handling —
  // same retry-after-login convention as TradeModal/ChartListView, needed
  // here since that edit is an inline write, not routed through TradeModal.
  onUnauthorized?: (retry: () => void) => void;
}

const maxPlottedPerSide = 3;

interface ClassifiedLevel extends ChartLevel {
  isSupport: boolean;
  distance: number;
  plotted: boolean;
}

function classifyLevels(levels: ChartLevel[], lastClose: number): ClassifiedLevel[] {
  const withSide = levels.map((l) => ({
    ...l,
    isSupport: l.price < lastClose,
    distance: Math.abs(l.price - lastClose),
  }));

  const nearestSupport = withSide
    .filter((l) => l.isSupport)
    .sort((a, b) => a.distance - b.distance)
    .slice(0, maxPlottedPerSide);
  const nearestResistance = withSide
    .filter((l) => !l.isSupport)
    .sort((a, b) => a.distance - b.distance)
    .slice(0, maxPlottedPerSide);
  const plottedPrices = new Set([...nearestSupport, ...nearestResistance].map((l) => l.price));

  return withSide
    .map((l) => ({ ...l, plotted: plottedPrices.has(l.price) }))
    .sort((a, b) => a.distance - b.distance);
}

function computeQuickStats(candles: Candle[]) {
  if (!candles || candles.length === 0) {
    return null;
  }
  const n = candles.length;
  const latest = candles[n - 1];

  const slice20 = candles.slice(-20);
  const ma20 = slice20.reduce((acc, c) => acc + c.close, 0) / slice20.length;

  const slice60 = candles.slice(-60);
  const ma60 = slice60.reduce((acc, c) => acc + c.close, 0) / slice60.length;

  let trSum = 0;
  const atrCount = Math.min(14, n);
  const atrSlice = candles.slice(-atrCount);
  for (let i = 0; i < atrSlice.length; i++) {
    const c = atrSlice[i];
    if (i === 0) {
      trSum += c.high - c.low;
    } else {
      const prev = atrSlice[i - 1];
      const tr = Math.max(c.high - c.low, Math.abs(c.high - prev.close), Math.abs(c.low - prev.close));
      trSum += tr;
    }
  }
  const atr14 = trSum / atrCount;

  const c20Ago = n > 20 ? candles[n - 1 - 20].close : candles[0].close;
  const ret20 = ((latest.close - c20Ago) / c20Ago) * 100;

  const slice120 = candles.slice(-120);
  const high120 = Math.max(...slice120.map((c) => c.high));
  const low120 = Math.min(...slice120.map((c) => c.low));
  const fromHigh = ((latest.close - high120) / high120) * 100;

  const avgVol20 = slice20.reduce((acc, c) => acc + c.volume, 0) / slice20.length;
  const vsAvg20 = avgVol20 > 0 ? ((latest.volume - avgVol20) / avgVol20) * 100 : 0;

  const rangeSpan = high120 - low120 || 1;
  const rangePct = Math.min(100, Math.max(0, ((latest.close - low120) / rangeSpan) * 100));

  return {
    latestClose: latest.close,
    ma20,
    ma60,
    atr14,
    ret20,
    fromHigh,
    vsAvg20,
    high120,
    low120,
    rangePct,
  };
}

function MAEMFEBar({ dict, maePct, mfePct }: { dict: Dictionary; maePct: number; mfePct: number }) {
  const range = Math.max(Math.abs(maePct), Math.abs(mfePct), 1) * 1.15;
  const losePct = (Math.abs(Math.min(maePct, 0)) / range) * 100;
  const winPct = (Math.max(mfePct, 0) / range) * 100;

  return (
    <div className="report-section">
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

export function ChartView({
  dict,
  ticker,
  initialRoundStart,
  onBack,
  names = {},
  writable = false,
  onTrade,
  onUnauthorized,
}: Props) {
  const [chart, setChart] = useState<Chart | null>(null);
  const [error, setError] = useState(false);
  const [selectedRoundStart, setSelectedRoundStart] = useState<string | null>(initialRoundStart ?? null);
  const [roundDetail, setRoundDetail] = useState<RoundDetail | null>(null);
  const [thesisDraft, setThesisDraft] = useState("");
  const [thesisSubmitting, setThesisSubmitting] = useState(false);
  const [thesisError, setThesisError] = useState<string | null>(null);

  const chartRef = useRef<IChartApi | null>(null);
  const seriesRef = useRef<ISeriesApi<"Candlestick"> | null>(null);
  const volumeSeriesRef = useRef<ISeriesApi<"Histogram"> | null>(null);
  const priceLinesRef = useRef<IPriceLine[]>([]);

  useEffect(() => {
    setChart(null);
    setError(false);
    setSelectedRoundStart(initialRoundStart ?? null);
    setRoundDetail(null);
    if (!ticker) {
      setError(true);
      return;
    }
    fetchChart(ticker)
      .then(setChart)
      .catch(() => setError(true));
  }, [ticker, initialRoundStart]);

  useEffect(() => {
    setThesisDraft("");
    setThesisError(null);
    if (!selectedRoundStart || !ticker) {
      setRoundDetail(null);
      return;
    }
    fetchRoundDetail(ticker, selectedRoundStart)
      .then(setRoundDetail)
      .catch(() => setRoundDetail(null));
  }, [ticker, selectedRoundStart]);

  async function submitThesis() {
    if (!ticker || !selectedRoundStart || !thesisDraft.trim()) return;
    setThesisSubmitting(true);
    setThesisError(null);
    try {
      await setThesis(ticker, thesisDraft.trim());
      setThesisDraft("");
      setRoundDetail(await fetchRoundDetail(ticker, selectedRoundStart));
    } catch (e) {
      if (e instanceof ApiError && e.status === 401 && onUnauthorized) {
        onUnauthorized(submitThesis);
      } else {
        setThesisError(e instanceof ApiError ? e.message : dict.error);
      }
    } finally {
      setThesisSubmitting(false);
    }
  }

  const containerRef = useCallback((node: HTMLDivElement | null) => {
    if (chartRef.current) {
      chartRef.current.remove();
      chartRef.current = null;
      seriesRef.current = null;
      volumeSeriesRef.current = null;
      priceLinesRef.current = [];
    }
    if (!node) return;
    const c = createChart(node, {
      layout: { background: { color: "transparent" }, textColor: "#9AA6BC", fontFamily: '"JetBrains Mono", monospace' },
      grid: { vertLines: { color: "#243044" }, horzLines: { color: "#243044" } },
      rightPriceScale: { borderColor: "#334155" },
      timeScale: { borderColor: "#334155" },
      autoSize: true,
    });

    const series = c.addCandlestickSeries({
      upColor: "#10B981",
      downColor: "#EF4444",
      borderVisible: false,
      wickUpColor: "#10B981",
      wickDownColor: "#EF4444",
    });

    const vol = c.addHistogramSeries({
      priceScaleId: "vol",
      priceFormat: { type: "volume" },
      priceLineVisible: false,
      lastValueVisible: false,
    });
    c.priceScale("vol").applyOptions({
      scaleMargins: { top: 0.8, bottom: 0 },
      visible: false,
    });

    chartRef.current = c;
    seriesRef.current = series;
    volumeSeriesRef.current = vol;
  }, []);

  const classified =
    chart && chart.candles.length > 0
      ? classifyLevels(chart.levels, chart.candles[chart.candles.length - 1].close)
      : [];

  useEffect(() => {
    const series = seriesRef.current;
    const vol = volumeSeriesRef.current;
    if (!series || !chart) return;

    series.setData(
      chart.candles.map((c) => ({
        time: c.date as Time,
        open: c.open,
        high: c.high,
        low: c.low,
        close: c.close,
      })),
    );

    if (vol) {
      vol.setData(
        chart.candles.map((c) => ({
          time: c.date as Time,
          value: c.volume,
          color: c.close >= c.open ? "rgba(16,185,129,0.4)" : "rgba(239,68,68,0.4)",
        })),
      );
    }

    for (const line of priceLinesRef.current) {
      series.removePriceLine(line);
    }
    priceLinesRef.current = classified
      .filter((l) => l.plotted)
      .map((l) =>
        series.createPriceLine({
          price: l.price,
          color: l.isSupport ? "#10B981" : "#EF4444",
          lineWidth: 1,
          lineStyle: 2,
          axisLabelVisible: true,
          title: `${l.price.toFixed(2)} ×${l.touches}`,
        }),
      );

    if (chart.position) {
      priceLinesRef.current.push(
        series.createPriceLine({
          price: chart.position.avgCost,
          color: "#818cf8",
          lineWidth: 1,
          lineStyle: 0,
          axisLabelVisible: true,
          title: `${dict.avgCost} ${chart.position.avgCost.toFixed(2)}`,
        }),
      );
    }

    if (roundDetail) {
      const trades = [...roundDetail.trades].sort((a, b) =>
        a.date < b.date ? -1 : a.date > b.date ? 1 : 0,
      );
      const currency = currencySymbol(marketOf(chart.ticker));
      const markers: SeriesMarker<Time>[] = trades.map((t) => ({
        time: t.date as Time,
        position: t.side === "BUY" ? "belowBar" : "aboveBar",
        color: t.side === "BUY" ? "#10B981" : "#EF4444",
        shape: t.side === "BUY" ? "arrowUp" : "arrowDown",
        text: `${t.side} ${t.shares}@${currency}${t.price.toFixed(2)}`,
      }));
      series.setMarkers(markers);

      const fromDate = roundDetail.start;
      const toDate = roundDetail.end || chart.candles[chart.candles.length - 1].date;
      chartRef.current?.timeScale().setVisibleRange({
        from: fromDate as Time,
        to: toDate as Time,
      });
    } else {
      series.setMarkers([]);
      chartRef.current?.timeScale().fitContent();
    }
  }, [chart, roundDetail, classified, dict.avgCost]);

  if (error) {
    return (
      <>
        <button className="back-link" onClick={onBack}>
          {dict.back}
        </button>
        <div className="error-message">{dict.error}</div>
      </>
    );
  }
  if (!chart) {
    return <div className="loading">{dict.loading}</div>;
  }

  const currency = currencySymbol(marketOf(chart.ticker));
  const stats = computeQuickStats(chart.candles);
  const latestPrice = chart.candles[chart.candles.length - 1]?.close ?? 0;
  const prevClose =
    chart.candles.length > 1 ? chart.candles[chart.candles.length - 2].close : latestPrice;
  const dayChangePct = prevClose ? ((latestPrice - prevClose) / prevClose) * 100 : 0;

  const handleChipClick = (round: RoundSummary) => {
    if (selectedRoundStart === round.start) {
      setSelectedRoundStart(null);
    } else {
      setSelectedRoundStart(round.start);
    }
  };

  return (
    <>
      <button className="back-link" onClick={onBack}>
        {dict.back}
      </button>

      <div className="ticker-header-row">
        <div>
          <div className="ticker-header-id mono">{tickerLabel(chart.ticker, names)}</div>
          <div className="ticker-price-row">
            <span className="ticker-price-val mono">
              {currency}
              {latestPrice.toFixed(2)}
            </span>
            <span className={`watchlist-change-badge ${dayChangePct >= 0 ? "profit" : "loss"}`}>
              {dayChangePct >= 0 ? "+" : ""}
              {dayChangePct.toFixed(2)}%
            </span>
          </div>
        </div>

        {stats && (
          <div className="ticker-stats-inline">
            <div>
              <div className="eyebrow">{dict.ma20}</div>
              <div className="ticker-stat-val mono">{currency}{stats.ma20.toFixed(2)}</div>
            </div>
            <div>
              <div className="eyebrow">{dict.ma60}</div>
              <div className="ticker-stat-val mono">{currency}{stats.ma60.toFixed(2)}</div>
            </div>
            <div>
              <div className="eyebrow">{dict.atr14}</div>
              <div className="ticker-stat-val mono">{currency}{stats.atr14.toFixed(2)}</div>
            </div>
            <div>
              <div className="eyebrow">{dict.ret20}</div>
              <div className="ticker-stat-val mono">
                {stats.ret20 >= 0 ? "+" : ""}
                {stats.ret20.toFixed(1)}%
              </div>
              <div className={`ticker-stat-note ${stats.ret20 >= 0 ? "profit" : "loss"}`}>
                {stats.ret20 >= 0 ? dict.above : dict.below}
              </div>
            </div>
            <div>
              <div className="eyebrow">{dict.fromHigh}</div>
              <div className="ticker-stat-val mono">{stats.fromHigh.toFixed(1)}%</div>
              <div className={`ticker-stat-note ${stats.fromHigh >= 0 ? "profit" : "loss"}`}>
                {stats.fromHigh >= 0 ? dict.above : dict.below}
              </div>
            </div>
            <div>
              <div className="eyebrow">{dict.vsAvg20}</div>
              <div className="ticker-stat-val mono">
                {stats.vsAvg20 >= 0 ? "+" : ""}
                {stats.vsAvg20.toFixed(1)}%
              </div>
              <div className={`ticker-stat-note ${stats.vsAvg20 >= 0 ? "profit" : "loss"}`}>
                {stats.vsAvg20 >= 0 ? dict.above : dict.below}
              </div>
            </div>
          </div>
        )}

        {writable && onTrade && (
          <div className="ticker-header-actions">
            <button
              className="btn-tint btn-tint-buy"
              onClick={() => onTrade("buy", chart.ticker, latestPrice)}
            >
              {dict.buy}
            </button>
            <button
              className="btn-tint btn-tint-sell"
              onClick={() => onTrade("sell", chart.ticker, latestPrice)}
            >
              {dict.sell}
            </button>
            <button
              className="btn-sm"
              onClick={() =>
                onTrade("stop", chart.ticker, chart.position?.stopPrice || latestPrice)
              }
            >
              {dict.stopPriceCol}
            </button>
            <button className="btn-sm" onClick={() => onTrade("buyalert", chart.ticker, latestPrice)}>
              {dict.addBuyAlert}
            </button>
          </div>
        )}
      </div>

      {stats && (
        <div className="range-bar-wrap">
          <div className="range-bar-track">
            <div className="range-bar-fill" style={{ width: `${stats.rangePct}%` }} />
            <div className="range-bar-marker" style={{ left: `${stats.rangePct}%` }} />
          </div>
          <div className="range-bar-labels">
            <span>
              {dict.rangeLow}: {currency}{stats.low120.toFixed(2)}
            </span>
            <span>
              {dict.rangeNote} ({stats.rangePct.toFixed(0)}%)
            </span>
            <span>
              {dict.rangeHigh}: {currency}{stats.high120.toFixed(2)}
            </span>
          </div>
        </div>
      )}

      {chart.rounds.length === 0 ? (
        <div className="empty-message" style={{ marginBottom: 16 }}>
          {dict.noRoundsHere}
        </div>
      ) : (
        <div className="round-chips">
          <span className="stat-note">{dict.roundPicker}</span>
          <button
            className={`round-chip ${selectedRoundStart === null ? "active" : ""}`}
            onClick={() => setSelectedRoundStart(null)}
          >
            {dict.allTrades}
          </button>
          {chart.rounds.map((r) => {
            const isActiveChip = selectedRoundStart === r.start;
            return (
              <button
                key={r.start}
                className={`round-chip ${isActiveChip ? "active" : ""}`}
                onClick={() => handleChipClick(r)}
              >
                {r.start} → {r.end || dict.open} ({r.shares} sh ·{" "}
                <span className={r.realizedPnL > 0 ? "profit" : r.realizedPnL < 0 ? "loss" : ""}>
                  {r.realizedPnL > 0 ? "+" : ""}
                  {currency}
                  {Math.abs(r.realizedPnL).toFixed(0)}
                </span>
                )
              </button>
            );
          })}
        </div>
      )}

      <div className="card chart-card" ref={containerRef} />

      {roundDetail && (
        <div className="detail-grid-2col">
          <div className="card">
            {roundDetail.hasMaeMfe && (
              <MAEMFEBar dict={dict} maePct={roundDetail.maePct} mfePct={roundDetail.mfePct} />
            )}
            <div className="eyebrow" style={{ marginTop: roundDetail.hasMaeMfe ? 16 : 0 }}>
              {dict.tradesInRound}
            </div>
            <TradesTable
              dict={dict}
              transactions={roundDetail.trades}
              currency={currency}
              names={names}
            />
          </div>
          <div className="card">
            {(roundDetail.theses.length > 0 || (writable && roundDetail.editable)) && (
              <>
                <div className="eyebrow">{dict.thesisLabel}</div>
                {roundDetail.theses.length > 0 && (
                  <ul className="lessons-list">
                    {roundDetail.theses.map((t, i) => (
                      <li key={i}>
                        <span className="stat-note">{t.date}</span> {t.text}
                      </li>
                    ))}
                  </ul>
                )}
                {writable && roundDetail.editable && (
                  <div style={{ marginTop: roundDetail.theses.length > 0 ? 8 : 0 }}>
                    <label className="form-field">
                      <textarea
                        className="mono"
                        rows={2}
                        value={thesisDraft}
                        placeholder={dict.thesisFieldPlaceholder}
                        onChange={(e) => setThesisDraft(e.target.value)}
                      />
                    </label>
                    {thesisError && <div className="error-message">{thesisError}</div>}
                    <button
                      type="button"
                      className="btn-sm"
                      disabled={!thesisDraft.trim() || thesisSubmitting}
                      onClick={submitThesis}
                    >
                      {dict.thesisEditToggle}
                    </button>
                  </div>
                )}
              </>
            )}
            {roundDetail.lessons.length > 0 && (
              <>
                <div className="eyebrow" style={{ marginTop: roundDetail.theses.length > 0 || roundDetail.editable ? 16 : 0 }}>
                  {dict.lessonsLabel}
                </div>
                <ul className="lessons-list">
                  {roundDetail.lessons.map((l, i) => (
                    <li key={i}>
                      <span className="stat-note">{l.date}</span> {l.lesson}
                    </li>
                  ))}
                </ul>
              </>
            )}
          </div>
        </div>
      )}

      <div className="detail-grid-2col">
        <div className="card">
          <div className="eyebrow">
            {dict.support} / {dict.resistance}
          </div>
          {classified.length === 0 ? (
            <div className="empty-message">{dict.noLevels}</div>
          ) : (
            <table className="mono">
              <thead>
                <tr>
                  <th>{dict.levelType}</th>
                  <th>{dict.price}</th>
                  <th>{dict.touches}</th>
                  <th>{dict.lastTouch}</th>
                </tr>
              </thead>
              <tbody>
                {classified.map((l) => (
                  <tr key={l.price}>
                    <td className={l.isSupport ? "profit" : "loss"}>
                      {l.plotted ? "● " : ""}
                      {l.isSupport ? dict.support : dict.resistance}
                    </td>
                    <td>
                      {currency}
                      {l.price.toFixed(2)}
                    </td>
                    <td>{l.touches}</td>
                    <td>{l.lastDate}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>

        <div className="card">
          <div className="eyebrow">{dict.thisPosition}</div>
          {!chart.position ? (
            <div className="empty-message">{dict.noPositionHere}</div>
          ) : (
            <div className="position-rows">
              <div className="position-row">
                <span className="stat-note">{dict.shares}</span>
                <span className="position-row-val">{chart.position.shares}</span>
              </div>
              <div className="position-row">
                <span className="stat-note">{dict.avgCost}</span>
                <span className="position-row-val">
                  {currency}
                  {chart.position.avgCost.toFixed(2)}
                </span>
              </div>
              <div className="position-row">
                <span className="stat-note">{dict.marketValue}</span>
                <span className="position-row-val">
                  {currency}
                  {chart.position.value.toLocaleString(undefined, { maximumFractionDigits: 0 })}
                </span>
              </div>
              <div className="position-row">
                <span className="stat-note">{dict.weight}</span>
                <span className="position-row-val">{chart.position.weightPct.toFixed(1)}%</span>
              </div>
              <div className="position-row">
                <span className="stat-note">{dict.unrealizedPnL}</span>
                <span
                  className={`position-row-val ${
                    chart.position.unrealizedPnLPct >= 0 ? "profit" : "loss"
                  }`}
                >
                  {chart.position.unrealizedPnLPct >= 0 ? "+" : ""}
                  {chart.position.unrealizedPnLPct.toFixed(1)}%
                </span>
              </div>
              <div className="position-row">
                <span className="stat-note">{dict.stopPriceCol}</span>
                <span className="position-row-val">
                  {chart.position.stopPrice > 0
                    ? `${currency}${chart.position.stopPrice.toFixed(2)}`
                    : "—"}
                </span>
              </div>
              <div className="position-row">
                <span className="stat-note">{dict.riskIfStopped}</span>
                <span className="position-row-val">
                  {chart.position.openRisk !== null
                    ? `${currency}${chart.position.openRisk.toLocaleString(undefined, {
                        maximumFractionDigits: 0,
                      })} (${chart.position.openRiskPct?.toFixed(1)}%)`
                    : "—"}
                </span>
              </div>
            </div>
          )}
        </div>
      </div>
    </>
  );
}
