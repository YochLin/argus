import { useEffect, useState } from "react";
import {
  ApiError,
  createWealthGoal,
  deleteWealthGoal,
  fetchWealthAssets,
  fetchWealthGoals,
  setWealthGoalEarmark,
  type Goal,
  type WealthAsset,
} from "../api";
import type { Dictionary } from "../i18n";
import { fmtMoney } from "./WealthHomeView";

interface Props {
  dict: Dictionary;
  writable: boolean;
  onUnauthorized: (retry: () => void) => void;
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
// 1193) — a track, a fill, and (when the goal has a targetDate) a tick at
// markPct showing where progress "should" be today, not just a color.
function GoalProgressBar({ pct, markPct, status }: { pct: number; markPct?: number; status: Goal["status"] }) {
  return (
    <div className="goal-progress-track">
      <span className={`goal-progress-fill ${status === "behind" ? "loss" : "profit"}`} style={{ width: `${pct}%` }} />
      {markPct != null && <span className="goal-progress-mark" style={{ left: `${Math.max(0, Math.min(100, markPct))}%` }} />}
    </div>
  );
}

function AddGoalForm({
  dict,
  onUnauthorized,
  onSaved,
}: {
  dict: Dictionary;
  onUnauthorized: (retry: () => void) => void;
  onSaved: () => void;
}) {
  const [name, setName] = useState("");
  const [targetAmount, setTargetAmount] = useState("");
  const [targetDate, setTargetDate] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit() {
    const amt = Number(targetAmount);
    if (!name.trim() || !(amt > 0)) {
      setError(dict.error);
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      await createWealthGoal({ name: name.trim(), targetAmount: amt, targetDate: targetDate.trim() || undefined });
      setName("");
      setTargetAmount("");
      setTargetDate("");
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
    <div className="card" style={{ marginBottom: 16 }}>
      <div className="eyebrow">{dict.wealthGoalsAddTitle}</div>
      <div style={{ display: "flex", gap: 10, flexWrap: "wrap", marginTop: 8 }}>
        <label className="form-field" style={{ flex: "1 1 200px" }}>
          <span>{dict.wealthGoalsNameLabel}</span>
          <input value={name} onChange={(e) => setName(e.target.value)} />
        </label>
        <label className="form-field" style={{ width: 160 }}>
          <span>{dict.wealthGoalsTargetAmountLabel}</span>
          <input className="mono" type="number" value={targetAmount} onChange={(e) => setTargetAmount(e.target.value)} />
        </label>
        <label className="form-field" style={{ width: 160 }}>
          <span>{dict.wealthGoalsTargetDateLabel}</span>
          <input className="mono" type="date" value={targetDate} onChange={(e) => setTargetDate(e.target.value)} />
        </label>
      </div>
      {error && <div className="error-message">{error}</div>}
      <div className="modal-actions">
        <button className="btn-primary" disabled={submitting} onClick={submit}>
          {dict.wealthGoalsAdd}
        </button>
      </div>
    </div>
  );
}

function EarmarkEditor({
  dict,
  goal,
  assets,
  onUnauthorized,
  onSaved,
}: {
  dict: Dictionary;
  goal: Goal;
  assets: WealthAsset[];
  onUnauthorized: (retry: () => void) => void;
  onSaved: () => void;
}) {
  const linkedIDs = new Set(goal.assets.map((a) => a.assetId));
  const linkable = assets.filter((a) => a.side === "asset" && !a.archivedAt && !linkedIDs.has(a.id));
  const [assetId, setAssetId] = useState<string>("");
  const [ratio, setRatio] = useState("1");
  const [error, setError] = useState<string | null>(null);

  async function link() {
    const id = Number(assetId);
    const r = Number(ratio);
    if (!(id > 0) || !(r > 0) || r > 1) {
      setError(dict.error);
      return;
    }
    try {
      await setWealthGoalEarmark(goal.id, id, r);
      setAssetId("");
      setRatio("1");
      setError(null);
      onSaved();
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        onUnauthorized(link);
      } else {
        setError(e instanceof ApiError ? e.message : dict.error);
      }
    }
  }

  async function unlink(targetAssetId: number) {
    try {
      await setWealthGoalEarmark(goal.id, targetAssetId, 0);
      onSaved();
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        onUnauthorized(() => unlink(targetAssetId));
      } else {
        window.alert(e instanceof ApiError ? e.message : dict.error);
      }
    }
  }

