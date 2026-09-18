import { useEffect, useState } from "react";
import {
  ApiError,
  createInsurancePolicy,
  fetchWealthInsure,
  fetchWealthProfile,
  saveWealthProfile,
  type InsuranceCoverageRow,
  type InsuranceKind,
  type InsurancePolicyItem,
  type WealthInsure,
} from "../api";
import type { Dictionary } from "../i18n";
import { fmtMoney } from "./WealthHomeView";
import { srcLabel } from "./WealthBalanceView";

interface Props {
  dict: Dictionary;
  writable: boolean;
  onUnauthorized: (retry: () => void) => void;
}

const CURRENCY = "NT$"; // display currency fixed to TWD, same known gap as every other wealth page

// KIND_ORDER/KIND_PER_PERIOD mirror internal/web/wealth_insure.go's
// insuranceKindOrder/insuranceKindPerPeriod exactly — the server derives
// per_period from kind and never trusts a client-sent value, so this map is
// display-only (the add form's amount-field suffix preview), not sent on
// submit.
const KIND_ORDER: InsuranceKind[] = ["life", "accident", "ci", "cancer", "disability", "hospital"];
const KIND_PER_PERIOD: Record<InsuranceKind, "" | "month" | "day"> = {
  life: "", accident: "", ci: "", cancer: "", disability: "month", hospital: "day",
};

function kindLabel(dict: Dictionary, kind: InsuranceKind): string {
  switch (kind) {
    case "life":
      return dict.wealthInsureKindLife;
    case "accident":
      return dict.wealthInsureKindAccident;
    case "ci":
      return dict.wealthInsureKindCi;
    case "cancer":
      return dict.wealthInsureKindCancer;
    case "disability":
      return dict.wealthInsureKindDisability;
    case "hospital":
      return dict.wealthInsureKindHospital;
  }
}

function periodSuffix(dict: Dictionary, perPeriod?: string): string {
  if (perPeriod === "month") return dict.wealthInsurePerMonthSuffix;
  if (perPeriod === "day") return dict.wealthInsurePerDaySuffix;
  return "";
}

function fmtCoverage(dict: Dictionary, amount: number, perPeriod?: string): string {
  return `${fmtMoney(amount, CURRENCY)}${periodSuffix(dict, perPeriod)}`;
}

// gapColor mirrors insureModel()'s barHave color bands: covered (green),
// at least half covered (amber), otherwise a real shortfall (red).
function gapColor(pct: number): string {
  if (pct >= 100) return "var(--profit)";
  if (pct >= 50) return "#f59e0b";
  return "var(--loss)";
}

