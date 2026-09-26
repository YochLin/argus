import { useEffect, useState } from "react";
import { fetchWealthAlloc, type AllocationModel, type WealthAlloc, type WealthAllocRow } from "../api";
import type { Dictionary } from "../i18n";
import { fmtMoney, DOTTED, MONO_LABEL } from "./WealthHomeView";
import { CATEGORY_COLOR, categoryLabel, loadModel, saveModel } from "../wealthCategory";
import { shortTWD } from "../currency";

interface Props {
  dict: Dictionary;
}

const MODELS: AllocationModel[] = ["conserv", "balanced", "growth"];
const currency = "NT$"; // display currency fixed to TWD for now, same as WealthHomeView/WealthBalanceView

function modelLabel(dict: Dictionary, m: AllocationModel): string {
  return m === "conserv" ? dict.wealthModelConserv : m === "growth" ? dict.wealthModelGrowth : dict.wealthModelBalanced;
}

// The design's drift rule: ink-3 inside ±1.5pt, red (over-weight) / green
// (under-weight) beyond. Same colours whether shown in the table or an order.
function driftColor(d: number): string {
  return Math.abs(d) < 1.5 ? "var(--ink-3)" : d > 0 ? "var(--loss)" : "var(--profit)";
}

function fmtDrift(d: number): string {
  return `${d >= 0 ? "+" : "−"}${Math.abs(d).toFixed(1)}pt`;
}

const dot = (color: string): React.CSSProperties => ({ width: 8, height: 8, borderRadius: 2, flexShrink: 0, background: color });

// donutGradient builds the design template's `wam.donut` ring — a CSS-only
// pie chart via conic-gradient, one slice per non-zero currentPct in the
// category's own colour. A card-coloured "hole" is overlaid (see JSX) to turn
// the pie into a donut.
function donutGradient(rows: WealthAllocRow[]): string {
  let acc = 0;
  const stops: string[] = [];
  for (const row of rows) {
    if (row.currentPct <= 0) continue;
    const from = acc;
    acc = Math.min(100, acc + row.currentPct);
    stops.push(`${CATEGORY_COLOR[row.category]} ${from.toFixed(2)}% ${acc.toFixed(2)}%`);
  }
  if (stops.length === 0) return "var(--border)";
  return `conic-gradient(${stops.join(", ")})`;
}

