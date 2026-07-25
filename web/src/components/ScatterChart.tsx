interface Point {
  x: number;
  y: number;
  label?: string;
}

interface Props {
  points: Point[];
  height?: number;
}

// A hand-rolled SVG scatter plot — no charting library dependency for what
// a few dozen <circle> elements and a linear scale function cover (Phase 8
// PR4's two scatter views, docs/phase-8-trader-analytics.md §6.1.2/6.1.3).
// lightweight-charts (already a dependency elsewhere in this app) is a
// time-series library — neither axis here is time, so it doesn't fit.
export function ScatterChart({ points, height = 240 }: Props) {
  if (points.length === 0) {
    return null;
  }
  const width = 640;
  const pad = 36;

  const xs = points.map((p) => p.x);
  const ys = points.map((p) => p.y);
  const xMin = Math.min(...xs, 0);
  const xMax = Math.max(...xs, 0);
  const yMin = Math.min(...ys, 0);
  const yMax = Math.max(...ys, 0);
  const xRange = xMax - xMin || 1;
  const yRange = yMax - yMin || 1;

  const sx = (x: number) => pad + ((x - xMin) / xRange) * (width - 2 * pad);
  const sy = (y: number) => height - pad - ((y - yMin) / yRange) * (height - 2 * pad);

  return (
    <svg viewBox={`0 0 ${width} ${height}`} className="scatter-chart">
      <line x1={pad} y1={sy(0)} x2={width - pad} y2={sy(0)} className="scatter-axis" />
      <line x1={sx(0)} y1={pad} x2={sx(0)} y2={height - pad} className="scatter-axis" />
      {points.map((p, i) => (
        <circle
          key={i}
          cx={sx(p.x)}
          cy={sy(p.y)}
          r={3.5}
          className={p.y >= 0 ? "scatter-point-profit" : "scatter-point-loss"}
        >
          <title>{p.label ?? `${p.x.toFixed(1)}, ${p.y.toFixed(1)}`}</title>
        </circle>
      ))}
    </svg>
  );
}
