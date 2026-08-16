import { useEffect, useMemo, useState } from "react";
import {
  currencySymbol,
  fetchSectorFlow,
  tickerLabel,
  type Market,
  type SectorFlow,
  type SectorFlowTimeframe,
} from "../api";
import type { Dictionary } from "../i18n";
import { InfoTip } from "./InfoTip";

interface Props {
  dict: Dictionary;
  market: Market;
  names?: Record<string, string>;
  onTickerClick?: (ticker: string) => void;
}

type SizeBy = "cap" | "flow";

// Virtual canvas squarify operates in — rendered as CSS percentages inside a
// fixed-aspect-ratio container, so the layout math never needs to know the
// container's real pixel size (no ResizeObserver). 1000x560 is close enough
// to a typical dashboard content area that the size-threshold checks below
// (which mirror the reference design's real-pixel rw>52/rh>34 checks) still
// land in roughly the right place.
const CANVAS_W = 1000;
const CANVAS_H = 560;
// ponytail: SECTOR_HEADER_H is virtual canvas units, but .flow-sector-header
// is a real 28px in CSS — the two only coincide when the rendered card is
// ~1000px wide. Item placement inside a sector still always sums to exactly
// 100% of the real header-subtracted area (percentages are self-consistent),
// so this mismatch only skews squarify's aspect-ratio optimization slightly
// at other widths, never breaks the layout. Fix if it looks visibly off:
// measure .flow-sector-body's real height via ResizeObserver instead.
const SECTOR_HEADER_H = 28;

interface Rect {
  x: number;
  y: number;
  w: number;
  h: number;
}

// squarify is the standard squarified-treemap layout algorithm (Bruls/
// Huizing/van Wijk): greedily grow the current row while doing so improves
// its worst box aspect ratio, lay the row out along the container's short
// side, then recurse into the remaining rect. Used twice — once for the
// sector-level blocks, once more inside each sector for its constituent
// tickers — per docs/phase-18-sector-money-flow.md §4.5.
function squarify<T extends { value: number }>(items: T[], x: number, y: number, w: number, h: number): (T & { rect: Rect })[] {
  const result: (T & { rect: Rect })[] = [];
  let remaining = items.filter((it) => it.value > 0);
  let rx = x, ry = y, rw = w, rh = h;

  while (remaining.length > 0 && rw > 0 && rh > 0) {
    const total = remaining.reduce((s, it) => s + it.value, 0);
    const shortSide = Math.min(rw, rh);
    const scale = (rw * rh) / total;

    let row: T[] = [];
    let rowSum = 0;
    let bestWorst = Infinity;
    for (const it of remaining) {
      const testSum = rowSum + it.value;
      const areas = [...row, it].map((r) => r.value * scale);
      const worst = worstRatio(areas, testSum * scale, shortSide);
      if (worst <= bestWorst) {
        row.push(it);
        rowSum = testSum;
        bestWorst = worst;
      } else {
        break;
      }
    }

    const rowArea = rowSum * scale;
    const rowThickness = rowArea / shortSide;
    const horizontal = rw >= rh; // row fills a vertical strip, items stack top-to-bottom
    let offset = 0;
    for (const it of row) {
      const itemLength = (it.value * scale) / rowThickness;
      if (horizontal) {
        result.push({ ...it, rect: { x: rx, y: ry + offset, w: rowThickness, h: itemLength } });
      } else {
        result.push({ ...it, rect: { x: rx + offset, y: ry, w: itemLength, h: rowThickness } });
      }
      offset += itemLength;
    }

    if (horizontal) {
      rx += rowThickness;
      rw -= rowThickness;
    } else {
      ry += rowThickness;
      rh -= rowThickness;
    }
    remaining = remaining.slice(row.length);
  }
  return result;
}

function worstRatio(areas: number[], sum: number, side: number): number {
  const maxA = Math.max(...areas);
  const minA = Math.min(...areas);
  return Math.max((side * side * maxA) / (sum * sum), (sum * sum) / (side * side * minA));
}

// tintFor mirrors the reference design's tint(chg): neutral gray under a
// 0.06% deadband, otherwise red/green with opacity scaled by |chg| relative
// to a per-timeframe expected-volatility band — 1d/1w/1m carry naturally
// different %-move magnitudes, so a longer window uses a wider band or the
// whole treemap saturates to the darkest color.
const bandByTf: Record<SectorFlowTimeframe, number> = { "1d": 3, "1w": 6, "1m": 12 };

