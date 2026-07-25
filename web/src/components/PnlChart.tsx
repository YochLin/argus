import { useEffect, useRef } from "react";
import { createChart, LineStyle, type IChartApi, type ISeriesApi, type LineData } from "lightweight-charts";
import type { DateValue } from "../api";
import type { Dictionary } from "../i18n";

interface Props {
  curve: DateValue[];
  // benchmark is the Phase 8 PR2 same-cash-flow SPY/0050 replay, overlaid on
  // the same time/value axis as curve — a neutral dashed line (theme's
  // --ink-3) so it never competes visually with the profit/loss-colored
  // primary area series. Its date axis commonly starts later than curve's
  // own (docs/phase-8-trader-analytics.md §7), which lightweight-charts
  // handles for free: a series with fewer points simply doesn't draw before
  // its first one.
  benchmark?: DateValue[];
  dict?: Dictionary;
}

// lightweight-charts (not a second charting library) even for PR1's plain
// cumulative-P&L line — PR3 reuses it for the candlestick + buy/sell
// markers view, and the design doc's s1 series color is specced against
// this library's theming, not a generic chart component's.
export function PnlChart({ curve, benchmark, dict }: Props) {
  const containerRef = useRef<HTMLDivElement>(null);
  const seriesRef = useRef<ISeriesApi<"Area"> | null>(null);
  const benchSeriesRef = useRef<ISeriesApi<"Line"> | null>(null);
  const chartRef = useRef<IChartApi | null>(null);

  useEffect(() => {
    if (!containerRef.current) return;
    const chart = createChart(containerRef.current, {
      layout: { background: { color: "transparent" }, textColor: "#9AA6BC" },
      grid: {
        vertLines: { color: "#26314B" },
        horzLines: { color: "#26314B" },
      },
      rightPriceScale: { borderColor: "#26314B" },
      timeScale: { borderColor: "#26314B" },
      autoSize: true,
    });
    const series = chart.addAreaSeries({
      lineColor: "#C07F27", // --s1
      topColor: "rgba(192, 127, 39, 0.3)",
      bottomColor: "rgba(192, 127, 39, 0.0)",
      lineWidth: 2,
    });
    const benchSeries = chart.addLineSeries({
      color: "#64708A", // --ink-3, a neutral gray that never competes with profit/loss colors
      lineWidth: 1,
      lineStyle: LineStyle.Dashed,
    });
    chartRef.current = chart;
    seriesRef.current = series;
    benchSeriesRef.current = benchSeries;

    return () => {
      chart.remove();
      chartRef.current = null;
      seriesRef.current = null;
      benchSeriesRef.current = null;
    };
  }, []);

  useEffect(() => {
    if (!seriesRef.current) return;
    const data: LineData[] = curve.map((d) => ({ time: d.date, value: d.value }));
    seriesRef.current.setData(data);
    chartRef.current?.timeScale().fitContent();
  }, [curve]);

  useEffect(() => {
    if (!benchSeriesRef.current) return;
    const data: LineData[] = (benchmark ?? []).map((d) => ({ time: d.date, value: d.value }));
    benchSeriesRef.current.setData(data);
  }, [benchmark]);

  const showLegend = dict && benchmark && benchmark.length > 0;

  return (
    <>
      <div className="card chart-card" ref={containerRef} />
      {showLegend && (
        <div className="chart-legend">
          <span className="chart-legend-item">
            <span className="chart-legend-dot" style={{ background: "#C07F27" }} />
            {dict.myPortfolio}
          </span>
          <span className="chart-legend-item">
            <span className="chart-legend-dot chart-legend-dot-dashed" />
            {dict.benchmarkReplay}
          </span>
        </div>
      )}
    </>
  );
}
