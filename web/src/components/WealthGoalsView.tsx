import { useEffect, useState } from "react";
import * as Dialog from "@radix-ui/react-dialog";
import { ApiError, createWealthGoal, deleteWealthGoal, fetchWealthGoals, updateWealthGoal, type Goal, type NewGoal } from "../api";
import type { Dictionary } from "../i18n";
import { fmtMoney } from "./WealthHomeView";

interface Props {
  dict: Dictionary;
  writable: boolean;
  onUnauthorized: (retry: () => void) => void;
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

// GOAL_KINDS mirrors the design template's goalKinds() — a preset only
// prefills name/note/target/monthly/span (years to the target); nothing about
// the chip is stored, so an edit re-detects it by label like the template's
// openGoalEdit does. "custom" unlocks the name field.
type StringKey = { [K in keyof Dictionary]: Dictionary[K] extends string ? K : never }[keyof Dictionary];
type KindId = "edu" | "home" | "emg" | "travel" | "car" | "study" | "wed" | "custom";
const GOAL_KINDS: Array<{ id: KindId; label: StringKey; note?: StringKey; target: number; mo: number; span: number }> = [
  { id: "edu", label: "wealthGoalsKindEdu", note: "wealthGoalsKindEduNote", target: 3000000, mo: 15000, span: 10 },
  { id: "home", label: "wealthGoalsKindHome", note: "wealthGoalsKindHomeNote", target: 5000000, mo: 22000, span: 7 },
  { id: "emg", label: "wealthGoalsKindEmg", note: "wealthGoalsKindEmgNote", target: 1200000, mo: 8000, span: 3 },
  { id: "travel", label: "wealthGoalsKindTravel", note: "wealthGoalsKindTravelNote", target: 600000, mo: 6000, span: 4 },
  { id: "car", label: "wealthGoalsKindCar", note: "wealthGoalsKindCarNote", target: 900000, mo: 12000, span: 5 },
  { id: "study", label: "wealthGoalsKindStudy", note: "wealthGoalsKindStudyNote", target: 800000, mo: 10000, span: 4 },
  { id: "wed", label: "wealthGoalsKindWed", note: "wealthGoalsKindWedNote", target: 800000, mo: 12000, span: 3 },
  { id: "custom", label: "wealthGoalsKindCustom", target: 1000000, mo: 5000, span: 5 },
];

const yearOf = (date?: string) => (date ? date.slice(0, 4) : "");
const num = (v: string, fallback: number) => {
  const n = parseFloat(v.replace(/[,\s]/g, ""));
  return Number.isNaN(n) ? fallback : n;
};

// GoalDrawer is the design's goalEditModel drawer on the shared
// wealth-drawer shell (same as WealthInsureView's AddPolicyForm): kind chips,
// name (custom only), note, the five numbers, and a live progress/remaining/
// pace footer. `goal` null = add; a goal = edit (with delete).
function GoalDrawer({
  dict,
  goal,
  onClose,
  onUnauthorized,
  onSaved,
}: {
  dict: Dictionary;
  goal: Goal | null;
  onClose: () => void;
  onUnauthorized: (retry: () => void) => void;
  onSaved: () => void;
}) {
  const thisYear = new Date().getFullYear();
  const initialKind = goal ? (GOAL_KINDS.find((k) => k.id !== "custom" && dict[k.label] === goal.name)?.id ?? "custom") : null;
  const [kind, setKind] = useState<KindId | null>(initialKind);
  const [name, setName] = useState(goal?.name ?? "");
  const [note, setNote] = useState(goal?.note ?? "");
  const [target, setTarget] = useState(goal ? String(goal.targetAmount) : "1000000");
  // Editing a goal whose monthly/start were never typed leaves them blank
  // (saved as "unset") instead of prefilling the add-mode defaults.
  const [saved, setSaved] = useState(!goal ? "0" : goal.saved != null ? String(Math.round(goal.saved)) : "");
  const [monthly, setMonthly] = useState(!goal ? "5000" : goal.monthlyContribution != null ? String(goal.monthlyContribution) : "");
  const [start, setStart] = useState(!goal ? String(thisYear) : goal.startYear ? String(goal.startYear) : "");
  const [eta, setEta] = useState(yearOf(goal?.targetDate) || String(thisYear + 4));
  const [noteTouched, setNoteTouched] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function pickKind(k: (typeof GOAL_KINDS)[number]) {
    setKind(k.id);
    if (k.id !== "custom") {
      setName(dict[k.label]);
      if (!note || !noteTouched) setNote(k.note ? dict[k.note] : "");
    }
    if (!goal) {
      setTarget(String(k.target));
      setMonthly(String(k.mo));
      setStart(String(thisYear));
      setEta(String(thisYear + k.span));
    }
  }

  const targetN = Math.max(num(target, 0), 1);
  const savedN = num(saved, 0);
  const monthlyN = num(monthly, 0);
  const remain = Math.max(targetN - savedN, 0);
  const months = monthlyN > 0 ? Math.ceil(remain / monthlyN) : null;
  const doneYear = months === null ? null : thisYear + Math.floor(months / 12);
  const pace =
    months === null ? dict.wealthGoalsPaceNone : remain === 0 ? dict.wealthGoalsPaceDone : dict.wealthGoalsPaceAt.replace("%s", String(doneYear));
  const late = doneYear !== null && remain > 0 && doneYear > num(eta, 9999);

  async function run(action: () => Promise<unknown>) {
    setBusy(true);
    setError(null);
    try {
      await action();
      onSaved();
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        onUnauthorized(() => run(action));
      } else {
        setError(e instanceof ApiError ? e.message : dict.error);
      }
    } finally {
      setBusy(false);
    }
  }

