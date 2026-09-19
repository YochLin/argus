import { useEffect, useState } from "react";
import { fetchWealthFunds, type FundRow, type WealthFunds } from "../api";
import type { Dictionary } from "../i18n";
import { fmtMoney } from "./WealthHomeView";

interface Props {
  dict: Dictionary;
}

const CURRENCY = "NT$"; // display currency fixed to TWD, same known gap as every other wealth page

function fmtSigned(v: number, currency: string): string {
  return `${v >= 0 ? "+" : "−"}${fmtMoney(Math.abs(v), currency)}`;
}

function pnlColor(v: number): string {
  return v >= 0 ? "var(--profit)" : "var(--loss)";
}

function fmtPct(v: number): string {
  return `${v >= 0 ? "+" : ""}${v.toFixed(1)}%`;
}

// FundsChart mirrors fundsModel()'s dual-line SVG (cost dashed, market value
// solid + area fill) — same hand-rolled-polyline pattern WealthRetireView's
// RetireChart already uses, no chart library. A point with a null cost/value
// (no fund had a priceable snapshot that far back yet) is simply skipped
// from that line rather than drawn as zero.
function FundsChart({ dict, points }: { dict: Dictionary; points: WealthFunds["chart"] }) {
  const known = points.filter((p) => p.value != null || p.cost != null);
  if (known.length < 2) {
    return <div className="empty-message">—</div>;
  }
  const width = 600;
  const height = 170;
  const allValues = known.flatMap((p) => [p.value, p.cost]).filter((v): v is number => v != null);
  const maxV = Math.max(...allValues) * 1.05;
  const minV = Math.min(0, Math.min(...allValues) * 0.95);
  const span = maxV - minV || 1;
  const x = (i: number) => (i / (points.length - 1)) * width;
  const y = (v: number) => height - ((v - minV) / span) * height;

  function lineFor(key: "value" | "cost"): string {
    let d = "";
    let started = false;
    points.forEach((p, i) => {
      const v = p[key];
      if (v == null) return;
      d += `${started ? "L" : "M"}${x(i).toFixed(1)},${y(v).toFixed(1)} `;
      started = true;
    });
    return d.trim();
  }

  function areaFor(): string {
    const idx = points.map((p, i) => (p.value != null ? i : -1)).filter((i) => i >= 0);
    if (idx.length < 2) return "";
    const first = idx[0];
    const last = idx[idx.length - 1];
    const mid = idx.map((i) => `L${x(i).toFixed(1)},${y(points[i].value as number).toFixed(1)}`).join(" ");
    return `M${x(first).toFixed(1)},${height} ${mid} L${x(last).toFixed(1)},${height} Z`;
  }

  const mvPath = lineFor("value");
  const costPath = lineFor("cost");
  const areaPath = areaFor();

  return (
    <>
      <div style={{ display: "flex", alignItems: "baseline", gap: 14, marginBottom: 14, flexWrap: "wrap" }}>
        <span className="eyebrow">{dict.wealthFundsChartTitle}</span>
        <span style={{ marginLeft: "auto", display: "flex", gap: 14 }}>
          <span style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 11, color: "var(--ink-3)" }}>
            <span style={{ width: 14, height: 2, background: "var(--ink-3)" }} />
            {dict.wealthFundsCostLeg}
          </span>
          <span style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 11, color: "var(--ink-3)" }}>
            <span style={{ width: 14, height: 2, background: "var(--accent)" }} />
            {dict.wealthFundsMvLeg}
          </span>
        </span>
      </div>
      <svg viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="none" style={{ width: "100%", height: 180, display: "block" }}>
        {areaPath && <path d={areaPath} fill="var(--tint)" stroke="none" />}
        {costPath && <path d={costPath} fill="none" stroke="var(--ink-3)" strokeWidth={1.5} strokeDasharray="4 3" vectorEffect="non-scaling-stroke" />}
        {mvPath && <path d={mvPath} fill="none" stroke="var(--accent)" strokeWidth={2} vectorEffect="non-scaling-stroke" />}
      </svg>
      <div style={{ display: "flex", justifyContent: "space-between", marginTop: 8 }}>
        <span className="mono" style={{ fontSize: 10, color: "var(--ink-3)" }}>{dict.wealthFundsChartAxis24}</span>
        <span className="mono" style={{ fontSize: 10, color: "var(--ink-3)" }}>{dict.wealthFundsChartAxis12}</span>
        <span className="mono" style={{ fontSize: 10, color: "var(--ink-3)" }}>{dict.wealthFundsChartAxisNow}</span>
      </div>
    </>
  );
}

