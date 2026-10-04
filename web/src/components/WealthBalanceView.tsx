import { useEffect, useState } from "react";
import {
  ApiError,
  fetchWealthAssets,
  fetchWealthBalance,
  fetchWealthDebtPayoff,
  saveWealthAssetSnapshot,
  saveWealthProfile,
  unarchiveWealthAsset,
  type AllocCategory,
  type BalanceSheet,
  type DebtPayoffPlan,
  type DebtPayoffResult,
  type QuarterPoint,
  type WealthAsset,
} from "../api";
import type { Dictionary } from "../i18n";
import { AddAssetModal, AssetValue, MONO_LABEL, fmtMoney, groupLabel, liabilityNote } from "./WealthHomeView";
import { CATEGORY_COLOR } from "../wealthCategory";
import { useFlash } from "../flash";
import { BalanceRow } from "./BalanceRow";
import { RowEditDrawer } from "./RowEditDrawer";
import { WealthEmptyCard } from "./WealthOnboarding";

interface Props {
  dict: Dictionary;
  writable: boolean;
  onUnauthorized: (retry: () => void) => void;
  onNavigate: (path: string) => void;
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

export function srcLabel(dict: Dictionary, source: string): string {
  if (source === "import") return dict.wealthSrcImport;
  if (source === "sync") return dict.wealthSrcSync;
  return dict.wealthSrcManual;
}

// srcTag adds the design mock's "N 天未更新" suffix (wSrcTag) on top of
// srcLabel's plain manual/import/sync pill, switching to the amber .stale
// class — staleDays is only ever set past the 90-day threshold (§ backend
// staleInfo), so its mere presence is the stale/not-stale flag.
function srcTag(dict: Dictionary, source: string, staleDays?: number) {
  const stale = staleDays != null;
  const label = stale ? `${srcLabel(dict, source)} · ${dict.wealthStaleDaysSuffix.replace("%s", String(staleDays))}` : srcLabel(dict, source);
  return <span className={`wealth-src-tag ${stale ? "stale" : source}`}>{label}</span>;
}

export function WealthBalanceView({ dict, writable, onUnauthorized, onNavigate }: Props) {
  const [sheet, setSheet] = useState<BalanceSheet | null>(null);
  const [assets, setAssets] = useState<WealthAsset[] | null>(null);
  const [error, setError] = useState(false);
  const [refreshSignal, setRefreshSignal] = useState(0);
  const [salaryInput, setSalaryInput] = useState("");
  const [savingSalary, setSavingSalary] = useState(false);
  const [extra, setExtra] = useState("");
  const [payoff, setPayoff] = useState<DebtPayoffResult | null>(null);
  const [payoffLoading, setPayoffLoading] = useState(false);
  const [showAdd, setShowAdd] = useState(false);
  const [drawer, setDrawer] = useState<WealthAsset | null>(null);
  const [view, setView] = useState<"live" | "arc">("live");
  const flash = useFlash();

  useEffect(() => {
    setError(false);
    fetchWealthBalance()
      .then(setSheet)
      .catch(() => setError(true));
  }, [refreshSignal]);

  // Fetched alongside the balance sheet (archived ones included — they fill
  // the 已封存 tab): BalanceSheetItem/LiabilityDetail only carry the
  // already-FX-converted valueTwd, but a row's editor needs the stored record
  // (native currency and value, source, archive date).
  useEffect(() => {
    fetchWealthAssets(true)
      .then((r) => setAssets(r.assets))
      .catch(() => setError(true));
  }, [refreshSignal]);

  function refresh() {
    setRefreshSignal((n) => n + 1);
  }

  function findAsset(assetId?: number): WealthAsset | undefined {
    return assetId != null ? assets?.find((a) => a.id === assetId) : undefined;
  }

  // A write that hit a 401 re-runs itself once the login modal succeeds, like
  // saveSalary below.
  async function guarded(op: () => Promise<unknown>, after: () => void): Promise<void> {
    try {
      await op();
      after();
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        onUnauthorized(() => void guarded(op, after));
      } else {
        flash(e instanceof ApiError ? e.message : dict.error, "error");
      }
    }
  }

  const saveInline = (a: WealthAsset, value: number) => guarded(() => saveWealthAssetSnapshot(a.id, value), refresh);

  const restore = (a: WealthAsset) => {
    if (!writable) {
      flash(dict.wealthRoNoChange);
      return;
    }
    void guarded(
      () => unarchiveWealthAsset(a.id),
      () => {
        flash(dict.wealthFlashRestored.replace("%s", a.name));
        refresh();
      },
    );
  };

