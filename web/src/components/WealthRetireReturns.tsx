import { useEffect, useState } from "react";
import { fetchWealthRetireLive, type RetireClass, type RetireReturnRow, type WealthRetire } from "../api";
import type { Dictionary } from "../i18n";
import { useAssistant } from "../assistant";
import { shortTWD } from "../currency";

// The drawer's 報酬率假設 section (Argus Trading WebUI's retMixModel): the
// pool's classes with an editable long-run return each, a trial mix over the
// movable classes, and the post-retirement mix. Everything here is a
// page-local what-if like the rest of the drawer — nothing is saved.

export type RetireMode = "auto" | "manual";
export type PostKey = "conserv" | "balanced" | "keep";
type PresetKey = "conserv" | "balanced" | "growth";

export interface ReturnsState {
  mode: RetireMode;
  edits: Partial<Record<RetireClass, string>>; // per-class return overrides, as typed
  post: PostKey;
}
export const RETURNS_DEFAULT: ReturnsState = { mode: "auto", edits: {}, post: "conserv" };

const MOVABLE: RetireClass[] = ["etf", "bond", "gold", "cash"];
// conserv must match the server's RetirementPostMix (its default after retiring).
const PRESETS: Record<PresetKey, Record<string, number>> = {
  conserv: { etf: 30, bond: 50, gold: 5, cash: 15 },
  balanced: { etf: 50, bond: 35, gold: 5, cash: 10 },
  growth: { etf: 70, bond: 20, gold: 5, cash: 5 },
};
const VOL_TONE: Record<RetireClass, string> = {
  etf: "#f59e0b",
  gold: "#f59e0b",
  crypto: "#f59e0b",
  bond: "var(--ink-2)",
  pension: "var(--ink-2)",
  estate: "var(--ink-2)",
  cash: "var(--ink-3)",
  policy: "var(--ink-3)",
};

export function className(dict: Dictionary, c: RetireClass): string {
  return {
    etf: dict.rxClassEtf,
    bond: dict.rxClassBond,
    gold: dict.rxClassGold,
    cash: dict.rxClassCash,
    policy: dict.rxClassPolicy,
    pension: dict.rxClassPension,
    estate: dict.rxClassEstate,
    crypto: dict.rxClassCrypto,
  }[c];
}
function volLabel(dict: Dictionary, c: RetireClass): string {
  return {
    etf: dict.rxVolEtf,
    bond: dict.rxVolBond,
    gold: dict.rxVolGold,
    cash: dict.rxVolCash,
    policy: dict.rxVolPolicy,
    pension: dict.rxVolPension,
    estate: dict.rxVolEstate,
    crypto: dict.rxVolCrypto,
  }[c];
}

const round1 = (v: number) => Math.round(v * 10) / 10;

function rateOf(row: RetireReturnRow, edits: ReturnsState["edits"]): number {
  const v = parseFloat(edits[row.class] ?? "");
  return Number.isFinite(v) ? v : row.returnPct;
}

// blend is the amount-weighted return, in percent.
export function blend(mix: RetireReturnRow[], amounts: Record<string, number>, edits: ReturnsState["edits"]): number {
  const total = mix.reduce((s, r) => s + (amounts[r.class] ?? 0), 0) || 1;
  return mix.reduce((s, r) => s + ((amounts[r.class] ?? 0) / total) * rateOf(r, edits), 0);
}

// spread re-distributes the movable total over pct (shares, any sum), the
// fixed classes unchanged.
export function spread(mix: RetireReturnRow[], pct: Record<string, number>): Record<string, number> {
  const out: Record<string, number> = {};
  mix.forEach((r) => (out[r.class] = r.amount));
  const movable = mix.filter((r) => r.movable).reduce((s, r) => s + r.amount, 0);
  const sum = MOVABLE.reduce((s, k) => s + (pct[k] || 0), 0) || 1;
  MOVABLE.forEach((k) => (out[k] = (movable * (pct[k] || 0)) / sum));
  return out;
}

const amountsOf = (mix: RetireReturnRow[]) => Object.fromEntries(mix.map((r) => [r.class, r.amount]));

