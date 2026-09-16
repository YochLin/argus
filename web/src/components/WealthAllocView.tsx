import { useEffect, useState } from "react";
import { fetchWealthAlloc, type AllocationModel, type AllocCategory, type WealthAlloc, type WealthAllocRow } from "../api";
import type { Dictionary } from "../i18n";
import { fmtMoney } from "./WealthHomeView";

interface Props {
  dict: Dictionary;
}

const MODELS: AllocationModel[] = ["conserv", "balanced", "growth"];
const currency = "NT$"; // display currency fixed to TWD for now, same as WealthHomeView/WealthBalanceView

function modelLabel(dict: Dictionary, m: AllocationModel): string {
  return m === "conserv" ? dict.wealthModelConserv : m === "growth" ? dict.wealthModelGrowth : dict.wealthModelBalanced;
}

// categoryLabel/categoryColorClass are /w/alloc's own nine-category
// taxonomy (assets.AllocCategories, docs/phase-9-asset-platform.md §8.5) —
// separate from WealthHomeView's four-bucket groupLabel/groupColorClass,
// which the home page and balance sheet still use.
function categoryLabel(dict: Dictionary, c: AllocCategory): string {
  switch (c) {
    case "cash":
      return dict.wealthCategoryCash;
    case "equity":
      return dict.wealthCategoryEquity;
    case "fund":
      return dict.wealthCategoryFund;
    case "bond":
      return dict.wealthCategoryBond;
    case "insurance":
      return dict.wealthCategoryInsurance;
    case "estate":
      return dict.wealthCategoryEstate;
    case "gold":
      return dict.wealthCategoryGold;
    case "crypto":
      return dict.wealthCategoryCrypto;
    case "pension":
      return dict.wealthCategoryPension;
  }
}

const CATEGORY_COLOR_CLASS: Record<AllocCategory, string> = {
  cash: "s1",
  equity: "s2",
  fund: "s7",
  bond: "s3",
  insurance: "s5",
  estate: "s4",
  gold: "s8",
  crypto: "s9",
  pension: "s6",
};

function categoryColorClass(c: AllocCategory): string {
  return CATEGORY_COLOR_CLASS[c];
}

// donutGradient builds the design template's `wam.donut` ring — a CSS-only
// pie chart via conic-gradient (no chart library needed), one slice per
// non-zero currentPct in the series-color CSS vars .wealth-bar-seg already
// uses. A center "hole" the same size as the card background is overlaid on
// top (see the JSX below) to turn the pie into a donut.
function donutGradient(rows: WealthAllocRow[]): string {
  let acc = 0;
  const stops: string[] = [];
  for (const row of rows) {
    if (row.currentPct <= 0) continue;
    const from = acc;
    acc = Math.min(100, acc + row.currentPct);
    stops.push(`var(--${categoryColorClass(row.category)}) ${from}% ${acc}%`);
  }
  if (stops.length === 0) return "var(--border)";
  return `conic-gradient(${stops.join(", ")})`;
}

