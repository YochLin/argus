import { useEffect, useState } from "react";
import {
  ApiError,
  fetchWealthProfile,
  fetchWealthRetire,
  saveWealthProfile,
  saveWealthRetire,
  type RetirementScenario,
  type WealthRetire,
} from "../api";
import type { Dictionary } from "../i18n";
import { fmtMoney } from "./WealthHomeView";

interface Props {
  dict: Dictionary;
  writable: boolean;
  onUnauthorized: (retry: () => void) => void;
  onNavigate: (path: string) => void;
}

const CURRENCY = "NT$"; // display currency fixed to TWD, same known gap as every other wealth page

// rateColor mirrors retireModel()'s rateStyle exactly: profit once funded,
// the template's own amber (#f59e0b) between 80-100%, loss below that.
function rateColor(pct: number, funded: boolean): string {
  if (funded) return "var(--profit)";
  if (pct >= 80) return "#f59e0b";
  return "var(--loss)";
}

// QuickSwitchGroup mirrors the template's wrm.ageBtns/spendBtns — a row of
// pill buttons, the active one outlined in accent (Argus Trading WebUI.dc.html
// lines 4375-4386), each click saving immediately ("就地重算", §8.7).
function QuickSwitchGroup<T extends number>({
  label,
  options,
  value,
  format,
  onSelect,
}: {
  label: string;
  options: T[];
  value: T;
  format: (v: T) => string;
  onSelect: (v: T) => void;
}) {
  return (
    <>
      <span style={{ fontFamily: "var(--font-mono)", fontSize: 10, letterSpacing: ".06em", color: "var(--ink-3)" }}>{label}</span>
      <div style={{ display: "flex", gap: 6 }}>
        {options.map((opt) => {
          const active = opt === value;
          return (
            <button
              key={opt}
              onClick={() => onSelect(opt)}
              style={{
                fontFamily: "var(--font-mono)",
                fontSize: 11,
                padding: "5px 11px",
                borderRadius: 7,
                cursor: "pointer",
                border: `1px solid ${active ? "var(--accent)" : "var(--border)"}`,
                background: active ? "var(--accent-tint-bg)" : "var(--surface)",
                color: active ? "var(--accent)" : "var(--ink-3)",
              }}
            >
              {format(opt)}
            </button>
          );
        })}
      </div>
    </>
  );
}

function ScenarioRow({ dict, label, scenario }: { dict: Dictionary; label: string; scenario: RetirementScenario }) {
  return (
    <div style={{ display: "flex", alignItems: "baseline", gap: 10, flexWrap: "wrap" }}>
      <span style={{ fontFamily: "var(--sans)", fontSize: 12.5, flex: "1 1 160px" }}>{label}</span>
      <span className="mono" style={{ fontSize: 12.5, color: rateColor(scenario.achievementPct, scenario.funded) }}>
        {Math.min(scenario.achievementPct, 999).toFixed(0)}%
      </span>
      <span className="mono" style={{ fontSize: 11, color: "var(--ink-3)", minWidth: 90, textAlign: "right" }}>
        {scenario.depletionAge != null ? `${scenario.depletionAge} 歲` : dict.wealthRetireDepleteNever}
      </span>
    </div>
  );
}