  async function saveSalary() {
    const v = Number(salaryInput);
    if (Number.isNaN(v) || v < 0) return;
    setSavingSalary(true);
    try {
      await saveWealthProfile({ annualSalary: v });
      setRefreshSignal((n) => n + 1);
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        onUnauthorized(saveSalary);
      } else {
        flash(e instanceof ApiError ? e.message : dict.error, "error");
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
  const archived = (assets ?? [])
    .filter((a) => a.archivedAt)
    .sort((a, b) => (b.archivedAt ?? "").localeCompare(a.archivedAt ?? ""));
  // Archived records count too, so the one-line empty card never stands in
  // for a sheet whose 已封存 tab still has something to restore.
  const empty = assets != null && assets.length === 0;
  // Counts the equity row too, like the design's tab does.
  const liveCount = sheet ? sheet.assetGroups.reduce((n, g) => n + g.assets.length, 0) + sheet.liabilities.length : 0;

  return (
    <>
      {/* Header + "+" trigger, matching the design template's isWBalance row
          (Argus Trading WebUI.dc.html lines 632-636), including its
          top+bottom margin:var(--gap) 0 — this row sits directly under the
          app's fixed topbar with nothing else providing breathing room
          above it, unlike a .card (which gets one from .content>.card+.card),
          so it needs its own top margin too, not just bottom. */}
      <div style={{ display: "flex", alignItems: "center", gap: 12, margin: "16px 0", flexWrap: "wrap" }}>
        <span style={{ ...MONO_LABEL, color: "var(--ink)" }}>{dict.navWealthBalance}</span>
        <div className="wealth-tabs" role="tablist">
          {(
            [
              ["live", dict.wealthTabActive, liveCount],
              ["arc", dict.wealthTabArchived, archived.length],
            ] as const
          ).map(([k, label, n]) => (
            <button key={k} type="button" role="tab" aria-selected={view === k} className={`wealth-tab${view === k ? " active" : ""}`} onClick={() => setView(k)}>
              <span>{label}</span>
              <span className="wealth-tab-n">{n}</span>
            </button>
          ))}
        </div>
        <span style={{ marginLeft: "auto", fontFamily: "var(--font-mono)", fontSize: 10, letterSpacing: ".06em", color: "var(--ink-3)" }}>
          {dict.wealthNetWorthFormula}
        </span>
        {writable && (
          <button className="btn-tint" onClick={() => setShowAdd(true)}>
            {dict.wealthAddAsset}
          </button>
        )}
      </div>

      {view === "arc" ? (
        <ArchivedCard dict={dict} items={archived} loaded={assets != null} onOpen={setDrawer} onRestore={restore} />
      ) : empty ? (
        <WealthEmptyCard dict={dict} line={dict.wealthEmptyBalance} onAdd={writable ? () => setShowAdd(true) : undefined} />
      ) : (
        <>
          <div className="card card--glow" style={{ padding: 19.2, marginBottom: 16 }}>
            <div className="wealth-hero-row">
              <div className="wealth-hero-block">
                <span className="wealth-hero-block-label">{dict.wealthNetWorth}</span>
                <span className="wealth-hero-block-value big">
                  {sheet?.netWorth != null ? fmtMoney(sheet.netWorth, CURRENCY) : "—"}
                </span>
              </div>
              <div className="wealth-hero-block">
                <span className="wealth-hero-block-label">{dict.wealthTotalAssets}</span>
                <span className="wealth-hero-block-value">
                  {sheet?.totalAssets != null ? fmtMoney(sheet.totalAssets, CURRENCY) : "—"}
                </span>
              </div>
              <div className="wealth-hero-block">
                <span className="wealth-hero-block-label">{dict.wealthTotalLiabilities}</span>
                <span className="wealth-hero-block-value loss">
                  {sheet?.totalLiabilities != null ? fmtMoney(sheet.totalLiabilities, CURRENCY) : "—"}
                </span>
              </div>
              <div className="wealth-hero-block">
                <span className="wealth-hero-block-label">
                  {dict.wealthNetWorth} / {dict.wealthTotalAssets}
                </span>
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
                          {groupLabel(dict, g.group)}
                          <span style={{ fontSize: 10, color: "var(--ink-3)" }}>
                            {g.pctOfAssets != null ? `${g.pctOfAssets.toFixed(1)}% ${dict.wealthPctOfAssets}` : ""}
                          </span>
                          <span className="wealth-group-header-value">{fmtMoney(g.marketValue, CURRENCY)}</span>
                        </div>
                        {g.assets.map((item, i) => {
                          const match = findAsset(item.assetId);
                          return (
                            <BalanceRow
                              key={item.assetId ?? `${g.group}-${i}`}
                              dict={dict}
                              dotColor={CATEGORY_COLOR[item.category as AllocCategory]}
                              name={item.assetId ? item.name : equityLabel(dict, item.type)}
                              tag={srcTag(dict, item.source, item.staleDays)}
                              valueText={fmtMoney(item.valueTwd, CURRENCY)}
                              asset={match}
                              link={item.assetId == null}
                              writable={writable}
                              open={match != null && drawer?.id === match.id}
                              onOpen={setDrawer}
                              onGoTrade={() => onNavigate("/")}
                              onInlineSave={saveInline}
                            />
                          );
                        })}
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
                  <div className="wealth-col-stack">
                    {sheet.liabGroups
                      .filter((g) => g.liabilities.length > 0)
                      .map((g) => (
                        <div key={g.kind} className="wealth-group-block">
                          <div className="wealth-group-header">
                            {g.kind === "long" ? dict.wealthLiabLongTerm : dict.wealthLiabShortTerm}
                            <span style={{ fontSize: 10, color: "var(--ink-3)" }}>
                              {g.pctOfLiabilities != null ? `${g.pctOfLiabilities.toFixed(1)}% ${dict.wealthPctOfLiabilities}` : ""}
                            </span>
                            <span className="wealth-group-header-value loss">{fmtMoney(g.marketValue, CURRENCY)}</span>
                          </div>
                          {g.liabilities.map((l) => {
                            const match = findAsset(l.assetId);
                            return (
                              <BalanceRow
                                key={l.assetId}
                                dict={dict}
                                name={l.name}
                                note={[l.ratePct != null ? `${dict.wealthRateLabel} ${l.ratePct}%` : "", liabilityNote(dict, CURRENCY, l)].filter(Boolean).join(" · ")}
                                tag={srcTag(dict, l.source, l.staleDays)}
                                valueText={fmtMoney(l.valueTwd, CURRENCY)}
                                loss
                                asset={match}
                                link={false}
                                writable={writable}
                                open={match != null && drawer?.id === match.id}
                                onOpen={setDrawer}
                                onGoTrade={() => onNavigate("/")}
                                onInlineSave={saveInline}
                              />
                            );
                          })}
                        </div>
                      ))}
                  </div>
                )}
              </div>

              <div className="card">
                <div className="eyebrow" style={{ marginBottom: 14 }}>
                  {dict.wealthDebtRatio} / {dict.wealthLiquidityMonths} / {dict.wealthSavingsRate} / {dict.wealthExpenseRatio}
                </div>
                <div className="wealth-ratios-grid">
                  <RatioBlock label={dict.wealthDebtRatio} value={pct(sheet?.debtRatioPct ?? null)} note={dict.wealthDebtRatioNote} />
                  <RatioBlock
                    label={dict.wealthLiquidityMonths}
                    value={sheet?.liquidityMonths != null ? sheet.liquidityMonths.toFixed(1) : "—"}
                    note={dict.wealthLiquidityNote}
                  />
                  <RatioBlock label={dict.wealthSavingsRate} value={pct(sheet?.savingsRatePct ?? null)} note={dict.wealthSavingsRateNote} />
                  <RatioBlock label={dict.wealthExpenseRatio} value={pct(sheet?.expenseRatioPct ?? null)} note={dict.wealthExpenseRatioNote} />
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
        />
      )}
      {drawer && (
        <RowEditDrawer
          dict={dict}
          asset={drawer}
          writable={writable}
          onClose={() => setDrawer(null)}
          onChanged={refresh}
          onUnauthorized={onUnauthorized}
        />
      )}
    </>
  );
}

