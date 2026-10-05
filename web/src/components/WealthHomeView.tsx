import { useEffect, useState } from "react";
import * as Dialog from "@radix-ui/react-dialog";
import {
  ApiError,
  archiveWealthAsset,
  createWealthAsset,
  fetchWealthAlloc,
  fetchWealthAssets,
  fetchWealthBalance,
  fetchWealthHome,
  saveWealthAssetSnapshot,
  type AllocCategory,
  type AllocationModel,
  type AssetGroup,
  type AssetSide,
  type BalanceSheet,
  type LiabilityDetail,
  type WealthAlloc,
  type WealthAsset,
  type WealthHome,
} from "../api";
import type { Dictionary } from "../i18n";
import { convertTWD, shortTWD } from "../currency";
import { CATEGORY_COLOR, categoryLabel, loadModel } from "../wealthCategory";
import { useFlash } from "../flash";
import { ConfirmDialog } from "./ConfirmDialog";
import { WealthEmptyCard, WealthGuide } from "./WealthOnboarding";

interface Props {
  dict: Dictionary;
  writable: boolean;
  onUnauthorized: (retry: () => void) => void;
  onNavigate: (path: string) => void;
}

const GROUPS: AssetGroup[] = ["liquid", "growth", "income", "hard"];
// CURRENCIES mirrors the design spec's display-currency menu (§8.1) — the
// backend only converts USD/TWD so far (wealth_home.go), the rest are
// accepted as an asset's native currency and simply won't convert into the
// net-worth total yet (rendered "—" if that ever matters — see
// wealth_home.go's "degrade the whole metric" rule).
const CURRENCIES = ["TWD", "USD", "JPY", "EUR", "CNY"];

// Kind mirrors assets.CategoryOf's recognized type strings (§8.5) plus
// "loan" (liability) and "other" (legacy catch-all) — every kind but
// deposit/loan submits straight through as `type` with no extra fields,
// same as "other" always did, since none of these have a detail table yet
// (insurance_details/fund_details/bond_details are PR8/10/11, still ahead).
type Kind = "deposit" | "loan" | "insurance" | "fund" | "bond" | "estate" | "gold" | "crypto" | "pension" | "other";

const KINDS: Kind[] = ["deposit", "loan", "insurance", "fund", "bond", "estate", "gold", "crypto", "pension", "other"];

// KIND_DEFAULT_GROUP is changeKind's asset_group suggestion per kind — the
// group dropdown stays editable afterward, this just saves a click for the
// common case (§8.1: asset_group is still required, independent of the
// finer /w/alloc-only category split).
const KIND_DEFAULT_GROUP: Partial<Record<Kind, AssetGroup>> = {
  deposit: "liquid",
  loan: "hard",
  insurance: "income",
  fund: "growth",
  bond: "income",
  estate: "hard",
  gold: "hard",
  crypto: "growth",
  pension: "hard",
};

function kindLabel(dict: Dictionary, k: Kind): string {
  switch (k) {
    case "deposit":
      return dict.wealthKindDeposit;
    case "loan":
      return dict.wealthKindLoan;
    case "insurance":
      return dict.wealthKindInsurance;
    case "fund":
      return dict.wealthKindFund;
    case "bond":
      return dict.wealthKindBond;
    case "estate":
      return dict.wealthKindEstate;
    case "gold":
      return dict.wealthKindGold;
    case "crypto":
      return dict.wealthKindCrypto;
    case "pension":
      return dict.wealthKindPension;
    case "other":
      return dict.wealthKindOther;
  }
}

// typeLabel names an asset's stored `type` — the Kind list above plus
// credit_card (which the quick-add form files under "loan"), falling back to
// the raw string for anything else the CSV import may have written.
export function typeLabel(dict: Dictionary, type: string): string {
  if (type === "credit_card") return dict.wealthKindCreditCard;
  return (KINDS as string[]).includes(type) ? kindLabel(dict, type as Kind) : type;
}

export function groupLabel(dict: Dictionary, g: AssetGroup): string {
  switch (g) {
    case "liquid":
      return dict.wealthGroupLiquid;
    case "growth":
      return dict.wealthGroupGrowth;
    case "income":
      return dict.wealthGroupIncome;
    case "hard":
      return dict.wealthGroupHard;
  }
}

// currency is the caller's own prefix; "NT$" marks a TWD-denominated wealth
// value, the only kind the 顯示幣別 selector converts (foreign-currency rows
// like a USD cash event keep their own symbol).
export function fmtMoney(v: number, currency: string): string {
  if (currency === "NT$") {
    const c = convertTWD(v);
    return `${c.symbol}${c.value.toLocaleString(undefined, { maximumFractionDigits: 0 })}`;
  }
  return `${currency}${v.toLocaleString(undefined, { maximumFractionDigits: 0 })}`;
}

// twdAmount is how every wealth page shows an amount that may be foreign: the
// TWD value (converted by the server at today's rate, then by fmtMoney for the
// 顯示幣別), with the original in a tooltip. With no TWD value — no exchange rate
// to be had — it falls back to the original amount and says why in the tooltip,
// rather than guessing a number.
export function twdAmount(
  dict: Dictionary,
  valueTwd: number | null | undefined,
  amount: number,
  currency?: string,
): { text: string; title?: string; priced: boolean } {
  const foreign = !!currency && currency !== "TWD";
  const original = foreign ? `${currency} ${amount.toLocaleString(undefined, { maximumFractionDigits: 0 })}` : undefined;
  if (valueTwd != null) return { text: fmtMoney(valueTwd, "NT$"), title: original, priced: true };
  if (foreign) return { text: original!, title: dict.wealthNoRate.replace("%s", currency), priced: false };
  return { text: fmtMoney(amount, "NT$"), priced: true };
}

// AssetValue is an asset's current value for a list: TWD, the original in a
// tooltip (see twdAmount), "—" while it has no value yet.
export function AssetValue({ dict, asset }: { dict: Dictionary; asset: WealthAsset }) {
  if (asset.value == null) return <>—</>;
  const v = twdAmount(dict, asset.valueTwd, asset.value, asset.currency);
  return <span title={v.title}>{v.text}</span>;
}

// groupColorClass maps the four asset_group buckets onto the app's generic
// four-way series palette (--s1..--s4, theme-invariant) for the allocation
// bar/legend/dots — matches the design mock's per-group dot/segment color,
// which isn't itself specified beyond "give each category its own color."
const GROUP_COLOR_CLASS: Record<AssetGroup, string> = {
  liquid: "s1",
  growth: "s2",
  income: "s3",
  hard: "s4",
};

