import { useEffect, useState } from "react";
import {
  ApiError,
  createWealthCashflow,
  deactivateWealthCashflow,
  fetchWealthCash,
  type CashflowDirection,
  type CashflowItem,
  type WealthCash,
} from "../api";
import type { Dictionary } from "../i18n";
import { fmtMoney } from "./WealthHomeView";

interface Props {
  dict: Dictionary;
  writable: boolean;
  onUnauthorized: (retry: () => void) => void;
}

const CURRENCY = "NT$"; // display currency fixed to TWD, same known gap as WealthHomeView/WealthBalanceView

// INCOME_CATEGORIES/EXPENSE_CATEGORIES mirror the design template's 收入五項/
// 支出六項 (docs/phase-9-asset-platform.md §8.4) — a free-text `category`
// column server-side, but the add form only ever offers these eleven so the
// item list groups predictably.
const INCOME_CATEGORIES = ["salary", "rent", "dividend", "bond_interest", "fund_dividend"] as const;
const EXPENSE_CATEGORIES = ["living", "mortgage", "sip", "loan", "insurance", "tax"] as const;

function categoryLabel(dict: Dictionary, category: string | undefined): string {
  switch (category) {
    case "salary":
      return dict.wealthCashCatSalary;
    case "rent":
      return dict.wealthCashCatRent;
    case "dividend":
      return dict.wealthCashCatDividend;
    case "bond_interest":
      return dict.wealthCashCatBondInterest;
    case "fund_dividend":
      return dict.wealthCashCatFundDividend;
    case "living":
      return dict.wealthCashCatLiving;
    case "mortgage":
      return dict.wealthCashCatMortgage;
    case "sip":
      return dict.wealthCashCatSip;
    case "loan":
      return dict.wealthCashCatLoan;
    case "insurance":
      return dict.wealthCashCatInsurance;
    case "tax":
      return dict.wealthCashCatTax;
    default:
      return category || "—";
  }
}

function AddCashflowForm({
  dict,
  onUnauthorized,
  onSaved,
}: {
  dict: Dictionary;
  onUnauthorized: (retry: () => void) => void;
  onSaved: () => void;
}) {
  const [direction, setDirection] = useState<CashflowDirection>("out");
  const [name, setName] = useState("");
  const [amount, setAmount] = useState("");
  const [dayOfMonth, setDayOfMonth] = useState("");
  const [category, setCategory] = useState<string>(EXPENSE_CATEGORIES[0]);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const categories = direction === "in" ? INCOME_CATEGORIES : EXPENSE_CATEGORIES;

  function changeDirection(next: CashflowDirection) {
    setDirection(next);
    setCategory(next === "in" ? INCOME_CATEGORIES[0] : EXPENSE_CATEGORIES[0]);
  }

  async function submit() {
    const amt = Number(amount);
    if (!name.trim() || !(amt > 0)) {
      setError(dict.error);
      return;
    }
    const day = dayOfMonth.trim() ? Number(dayOfMonth) : undefined;
    setSubmitting(true);
    setError(null);
    try {
      await createWealthCashflow({ direction, name: name.trim(), amount: amt, dayOfMonth: day, category });
      setName("");
      setAmount("");
      setDayOfMonth("");
      onSaved();
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        onUnauthorized(submit);
      } else {
        setError(e instanceof ApiError ? e.message : dict.error);
      }
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="card" style={{ marginBottom: 16 }}>
      <div className="eyebrow">{dict.wealthCashAddTitle}</div>
      <div className="topbar-tabs" role="group" aria-label="direction" style={{ marginBottom: 10 }}>
        <button className={`topbar-tab${direction === "in" ? " active" : ""}`} onClick={() => changeDirection("in")}>
          {dict.wealthCashDirectionIn}
        </button>
        <button className={`topbar-tab${direction === "out" ? " active" : ""}`} onClick={() => changeDirection("out")}>
          {dict.wealthCashDirectionOut}
        </button>
      </div>
      <div style={{ display: "flex", gap: 10, flexWrap: "wrap" }}>
        <label className="form-field" style={{ flex: "1 1 160px" }}>
          <span>{dict.wealthCashNameLabel}</span>
          <input value={name} onChange={(e) => setName(e.target.value)} />
        </label>
        <label className="form-field" style={{ width: 130 }}>
          <span>{dict.wealthCashAmountLabel}</span>
          <input className="mono" type="number" value={amount} onChange={(e) => setAmount(e.target.value)} />
        </label>
        <label className="form-field" style={{ width: 110 }}>
          <span>{dict.wealthCashDayOfMonthLabel}</span>
          <input className="mono" type="number" min={1} max={31} value={dayOfMonth} onChange={(e) => setDayOfMonth(e.target.value)} />
        </label>
        <label className="form-field" style={{ width: 160 }}>
          <span>{dict.wealthCashCategoryLabel}</span>
          <select value={category} onChange={(e) => setCategory(e.target.value)}>
            {categories.map((c) => (
              <option key={c} value={c}>
                {categoryLabel(dict, c)}
              </option>
            ))}
          </select>
        </label>
      </div>
      {error && <div className="error-message">{error}</div>}
      <div className="modal-actions">
        <button className="btn-primary" disabled={submitting} onClick={submit}>
          {dict.wealthCashAdd}
        </button>
      </div>
    </div>
  );
}

