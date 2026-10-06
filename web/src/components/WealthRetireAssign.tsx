import { useEffect, useState } from "react";
import {
  ApiError,
  assignRetirementAssets,
  fetchWealthAssets,
  fetchWealthGoals,
  fetchWealthRetireLive,
  type WealthAsset,
  type WealthRetire,
} from "../api";
import type { Dictionary } from "../i18n";
import { useFlash } from "../flash";
import { fmtMoney } from "./WealthHomeView";

const CURRENCY = "NT$";

interface Props {
  dict: Dictionary;
  retire: WealthRetire;
  // The settings drawer's what-if assumptions, so the funded-% preview reads
  // the same plan the page is showing.
  cfg: Record<string, string>;
  writable: boolean;
  onClose: () => void;
  onSaved: (updated: WealthRetire) => void;
  onUnauthorized: (retry: () => void) => void;
  onNavigate: (path: string) => void;
}

// What other goals already hold of an asset (summed, with their names).
interface Other {
  names: string[];
  ratio: number;
}
type Draft = Record<number, { on: boolean; pct: string; sug?: boolean }>;

const pc = (r: number) => `${Math.round(r * 100)}%`;
const round4 = (x: number) => Math.round(x * 10000) / 10000;
const fill = (tpl: string, vars: Record<string, string | number>) =>
  Object.entries(vars).reduce((s, [k, v]) => s.replace(`{${k}}`, String(v)), tpl);

const MONO_LABEL = { fontFamily: "var(--font-mono)", fontSize: 9.5, letterSpacing: ".06em", color: "var(--ink-3)" } as const;