export function groupColorClass(g: AssetGroup): string {
  return GROUP_COLOR_CLASS[g];
}

// liabilityNote renders a liability's "剩 N 年 · 月付 NT$X" line (design mock's
// l.term/l.pay), or "建議優先清償" for a credit-card balance with no term set
// — shared by the home page's off-target list and WealthBalanceView's
// liability rows so the wording/rounding stays in exactly one place.
export function liabilityNote(dict: Dictionary, currency: string, l: LiabilityDetail): string {
  const term =
    l.type === "credit_card" && l.remainingMonths == null
      ? dict.wealthHomePayFirst
      : l.remainingMonths != null
        ? dict.wealthHomeYearsLeft.replace("%s", String(Math.round(l.remainingMonths / 12)))
        : "";
  const pay = l.minPayment ? dict.wealthHomeMonthlyPay.replace("%s", fmtMoney(l.minPayment, currency)) : "";
  return [term, pay].filter(Boolean).join(" · ");
}

// The two home variants (design template's wVariant "a"/"b") share one data
// model; which is showing is a per-browser preference like the theme.
type Variant = "a" | "b";
const VARIANT_KEY = "argus.wealth.homeVariant";

function loadVariant(): Variant {
  try {
    return localStorage.getItem(VARIANT_KEY) === "b" ? "b" : "a";
  } catch {
    return "a";
  }
}

const CARD_BASE: React.CSSProperties = { marginBottom: 16 };
export const MONO_LABEL: React.CSSProperties = {
  fontFamily: "var(--font-mono)",
  textTransform: "uppercase",
  letterSpacing: ".08em",
  fontSize: 11,
};
export const DOTTED: React.CSSProperties = { borderBottom: "1px dotted currentColor", cursor: "help" };

// mmdd formats a "YYYY-MM-DD" (or "YYYY-MM") date string as "MM/DD" (or
// "MM"), the design template's short date style for anything inside a
// 90-day/12-month window where the year is implied.
export function mmdd(iso: string): string {
  return iso.slice(5).replace("-", "/");
}

function driftColor(thin: boolean, d: number): string {
  return thin || Math.abs(d) < 2 ? "var(--ink-3)" : d > 0 ? "var(--loss)" : "var(--profit)";
}

function fmtDrift(d: number): string {
  return `${d >= 0 ? "+" : "−"}${Math.abs(d).toFixed(1)}`;
}

interface HomeRow {
  category: AllocCategory;
  name: string;
  color: string;
  cur: number;
  target: number;
  drift: number;
  value: number;
  action: string;
  actionColor: string;
}

