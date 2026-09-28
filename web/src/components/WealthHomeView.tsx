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
  type WealthAlloc,
  type WealthAsset,
  type WealthHome,
} from "../api";
import type { Dictionary } from "../i18n";
import { convertTWD, shortTWD } from "../currency";
import { CATEGORY_COLOR, categoryLabel, loadModel } from "../wealthCategory";

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

  async function handleArchive(a: WealthAsset) {
    if (!window.confirm(dict.wealthArchiveConfirm)) return;
    try {
      await archiveWealthAsset(a.id);
      refresh();
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        onUnauthorized(() => handleArchive(a));
      } else {
        window.alert(e instanceof ApiError ? e.message : dict.error);
      }
    }
  }

  if (error) {
    return <div className="error-message">{dict.error}</div>;
  }

  const currency = "NT$"; // converted for display by fmtMoney (顯示幣別)

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
    const term =
      l.type === "credit_card" && l.remainingMonths == null
        ? dict.wealthHomePayFirst
        : l.remainingMonths != null
          ? dict.wealthHomeYearsLeft.replace("%s", String(Math.round(l.remainingMonths / 12)))
          : "";
    const pay = l.minPayment ? dict.wealthHomeMonthlyPay.replace("%s", fmtMoney(l.minPayment, currency)) : "";
    return {
      key: l.assetId,
      name: l.name,
      rate: l.ratePct != null ? `${l.ratePct}%` : "—",
      value: fmtMoney(l.valueTwd, currency),
      share: liabTotal > 0 ? (l.valueTwd / liabTotal) * 100 : 0,
      note: [term, pay].filter(Boolean).join(" · "),
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
                    {a.value != null ? fmtMoney(a.value, a.currency === "TWD" ? currency : a.currency) : "—"}
                  </td>
                  {writable && (
                    <td className="row-actions">
                      <button onClick={() => setEditing(a)}>{dict.wealthEditValueTitle}</button>
                      <button onClick={() => handleArchive(a)}>{dict.wealthArchive}</button>
                    </td>
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {showAdd && (
        <AddAssetModal
          dict={dict}
          onClose={() => setShowAdd(false)}
          onSuccess={() => {
            setShowAdd(false);
            refresh();
          }}
          onUnauthorized={onUnauthorized}
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
    </>
  );
}

// AddAssetModal matches the design template's shared "quick-add drawer"
// (Argus Trading WebUI.dc.html lines 3304-3438, dw.*) as closely as this
// app's data model allows: a right-side sliding panel, step 1 picks a kind,
// step 2 is that kind's form. The template's drawer actually picks from
// FOUR top-level kinds (asset/liability/cash-flow-item/insurance policy) —
// "policy" has no backend yet (insurance_details is a future phase) and
// "flow" already has its own dedicated add flow on /w/cash, so this drawer
// only ever creates assets/liabilities, and its own step-1 list is this
// app's finer asset `type` taxonomy (KINDS) instead of the template's
// coarse kind split — "side" is then implied by which KIND was picked,
// same as the template implies it by which top-level kind bucket you're in,
// so there's no separate side selector. Skipped: the duplicate-name/
// LTV-conflict warning banners and the recurring-contribution toggle — both
// need data (existing asset names, cash-flow context) this modal doesn't
// have wired in, and a live FX-conversion preview line, since the frontend
// has no FX rate to show one with (conversion happens server-side only).
export function AddAssetModal({
  dict,
  onClose,
  onSuccess,
  onUnauthorized,
}: {
  dict: Dictionary;
  onClose: () => void;
  onSuccess: () => void;
  onUnauthorized: (retry: () => void) => void;
}) {
  const [step, setStep] = useState<"pick" | "form">("pick");
  const [kind, setKind] = useState<Kind>("deposit");
  const [name, setName] = useState("");
  const [group, setGroup] = useState<AssetGroup>("liquid");
  const [venue, setVenue] = useState("");
  const [currency, setCurrency] = useState("TWD");
  const [initialValue, setInitialValue] = useState("");
  const [bank, setBank] = useState("");
  const [accountNote, setAccountNote] = useState("");
  const [lender, setLender] = useState("");
  const [ratePct, setRatePct] = useState("");
  const [originalPrincipal, setOriginalPrincipal] = useState("");
  const [remainingMonths, setRemainingMonths] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function pickKind(next: Kind) {
    setKind(next);
    const defaultGroup = KIND_DEFAULT_GROUP[next];
    if (defaultGroup) {
      setGroup(defaultGroup);
    }
    setStep("form");
  }

  const side: AssetSide = kind === "loan" ? "liability" : "asset";

  async function submit() {
    setSubmitting(true);
    setError(null);
    try {
      await createWealthAsset({
        side,
        type: kind,
        name: name.trim(),
        assetGroup: group,
        venue: venue.trim() || undefined,
        currency,
        initialValue: Number(initialValue),
        deposit: kind === "deposit" ? { bank: bank.trim() || undefined, accountNote: accountNote.trim() || undefined } : undefined,
        loan:
          kind === "loan"
            ? {
                lender: lender.trim() || undefined,
                ratePct: ratePct ? Number(ratePct) : undefined,
                originalPrincipal: originalPrincipal ? Number(originalPrincipal) : undefined,
                remainingMonths: remainingMonths ? Number(remainingMonths) : undefined,
              }
            : undefined,
      });
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

  const canSubmit = name.trim() !== "" && Number(initialValue) !== 0 && !Number.isNaN(Number(initialValue));

  return (
    <Dialog.Root open onOpenChange={(open) => !open && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="wealth-drawer-overlay">
          <Dialog.Content className="wealth-drawer-panel" aria-describedby={undefined} onOpenAutoFocus={(e) => e.preventDefault()}>
            <div className="wealth-drawer-header">
              <Dialog.Title style={{ fontFamily: "var(--font-mono)", textTransform: "uppercase", letterSpacing: ".08em", fontSize: 11 }}>
                {step === "pick" ? dict.wealthAddTitle : kindLabel(dict, kind)}
              </Dialog.Title>
              <span className="wealth-drawer-step">{step === "pick" ? "1/2" : "2/2"}</span>
              <Dialog.Close className="modal-close" style={{ marginLeft: "auto" }} aria-label="close">
                ×
              </Dialog.Close>
            </div>
            <div className="wealth-drawer-body">
              {step === "pick" ? (
                <div style={{ display: "flex", flexDirection: "column", gap: 9 }}>
                  {KINDS.map((k) => (
                    <button key={k} className="wealth-kind-btn" onClick={() => pickKind(k)}>
                      <span className="wealth-kind-btn-label">{kindLabel(dict, k)}</span>
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
                    </span>
                    <input value={name} onChange={(e) => setName(e.target.value)} autoFocus />
                  </div>

                  <div className="wealth-drawer-field">
                    <span className="wealth-drawer-field-label">
                      {dict.wealthInitialValue}
                      <span className="wealth-drawer-required">*</span>
                    </span>
                    <div style={{ display: "flex", gap: 6, alignItems: "center" }}>
                      <input
                        className="mono"
                        type="number"
                        value={initialValue}
                        onChange={(e) => setInitialValue(e.target.value)}
                        style={{ flex: 1, minWidth: 0 }}
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
                  </div>

                  <div className="wealth-drawer-field">
                    <span className="wealth-drawer-field-label">{dict.wealthVenue}</span>
                    <input value={venue} onChange={(e) => setVenue(e.target.value)} />
                  </div>

                  {kind === "deposit" && (
                    <>
                      <div className="wealth-drawer-field">
                        <span className="wealth-drawer-field-label">{dict.wealthBank}</span>
                        <input value={bank} onChange={(e) => setBank(e.target.value)} />
                      </div>
                      <div className="wealth-drawer-field">
                        <span className="wealth-drawer-field-label">{dict.wealthAccountNote}</span>
                        <input value={accountNote} onChange={(e) => setAccountNote(e.target.value)} />
                      </div>
                    </>
                  )}
                  {kind === "loan" && (
                    <>
                      <div className="wealth-drawer-field">
                        <span className="wealth-drawer-field-label">{dict.wealthLender}</span>
                        <input value={lender} onChange={(e) => setLender(e.target.value)} />
                      </div>
                      <div className="wealth-drawer-field">
                        <span className="wealth-drawer-field-label">{dict.wealthRatePct}</span>
                        <input className="mono" type="number" value={ratePct} onChange={(e) => setRatePct(e.target.value)} />
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
                      </div>
                    </>
                  )}
                  {error && <div className="error-message">{error}</div>}
                </>
              )}
            </div>
            {step === "form" && (
              <div className="wealth-drawer-footer">
                <button className="wealth-drawer-cancel" onClick={onClose}>
                  {dict.cancel}
                </button>
                <button className="btn-primary" style={{ marginLeft: "auto" }} disabled={!canSubmit || submitting} onClick={submit}>
                  {dict.wealthAddAsset}
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
