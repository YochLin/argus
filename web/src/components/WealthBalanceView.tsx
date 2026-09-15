import { useEffect, useState } from "react";
import {
  ApiError,
  fetchWealthBalance,
  fetchWealthDebtPayoff,
  saveWealthProfile,
  type BalanceSheet,
  type DebtPayoffPlan,
  type DebtPayoffResult,
  type QuarterPoint,
} from "../api";
import type { Dictionary } from "../i18n";
import { fmtMoney, groupLabel } from "./WealthHomeView";

interface Props {
  dict: Dictionary;
  writable: boolean;
  onUnauthorized: (retry: () => void) => void;
}

const CURRENCY = "NT$"; // display currency fixed to TWD, same known gap as WealthHomeView

function pct(v: number | null): string {
  return v != null ? `${v.toFixed(1)}%` : "—";
}

function equityLabel(dict: Dictionary, type: string): string {
  if (type === "equity_us") return dict.wealthEquityUS;
  if (type === "equity_tw") return dict.wealthEquityTW;
  return "";
}

export function WealthBalanceView({ dict, writable, onUnauthorized }: Props) {
  const [sheet, setSheet] = useState<BalanceSheet | null>(null);
  const [error, setError] = useState(false);
  const [refreshSignal, setRefreshSignal] = useState(0);
  const [salaryInput, setSalaryInput] = useState("");
  const [savingSalary, setSavingSalary] = useState(false);
  const [extra, setExtra] = useState("");
  const [payoff, setPayoff] = useState<DebtPayoffResult | null>(null);
  const [payoffLoading, setPayoffLoading] = useState(false);

  useEffect(() => {
    setError(false);
    fetchWealthBalance()
      .then(setSheet)
      .catch(() => setError(true));
  }, [refreshSignal]);

  async function saveSalary() {
    const v = Number(salaryInput);
    if (Number.isNaN(v) || v < 0) return;
    setSavingSalary(true);
    try {
      await saveWealthProfile(v);
      setRefreshSignal((n) => n + 1);
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        onUnauthorized(saveSalary);
      } else {
        window.alert(e instanceof ApiError ? e.message : dict.error);
      }
    } finally {
      setSavingSalary(false);
    }
  }

  async function calcPayoff() {
    setPayoffLoading(true);
    try {
      setPayoff(await fetchWealthDebtPayoff(Number(extra) || 0));
    } catch {
      // best-effort calculator — a failed fetch just leaves whatever was
      // showing (or nothing) rather than blocking the rest of the page
    } finally {
      setPayoffLoading(false);
    }
  }

  if (error) {
    return <div className="error-message">{dict.error}</div>;
  }

  return (
    <>
      <div className="stat-grid">
        <StatCard label={dict.wealthNetWorth} text={sheet?.netWorth != null ? fmtMoney(sheet.netWorth, CURRENCY) : "—"} />
        <StatCard label={dict.wealthTotalAssets} text={sheet?.totalAssets != null ? fmtMoney(sheet.totalAssets, CURRENCY) : "—"} />
        <StatCard
          label={dict.wealthTotalLiabilities}
          text={sheet?.totalLiabilities != null ? fmtMoney(sheet.totalLiabilities, CURRENCY) : "—"}
        />
      </div>

      <div className="stat-grid">
        <StatCard label={dict.wealthDebtRatio} text={pct(sheet?.debtRatioPct ?? null)} />
        <StatCard label={dict.wealthLiquidityMonths} text={sheet?.liquidityMonths != null ? sheet.liquidityMonths.toFixed(1) : "—"} />
        <StatCard label={dict.wealthSavingsRate} text={pct(sheet?.savingsRatePct ?? null)} />
        <StatCard label={dict.wealthExpenseRatio} text={pct(sheet?.expenseRatioPct ?? null)} />
      </div>

      {sheet && sheet.monthlySalary == null && writable && (
        <div className="card">
          <div className="eyebrow">{dict.wealthSetSalary}</div>
          <div className="empty-message">{dict.wealthNoSalarySet}</div>
          <label className="form-field" style={{ maxWidth: 240 }}>
            <span>{dict.wealthAnnualSalary}</span>
            <input className="mono" type="number" value={salaryInput} onChange={(e) => setSalaryInput(e.target.value)} />
          </label>
          <div className="modal-actions">
            <button className="btn-primary" disabled={savingSalary || salaryInput.trim() === ""} onClick={saveSalary}>
              {dict.wealthSalarySave}
            </button>
          </div>
        </div>
      )}

      <div className="card">
        <div className="eyebrow">{dict.wealthAssetsLabel}</div>
        {!sheet ? (
          <div className="loading">{dict.loading}</div>
        ) : (
          sheet.assetGroups.map((g) => (
            <div key={g.group} style={{ marginBottom: 16 }}>
              <div className="row-actions" style={{ justifyContent: "space-between" }}>
                <strong>{groupLabel(dict, g.group)}</strong>
                <span className="mono">
                  {fmtMoney(g.marketValue, CURRENCY)}
                  {g.pctOfAssets != null ? ` (${g.pctOfAssets.toFixed(1)}%)` : ""}
                </span>
              </div>
              {g.assets.length > 0 && (
                <table className="mono">
                  <tbody>
                    {g.assets.map((item, i) => (
                      <tr key={item.assetId ?? `${g.group}-${i}`}>
                        <td>{item.assetId ? item.name : equityLabel(dict, item.type)}</td>
                        <td>{item.venue || ""}</td>
                        <td>{fmtMoney(item.valueTwd, CURRENCY)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            </div>
          ))
        )}
      </div>

      <div className="card">
        <div className="eyebrow">{dict.wealthLiabilitiesLabel}</div>
        {!sheet ? (
          <div className="loading">{dict.loading}</div>
        ) : sheet.liabilities.length === 0 ? (
          <div className="empty-message">{dict.wealthEmpty}</div>
        ) : (
          <table className="mono">
            <thead>
              <tr>
                <th>{dict.wealthName}</th>
                <th>{dict.wealthRatePct}</th>
                <th>{dict.wealthRemainingMonths}</th>
                <th>{dict.wealthMinPayment}</th>
                <th>{dict.wealthValue}</th>
              </tr>
            </thead>
            <tbody>
              {sheet.liabilities.map((l) => (
                <tr key={l.assetId}>
                  <td>{l.name}</td>
                  <td>{l.ratePct != null ? `${l.ratePct}%` : "—"}</td>
                  <td>{l.remainingMonths ?? "—"}</td>
                  <td>{l.minPayment != null ? fmtMoney(l.minPayment, CURRENCY) : "—"}</td>
                  <td>{fmtMoney(l.valueTwd, CURRENCY)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <div className="card">
        <div className="eyebrow">{dict.wealthQuarterlyTrend}</div>
        {sheet && sheet.quarterlyTrend.length > 0 && <QuarterlyBars points={sheet.quarterlyTrend} />}
      </div>

      {sheet && sheet.liabilities.length > 0 && (
        <div className="card">
          <div className="eyebrow">{dict.wealthDebtPayoffTitle}</div>
          <label className="form-field" style={{ maxWidth: 240 }}>
            <span>{dict.wealthExtraPayment}</span>
            <input className="mono" type="number" value={extra} onChange={(e) => setExtra(e.target.value)} />
          </label>
          <div className="modal-actions">
            <button className="btn-primary" disabled={payoffLoading} onClick={calcPayoff}>
              {dict.wealthCalculate}
            </button>
          </div>
          {payoff && (
            <>
              {payoff.loans.length === 0 ? (
                <div className="empty-message">{dict.wealthPayoffNoLoans}</div>
              ) : (
                <>
                  <table className="mono">
                    <thead>
                      <tr>
                        <th></th>
                        <th>{dict.wealthPayoffOrder}</th>
                        <th>{dict.wealthPayoffMonths}</th>
                        <th>{dict.wealthPayoffMonthsSaved}</th>
                        <th>{dict.wealthPayoffTotalInterest}</th>
                      </tr>
                    </thead>
                    <tbody>
                      <PayoffRow label={dict.wealthSnowball} plan={payoff.snowball} />
                      <PayoffRow label={dict.wealthAvalanche} plan={payoff.avalanche} />
                    </tbody>
                  </table>
                  <div className="eyebrow" style={{ marginTop: 8 }}>
                    {dict.wealthPayoffInterestDiff}: {fmtMoney(payoff.interestDifference, CURRENCY)}
                  </div>
                </>
              )}
            </>
          )}
        </div>
      )}
    </>
  );
}

function StatCard({ label, text }: { label: string; text: string }) {
  return (
    <div className="card kpi-card">
      <div className="eyebrow">{label}</div>
      <div className="kpi-value">{text}</div>
    </div>
  );
}

function PayoffRow({ label, plan }: { label: string; plan: DebtPayoffPlan }) {
  return (
    <tr>
      <td>{label}</td>
      <td>{plan.order.join(" → ")}</td>
      <td>{plan.months}</td>
      <td>{plan.monthsSaved}</td>
      <td>{fmtMoney(plan.totalInterest, CURRENCY)}</td>
    </tr>
  );
}

// Bar height is scaled off magnitude (not signed value) since net worth can
// be negative (e.g. early in a mortgage) — a signed scale would flatten
// every other bar to near-zero the moment one quarter goes negative. Sign
// is instead shown by color (loss red vs. accent), matching the rest of the
// app's P&L convention.
function QuarterlyBars({ points }: { points: QuarterPoint[] }) {
  const maxAbs = Math.max(1, ...points.map((p) => Math.abs(p.netWorth ?? 0)));
  return (
    <div style={{ display: "flex", alignItems: "flex-end", gap: 8, height: 140, marginTop: 8 }}>
      {points.map((p) => {
        const height = p.netWorth != null ? Math.max(2, (Math.abs(p.netWorth) / maxAbs) * 120) : 2;
        const negative = (p.netWorth ?? 0) < 0;
        return (
          <div key={p.quarter} style={{ flex: 1, display: "flex", flexDirection: "column", alignItems: "center", gap: 4 }}>
            <div
              style={{
                width: "100%",
                height: `${height}px`,
                background: p.netWorth == null ? "var(--border)" : negative ? "var(--loss)" : "var(--accent)",
                borderRadius: 2,
              }}
              title={p.netWorth != null ? fmtMoney(p.netWorth, CURRENCY) : "—"}
            />
            <span className="mono" style={{ fontSize: 11 }}>
              {p.quarter}
            </span>
          </div>
        );
      })}
    </div>
  );
}
