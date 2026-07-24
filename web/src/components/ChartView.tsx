import { useEffect, useRef, useState } from "react";
import { createChart, type IChartApi, type IPriceLine, type ISeriesApi, type Time } from "lightweight-charts";
import { currencySymbol, fetchChart, marketOf, type Chart, type ChartLevel } from "../api";
import type { Dictionary } from "../i18n";

interface Props {
  dict: Dictionary;
  ticker: string;
  onBack: () => void;
}

// maxPlottedPerSide caps how many support/resistance lines get drawn on the
// chart itself (nearest to the latest close, each side) — docs/phase-7-
// support-resistance.md §5.2: drawing every qualifying level at once turns
// into an unreadable smear of dashed lines, so only the most immediately
// relevant few are plotted; the rest are still visible in the table below.
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

// Phase 7's individual-ticker chart page (docs/phase-7-support-resistance.md
// §5.2): the same lightweight-charts candlestick setup RoundDetailView
// introduced, plus horizontal support/resistance price lines from
// signals.PriceLevels (internal/web's /api/chart). A visual reference tool
// only — no alerts, nothing written to signal_states, see the design doc's
// "誠實價值評估".
export function ChartView({ dict, ticker, onBack }: Props) {
  const [chart, setChart] = useState<Chart | null>(null);
  const [error, setError] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);
  const chartRef = useRef<IChartApi | null>(null);
  const seriesRef = useRef<ISeriesApi<"Candlestick"> | null>(null);
  const priceLinesRef = useRef<IPriceLine[]>([]);

  useEffect(() => {
    setChart(null);
    setError(false);
    if (!ticker) {
      setError(true);
      return;
    }
    fetchChart(ticker)
      .then(setChart)
      .catch(() => setError(true));
  }, [ticker]);

  useEffect(() => {
    if (!containerRef.current) return;
    const c = createChart(containerRef.current, {
      layout: { background: { color: "transparent" }, textColor: "#9AA6BC" },
      grid: { vertLines: { color: "#26314B" }, horzLines: { color: "#26314B" } },
      rightPriceScale: { borderColor: "#26314B" },
      timeScale: { borderColor: "#26314B" },
      autoSize: true,
    });
    const series = c.addCandlestickSeries({
      upColor: "#2FBF71",
      downColor: "#E5484D",
      borderVisible: false,
      wickUpColor: "#2FBF71",
      wickDownColor: "#E5484D",
    });
    chartRef.current = c;
    seriesRef.current = series;

    return () => {
      c.remove();
      chartRef.current = null;
      seriesRef.current = null;
      priceLinesRef.current = [];
    };
  }, []);

  const classified = chart && chart.candles.length > 0 ? classifyLevels(chart.levels, chart.candles[chart.candles.length - 1].close) : [];

  useEffect(() => {
    const series = seriesRef.current;
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

    for (const line of priceLinesRef.current) {
      series.removePriceLine(line);
    }
    priceLinesRef.current = classified
      .filter((l) => l.plotted)
      .map((l) =>
        series.createPriceLine({
          price: l.price,
          color: l.isSupport ? "#2FBF71" : "#E5484D",
          lineWidth: 1,
          lineStyle: 2, // dashed
          axisLabelVisible: true,
          title: `${l.price.toFixed(2)} ×${l.touches}`,
        }),
      );

    chartRef.current?.timeScale().fitContent();
  }, [chart]);

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

  return (
    <>
      <button className="back-link" onClick={onBack}>
        {dict.back}
      </button>
      <div className="eyebrow round-detail-title">{chart.ticker}</div>
      <div className="card chart-card" ref={containerRef} />
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
    </>
  );
}
