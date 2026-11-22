import { useState } from "react";
import { ApiError, saveWealthProfile, type WealthProfile } from "../api";
import type { Dictionary } from "../i18n";
import { useFlash } from "../flash";

interface Props {
  dict: Dictionary;
  writable: boolean;
  profile: WealthProfile;
  onSaved: () => void;
  onUnauthorized: (retry: () => void) => void;
}

const DEPS = [0, 1, 2, 3];

// The profile stores a birth year (the retirement projection's input); the
// card speaks in age, like the design.
const thisYear = () => new Date().getFullYear();

// WealthHousehold is /w/balance's 家庭狀況: the profile facts the emergency
// fund, coverage and retirement checks read. It opens in edit mode while
// something is missing, which is how the home page's guide step and its
// "其他家庭資料" link land on the form.
export function WealthHousehold({ dict, writable, profile, onSaved, onUnauthorized }: Props) {
  const missing = profile.birthYear == null || profile.dependents == null || profile.spouseHasIncome == null;
  const [editing, setEditing] = useState(missing && writable);
  const [age, setAge] = useState(profile.birthYear != null ? String(thisYear() - profile.birthYear) : "");
  const [deps, setDeps] = useState(profile.dependents);
  const [child, setChild] = useState(profile.youngestChildAge != null ? String(profile.youngestChildAge) : "");
  const [spouse, setSpouse] = useState(profile.spouseHasIncome);
  const [saving, setSaving] = useState(false);
  const flash = useFlash();

  async function save() {
    const patch: Parameters<typeof saveWealthProfile>[0] = {};
    const a = Number(age);
    if (age.trim() !== "" && a > 0 && a < 120) patch.birthYear = thisYear() - a;
    if (deps != null) patch.dependents = deps;
    if (deps != null && deps > 0 && child.trim() !== "") patch.youngestChildAge = Number(child);
    if (spouse != null) patch.spouseHasIncome = spouse;
    if (Object.keys(patch).length === 0) {
      setEditing(false);
      return;
    }
    setSaving(true);
    try {
      await saveWealthProfile(patch);
      setEditing(false);
      onSaved();
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) onUnauthorized(save);
      else flash(e instanceof ApiError ? e.message : dict.error, "error");
    } finally {
      setSaving(false);
    }
  }

  function toggle() {
    if (!editing) {
      if (!writable) {
        flash(dict.wealthRoNoChange);
        return;
      }
      setEditing(true);
    } else void save();
  }

  const fill = (s: string, n: number | string) => s.replace("{n}", String(n));
  const items: [string, string | null][] = [
    [dict.hhAge, profile.birthYear != null ? fill(dict.hhAgeVal, thisYear() - profile.birthYear) : null],
    [dict.hhDeps, profile.dependents == null ? null : profile.dependents >= 3 ? dict.hhDeps3 : fill(dict.hhDepsVal, profile.dependents)],
    [dict.hhSpouse, profile.spouseHasIncome == null ? null : profile.spouseHasIncome ? dict.wealthInsureSpouseIncomeYes : dict.hhSpouseNo],
  ];

  return (
    <section className={`hh-card${editing ? " editing" : ""}`} aria-labelledby="hh-title">
      <div className="hh-head">
        <h2 id="hh-title" className="hh-title">
          {dict.hhTitle}
        </h2>
        <span className="hh-note">{dict.hhNote}</span>
        <button className={`hh-btn${editing || missing ? " primary" : ""}`} aria-expanded={editing} aria-controls="hh-body" disabled={saving} onClick={toggle}>
          {editing ? dict.hhDone : missing ? dict.hhFill : dict.hhEdit}
        </button>
      </div>
      <div id="hh-body">
        {!editing ? (
          <dl className="hh-view">
            {items.map(([label, value]) => (
              <div key={label}>
                <dt>{label}</dt>
                <dd className={value == null ? "unset" : undefined}>{value ?? dict.hhUnset}</dd>
              </div>
            ))}
          </dl>
        ) : (
          <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
            <div className="hh-form">
              <div className="hh-field">
                <label htmlFor="hh-age" className="hh-label">
                  {dict.hhAge}
                </label>
                <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
                  <input
                    id="hh-age"
                    className="hh-input"
                    type="text"
                    inputMode="numeric"
                    autoComplete="off"
                    value={age}
                    placeholder={dict.hhAgePh}
                    onChange={(e) => setAge(e.target.value.replace(/[^0-9]/g, ""))}
                  />
                  <span className="hh-note">{dict.hhAgeUnit}</span>
                </div>
              </div>
              <div className="hh-field" role="group" aria-labelledby="hh-deps-l">
                <span id="hh-deps-l" className="hh-label">
                  {dict.hhDeps}
                </span>
                <div className="hh-segs">
                  {DEPS.map((n) => (
                    <button key={n} type="button" className="hh-seg" aria-pressed={deps === n || (n === 3 && deps != null && deps > 3)} onClick={() => setDeps(n)}>
                      {n === 3 ? "3+" : n}
                    </button>
                  ))}
                </div>
              </div>
              {deps != null && deps > 0 && (
                <div className="hh-field">
                  <label htmlFor="hh-child" className="hh-label">
                    {dict.wealthInsureYoungestChildAgeLabel}
                  </label>
                  <input
                    id="hh-child"
                    className="hh-input"
                    type="text"
                    inputMode="numeric"
                    autoComplete="off"
                    value={child}
                    onChange={(e) => setChild(e.target.value.replace(/[^0-9]/g, ""))}
                  />
                </div>
              )}
              <div className="hh-field" role="group" aria-labelledby="hh-sp-l">
                <span id="hh-sp-l" className="hh-label">
                  {dict.hhSpouse}
                </span>
                <div className="hh-segs">
                  {(
                    [
                      [true, dict.wealthInsureSpouseIncomeYes],
                      [false, dict.hhSpouseNo],
                    ] as const
                  ).map(([v, label]) => (
                    <button key={label} type="button" className="hh-seg" aria-pressed={spouse === v} onClick={() => setSpouse(v)}>
                      {label}
                    </button>
                  ))}
                </div>
              </div>
            </div>
            <span className="hh-hint">{dict.hhDepsHint}</span>
          </div>
        )}
      </div>
    </section>
  );
}