// BreakdownCard mirrors the template's income/expense breakdown cards
// (Argus Trading WebUI.dc.html lines 811-850, cashModel()'s inRows/outRows)
// — each active item sorted by its own TWD value, a bar sized relative to
// the largest item across BOTH cards (cashModel()'s maxRow), and a % of
// that column's own total.
function BreakdownCard({
  title,
  items,
  total,
  maxValue,
  positive,
}: {
  title: string;
  items: CashflowItem[];
  total: number;
  maxValue: number;
  positive: boolean;
}) {
  return (
    <div className="card">
      <div style={{ display: "flex", alignItems: "baseline", gap: 10, marginBottom: 14 }}>
        <span className="eyebrow">{title}</span>
        <span className={`mono ${positive ? "profit" : "loss"}`} style={{ marginLeft: "auto", fontSize: 13 }}>
          {fmtMoney(total, CURRENCY)}
        </span>
      </div>
      <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
        {items.map((item) => {
          const value = item.valueTwd ?? 0;
          const pct = total > 0 ? (value / total) * 100 : 0;
          const barPct = maxValue > 0 ? (value / maxValue) * 100 : 0;
          return (
            <div key={item.id} style={{ display: "flex", flexDirection: "column", gap: 5 }}>
              <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                <span style={{ fontFamily: "var(--sans)", fontSize: 12.5 }}>{item.name}</span>
                <span className="mono" style={{ fontSize: 10, color: "var(--ink-3)" }}>
                  {pct.toFixed(0)}%
                </span>
                <span className="mono" style={{ marginLeft: "auto", fontSize: 12.5 }}>
                  {fmtMoney(value, CURRENCY)}
                </span>
              </div>
              <div
                style={{
                  height: 6,
                  borderRadius: 3,
                  width: `${barPct}%`,
                  background: positive ? "var(--profit)" : "var(--loss)",
                  opacity: 0.7,
                }}
              />
            </div>
          );
        })}
      </div>
    </div>
  );
}

