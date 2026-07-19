import { useEffect, useRef } from "react";
import { createChart, type IChartApi, type ISeriesApi, type LineData } from "lightweight-charts";
import type { DateValue } from "../api";

interface Props {
  curve: DateValue[];
}

// lightweight-charts (not a second charting library) even for PR1's plain
// cumulative-P&L line — PR3 reuses it for the candlestick + buy/sell
// markers view, and the design doc's s1 series color is specced against
// this library's theming, not a generic chart component's.
export function PnlChart({ curve }: Props) {
  const containerRef = useRef<HTMLDivElement>(null);
  const seriesRef = useRef<ISeriesApi<"Area"> | null>(null);
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
    const data: LineData[] = curve.map((d) => ({ time: d.date, value: d.value }));
    seriesRef.current.setData(data);
    chartRef.current?.timeScale().fitContent();
  }, [curve]);

  return <div className="card chart-card" ref={containerRef} />;
}
