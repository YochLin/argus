import { useEffect, useState } from "react";
import * as Dialog from "@radix-ui/react-dialog";
import {
  ApiError,
  createWealthCashflow,
  deactivateWealthCashflow,
  deleteWealthCashflow,
  fetchWealthCash,
  resumeWealthCashflow,
  type CashflowDirection,
  type CashflowItem,
  type CashForecastMonth,
  type WealthCash,
} from "../api";
import type { Dictionary } from "../i18n";
import { fmtMoney, mmdd } from "./WealthHomeView";
import { shortTWD } from "../currency";
import { useFlash } from "../flash";
import { ConfirmDialog } from "./ConfirmDialog";

interface Props {
  dict: Dictionary;
  writable: boolean;
  onUnauthorized: (retry: () => void) => void;
}

const CURRENCY = "NT$"; // display currency fixed to TWD, same known gap as WealthHomeView/WealthBalanceView

const isTWD = (currency?: string) => !currency || currency === "TWD";

// twdAmount is how every amount on this page is shown: converted to TWD (by the
// server, at today's rate), with the original in a tooltip when it was foreign.
// With no TWD value — no exchange rate to be had — it falls back to the original
// amount and says why in the tooltip, rather than guessing a number.
function twdAmount(dict: Dictionary, valueTwd: number | undefined, amount: number, currency?: string): { text: string; title?: string; priced: boolean } {
  const original = isTWD(currency) ? undefined : `${currency} ${amount.toLocaleString(undefined, { maximumFractionDigits: 0 })}`;
  if (valueTwd != null) return { text: fmtMoney(valueTwd, CURRENCY), title: original, priced: true };
  if (original) return { text: original, title: dict.wealthCashNoRate.replace("%s", currency ?? ""), priced: false };
  return { text: fmtMoney(amount, CURRENCY), priced: true };
}

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