// ArchivedCard is the 已封存 tab (design: wae.rows): what was archived, when,
// and what it was worth then, with a way back. Archived values come straight
// from the stored record (frozen at archive time — the server refuses writes
// to an archived asset), in the asset's own currency like the /w asset list.
function ArchivedCard({
  dict,
  items,
  loaded,
  onOpen,
  onRestore,
}: {
  dict: Dictionary;
  items: WealthAsset[];
  loaded: boolean;
  onOpen: (a: WealthAsset) => void;
  onRestore: (a: WealthAsset) => void;
}) {
  return (
    <div className="card wealth-arc">
      <div className="wealth-arc-head">
        <span style={MONO_LABEL}>{dict.wealthTabArchived}</span>
        <span className="wealth-arc-desc">{dict.wealthArcDesc}</span>
      </div>
      <div className="wealth-arc-table">
        <div className="wealth-arc-row head">
          <span>{dict.wealthName}</span>
          <span>{dict.wealthArcColKind}</span>
          <span>{dict.wealthVenue}</span>
          <span>{dict.wealthArcColDate}</span>
          <span style={{ textAlign: "right" }}>{dict.wealthArcColValue}</span>
          <span />
        </div>
        {items.map((a) => (
          <div key={a.id} className="wealth-arc-row">
            <button type="button" className="wealth-arc-name" onClick={() => onOpen(a)}>
              {a.name}
            </button>
            <span className="wealth-arc-kind">{a.side === "asset" ? dict.wealthSideAsset : dict.wealthSideLiability}</span>
            <span className="wealth-arc-inst">{a.venue || "—"}</span>
            <span className="wealth-arc-date">{(a.archivedAt ?? "").slice(0, 10)}</span>
            <span className="wealth-arc-val">
              <AssetValue dict={dict} asset={a} />
            </span>
            <button type="button" className="wealth-arc-restore" onClick={() => onRestore(a)}>
              {dict.wealthRestore}
            </button>
          </div>
        ))}
      </div>
      {loaded && items.length === 0 && <div className="wealth-arc-none">{dict.wealthArcNone}</div>}
    </div>
  );
}

function RatioBlock({ label, value, note }: { label: string; value: string; note: string }) {
  return (
    <div className="wealth-ratio-block">
      <span className="wealth-ratio-block-label">{label}</span>
      <span className="wealth-ratio-block-value">{value}</span>
      <span className="wealth-ratio-block-note">{note}</span>
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