// RetireAssignDrawer is the design's 指定退休資產 drawer: tick the assets set
// aside for retirement (all of what's left of them, or a percentage under the
// 進階 switch). It saves the whole list at once, so un-ticking removes.
export function RetireAssignDrawer({ dict, retire, cfg, writable, onClose, onSaved, onUnauthorized, onNavigate }: Props) {
  const flash = useFlash();
  const [data, setData] = useState<{ assets: WealthAsset[]; others: Map<number, Other> } | null>(null);
  const [failed, setFailed] = useState(false);
  const [draft, setDraft] = useState<Draft>({});
  const [adv, setAdv] = useState(false);
  const [saving, setSaving] = useState(false);
  const [rate, setRate] = useState<number | null>(null);

  // Load the assets and what other goals hold of each, then seed the draft the
  // way the design's openEm() does: what's saved, or — for a goal with nothing
  // yet — the pension accounts, suggested.
  useEffect(() => {
    let cancelled = false;
    Promise.all([fetchWealthAssets(), fetchWealthGoals()])
      .then(([a, g]) => {
        if (cancelled) return;
        const assets = a.assets.filter((x) => x.side === "asset");
        const others = new Map<number, Other>();
        for (const goal of g.goals) {
          if (goal.kind === "retirement" || goal.id === retire.goal?.id) continue;
          for (const ga of goal.assets) {
            const o = others.get(ga.assetId) ?? { names: [], ratio: 0 };
            o.names.push(goal.name);
            o.ratio += ga.ratio;
            others.set(ga.assetId, o);
          }
        }
        const saved = new Map((retire.goal?.assets ?? []).map((x) => [x.assetId, x.ratio]));
        const any = Array.from(saved.values()).some((r) => r > 0);
        const d: Draft = {};
        let advanced = false;
        for (const asset of assets) {
          const left = Math.max(0, 1 - (others.get(asset.id)?.ratio ?? 0));
          const s = saved.get(asset.id) ?? 0;
          if (s > 0) {
            d[asset.id] = { on: true, pct: String(Math.round(s * 100)) };
            if (s < left - 0.001) advanced = true;
          } else if (!any && asset.type === "pension" && left > 0) {
            d[asset.id] = { on: true, pct: String(Math.round(left * 100)), sug: true };
          }
        }
        setData({ assets, others });
        setDraft(d);
        setAdv(advanced);
      })
      .catch(() => !cancelled && setFailed(true));
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [onClose]);

  const rows = (data?.assets ?? []).map((asset) => {
    const other = data!.others.get(asset.id);
    const oR = other?.ratio ?? 0;
    const left = Math.max(0, 1 - oR);
    const st = draft[asset.id] ?? { on: false, pct: "" };
    const pN = parseFloat(st.pct);
    const r = st.on ? (adv ? (Number.isFinite(pN) ? pN / 100 : 0) : left) : 0;
    return {
      asset,
      other,
      oR,
      left,
      full: left <= 0,
      st,
      r,
      over: st.on && r + oR > 1.0001,
      counted: Math.round((asset.valueTwd ?? 0) * Math.min(r, left)),
    };
  });
  const picked = rows.filter((x) => x.st.on && x.r > 0);
  const sum = picked.reduce((s, x) => s + x.counted, 0);
  const overCount = rows.filter((x) => x.over).length;
  const blocked = overCount > 0 || !writable || saving;

  // The funded-% in the footer is the server's own projection for this pool,
  // not a second copy of the formula here.
  useEffect(() => {
    if (sum <= 0) {
      setRate(null);
      return;
    }
    let cancelled = false;
    const t = setTimeout(() => {
      fetchWealthRetireLive(new URLSearchParams({ ...cfg, pool: String(sum) }))
        .then((r) => !cancelled && setRate(r.baseline?.achievementPct ?? null))
        .catch(() => {});
    }, 200);
    return () => {
      cancelled = true;
      clearTimeout(t);
    };
  }, [sum, cfg]);

  function toggle(id: number, left: number) {
    if (left <= 0) return;
    setDraft((d) => {
      const cur = d[id] ?? { on: false, pct: "" };
      return { ...d, [id]: { on: !cur.on, pct: cur.pct || String(Math.round(left * 100)) } };
    });
  }

  function setPct(id: number, value: string) {
    setDraft((d) => ({ ...d, [id]: { on: d[id]?.on ?? true, pct: value.replace(/[^\d.]/g, "") } }));
  }

  async function save() {
    if (blocked) return;
    setSaving(true);
    try {
      const updated = await assignRetirementAssets(
        dict.navWealthRetire,
        retire.retirementAge,
        retire.monthlySpend,
        picked.map((x) => ({ assetId: x.asset.id, ratio: round4(adv ? x.r : x.left) })),
      );
      onSaved(updated);
      onClose();
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        onUnauthorized(save);
      } else {
        flash(e instanceof ApiError ? e.message : dict.error, "error");
      }
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="wealth-drawer-overlay" onClick={onClose}>
      <div
        className="wealth-drawer-panel"
        style={{ width: "min(460px,100%)" }}
        role="dialog"
        aria-modal="true"
        aria-label={dict.wealthRetireAssignTitle}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="wealth-drawer-header">
          <span style={{ fontFamily: "var(--font-mono)", textTransform: "uppercase", letterSpacing: ".08em", fontSize: 11 }}>
            {dict.wealthRetireAssignTitle}
          </span>
          <button
            className="wealth-drawer-cancel"
            style={{
              marginLeft: "auto",
              fontFamily: "var(--font-mono)",
              fontSize: 12,
              fontWeight: 400,
              padding: "4px 9px",
              borderRadius: 6,
              background: "var(--surface)",
              color: "var(--ink-3)",
            }}
            onClick={onClose}
            aria-label={dict.cancel}
          >
            ✕
          </button>
        </div>

        <div className="wealth-drawer-body" style={{ padding: "16px 18px", gap: 14 }}>
          <div style={{ fontSize: 12, color: "var(--ink-2)", lineHeight: 1.65 }}>{dict.wealthRetireAssignIntro}</div>
          {!retire.goal && (
            <div style={{ fontSize: 11.5, color: "var(--ink-2)", lineHeight: 1.6, padding: "9px 11px", borderRadius: 8, border: "1px dashed var(--border)" }}>
              {fill(dict.wealthRetireAssignNoGoal, { age: retire.retirementAge })}
            </div>
          )}
          <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
            <button
              role="switch"
              aria-checked={adv}
              aria-label={dict.wealthRetireAssignAdvLabel}
              onClick={() => setAdv((v) => !v)}
              style={{
                position: "relative",
                width: 32,
                height: 18,
                borderRadius: 9,
                flexShrink: 0,
                border: "none",
                cursor: "pointer",
                padding: 0,
                background: adv ? "var(--accent)" : "var(--border)",
              }}
            >
              <span
                style={{
                  position: "absolute",
                  top: 2,
                  left: adv ? 16 : 2,
                  width: 14,
                  height: 14,
                  borderRadius: "50%",
                  background: "var(--surface)",
                  transition: "left .15s",
                }}
              />
            </button>
            <span style={{ fontSize: 12.5, color: "var(--ink)" }}>{dict.wealthRetireAssignAdvLabel}</span>
            <span style={{ fontSize: 11, color: "var(--ink-3)" }}>{dict.wealthRetireAssignAdvHint}</span>
          </div>

          {failed && <div className="error-message">{dict.error}</div>}
          {data && data.assets.length === 0 && (
            <div style={{ display: "flex", flexDirection: "column", gap: 10, alignItems: "flex-start", padding: 14, borderRadius: 9, border: "1px dashed var(--border)" }}>
              <span style={{ fontSize: 12.5, color: "var(--ink-2)" }}>{dict.wealthRetireAssignNoAssets}</span>
              <a
                href="#"
                className="accent-link"
                style={{ fontFamily: "var(--font-mono)", fontSize: 12 }}
                onClick={(e) => {
                  e.preventDefault();
                  onClose();
                  onNavigate("/w/balance");
                }}
              >
                {dict.wealthRetireAssignGoBalance}
              </a>
            </div>
          )}

          <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
            {rows.map(({ asset, other, oR, left, full, st, r, over, counted }) => (
              <div
                key={asset.id}
                style={{
                  display: "flex",
                  flexDirection: "column",
                  gap: 8,
                  padding: "11px 12px",
                  borderRadius: 9,
                  border: `1px solid ${over ? "var(--loss)" : st.on ? "var(--accent-tint-border)" : "var(--border)"}`,
                  background: over ? "rgba(239,68,68,.07)" : st.on ? "var(--accent-tint-bg)" : "var(--bg)",
                  opacity: full ? 0.55 : 1,
                }}
              >
                <button
                  role="checkbox"
                  aria-checked={st.on}
                  disabled={full}
                  onClick={() => toggle(asset.id, left)}
                  style={{
                    display: "flex",
                    alignItems: "center",
                    gap: 10,
                    width: "100%",
                    textAlign: "left",
                    background: "none",
                    border: "none",
                    padding: 0,
                    color: "var(--ink)",
                    font: "inherit",
                    cursor: full ? "not-allowed" : "pointer",
                  }}
                >
                  <span
                    aria-hidden="true"
                    style={{
                      width: 16,
                      height: 16,
                      flexShrink: 0,
                      borderRadius: 4,
                      display: "flex",
                      alignItems: "center",
                      justifyContent: "center",
                      fontSize: 11,
                      lineHeight: 1,
                      color: "var(--bg)",
                      border: `1.5px solid ${st.on ? "var(--accent)" : "var(--ink-3)"}`,
                      background: st.on ? "var(--accent)" : "transparent",
                    }}
                  >
                    {st.on ? "✓" : ""}
                  </span>
                  <span style={{ fontSize: 13, flex: 1, minWidth: 0 }}>{asset.name}</span>
                  <span className="mono" style={{ fontSize: 12, color: "var(--ink-2)" }}>
                    {asset.valueTwd != null ? fmtMoney(asset.valueTwd, CURRENCY) : "—"}
                  </span>
                </button>
                <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap", paddingLeft: 26 }}>
                  {st.sug && (
                    <span
                      className="mono"
                      style={{ fontSize: 9.5, letterSpacing: ".04em", color: "var(--accent)", border: "1px solid var(--accent-tint-border)", borderRadius: 4, padding: "2px 6px" }}
                    >
                      {dict.wealthRetireAssignSuggested}
                    </span>
                  )}
                  {other && (
                    <span className="mono" style={{ fontSize: 10.5, color: "var(--ink-3)", background: "var(--surface-2)", borderRadius: 4, padding: "2px 7px" }}>
                      {full
                        ? fill(dict.wealthRetireAssignOtherFull, { goal: other.names.join("、") })
                        : fill(dict.wealthRetireAssignOtherPart, { goal: other.names.join("、"), pct: pc(oR) })}
                    </span>
                  )}
                  {adv && st.on && (
                    <span style={{ display: "flex", alignItems: "center", gap: 6 }}>
                      <input
                        value={st.pct}
                        onChange={(e) => setPct(asset.id, e.target.value)}
                        inputMode="decimal"
                        autoComplete="off"
                        aria-label={`${asset.name} %`}
                        className="mono"
                        style={{
                          width: 64,
                          boxSizing: "border-box",
                          background: "var(--bg)",
                          border: `1px solid ${over ? "var(--loss)" : "var(--border)"}`,
                          borderRadius: 6,
                          padding: "5px 8px",
                          color: "var(--ink)",
                          fontSize: 12.5,
                          textAlign: "right",
                        }}
                      />
                      <span className="mono" style={{ fontSize: 12, color: "var(--ink-3)" }}>
                        %
                      </span>
                      <span className="mono" style={{ fontSize: 10.5, color: "var(--ink-3)" }}>
                        {fill(dict.wealthRetireAssignMax, { pct: pc(left) })}
                      </span>
                    </span>
                  )}
                  {st.on && !over && (
                    <span className="mono" style={{ marginLeft: "auto", fontSize: 11, color: "var(--ink-2)" }}>
                      {!adv && left < 1
                        ? fill(dict.wealthRetireAssignCountsRemain, { amount: fmtMoney(counted, CURRENCY), pct: pc(left) })
                        : fill(dict.wealthRetireAssignCounts, { amount: fmtMoney(counted, CURRENCY) })}
                    </span>
                  )}
                </div>
                {over && (
                  <div style={{ paddingLeft: 26, fontSize: 11.5, color: "var(--loss)", lineHeight: 1.5 }}>
                    {other
                      ? fill(dict.wealthRetireAssignOverWith, { goal: other.names.join("、"), total: pc(r + oR), left: pc(left) })
                      : dict.wealthRetireAssignOverAlone}
                  </div>
                )}
              </div>
            ))}
          </div>
        </div>

        <div style={{ borderTop: "1px solid var(--border)", padding: "14px 18px", display: "flex", flexDirection: "column", gap: 12 }}>
          <div style={{ display: "flex", gap: 22, flexWrap: "wrap" }}>
            <div style={{ display: "flex", flexDirection: "column", gap: 3 }}>
              <span style={MONO_LABEL}>
                {dict.wealthRetireAssignSumLabel} · {fill(dict.wealthRetireAssignSumCount, { n: picked.length })}
              </span>
              <span className="mono" style={{ fontSize: 19 }}>
                {fmtMoney(sum, CURRENCY)}
              </span>
            </div>
            <div style={{ display: "flex", flexDirection: "column", gap: 3 }}>
              <span style={MONO_LABEL}>{dict.wealthRetireRateLabel}</span>
              <span className="mono" style={{ fontSize: 19 }}>
                {sum > 0 && rate != null ? `${Math.min(rate, 999).toFixed(0)}%` : "—"}
              </span>
            </div>
          </div>
          {overCount > 0 && (
            <div style={{ fontSize: 11.5, color: "var(--loss)", lineHeight: 1.5 }}>{fill(dict.wealthRetireAssignOverMsg, { n: overCount })}</div>
          )}
          {!writable && <div style={{ fontSize: 11.5, color: "var(--ink-3)", lineHeight: 1.5 }}>{dict.wealthRetireAssignReadOnly}</div>}
          <div style={{ display: "flex", gap: 10 }}>
            <button className="wealth-drawer-cancel" style={{ fontSize: 12.5, fontWeight: 400, padding: "8px 14px", background: "var(--surface)" }} onClick={onClose}>
              {dict.cancel}
            </button>
            <button
              onClick={save}
              disabled={blocked}
              style={{
                flex: 1,
                fontFamily: "var(--font-sans)",
                fontSize: 12.5,
                fontWeight: 700,
                padding: "8px 14px",
                borderRadius: 8,
                border: "1px solid var(--accent-tint-border)",
                background: "var(--accent-tint-bg)",
                color: "var(--accent)",
                opacity: blocked ? 0.4 : 1,
                cursor: blocked ? "not-allowed" : "pointer",
              }}
            >
              {dict.wealthRetireAssignSave}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