  return (
    <div style={{ marginTop: 4, paddingTop: 9, borderTop: "1px solid var(--border)" }}>
      <div style={{ fontFamily: "var(--font-mono)", textTransform: "uppercase", letterSpacing: ".08em", fontSize: 10.5, color: "var(--ink-3)" }}>
        {dict.wealthGoalsEarmarkTitle}
      </div>
      {goal.assets.length > 0 ? (
        <ul style={{ margin: "6px 0", paddingLeft: 0, listStyle: "none" }}>
          {goal.assets.map((a) => (
            <li key={a.assetId} style={{ display: "flex", justifyContent: "space-between", alignItems: "center", padding: "4px 0", fontSize: 12.5 }}>
              <span style={{ fontFamily: "var(--sans)" }}>
                {a.name} {a.venue ? `· ${a.venue}` : ""} ({Math.round(a.ratio * 100)}%)
              </span>
              <span style={{ display: "flex", alignItems: "center", gap: 8 }}>
                <span className="mono">{a.value != null ? fmtMoney(a.value, CURRENCY) : "—"}</span>
                <button onClick={() => unlink(a.assetId)}>{dict.wealthGoalsEarmarkRemove}</button>
              </span>
            </li>
          ))}
        </ul>
      ) : (
        <div className="empty-message">{dict.wealthGoalsNoEarmarks}</div>
      )}
      {linkable.length > 0 && (
        <div style={{ display: "flex", gap: 8, flexWrap: "wrap", marginTop: 8 }}>
          <select value={assetId} onChange={(e) => setAssetId(e.target.value)} style={{ flex: "1 1 160px" }}>
            <option value="">{dict.wealthGoalsEarmarkAssetLabel}</option>
            {linkable.map((a) => (
              <option key={a.id} value={a.id}>
                {a.name}
              </option>
            ))}
          </select>
          <input
            className="mono"
            type="number"
            min={0.01}
            max={1}
            step={0.01}
            value={ratio}
            onChange={(e) => setRatio(e.target.value)}
            style={{ width: 80 }}
            aria-label={dict.wealthGoalsEarmarkRatioLabel}
          />
          <button onClick={link}>{dict.wealthGoalsEarmarkAdd}</button>
        </div>
      )}
      {error && <div className="error-message">{error}</div>}
    </div>
  );
}

// GoalCard mirrors the template's goal row (lines 1166-1201): name + state
// chip + pct on top, note below, progress bar with the expected-progress
// mark, then a Saved/Target/Monthly/ETA stat line. `big` renders it as the
// enlarged "glow" card the template reserves for the retirement row (lines
// 1166-1182) instead of the plain grid card (lines 1186-1200) — Monthly is
// always "—": the goals schema (§8.8/§9.2) has no monthly-contribution
// column, so there's nothing to compute it from yet; ETA falls back to the
// user's own targetDate rather than a projected completion date, since that
// projection also needs the monthly figure we don't have.
function GoalCard({
  dict,
  goal,
  big,
  writable,
  assets,
  onUnauthorized,
  onSaved,
  onDelete,
}: {
  dict: Dictionary;
  goal: Goal;
  big: boolean;
  writable: boolean;
  assets: WealthAsset[];
  onUnauthorized: (retry: () => void) => void;
  onSaved: () => void;
  onDelete: (goal: Goal) => void;
}) {
  const pct = goal.progressPct != null ? Math.max(0, Math.min(100, goal.progressPct)) : 0;
  return (
    <div className={`card${big ? " card--glow" : ""}`} style={{ display: "flex", flexDirection: "column", gap: 9 }}>
      <div style={{ display: "flex", alignItems: "baseline", gap: 10, flexWrap: "wrap" }}>
        <span style={{ fontFamily: "var(--sans)", fontSize: big ? 16 : 13.5 }}>{goal.name}</span>
        {goal.kind === "retirement" && <span style={{ fontSize: 11.5, color: "var(--ink-3)" }}>{dict.wealthGoalsKindRetirement}</span>}
        <StatusChip dict={dict} status={goal.status} />
        <span className="mono" style={{ marginLeft: "auto", fontSize: big ? 24 : 17 }}>
          {goal.progressPct != null ? `${goal.progressPct.toFixed(0)}%` : "—"}
        </span>
      </div>
      {goal.note && <div style={{ fontSize: 11.5, color: "var(--ink-3)" }}>{goal.note}</div>}
      <GoalProgressBar pct={pct} markPct={goal.markPct} status={goal.status} />
      <div style={{ display: "flex", gap: big ? 22 : 18, flexWrap: "wrap", fontFamily: "var(--font-mono)", fontSize: big ? 11 : 10.5, color: "var(--ink-3)" }}>
        <span>
          {dict.wealthGoalsSavedLabel} <span style={{ color: "var(--ink)" }}>{goal.saved != null ? fmtMoney(goal.saved, CURRENCY) : "—"}</span>
        </span>
        <span>
          {dict.wealthGoalsTargetLabel} <span style={{ color: "var(--ink)" }}>{fmtMoney(goal.targetAmount, CURRENCY)}</span>
        </span>
        <span>
          {dict.wealthGoalsMonthlyLabel} <span style={{ color: "var(--ink)" }}>—</span>
        </span>
        <span>
          {dict.wealthGoalsEtaLabel} <span style={{ color: "var(--ink)" }}>{goal.targetDate || "—"}</span>
        </span>
      </div>
      {writable && (
        <div style={{ display: "flex", justifyContent: "flex-end" }}>
          <button onClick={() => onDelete(goal)}>{dict.wealthGoalsDelete}</button>
        </div>
      )}
      <EarmarkEditor dict={dict} goal={goal} assets={assets} onUnauthorized={onUnauthorized} onSaved={onSaved} />
    </div>
  );
}