export function WealthAllocView({ dict }: Props) {
  const [model, setModel] = useState<AllocationModel>(loadModel);
  const [alloc, setAlloc] = useState<WealthAlloc | null>(null);
  const [error, setError] = useState(false);

  useEffect(() => {
    setError(false);
    fetchWealthAlloc(model)
      .then(setAlloc)
      .catch(() => setError(true));
  }, [model]);

  if (error) {
    return <div className="error-message">{dict.error}</div>;
  }

  const hasData = alloc != null && alloc.totalAssets != null;
  const rows = hasData ? alloc!.allocation : [];
  const orders = hasData ? alloc!.orders : [];
  const riskPct = hasData ? alloc!.riskPct : null;
  // The design's band words: >40% aggressive, >25% balanced-growth, else conservative.
  const riskBand =
    riskPct == null ? "—" : riskPct > 40 ? dict.wealthRiskAggressive : riskPct > 25 ? dict.wealthRiskBalanced : dict.wealthRiskConservative;

  return (
    <>
      <div style={{ display: "flex", alignItems: "center", gap: 12, margin: "16px 0", flexWrap: "wrap" }}>
        <span style={{ ...MONO_LABEL, color: "var(--ink)" }}>{dict.navWealthAlloc}</span>
        <span style={{ marginLeft: "auto", fontFamily: "var(--font-mono)", fontSize: 10, letterSpacing: ".06em", color: "var(--ink-3)" }}>
          {dict.wealthTargetModelLabel}
        </span>
        <div style={{ display: "flex", gap: 6 }}>
          {MODELS.map((m) => (
            <button
              key={m}
              className={`home-variant-btn${model === m ? " active" : ""}`}
              onClick={() => {
                setModel(m);
                saveModel(m);
              }}
            >
              {modelLabel(dict, m)}
            </button>
          ))}
        </div>
      </div>

      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(240px, 1fr))", gap: 16, marginBottom: 16 }}>
        <div className="card" style={{ display: "flex", flexDirection: "column" }}>
          <div style={{ ...MONO_LABEL, marginBottom: 14 }}>{dict.wealthMixTitle}</div>
          {rows.length > 0 ? (
            <div style={{ display: "flex", alignItems: "center", gap: 19.2, flexWrap: "wrap", flex: 1 }}>
              <div style={{ position: "relative", display: "flex", alignItems: "center", justifyContent: "center" }}>
                <div style={{ width: 172, height: 172, borderRadius: "50%", flexShrink: 0, background: donutGradient(rows) }} />
                <div
                  style={{
                    position: "absolute",
                    width: 104,
                    height: 104,
                    borderRadius: "50%",
                    background: "var(--surface)",
                    display: "flex",
                    flexDirection: "column",
                    alignItems: "center",
                    justifyContent: "center",
                    gap: 2,
                  }}
                >
                  <span className="mono" style={{ fontSize: 9.5, letterSpacing: "0.06em", color: "var(--ink-3)" }}>
                    {dict.wealthTotalAssets}
                  </span>
                  <span className="mono" style={{ fontSize: 14 }}>
                    {fmtMoney(alloc!.totalAssets ?? 0, currency)}
                  </span>
                </div>
              </div>
              <div style={{ display: "flex", flexDirection: "column", justifyContent: "space-between", gap: 7, flex: 1, minWidth: 180, alignSelf: "stretch" }}>
                {rows.map((row) => (
                  <div key={row.category} style={{ display: "flex", alignItems: "center", gap: 8 }}>
                    <span style={dot(CATEGORY_COLOR[row.category])} />
                    <span style={{ fontSize: 12, color: "var(--ink-2)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                      {categoryLabel(dict, row.category)}
                    </span>
                    <span className="mono" style={{ marginLeft: "auto", fontSize: 12 }}>
                      {row.currentPct.toFixed(1)}%
                    </span>
                    <span className="mono" style={{ fontSize: 10.5, color: "var(--ink-3)", width: 34, textAlign: "right" }}>
                      {Math.round(row.targetPct)}%
                    </span>
                  </div>
                ))}
              </div>
            </div>
          ) : (
            <div className="empty-message">{dict.noData}</div>
          )}
        </div>

        <div className="card card--glow" style={{ display: "flex", flexDirection: "column" }}>
          <div style={{ display: "flex", alignItems: "baseline", gap: 10, marginBottom: 12 }}>
            <span style={MONO_LABEL}>{dict.wealthOrdersTitle}</span>
            <span className="mono" style={{ fontSize: 11, color: "var(--ink-3)" }}>
              {orders.length} · {hasData ? shortTWD(alloc!.rebalTotal) : "—"}
            </span>
          </div>
          {orders.length > 0 ? (
            <div style={{ display: "flex", flexDirection: "column", gap: 9 }}>
              {orders.map((o) => (
                <div key={o.category} style={{ display: "flex", alignItems: "center", gap: 9, paddingBottom: 9, borderBottom: "1px solid var(--border)" }}>
                  <span style={dot(CATEGORY_COLOR[o.category])} />
                  <span
                    className="mono"
                    style={{
                      fontSize: 10,
                      letterSpacing: "0.06em",
                      padding: "2px 7px",
                      borderRadius: 4,
                      whiteSpace: "nowrap",
                      background: o.side === "buy" ? "rgba(52,211,153,.14)" : "rgba(248,113,113,.14)",
                      color: o.side === "buy" ? "var(--profit)" : "var(--loss)",
                    }}
                  >
                    {o.side === "buy" ? dict.wealthOrderBuy : dict.wealthOrderSell}
                  </span>
                  <span style={{ display: "flex", flexDirection: "column", gap: 1, minWidth: 0 }}>
                    <span style={{ fontSize: 12.5, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                      {o.assetName || categoryLabel(dict, o.category)}
                    </span>
                    <span className="mono" style={{ fontSize: 10, color: "var(--ink-3)" }}>
                      {dict.wealthAllocVenue} · {o.venue || "—"}
                    </span>
                  </span>
                  <span style={{ marginLeft: "auto", display: "flex", flexDirection: "column", alignItems: "flex-end", gap: 1 }}>
                    <span className="mono" style={{ fontSize: 13 }}>
                      {shortTWD(o.amount)}
                    </span>
                    <span className="mono" style={{ fontSize: 10, color: driftColor(o.deviationPt) }}>
                      {fmtDrift(o.deviationPt)}
                    </span>
                  </span>
                </div>
              ))}
            </div>
          ) : (
            <div style={{ fontSize: 12.5, lineHeight: 1.6, color: "var(--ink-2)" }}>{dict.wealthNoOrders}</div>
          )}
          {hasData && alloc!.locked.length > 0 && (
            <div style={{ marginTop: 12, paddingTop: 10, borderTop: "1px solid var(--border)" }}>
              <div className="mono" style={{ fontSize: 10, letterSpacing: "0.06em", color: "var(--ink-3)", marginBottom: 7 }}>
                {dict.wealthLockedTitle}
              </div>
              <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
                {alloc!.locked.map((l) => (
                  <div key={l.category} style={{ display: "flex", alignItems: "center", gap: 8 }}>
                    <span style={dot(CATEGORY_COLOR[l.category])} />
                    <span style={{ fontSize: 12, color: "var(--ink-2)" }}>{categoryLabel(dict, l.category)}</span>
                    <span className="mono" style={{ marginLeft: "auto", fontSize: 11, color: driftColor(l.deviationPt) }}>
                      {fmtDrift(l.deviationPt)}
                    </span>
                  </div>
                ))}
              </div>
            </div>
          )}
          <div style={{ marginTop: "auto", paddingTop: 12, fontSize: 11, color: "var(--ink-3)", lineHeight: 1.6 }}>{dict.wealthOrdersHint}</div>
        </div>
      </div>

      <div className="card" style={{ marginBottom: 16, overflowX: "auto" }}>
        <div style={{ ...MONO_LABEL, marginBottom: 4 }}>{dict.wealthAllocVsTarget}</div>
        {rows.length > 0 ? (
          <table className="mono">
            <thead>
              <tr>
                <th>{dict.wealthAllocClass}</th>
                <th style={{ width: "28%" }} />
                <th>{dict.wealthAllocCurrent}</th>
                <th>{dict.wealthAllocTarget}</th>
                <th>{dict.wealthDeviation}</th>
                <th>{dict.wealthAllocVenue}</th>
                <th>{dict.wealthMarketValue}</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => {
                const color = CATEGORY_COLOR[row.category];
                return (
                  <tr key={row.category}>
                    <td style={{ fontFamily: "var(--font-sans)" }}>
                      <span style={{ display: "flex", alignItems: "center", gap: 8 }}>
                        <span style={dot(color)} />
                        {categoryLabel(dict, row.category)}
                      </span>
                    </td>
                    <td>
                      <span style={{ position: "relative", display: "block" }}>
                        <span style={{ display: "block", height: 8, borderRadius: 4, background: color, width: `${Math.min(row.currentPct * 2.2, 100).toFixed(1)}%` }} />
                        <span
                          style={{
                            position: "absolute",
                            top: -2,
                            bottom: -2,
                            width: 2,
                            borderRadius: 1,
                            background: "var(--ink-2)",
                            left: `${Math.min(row.targetPct * 2.2, 100).toFixed(1)}%`,
                          }}
                        />
                      </span>
                    </td>
                    <td>{row.currentPct.toFixed(1)}%</td>
                    <td style={{ color: "var(--ink-3)" }}>{Math.round(row.targetPct)}%</td>
                    <td style={{ color: driftColor(row.deviationPt) }}>{fmtDrift(row.deviationPt)}</td>
                    <td style={{ fontFamily: "var(--font-sans)", fontSize: 12, color: "var(--ink-3)" }}>{row.venue || "—"}</td>
                    <td>{fmtMoney(row.marketValue, currency)}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        ) : (
          <div className="empty-message">{dict.noData}</div>
        )}
      </div>

      <div style={{ display: "flex", flexWrap: "wrap", gap: 16, marginBottom: 16 }}>
        <div className="card" style={{ flex: "1 1 200px" }}>
          <div style={{ ...MONO_LABEL, marginBottom: 12 }}>{dict.wealthRiskTitle}</div>
          <div className="mono" style={{ fontSize: 32, lineHeight: 1.1 }}>
            {riskPct != null ? `${riskPct.toFixed(1)}%` : "—"}
          </div>
          <div style={{ fontSize: 12, color: "var(--ink-2)", marginTop: 2 }}>
            <span title={dict.wealthTipRisk} style={DOTTED}>
              {dict.wealthRiskShare}
            </span>{" "}
            · {riskBand}
          </div>
          <div style={{ marginTop: 14, background: "var(--bg)", borderRadius: 3 }}>
            <div style={{ height: 6, borderRadius: 3, background: "var(--accent)", width: `${Math.min((riskPct ?? 0) * 2, 100).toFixed(1)}%` }} />
          </div>
        </div>

        <div className="card" style={{ flex: "1 1 200px" }}>
          <div style={{ ...MONO_LABEL, marginBottom: 12 }}>
            <span title={dict.wealthTipFx} style={DOTTED}>
              {dict.wealthFxExposure}
            </span>
          </div>
          <div style={{ display: "flex", flexDirection: "column", gap: 9 }}>
            {(hasData ? alloc!.currencyExposure : []).map((e) => (
              <div key={e.currency} style={{ display: "flex", alignItems: "center", gap: 10 }}>
                <span className="mono" style={{ fontSize: 11, color: "var(--ink-2)", width: 36, flexShrink: 0 }}>
                  {e.currency}
                </span>
                <span style={{ flex: 1, background: "var(--bg)", borderRadius: 3 }}>
                  <span style={{ display: "block", height: 6, borderRadius: 3, background: "#38bdf8", width: `${Math.min(e.pct, 100)}%` }} />
                </span>
                <span className="mono" style={{ fontSize: 11.5, width: 34, textAlign: "right" }}>
                  {Math.round(e.pct)}%
                </span>
              </div>
            ))}
          </div>
        </div>

        <div className="card" style={{ flex: "1 1 200px" }}>
          <div style={{ ...MONO_LABEL, marginBottom: 12 }}>{dict.wealthGeoExposure}</div>
          {/* ponytail: no geo data source yet (needs holdings detail), so only the
              design's "needs holdings" note renders; add rows when a source exists. */}
          <div style={{ fontSize: 12, color: "var(--ink-3)", lineHeight: 1.6 }}>{dict.wealthNeedGeo}</div>
        </div>
      </div>

      <div className="card">
        <div className="eyebrow">{dict.wealthConcentrationTitle}</div>
        {hasData && alloc!.concentration.length > 0 ? (
          <table className="mono">
            <thead>
              <tr>
                <th>{dict.ticker}</th>
                <th>{dict.wealthPctOfAssets}</th>
                <th>{dict.wealthAllocation}</th>
              </tr>
            </thead>
            <tbody>
              {alloc!.concentration.map((c) => (
                <tr key={`${c.market}-${c.ticker}`}>
                  <td>{c.ticker}</td>
                  <td className="loss">{c.pctOfEquity.toFixed(1)}%</td>
                  <td>{c.pctOfAssets.toFixed(1)}%</td>
                </tr>
              ))}
            </tbody>
          </table>
        ) : (
          <div className="empty-message">{dict.wealthNoConcentration}</div>
        )}
        <div style={{ marginTop: 10, fontSize: 11, color: "var(--ink-3)", lineHeight: 1.6 }}>{dict.wealthConcentrationHint}</div>
      </div>
    </>
  );
}
