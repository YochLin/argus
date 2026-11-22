import { useEffect, useState } from "react";
import { ApiError, fetchWealthChecks, saveWealthProfile, type CheckItem, type InsuranceKind, type WealthChecks } from "../api";
import { useAssistant } from "../assistant";
import type { Dictionary } from "../i18n";
import { fmtMoney } from "./WealthHomeView";
import { kindLabel } from "./WealthInsureView";
import { useFlash } from "../flash";

// fill swaps {name} placeholders; the check texts carry several each.
function fill(s: string, vars: Record<string, string | number | undefined>): string {
  return s.replace(/\{(\w+)\}/g, (m, k: string) => (vars[k] === undefined ? m : String(vars[k])));
}

const num = (v?: number) => (v == null ? "—" : String(Math.round(v * 10) / 10));

interface Text {
  title: string;
  reason: string;
  fix: string;
  gapLabel: string;
}

// checkText words one item. The rule and its figures come from the server
// (service.EvaluateWealthChecks); only the wording lives here.
export function checkText(dict: Dictionary, it: CheckItem): Text {
  const vars = {
    v: num(it.value),
    t: num(it.target),
    n: it.count,
    name: (it.names ?? []).join(dict.ckListSep),
    kind: it.kind ? kindLabel(dict, it.kind as InsuranceKind) : "",
  };
  const t = (title: string, reason: string, fix = "", gapLabel = ""): Text => ({
    title: fill(title, vars),
    reason: fill(reason, vars),
    fix: fill(fix, vars),
    gapLabel,
  });
  switch (it.code) {
    case "emergency":
      // The server raises the target above 6 months when there are dependents.
      return t(dict.ckEmergencyTitle, (it.target ?? 0) > 6 ? dict.ckEmergencyReasonDeps : dict.ckEmergencyReason, dict.ckEmergencyFix, dict.ckEmergencyGap);
    case "debtRatio":
      return t(dict.ckDebtRatioTitle, dict.ckDebtRatioReason, dict.ckDebtRatioFix);
    case "card":
      return t(dict.ckCardTitle, it.value != null ? dict.ckCardReasonRate : dict.ckCardReason, dict.ckCardFix, dict.ckCardGap);
    case "coverage":
      return t(dict.ckCoverageTitle, dict.ckCoverageReason, dict.ckCoverageFix, dict.ckCoverageGap);
    case "retirement":
      return t(dict.ckRetirementTitle, dict.ckRetirementReason, dict.ckRetirementFix, dict.ckRetirementGap);
    case "goals":
      return t(dict.ckGoalsTitle, dict.ckGoalsReason, dict.ckGoalsFix);
    case "alloc":
      return t(dict.ckAllocTitle, dict.ckAllocReason, dict.ckAllocFix, dict.ckAllocGap);
    case "savings":
      return t(dict.ckSavingsTitle, dict.ckSavingsReason, dict.ckSavingsFix);
    case "dependents":
      return t(dict.ckDependentsTitle, dict.ckDependentsReason, dict.ckProfileFix);
    case "age":
      return t(dict.ckAgeTitle, dict.ckAgeReason, dict.ckProfileFix);
    case "salary":
      return t(dict.ckSalaryTitle, dict.ckSalaryReason, dict.ckProfileFix);
    case "stale":
      return t(dict.ckStaleTitle, dict.ckStaleReason, dict.ckStaleFix);
  }
}

interface Props {
  dict: Dictionary;
  writable: boolean;
  refresh: number;
  onUnauthorized: (retry: () => void) => void;
  onNavigate: (path: string) => void;
  // guide is shown in the card's place while nothing has been recorded.
  guide: React.ReactNode;
}

const DEPS = [0, 1, 2, 3];

