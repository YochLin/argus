interface Point {
  x: number;
  y: number;
  label?: string;
}

interface Props {
  points: Point[];
  height?: number;
  xLabel?: string;
  yLabel?: string;
  fmtX?: (n: number) => string;
  fmtY?: (n: number) => string;
}

const defaultFmt = (n: number) => (Number.isInteger(n) ? n.toFixed(0) : n.toFixed(1));

function anchorFor(t: number, ticks: number[]): "start" | "middle" | "end" {
  if (ticks.length === 1) return "middle";
  if (t === ticks[0]) return "start";
  if (t === ticks[ticks.length - 1]) return "end";
  return "middle";
}

// A hand-rolled SVG scatter plot — no charting library dependency for what
// a few dozen <circle> elements and a linear scale function cover (Phase 8
// PR4's two scatter views, docs/phase-8-trader-analytics.md §6.1.2/6.1.3).
// lightweight-charts (already a dependency elsewhere in this app) is a
// time-series library — neither axis here is time, so it doesn't fit.
//
// Points carry no native <title> tooltip — that's a 1s-delayed, OS-styled
// browser tooltip on a 3.5px hit target. Each point instead gets a 10px
// transparent hit circle plus a CSS-revealed <text> label (Reports page
// readability pass, 2026-08-08).
export function ScatterChart({ points, height = 240, xLabel, yLabel, fmtX = defaultFmt, fmtY = defaultFmt }: Props) {
  if (points.length === 0) {
    return null;
  }
  const width = 640;
  const padL = 64;
  const padB = 40;
  const padT = 14;
  const padR = 16;

  const xs = points.map((p) => p.x);
  const ys = points.map((p) => p.y);
  const xMin = Math.min(...xs, 0);
  const xMax = Math.max(...xs, 0);
  const yMin = Math.min(...ys, 0);
  const yMax = Math.max(...ys, 0);
  const xRange = xMax - xMin || 1;
  const yRange = yMax - yMin || 1;

  const sx = (x: number) => padL + ((x - xMin) / xRange) * (width - padL - padR);
  const sy = (y: number) => height - padB - ((y - yMin) / yRange) * (height - padT - padB);

  const xTicks = Array.from(new Set([xMin, (xMin + xMax) / 2, xMax]));
  const yTicks = Array.from(new Set([yMin, (yMin + yMax) / 2, yMax]));
  const yMidY = (padT + height - padB) / 2;

  return (
    <svg viewBox={`0 0 ${width} ${height}`} className="scatter-chart">
      <line x1={padL} y1={sy(0)} x2={width - padR} y2={sy(0)} className="scatter-axis" />
      <line x1={sx(0)} y1={padT} x2={sx(0)} y2={height - padB} className="scatter-axis" />
      {xTicks.map((t, i) => (
        <text key={`x${i}`} x={sx(t)} y={height - padB + 14} textAnchor={anchorFor(t, xTicks)} className="scatter-tick">
          {fmtX(t)}
        </text>
      ))}
      {yTicks.map((t, i) => (
        <text key={`y${i}`} x={padL - 8} y={sy(t) + 3} textAnchor="end" className="scatter-tick">
          {fmtY(t)}
        </text>
      ))}
      {xLabel && (
        <text x={(padL + width - padR) / 2} y={height - 4} textAnchor="middle" className="scatter-axis-label">
          {xLabel}
        </text>
      )}
      {yLabel && (
        <text x={10} y={yMidY} textAnchor="middle" className="scatter-axis-label" transform={`rotate(-90, 10, ${yMidY})`}>
          {yLabel}
        </text>
      )}
      {points.map((p, i) => {
        const cx = sx(p.x);
        const cy = sy(p.y);
        const onRight = cx > width / 2;
        return (
          <g key={i} className="scatter-point-group" tabIndex={0}>
            <circle cx={cx} cy={cy} r={10} className="scatter-hit" />
            <circle cx={cx} cy={cy} r={3.5} className={p.y >= 0 ? "scatter-point-profit" : "scatter-point-loss"} />
            <text x={cx + (onRight ? -8 : 8)} y={cy - 8} textAnchor={onRight ? "end" : "start"} className="scatter-point-label">
              {p.label ?? `${fmtX(p.x)}, ${fmtY(p.y)}`}
            </text>
          </g>
        );
      })}
    </svg>
  );
}