export function WealthGoalsView({ dict, writable, onUnauthorized }: Props) {
  const [goals, setGoals] = useState<Goal[] | null>(null);
  const [assets, setAssets] = useState<WealthAsset[]>([]);
  const [error, setError] = useState(false);
  const [refreshSignal, setRefreshSignal] = useState(0);

  useEffect(() => {
    setError(false);
    Promise.all([fetchWealthGoals(), fetchWealthAssets()])
      .then(([g, a]) => {
        setGoals(g.goals);
        setAssets(a.assets);
      })
      .catch(() => setError(true));
  }, [refreshSignal]);

  async function remove(goal: Goal) {
    if (!window.confirm(goal.name)) return;
    try {
      await deleteWealthGoal(goal.id);
      setRefreshSignal((n) => n + 1);
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        onUnauthorized(() => remove(goal));
      } else {
        window.alert(e instanceof ApiError ? e.message : dict.error);
      }
    }
  }

  if (error) {
    return <div className="error-message">{dict.error}</div>;
  }

  const onSaved = () => setRefreshSignal((n) => n + 1);
  const bigGoal = goals?.find((g) => g.kind === "retirement");
  const otherGoals = goals?.filter((g) => g.id !== bigGoal?.id) ?? [];
  const totalSaved = goals?.reduce((sum, g) => (g.saved != null ? sum + g.saved : sum), 0) ?? 0;
  const totalTarget = goals?.reduce((sum, g) => sum + g.targetAmount, 0) ?? 0;
  const totalPct = totalTarget > 0 ? (totalSaved / totalTarget) * 100 : null;
  const behindCount = goals?.filter((g) => g.status === "behind").length ?? 0;

  return (
    <>
      {writable && <AddGoalForm dict={dict} onUnauthorized={onUnauthorized} onSaved={onSaved} />}

      {goals && goals.length === 0 && <div className="card empty-message">{dict.wealthGoalsNoGoals}</div>}

      {goals && goals.length > 0 && (
        <div style={{ display: "flex", flexWrap: "wrap", gap: 16, marginBottom: 16 }}>
          <div className="card" style={{ flex: "1 1 200px" }}>
            <div className="eyebrow">{dict.wealthGoalsTotalProgress}</div>
            <div className="mono" style={{ fontSize: "clamp(17px,2.1vw,28px)", marginTop: 8 }}>
              {totalPct != null ? `${totalPct.toFixed(0)}%` : "—"}
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
            <div className={`mono ${behindCount > 0 ? "loss" : ""}`} style={{ fontSize: "clamp(17px,2.1vw,28px)", marginTop: 8 }}>
              {behindCount}
            </div>
          </div>
        </div>
      )}

      {bigGoal && (
        <div style={{ marginBottom: 16 }}>
          <GoalCard dict={dict} goal={bigGoal} big writable={writable} assets={assets} onUnauthorized={onUnauthorized} onSaved={onSaved} onDelete={remove} />
        </div>
      )}

      {otherGoals.length > 0 && (
        <div className="goal-grid">
          {otherGoals.map((goal) => (
            <GoalCard
              key={goal.id}
              dict={dict}
              goal={goal}
              big={false}
              writable={writable}
              assets={assets}
              onUnauthorized={onUnauthorized}
              onSaved={onSaved}
              onDelete={remove}
            />
          ))}
        </div>
      )}
    </>
  );
}