// FundTableRow mirrors fundsModel()'s row shape (monthlyStyle grays out a
// stopped/lump-sum holding rather than showing "NT$0"). Every TWD figure
// besides Monthly (a raw fund_details value, no FX involved) degrades
// independently per §8.17.1 — a single unpriceable holding shows "—" in its
// own cells without blanking the rest of the row.
function FundTableRow({ dict, row }: { dict: Dictionary; row: FundRow }) {
  const stopped = row.monthlyAmount == null || row.monthlyAmount <= 0;
  return (
    <tr>
      <td style={{ fontFamily: "var(--sans)" }}>{dict.wealthFundsClassName}</td>
      <td style={{ textAlign: "left", color: "var(--ink-3)" }}>{row.code || "—"}</td>
      <td style={{ fontFamily: "var(--sans)", fontSize: 12, textAlign: "left", color: "var(--ink-3)" }}>{row.platform || "—"}</td>
      <td style={stopped ? { color: "var(--ink-3)" } : undefined}>
        {stopped ? dict.wealthFundsStopped : fmtMoney(row.monthlyAmount as number, CURRENCY)}
      </td>
      <td>{row.cost != null ? fmtMoney(row.cost, CURRENCY) : "—"}</td>
      <td>{row.value != null ? fmtMoney(row.value, CURRENCY) : "—"}</td>
      <td style={row.pnl != null ? { color: pnlColor(row.pnl) } : undefined}>
        {row.pnl != null ? fmtSigned(row.pnl, CURRENCY) : "—"}
      </td>
      <td style={row.returnPct != null ? { color: pnlColor(row.returnPct) } : undefined}>
        {row.returnPct != null ? fmtPct(row.returnPct) : "—"}
      </td>
      <td style={{ color: "var(--ink-3)" }}>{row.oneYearReturnPct != null ? fmtPct(row.oneYearReturnPct) : "—"}</td>
      <td style={{ color: "var(--ink-3)" }}>{row.nextContributionDate || "—"}</td>
    </tr>
  );
}