function AddPolicyForm({
  dict,
  onUnauthorized,
  onSaved,
}: {
  dict: Dictionary;
  onUnauthorized: (retry: () => void) => void;
  onSaved: () => void;
}) {
  const [insurer, setInsurer] = useState("");
  const [name, setName] = useState("");
  const [kind, setKind] = useState<InsuranceKind>("life");
  const [amount, setAmount] = useState("");
  const [insured, setInsured] = useState("");
  const [annualPremium, setAnnualPremium] = useState("");
  const [premiumYears, setPremiumYears] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit() {
    const amt = Number(amount);
    if (!insurer.trim() || !name.trim() || !(amt > 0)) {
      setError(dict.error);
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      await createInsurancePolicy({
        insurer: insurer.trim(),
        name: name.trim(),
        kind,
        amount: amt,
        insured: insured.trim() || undefined,
        annualPremium: annualPremium.trim() ? Number(annualPremium) : undefined,
        premiumYears: premiumYears.trim() ? Number(premiumYears) : undefined,
      });
      setInsurer("");
      setName("");
      setAmount("");
      setInsured("");
      setAnnualPremium("");
      setPremiumYears("");
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

  const amountLabel = `${dict.wealthInsureAmountLabel}${periodSuffix(dict, KIND_PER_PERIOD[kind])}`;

  return (
    <div className="card" style={{ marginBottom: 16 }}>
      <div className="eyebrow">{dict.wealthInsureAddTitle}</div>
      <div style={{ display: "flex", gap: 10, flexWrap: "wrap", marginTop: 10 }}>
        <label className="form-field" style={{ flex: "1 1 160px" }}>
          <span>{dict.wealthInsureInsurerLabel}</span>
          <input value={insurer} onChange={(e) => setInsurer(e.target.value)} />
        </label>
        <label className="form-field" style={{ flex: "1 1 160px" }}>
          <span>{dict.wealthInsurePolicyNameLabel}</span>
          <input value={name} onChange={(e) => setName(e.target.value)} />
        </label>
        <label className="form-field" style={{ width: 160 }}>
          <span>{dict.wealthInsureKindLabel}</span>
          <select value={kind} onChange={(e) => setKind(e.target.value as InsuranceKind)}>
            {KIND_ORDER.map((k) => (
              <option key={k} value={k}>
                {kindLabel(dict, k)}
              </option>
            ))}
          </select>
        </label>
        <label className="form-field" style={{ width: 150 }}>
          <span>{amountLabel}</span>
          <input className="mono" type="number" value={amount} onChange={(e) => setAmount(e.target.value)} />
        </label>
        <label className="form-field" style={{ width: 130 }}>
          <span>{dict.wealthInsuredLabel}</span>
          <input value={insured} onChange={(e) => setInsured(e.target.value)} />
        </label>
        <label className="form-field" style={{ width: 140 }}>
          <span>{dict.wealthInsureAnnualPremiumLabel}</span>
          <input className="mono" type="number" value={annualPremium} onChange={(e) => setAnnualPremium(e.target.value)} />
        </label>
        <label className="form-field" style={{ width: 120 }}>
          <span>{dict.wealthInsurePremiumYearsLabel}</span>
          <input className="mono" type="number" value={premiumYears} onChange={(e) => setPremiumYears(e.target.value)} />
        </label>
      </div>
      {error && <div className="error-message">{error}</div>}
      <div className="modal-actions">
        <button className="btn-primary" disabled={submitting} onClick={submit}>
          {dict.wealthInsureAdd}
        </button>
      </div>
    </div>
  );
}

function CoverageRow({ dict, row }: { dict: Dictionary; row: InsuranceCoverageRow }) {
  const pct = row.pctOfNeed != null ? Math.min(row.pctOfNeed, 100) : 0;
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
      <div style={{ display: "flex", alignItems: "baseline", gap: 10, flexWrap: "wrap" }}>
        <span style={{ fontSize: 13, flex: "1 1 120px" }}>{kindLabel(dict, row.kind)}</span>
        <span className="mono" style={{ fontSize: 11, color: "var(--ink-3)" }}>
          {dict.wealthInsureHaveLabel} {fmtCoverage(dict, row.have, row.perPeriod)} / {dict.wealthInsureNeedLabel}{" "}
          {row.need != null ? fmtCoverage(dict, row.need, row.perPeriod) : "—"}
        </span>
        <span className="mono" style={{ marginLeft: "auto", fontSize: 12, color: "var(--ink-2)" }}>
          {row.pctOfNeed != null ? `${row.pctOfNeed.toFixed(0)}%` : "—"}
        </span>
        <span className="mono" style={{ fontSize: 12, width: 130, textAlign: "right", color: row.gap != null ? gapColor(row.pctOfNeed ?? 0) : undefined }}>
          {row.gap == null ? "—" : row.gap <= 0 ? dict.wealthInsureCoveredLabel : fmtCoverage(dict, row.gap, row.perPeriod)}
        </span>
      </div>
      <div style={{ background: "var(--bg)", borderRadius: 4, height: 8 }}>
        <div style={{ height: 8, borderRadius: 4, width: `${pct}%`, background: row.need != null ? gapColor(row.pctOfNeed ?? 0) : "var(--ink-3)" }} />
      </div>
    </div>
  );
}

function PolicyRow({ dict, policy }: { dict: Dictionary; policy: InsurancePolicyItem }) {
  return (
    <tr>
      <td style={{ fontFamily: "var(--sans)" }}>
        <span style={{ display: "inline-flex", alignItems: "center", gap: 6 }}>
          {policy.name}
          <span className={`wealth-src-tag ${policy.source}`}>{srcLabel(dict, policy.source)}</span>
        </span>
      </td>
      <td style={{ fontFamily: "var(--sans)", fontSize: 12, color: "var(--ink-2)" }}>{policy.insurer}</td>
      <td style={{ fontFamily: "var(--sans)", fontSize: 12, color: "var(--ink-3)" }}>{kindLabel(dict, policy.kind)}</td>
      <td>{fmtCoverage(dict, policy.amount, policy.perPeriod)}</td>
      <td>{policy.annualPremium != null ? fmtMoney(policy.annualPremium, CURRENCY) : "—"}</td>
      <td style={{ fontFamily: "var(--sans)", fontSize: 12, color: "var(--ink-3)" }}>
        {policy.premiumYears != null ? `${policy.premiumYears} 年期` : "—"}
      </td>
    </tr>
  );
}

// InsureSetupCard is the "個人參數" mini setup card (§8.16.1's two
// asked-once inputs), same pattern as WealthRetireView's birth-year card —
// each wealth page that needs a profile field owns its own small form
// rather than a shared settings page (see PLAN.md's 2026-09-18 note).
function InsureSetupCard({
  dict,
  onUnauthorized,
  onSaved,
}: {
  dict: Dictionary;
  onUnauthorized: (retry: () => void) => void;
  onSaved: () => void;
}) {
  const [dependents, setDependents] = useState("");
  const [age, setAge] = useState("");
  const [spouseIncome, setSpouseIncome] = useState<"yes" | "no" | "">("");
  const [saving, setSaving] = useState(false);

  const dependentsNum = Number(dependents);
  const hasDependents = Number.isFinite(dependentsNum) && dependentsNum > 0;
  const canSave =
    dependents.trim() !== "" &&
    Number.isFinite(dependentsNum) &&
    dependentsNum >= 0 &&
    spouseIncome !== "" &&
    (!hasDependents || age.trim() !== "");

  async function save() {
    if (!canSave) return;
    setSaving(true);
    try {
      await fetchWealthProfile(); // warms auth state, same as WealthRetireView's setup save
      await saveWealthProfile({
        dependents: dependentsNum,
        youngestChildAge: hasDependents ? Number(age) : 0,
        spouseHasIncome: spouseIncome === "yes",
      });
      onSaved();
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        onUnauthorized(save);
      } else {
        window.alert(e instanceof ApiError ? e.message : dict.error);
      }
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="card" style={{ marginBottom: 16 }}>
      <div className="eyebrow">{dict.wealthInsureSetupTitle}</div>
      <div style={{ fontSize: 12, color: "var(--ink-3)", marginTop: 6 }}>{dict.wealthInsureNeedPendingNote}</div>
      <div style={{ display: "flex", gap: 10, flexWrap: "wrap", marginTop: 10 }}>
        <label className="form-field" style={{ width: 130 }}>
          <span>{dict.wealthInsureDependentsLabel}</span>
          <input className="mono" type="number" min={0} value={dependents} onChange={(e) => setDependents(e.target.value)} />
        </label>
        {hasDependents && (
          <label className="form-field" style={{ width: 160 }}>
            <span>{dict.wealthInsureYoungestChildAgeLabel}</span>
            <input className="mono" type="number" min={0} value={age} onChange={(e) => setAge(e.target.value)} />
          </label>
        )}
        <label className="form-field" style={{ width: 180 }}>
          <span>{dict.wealthInsureSpouseIncomeLabel}</span>
          <select value={spouseIncome} onChange={(e) => setSpouseIncome(e.target.value as "yes" | "no")}>
            <option value="" disabled>
              —
            </option>
            <option value="yes">{dict.wealthInsureSpouseIncomeYes}</option>
            <option value="no">{dict.wealthInsureSpouseIncomeNo}</option>
          </select>
        </label>
      </div>
      <div className="modal-actions">
        <button className="btn-primary" disabled={saving || !canSave} onClick={save}>
          {dict.wealthInsureSetupSave}
        </button>
      </div>
    </div>
  );
}

export function WealthInsureView({ dict, writable, onUnauthorized }: Props) {
  const [data, setData] = useState<WealthInsure | null>(null);
  const [error, setError] = useState(false);
  const [refreshSignal, setRefreshSignal] = useState(0);
  const [showAddForm, setShowAddForm] = useState(false);

  useEffect(() => {
    setError(false);
    fetchWealthInsure()
      .then(setData)
      .catch(() => setError(true));
  }, [refreshSignal]);

  function refresh() {
    setRefreshSignal((n) => n + 1);
  }

  if (error) {
    return <div className="error-message">{dict.error}</div>;
  }

  const worst = data?.rows.find((r) => r.kind === data.worstKind);

  return (
    <>
      {/* Header + "+" trigger — the design template's isWInsure row (line
          998) is the one wealth page besides /w/cash whose template
          actually draws a write affordance here (goals' isWGoals has none,
          see WealthGoalsView.tsx's comment on the opposite case). */}
      <div style={{ display: "flex", alignItems: "center", gap: 12, margin: "0 0 16px", flexWrap: "wrap" }}>
        <span style={{ fontFamily: "var(--font-mono)", textTransform: "uppercase", letterSpacing: ".08em", fontSize: 11, color: "var(--ink)" }}>
          {dict.navWealthInsure}
        </span>
        {writable && (
          <button className="btn-tint" style={{ marginLeft: "auto" }} onClick={() => setShowAddForm((v) => !v)}>
            + {dict.wealthInsureAdd}
          </button>
        )}
      </div>

      {writable && showAddForm && (
        <AddPolicyForm
          dict={dict}
          onUnauthorized={onUnauthorized}
          onSaved={() => {
            setShowAddForm(false);
            refresh();
          }}
        />
      )}

      {writable && data && !data.hasProfile && (
        <InsureSetupCard dict={dict} onUnauthorized={onUnauthorized} onSaved={refresh} />
      )}

      <div style={{ display: "flex", flexWrap: "wrap", gap: 16, marginBottom: 16 }}>
        <div className="card card--glow" style={{ flex: "1.5 1 250px" }}>
          <div className="eyebrow">{dict.wealthInsureBiggestGapLabel}</div>
          <div className="mono loss" style={{ fontSize: 30, lineHeight: 1.15, marginTop: 8, wordBreak: "keep-all" }}>
            {data && data.worstKind ? fmtCoverage(dict, data.worstGap, worst?.perPeriod) : "—"}
          </div>
          <div style={{ fontSize: 13, color: "var(--ink-2)", marginTop: 4 }}>
            {data?.worstKind ? kindLabel(dict, data.worstKind) : "—"} · {dict.wealthInsureCoverageLabel}{" "}
            {data?.worstKind ? `${data.worstPct.toFixed(0)}%` : "—"}
          </div>
        </div>
        <div className="card" style={{ flex: "1 1 200px" }}>
          <div className="eyebrow">{dict.wealthInsureTotalGapLabel}</div>
          <div className="mono loss" style={{ fontSize: "clamp(17px,2.1vw,28px)", marginTop: 8 }}>
            {data ? fmtMoney(data.totalGapLumpSum, CURRENCY) : "—"}
          </div>
        </div>
        <div className="card" style={{ flex: "1 1 200px" }}>
          <div className="eyebrow">{dict.wealthInsurePremiumLabel}</div>
          <div className="mono" style={{ fontSize: "clamp(17px,2.1vw,28px)", marginTop: 8 }}>
            {data ? fmtMoney(data.premium, CURRENCY) : "—"}
          </div>
          <div style={{ fontSize: 11, color: "var(--ink-3)", marginTop: 6 }}>
            {dict.wealthInsurePremShareLabel} {data?.premSharePct != null ? `${data.premSharePct.toFixed(1)}%` : "—"}
          </div>
        </div>
        <div className="card" style={{ flex: "1 1 200px" }}>
          <div className="eyebrow">{dict.wealthInsureCountLabel}</div>
          <div className="mono" style={{ fontSize: "clamp(17px,2.1vw,28px)", marginTop: 8 }}>
            {data?.count ?? "—"}
          </div>
        </div>
      </div>

      <div className="card" style={{ marginBottom: 16 }}>
        <div className="eyebrow" style={{ marginBottom: 18 }}>
          {dict.wealthInsureGapTitle}
        </div>
        <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
          {data?.rows.map((row) => (
            <CoverageRow key={row.kind} dict={dict} row={row} />
          ))}
        </div>
      </div>

      <div className="card" style={{ overflowX: "auto" }}>
        <div className="eyebrow" style={{ marginBottom: 4 }}>
          {dict.wealthInsurePoliciesTitle}
        </div>
        {data && data.policies.length > 0 ? (
          <table className="mono">
            <thead>
              <tr>
                <th style={{ textAlign: "left" }}>{dict.wealthInsurePolicyNameLabel}</th>
                <th style={{ textAlign: "left" }}>{dict.wealthInsureInsurerLabel}</th>
                <th style={{ textAlign: "left" }}>{dict.wealthInsureKindLabel}</th>
                <th>{dict.wealthInsureAmountLabel}</th>
                <th>{dict.wealthInsureAnnualPremiumLabel}</th>
                <th style={{ textAlign: "left" }}>{dict.wealthInsurePremiumYearsLabel}</th>
              </tr>
            </thead>
            <tbody>
              {data.policies.map((p, i) => (
                <PolicyRow key={`${p.assetId}-${i}`} dict={dict} policy={p} />
              ))}
            </tbody>
          </table>
        ) : (
          <div className="empty-message">{dict.wealthInsureNoPolicies}</div>
        )}
      </div>
    </>
  );
}