function tintFor(changePct: number, tf: SectorFlowTimeframe): string {
  if (Math.abs(changePct) < 0.06) return "rgba(148, 163, 184, 0.16)";
  const ratio = Math.min(1, Math.abs(changePct) / bandByTf[tf]);
  const opacity = 0.14 + ratio * 0.55;
  return changePct > 0 ? `rgba(16, 185, 129, ${opacity.toFixed(2)})` : `rgba(239, 68, 68, ${opacity.toFixed(2)})`;
}

function fmtFlow(v: number, currency: string): string {
  const sign = v >= 0 ? "+" : "-";
  const abs = Math.abs(v);
  // Finnhub marketCapMillion / sectorflow's dollarVolume proxy are both
  // already in millions of the market's own currency (USD or TWD), so a
  // further /100 turns TWD millions into 億 (hundred-million), matching the
  // "+12.3億" TW convention while US stays in the more natural "$1.2B".
  if (currency === "NT$") {
    return `${sign}${currency}${(abs / 100).toFixed(1)}億`;
  }
  if (abs >= 1000) return `${sign}${currency}${(abs / 1000).toFixed(1)}B`;
  return `${sign}${currency}${abs.toFixed(0)}M`;
}

function fmtPct(v: number): string {
  const sign = v > 0 ? "+" : "";
  return `${sign}${v.toFixed(1)}%`;
}

