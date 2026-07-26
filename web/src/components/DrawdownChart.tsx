import { useEffect, useRef } from "react";
import { createChart, type IChartApi, type ISeriesApi, type LineData } from "lightweight-charts";
import type { DateValue } from "../api";

interface Props {
  drawdown: DateValue[];
}

// The underwater chart (Phase 8 PR2, docs/phase-8-trader-analytics.md §4.1)
// — a short area chart directly below the main P&L curve, sharing its time
// axis visually (not literally synced — lightweight-charts has no built-in
// cross-chart axis lock, and drawdown's own values are a pure derivation of
// the same curve so the two always agree at a glance). Its lowest point is
// always exactly the dashboard's Max Drawdown KPI card figure.
export function DrawdownChart({ drawdown }: Props) {
  const containerRef = useRef<HTMLDivElement>(null);
  const seriesRef = useRef<ISeriesApi<"Area"> | null>(null);
  const chartRef = useRef<IChartApi | null>(null);

  useEffect(() => {
    if (!containerRef.current) return;
    // ponytail: fixed dark-palette grid/axis colors regardless of the App.tsx
      // light/dark toggle -- lightweight-charts takes plain JS config, not CSS
      // vars, and no view threads isDark down to this component yet. Upgrade:
      // accept an isDark prop and branch these per theme.css:root.light.
const chart = createChart(containerRef.current, {
      layout: { background: { color: "transparent" }, textColor: "#9AA6BC" },
      grid: {
        vertLines: { color: "#334155" },
        horzLines: { color: "#334155" },
      },
      rightPriceScale: { borderColor: "#334155" },
      timeScale: { borderColor: "#334155" },
      autoSize: true,
    });
    const series = chart.addAreaSeries({
      lineColor: "#EF4444", // --loss
      topColor: "rgba(229, 72, 77, 0.0)",
      bottomColor: "rgba(229, 72, 77, 0.35)",
      lineWidth: 1,
      invertFilledArea: true,
    });
    chartRef.current = chart;
    seriesRef.current = series;

    return () => {
      chart.remove();
      chartRef.current = null;
      seriesRef.current = null;
    };
  }, []);

  useEffect(() => {
    if (!seriesRef.current) return;
    const data: LineData[] = drawdown.map((d) => ({ time: d.date, value: d.value }));
    seriesRef.current.setData(data);
    chartRef.current?.timeScale().fitContent();
  }, [drawdown]);

  return <div className="card chart-card chart-card-sm" ref={containerRef} />;
}
