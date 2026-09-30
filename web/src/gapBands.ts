import type {
  ISeriesPrimitive,
  ISeriesPrimitivePaneRenderer,
  ISeriesPrimitivePaneView,
  SeriesAttachedParameter,
  Time,
} from "lightweight-charts";

// Phase 27 P4b — price bands for gaps, drawn by the chart itself as a series
// primitive (the design overlays a div, which drifts on zoom). A band runs from
// the gap bar to the bar that filled it, or to the right edge while open; open
// ones are brighter, dashed top and bottom, and labelled.

export interface GapBand {
  lo: number;
  hi: number;
  from: string; // the gap bar's date
  fill: string; // the filling bar's date, "" while open
  bull: boolean;
  selected: boolean;
}

type Target = Parameters<ISeriesPrimitivePaneRenderer["draw"]>[0];

const GREEN = "16,185,129";
const RED = "239,68,68";

export class GapBands implements ISeriesPrimitive<Time> {
  private param: SeriesAttachedParameter<Time> | null = null;
  private bands: GapBand[] = [];
  private openLabel = "";

  private view: ISeriesPrimitivePaneView = {
    zOrder: () => "bottom",
    renderer: () => ({ draw: (t) => this.draw(t) }),
  };

  attached(param: SeriesAttachedParameter<Time>) {
    this.param = param;
  }

  detached() {
    this.param = null;
  }

  paneViews() {
    return [this.view];
  }

  set(bands: GapBand[], openLabel: string) {
    this.bands = bands;
    this.openLabel = openLabel;
    this.param?.requestUpdate();
  }

  private draw(target: Target) {
    const p = this.param;
    if (!p || this.bands.length === 0) return;
    const ts = p.chart.timeScale();
    const bs = ts.options().barSpacing;
    target.useMediaCoordinateSpace(({ context: ctx, mediaSize }) => {
      for (const g of this.bands) {
        const x1 = ts.timeToCoordinate(g.from as Time);
        const y1 = p.series.priceToCoordinate(g.hi);
        const y2 = p.series.priceToCoordinate(g.lo);
        if (x1 === null || y1 === null || y2 === null) continue;
        const open = g.fill === "";
        const xFill = open ? null : ts.timeToCoordinate(g.fill as Time);
        const left = x1 - bs / 2;
        const right = open || xFill === null ? mediaSize.width : xFill + bs / 2;
        if (right < 0 || left > mediaSize.width) continue;

        const rgb = g.bull ? GREEN : RED;
        const top = Math.min(y1, y2);
        const h = Math.max(2, Math.abs(y2 - y1));
        const w = Math.max(2, right - left);

        ctx.fillStyle = `rgba(${rgb},${open ? 0.18 : 0.08})`;
        ctx.fillRect(left, top, w, h);
        if (open) {
          ctx.save();
          ctx.strokeStyle = `rgba(${rgb},0.75)`;
          ctx.lineWidth = 1;
          ctx.setLineDash([3, 3]);
          ctx.beginPath();
          ctx.moveTo(left, top + 0.5);
          ctx.lineTo(left + w, top + 0.5);
          ctx.moveTo(left, top + h - 0.5);
          ctx.lineTo(left + w, top + h - 0.5);
          ctx.stroke();
          ctx.restore();
          ctx.font = "10px 'JetBrains Mono', monospace";
          ctx.textAlign = "right";
          ctx.fillStyle = `rgb(${rgb})`;
          ctx.fillText(`${this.openLabel} ${g.lo.toFixed(2)}–${g.hi.toFixed(2)}`, mediaSize.width - 6, top - 5);
        }
        if (g.selected) {
          ctx.strokeStyle = "#a5b4fc";
          ctx.lineWidth = 1;
          ctx.strokeRect(left + 0.5, top + 0.5, w - 1, h - 1);
        }
      }
    });
  }
}
