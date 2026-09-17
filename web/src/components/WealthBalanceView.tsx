import { useEffect, useState } from "react";
import {
  ApiError,
  fetchWealthBalance,
  fetchWealthDebtPayoff,
  saveWealthProfile,
  type BalanceSheet,
  type BalanceSheetItem,
  type DebtPayoffPlan,
  type DebtPayoffResult,
  type LiabilityDetail,
  type QuarterPoint,
} from "../api";
import type { Dictionary } from "../i18n";
import { fmtMoney, groupColorClass, groupLabel } from "./WealthHomeView";

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

function srcLabel(dict: Dictionary, source: string): string {
  if (source === "import") return dict.wealthSrcImport;
  if (source === "sync") return dict.wealthSrcSync;
  return dict.wealthSrcManual;
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

  const netPct = sheet?.netWorth != null && sheet.totalAssets ? (sheet.netWorth / sheet.totalAssets) * 100 : null;

  return (
    <>
      <div className="card card--glow" style={{ marginBottom: 16 }}>
        <div className="eyebrow">{dict.wealthNetWorth}</div>
        <div className="wealth-hero-row" style={{ marginTop: 8 }}>
          <div className="wealth-hero-block">
            <div className="wealth-hero-block-value big">
              {sheet?.netWorth != null ? fmtMoney(sheet.netWorth, CURRENCY) : "—"}
            </div>
          </div>
          <div className="wealth-hero-block">
            <span className="eyebrow">{dict.wealthTotalAssets}</span>
            <span className="wealth-hero-block-value">
              {sheet?.totalAssets != null ? fmtMoney(sheet.totalAssets, CURRENCY) : "—"}
            </span>
          </div>
          <div className="wealth-hero-block">
            <span className="eyebrow">{dict.wealthTotalLiabilities}</span>
            <span className="wealth-hero-block-value loss">
              {sheet?.totalLiabilities != null ? fmtMoney(sheet.totalLiabilities, CURRENCY) : "—"}
            </span>
          </div>
          <div className="wealth-hero-block">
            <span className="eyebrow">{dict.wealthNetWorth} / {dict.wealthTotalAssets}</span>
            <span className="wealth-hero-block-value">{pct(netPct)}</span>
          </div>
        </div>
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

      <div className="detail-grid-2col">
        <div className="card">
          <div className="wealth-group-header" style={{ border: "none", padding: 0, marginBottom: 14 }}>
            <span>{dict.wealthAssetsLabel}</span>
            <span className="wealth-group-header-value">
              {sheet?.totalAssets != null ? fmtMoney(sheet.totalAssets, CURRENCY) : "—"}
            </span>
          </div>
          {!sheet ? (
            <div className="loading">{dict.loading}</div>
          ) : (
            <div className="wealth-col-stack">
              {sheet.assetGroups
                .filter((g) => g.assets.length > 0)
                .map((g) => (
                  <div key={g.group} className="wealth-group-block">
                    <div className="wealth-group-header">
                      <span className={`wealth-dot ${groupColorClass(g.group)}`} />
                      {groupLabel(dict, g.group)}
                      <span style={{ fontSize: 10, color: "var(--ink-3)" }}>
                        {g.pctOfAssets != null ? `${g.pctOfAssets.toFixed(1)}% ${dict.wealthPctOfAssets}` : ""}
                      </span>
                      <span className="wealth-group-header-value">{fmtMoney(g.marketValue, CURRENCY)}</span>
                    </div>
                    {g.assets.map((item, i) => (
                      <AssetItemRow key={item.assetId ?? `${g.group}-${i}`} dict={dict} item={item} />
                    ))}
                  </div>
                ))}
            </div>
          )}
        </div>

        <div className="wealth-col-stack">
          <div className="card">
            <div className="wealth-group-header" style={{ border: "none", padding: 0, marginBottom: 14 }}>
              <span>{dict.wealthLiabilitiesLabel}</span>
              <span className="wealth-group-header-value loss">
                {sheet?.totalLiabilities != null ? fmtMoney(sheet.totalLiabilities, CURRENCY) : "—"}
              </span>
            </div>
            {!sheet ? (
              <div className="loading">{dict.loading}</div>
            ) : sheet.liabilities.length === 0 ? (
              <div className="empty-message">{dict.wealthEmpty}</div>
            ) : (
              <div className="wealth-col-stack" style={{ gap: 8 }}>
                {sheet.liabilities.map((l) => (
                  <LiabilityItemRow key={l.assetId} dict={dict} item={l} />
                ))}
              </div>
            )}
          </div>

          <div className="card">
            <div className="eyebrow" style={{ marginBottom: 14 }}>
              {dict.wealthDebtRatio} / {dict.wealthLiquidityMonths} / {dict.wealthSavingsRate} / {dict.wealthExpenseRatio}
            </div>
            <div className="wealth-ratios-grid">
              <RatioBlock label={dict.wealthDebtRatio} value={pct(sheet?.debtRatioPct ?? null)} />
              <RatioBlock
                label={dict.wealthLiquidityMonths}
                value={sheet?.liquidityMonths != null ? sheet.liquidityMonths.toFixed(1) : "—"}
              />
              <RatioBlock label={dict.wealthSavingsRate} value={pct(sheet?.savingsRatePct ?? null)} />
              <RatioBlock label={dict.wealthExpenseRatio} value={pct(sheet?.expenseRatioPct ?? null)} />
            </div>
          </div>
        </div>
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

function AssetItemRow({ dict, item }: { dict: Dictionary; item: BalanceSheetItem }) {
  return (
    <div className="wealth-item-row">
      <span>{item.assetId ? item.name : equityLabel(dict, item.type)}</span>
      <span className={`wealth-src-tag ${item.source}`}>{srcLabel(dict, item.source)}</span>
      <span className="wealth-item-row-value">{fmtMoney(item.valueTwd, CURRENCY)}</span>
    </div>
  );
}

function LiabilityItemRow({ dict, item }: { dict: Dictionary; item: LiabilityDetail }) {
  return (
    <div className="wealth-item-row">
      <span>{item.name}</span>
      <span className="mono" style={{ fontSize: 10.5, color: "var(--ink-3)" }}>
        {item.ratePct != null ? `${item.ratePct}%` : ""}
        {item.remainingMonths != null ? ` · ${item.remainingMonths} ${dict.wealthRemainingMonths}` : ""}
      </span>
      <span className={`wealth-src-tag ${item.source}`}>{srcLabel(dict, item.source)}</span>
      <span className="wealth-item-row-value loss">{fmtMoney(item.valueTwd, CURRENCY)}</span>
    </div>
  );
}

function RatioBlock({ label, value }: { label: string; value: string }) {
  return (
    <div className="wealth-ratio-block">
      <span className="wealth-ratio-block-label">{label}</span>
      <span className="wealth-ratio-block-value">{value}</span>
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
    <div style={{ display: "flex", alignItems: "flex-end", gap: 8, height: 150, marginTop: 12 }}>
      {points.map((p) => {
        const height = p.netWorth != null ? Math.max(2, (Math.abs(p.netWorth) / maxAbs) * 100) : 2;
        const negative = (p.netWorth ?? 0) < 0;
        return (
          <div
            key={p.quarter}
            style={{
              flex: 1,
              display: "flex",
              flexDirection: "column",
              alignItems: "center",
              justifyContent: "flex-end",
              height: "100%",
              gap: 8,
            }}
          >
            <span className="mono" style={{ fontSize: 11, color: "var(--ink-2)" }}>
              {p.netWorth != null ? fmtMoney(p.netWorth, CURRENCY) : "—"}
            </span>
            <div
              style={{
                width: "100%",
                height: `${height}px`,
                background: p.netWorth == null ? "var(--border)" : negative ? "var(--loss)" : "var(--accent)",
                borderRadius: 2,
              }}
            />
            <span className="mono" style={{ fontSize: 11, color: "var(--ink-3)" }}>
              {p.quarter}
            </span>
          </div>
        );
      })}
    </div>
  );
}