  function save() {
    const body: NewGoal = {
      name: name.trim() || dict.wealthGoalsKindCustom,
      note: note.trim(),
      targetAmount: targetN,
      targetDate: `${Math.round(num(eta, thisYear))}-12-31`,
      savedAmount: saved.trim() ? Math.max(savedN, 0) : undefined,
      monthlyContribution: monthly.trim() ? Math.max(monthlyN, 0) : undefined,
      startYear: start.trim() ? Math.round(num(start, thisYear)) : undefined,
    };
    run(() => (goal ? updateWealthGoal(goal.id, body) : createWealthGoal(body)));
  }

  const field = (label: string, value: string, set: (v: string) => void, opts?: { wide?: boolean; mono?: boolean; placeholder?: string }) => (
    <div className="wealth-drawer-field" style={opts?.wide ? { gridColumn: "1 / -1" } : undefined}>
      <span className="wealth-drawer-field-label">{label}</span>
      <input className={opts?.mono ? "mono" : undefined} value={value} placeholder={opts?.placeholder} onChange={(e) => set(e.target.value)} />
    </div>
  );

  return (
    <Dialog.Root open onOpenChange={(open) => !open && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="wealth-drawer-overlay">
          <Dialog.Content className="wealth-drawer-panel" aria-describedby={undefined} onOpenAutoFocus={(e) => e.preventDefault()}>
            <div className="wealth-drawer-header">
              <Dialog.Title style={{ fontFamily: "var(--font-mono)", textTransform: "uppercase", letterSpacing: ".08em", fontSize: 11 }}>
                {goal ? dict.wealthGoalsEditTitle : dict.wealthGoalsAddTitle}
              </Dialog.Title>
              <Dialog.Close className="modal-close" style={{ marginLeft: "auto" }} aria-label="close">
                ×
              </Dialog.Close>
            </div>
            <div className="wealth-drawer-body">
              <div className="wealth-drawer-field" style={{ gridColumn: "1 / -1" }}>
                <span className="wealth-drawer-field-label">{dict.wealthGoalsKindLabel}</span>
                <div style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
                  {GOAL_KINDS.map((k) => (
                    <button key={k.id} type="button" className={`goal-kind-chip${kind === k.id ? " active" : ""}`} onClick={() => pickKind(k)}>
                      {dict[k.label]}
                    </button>
                  ))}
                </div>
              </div>
              {kind === "custom" && field(dict.wealthGoalsNameLabel, name, setName, { placeholder: dict.wealthGoalsNamePlaceholder })}
              <div className="goal-drawer-grid">
                {field(dict.wealthGoalsNoteLabel, note, (v) => { setNote(v); setNoteTouched(true); }, { wide: true })}
                {field(dict.wealthGoalsTargetLabel, target, setTarget, { mono: true })}
                {field(dict.wealthGoalsSavedLabel, saved, setSaved, { mono: true })}
                {field(dict.wealthGoalsMonthlyLabel, monthly, setMonthly, { mono: true })}
                {field(dict.wealthGoalsStartYearLabel, start, setStart, { mono: true })}
                {field(dict.wealthGoalsEtaYearLabel, eta, setEta, { mono: true })}
              </div>
              {error && <div className="error-message">{error}</div>}
            </div>
            <div className="wealth-drawer-footer" style={{ flexDirection: "column", alignItems: "stretch", gap: 12 }}>
              <div style={{ display: "flex", gap: 20, flexWrap: "wrap" }}>
                <div className="goal-live-stat">
                  <span>{dict.wealthGoalsProgressLabel}</span>
                  <span className="mono" style={{ fontSize: 19 }}>{Math.min((savedN / targetN) * 100, 100).toFixed(1)}%</span>
                </div>
                <div className="goal-live-stat">
                  <span>{dict.wealthGoalsRemainLabel}</span>
                  <span className="mono" style={{ fontSize: 14, marginTop: 4 }}>{fmtMoney(remain, CURRENCY)}</span>
                </div>
                <div className="goal-live-stat">
                  <span>{dict.wealthGoalsPaceLabel}</span>
                  <span className="mono" style={{ fontSize: 13, color: late ? "var(--loss)" : "var(--ink-2)" }}>{pace}</span>
                </div>
              </div>
              <div style={{ display: "flex", gap: 10 }}>
                {goal && (
                  <button className="goal-delete-btn" disabled={busy} onClick={() => run(() => deleteWealthGoal(goal.id))}>
                    {dict.wealthGoalsDelete}
                  </button>
                )}
                <button className="wealth-drawer-cancel" onClick={onClose}>
                  {dict.cancel}
                </button>
                <button className="btn-primary" style={{ flex: 1 }} disabled={!kind || busy} onClick={save}>
                  {dict.wealthGoalsSave}
                </button>
              </div>
            </div>
          </Dialog.Content>
        </Dialog.Overlay>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

// GoalCard mirrors the template's goal row (Argus Trading WebUI.dc.html
// 1446-1510): the big (retirement) row is the enlarged glow card with the
// "driven by 退休規劃" hint and no edit button; the small grid cards carry
// name + status chip + 編輯 on row one, note + percent on row two. Monthly
// is "—" until a monthly figure exists (typed in the drawer, or the
// retirement setting). ETA is the target year.
function GoalCard({
  dict,
  goal,
  big,
  onEdit,
  onNavigate,
}: {
  dict: Dictionary;
  goal: Goal;
  big: boolean;
  onEdit?: () => void;
  onNavigate: (path: string) => void;
}) {
  const pct = goal.progressPct != null ? Math.max(0, Math.min(100, goal.progressPct)) : 0;
  const pctText = goal.progressPct != null ? `${goal.progressPct.toFixed(1)}%` : "—";
  const valueColor = big ? "var(--ink)" : "var(--ink-2)";
  return (
    <div className={`card${big ? " card--glow" : ""}`} style={{ display: "flex", flexDirection: "column", gap: 9 }}>
      <div style={{ display: "flex", alignItems: "baseline", gap: 10, flexWrap: "wrap" }}>
        <span style={{ fontFamily: "var(--sans)", fontSize: big ? 16 : 13.5 }}>{goal.name}</span>
        {big && goal.note && <span style={{ fontSize: 11.5, color: "var(--ink-3)" }}>{goal.note}</span>}
        <StatusChip dict={dict} status={goal.status} />
        {big ? (
          <span className="mono" style={{ marginLeft: "auto", fontSize: 24 }}>{pctText}</span>
        ) : (
          onEdit && (
            <button className="goal-edit-btn" style={{ marginLeft: "auto" }} onClick={onEdit}>
              {dict.wealthGoalsEditBtn}
            </button>
          )
        )}
      </div>
      {!big && (
        <div style={{ display: "flex", alignItems: "baseline", gap: 10 }}>
          <span style={{ fontSize: 11.5, color: "var(--ink-3)" }}>{goal.note}</span>
          <span className="mono" style={{ marginLeft: "auto", fontSize: 17 }}>{pctText}</span>
        </div>
      )}
      <GoalProgressBar dict={dict} pct={pct} markPct={goal.markPct} status={goal.status} />
      {big && (
        <div style={{ display: "flex", alignItems: "baseline", gap: 10, flexWrap: "wrap" }}>
          <span style={{ fontFamily: "var(--font-mono)", fontSize: 10.5, color: "var(--ink-3)" }}>{dict.wealthGoalsRetireLocked}</span>
          <a href="#" style={{ marginLeft: "auto", fontFamily: "var(--font-mono)", fontSize: 11 }} onClick={(e) => { e.preventDefault(); onNavigate("/w/retire"); }}>
            {dict.wealthRetireSettingsButton}
          </a>
        </div>
      )}
      <div style={{ display: "flex", gap: big ? 22 : 18, flexWrap: "wrap", fontFamily: "var(--font-mono)", fontSize: big ? 11 : 10.5, color: "var(--ink-3)" }}>
        <span>
          {dict.wealthGoalsSavedLabel} <span style={{ color: valueColor }}>{goal.saved != null ? fmtMoney(goal.saved, CURRENCY) : "—"}</span>
        </span>
        <span>
          {dict.wealthGoalsTargetLabel} <span style={{ color: valueColor }}>{fmtMoney(goal.targetAmount, CURRENCY)}</span>
        </span>
        <span>
          {dict.wealthGoalsMonthlyLabel}{" "}
          <span style={{ color: valueColor }}>{goal.monthlyContribution != null ? fmtMoney(goal.monthlyContribution, CURRENCY) : "—"}</span>
        </span>
        <span>
          {dict.wealthGoalsEtaLabel} <span style={{ color: valueColor }}>{yearOf(goal.targetDate) || "—"}</span>
        </span>
      </div>
    </div>
  );
}

export function WealthGoalsView({ dict, writable, onUnauthorized, onNavigate }: Props) {
  const [goals, setGoals] = useState<Goal[] | null>(null);
  const [error, setError] = useState(false);
  const [refreshSignal, setRefreshSignal] = useState(0);
  // drawer: undefined = closed, null = add, Goal = edit.
  const [drawer, setDrawer] = useState<Goal | null | undefined>(undefined);

  useEffect(() => {
    setError(false);
    fetchWealthGoals()
      .then((g) => setGoals(g.goals))
      .catch(() => setError(true));
  }, [refreshSignal]);

  if (error) {
    return <div className="error-message">{dict.error}</div>;
  }

  const bigGoal = goals?.find((g) => g.kind === "retirement");
  const otherGoals = goals?.filter((g) => g.id !== bigGoal?.id) ?? [];
  const totalSaved = goals?.reduce((sum, g) => (g.saved != null ? sum + g.saved : sum), 0) ?? 0;
  const totalTarget = goals?.reduce((sum, g) => sum + g.targetAmount, 0) ?? 0;
  const totalPct = totalTarget > 0 ? (totalSaved / totalTarget) * 100 : null;
  const behindCount = goals?.filter((g) => g.status === "behind").length ?? 0;
  const monthlyValues = goals?.flatMap((g) => (g.monthlyContribution != null ? [g.monthlyContribution] : [])) ?? [];
  const monthlyTotal = monthlyValues.length > 0 ? monthlyValues.reduce((a, b) => a + b, 0) : null;

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
        {writable && (
          <button className="btn-tint" onClick={() => setDrawer(null)}>
            + {dict.wealthGoalsAddBtn}
          </button>
        )}
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
            <div className="mono" style={{ fontSize: "clamp(17px,2.1vw,28px)", marginTop: 8 }}>
              {monthlyTotal != null ? fmtMoney(monthlyTotal, CURRENCY) : "—"}
            </div>
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
          <GoalCard dict={dict} goal={bigGoal} big onNavigate={onNavigate} />
        </div>
      )}

      {otherGoals.length > 0 && (
        <div className="goal-grid">
          {otherGoals.map((goal) => (
            <GoalCard key={goal.id} dict={dict} goal={goal} big={false} onNavigate={onNavigate} onEdit={writable ? () => setDrawer(goal) : undefined} />
          ))}
        </div>
      )}
      {drawer !== undefined && (
        <GoalDrawer
          dict={dict}
          goal={drawer}
          onClose={() => setDrawer(undefined)}
          onUnauthorized={onUnauthorized}
          onSaved={() => {
            setDrawer(undefined);
            setRefreshSignal((n) => n + 1);
          }}
        />
      )}
    </>
  );
}
