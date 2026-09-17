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
    <div className="card">
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
    <div style={{ marginTop: 10, paddingTop: 10, borderTop: "1px solid var(--border)" }}>
      <div className="eyebrow">{dict.wealthGoalsEarmarkTitle}</div>
      {goal.assets.length > 0 ? (
        <ul style={{ margin: "6px 0", paddingLeft: 0, listStyle: "none" }}>
          {goal.assets.map((a) => (
            <li key={a.assetId} style={{ display: "flex", justifyContent: "space-between", alignItems: "center", padding: "4px 0", fontSize: 13 }}>
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

  return (
    <>
      {writable && <div style={{ marginBottom: 16 }}><AddGoalForm dict={dict} onUnauthorized={onUnauthorized} onSaved={() => setRefreshSignal((n) => n + 1)} /></div>}

      {goals && goals.length === 0 && <div className="card empty-message">{dict.wealthGoalsNoGoals}</div>}

      {goals?.map((goal) => {
        const pct = goal.progressPct != null ? Math.max(0, Math.min(100, goal.progressPct)) : 0;
        const status = statusLabel(dict, goal.status);
        return (
          <div className="card" key={goal.id} style={{ marginBottom: 16 }}>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "baseline" }}>
              <div className="eyebrow">
                {goal.name}
                {goal.kind === "retirement" && ` · ${dict.wealthGoalsKindRetirement}`}
              </div>
              {writable && <button onClick={() => remove(goal)}>{dict.wealthGoalsDelete}</button>}
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 10, marginTop: 8 }}>
              <span className="edge-bar-track">
                <span className={`edge-bar-fill ${goal.status === "behind" ? "loss" : "profit"}`} style={{ width: `${pct}%` }} />
              </span>
              <span className="edge-bar-value mono">{goal.progressPct != null ? `${goal.progressPct.toFixed(0)}%` : "—"}</span>
            </div>
            <div style={{ display: "flex", gap: 24, marginTop: 6, fontSize: 13 }}>
              <span>
                {dict.wealthGoalsSavedLabel}:{" "}
                <span className="mono">{goal.saved != null ? fmtMoney(goal.saved, CURRENCY) : "—"}</span>
              </span>
              <span>
                {dict.wealthGoalsTargetLabel}: <span className="mono">{fmtMoney(goal.targetAmount, CURRENCY)}</span>
              </span>
              {goal.targetDate && <span className="mono">{goal.targetDate}</span>}
              {status && <span>{status}</span>}
            </div>
            <EarmarkEditor
              dict={dict}
              goal={goal}
              assets={assets}
              onUnauthorized={onUnauthorized}
              onSaved={() => setRefreshSignal((n) => n + 1)}
            />
          </div>
        );
      })}
    </>
  );
}
