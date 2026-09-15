import { useEffect, useState } from "react";
import { fetchWealthAlloc, type AllocationModel, type WealthAlloc } from "../api";
import type { Dictionary } from "../i18n";
import { fmtMoney, groupColorClass, groupLabel } from "./WealthHomeView";

interface Props {
  dict: Dictionary;
}

const MODELS: AllocationModel[] = ["conserv", "balanced", "growth"];
const currency = "NT$"; // display currency fixed to TWD for now, same as WealthHomeView/WealthBalanceView

function modelLabel(dict: Dictionary, m: AllocationModel): string {
  return m === "conserv" ? dict.wealthModelConserv : m === "growth" ? dict.wealthModelGrowth : dict.wealthModelBalanced;
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
        <div className="topbar-tabs" role="group" aria-label="allocation model" style={{ marginLeft: "auto" }}>
          {MODELS.map((m) => (
            <button key={m} className={`topbar-tab${model === m ? " active" : ""}`} onClick={() => setModel(m)}>
              {modelLabel(dict, m)}
            </button>
          ))}
        </div>
      </div>

      <div className="card">
        <div className="eyebrow">{dict.wealthAllocation}</div>
        {hasData && alloc!.allocation.length > 0 ? (
          <table className="mono">
            <thead>
              <tr>
                <th>{dict.wealthGroupLabel}</th>
                <th>{dict.wealthCurrentPct}</th>
                <th>{dict.wealthTargetPct}</th>
                <th>{dict.wealthDeviation}</th>
                <th>{dict.wealthVenueLabel}</th>
                <th>{dict.wealthMarketValue}</th>
              </tr>
            </thead>
            <tbody>
              {alloc!.allocation.map((row) => (
                <tr key={row.group}>
                  <td>
                    <span style={{ display: "flex", alignItems: "center", gap: 8 }}>
                      <span className={`wealth-dot ${groupColorClass(row.group)}`} />
                      {groupLabel(dict, row.group)}
                    </span>
                  </td>
                  <td>{row.currentPct.toFixed(1)}%</td>
                  <td>{row.targetPct.toFixed(1)}%</td>
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

      <div className="card">
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
          <div className="wealth-col-stack">
            {alloc!.orders.map((o) => (
              <div key={o.group} className="wealth-item-row">
                <span className={`wealth-dot ${groupColorClass(o.group)}`} />
                <span style={{ fontWeight: 600, color: o.side === "buy" ? "var(--profit)" : "var(--loss)" }}>
                  {o.side === "buy" ? dict.wealthOrderBuy : dict.wealthOrderSell}
                </span>
                <span style={{ display: "flex", flexDirection: "column", gap: 1 }}>
                  {o.assetName || groupLabel(dict, o.group)}
                  <span className="mono" style={{ fontSize: 10, color: "var(--ink-3)" }}>
                    {dict.wealthVenueLabel} {o.venue || "—"}
                  </span>
                </span>
                <span className="wealth-item-row-value mono">{fmtMoney(o.amount, currency)}</span>
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
            {alloc!.locked.map((l) => (
              <div key={l.group} className="wealth-item-row">
                <span className={`wealth-dot ${groupColorClass(l.group)}`} />
                {groupLabel(dict, l.group)}
                <span
                  className={`wealth-item-row-value mono ${l.deviationPt > 0 ? "profit" : l.deviationPt < 0 ? "loss" : ""}`}
                >
                  {l.deviationPt > 0 ? "+" : ""}
                  {l.deviationPt.toFixed(1)}pt
                </span>
              </div>
            ))}
          </div>
        )}
        <div style={{ marginTop: 12, fontSize: 11, color: "var(--ink-3)", lineHeight: 1.6 }}>{dict.wealthOrdersHint}</div>
      </div>

      <div style={{ display: "flex", flexWrap: "wrap", gap: "var(--gap)" }}>
        <div className="card" style={{ flex: "1 1 200px" }}>
          <div className="eyebrow">{dict.wealthRiskTitle}</div>
          <div className="mono" style={{ fontSize: 32, lineHeight: 1.1 }}>
            {hasData && alloc!.riskPct != null ? `${alloc!.riskPct.toFixed(1)}%` : "—"}
          </div>
          <div style={{ fontSize: 12, color: "var(--ink-2)", marginTop: 2 }}>
            {hasData && alloc!.riskTargetLow != null && alloc!.riskTargetHigh != null
              ? `${dict.wealthRiskBand} ${alloc!.riskTargetLow.toFixed(0)}–${alloc!.riskTargetHigh.toFixed(0)}%`
              : "—"}
          </div>
        </div>

        <div className="card" style={{ flex: "1 1 200px" }}>
          <div className="eyebrow">{dict.wealthFxExposure}</div>
          {hasData && alloc!.currencyExposure.length > 0 ? (
            <div className="wealth-col-stack">
              {alloc!.currencyExposure.map((e) => (
                <div key={e.currency} style={{ display: "flex", alignItems: "center", gap: 10 }}>
                  <span className="mono" style={{ fontSize: 11, color: "var(--ink-2)", width: 40, flexShrink: 0 }}>
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
                  <span className="mono" style={{ fontSize: 11.5, width: 40, textAlign: "right" }}>
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