// RetireChart mirrors retireModel()'s SVG (lines 1106-1116): the baseline
// balance path, a dashed horizontal "need" reference line, and a dashed
// vertical line at the retirement year — plain straight-segment polylines
// rather than the template's smoothed area fill, which needs no library and
// reads identically at this data density.
function RetireChart({ path, need, retirementYear }: { path: { year: number; balance: number }[]; need: number; retirementYear?: number }) {
  if (path.length < 2) return null;
  const width = 600;
  const height = 170;
  const maxV = Math.max(...path.map((p) => p.balance), need) * 1.06;
  const x = (i: number) => (i / (path.length - 1)) * width;
  const y = (v: number) => height - (v / maxV) * height;
  const points = path.map((p, i) => `${x(i).toFixed(1)},${y(p.balance).toFixed(1)}`).join(" ");
  const needY = y(need).toFixed(1);
  const retIdx = retirementYear != null ? path.findIndex((p) => p.year === retirementYear) : -1;

  return (
    <svg viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="none" style={{ width: "100%", height: 200, display: "block" }}>
      <line x1={0} x2={width} y1={needY} y2={needY} stroke="#f59e0b" strokeWidth={1.5} strokeDasharray="5 4" vectorEffect="non-scaling-stroke" />
      {retIdx >= 0 && (
        <line x1={x(retIdx)} x2={x(retIdx)} y1={0} y2={height} stroke="var(--ink-3)" strokeWidth={1} strokeDasharray="3 3" vectorEffect="non-scaling-stroke" />
      )}
      <polyline points={points} fill="none" stroke="var(--accent)" strokeWidth={2} vectorEffect="non-scaling-stroke" />
    </svg>
  );
}

