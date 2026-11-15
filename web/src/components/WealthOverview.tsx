import { useEffect, useState, type ReactNode } from "react";
import { fetchWealthTrend, type BalanceSheet, type TrendPeriod, type WealthHome, type WealthTrendPoint } from "../api";
import { shortTWD } from "../currency";
import type { Dictionary } from "../i18n";
import { OVERVIEW_COLOR, overviewAssetSlices, overviewLiabSlices, type OverviewCategory, type OverviewLiab } from "../wealthCategory";

// 淨值總覽 (/w) — the 2026-11 redesign: three headline cards, the net-worth
// trend and the asset/liability donut. The chart is plain SVG rather than
// lightweight-charts: the design has no axes, spaces points by index, and has
// to follow the light/dark toggle through CSS variables, all of which that
// library fights.

const PERIODS: TrendPeriod[] = ["mtd", "3m", "ytd", "1y"];

function periodLabel(dict: Dictionary, p: TrendPeriod): string {
  return { mtd: dict.wealthOvPeriodMtd, "3m": dict.wealthOvPeriod3m, ytd: dict.wealthOvPeriodYtd, "1y": dict.wealthOvPeriod1y }[p];
}

export interface TrendSeriesPoint {
  date: string;
  value: number;
}

// trendSeries keeps the points that have a figure. A new account's empty
// leading days drop out so the line starts where the records do; a gap in the
// middle is bridged, not drawn as a drop.
export function trendSeries(points: WealthTrendPoint[]): TrendSeriesPoint[] {
  const out: TrendSeriesPoint[] = [];
  for (const p of points) if (p.netWorth != null) out.push({ date: p.date, value: p.netWorth });
  return out;
}

export const TREND_W = 600;
export const TREND_H = 280;
const TREND_PAD = 16;

// trendGeometry maps values (at least two) onto the 600x280 chart: x spaced by
// index, y padded 8% so the line never touches the edge.
export function trendGeometry(values: number[]) {
  let lo = Math.min(...values);
  let hi = Math.max(...values);
  const span = hi - lo || 1;
  lo -= span * 0.08;
  hi += span * 0.08;
  const x = (i: number) => (i / (values.length - 1)) * TREND_W;
  const y = (v: number) => TREND_PAD + ((hi - v) / (hi - lo)) * (TREND_H - TREND_PAD * 2);
  const line = "M" + values.map((v, i) => `${x(i).toFixed(1)},${y(v).toFixed(1)}`).join(" L");
  return { x, y, lo, hi, line, area: `${line} L${TREND_W},${TREND_H} L0,${TREND_H} Z` };
}

export const DONUT_R = 76;

// donutArcs turns values into stroke-dasharray/offset pairs around a circle of
// radius R, leaving a 2px gap between slices.
export function donutArcs(values: number[], r = DONUT_R, gap = 2): { dash: string; off: string }[] {
  const c = 2 * Math.PI * r;
  const total = values.reduce((a, b) => a + b, 0);
  let acc = 0;
  return values.map((v) => {
    const len = total > 0 ? (v / total) * c : 0;
    const seg = Math.max(0.5, len - (values.length > 1 ? gap : 0));
    const arc = { dash: `${seg.toFixed(2)} ${(c - seg).toFixed(2)}`, off: (-acc).toFixed(2) };
    acc += len;
    return arc;
  });
}

function fmtDate(iso: string, period: TrendPeriod): string {
  return period === "1y" ? iso.replace(/-/g, "/") : iso.slice(5).replace("-", "/");
}

const LIAB_VAR = ["var(--ov-liab-0)", "var(--ov-liab-1)", "var(--ov-liab-2)"];

interface Slice {
  key: string;
  name: string;
  color: string;
  value: number;
}

interface Props {
  dict: Dictionary;
  home: WealthHome | null;
  balance: BalanceSheet | null;
  // refresh changes whenever the page's records do, to re-pull the trend.
  refresh: number;
  // money formats a TWD amount in the display currency (WealthHomeView's fmtMoney).
  money: (v: number) => string;
  // actions sits beside the period switch (the page's add button).
  actions?: ReactNode;
}