// returnOverrides is what the "依配置計算" mode sends the projection: nothing
// while the returns and the post mix are the server's own (it blends those
// itself), else both returns recomputed here.
export function returnOverrides(mix: RetireReturnRow[] | undefined, s: ReturnsState): Record<string, string> {
  if (!mix?.length || (Object.keys(s.edits).length === 0 && s.post === "conserv")) return {};
  const amt = amountsOf(mix);
  const pre = blend(mix, amt, s.edits);
  const hasMovable = mix.some((r) => r.movable && r.amount > 0);
  const post = s.post === "keep" || !hasMovable ? pre : blend(mix, spread(mix, PRESETS[s.post]), s.edits);
  return { preR: String(round1(pre)), postR: String(round1(post)) };
}

const btnStyle = (on: boolean): React.CSSProperties => ({
  fontFamily: "var(--font-sans)",
  fontSize: 12,
  fontWeight: 600,
  minHeight: 32,
  padding: "0 11px",
  borderRadius: 7,
  cursor: "pointer",
  border: `1px solid ${on ? "var(--accent)" : "var(--border)"}`,
  background: on ? "var(--accent-tint-bg)" : "transparent",
  color: on ? "var(--accent)" : "var(--ink-2)",
});
const inputStyle: React.CSSProperties = {
  width: "100%",
  boxSizing: "border-box",
  minHeight: 30,
  padding: "0 6px",
  borderRadius: 6,
  border: "1px solid var(--border)",
  background: "var(--surface)",
  color: "var(--ink)",
  fontFamily: "var(--font-mono)",
  fontSize: 12.5,
  textAlign: "right",
  fontVariantNumeric: "tabular-nums",
};
const num: React.CSSProperties = { fontFamily: "var(--font-mono)", fontSize: 12, textAlign: "right", color: "var(--ink-2)", fontVariantNumeric: "tabular-nums" };
const GRID = "minmax(0,1.5fr) 64px 50px 78px minmax(0,1fr)";
const TRIAL_GRID = "minmax(0,1.5fr) 50px 78px 72px";
const pc = (v: number) => `${round1(v).toFixed(1)}%`;
const fundColor = (f: number) => (f >= 100 ? "var(--profit)" : f >= 80 ? "#f59e0b" : "var(--loss)");

interface Props {
  dict: Dictionary;
  plan: WealthRetire; // the saved plan: the pool's real mix
  current: WealthRetire; // what the page shows now (the live what-if, if any)
  params: Record<string, string>; // the what-if the page is on, for the trial's projection
  state: ReturnsState;
  onChange: (s: ReturnsState) => void;
  onAskAI: () => void;
}