export function WealthRetireView({ dict, writable, onUnauthorized, onNavigate }: Props) {
  const [retire, setRetire] = useState<WealthRetire | null>(null);
  const [error, setError] = useState(false);
  const [refreshSignal, setRefreshSignal] = useState(0);
  const [birthYearInput, setBirthYearInput] = useState("");
  const [contribInput, setContribInput] = useState("");
  const [savingSetup, setSavingSetup] = useState(false);

  useEffect(() => {
    setError(false);
    fetchWealthRetire()
      .then(setRetire)
      .catch(() => setError(true));
  }, [refreshSignal]);

  async function saveQuickSwitch(age: number, spend: number) {
    try {
      const updated = await saveWealthRetire(dict.navWealthRetire, age, spend);
      setRetire(updated);
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        onUnauthorized(() => saveQuickSwitch(age, spend));
      } else {
        window.alert(e instanceof ApiError ? e.message : dict.error);
      }
    }
  }

  async function saveSetup() {
    const birthYear = Number(birthYearInput);
    const contribution = contribInput.trim() === "" ? 0 : Number(contribInput);
    if (!Number.isFinite(birthYear) || birthYear < 1900 || Number.isNaN(contribution) || contribution < 0) return;
    setSavingSetup(true);
    try {
      await fetchWealthProfile(); // ensure the profile endpoint exists before a partial save (also warms auth state)
      await saveWealthProfile({ birthYear, retirementMonthlyContribution: contribution });
      setRefreshSignal((n) => n + 1);
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        onUnauthorized(saveSetup);
      } else {
        window.alert(e instanceof ApiError ? e.message : dict.error);
      }
    } finally {
      setSavingSetup(false);
    }
  }

  if (error) {
    return <div className="error-message">{dict.error}</div>;
  }

  return (
    <>
      {/* Unboxed header + quick-switch buttons, matching the template's
          isWRetire row exactly (Argus Trading WebUI.dc.html lines 1060-1074)
          — retirement age and monthly spend each save immediately on click
          rather than needing a separate "save" step. */}
      <div style={{ display: "flex", alignItems: "center", gap: 12, margin: "0 0 16px", flexWrap: "wrap" }}>
        <span style={{ fontFamily: "var(--font-mono)", textTransform: "uppercase", letterSpacing: ".08em", fontSize: 11, color: "var(--ink)" }}>
          {dict.navWealthRetire}
        </span>
        {writable && retire?.hasBirthYear && (
          <div style={{ marginLeft: "auto", display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
            <QuickSwitchGroup
              label={dict.wealthRetireTargetAgeLabel}
              options={retire.retirementAgeOptions}
              value={retire.retirementAge}
              format={(v) => `${v} 歲`}
              onSelect={(age) => saveQuickSwitch(age, retire.monthlySpend)}
            />
            <QuickSwitchGroup
              label={dict.wealthRetireSpendLabel}
              options={retire.monthlySpendOptions}
              value={retire.monthlySpend}
              format={(v) => fmtMoney(v, CURRENCY)}
              onSelect={(spend) => saveQuickSwitch(retire.retirementAge, spend)}
            />
          </div>
        )}
      </div>

      {writable && retire && !retire.hasBirthYear && (
        <div className="card" style={{ marginBottom: 16 }}>
          <div className="eyebrow">{dict.wealthRetireSetupTitle}</div>
          <div style={{ display: "flex", gap: 10, flexWrap: "wrap", marginTop: 10 }}>
            <label className="form-field" style={{ width: 140 }}>
              <span>{dict.wealthRetireBirthYearLabel}</span>
              <input className="mono" type="number" value={birthYearInput} onChange={(e) => setBirthYearInput(e.target.value)} />
            </label>
            <label className="form-field" style={{ width: 180 }}>
              <span>{dict.wealthRetireContribInputLabel}</span>
              <input className="mono" type="number" value={contribInput} onChange={(e) => setContribInput(e.target.value)} />
            </label>
          </div>
          <div className="modal-actions">
            <button className="btn-primary" disabled={savingSetup || birthYearInput.trim() === ""} onClick={saveSetup}>
              {dict.wealthRetireSetupSave}
            </button>
          </div>
        </div>
      )}

      <div style={{ display: "flex", flexWrap: "wrap", gap: 16, marginBottom: 16 }}>
        <div className="card card--glow" style={{ flex: "1.5 1 250px" }}>
          <div className="eyebrow">{dict.wealthRetireRateLabel}</div>
          <div className="mono" style={{ fontSize: 40, lineHeight: 1.1, marginTop: 8, color: retire?.baseline ? rateColor(retire.baseline.achievementPct, retire.baseline.funded) : undefined }}>
            {retire?.baseline ? `${Math.min(retire.baseline.achievementPct, 999).toFixed(0)}%` : "—"}
          </div>
          <div style={{ fontSize: 13, color: "var(--ink-2)", marginTop: 4 }}>
            {dict.wealthRetireGapLabel}{" "}
            <span className="mono" style={{ color: retire?.baseline == null ? undefined : retire.baseline.funded ? "var(--profit)" : "var(--loss)" }}>
              {retire?.baseline == null ? "—" : retire.baseline.funded ? dict.wealthRetireFundedLabel : fmtMoney(retire.baseline.gapAmount, CURRENCY)}
            </span>
          </div>
          <div style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--ink-3)", marginTop: 8 }}>
            {dict.wealthRetirePoolLabel} {retire?.pool != null ? fmtMoney(retire.pool, CURRENCY) : "—"} · {dict.wealthRetireContribLabel}{" "}
            {retire ? fmtMoney(retire.monthlyContribution, CURRENCY) : "—"}
          </div>
        </div>
        <div className="card" style={{ flex: "1 1 200px" }}>
          <div className="eyebrow">{dict.wealthRetireProjLabel}</div>
          <div className="mono" style={{ fontSize: "clamp(17px,2.1vw,28px)", marginTop: 8 }}>
            {retire?.baseline != null ? fmtMoney(retire.baseline.projectedAtRetirement, CURRENCY) : "—"}
          </div>
        </div>
        <div className="card" style={{ flex: "1 1 200px" }}>
          <div className="eyebrow">{dict.wealthRetireNeedLabel}</div>
          <div className="mono" style={{ fontSize: "clamp(17px,2.1vw,28px)", marginTop: 8 }}>
            {retire?.need != null ? fmtMoney(retire.need, CURRENCY) : "—"}
          </div>
          <div style={{ fontSize: 11, color: "var(--ink-3)", marginTop: 6 }}>
            {dict.wealthRetireSpendLabel} {retire ? fmtMoney(retire.monthlySpend, CURRENCY) : "—"}
          </div>
        </div>
        <div className="card" style={{ flex: "1 1 200px" }}>
          <div className="eyebrow">{dict.wealthRetireDepleteLabel}</div>
          <div className="mono" style={{ fontSize: 19, marginTop: 10, lineHeight: 1.35 }}>
            {retire?.baseline == null
              ? "—"
              : retire.baseline.depletionAge != null
                ? `${retire.baseline.depletionAge} 歲`
                : dict.wealthRetireDepleteNever}
          </div>
        </div>
      </div>

      {retire?.baseline && retire.need != null && (
        <div className="card" style={{ marginBottom: 16 }}>
          <div style={{ display: "flex", alignItems: "baseline", gap: 14, marginBottom: 14, flexWrap: "wrap" }}>
            <span className="eyebrow">{dict.wealthRetireChartLabel}</span>
            <span style={{ marginLeft: "auto", display: "flex", gap: 14 }}>
              <span style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 11, color: "var(--ink-3)" }}>
                <span style={{ width: 14, height: 2, background: "var(--accent)", display: "inline-block" }} />
                {dict.wealthRetireProjLabel}
              </span>
              <span style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 11, color: "var(--ink-3)" }}>
                <span style={{ width: 14, height: 2, background: "#f59e0b", display: "inline-block" }} />
                {dict.wealthRetireNeedLabel}
              </span>
            </span>
          </div>
          <RetireChart path={retire.path} need={retire.need} retirementYear={retire.retirementYear} />

          {/* Three §10.2② scenarios as a compact table rather than three
              overlaid chart lines — the achievement%/depletion-age numbers
              are the decision-relevant output; a third/fourth polyline on
              top of the baseline path reads as clutter at this size. */}
          {retire.crash && retire.lowReturn && (
            <div style={{ display: "flex", flexDirection: "column", gap: 8, marginTop: 16, paddingTop: 14, borderTop: "1px solid var(--border)" }}>
              <ScenarioRow dict={dict} label={dict.wealthRetireScenarioBaseline} scenario={retire.baseline} />
              <ScenarioRow dict={dict} label={dict.wealthRetireScenarioCrash} scenario={retire.crash} />
              <ScenarioRow dict={dict} label={dict.wealthRetireScenarioLowReturn} scenario={retire.lowReturn} />
            </div>
          )}

          <div style={{ fontSize: 11, color: "var(--ink-3)", lineHeight: 1.6, marginTop: 14 }}>{dict.wealthRetireAssumption}</div>
        </div>
      )}

      {retire?.goal && (
        <div className="card">
          <div style={{ display: "flex", alignItems: "baseline", gap: 10, marginBottom: 14 }}>
            <span className="eyebrow">{dict.wealthRetireGoalProgressLabel}</span>
            <a href="#" style={{ marginLeft: "auto", fontFamily: "var(--font-mono)", fontSize: 11 }} onClick={(e) => { e.preventDefault(); onNavigate("/w/goals"); }}>
              {dict.wealthRetireSeeGoalsLink}
            </a>
          </div>
          <div style={{ display: "flex", gap: 22, flexWrap: "wrap", fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--ink-3)" }}>
            <span>
              {dict.wealthGoalsSavedLabel} <span style={{ color: "var(--ink)" }}>{retire.goal.saved != null ? fmtMoney(retire.goal.saved, CURRENCY) : "—"}</span>
            </span>
            <span>
              {dict.wealthGoalsTargetLabel} <span style={{ color: "var(--ink)" }}>{fmtMoney(retire.goal.targetAmount, CURRENCY)}</span>
            </span>
            <span>
              {dict.wealthGoalsMonthlyLabel} <span style={{ color: "var(--ink)" }}>{fmtMoney(retire.monthlyContribution, CURRENCY)}</span>
            </span>
            <span>
              {dict.wealthGoalsEtaLabel} <span style={{ color: "var(--ink)" }}>{retire.retirementAge} 歲</span>
            </span>
          </div>
        </div>
      )}
    </>
  );
}
