interface Props {
  values: number[];
  binCount?: number;
  unit?: string;
  xAxisLabel?: string;
  yAxisLabel?: string;
}

// A hand-rolled bar histogram (plain divs, same shape as RoundDetailView's
// pre-existing MAEMFEBar) — no charting library for what fixed-width bins
// and a percentage-height bar per bin cover (Phase 8 PR4's R-multiple
// distribution, docs/phase-8-trader-analytics.md §6.1.1).
export function Histogram({ values, binCount = 8, unit = "R", xAxisLabel, yAxisLabel }: Props) {
  if (values.length === 0) {
    return null;
  }
  const min = Math.min(...values);
  const max = Math.max(...values);
  const range = max - min || 1;
  const binWidth = range / binCount;

  const bins = new Array(binCount).fill(0);
  for (const v of values) {
    let idx = Math.floor((v - min) / binWidth);
    if (idx >= binCount) idx = binCount - 1;
    if (idx < 0) idx = 0;
    bins[idx]++;
  }
  const maxCount = Math.max(...bins);

  return (
    <div className="histogram-wrap">
      <div className="histogram-body">
        {yAxisLabel && <div className="histogram-y-label">{yAxisLabel}</div>}
        <div className="histogram-y-axis">
          <span>{maxCount}</span>
          <span>0</span>
        </div>
        <div className="histogram">
          {bins.map((count, i) => {
            const binStart = min + i * binWidth;
            const heightPct = maxCount > 0 ? (count / maxCount) * 100 : 0;
            return (
              <div className="histogram-col" key={i}>
                <div className="histogram-tip">
                  {binStart.toFixed(1)}
                  {unit} — n={count}
                </div>
                <div className="histogram-count">{count > 0 ? count : ""}</div>
                <div className="histogram-bar-track">
                  <div
                    className={`histogram-bar ${binStart >= 0 ? "histogram-bar-profit" : "histogram-bar-loss"}`}
                    style={{ height: `${heightPct}%` }}
                  />
                </div>
                <div className="histogram-label">{binStart.toFixed(1)}</div>
              </div>
            );
          })}
        </div>
      </div>
      {xAxisLabel && <div className="histogram-x-label">{xAxisLabel}</div>}
    </div>
  );
}
