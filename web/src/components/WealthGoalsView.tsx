import { useEffect, useState } from "react";
import { fetchWealthGoals, type Goal } from "../api";
import type { Dictionary } from "../i18n";
import { fmtMoney } from "./WealthHomeView";

interface Props {
  dict: Dictionary;
  onNavigate: (path: string) => void;
}

const CURRENCY = "NT$"; // display currency fixed to TWD, same known gap as WealthHomeView/WealthCashView

function statusLabel(dict: Dictionary, status: Goal["status"]): string {
  switch (status) {
    case "ahead":
      return dict.wealthGoalsStatusAhead;
    case "behind":
      return dict.wealthGoalsStatusBehind;
    case "onTrack":
      return dict.wealthGoalsStatusOnTrack;
    default:
      return "";
  }
}

// StatusChip mirrors the template's wrm.goal.stateStyle — a colored pill,
// not plain text (Argus Trading WebUI.dc.html lines 1129/1171/1189).
function StatusChip({ dict, status }: { dict: Dictionary; status: Goal["status"] }) {
  if (!status) return null;
  return <span className={`goal-state-chip ${status}`}>{statusLabel(dict, status)}</span>;
}

// GoalProgressBar mirrors the template's bar+mark pair (lines 1132/1174/
// 1193, goalRow()'s barStyle/markStyle) — a track, a fill colored by status
// (ahead=profit, behind=loss, on-track-or-unknown=accent — the template
// never colors this gray), and, when the goal has a targetDate, a tick at
// markPct showing where progress "should" be today.
function GoalProgressBar({
  dict,
  pct,
  markPct,
  status,
}: {
  dict: Dictionary;
  pct: number;
  markPct?: number;
  status: Goal["status"];
}) {
  const fillClass = status === "ahead" ? "profit" : status === "behind" ? "loss" : "accent";
  return (
    <div className="goal-progress-track">
      <span className={`goal-progress-fill ${fillClass}`} style={{ width: `${pct}%` }} />
      {markPct != null && (
        <span
          className="goal-progress-mark"
          title={dict.wealthGoalsExpectedLabel}
          style={{ left: `${Math.max(0, Math.min(100, markPct))}%` }}
        />
      )}
    </div>
  );
}

// GoalCard mirrors the template's goal row exactly (lines 1166-1201): the
// big (retirement) row puts note inline in the header next to the name
// (line 1170), the small grid cards put it on its own line below the header
// instead (line 1192) — two different layouts for the same field, not an
// oversight, so `big` picks which one renders. Progress bar with the
// expected-progress mark, then a Saved/Target/Monthly/ETA stat line — no
// write affordances anywhere, since the template's own /w/goals section has
// none (unlike /w/cash's "+" drawer trigger, this page's header is just a
// "see retirement" link). `big` also renders the enlarged "glow" card the
// template reserves for the retirement row (lines 1166-1182) instead of the
// plain grid card (lines 1186-1200). Monthly is real data only for the
// retirement row (goal.monthlyContribution, read from the same
// profile.retirement_monthly_contribution setting /w/retire uses) — general
// goals stay "—": the goals schema (§8.8/§9.2) has no monthly-contribution
// column for them, so there's nothing to compute it from yet. ETA falls back
// to the user's own targetDate rather than a projected completion date,
// since that projection also needs a monthly figure most goals don't have.
function GoalCard({ dict, goal, big }: { dict: Dictionary; goal: Goal; big: boolean }) {
  const pct = goal.progressPct != null ? Math.max(0, Math.min(100, goal.progressPct)) : 0;
  return (
    <div className={`card${big ? " card--glow" : ""}`} style={{ display: "flex", flexDirection: "column", gap: 9 }}>
      <div style={{ display: "flex", alignItems: "baseline", gap: 10, flexWrap: "wrap" }}>
        <span style={{ fontFamily: "var(--sans)", fontSize: big ? 16 : 13.5 }}>{goal.name}</span>
        {big && goal.note && <span style={{ fontSize: 11.5, color: "var(--ink-3)" }}>{goal.note}</span>}
        <StatusChip dict={dict} status={goal.status} />
        <span className="mono" style={{ marginLeft: "auto", fontSize: big ? 24 : 17 }}>
          {goal.progressPct != null ? `${goal.progressPct.toFixed(1)}%` : "—"}
        </span>
      </div>
      {!big && goal.note && <div style={{ fontSize: 11.5, color: "var(--ink-3)" }}>{goal.note}</div>}
      <GoalProgressBar dict={dict} pct={pct} markPct={goal.markPct} status={goal.status} />
      <div style={{ display: "flex", gap: big ? 22 : 18, flexWrap: "wrap", fontFamily: "var(--font-mono)", fontSize: big ? 11 : 10.5, color: "var(--ink-3)" }}>
        <span>
          {dict.wealthGoalsSavedLabel} <span style={{ color: "var(--ink)" }}>{goal.saved != null ? fmtMoney(goal.saved, CURRENCY) : "—"}</span>
        </span>
        <span>
          {dict.wealthGoalsTargetLabel} <span style={{ color: "var(--ink)" }}>{fmtMoney(goal.targetAmount, CURRENCY)}</span>
        </span>
        <span>
          {dict.wealthGoalsMonthlyLabel}{" "}
          <span style={{ color: "var(--ink)" }}>{goal.monthlyContribution != null ? fmtMoney(goal.monthlyContribution, CURRENCY) : "—"}</span>
        </span>
        <span>
          {dict.wealthGoalsEtaLabel} <span style={{ color: "var(--ink)" }}>{goal.targetDate || "—"}</span>
        </span>
      </div>
    </div>
  );
}