export function SectorFlowView({ dict, market, names = {}, onTickerClick }: Props) {
  const [data, setData] = useState<SectorFlow | null>(null);
  const [error, setError] = useState(false);
  const [tf, setTf] = useState<SectorFlowTimeframe>("1d");
  const [sizeBy, setSizeBy] = useState<SizeBy>("cap");

  useEffect(() => {
    setData(null);
    setError(false);
    fetchSectorFlow(market, tf)
      .then(setData)
      .catch(() => setError(true));
  }, [market, tf]);

  const currency = currencySymbol(market);

  const layout = useMemo(() => {
    if (!data || !data.ready) return [];
    const sectorItems = data.sectors.map((s) => ({
      sector: s,
      value: sizeBy === "cap" ? s.marketCap : Math.max(Math.abs(s.netFlow), s.marketCap * 0.02),
    }));
    return squarify(sectorItems, 0, 0, CANVAS_W, CANVAS_H).map((placed) => {
      const innerH = Math.max(0, placed.rect.h - SECTOR_HEADER_H);
      const itemInputs = placed.sector.items.map((it) => ({
        item: it,
        value: sizeBy === "cap" ? it.cap : Math.max(Math.abs(it.flow), it.cap * 0.02),
      }));
      const items = innerH > 0 && placed.rect.w > 0
        ? squarify(itemInputs, 0, 0, placed.rect.w, innerH)
        : [];
      return { ...placed, items };
    });
  }, [data, sizeBy]);

  if (error) {
    return <div className="error-message">{dict.error}</div>;
  }
  if (!data) {
    return <div className="loading">{dict.loading}</div>;
  }
  if (!data.ready) {
    return <div className="empty-message">{dict.sectorFlowNotReady}</div>;
  }

  const totalNetFlow = data.sectors.reduce((s, sec) => s + sec.netFlow, 0);
  let up = 0, down = 0;
  for (const sec of data.sectors) {
    for (const it of sec.items) {
      if (it.changePct > 0) up++;
      else if (it.changePct < 0) down++;
    }
  }
  const bySectorFlow = [...data.sectors].sort((a, b) => b.netFlow - a.netFlow);
  const strongest = bySectorFlow[0];
  const weakest = bySectorFlow[bySectorFlow.length - 1];
  const maxAbsFlow = Math.max(1, ...data.sectors.map((s) => Math.abs(s.netFlow)));

  return (
    <>
      <div className="flow-controls">
        <div className="topbar-tabs" role="group" aria-label="size by">
          {(["cap", "flow"] as SizeBy[]).map((v) => (
            <button key={v} className={`topbar-tab${sizeBy === v ? " active" : ""}`} onClick={() => setSizeBy(v)}>
              {v === "cap" ? dict.sectorFlowSizeByCap : dict.sectorFlowSizeByFlow}
            </button>
          ))}
        </div>
        <div className="topbar-tabs" role="group" aria-label="timeframe">
          {(["1d", "1w", "1m"] as SectorFlowTimeframe[]).map((v) => (
            <button key={v} className={`topbar-tab${tf === v ? " active" : ""}`} onClick={() => setTf(v)}>
              {v.toUpperCase()}
            </button>
          ))}
        </div>
        {market === "tw" && sizeBy === "cap" && <InfoTip text={dict.sectorFlowTWCapNote} />}
      </div>

      <div className="kpi-grid flow-stats-grid">
        <div className="card">
          <div className="eyebrow">{dict.sectorFlowTotalNetFlow}</div>
          <div className={`kpi-value ${totalNetFlow >= 0 ? "profit" : "loss"}`}>{fmtFlow(totalNetFlow, currency)}</div>
        </div>
        <div className="card">
          <div className="eyebrow">{dict.sectorFlowBreadth}</div>
          <div className="kpi-value">
            <span className="profit">{up}</span> / <span className="loss">{down}</span>
          </div>
        </div>
        <div className="card">
          <div className="eyebrow">{dict.sectorFlowStrongest}</div>
          <div className="kpi-value">{strongest?.name ?? "—"}</div>
        </div>
        <div className="card">
          <div className="eyebrow">{dict.sectorFlowWeakest}</div>
          <div className="kpi-value">{weakest?.name ?? "—"}</div>
        </div>
      </div>

      <div className="card flow-treemap-card">
        <div className="flow-treemap">
          {layout.map(({ sector, rect, items }) => (
            <div
              key={sector.name}
              className="flow-sector"
              style={{
                left: `${(rect.x / CANVAS_W) * 100}%`,
                top: `${(rect.y / CANVAS_H) * 100}%`,
                width: `${(rect.w / CANVAS_W) * 100}%`,
                height: `${(rect.h / CANVAS_H) * 100}%`,
              }}
            >
              <div className="flow-sector-header mono">
                <span>{sector.name}</span>
                <span className={sector.netFlow >= 0 ? "profit" : "loss"}>{fmtFlow(sector.netFlow, currency)}</span>
              </div>
              <div className="flow-sector-body">
                {items.map(({ item, rect: r }) => (
                  <div
                    key={item.ticker}
                    className={`flow-block${item.held ? " held" : ""}`}
                    style={{
                      left: `${(r.x / rect.w) * 100}%`,
                      top: `${(r.y / Math.max(1, rect.h - SECTOR_HEADER_H)) * 100}%`,
                      width: `${(r.w / rect.w) * 100}%`,
                      height: `${(r.h / Math.max(1, rect.h - SECTOR_HEADER_H)) * 100}%`,
                      background: tintFor(item.changePct, tf),
                    }}
                    title={`${tickerLabel(item.ticker, names)} ${fmtPct(item.changePct)} ${fmtFlow(item.flow, currency)}`}
                    onClick={() => onTickerClick?.(item.ticker)}
                  >
                    {item.held && r.w > 52 && r.h > 34 && <span className="flow-block-held-dot" />}
                    <span className="flow-block-ticker mono">{item.ticker}</span>
                    {r.w > 52 && r.h > 34 && (
                      <span className={`flow-block-change mono ${item.changePct >= 0 ? "profit" : "loss"}`}>
                        {fmtPct(item.changePct)}
                      </span>
                    )}
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>
      </div>

      <div className="card">
        <div className="eyebrow" style={{ marginBottom: 12 }}>
          {dict.sectorFlowRanking}
        </div>
        <table className="mono">
          <thead>
            <tr>
              <th>{dict.group}</th>
              <th>{dict.sectorFlowChange}</th>
              <th>{dict.sectorFlowNetFlow}</th>
              <th>{dict.sectorFlowTickerCount}</th>
              <th>{dict.sectorFlowHeld}</th>
            </tr>
          </thead>
          <tbody>
            {bySectorFlow.map((s) => (
              <tr key={s.name}>
                <td>{s.name}</td>
                <td className={s.changePct >= 0 ? "profit" : "loss"}>{fmtPct(s.changePct)}</td>
                <td className={s.netFlow >= 0 ? "profit" : "loss"}>
                  <div className="flow-rank-bar-wrap">
                    <span>{fmtFlow(s.netFlow, currency)}</span>
                    <div
                      className={`flow-rank-bar ${s.netFlow >= 0 ? "profit-bar" : "loss-bar"}`}
                      style={{ width: `${(Math.abs(s.netFlow) / maxAbsFlow) * 100}%` }}
                    />
                  </div>
                </td>
                <td>{s.tickerCount}</td>
                <td>{s.heldCount}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  );
}