export function WealthHomeView({ dict, writable, onUnauthorized, onNavigate }: Props) {
  const [model] = useState<AllocationModel>(loadModel);
  const [variant, setVariant] = useState<Variant>(loadVariant);
  const [home, setHome] = useState<WealthHome | null>(null);
  const [alloc, setAlloc] = useState<WealthAlloc | null>(null);
  const [balance, setBalance] = useState<BalanceSheet | null>(null);
  const [assets, setAssets] = useState<WealthAsset[] | null>(null);
  const [error, setError] = useState(false);
  const [refreshSignal, setRefreshSignal] = useState(0);
  const [showAdd, setShowAdd] = useState(false);
  const [editing, setEditing] = useState<WealthAsset | null>(null);
  const [archiving, setArchiving] = useState<WealthAsset | null>(null);
  const flash = useFlash();

  useEffect(() => {
    setError(false);
    fetchWealthHome(model)
      .then(setHome)
      .catch(() => setError(true));
    fetchWealthAlloc(model)
      .then(setAlloc)
      .catch(() => setError(true));
    fetchWealthBalance()
      .then(setBalance)
      .catch(() => setError(true));
  }, [model, refreshSignal]);

  useEffect(() => {
    fetchWealthAssets()
      .then((r) => setAssets(r.assets))
      .catch(() => setError(true));
  }, [refreshSignal]);

  function refresh() {
    setRefreshSignal((n) => n + 1);
  }

  function pickVariant(v: Variant) {
    setVariant(v);
    try {
      localStorage.setItem(VARIANT_KEY, v);
    } catch {
      // per-browser convenience only; nothing to do if storage is blocked
    }
  }

  async function doArchive(a: WealthAsset) {
    try {
      await archiveWealthAsset(a.id);
      refresh();
      flash(dict.wealthFlashArchived.replace("%s", a.name));
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        onUnauthorized(() => doArchive(a));
      } else {
        flash(e instanceof ApiError ? e.message : dict.error, "error");
      }
    }
  }

  if (error) {
    return <div className="error-message">{dict.error}</div>;
  }

  const currency = "NT$"; // converted for display by fmtMoney (顯示幣別)
  // Nothing recorded (archived ones don't count): the page is a one-line card
  // and the setup guide instead of a screen of "—".
  const empty = assets != null && assets.length === 0;

  // --- derived, all from data the three endpoints already send ------------
  const assetCount = assets ? assets.filter((a) => a.side === "asset").length : 3;
  const thin = assetCount < 3 || !alloc || alloc.allocation.length === 0;
  const total = alloc?.totalAssets ?? 0;
  const rows: HomeRow[] = (alloc?.allocation ?? []).map((r) => {
    const drift = r.deviationPt;
    const reb = (drift / 100) * total;
    const ok = Math.abs(drift) < 2;
    return {
      category: r.category,
      name: categoryLabel(dict, r.category),
      color: CATEGORY_COLOR[r.category],
      cur: r.currentPct,
      target: r.targetPct,
      drift,
      value: r.marketValue,
      action: thin
        ? "—"
        : ok
          ? dict.wealthHomeActionOk
          : (drift > 0 ? dict.wealthHomeActionTrim : dict.wealthHomeActionAdd).replace("%s", shortTWD(Math.abs(reb))),
      actionColor: ok ? "var(--ink-3)" : "var(--ink)",
    };
  });
  const off = thin ? [] : rows.filter((r) => Math.abs(r.drift) >= 2).sort((a, b) => Math.abs(b.drift) - Math.abs(a.drift));
  const driftN = off.length;
  const heroDriftColor = thin ? "var(--ink-3)" : driftN > 2 ? "var(--loss)" : driftN > 0 ? "#f59e0b" : "var(--profit)";
  const rebalance = thin ? "—" : shortTWD(off.reduce((s, r) => s + (Math.abs(r.drift) / 100) * total, 0) / 2);
  const thinReason = thin ? dict.wealthHomeNeedAssets.replace("%s", String(assetCount)) : "";
  const groupOrder: AssetGroup[] = ["growth", "income", "hard", "liquid"];
  // 大類配置 is the share of total *assets* per asset_group (balance's
  // pctOfAssets), against the model's group target; home.allocation itself
  // is net-worth based, so a liability filed under a group would skew it.
  const groups = groupOrder
    .map((g) => {
      const target = home?.allocation.find((r) => r.group === g)?.targetPct;
      const cur = balance?.assetGroups.find((r) => r.group === g)?.pctOfAssets;
      return target != null && cur != null ? { group: g, cur, target, drift: cur - target } : null;
    })
    .filter((r): r is { group: AssetGroup; cur: number; target: number; drift: number } => !!r);
  const liabTotal = balance?.totalLiabilities ?? 0;
  const liabs = [...(balance?.liabilities ?? [])].sort((a, b) => b.valueTwd - a.valueTwd).map((l) => {
    return {
      key: l.assetId,
      name: l.name,
      rate: l.ratePct != null ? `${l.ratePct}%` : "—",
      value: fmtMoney(l.valueTwd, currency),
      share: liabTotal > 0 ? (l.valueTwd / liabTotal) * 100 : 0,
      note: liabilityNote(dict, currency, l),
    };
  });
  const debtRatio = balance?.debtRatioPct != null ? `${balance.debtRatioPct.toFixed(1)}%` : "—";
  const liquidMonths = balance?.liquidityMonths != null ? dict.wealthHomeMonths.replace("%s", balance.liquidityMonths.toFixed(1)) : "—";
  const pctText = (v: number | null | undefined) => (v != null ? `${v >= 0 ? "+" : ""}${v.toFixed(1)}%` : "—");
  const signColor = (v: number | null | undefined) => (v == null ? "var(--ink-3)" : v >= 0 ? "var(--profit)" : "var(--loss)");

  const bar = (r: HomeRow, h: number, radius: number) => (
    <span style={{ position: "relative", display: "block" }}>
      <span style={{ display: "block", height: h, borderRadius: radius, width: `${Math.min(r.cur * 2.2, 100).toFixed(1)}%`, background: r.color }} />
      <span style={{ position: "absolute", top: -2, bottom: -2, width: 2, borderRadius: 1, background: "var(--ink-2)", left: `${Math.min(r.target * 2.2, 100).toFixed(1)}%` }} />
    </span>
  );
  const dot = (color: string) => <span style={{ width: 8, height: 8, borderRadius: 2, flexShrink: 0, background: color, display: "inline-block" }} />;
  const segBar = (h: number, radius: number, mt?: number) => (
    <div style={{ display: "flex", height: h, borderRadius: radius, overflow: "hidden", background: "var(--bg)", marginTop: mt }}>
      {rows.map((r) => (
        <div key={r.category} style={{ width: `${r.cur.toFixed(2)}%`, background: r.color, height: "100%" }} />
      ))}
    </div>
  );
  const nameCell = (r: HomeRow) => (
    <span style={{ display: "flex", alignItems: "center", gap: 8 }}>
      {dot(r.color)}
      {r.category === "equity" ? (
        <a
          href="/"
          onClick={(e) => {
            e.preventDefault();
            onNavigate("/");
          }}
          style={{ textDecoration: "none", color: "var(--accent)", cursor: "pointer" }}
        >
          {r.name}
        </a>
      ) : (
        <span style={{ color: "var(--ink)" }}>{r.name}</span>
      )}
    </span>
  );
  const liabList = (valueTabular: boolean) =>
    liabs.map((l) => (
      <div key={l.key} style={{ display: "flex", flexDirection: "column", gap: 5 }}>
        <div style={{ display: "flex", alignItems: "baseline", gap: 8 }}>
          <span style={{ fontSize: 12.5 }}>{l.name}</span>
          <span style={{ fontFamily: "var(--font-mono)", fontSize: 10.5, color: "var(--ink-3)" }}>
            {dict.wealthHomeRate} {l.rate}
          </span>
          <span
            style={{
              marginLeft: "auto",
              fontFamily: "var(--font-mono)",
              fontSize: 12.5,
              color: "var(--loss)",
              fontVariantNumeric: valueTabular ? "tabular-nums" : undefined,
            }}
          >
            {l.value}
          </span>
        </div>
        <div style={{ height: 6, borderRadius: 3, background: "var(--loss)", opacity: 0.6, width: `${l.share.toFixed(1)}%` }} />
        <div style={{ fontSize: 10.5, color: "var(--ink-3)" }}>{l.note}</div>
      </div>
    ));
  const kpiCard = (label: React.ReactNode, value: string, opts?: { color?: string; note?: string }) => (
    <div className="card" style={{ margin: 0 }}>
      <div style={MONO_LABEL}>{label}</div>
      <div style={{ fontFamily: "var(--font-mono)", fontSize: "clamp(17px,2.1vw,28px)", fontVariantNumeric: "tabular-nums", marginTop: 8, color: opts?.color }}>{value}</div>
      {opts?.note !== undefined && <div style={{ fontSize: 11, color: "var(--ink-3)", marginTop: 6 }}>{opts.note}</div>}
    </div>
  );

  return (
    <>
      <div style={{ display: "flex", alignItems: "center", gap: 12, margin: "16px 0", flexWrap: "wrap" }}>
        <span style={{ ...MONO_LABEL, color: "var(--ink)" }}>{dict.navWealth}</span>
        <span style={{ marginLeft: "auto", fontFamily: "var(--font-mono)", fontSize: 10, letterSpacing: ".06em", color: "var(--ink-3)" }}>
          {dict.wealthHomeVariantLabel}
        </span>
        <div style={{ display: "flex", gap: 6 }}>
          {(["a", "b"] as Variant[]).map((v) => (
            <button key={v} className={`home-variant-btn${variant === v ? " active" : ""}`} onClick={() => pickVariant(v)}>
              {v === "a" ? dict.wealthHomeVariantBoard : dict.wealthHomeVariantStory}
            </button>
          ))}
        </div>
        {writable && (
          <button className="home-add-btn" onClick={() => setShowAdd(true)}>
            <span style={{ fontSize: 14, lineHeight: 1 }}>＋</span>
            {dict.wealthHomeAddNew}
          </button>
        )}
      </div>

      {empty && <WealthEmptyCard dict={dict} line={dict.wealthEmptyNet} onAdd={writable ? () => setShowAdd(true) : undefined} />}
      {assets && <WealthGuide dict={dict} hasAssets={!empty} onNavigate={onNavigate} />}

      {!empty && (
        <>
          {home && home.staleCount > 0 && (
            <div
              style={{
                display: "flex",
                alignItems: "center",
                gap: 12,
                flexWrap: "wrap",
                padding: "11px 14px",
                borderRadius: 10,
                background: "rgba(245,158,11,.1)",
                border: "1px solid rgba(245,158,11,.35)",
                marginBottom: 16,
              }}
            >
              <span style={{ fontSize: 12.5, color: "#f59e0b" }}>{dict.wealthHomeStaleBanner.replace("%s", String(home.staleCount))}</span>
              <a
                href="/w/balance"
                onClick={(e) => {
                  e.preventDefault();
                  onNavigate("/w/balance");
                }}
                style={{ marginLeft: "auto", fontFamily: "var(--font-mono)", fontSize: 11.5, color: "#f59e0b", textDecoration: "none" }}
              >
                {dict.wealthHomeStaleGo}
              </a>
            </div>
          )}

          {variant === "a" ? (
            <>
              <div className="card card--glow" style={{ ...CARD_BASE, padding: 16 * 1.3, display: "flex", gap: 16 * 1.6, flexWrap: "wrap", alignItems: "center" }}>
                <div style={{ flex: "0 1 auto", minWidth: 0 }}>
                  <div style={MONO_LABEL}>
                    <span title={dict.wealthHomeTipDrift} style={DOTTED}>{dict.wealthHomeDriftLabel}</span>
                  </div>
                  <div style={{ fontFamily: "var(--font-mono)", fontSize: 44, lineHeight: 1.1, marginTop: 10, wordBreak: "keep-all", color: heroDriftColor }}>
                    {thin ? "—" : driftN === 0 ? dict.wealthHomeDriftOnTarget : dict.wealthHomeDriftCount.replace("%s", String(driftN))}
                  </div>
                  <div style={{ fontSize: 13, color: "var(--ink-2)", marginTop: 4, wordBreak: "keep-all" }}>
                    {thin ? thinReason : driftN === 0 ? dict.wealthHomeDriftNoRebal : dict.wealthHomeDriftOff}
                  </div>
                </div>
                <div style={{ flex: "1 1 260px", minWidth: 0, display: "flex", flexDirection: "column", gap: 12 }}>
                  <div style={{ display: "flex", gap: 24, flexWrap: "wrap", fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--ink-3)" }}>
                    <span>
                      <span title={dict.wealthHomeTipRebal} style={DOTTED}>{dict.wealthHomeRebalanceLabel}</span>{" "}
                      <span style={{ color: "var(--ink)" }}>{rebalance}</span>
                    </span>
                    <span>
                      {dict.wealthHomeNetWorth}{" "}
                      <span style={{ color: "var(--ink)" }}>{home?.netWorth != null ? fmtMoney(home.netWorth, currency) : "—"}</span>
                    </span>
                  </div>
                  {segBar(10, 5)}
                </div>
              </div>

              <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit,minmax(45%,1fr))", gap: 16, marginBottom: 16 }}>
                {kpiCard(dict.wealthTotalAssets, home?.totalAssets != null ? fmtMoney(home.totalAssets, currency) : "—")}
                {kpiCard(dict.wealthTotalLiabilities, home?.totalLiabilities != null ? fmtMoney(home.totalLiabilities, currency) : "—", { color: "var(--loss)" })}
                {kpiCard(<span title={dict.wealthHomeTipDebt} style={DOTTED}>{dict.wealthDebtRatio}</span>, debtRatio, { note: "" })}
                {kpiCard(<span title={dict.wealthHomeTipLiquid} style={DOTTED}>{dict.wealthHomeLiquidLabel}</span>, liquidMonths, { note: "" })}
              </div>

              <div className="card" style={{ ...CARD_BASE, overflowX: "auto" }}>
                <div style={{ ...MONO_LABEL, marginBottom: 4 }}>{dict.wealthHomeAllocTitle}</div>
                <table className="mono" style={{ width: "100%" }}>
                  <thead>
                    <tr>
                      <th>{dict.wealthHomeClass}</th>
                      <th style={{ width: "26%" }} />
                      <th>{dict.wealthHomeCurrent}</th>
                      <th>{dict.wealthHomeTarget}</th>
                      <th>{dict.wealthHomeDrift}</th>
                      <th>{dict.wealthHomeAction}</th>
                      <th>{dict.wealthMarketValue}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {rows.map((r) => (
                      <tr key={r.category}>
                        <td style={{ fontFamily: "var(--font-sans)" }}>{nameCell(r)}</td>
                        <td>{bar(r, 8, 4)}</td>
                        <td>{r.cur.toFixed(1)}%</td>
                        <td style={{ color: "var(--ink-3)" }}>{r.target}%</td>
                        <td style={{ color: driftColor(thin, r.drift) }}>{fmtDrift(r.drift)}pt</td>
                        <td style={{ fontFamily: "var(--font-sans)", fontSize: 12, color: r.actionColor }}>{r.action}</td>
                        <td>{fmtMoney(r.value, currency)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>

              <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit,minmax(240px,1fr))", gap: 16, marginBottom: 16 }}>
                <div className="card" style={{ margin: 0 }}>
                  <div style={{ ...MONO_LABEL, marginBottom: 10 }}>{dict.wealthHomeGroupTitle}</div>
                  <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
                    {groups.map((g) => (
                      <div key={g.group} style={{ display: "flex", alignItems: "baseline", gap: 10 }}>
                        <span style={{ fontSize: 12.5, color: "var(--ink)", width: 72, flexShrink: 0 }}>{groupLabel(dict, g.group)}</span>
                        <span style={{ fontFamily: "var(--font-mono)", fontSize: 13, fontVariantNumeric: "tabular-nums" }}>{g.cur.toFixed(1)}%</span>
                        <span style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--ink-3)" }}>/ {g.target}%</span>
                        <span style={{ marginLeft: "auto", fontFamily: "var(--font-mono)", fontSize: 12, color: driftColor(thin, g.drift) }}>{fmtDrift(g.drift)}pt</span>
                      </div>
                    ))}
                  </div>
                </div>
                <div className="card" style={{ margin: 0 }}>
                  <div style={{ ...MONO_LABEL, marginBottom: 10 }}>{dict.wealthHomeLiabTitle}</div>
                  <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>{liabList(true)}</div>
                </div>
              </div>
            </>
          ) : (
            <>
              <div className="card card--glow" style={{ ...CARD_BASE, padding: 16 * 1.6 }}>
                <div style={{ ...MONO_LABEL, color: "var(--ink-3)" }}>{dict.wealthHomeNetWorth}</div>
                <div style={{ fontFamily: "var(--font-mono)", fontSize: 60, lineHeight: 1.05, fontVariantNumeric: "tabular-nums", marginTop: 8 }}>
                  {home?.netWorth != null ? fmtMoney(home.netWorth, currency) : "—"}
                </div>
                <div style={{ display: "flex", gap: 28, marginTop: 16, flexWrap: "wrap", fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--ink-3)" }}>
                  <span>
                    <span title={dict.wealthHomeTipYtd} style={DOTTED}>{dict.wealthHomeYtd}</span>{" "}
                    <span style={{ color: signColor(home?.ytdPct) }}>{pctText(home?.ytdPct)}</span>
                  </span>
                  <span>
                    {dict.wealthHomeMom} <span style={{ color: signColor(home?.momPct) }}>{pctText(home?.momPct)}</span>
                  </span>
                  <span>
                    {dict.wealthTotalAssets}{" "}
                    <span style={{ color: "var(--ink)" }}>{home?.totalAssets != null ? fmtMoney(home.totalAssets, currency) : "—"}</span>
                  </span>
                  <span>
                    {dict.wealthTotalLiabilities}{" "}
                    <span style={{ color: "var(--loss)" }}>{home?.totalLiabilities != null ? fmtMoney(home.totalLiabilities, currency) : "—"}</span>
                  </span>
                </div>
                {segBar(12, 6, 20)}
                <div style={{ display: "flex", flexWrap: "wrap", gap: 14, marginTop: 12 }}>
                  {rows.map((r) => (
                    <span key={r.category} style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 11, color: "var(--ink-3)" }}>
                      {dot(r.color)}
                      {r.name} <span style={{ fontFamily: "var(--font-mono)", color: "var(--ink-2)" }}>{r.cur.toFixed(1)}%</span>
                    </span>
                  ))}
                </div>
              </div>

              <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit,minmax(240px,1fr))", gap: 16, marginBottom: 16 }}>
                <div className="card" style={{ margin: 0 }}>
                  <div style={{ display: "flex", alignItems: "baseline", gap: 10, marginBottom: 12 }}>
                    <span style={MONO_LABEL}>{dict.wealthHomeOffTitle}</span>
                    <span style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: heroDriftColor }}>
                      {thin ? `${dict.wealthHomeThinShort} · ${thinReason}` : driftN === 0 ? dict.wealthHomeHeadOk : dict.wealthHomeDriftHead.replace("%s", String(driftN))}
                    </span>
                  </div>
                  <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
                    {off.slice(0, 4).map((r) => (
                      <div key={r.category} style={{ display: "flex", flexDirection: "column", gap: 5 }}>
                        <div style={{ display: "flex", alignItems: "baseline", gap: 8 }}>
                          {dot(r.color)}
                          <span style={{ fontSize: 12.5 }}>{r.name}</span>
                          <span style={{ marginLeft: "auto", fontFamily: "var(--font-mono)", fontSize: 12, color: driftColor(thin, r.drift) }}>{fmtDrift(r.drift)}pt</span>
                        </div>
                        {bar(r, 8, 4)}
                        <div style={{ fontSize: 11, color: "var(--ink-3)" }}>
                          {r.cur.toFixed(1)}% → {r.target}% · {r.action}
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
                <div className="card" style={{ margin: 0 }}>
                  <div style={{ ...MONO_LABEL, marginBottom: 12 }}>{dict.wealthHomeLiabTitle}</div>
                  <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
                    {liabList(false)}
                    <div style={{ display: "flex", alignItems: "baseline", gap: 10, paddingTop: 10, borderTop: "1px solid var(--border)" }}>
                      <span style={{ fontFamily: "var(--font-mono)", fontSize: 10.5, letterSpacing: ".06em", color: "var(--ink-3)" }}>
                        <span title={dict.wealthHomeTipDebt} style={DOTTED}>{dict.wealthDebtRatio}</span>
                      </span>
                      <span style={{ fontFamily: "var(--font-mono)", fontSize: 15 }}>{debtRatio}</span>
                      <span style={{ marginLeft: "auto", fontFamily: "var(--font-mono)", fontSize: 10.5, letterSpacing: ".06em", color: "var(--ink-3)" }}>
                        <span title={dict.wealthHomeTipLiquid} style={DOTTED}>{dict.wealthHomeLiquidLabel}</span>
                      </span>
                      <span style={{ fontFamily: "var(--font-mono)", fontSize: 15 }}>{liquidMonths}</span>
                    </div>
                  </div>
                </div>
              </div>

              <div className="card" style={{ ...CARD_BASE, overflowX: "auto" }}>
                <div style={{ ...MONO_LABEL, marginBottom: 4 }}>{dict.wealthHomeAllocTitle}</div>
                <table className="mono" style={{ width: "100%" }}>
                  <thead>
                    <tr>
                      <th>{dict.wealthHomeClass}</th>
                      <th>{dict.wealthHomeCurrent}</th>
                      <th>{dict.wealthHomeTarget}</th>
                      <th>{dict.wealthHomeDrift}</th>
                      <th>{dict.wealthMarketValue}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {rows.map((r) => (
                      <tr key={r.category}>
                        <td style={{ fontFamily: "var(--font-sans)" }}>{nameCell(r)}</td>
                        <td>{r.cur.toFixed(1)}%</td>
                        <td style={{ color: "var(--ink-3)" }}>{r.target}%</td>
                        <td style={{ color: driftColor(thin, r.drift) }}>{fmtDrift(r.drift)}pt</td>
                        <td>{fmtMoney(r.value, currency)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </>
          )}

          <div className="card">
            <div className="modal-header" style={{ border: "none", padding: 0, marginBottom: 12 }}>
              <div className="eyebrow">{dict.wealthAssetsLabel}</div>
            </div>
            {!assets ? (
              <div className="loading">{dict.loading}</div>
            ) : assets.length === 0 ? (
              <div className="empty-message">{dict.wealthEmpty}</div>
            ) : (
              <table className="mono">
                <thead>
                  <tr>
                    <th>{dict.wealthName}</th>
                    <th>{dict.wealthGroupLabel}</th>
                    <th>{dict.wealthVenue}</th>
                    <th>{dict.wealthValue}</th>
                    {writable && <th />}
                  </tr>
                </thead>
                <tbody>
                  {assets.map((a) => (
                    <tr key={a.id}>
                      <td>{a.name}</td>
                      <td>{groupLabel(dict, a.assetGroup)}</td>
                      <td>{a.venue || dict.wealthVenueUnset}</td>
                      <td className={a.side === "liability" ? "loss" : ""}>
                        <AssetValue dict={dict} asset={a} />
                      </td>
                      {writable && (
                        <td className="row-actions">
                          <button onClick={() => setEditing(a)}>{dict.wealthEditValueTitle}</button>
                          <button onClick={() => setArchiving(a)}>{dict.wealthArchive}</button>
                        </td>
                      )}
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </>
      )}

      {showAdd && (
        <AddAssetModal
          dict={dict}
          onClose={() => setShowAdd(false)}
          onSuccess={() => {
            setShowAdd(false);
            refresh();
          }}
          onUnauthorized={onUnauthorized}
          onNavigate={onNavigate}
        />
      )}
      {editing && (
        <EditValueModal
          dict={dict}
          asset={editing}
          onClose={() => setEditing(null)}
          onSuccess={() => {
            setEditing(null);
            refresh();
          }}
          onUnauthorized={onUnauthorized}
        />
      )}
      {archiving && (
        <ConfirmDialog
          title={dict.wealthArchiveTitle.replace("%s", archiving.name)}
          body={dict.wealthArchiveBody}
          okLabel={dict.wealthArchive}
          cancelLabel={dict.cancel}
          onCancel={() => setArchiving(null)}
          onConfirm={() => {
            const a = archiving;
            setArchiving(null);
            void doArchive(a);
          }}
        />
      )}
    </>
  );
}

// WealthEmptyAssets is the empty-page card of a page that has no add button of
// its own (/w/alloc, /w/retire): its "新增" opens the add-asset drawer.
export function WealthEmptyAssets({
  dict,
  line,
  writable,
  onUnauthorized,
  onAdded,
  onNavigate,
}: {
  dict: Dictionary;
  line: string;
  writable: boolean;
  onUnauthorized: (retry: () => void) => void;
  onAdded: () => void;
  onNavigate?: (path: string) => void;
}) {
  const [adding, setAdding] = useState(false);
  return (
    <>
      <WealthEmptyCard dict={dict} line={line} onAdd={writable ? () => setAdding(true) : undefined} />
      {adding && (
        <AddAssetModal
          dict={dict}
          onClose={() => setAdding(false)}
          onSuccess={() => {
            setAdding(false);
            onAdded();
          }}
          onUnauthorized={onUnauthorized}
          onNavigate={onNavigate}
        />
      )}
    </>
  );
}

// ASSET_KINDS is what step 1 lists as assets, in the design's order; the loan
// follows, then the three things this drawer hands over to their own page.
// (A bare "insurance" asset made here would have no coverage rows, so 保單 goes
// to /w/insure like the design's policy kind.)
const ASSET_KINDS: Kind[] = ["deposit", "fund", "bond", "estate", "gold", "crypto", "pension", "other"];

function kindNote(dict: Dictionary, k: Kind): string {
  switch (k) {
    case "deposit":
      return dict.wealthKindNoteDeposit;
    case "loan":
      return dict.wealthKindNoteLoan;
    case "fund":
      return dict.wealthKindNoteFund;
    case "bond":
      return dict.wealthKindNoteBond;
    case "estate":
      return dict.wealthKindNoteEstate;
    case "gold":
      return dict.wealthKindNoteGold;
    case "crypto":
      return dict.wealthKindNoteCrypto;
    case "pension":
      return dict.wealthKindNotePension;
    case "other":
      return dict.wealthKindNoteOther;
    default:
      return "";
  }
}

// classNote is the design's wClassNotes: what the chosen 分組 means and which
// balance-sheet / overview rows it lands in.
function classNote(dict: Dictionary, g: AssetGroup): string {
  return g === "liquid"
    ? dict.wealthClassNoteLiquid
    : g === "growth"
      ? dict.wealthClassNoteGrowth
      : g === "income"
        ? dict.wealthClassNoteIncome
        : dict.wealthClassNoteHard;
}

// similarName is the design's duplicate check: same name, or one contains the
// other (only once it's longer than 2 characters, so "貸" doesn't match "房貸").
function similarName(name: string, existing: string[]): string | undefined {
  const n = name.trim().toLowerCase();
  if (!n) return undefined;
  return existing.find((x) => {
    const l = x.toLowerCase();
    return l === n || (n.length > 2 && l.includes(n)) || (l.length > 2 && n.includes(l));
  });
}

// AddAssetModal follows the design template's shared "quick-add drawer"
// (Argus Trading WebUI.dc.html, dw.*): a right-side panel, step 1 picks a type
// (each with a one-line note), step 2 is that type's form with inline required
// errors, a duplicate-name warning and a footer hint. The design's four coarse
// kinds are this app's finer asset `type` taxonomy (Kind) instead — the type
// feeds the nine-way allocation split — and "side" is implied by which kind
// was picked, so there's no separate side selector. A loan has no 分組 chip:
// it keeps its default group, which only the net-worth drift reads.
// Handed over: 現金流項目, 保單 and CSV import each have their own page.
// Not here: the monthly-payment input (the app derives it from rate and
// remaining months, it isn't stored), rate type, secured-by with its LTV
// warning (no column yet), the recurring-contribution toggle, and the live
// FX-conversion and amount preview lines.
export function AddAssetModal({
  dict,
  onClose,
  onSuccess,
  onUnauthorized,
  onNavigate,
}: {
  dict: Dictionary;
  onClose: () => void;
  onSuccess: () => void;
  onUnauthorized: (retry: () => void) => void;
  onNavigate?: (path: string) => void;
}) {
  const [step, setStep] = useState<"pick" | "form">("pick");
  const [kind, setKind] = useState<Kind>("deposit");
  const [name, setName] = useState("");
  const [group, setGroup] = useState<AssetGroup>("liquid");
  const [venue, setVenue] = useState("");
  const [currency, setCurrency] = useState("TWD");
  const [initialValue, setInitialValue] = useState("");
  const [accountNote, setAccountNote] = useState("");
  const [lender, setLender] = useState("");
  const [ratePct, setRatePct] = useState("");
  const [originalPrincipal, setOriginalPrincipal] = useState("");
  const [remainingMonths, setRemainingMonths] = useState("");
  const [errors, setErrors] = useState<{ name?: string; value?: string }>({});
  const [existing, setExisting] = useState<WealthAsset[]>([]);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const flash = useFlash();

  useEffect(() => {
    fetchWealthAssets()
      .then((r) => setExisting(r.assets))
      .catch(() => {});
  }, []);

  function pickKind(next: Kind) {
    setKind(next);
    const defaultGroup = KIND_DEFAULT_GROUP[next];
    if (defaultGroup) {
      setGroup(defaultGroup);
    }
    setStep("form");
  }

  function hand(path: string) {
    onClose();
    onNavigate?.(path);
  }

  const side: AssetSide = kind === "loan" ? "liability" : "asset";
  const dup = similarName(
    name,
    existing.filter((a) => a.side === side).map((a) => a.name),
  );
  const blocked = !!(errors.name || errors.value);
  const picks: { key: string; label: string; note: string; go: () => void }[] = [
    ...ASSET_KINDS.map((k) => ({ key: k, label: kindLabel(dict, k), note: kindNote(dict, k), go: () => pickKind(k) })),
    { key: "loan", label: kindLabel(dict, "loan"), note: kindNote(dict, "loan"), go: () => pickKind("loan") },
    ...(onNavigate
      ? [
          { key: "flow", label: dict.wealthKindFlow, note: dict.wealthKindNoteFlow, go: () => hand("/w/cash") },
          { key: "policy", label: dict.wealthKindInsurance, note: dict.wealthKindNotePolicy, go: () => hand("/w/insure") },
          { key: "csv", label: dict.wealthKindImport, note: dict.wealthKindNoteImport, go: () => hand("/w/import") },
        ]
      : []),
  ];

  async function submit() {
    const e: typeof errors = {};
    if (!name.trim()) e.name = dict.wealthErrRequired;
    if (!(Number(initialValue) > 0)) e.value = dict.wealthErrAmount;
    setErrors(e);
    if (e.name || e.value) return;
    setSubmitting(true);
    setError(null);
    // One institution field for the user: a loan's lender, otherwise the venue
    // (which a deposit also files as its bank).
    const inst = (kind === "loan" ? lender : venue).trim() || undefined;
    try {
      await createWealthAsset({
        side,
        type: kind,
        name: name.trim(),
        assetGroup: group,
        venue: inst,
        currency,
        initialValue: Number(initialValue),
        deposit: kind === "deposit" ? { bank: inst, accountNote: accountNote.trim() || undefined } : undefined,
        loan:
          kind === "loan"
            ? {
                lender: inst,
                ratePct: ratePct ? Number(ratePct) : undefined,
                originalPrincipal: originalPrincipal ? Number(originalPrincipal) : undefined,
                remainingMonths: remainingMonths ? Number(remainingMonths) : undefined,
              }
            : undefined,
      });
      flash(`${name.trim()}${dict.wealthAddedToast}`);
      onSuccess();
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

  const bad = (on?: string): React.CSSProperties => (on ? { borderColor: "var(--loss)" } : {});
  const errTag = (on?: string) =>
    on && <span style={{ marginLeft: "auto", fontSize: 11, color: "var(--loss)" }}>{on}</span>;

  return (
    <Dialog.Root open onOpenChange={(open) => !open && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="wealth-drawer-overlay">
          <Dialog.Content className="wealth-drawer-panel" aria-describedby={undefined} onOpenAutoFocus={(e) => e.preventDefault()}>
            <div className="wealth-drawer-header">
              <Dialog.Title style={{ fontFamily: "var(--font-mono)", textTransform: "uppercase", letterSpacing: ".08em", fontSize: 11 }}>
                {step === "pick" ? dict.wealthAddTitle : kindLabel(dict, kind)}
              </Dialog.Title>
              <span className="wealth-drawer-step">{dict.wealthAddStep.replace("%s", step === "pick" ? "1" : "2")}</span>
              <Dialog.Close className="modal-close" style={{ marginLeft: "auto" }} aria-label="close">
                ×
              </Dialog.Close>
            </div>
            <div className="wealth-drawer-body">
              {step === "pick" ? (
                <div style={{ display: "flex", flexDirection: "column", gap: 9 }}>
                  <span style={{ fontSize: 12, color: "var(--ink-3)" }}>{dict.wealthAddPickHint}</span>
                  {picks.map((p) => (
                    <button key={p.key} className="wealth-kind-btn" onClick={p.go}>
                      <span style={{ display: "flex", flexDirection: "column", gap: 3, textAlign: "left" }}>
                        <span className="wealth-kind-btn-label">{p.label}</span>
                        <span style={{ fontSize: 11.5, color: "var(--ink-3)", fontWeight: 400 }}>{p.note}</span>
                      </span>
                      <span className="wealth-kind-btn-chevron">›</span>
                    </button>
                  ))}
                </div>
              ) : (
                <>
                  <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                    <span className="wealth-drawer-kind-tag">{kindLabel(dict, kind)}</span>
                    <button className="wealth-drawer-back" onClick={() => setStep("pick")}>
                      {dict.wealthAddChange}
                    </button>
                  </div>

                  <div className="wealth-drawer-field">
                    <span className="wealth-drawer-field-label">
                      {dict.wealthName}
                      <span className="wealth-drawer-required">*</span>
                      {errTag(errors.name)}
                    </span>
                    <input
                      value={name}
                      placeholder={side === "liability" ? dict.wealthPhLoanName : dict.wealthPhAssetName}
                      style={bad(errors.name)}
                      onChange={(e) => {
                        setName(e.target.value);
                        setErrors((p) => ({ ...p, name: undefined }));
                      }}
                      autoFocus
                    />
                  </div>

                  <div className="wealth-drawer-field">
                    <span className="wealth-drawer-field-label">
                      {side === "liability" ? dict.wealthFBalance : dict.wealthFValue}
                      <span className="wealth-drawer-required">*</span>
                      {errTag(errors.value)}
                    </span>
                    <div style={{ display: "flex", gap: 6, alignItems: "center" }}>
                      <input
                        className="mono"
                        type="number"
                        placeholder="0"
                        value={initialValue}
                        onChange={(e) => {
                          setInitialValue(e.target.value);
                          setErrors((p) => ({ ...p, value: undefined }));
                        }}
                        style={{ flex: 1, minWidth: 0, textAlign: "right", ...bad(errors.value) }}
                      />
                      <div className="wealth-chip-row" style={{ flexShrink: 0 }}>
                        {CURRENCIES.map((c) => (
                          <button
                            key={c}
                            type="button"
                            className={`wealth-chip${currency === c ? " active" : ""}`}
                            onClick={() => setCurrency(c)}
                          >
                            {c}
                          </button>
                        ))}
                      </div>
                    </div>
                  </div>

                  {side === "asset" && (
                    <div className="wealth-drawer-field">
                      <span className="wealth-drawer-field-label">
                        {dict.wealthGroupLabel}
                        <span className="wealth-drawer-required">*</span>
                      </span>
                      <div className="wealth-chip-row">
                        {GROUPS.map((g) => (
                          <button
                            key={g}
                            type="button"
                            className={`wealth-chip${group === g ? " active" : ""}`}
                            onClick={() => setGroup(g)}
                          >
                            {groupLabel(dict, g)}
                          </button>
                        ))}
                      </div>
                      <span style={{ fontSize: 12, color: "var(--ink-3)", lineHeight: 1.6, whiteSpace: "pre-line" }}>{classNote(dict, group)}</span>
                    </div>
                  )}

                  <div className="wealth-drawer-field">
                    <span className="wealth-drawer-field-label">{kind === "loan" ? dict.wealthLender : dict.wealthFInst}</span>
                    <input
                      value={kind === "loan" ? lender : venue}
                      placeholder={dict.wealthPhInst}
                      onChange={(e) => (kind === "loan" ? setLender(e.target.value) : setVenue(e.target.value))}
                    />
                  </div>

                  {kind === "deposit" && (
                    <div className="wealth-drawer-field">
                      <span className="wealth-drawer-field-label">{dict.wealthAccountNote}</span>
                      <input value={accountNote} onChange={(e) => setAccountNote(e.target.value)} />
                    </div>
                  )}
                  {kind === "loan" && (
                    <>
                      <div className="wealth-drawer-field">
                        <span className="wealth-drawer-field-label">{dict.wealthRatePct}</span>
                        <input className="mono" type="number" placeholder="2.5" value={ratePct} onChange={(e) => setRatePct(e.target.value)} />
                      </div>
                      <div className="wealth-drawer-field">
                        <span className="wealth-drawer-field-label">{dict.wealthOriginalPrincipal}</span>
                        <input
                          className="mono"
                          type="number"
                          value={originalPrincipal}
                          onChange={(e) => setOriginalPrincipal(e.target.value)}
                        />
                      </div>
                      <div className="wealth-drawer-field">
                        <span className="wealth-drawer-field-label">{dict.wealthRemainingMonths}</span>
                        <input
                          className="mono"
                          type="number"
                          value={remainingMonths}
                          onChange={(e) => setRemainingMonths(e.target.value)}
                        />
                        <span style={{ fontSize: 11, color: "var(--ink-3)", lineHeight: 1.6 }}>{dict.wealthLoanHint}</span>
                      </div>
                    </>
                  )}

                  {dup && (
                    <div
                      style={{
                        display: "flex",
                        gap: 9,
                        alignItems: "flex-start",
                        padding: "10px 12px",
                        borderRadius: 9,
                        background: "rgba(245,158,11,.1)",
                        border: "1px solid rgba(245,158,11,.35)",
                        color: "#f59e0b",
                      }}
                    >
                      <span style={{ fontFamily: "var(--font-mono)", fontSize: 9.5, letterSpacing: ".06em", flexShrink: 0 }}>{dict.wealthWarnDup}</span>
                      <span style={{ fontSize: 11.5, lineHeight: 1.45 }}>{dict.wealthWarnDupMsg + dup}</span>
                    </div>
                  )}
                  {error && <div className="error-message">{error}</div>}
                </>
              )}
            </div>
            {step === "form" && (
              <div className="wealth-drawer-footer">
                <span style={{ fontSize: 11.5, alignSelf: "center", maxWidth: 150, color: blocked ? "var(--loss)" : "var(--ink-3)" }}>
                  {blocked ? dict.wealthErrBlocked : dict.wealthAddHintLive}
                </span>
                <button className="wealth-drawer-cancel" style={{ marginLeft: "auto" }} onClick={onClose}>
                  {dict.cancel}
                </button>
                <button className="btn-primary" disabled={submitting} onClick={submit}>
                  {dict.wealthAddSave}
                </button>
              </div>
            )}
          </Dialog.Content>
        </Dialog.Overlay>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

export function EditValueModal({
  dict,
  asset,
  onClose,
  onSuccess,
  onUnauthorized,
}: {
  dict: Dictionary;
  asset: WealthAsset;
  onClose: () => void;
  onSuccess: () => void;
  onUnauthorized: (retry: () => void) => void;
}) {
  const [value, setValue] = useState(asset.value != null ? String(asset.value) : "");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit() {
    setSubmitting(true);
    setError(null);
    try {
      await saveWealthAssetSnapshot(asset.id, Number(value));
      onSuccess();
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
    <Dialog.Root open onOpenChange={(open) => !open && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="modal-backdrop">
          <Dialog.Content className="modal" aria-describedby={undefined} onOpenAutoFocus={(e) => e.preventDefault()}>
            <div className="modal-header">
              <Dialog.Title className="eyebrow">
                {dict.wealthEditValueTitle} — {asset.name}
              </Dialog.Title>
              <Dialog.Close className="modal-close" aria-label="close">
                ×
              </Dialog.Close>
            </div>
            <div className="modal-body">
              <label className="form-field">
                <span>{dict.wealthValue}</span>
                <input className="mono" type="number" value={value} onChange={(e) => setValue(e.target.value)} autoFocus />
              </label>
              {error && <div className="error-message">{error}</div>}
              <div className="modal-actions">
                <button
                  className="btn-primary"
                  disabled={submitting || value.trim() === "" || Number.isNaN(Number(value))}
                  onClick={submit}
                >
                  {dict.wealthEditValueTitle}
                </button>
              </div>
            </div>
          </Dialog.Content>
        </Dialog.Overlay>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