export function RetireReturns({ dict, plan, current, params, state, onChange, onAskAI }: Props) {
  const assistant = useAssistant();
  const mix = plan.returnMix ?? [];
  const auto = state.mode === "auto";
  const rows = mix.filter((r) => r.amount > 0);
  const pool = rows.reduce((s, r) => s + r.amount, 0);
  const movableTotal = mix.filter((r) => r.movable).reduce((s, r) => s + r.amount, 0);

  const curPct: Record<string, number> = {};
  MOVABLE.forEach((k) => (curPct[k] = movableTotal > 0 ? Math.round(((mix.find((r) => r.class === k)?.amount ?? 0) / movableTotal) * 100) : 0));
  const [trialKey, setTrialKey] = useState<PresetKey | "cur" | "custom">("cur");
  const [trial, setTrial] = useState<Record<string, string> | null>(null);
  const pctS: Record<string, string> = trial ?? Object.fromEntries(MOVABLE.map((k) => [k, String(curPct[k])]));
  const pctN = Object.fromEntries(MOVABLE.map((k) => [k, parseFloat(pctS[k]) || 0]));
  const sum = MOVABLE.reduce((s, k) => s + pctN[k], 0);
  const sumOk = Math.abs(sum - 100) < 0.5;
  const amt = amountsOf(mix);
  const newAmt = trial ? spread(mix, pctN) : amt;
  const preNow = current.preReturnPct;
  const preNew = trial && sumOk ? blend(mix, newAmt, state.edits) : preNow;
  const fundNow = current.baseline?.achievementPct ?? null;

  // The trial's funded % comes from the server's projection at the trial's
  // return, over whatever else the drawer is previewing.
  const [fundNew, setFundNew] = useState<number | null>(null);
  const preNewR = round1(preNew);
  useEffect(() => {
    if (!trial || !sumOk) {
      setFundNew(null);
      return;
    }
    let cancelled = false;
    const t = setTimeout(() => {
      fetchWealthRetireLive(new URLSearchParams({ ...params, preR: String(preNewR) }))
        .then((r) => !cancelled && setFundNew(r.baseline?.achievementPct ?? null))
        .catch(() => {});
    }, 200);
    return () => {
      cancelled = true;
      clearTimeout(t);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [preNewR, sumOk, trial === null, JSON.stringify(params)]);
  const shownFundNew = trial ? fundNew : fundNow;

  const up = preNew - preNow > 0.05;
  const down = preNew - preNow < -0.05;
  const etfUp = pctN.etf - curPct.etf;
  const setEdit = (c: RetireClass, v: string) => onChange({ ...state, edits: { ...state.edits, [c]: v.replace(/[^0-9.-]/g, "") } });

  const section: React.CSSProperties = { display: "flex", flexDirection: "column", gap: 12 };
  const eyebrow: React.CSSProperties = { margin: 0, fontFamily: "var(--font-mono)", textTransform: "uppercase", letterSpacing: ".08em", fontSize: 10, fontWeight: 500, color: "var(--ink-3)" };

  return (
    <section aria-labelledby="rx-title" style={section}>
      <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
        <h3 id="rx-title" style={eyebrow}>
          {dict.rxTitle}
        </h3>
        <div role="group" aria-label={dict.rxTitle} style={{ marginLeft: "auto", display: "flex", gap: 4 }}>
          {(["auto", "manual"] as RetireMode[]).map((m) => (
            <button key={m} aria-pressed={state.mode === m} style={btnStyle(state.mode === m)} onClick={() => onChange({ ...state, mode: m })}>
              {m === "auto" ? dict.rxModeAuto : dict.rxModeManual}
            </button>
          ))}
        </div>
      </div>

      {!auto && <div style={{ fontSize: 11.5, color: "var(--ink-3)", lineHeight: 1.6 }}>{dict.rxManualNote}</div>}

      {auto && (
        <>
          <div style={{ display: "flex", flexDirection: "column", gap: 4, padding: 12, borderRadius: 9, border: "1px solid var(--border)", background: "var(--bg)" }}>
            <div style={{ display: "flex", alignItems: "baseline", gap: 8 }}>
              <span style={{ fontSize: 12, color: "var(--ink-2)" }}>{dict.rxPreLabel}</span>
              <span className="mono" style={{ marginLeft: "auto", fontSize: 22, fontWeight: 600 }}>
                {pool > 0 ? pc(current.preReturnPct) : "—"}
              </span>
            </div>
            <span style={{ fontSize: 10.5, color: "var(--ink-3)", lineHeight: 1.5 }}>{dict.rxPreSub}</span>
          </div>

          {rows.length > 0 && (
            <div role="table" aria-labelledby="rx-title" style={{ display: "flex", flexDirection: "column" }}>
              <div
                role="row"
                style={{ display: "grid", gridTemplateColumns: GRID, gap: 8, alignItems: "end", padding: "0 2px 6px", borderBottom: "1px solid var(--border)", fontFamily: "var(--font-mono)", fontSize: 9.5, letterSpacing: ".05em", color: "var(--ink-3)" }}
              >
                <span role="columnheader">{dict.rxHClass}</span>
                <span role="columnheader" style={{ textAlign: "right" }}>{dict.rxHAmount}</span>
                <span role="columnheader" style={{ textAlign: "right" }}>{dict.rxHShare}</span>
                <span role="columnheader" style={{ textAlign: "right" }}>{dict.rxHReturn}</span>
                <span role="columnheader">{dict.rxHVol}</span>
              </div>
              {rows.map((r) => {
                const name = className(dict, r.class);
                const edited = state.edits[r.class] !== undefined && parseFloat(state.edits[r.class]!) !== r.returnPct;
                return (
                  <div key={r.class} role="row" style={{ display: "grid", gridTemplateColumns: GRID, gap: 8, alignItems: "center", padding: "7px 2px", borderBottom: "1px solid var(--border)" }}>
                    <span role="cell" style={{ display: "flex", alignItems: "center", gap: 6, minWidth: 0, fontSize: 12.5, color: "var(--ink)" }}>
                      <span style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{name}</span>
                      {!r.movable && (
                        <span style={{ flexShrink: 0, fontSize: 10, padding: "0 5px", borderRadius: 4, border: "1px solid var(--border)", color: "var(--ink-3)" }}>{dict.rxFixed}</span>
                      )}
                    </span>
                    <span role="cell" style={num}>{shortTWD(r.amount)}</span>
                    <span role="cell" style={num}>{pc((r.amount / pool) * 100)}</span>
                    <span role="cell" style={{ display: "flex", alignItems: "center", gap: 3 }}>
                      <input
                        aria-label={dict.rxReturnAria.replace("{name}", name)}
                        inputMode="decimal"
                        autoComplete="off"
                        value={state.edits[r.class] ?? String(r.returnPct)}
                        onChange={(e) => setEdit(r.class, e.target.value)}
                        style={{ ...inputStyle, ...(edited ? { borderColor: "var(--accent)" } : {}) }}
                      />
                      <span aria-hidden="true" style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--ink-3)" }}>%</span>
                    </span>
                    <span role="cell" style={{ fontSize: 11, lineHeight: 1.35, color: VOL_TONE[r.class] }}>{volLabel(dict, r.class)}</span>
                  </div>
                );
              })}
            </div>
          )}
          <div style={{ fontSize: 10.5, color: "var(--ink-3)", lineHeight: 1.6 }}>{dict.rxAssumpNote}</div>

          <div style={{ display: "flex", flexDirection: "column", gap: 10, padding: 12, borderRadius: 9, border: "1px dashed var(--border)" }}>
            <div style={{ fontSize: 12.5, fontWeight: 700, color: "var(--ink)" }}>{dict.rxWhatTitle}</div>
            {movableTotal <= 0 ? (
              <div style={{ fontSize: 11.5, color: "var(--ink-3)", lineHeight: 1.6 }}>{dict.rxNoMovable}</div>
            ) : (
              <>
                <div role="group" aria-label={dict.rxWhatTitle} style={{ display: "flex", gap: 4, flexWrap: "wrap" }}>
                  {(["cur", "conserv", "balanced", "growth"] as const).map((k) => (
                    <button
                      key={k}
                      aria-pressed={trialKey === k}
                      style={btnStyle(trialKey === k)}
                      onClick={() => {
                        setTrialKey(k);
                        setTrial(k === "cur" ? null : Object.fromEntries(MOVABLE.map((c) => [c, String(PRESETS[k][c])])));
                      }}
                    >
                      {{ cur: dict.rxPresetCur, conserv: dict.rxPresetConserv, balanced: dict.rxPresetBalanced, growth: dict.rxPresetGrowth }[k]}
                    </button>
                  ))}
                </div>
                <div style={{ display: "flex", flexDirection: "column" }}>
                  <div style={{ display: "grid", gridTemplateColumns: TRIAL_GRID, gap: 8, padding: "0 2px 6px", borderBottom: "1px solid var(--border)", fontFamily: "var(--font-mono)", fontSize: 9.5, letterSpacing: ".05em", color: "var(--ink-3)" }}>
                    <span />
                    <span style={{ textAlign: "right" }}>{dict.rxHNow}</span>
                    <span style={{ textAlign: "right" }}>{dict.rxHTry}</span>
                    <span style={{ textAlign: "right" }}>{dict.rxHMove}</span>
                  </div>
                  {MOVABLE.map((k) => {
                    const name = className(dict, k);
                    const d = sumOk ? newAmt[k] - amt[k] : 0;
                    const small = Math.abs(d) < 5000;
                    return (
                      <div key={k} style={{ display: "grid", gridTemplateColumns: TRIAL_GRID, gap: 8, alignItems: "center", padding: "6px 2px", borderBottom: "1px solid var(--border)" }}>
                        <span style={{ fontSize: 12.5, color: "var(--ink)" }}>{name}</span>
                        <span style={num}>{curPct[k]}%</span>
                        <span style={{ display: "flex", alignItems: "center", gap: 3 }}>
                          <input
                            aria-label={dict.rxTrialAria.replace("{name}", name)}
                            inputMode="decimal"
                            autoComplete="off"
                            value={pctS[k]}
                            onChange={(e) => {
                              setTrialKey("custom");
                              setTrial({ ...pctS, [k]: e.target.value.replace(/[^0-9.]/g, "") });
                            }}
                            style={inputStyle}
                          />
                          <span aria-hidden="true" style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--ink-3)" }}>%</span>
                        </span>
                        <span style={{ ...num, fontSize: 12.5, color: small ? "var(--ink-3)" : d > 0 ? "var(--ink)" : "var(--ink-2)" }}>
                          {small ? "—" : `${d > 0 ? "+" : "−"}${shortTWD(Math.abs(d))}`}
                        </span>
                      </div>
                    );
                  })}
                </div>
                {!sumOk && (
                  <div role="alert" style={{ fontSize: 11.5, fontWeight: 600, color: "var(--loss)" }}>
                    {dict.rxSumBad.replace("{n}", String(round1(sum)))}
                  </div>
                )}
                {sumOk && (
                  <div style={{ display: "flex", gap: 18, flexWrap: "wrap" }}>
                    <div style={{ display: "flex", flexDirection: "column", gap: 2 }}>
                      <span style={{ fontSize: 11, color: "var(--ink-3)" }}>{dict.rxResReturn}</span>
                      <span style={{ display: "flex", alignItems: "baseline", gap: 6 }}>
                        <span className="mono" style={{ fontSize: 12.5, color: "var(--ink-3)" }}>{pc(preNow)} →</span>
                        <span className="mono" style={{ fontSize: 17, fontWeight: 600, color: up ? "var(--profit)" : down ? "var(--loss)" : "var(--ink)" }}>{pc(preNew)}</span>
                      </span>
                    </div>
                    <div style={{ display: "flex", flexDirection: "column", gap: 2 }}>
                      <span style={{ fontSize: 11, color: "var(--ink-3)" }}>{dict.rxResFunded}</span>
                      <span style={{ display: "flex", alignItems: "baseline", gap: 6 }}>
                        <span className="mono" style={{ fontSize: 12.5, color: "var(--ink-3)" }}>{fundNow != null ? `${Math.round(fundNow)}%` : "—"} →</span>
                        <span className="mono" style={{ fontSize: 17, fontWeight: 600, color: shownFundNew != null ? fundColor(shownFundNew) : "var(--ink-3)" }}>
                          {shownFundNew != null ? `${Math.round(shownFundNew)}%` : "—"}
                        </span>
                      </span>
                    </div>
                  </div>
                )}
                {sumOk && etfUp >= 10 && (
                  <div style={{ fontSize: 11.5, lineHeight: 1.6, color: "var(--ink-2)", padding: "8px 10px", borderRadius: 8, background: "var(--bg)" }}>
                    {dict.rxRisk.replace("{n}", String(Math.round(etfUp)))}
                  </div>
                )}
                <div style={{ fontSize: 10.5, color: "var(--ink-3)", lineHeight: 1.6 }}>{dict.rxWhatNote.replace("{m}", shortTWD(movableTotal))}</div>
                {assistant && (
                  <button
                    onClick={() => {
                      onAskAI();
                      assistant.open(
                        "retire",
                        dict.rxAiAsk.replace("{etf}", pctS.etf).replace("{bond}", pctS.bond).replace("{gold}", pctS.gold).replace("{cash}", pctS.cash),
                      );
                    }}
                    style={{ alignSelf: "flex-start", display: "inline-flex", alignItems: "center", gap: 6, fontFamily: "var(--font-sans)", fontSize: 12, fontWeight: 700, padding: "7px 12px", borderRadius: 7, cursor: "pointer", border: "1px solid var(--accent-tint-border)", background: "var(--accent-tint-bg)", color: "var(--accent)" }}
                  >
                    {dict.rxAiLabel}
                  </button>
                )}
              </>
            )}
          </div>

          <div style={{ display: "flex", flexDirection: "column", gap: 8, padding: 12, borderRadius: 9, border: "1px solid var(--border)", background: "var(--bg)" }}>
            <div style={{ display: "flex", alignItems: "baseline", gap: 8 }}>
              <span style={{ fontSize: 12, color: "var(--ink-2)" }}>{dict.rxPostTitle}</span>
              <span className="mono" style={{ marginLeft: "auto", fontSize: 16, fontWeight: 600 }}>
                {pool > 0 ? pc(current.postReturnPct) : "—"}
              </span>
            </div>
            <div role="group" aria-label={dict.rxPostTitle} style={{ display: "flex", gap: 4, flexWrap: "wrap" }}>
              {(["conserv", "balanced", "keep"] as PostKey[]).map((k) => (
                <button key={k} aria-pressed={state.post === k} style={btnStyle(state.post === k)} onClick={() => onChange({ ...state, post: k })}>
                  {{ conserv: dict.rxPresetConserv, balanced: dict.rxPresetBalanced, keep: dict.rxPostKeep }[k]}
                </button>
              ))}
            </div>
            <span style={{ fontSize: 10.5, color: "var(--ink-3)", lineHeight: 1.5 }}>{dict.rxPostSub}</span>
          </div>
          <div style={{ fontSize: 10.5, color: "var(--ink-3)" }}>{dict.rxDisclaimer}</div>
        </>
      )}
    </section>
  );
}