// WealthFundsView backs /w/funds (Phase 9 波次4 PR10). Unlike every other
// wealth page besides /w/goals, this page has no add form (§9.4 PR10: fund
// rows only ever arrive via /w/import's CSV path, so there's no writable
// prop here) — hence no writable/onUnauthorized props either.
export function WealthFundsView({ dict }: Props) {
  const [data, setData] = useState<WealthFunds | null>(null);
  const [error, setError] = useState(false);

  useEffect(() => {
    setError(false);
    fetchWealthFunds()
      .then(setData)
      .catch(() => setError(true));
  }, []);

  if (error) {
    return <div className="error-message">{dict.error}</div>;
  }

  return (
    <>
      <div style={{ display: "flex", alignItems: "center", gap: 12, margin: "16px 0", flexWrap: "wrap" }}>
        <span style={{ fontFamily: "var(--font-mono)", textTransform: "uppercase", letterSpacing: ".08em", fontSize: 11, color: "var(--ink)" }}>
          {dict.navWealthFunds}
        </span>
      </div>

      <div style={{ display: "flex", flexWrap: "wrap", gap: 16, marginBottom: 16 }}>
        <div className="card card--glow" style={{ flex: "1.5 1 250px" }}>
          <div className="eyebrow">{dict.wealthFundsMvLabel}</div>
          <div className="mono" style={{ fontSize: 36, lineHeight: 1.1, marginTop: 8 }}>
            {data?.marketValue != null ? fmtMoney(data.marketValue, CURRENCY) : "—"}
          </div>
          <div className="mono" style={{ fontSize: 12, marginTop: 8, color: data?.pnl != null ? pnlColor(data.pnl) : "var(--ink-3)" }}>
            {data?.pnl != null && data.returnPct != null ? `${fmtSigned(data.pnl, CURRENCY)} · ${fmtPct(data.returnPct)}` : "—"}
          </div>
        </div>
        <div className="card" style={{ flex: "1 1 200px" }}>
          <div className="eyebrow">{dict.wealthFundsCostLabel}</div>
          <div className="mono" style={{ fontSize: "clamp(17px,2.1vw,28px)", marginTop: 8 }}>
            {data?.cost != null ? fmtMoney(data.cost, CURRENCY) : "—"}
          </div>
        </div>
        <div className="card" style={{ flex: "1 1 200px" }}>
          <div className="eyebrow">{dict.wealthFundsPnlLabel}</div>
          <div className="mono" style={{ fontSize: "clamp(17px,2.1vw,28px)", marginTop: 8, color: data?.pnl != null ? pnlColor(data.pnl) : undefined }}>
            {data?.pnl != null ? fmtSigned(data.pnl, CURRENCY) : "—"}
          </div>
        </div>
        <div className="card" style={{ flex: "1 1 200px" }}>
          <div className="eyebrow">{dict.wealthFundsMonthlyLabel}</div>
          <div className="mono" style={{ fontSize: "clamp(17px,2.1vw,28px)", marginTop: 8 }}>
            {data ? fmtMoney(data.monthlyTotal, CURRENCY) : "—"}
          </div>
        </div>
      </div>

      <div style={{ display: "flex", flexWrap: "wrap", gap: 16, marginBottom: 16, alignItems: "flex-start" }}>
        <div className="card" style={{ flex: "2 1 480px" }}>
          {data ? <FundsChart dict={dict} points={data.chart} /> : <div className="empty-message">—</div>}
        </div>
        <div className="card" style={{ flex: "1 1 280px" }}>
          <div className="eyebrow" style={{ marginBottom: 12 }}>
            {dict.wealthFundsScheduleTitle}
          </div>
          {data && data.schedule.length > 0 ? (
            <div style={{ display: "flex", flexDirection: "column", gap: 11 }}>
              {data.schedule.map((s, i) => (
                <div
                  key={i}
                  style={{ display: "flex", alignItems: "baseline", gap: 10, paddingBottom: 9, borderBottom: "1px solid var(--border)" }}
                >
                  <span className="mono" style={{ fontSize: 12, color: "var(--ink-3)" }}>{s.date}</span>
                  <span className="mono" style={{ fontSize: 12 }}>{s.names}</span>
                  <span className="mono" style={{ marginLeft: "auto", fontSize: 12.5, color: "var(--loss)" }}>
                    {fmtMoney(s.amount, CURRENCY)}
                  </span>
                </div>
              ))}
              <div style={{ display: "flex", alignItems: "baseline", gap: 10 }}>
                <span style={{ fontFamily: "var(--font-mono)", fontSize: 10.5, letterSpacing: ".06em", color: "var(--ink-3)" }}>
                  {dict.wealthFundsMonthlyLabel}
                </span>
                <span className="mono" style={{ marginLeft: "auto", fontSize: 15 }}>
                  {data ? fmtMoney(data.monthlyTotal, CURRENCY) : "—"}
                </span>
              </div>
            </div>
          ) : (
            <div className="empty-message">{dict.wealthFundsScheduleEmpty}</div>
          )}
        </div>
      </div>

      <div className="card" style={{ overflowX: "auto" }}>
        <div className="eyebrow" style={{ marginBottom: 4 }}>
          {dict.wealthFundsTableTitle}
        </div>
        {data && data.rows.length > 0 ? (
          <table className="mono">
            <thead>
              <tr>
                <th>{dict.wealthFundsColClass}</th>
                <th style={{ textAlign: "left" }}>{dict.wealthFundsColCode}</th>
                <th style={{ textAlign: "left" }}>{dict.wealthFundsColPlatform}</th>
                <th>{dict.wealthFundsColMonthly}</th>
                <th>{dict.wealthFundsColCost}</th>
                <th>{dict.wealthFundsColMv}</th>
                <th>{dict.wealthFundsColPnl}</th>
                <th>{dict.wealthFundsColReturn}</th>
                <th>{dict.wealthFundsColOneYear}</th>
                <th>{dict.wealthFundsColNext}</th>
              </tr>
            </thead>
            <tbody>
              {data.rows.map((r) => (
                <FundTableRow key={r.assetId} dict={dict} row={r} />
              ))}
            </tbody>
          </table>
        ) : (
          <div className="empty-message">{dict.wealthFundsNoFunds}</div>
        )}
      </div>
    </>
  );
}