// WealthChecks is /w's "本月該做的事" card: up to three actions (all on
// request), a passed state, or — on an empty account — the setup guide.
// Nothing here is stored: an item goes away when its data changes.
export function WealthChecksCard({ dict, writable, refresh, onUnauthorized, onNavigate, guide }: Props) {
  const [checks, setChecks] = useState<WealthChecks | null>(null);
  const [all, setAll] = useState(false);
  const [depsOpen, setDepsOpen] = useState(false);
  const [reload, setReload] = useState(0);
  const assistant = useAssistant();
  const flash = useFlash();

  useEffect(() => {
    fetchWealthChecks()
      .then(setChecks)
      .catch(() => setChecks(null));
  }, [refresh, reload]);

  if (!checks) return null;
  if (checks.new) return <>{guide}</>;

  async function pickDeps(n: number) {
    if (!writable) {
      flash(dict.wealthRoNoAdd, "error");
      return;
    }
    try {
      await saveWealthProfile({ dependents: n });
      setDepsOpen(false);
      setReload((r) => r + 1);
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) onUnauthorized(() => pickDeps(n));
      else flash(e instanceof ApiError ? e.message : dict.error, "error");
    }
  }

  const items = checks.items;
  const sevLabel = { do: dict.ckSevDo, tip: dict.ckSevTip, data: dict.ckSevData };

  if (items.length === 0) {
    return (
      <>
        <section className="ck-card" aria-labelledby="ck-title">
          <div className="ck-clear">
            <span className="ck-clear-mark" aria-hidden="true">
              ✓
            </span>
            <h2 id="ck-title" className="ck-title">
              {dict.ckClearTitle}
            </h2>
            <span className="ck-meta">{fill(dict.ckClearSub, { n: checks.rules })}</span>
          </div>
        </section>
        <p className="ck-disclaimer">{dict.ckDisclaimer}</p>
      </>
    );
  }

  const visible = all ? items : items.slice(0, 3);

  return (
    <>
      <section className="ck-card" aria-labelledby="ck-title">
        <div className="ck-head">
          <h2 id="ck-title" className="ck-title">
            {dict.ckTitle}
          </h2>
          <span className="ck-meta">{fill(dict.ckMeta, { n: checks.rules })}</span>
        </div>
        <ul className="ck-list">
          {visible.map((it) => {
            const tx = checkText(dict, it);
            const isDeps = it.code === "dependents";
            const gap = it.gap != null ? fmtMoney(it.gap, "NT$") : "";
            const ask = fill(dict.ckAiAsk, { title: tx.title, reason: tx.reason }) + (gap ? fill(dict.ckAiAskGap, { label: tx.gapLabel, gap }) : "");
            return (
              <li key={it.code} className="ck-item">
                <div className="ck-row">
                  <button
                    className="ck-main"
                    aria-expanded={isDeps ? depsOpen : undefined}
                    onClick={() => (isDeps ? setDepsOpen(!depsOpen) : onNavigate(it.link))}
                  >
                    <span className={`ck-pill ck-${it.severity}`}>
                      <span className="ck-dot" aria-hidden="true" />
                      {sevLabel[it.severity]}
                    </span>
                    <span className="ck-text">
                      <span className="ck-item-title">{tx.title}</span>
                      <span className="ck-reason">
                        {tx.reason}
                        {tx.fix && <span className="ck-fix">　·　{tx.fix}</span>}
                      </span>
                    </span>
                    {gap && (
                      <span className="ck-gap">
                        <span className="ck-gap-label">{tx.gapLabel}</span>
                        <span className={`ck-gap-value${it.severity === "do" ? " ck-do-fg" : ""}`}>{gap}</span>
                      </span>
                    )}
                    <svg className="ck-chev" width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                      <path d="m6 3.5 4.5 4.5L6 12.5" />
                    </svg>
                  </button>
                  {assistant && (
                    <button className="ck-ai" aria-label={dict.ckAiPlan} onClick={() => assistant.open("home", ask)}>
                      <svg width="13" height="13" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
                        <path d="M8 1.5 9.4 6.6 14.5 8 9.4 9.4 8 14.5 6.6 9.4 1.5 8 6.6 6.6Z" />
                      </svg>
                      <span className="ck-ai-label">{dict.ckAiPlan}</span>
                    </button>
                  )}
                </div>
                {isDeps && depsOpen && (
                  <div className="ck-deps" role="group" aria-label={dict.ckDepsQ}>
                    <span className="ck-deps-q">{dict.ckDepsQ}</span>
                    {DEPS.map((n) => (
                      <button key={n} className="ck-deps-opt" onClick={() => pickDeps(n)}>
                        {n === 3 ? "3+" : n}
                      </button>
                    ))}
                    <a
                      href="/w/balance"
                      onClick={(e) => {
                        e.preventDefault();
                        onNavigate("/w/balance");
                      }}
                    >
                      {dict.ckDepsMore} ›
                    </a>
                  </div>
                )}
              </li>
            );
          })}
        </ul>
        {items.length > 3 && (
          <button className="ck-more" aria-expanded={all} onClick={() => setAll(!all)}>
            {all ? dict.ckLess : fill(dict.ckMore, { n: items.length })}
          </button>
        )}
      </section>
      <p className="ck-disclaimer">{dict.ckDisclaimer}</p>
    </>
  );
}