export function WealthGoalsView({ dict, onNavigate }: Props) {
  const [goals, setGoals] = useState<Goal[] | null>(null);
  const [error, setError] = useState(false);

  useEffect(() => {
    setError(false);
    fetchWealthGoals()
      .then((g) => setGoals(g.goals))
      .catch(() => setError(true));
  }, []);

  if (error) {
    return <div className="error-message">{dict.error}</div>;
  }

  const bigGoal = goals?.find((g) => g.kind === "retirement");
  const otherGoals = goals?.filter((g) => g.id !== bigGoal?.id) ?? [];
  const totalSaved = goals?.reduce((sum, g) => (g.saved != null ? sum + g.saved : sum), 0) ?? 0;
  const totalTarget = goals?.reduce((sum, g) => sum + g.targetAmount, 0) ?? 0;
  const totalPct = totalTarget > 0 ? (totalSaved / totalTarget) * 100 : null;
  const behindCount = goals?.filter((g) => g.status === "behind").length ?? 0;

  return (
    <>
      {/* Unboxed section header, matching every other wealth page in the
          template (isWAlloc/isWBalance/isWCash all open with the same
          flex row before any card) rather than the eyebrow-in-first-card
          shape this page used before its template pass. */}
      <div style={{ display: "flex", alignItems: "center", gap: 12, margin: "16px 0", flexWrap: "wrap" }}>
        <span style={{ fontFamily: "var(--font-mono)", textTransform: "uppercase", letterSpacing: ".08em", fontSize: 11, color: "var(--ink)" }}>
          {dict.navWealthGoals}
        </span>
        <a href="#" style={{ marginLeft: "auto", fontFamily: "var(--font-mono)", fontSize: 11 }} onClick={(e) => { e.preventDefault(); onNavigate("/w/retire"); }}>
          {dict.wealthGoalsSeeRetireLink}
        </a>
      </div>

      {goals && goals.length === 0 && <div className="card empty-message">{dict.wealthGoalsNoGoals}</div>}

      {goals && goals.length > 0 && (
        <div style={{ display: "flex", flexWrap: "wrap", gap: 16, marginBottom: 16 }}>
          <div className="card" style={{ flex: "1 1 200px" }}>
            <div className="eyebrow">{dict.wealthGoalsTotalProgress}</div>
            <div className="mono" style={{ fontSize: "clamp(17px,2.1vw,28px)", marginTop: 8 }}>
              {totalPct != null ? `${totalPct.toFixed(1)}%` : "—"}
            </div>
            <div style={{ fontSize: 11, color: "var(--ink-3)", marginTop: 6 }}>
              {fmtMoney(totalSaved, CURRENCY)} / {fmtMoney(totalTarget, CURRENCY)}
            </div>
          </div>
          <div className="card" style={{ flex: "1 1 200px" }}>
            <div className="eyebrow">{dict.wealthGoalsMonthlyLabel}</div>
            <div className="mono" style={{ fontSize: "clamp(17px,2.1vw,28px)", marginTop: 8 }}>—</div>
          </div>
          <div className="card" style={{ flex: "1 1 200px" }}>
            <div className="eyebrow">{dict.wealthGoalsBehindCountLabel}</div>
            <div className={`mono ${behindCount > 0 ? "loss" : "profit"}`} style={{ fontSize: "clamp(17px,2.1vw,28px)", marginTop: 8 }}>
              {behindCount}
            </div>
          </div>
        </div>
      )}

      {bigGoal && (
        <div style={{ marginBottom: 16 }}>
          <GoalCard dict={dict} goal={bigGoal} big />
        </div>
      )}

      {otherGoals.length > 0 && (
        <div className="goal-grid">
          {otherGoals.map((goal) => (
            <GoalCard key={goal.id} dict={dict} goal={goal} big={false} />
          ))}
        </div>
      )}
    </>
  );
}