export function WealthAllocView({ dict }: Props) {
  const [model, setModel] = useState<AllocationModel>("balanced");
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

  return (
    <>
      <div style={{ display: "flex", alignItems: "center", gap: 12, marginBottom: 12, flexWrap: "wrap" }}>
        <div className="eyebrow">{dict.navWealthAlloc}</div>
        <span className="mono" style={{ marginLeft: "auto", fontSize: 10, letterSpacing: "0.06em", color: "var(--ink-3)" }}>
          {dict.wealthTargetModelLabel}
        </span>
        <div className="topbar-tabs" role="group" aria-label="allocation model">
          {MODELS.map((m) => (
            <button key={m} className={`topbar-tab${model === m ? " active" : ""}`} onClick={() => setModel(m)}>
              {modelLabel(dict, m)}
            </button>
          ))}
        </div>
      </div>

      <div className="detail-grid-2col" style={{ alignItems: "stretch" }}>
        <div className="card" style={{ display: "flex", flexDirection: "column" }}>
          <div className="eyebrow" style={{ marginBottom: 14 }}>
            {dict.wealthMixTitle}
          </div>
          {hasData && alloc!.allocation.length > 0 ? (
            <div style={{ display: "flex", alignItems: "center", gap: 20, flexWrap: "wrap", flex: 1 }}>
              <div style={{ position: "relative", display: "flex", alignItems: "center", justifyContent: "center" }}>
                <div style={{ width: 140, height: 140, borderRadius: "50%", background: donutGradient(alloc!.allocation) }} />
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
              <div
                style={{
                  display: "flex",
                  flexDirection: "column",
                  justifyContent: "space-between",
                  gap: 7,
                  flex: 1,
                  minWidth: 180,
                  alignSelf: "stretch",
                }}
              >
                {alloc!.allocation.map((row) => (
                  <div key={row.category} style={{ display: "flex", alignItems: "center", gap: 8 }}>
                    <span className={`wealth-dot ${categoryColorClass(row.category)}`} />
                    <span style={{ fontSize: 12, color: "var(--ink-2)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                      {categoryLabel(dict, row.category)}
                    </span>
                    <span className="mono" style={{ marginLeft: "auto", fontSize: 12 }}>
                      {row.currentPct.toFixed(1)}%
                    </span>
                    <span className="mono" style={{ fontSize: 10.5, color: "var(--ink-3)", width: 34, textAlign: "right" }}>
                      {row.targetPct.toFixed(1)}%
                    </span>
                  </div>
                ))}
              </div>
            </div>
          ) : (
            <div className="empty-message">{dict.noData}</div>
          )}
        </div>

        <div className="card" style={{ display: "flex", flexDirection: "column" }}>
          <div style={{ display: "flex", alignItems: "baseline", gap: 10, marginBottom: 12 }}>
            <div className="eyebrow" style={{ marginBottom: 0 }}>
              {dict.wealthOrdersTitle}
            </div>
            {hasData && alloc!.orders.length > 0 && (
              <span className="mono" style={{ fontSize: 11, color: "var(--ink-3)" }}>
                {alloc!.orders.length} · {fmtMoney(alloc!.rebalTotal, currency)} {dict.wealthRebalTotal}
              </span>
            )}
          </div>
          {hasData && alloc!.orders.length > 0 ? (
            <div style={{ display: "flex", flexDirection: "column", gap: 9 }}>
              {alloc!.orders.map((o) => (
                <div
                  key={o.category}
                  style={{ display: "flex", alignItems: "center", gap: 9, paddingBottom: 9, borderBottom: "1px solid var(--border)" }}
                >
                  <span className={`wealth-dot ${categoryColorClass(o.category)}`} />
                  <span style={{ fontWeight: 600, color: o.side === "buy" ? "var(--profit)" : "var(--loss)" }}>
                    {o.side === "buy" ? dict.wealthOrderBuy : dict.wealthOrderSell}
                  </span>
                  <span style={{ display: "flex", flexDirection: "column", gap: 1, minWidth: 0 }}>
                    <span style={{ fontSize: 12.5, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                      {o.assetName || categoryLabel(dict, o.category)}
                    </span>
                    <span className="mono" style={{ fontSize: 10, color: "var(--ink-3)" }}>
                      {dict.wealthVenueLabel} {o.venue || "—"}
                    </span>
                  </span>
                  <span style={{ marginLeft: "auto", display: "flex", flexDirection: "column", alignItems: "flex-end", gap: 1 }}>
                    <span className="mono" style={{ fontSize: 13 }}>
                      {fmtMoney(o.amount, currency)}
                    </span>
                    <span className={`mono ${o.deviationPt > 0 ? "profit" : "loss"}`} style={{ fontSize: 10 }}>
                      {o.deviationPt > 0 ? "+" : ""}
                      {o.deviationPt.toFixed(1)}pt
                    </span>
                  </span>
                </div>
              ))}
            </div>
          ) : (
            <div className="empty-message">{dict.wealthNoOrders}</div>
          )}
        {hasData && alloc!.locked.length > 0 && (
          <div style={{ marginTop: 12, paddingTop: 10, borderTop: "1px solid var(--border)" }}>
            <div className="mono" style={{ fontSize: 10, letterSpacing: "0.06em", color: "var(--ink-3)", marginBottom: 7 }}>
              {dict.wealthLockedTitle}
            </div>
            <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
              {alloc!.locked.map((l) => (
                <div key={l.category} style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 12, color: "var(--ink-2)" }}>
                  <span className={`wealth-dot ${categoryColorClass(l.category)}`} />
                  {categoryLabel(dict, l.category)}
                  <span
                    className={`mono ${l.deviationPt > 0 ? "profit" : l.deviationPt < 0 ? "loss" : ""}`}
                    style={{ marginLeft: "auto" }}
                  >
                    {l.deviationPt > 0 ? "+" : ""}
                    {l.deviationPt.toFixed(1)}pt
                  </span>
                </div>
              ))}
            </div>
          </div>
        )}
          <div style={{ marginTop: 12, fontSize: 11, color: "var(--ink-3)", lineHeight: 1.6 }}>{dict.wealthOrdersHint}</div>
        </div>
      </div>

      <div className="card" style={{ marginBottom: 16 }}>
        <div className="eyebrow">{dict.wealthAllocation}</div>
        {hasData && alloc!.allocation.length > 0 ? (
          <table className="mono">
            <thead>
              <tr>
                <th>{dict.wealthGroupLabel}</th>
                <th style={{ width: "28%" }} />
                <th>{dict.wealthCurrentPct}</th>
                <th>{dict.wealthTargetPct}</th>
                <th>{dict.wealthDeviation}</th>
                <th>{dict.wealthVenueLabel}</th>
                <th>{dict.wealthMarketValue}</th>
              </tr>
            </thead>
            <tbody>
              {alloc!.allocation.map((row) => (
                <tr key={row.category}>
                  <td>
                    <span style={{ display: "flex", alignItems: "center", gap: 8 }}>
                      <span className={`wealth-dot ${categoryColorClass(row.category)}`} />
                      {categoryLabel(dict, row.category)}
                    </span>
                  </td>
                  <td>
                    <span style={{ position: "relative", display: "block", background: "var(--bg)", borderRadius: 3, height: 6 }}>
                      <span
                        className={`wealth-bar-seg ${categoryColorClass(row.category)}`}
                        style={{ display: "block", width: `${Math.max(0, Math.min(100, row.currentPct))}%`, borderRadius: 3 }}
                      />
                      <span
                        style={{
                          position: "absolute",
                          top: -2,
                          left: `${Math.max(0, Math.min(100, row.targetPct))}%`,
                          width: 2,
                          height: 10,
                          background: "var(--ink)",
                        }}
                      />
                    </span>
                  </td>
                  <td>{row.currentPct.toFixed(1)}%</td>
                  <td style={{ color: "var(--ink-3)" }}>{row.targetPct.toFixed(1)}%</td>
                  <td className={row.deviationPt > 0 ? "profit" : row.deviationPt < 0 ? "loss" : ""}>
                    {row.deviationPt > 0 ? "+" : ""}
                    {row.deviationPt.toFixed(1)}pt
                  </td>
                  <td style={{ fontFamily: "var(--sans)", fontSize: 12, color: "var(--ink-3)" }}>{row.venue || "—"}</td>
                  <td>{fmtMoney(row.marketValue, currency)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        ) : (
          <div className="empty-message">{dict.noData}</div>
        )}
      </div>

      <div className="detail-grid-2col" style={{ alignItems: "stretch" }}>
        <div className="card">
          <div className="eyebrow">{dict.wealthRiskTitle}</div>
          <div className="mono" style={{ fontSize: 32, lineHeight: 1.1 }}>
            {hasData && alloc!.riskPct != null ? `${alloc!.riskPct.toFixed(1)}%` : "—"}
          </div>
          <div style={{ fontSize: 12, color: "var(--ink-2)", marginTop: 2 }}>
            {hasData && alloc!.riskTargetLow != null && alloc!.riskTargetHigh != null
              ? `${dict.wealthRiskBand} ${alloc!.riskTargetLow.toFixed(0)}–${alloc!.riskTargetHigh.toFixed(0)}%`
              : "—"}
          </div>
          {hasData && alloc!.riskPct != null && (
            <div style={{ marginTop: 14, background: "var(--bg)", borderRadius: 3, height: 6 }}>
              <div
                style={{
                  display: "block",
                  width: `${Math.max(0, Math.min(100, alloc!.riskPct))}%`,
                  height: 6,
                  borderRadius: 3,
                  background: "var(--accent)",
                }}
              />
            </div>
          )}
        </div>

        <div className="card">
          <div className="eyebrow">{dict.wealthFxExposure}</div>
          {hasData && alloc!.currencyExposure.length > 0 ? (
            <div style={{ display: "flex", flexDirection: "column", gap: 9 }}>
              {alloc!.currencyExposure.map((e) => (
                <div key={e.currency} style={{ display: "flex", alignItems: "center", gap: 10 }}>
                  <span className="mono" style={{ fontSize: 11, color: "var(--ink-2)", width: 36, flexShrink: 0 }}>
                    {e.currency}
                  </span>
                  <span style={{ flex: 1, background: "var(--bg)", borderRadius: 3, height: 6 }}>
                    <span
                      style={{
                        display: "block",
                        width: `${Math.max(0, Math.min(100, e.pct))}%`,
                        height: 6,
                        borderRadius: 3,
                        background: "var(--accent)",
                      }}
                    />
                  </span>
                  <span className="mono" style={{ fontSize: 11.5, width: 34, textAlign: "right" }}>
                    {e.pct.toFixed(1)}%
                  </span>
                </div>
              ))}
            </div>
          ) : (
            <div className="empty-message">{dict.noData}</div>
          )}
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
