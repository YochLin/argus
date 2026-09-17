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
    <div className="card">
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

export function WealthCashView({ dict, writable, onUnauthorized }: Props) {
  const [cash, setCash] = useState<WealthCash | null>(null);
  const [error, setError] = useState(false);
  const [refreshSignal, setRefreshSignal] = useState(0);

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

  return (
    <>
      <div className="card card--glow" style={{ marginBottom: 16 }}>
        <div className="eyebrow">{dict.navWealthCash}</div>
        <div className="wealth-hero-row" style={{ marginTop: 8 }}>
          <div className="wealth-hero-block">
            <span className="eyebrow">{dict.wealthCashMonthlyIn}</span>
            <span className="wealth-hero-block-value profit">
              {cash?.monthlyIn != null ? fmtMoney(cash.monthlyIn, CURRENCY) : "—"}
            </span>
          </div>
          <div className="wealth-hero-block">
            <span className="eyebrow">{dict.wealthCashMonthlyOut}</span>
            <span className="wealth-hero-block-value loss">
              {cash?.monthlyOut != null ? fmtMoney(cash.monthlyOut, CURRENCY) : "—"}
            </span>
          </div>
          <div className="wealth-hero-block">
            <span className="eyebrow">{dict.wealthCashMonthlyNet}</span>
            <span className="wealth-hero-block-value big">
              {cash?.monthlyNet != null ? fmtMoney(cash.monthlyNet, CURRENCY) : "—"}
            </span>
          </div>
        </div>
      </div>

      {writable && <div style={{ marginBottom: 16 }}><AddCashflowForm dict={dict} onUnauthorized={onUnauthorized} onSaved={() => setRefreshSignal((n) => n + 1)} /></div>}

      <div className="card" style={{ marginBottom: 16 }}>
        <div className="eyebrow">{dict.wealthCashItemsTitle}</div>
        {cash && cash.items.length > 0 ? (
          <table className="mono">
            <thead>
              <tr>
                <th>{dict.wealthCashNameLabel}</th>
                <th>{dict.wealthCashCategoryLabel}</th>
                <th>{dict.wealthCashAmountLabel}</th>
                <th>{dict.wealthCashDayOfMonthLabel}</th>
                <th>{dict.wealthVenueLabel}</th>
                {writable && <th />}
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
                  {writable && (
                    <td>
                      {item.active && (
                        <button onClick={() => pause(item)}>{dict.wealthCashPause}</button>
                      )}
                    </td>
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        ) : (
          <div className="empty-message">{dict.wealthCashNoItems}</div>
        )}
      </div>

      <div className="card">
        <div className="eyebrow">{dict.wealthCashEventsTitle}</div>
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
    </>
  );
}