export function WealthCashView({ dict, writable, onUnauthorized }: Props) {
  const [cash, setCash] = useState<WealthCash | null>(null);
  const [error, setError] = useState(false);
  const [refreshSignal, setRefreshSignal] = useState(0);
  const [showAddForm, setShowAddForm] = useState(false);

  useEffect(() => {
    setError(false);
    fetchWealthCash()
      .then(setCash)
      .catch(() => setError(true));
  }, [refreshSignal]);

  async function pause(item: CashflowItem) {
    try {
      await deactivateWealthCashflow(item.id);
      setRefreshSignal((n) => n + 1);
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        onUnauthorized(() => pause(item));
      } else {
        window.alert(e instanceof ApiError ? e.message : dict.error);
      }
    }
  }

  if (error) {
    return <div className="error-message">{dict.error}</div>;
  }

  const inItems = (cash?.items ?? [])
    .filter((i) => i.active && i.direction === "in" && i.valueTwd != null)
    .sort((a, b) => (b.valueTwd ?? 0) - (a.valueTwd ?? 0));
  const outItems = (cash?.items ?? [])
    .filter((i) => i.active && i.direction === "out" && i.valueTwd != null)
    .sort((a, b) => (b.valueTwd ?? 0) - (a.valueTwd ?? 0));
  const maxRow = Math.max(inItems[0]?.valueTwd ?? 0, outItems[0]?.valueTwd ?? 0);

  return (
    <>
      {/* Unboxed header + "+" trigger, matching the design template's isWCash
          row exactly (Argus Trading WebUI.dc.html lines 757-760) — unlike
          /w/goals, this page's template DOES want a write affordance here,
          it just opens a shared drawer component this app has never built;
          toggling the existing inline form is the lazy substitute already
          used by every other wealth page's "+"-less version of this. */}
      <div style={{ display: "flex", alignItems: "center", gap: 12, margin: "0 0 16px", flexWrap: "wrap" }}>
        <span style={{ fontFamily: "var(--font-mono)", textTransform: "uppercase", letterSpacing: ".08em", fontSize: 11, color: "var(--ink)" }}>
          {dict.navWealthCash}
        </span>
        {writable && (
          <button className="btn-tint" style={{ marginLeft: "auto" }} onClick={() => setShowAddForm((v) => !v)}>
            + {dict.wealthCashAdd}
          </button>
        )}
      </div>

      {writable && showAddForm && (
        <AddCashflowForm
          dict={dict}
          onUnauthorized={onUnauthorized}
          onSaved={() => {
            setShowAddForm(false);
            setRefreshSignal((n) => n + 1);
          }}
        />
      )}

      <div style={{ display: "flex", flexWrap: "wrap", gap: 16, marginBottom: 16 }}>
        <div className="card card--glow" style={{ flex: "1.5 1 250px" }}>
          <div className="eyebrow">{dict.wealthCashMonthlyNet}</div>
          <div
            className={`mono ${cash?.monthlyNet != null && cash.monthlyNet < 0 ? "loss" : "profit"}`}
            style={{ fontSize: 40, lineHeight: 1.1, marginTop: 8 }}
          >
            {cash?.monthlyNet != null ? fmtMoney(cash.monthlyNet, CURRENCY) : "—"}
          </div>
          <div style={{ display: "flex", gap: 22, marginTop: 12, fontFamily: "var(--font-mono)", fontSize: 11.5, color: "var(--ink-3)", flexWrap: "wrap" }}>
            <span>
              {dict.wealthCashMonthlyIn} <span style={{ color: "var(--ink)" }}>{cash?.monthlyIn != null ? fmtMoney(cash.monthlyIn, CURRENCY) : "—"}</span>
            </span>
            <span>
              {dict.wealthCashMonthlyOut} <span style={{ color: "var(--ink)" }}>{cash?.monthlyOut != null ? fmtMoney(cash.monthlyOut, CURRENCY) : "—"}</span>
            </span>
          </div>
        </div>
        <div className="card" style={{ flex: "1 1 200px" }}>
          <div className="eyebrow">{dict.wealthCashSaveRateLabel}</div>
          <div className="mono profit" style={{ fontSize: "clamp(17px,2.1vw,28px)", marginTop: 8 }}>
            {cash?.saveRatePct != null ? `${cash.saveRatePct.toFixed(1)}%` : "—"}
          </div>
          <div style={{ fontSize: 11, color: "var(--ink-3)", marginTop: 6 }}>
            {dict.wealthCashDcaShareLabel} {cash?.dcaSharePct != null ? `${cash.dcaSharePct.toFixed(1)}%` : "—"}
          </div>
        </div>
        <div className="card" style={{ flex: "1 1 200px" }}>
          <div className="eyebrow">{dict.wealthCashFixedShareLabel}</div>
          <div className="mono" style={{ fontSize: "clamp(17px,2.1vw,28px)", marginTop: 8 }}>
            {cash?.fixedSharePct != null ? `${cash.fixedSharePct.toFixed(1)}%` : "—"}
          </div>
          <div style={{ fontSize: 11, color: "var(--ink-3)", marginTop: 6 }}>
            {dict.wealthCashAnnualNetLabel} {cash?.annualNet != null ? fmtMoney(cash.annualNet, CURRENCY) : "—"}
          </div>
        </div>
      </div>

      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit,minmax(240px,1fr))", gap: 16, marginBottom: 16 }}>
        <BreakdownCard title={dict.wealthCashInBreakdownTitle} items={inItems} total={cash?.monthlyIn ?? 0} maxValue={maxRow} positive />
        <BreakdownCard title={dict.wealthCashOutBreakdownTitle} items={outItems} total={cash?.monthlyOut ?? 0} maxValue={maxRow} positive={false} />
      </div>

      <div className="card" style={{ marginBottom: 16 }}>
        <div style={{ display: "flex", alignItems: "baseline", gap: 12, marginBottom: 4, flexWrap: "wrap" }}>
          <span className="eyebrow">{dict.wealthCashEventsTitle}</span>
          <span style={{ marginLeft: "auto", fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--ink-3)" }}>
            {dict.wealthCashNet90Label}{" "}
            <span className={cash?.eventsNet != null && cash.eventsNet < 0 ? "loss" : "profit"}>
              {cash?.eventsNet != null ? fmtMoney(cash.eventsNet, CURRENCY) : "—"}
            </span>
          </span>
        </div>
        {cash && cash.events.length > 0 ? (
          <table className="mono">
            <thead>
              <tr>
                <th>{dict.wealthCashDateLabel}</th>
                <th>{dict.wealthCashNameLabel}</th>
                <th>{dict.wealthVenueLabel}</th>
                <th>{dict.wealthCashAmountLabel}</th>
              </tr>
            </thead>
            <tbody>
              {cash.events.map((e, i) => (
                <tr key={`${e.date}-${e.item}-${i}`}>
                  <td>{e.date}</td>
                  <td style={{ fontFamily: "var(--sans)" }}>{e.item}</td>
                  <td style={{ fontFamily: "var(--sans)", fontSize: 12, color: "var(--ink-3)" }}>{e.venue || "—"}</td>
                  <td className={e.direction === "in" ? "profit" : e.direction === "out" ? "loss" : undefined}>
                    {e.amount != null ? `${e.direction === "in" ? "+" : "-"}${fmtMoney(e.amount, e.currency || CURRENCY)}` : "—"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        ) : (
          <div className="empty-message">{dict.wealthCashNoEvents}</div>
        )}
      </div>

      {writable && (
        <details className="card">
          <summary style={{ cursor: "pointer", fontFamily: "var(--font-mono)", fontSize: 10.5, color: "var(--ink-3)" }}>
            {dict.wealthManageLabel}
          </summary>
          <div className="eyebrow" style={{ marginTop: 12 }}>
            {dict.wealthCashItemsTitle}
          </div>
          {cash && cash.items.length > 0 ? (
            <table className="mono">
              <thead>
                <tr>
                  <th>{dict.wealthCashNameLabel}</th>
                  <th>{dict.wealthCashCategoryLabel}</th>
                  <th>{dict.wealthCashAmountLabel}</th>
                  <th>{dict.wealthCashDayOfMonthLabel}</th>
                  <th>{dict.wealthVenueLabel}</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {cash.items.map((item) => (
                  <tr key={item.id} style={{ opacity: item.active ? 1 : 0.5 }}>
                    <td style={{ fontFamily: "var(--sans)" }}>
                      {item.name}
                      {!item.active && ` (${dict.wealthCashPaused})`}
                    </td>
                    <td style={{ fontFamily: "var(--sans)", fontSize: 12, color: "var(--ink-3)" }}>
                      {categoryLabel(dict, item.category)}
                    </td>
                    <td className={item.direction === "in" ? "profit" : "loss"}>
                      {item.direction === "in" ? "+" : "-"}
                      {fmtMoney(item.amount, CURRENCY)}
                    </td>
                    <td>{item.dayOfMonth ?? "—"}</td>
                    <td style={{ fontFamily: "var(--sans)", fontSize: 12, color: "var(--ink-3)" }}>{item.venue || "—"}</td>
                    <td>{item.active && <button onClick={() => pause(item)}>{dict.wealthCashPause}</button>}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          ) : (
            <div className="empty-message">{dict.wealthCashNoItems}</div>
          )}
        </details>
      )}
    </>
  );
}