// AddCashflowForm matches WealthHomeView.tsx's AddAssetModal shell (the
// design template's shared "quick-add drawer", Argus Trading WebUI.dc.html
// lines 3304-3438) — right-side sliding panel, same wealth-drawer-* classes
// — but skips AddAssetModal's step-1 kind picker: this button only ever
// creates one kind of thing (a recurring cash flow), so there's nothing to
// pick between. This used to be a plain .card toggled inline in the page
// flow instead of a real Dialog/drawer — a stopgap from before this
// component existed, now replaced so /w/cash's "+" matches every other
// wealth page with a write affordance (/w/balance) instead of being the one
// page where it behaves differently.
function AddCashflowForm({
  dict,
  onClose,
  onUnauthorized,
  onSaved,
}: {
  dict: Dictionary;
  onClose: () => void;
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

  const canSubmit = name.trim() !== "" && Number(amount) > 0;

  return (
    <Dialog.Root open onOpenChange={(open) => !open && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="wealth-drawer-overlay">
          <Dialog.Content className="wealth-drawer-panel" aria-describedby={undefined} onOpenAutoFocus={(e) => e.preventDefault()}>
            <div className="wealth-drawer-header">
              <Dialog.Title style={{ fontFamily: "var(--font-mono)", textTransform: "uppercase", letterSpacing: ".08em", fontSize: 11 }}>
                {dict.wealthCashAddTitle}
              </Dialog.Title>
              <Dialog.Close className="modal-close" style={{ marginLeft: "auto" }} aria-label="close">
                ×
              </Dialog.Close>
            </div>
            <div className="wealth-drawer-body">
              <div className="topbar-tabs" role="group" aria-label="direction">
                <button className={`topbar-tab${direction === "in" ? " active" : ""}`} onClick={() => changeDirection("in")}>
                  {dict.wealthCashDirectionIn}
                </button>
                <button className={`topbar-tab${direction === "out" ? " active" : ""}`} onClick={() => changeDirection("out")}>
                  {dict.wealthCashDirectionOut}
                </button>
              </div>

              <div className="wealth-drawer-field">
                <span className="wealth-drawer-field-label">
                  {dict.wealthCashNameLabel}
                  <span className="wealth-drawer-required">*</span>
                </span>
                <input value={name} onChange={(e) => setName(e.target.value)} autoFocus />
              </div>

              <div className="wealth-drawer-field">
                <span className="wealth-drawer-field-label">
                  {dict.wealthCashAmountLabel}
                  <span className="wealth-drawer-required">*</span>
                </span>
                <input className="mono" type="number" value={amount} onChange={(e) => setAmount(e.target.value)} />
              </div>

              <div className="wealth-drawer-field">
                <span className="wealth-drawer-field-label">{dict.wealthCashDayOfMonthLabel}</span>
                <input className="mono" type="number" min={1} max={31} value={dayOfMonth} onChange={(e) => setDayOfMonth(e.target.value)} />
              </div>

              <div className="wealth-drawer-field">
                <span className="wealth-drawer-field-label">{dict.wealthCashCategoryLabel}</span>
                <select value={category} onChange={(e) => setCategory(e.target.value)}>
                  {categories.map((c) => (
                    <option key={c} value={c}>
                      {categoryLabel(dict, c)}
                    </option>
                  ))}
                </select>
              </div>
              {error && <div className="error-message">{error}</div>}
            </div>
            <div className="wealth-drawer-footer">
              <button className="wealth-drawer-cancel" onClick={onClose}>
                {dict.cancel}
              </button>
              <button className="btn-primary" style={{ marginLeft: "auto" }} disabled={!canSubmit || submitting} onClick={submit}>
                {dict.wealthCashAdd}
              </button>
            </div>
          </Dialog.Content>
        </Dialog.Overlay>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

// BreakdownCard mirrors the template's income/expense breakdown cards
// (Argus Trading WebUI.dc.html lines 811-850, cashModel()'s inRows/outRows)
// — each active item sorted by its own TWD value, a bar sized relative to
// the largest item across BOTH cards (cashModel()'s maxRow), and a % of
// that column's own total. A writable page also gets each row's 暫停 button,
// revealed on hover/focus (design: r.pauseStyle).
function BreakdownCard({
  dict,
  title,
  items,
  total,
  maxValue,
  positive,
  onPause,
}: {
  dict: Dictionary;
  title: string;
  items: CashflowItem[];
  total: number | null; // null while some active flow can't be priced in TWD
  maxValue: number;
  positive: boolean;
  onPause?: (item: CashflowItem) => void;
}) {
  return (
    <div className="card">
      <div style={{ display: "flex", alignItems: "baseline", gap: 10, marginBottom: 14 }}>
        <span className="eyebrow">{title}</span>
        <span className={`mono ${positive ? "profit" : "loss"}`} style={{ marginLeft: "auto", fontSize: 13 }}>
          {total != null ? fmtMoney(total, CURRENCY) : "—"}
        </span>
      </div>
      <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
        {items.map((item) => {
          const amount = twdAmount(dict, item.valueTwd, item.amount, item.currency);
          const value = item.valueTwd ?? 0;
          const pct = total != null && total > 0 ? (value / total) * 100 : 0;
          const barPct = maxValue > 0 ? (value / maxValue) * 100 : 0;
          return (
            <div key={item.id} className="wealth-flow-row" style={{ display: "flex", flexDirection: "column", gap: 5 }}>
              <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                <span style={{ fontFamily: "var(--sans)", fontSize: 12.5 }}>{item.name}</span>
                <span className="mono" style={{ fontSize: 10, color: "var(--ink-3)" }}>
                  {amount.priced && total != null ? `${pct.toFixed(0)}%` : "—"}
                </span>
                <span className="mono" style={{ marginLeft: "auto", fontSize: 12.5 }} title={amount.title}>
                  {amount.text}
                </span>
                {onPause && (
                  <button
                    className="wealth-flow-pause"
                    title={dict.wealthCashPauseTitle}
                    aria-label={`${dict.wealthCashPause} ${item.name}`}
                    onClick={() => onPause(item)}
                  >
                    {dict.wealthCashPause}
                  </button>
                )}
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

// ForecastChart mirrors the template's 12-month forecast bars (Argus Trading
// WebUI.dc.html lines 991-1014, cashModel()'s wcm.forecast) — an up-area/
// baseline/down-area column per month so a negative net renders below the
// line instead of just a shorter positive bar. Every bar here is the same
// height since the backend projects a flat monthlyNet (see api.ts's
// CashForecastMonth doc comment) rather than fabricating the design mock's
// Taiwan-calendar lumps — still the same chart mechanism, just honest data.
function ForecastChart({ dict, forecast, annualNet }: { dict: Dictionary; forecast: CashForecastMonth[]; annualNet: number | null }) {
  const maxAbs = Math.max(1, ...forecast.map((f) => Math.abs(f.net)));
  return (
    <div className="card" style={{ marginBottom: 16 }}>
      <div style={{ display: "flex", alignItems: "baseline", gap: 12, marginBottom: 6, flexWrap: "wrap" }}>
        <span className="eyebrow">{dict.wealthCashForecastTitle}</span>
        <span style={{ marginLeft: "auto", fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--ink-3)" }}>
          {dict.wealthCashAnnualNetLabel}{" "}
          <span className={annualNet != null && annualNet < 0 ? "loss" : "profit"}>{annualNet != null ? shortTWD(annualNet) : "—"}</span>
        </span>
      </div>
      <div style={{ fontSize: 11, color: "var(--ink-3)", marginBottom: 16 }}>{dict.wealthCashForecastNote}</div>
      <div style={{ display: "flex", gap: 8, alignItems: "stretch" }}>
        {forecast.map((f) => {
          const up = f.net >= 0;
          const barPct = (Math.abs(f.net) / maxAbs) * 100;
          const monthIndex = Number(f.month.slice(5, 7)) - 1;
          return (
            <div key={f.month} style={{ flex: 1, display: "flex", flexDirection: "column", alignItems: "center", minWidth: 0 }}>
              <div style={{ height: 104, width: "100%", display: "flex", alignItems: "flex-end", justifyContent: "center" }}>
                {up && <span style={{ width: "62%", borderRadius: "3px 3px 0 0", height: `${barPct}%`, background: "var(--profit)", opacity: 0.5 }} />}
              </div>
              <div style={{ height: 1, width: "100%", background: "var(--border)" }} />
              <div style={{ height: 56, width: "100%", display: "flex", alignItems: "flex-start", justifyContent: "center" }}>
                {!up && <span style={{ width: "62%", borderRadius: "0 0 3px 3px", height: `${barPct}%`, background: "var(--loss)", opacity: 0.5 }} />}
              </div>
              <span className="mono" style={{ fontSize: 10, color: "var(--ink-3)", marginTop: 4 }}>
                {dict.months[monthIndex]}
              </span>
              <span className="mono" style={{ fontSize: 10, color: "var(--ink-2)", marginTop: 2 }}>
                {shortTWD(f.net)}
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
}

function PausedValue({ dict, item }: { dict: Dictionary; item: CashflowItem }) {
  const amount = twdAmount(dict, item.valueTwd, item.amount, item.currency);
  return (
    <span className="mono" style={{ fontSize: 12.5, color: "var(--ink-3)", textAlign: "right" }} title={amount.title}>
      {amount.text}
      {dict.wealthCashPerMonth}
    </span>
  );
}

// PausedCard mirrors the template's 已暫停 card (Argus Trading WebUI.dc.html's
// wcm.pausedRows): flows left out of the monthly totals and forecast, each with
// the day it was paused and the two ways out — 恢復, or 刪除 for good. Shown to
// a read-only visitor too, whose buttons only explain why nothing changes.
function PausedCard({
  dict,
  items,
  onResume,
  onDelete,
}: {
  dict: Dictionary;
  items: CashflowItem[];
  onResume: (item: CashflowItem) => void;
  onDelete: (item: CashflowItem) => void;
}) {
  return (
    <div className="card" style={{ marginBottom: 16, overflowX: "auto" }}>
      <div style={{ display: "flex", alignItems: "baseline", gap: 10, flexWrap: "wrap", marginBottom: 10 }}>
        <span className="eyebrow">{dict.wealthCashPaused}</span>
        <span className="mono" style={{ fontSize: 11, color: "var(--ink-3)" }}>
          {items.length}
        </span>
        <span style={{ fontSize: 12, color: "var(--ink-3)", textWrap: "pretty" }}>{dict.wealthCashPausedNote}</span>
      </div>
      <div style={{ minWidth: 520 }}>
        {items.map((item) => (
          <div
            key={item.id}
            style={{
              display: "grid",
              gridTemplateColumns: "minmax(140px,2fr) 52px minmax(110px,1fr) minmax(110px,1fr) auto",
              gap: 12,
              alignItems: "center",
              padding: "9px 10px",
              borderTop: "1px solid var(--border)",
            }}
          >
            <span style={{ fontSize: 12.5, color: "var(--ink-2)" }}>{item.name}</span>
            <span className="mono" style={{ fontSize: 10.5, color: item.direction === "in" ? "var(--profit)" : "var(--loss)" }}>
              {item.direction === "in" ? dict.wealthCashTagIn : dict.wealthCashTagOut}
            </span>
            <PausedValue dict={dict} item={item} />
            <span className="mono" style={{ fontSize: 11.5, color: "var(--ink-3)" }}>
              {item.pausedAt ? dict.wealthCashPausedSince.replace("%s", item.pausedAt) : ""}
            </span>
            <span style={{ display: "flex", gap: 6, justifyContent: "flex-end" }}>
              <button className="wealth-flow-resume" aria-label={`${dict.wealthCashResume} ${item.name}`} onClick={() => onResume(item)}>
                {dict.wealthCashResume}
              </button>
              <button className="wealth-flow-delete" aria-label={`${dict.wealthCashDelete} ${item.name}`} onClick={() => onDelete(item)}>
                {dict.wealthCashDelete}
              </button>
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}

export function WealthCashView({ dict, writable, onUnauthorized }: Props) {
  const [cash, setCash] = useState<WealthCash | null>(null);
  const [error, setError] = useState(false);
  const [refreshSignal, setRefreshSignal] = useState(0);
  const [showAddForm, setShowAddForm] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState<CashflowItem | null>(null);
  const flash = useFlash();

  useEffect(() => {
    setError(false);
    fetchWealthCash()
      .then(setCash)
      .catch(() => setError(true));
  }, [refreshSignal]);

  // One write: reload and say `done` on success; on a 401 sign in and retry; on
  // any other failure say why and reload anyway, so a stale list (another tab
  // already resumed or deleted this flow) catches up instead of staying wrong.
  async function mutate(op: () => Promise<unknown>, done: string): Promise<void> {
    try {
      await op();
      setRefreshSignal((n) => n + 1);
      flash(done);
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        onUnauthorized(() => void mutate(op, done));
        return;
      }
      flash(e instanceof ApiError ? e.message : dict.error, "error");
      setRefreshSignal((n) => n + 1);
    }
  }

  const named = (template: string, item: CashflowItem) => template.replace("%s", item.name);
  const pause = (item: CashflowItem) => mutate(() => deactivateWealthCashflow(item.id), named(dict.wealthFlashPaused, item));
  // Resume and delete are on screen read-only too (the design shows the paused
  // card to everyone), so they say why nothing happens instead of failing a 403.
  const resume = (item: CashflowItem) =>
    writable ? mutate(() => resumeWealthCashflow(item.id), named(dict.wealthFlashResumed, item)) : flash(dict.wealthRoNoChange);
  const askDelete = (item: CashflowItem) => (writable ? setConfirmDelete(item) : flash(dict.wealthRoNoChange));
  function confirmedDelete(item: CashflowItem) {
    setConfirmDelete(null);
    void mutate(() => deleteWealthCashflow(item.id), named(dict.wealthFlashDeleted, item));
  }

  if (error) {
    return <div className="error-message">{dict.error}</div>;
  }

  // A flow with no TWD value (no exchange rate) is still listed, last, in its own
  // currency — otherwise nothing on the page would say why the totals are "—".
  const inItems = (cash?.items ?? [])
    .filter((i) => i.active && i.direction === "in")
    .sort((a, b) => (b.valueTwd ?? 0) - (a.valueTwd ?? 0));
  const outItems = (cash?.items ?? [])
    .filter((i) => i.active && i.direction === "out")
    .sort((a, b) => (b.valueTwd ?? 0) - (a.valueTwd ?? 0));
  const maxRow = Math.max(inItems[0]?.valueTwd ?? 0, outItems[0]?.valueTwd ?? 0);
  // The design lists income before expense, each in the order they were added.
  const pausedItems = (cash?.items ?? [])
    .filter((i) => !i.active)
    .sort((a, b) => Number(a.direction === "out") - Number(b.direction === "out") || a.id - b.id);

  return (
    <>
      {/* Unboxed header + "+" trigger, matching the design template's isWCash
          row exactly (Argus Trading WebUI.dc.html lines 757-760) — opens the
          same drawer shell as /w/balance's AddAssetModal (AddCashflowForm
          below), so every wealth page's "+" behaves the same way now. */}
      <div style={{ display: "flex", alignItems: "center", gap: 12, margin: "16px 0", flexWrap: "wrap" }}>
        <span style={{ fontFamily: "var(--font-mono)", textTransform: "uppercase", letterSpacing: ".08em", fontSize: 11, color: "var(--ink)" }}>
          {dict.navWealthCash}
        </span>
        {writable && (
          <button className="btn-tint" style={{ marginLeft: "auto" }} onClick={() => setShowAddForm(true)}>
            + {dict.wealthCashAdd}
          </button>
        )}
      </div>

      {writable && showAddForm && (
        <AddCashflowForm
          dict={dict}
          onClose={() => setShowAddForm(false)}
          onUnauthorized={onUnauthorized}
          onSaved={() => {
            setShowAddForm(false);
            setRefreshSignal((n) => n + 1);
          }}
        />
      )}

      <div style={{ display: "flex", flexWrap: "wrap", gap: 16, marginBottom: 16 }}>
        {/* padding:19.2 matches the design's calc(var(--pad)*1.2) on this one
            card (Argus Trading WebUI.dc.html's wCashNet block) — same 1.2x
            hero-card padding already applied to /w/balance's net-worth card. */}
        <div className="card card--glow" style={{ flex: "1.5 1 250px", padding: 19.2 }}>
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
        <BreakdownCard
          dict={dict}
          title={dict.wealthCashInBreakdownTitle}
          items={inItems}
          total={cash?.monthlyIn ?? null}
          maxValue={maxRow}
          positive
          onPause={writable ? pause : undefined}
        />
        <BreakdownCard
          dict={dict}
          title={dict.wealthCashOutBreakdownTitle}
          items={outItems}
          total={cash?.monthlyOut ?? null}
          maxValue={maxRow}
          positive={false}
          onPause={writable ? pause : undefined}
        />
      </div>

      {pausedItems.length > 0 && (
        <PausedCard dict={dict} items={pausedItems} onResume={resume} onDelete={askDelete} />
      )}

      {cash && cash.forecast.length > 0 && <ForecastChart dict={dict} forecast={cash.forecast} annualNet={cash.annualNet} />}

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
              {cash.events.map((e, i) => {
                const amount = e.amount != null ? twdAmount(dict, e.valueTwd, e.amount, e.currency) : null;
                return (
                  <tr key={`${e.date}-${e.item}-${i}`}>
                    <td>{mmdd(e.date)}</td>
                    <td style={{ fontFamily: "var(--sans)" }}>{e.item}</td>
                    <td style={{ fontFamily: "var(--sans)", fontSize: 12, color: "var(--ink-3)" }}>{e.venue || "—"}</td>
                    <td className={e.direction === "in" ? "profit" : e.direction === "out" ? "loss" : undefined} title={amount?.title}>
                      {amount ? `${e.direction === "in" ? "+" : "-"}${amount.text}` : "—"}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        ) : (
          <div className="empty-message">{dict.wealthCashNoEvents}</div>
        )}
      </div>

      {confirmDelete && (
        <ConfirmDialog
          title={named(dict.wealthCashDeleteTitle, confirmDelete)}
          body={dict.wealthCashDeleteBody}
          okLabel={dict.wealthCashDelete}
          cancelLabel={dict.cancel}
          onCancel={() => setConfirmDelete(null)}
          onConfirm={() => confirmedDelete(confirmDelete)}
        />
      )}
    </>
  );
}