export function WealthOverview({ dict, home, balance, refresh, money, actions }: Props) {
  const [period, setPeriod] = useState<TrendPeriod>("ytd");
  const [points, setPoints] = useState<WealthTrendPoint[] | null>(null);
  const [hover, setHover] = useState<number | null>(null);
  const [mode, setMode] = useState<"asset" | "liab">("asset");
  const [active, setActive] = useState<number | null>(null);

  useEffect(() => {
    let stale = false;
    fetchWealthTrend(period)
      .then((r) => !stale && setPoints(r.points))
      .catch(() => !stale && setPoints([]));
    return () => {
      stale = true;
    };
  }, [period, refresh]);

  const series = trendSeries(points ?? []);
  const drawable = series.length >= 2;
  const first = series[0];
  const last = series[series.length - 1];
  const delta = drawable ? last.value - first.value : null;
  const deltaPct = drawable && first.value !== 0 ? (delta! / Math.abs(first.value)) * 100 : null;
  const up = delta == null || delta >= 0;

  const catName: Record<OverviewCategory, string> = {
    estate: dict.wealthOvCatEstate,
    equity: dict.wealthOvCatEquity,
    cash: dict.wealthOvCatCash,
    fund: dict.wealthOvCatFund,
    insurance: dict.wealthOvCatInsurance,
    bond: dict.wealthOvCatBond,
    other: dict.wealthOvCatOther,
  };
  const assetSlices: Slice[] = balance
    ? overviewAssetSlices(balance.assetGroups).map((s) => ({
        key: s.key,
        name: catName[s.key],
        color: OVERVIEW_COLOR[s.key],
        value: s.value,
      }))
    : [];
  const liabName: Record<OverviewLiab, string> = { mortgage: dict.wealthOvLiabMortgage, loan: dict.wealthOvLiabLoan, card: dict.wealthOvLiabCard };
  const liabSlices: Slice[] = balance
    ? overviewLiabSlices(balance).map((s, i) => ({ key: s.key, name: liabName[s.key], color: LIAB_VAR[i % LIAB_VAR.length], value: s.value }))
    : [];

  const isLiab = mode === "liab";
  const slices = isLiab ? liabSlices : assetSlices;
  const sliceTotal = slices.reduce((a, s) => a + s.value, 0);
  const arcs = donutArcs(slices.map((s) => s.value));
  const act = active != null && slices[active] ? slices[active] : null;
  const pct = (v: number) => (sliceTotal > 0 ? `${((v / sliceTotal) * 100).toFixed(1)}%` : "—");
  const modeName = isLiab ? dict.wealthOvModeLiabs : dict.wealthOvModeAssets;

  // trend chart
  const geo = drawable ? trendGeometry(series.map((s) => s.value)) : null;
  const idx = hover != null && hover < series.length ? hover : series.length - 1;
  const cur = drawable ? series[idx] : null;
  const picks = drawable ? [...new Set([0, Math.round((series.length - 1) / 3), Math.round(((series.length - 1) * 2) / 3), series.length - 1])] : [];
  const step = drawable ? TREND_W / (series.length - 1) : 0;

  const assetTotal = assetSlices.reduce((a, s) => a + s.value, 0) || 1;

  return (
    <div className="ov">
      <div className="ov-head">
        <h1 className="ov-title">{dict.wealthOvTitle}</h1>
        <div className="ov-head-right">
          <div className="ov-seg ov-seg--lg" role="group" aria-label={dict.wealthOvPeriodAria}>
            {PERIODS.map((p) => (
              <button
                key={p}
                type="button"
                aria-pressed={p === period}
                onClick={() => {
                  setPeriod(p);
                  setHover(null);
                }}
              >
                {periodLabel(dict, p)}
              </button>
            ))}
          </div>
          {actions}
        </div>
      </div>

      <div className="ov-cards">
        <section className="card ov-card ov-card--hero" aria-label={dict.wealthOvNetWorth}>
          <span className="ov-label">{dict.wealthOvNetWorth}</span>
          <span className="ov-big">{home?.netWorth != null ? money(home.netWorth) : "—"}</span>
          <span className={`ov-pill${up ? "" : " ov-pill--down"}`}>
            {delta != null && (
              <svg width="11" height="11" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                <path d={up ? "M12 5 20 18H4Z" : "M12 19 4 6h16Z"} />
              </svg>
            )}
            {delta != null
              ? `${delta >= 0 ? "+" : ""}${money(delta)}${deltaPct != null ? `（${deltaPct >= 0 ? "+" : ""}${deltaPct.toFixed(1)}%）` : ""}${periodLabel(dict, period)}`
              : "—"}
          </span>
        </section>

        <section className="card ov-card" aria-label={dict.wealthTotalAssets}>
          <span className="ov-label">{dict.wealthTotalAssets}</span>
          <span className="ov-mid">{home?.totalAssets != null ? money(home.totalAssets) : "—"}</span>
          <div className="ov-stack" aria-hidden="true">
            {assetSlices.map((s) => (
              <div key={s.key} style={{ width: `${(s.value / assetTotal) * 100}%`, background: s.color }} />
            ))}
          </div>
        </section>

        <section className="card ov-card" aria-label={dict.wealthTotalLiabilities}>
          <span className="ov-label">{dict.wealthTotalLiabilities}</span>
          <span className="ov-mid">{home?.totalLiabilities != null ? money(home.totalLiabilities) : "—"}</span>
          <span className="ov-sub">
            {dict.wealthDebtRatio} <b>{balance?.debtRatioPct != null ? `${balance.debtRatioPct.toFixed(1)}%` : "—"}</b>
          </span>
        </section>
      </div>

      <div className="ov-main">
        <section className="card ov-card ov-trend" aria-labelledby="ov-trend-h">
          <div className="ov-row">
            <h2 id="ov-trend-h">{dict.wealthOvTrend}</h2>
            <span className="ov-readout" aria-live="polite">
              {cur ? `${fmtDate(cur.date, period)}　${money(cur.value)}` : ""}
            </span>
          </div>
          {geo && cur ? (
            <>
              <div className="ov-chart" onMouseLeave={() => setHover(null)}>
                <svg
                  viewBox={`0 0 ${TREND_W} ${TREND_H}`}
                  preserveAspectRatio="none"
                  role="img"
                  aria-label={dict.wealthOvChartAria.replace("%s", periodLabel(dict, period)).replace("%s", money(first.value)).replace("%s", money(last.value))}
                >
                  {[16, 140, 264].map((y) => (
                    <line key={y} x1="0" y1={y} x2={TREND_W} y2={y} className="ov-grid" vectorEffect="non-scaling-stroke" />
                  ))}
                  <path d={geo.area} className="ov-area" />
                  <path d={geo.line} className="ov-line" vectorEffect="non-scaling-stroke" />
                  {series.map((s, i) => (
                    <rect
                      key={s.date}
                      x={Math.max(0, geo.x(i) - step / 2)}
                      y="0"
                      width={step}
                      height={TREND_H}
                      fill="transparent"
                      onMouseEnter={() => setHover(i)}
                      onClick={() => setHover(i)}
                    />
                  ))}
                </svg>
                <div className="ov-cursor" aria-hidden="true" style={{ left: `${((geo.x(idx) / TREND_W) * 100).toFixed(2)}%` }} />
                <div className="ov-dot" aria-hidden="true" style={{ left: `${((geo.x(idx) / TREND_W) * 100).toFixed(2)}%`, top: geo.y(cur.value) }} />
                <span className="ov-ymax" aria-hidden="true">{shortTWD(geo.hi)}</span>
                <span className="ov-ymin" aria-hidden="true">{shortTWD(geo.lo)}</span>
              </div>
              <div className="ov-xlabels" aria-hidden="true">
                {picks.map((i) => (
                  <span key={i}>{fmtDate(series[i].date, period)}</span>
                ))}
              </div>
            </>
          ) : (
            <div className="ov-chart-empty">{points == null ? dict.loading : dict.wealthOvTrendEmpty}</div>
          )}
        </section>

        <section className="card ov-card ov-alloc" aria-labelledby="ov-alloc-h">
          <div className="ov-row">
            <h2 id="ov-alloc-h">{dict.wealthOvAlloc}</h2>
            <div className="ov-seg" role="group" aria-label={dict.wealthOvModeAria}>
              {(["asset", "liab"] as const).map((m) => (
                <button
                  key={m}
                  type="button"
                  aria-pressed={m === mode}
                  onClick={() => {
                    setMode(m);
                    setActive(null);
                  }}
                >
                  {m === "asset" ? dict.wealthOvModeAssets : dict.wealthOvModeLiabs}
                </button>
              ))}
            </div>
          </div>
          <div className="ov-donut">
            <svg
              width="200"
              height="200"
              viewBox="0 0 200 200"
              role="img"
              aria-label={dict.wealthOvDonutAria.replace("%s", modeName).replace("%s", slices.map((s) => `${s.name} ${pct(s.value)}`).join("、"))}
            >
              <circle cx="100" cy="100" r={DONUT_R} fill="none" className="ov-track" strokeWidth="26" />
              <g transform="rotate(-90 100 100)">
                {slices.map((s, i) => (
                  <circle
                    key={s.key}
                    cx="100"
                    cy="100"
                    r={DONUT_R}
                    fill="none"
                    stroke={s.color}
                    strokeWidth={active === i ? 32 : 26}
                    strokeDasharray={arcs[i].dash}
                    strokeDashoffset={arcs[i].off}
                    opacity={active == null || active === i ? 1 : 0.35}
                  />
                ))}
              </g>
            </svg>
            <div className="ov-donut-center">
              <span className="ov-donut-label">{act ? act.name : isLiab ? dict.wealthTotalLiabilities : dict.wealthTotalAssets}</span>
              <span className="ov-donut-value">{shortTWD(act ? act.value : sliceTotal)}</span>
              <span className="ov-donut-sub">{act ? pct(act.value) : dict.wealthOvKinds.replace("%s", String(slices.length))}</span>
            </div>
          </div>
          <ul className="ov-legend" onMouseLeave={() => setActive(null)}>
            {slices.map((s, i) => (
              <li key={s.key}>
                <button
                  type="button"
                  className={active === i ? "on" : ""}
                  onMouseEnter={() => setActive(i)}
                  onFocus={() => setActive(i)}
                  onClick={() => setActive(i)}
                >
                  <span className="ov-swatch" style={{ background: s.color }} aria-hidden="true" />
                  <span className="ov-legend-name">{s.name}</span>
                  <span className="ov-legend-amt">{shortTWD(s.value)}</span>
                  <span className="ov-legend-pct">{pct(s.value)}</span>
                </button>
              </li>
            ))}
          </ul>
        </section>
      </div>
    </div>
  );
}
