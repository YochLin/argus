import { useEffect, useState, type ReactElement } from "react";
import { ApiError, fetchSettings, saveSettings, type Setting } from "../api";
import type { Dictionary } from "../i18n";
import { settingFieldText } from "../i18n/settingFields";
import {
  atrActive,
  chips,
  claudeModels,
  fieldMeta,
  fieldPairs,
  initialVals,
  isDirty,
  payload,
  validate,
  withUnit,
  type FormState,
} from "../settingsFields";
import { NewsSourceBlock } from "./NewsSourceBlock";
import { WealthResetZone } from "./WealthResetZone";

interface Props {
  dict: Dictionary;
  onUnauthorized: (retry: () => void) => void;
  // wealth: the page was opened from the wealth account, which adds the
  // reset danger zone at the bottom (Phase 28 D).
  wealth?: boolean;
}

const groups: Array<{ id: string; title: keyof Dictionary; hint?: keyof Dictionary; foot?: keyof Dictionary }> = [
  { id: "telegram", title: "settingsGroupTelegram" },
  { id: "data", title: "settingsGroupData" },
  { id: "sinopac", title: "settingsGroupSinopac", foot: "settingsSinopacFoot" },
  { id: "risk", title: "settingsGroupRisk" },
  { id: "ai", title: "settingsGroupAi", hint: "settingsAiHint" },
];

type Phase = "idle" | "saving" | "restarting" | "done";

// Saving ends the process (internal/web/settings.go's exitForRestart), so
// "applied" can only be inferred: wait for it to go down, then for
// /api/status to answer again. Plain fetch, not getJSON, which would answer
// from mock data while the server is down.
async function waitForRestart(): Promise<boolean> {
  await new Promise((r) => setTimeout(r, 1500));
  for (let i = 0; i < 60; i++) {
    try {
      if ((await fetch("/api/status")).ok) return true;
    } catch {
      // still down
    }
    await new Promise((r) => setTimeout(r, 1000));
  }
  return false;
}

// Phase 17's settings page, extended to the exit-discipline numbers and the
// model choices: everything here is an env var written to .env, applied by a
// restart — except the news-source blocklist at the bottom, which writes the
// database directly and applies at once.
export function SettingsView({ dict, onUnauthorized, wealth = false }: Props) {
  const [settings, setSettings] = useState<Setting[] | null>(null);
  const [form, setForm] = useState<FormState>({ vals: {}, cleared: {} });
  const [phase, setPhase] = useState<Phase>("idle");
  const [error, setError] = useState<string | null>(null);

  function load() {
    fetchSettings()
      .then((r) => {
        setSettings(r.settings);
        setForm({ vals: initialVals(r.settings), cleared: {} });
      })
      .catch((e) => {
        if (e instanceof ApiError && e.status === 401) onUnauthorized(load);
        else setError(e instanceof ApiError ? e.message : dict.error);
      });
  }

  useEffect(load, []);

  if (settings === null) {
    return <div className="card">{error ?? dict.loading}</div>;
  }

  const byKey = Object.fromEntries(settings.map((s) => [s.key, s]));
  const visible = (s: Setting) => {
    const gate = fieldMeta[s.key]?.showIf;
    return !gate || form.vals[gate] === "true";
  };
  const shown = settings.filter(visible);
  const dirtyN = shown.filter((s) => isDirty(s, form)).length;
  const errN = shown.filter((s) => validate(s, form, dict)).length;
  const busy = phase !== "idle";
  const canSave = dirtyN > 0 && errN === 0 && !busy;

  const setVal = (key: string, v: string) => setForm((f) => ({ ...f, vals: { ...f.vals, [key]: v } }));
  const clear = (key: string) => setForm((f) => ({ ...f, cleared: { ...f.cleared, [key]: true } }));
  const undo = (key: string) =>
    setForm((f) => {
      const cleared = { ...f.cleared };
      delete cleared[key];
      const s = byKey[key];
      return { cleared, vals: { ...f.vals, [key]: s.kind === "secret" ? "" : s.value } };
    });

  async function save() {
    setPhase("saving");
    setError(null);
    try {
      await saveSettings(payload(shown, form));
      setPhase("restarting");
      // .env is written either way; a timeout only puts the restart in doubt.
      if (!(await waitForRestart())) setError(dict.settingsRestartTimeout);
      setPhase("done");
    } catch (e) {
      setPhase("idle");
      if (e instanceof ApiError && e.status === 401) onUnauthorized(save);
      else setError(e instanceof ApiError ? e.message : dict.error);
    }
  }

  let status = dict.settingsClean;
  let statusTone = "";
  if (phase === "restarting" || phase === "saving") status = dict.settingsRestarting;
  else if (phase === "done") {
    status = error ?? dict.settingsDone;
    statusTone = error ? "err" : "ok";
  } else if (errN > 0) {
    status = dict.settingsErrN.replace("%s", String(errN));
    statusTone = "err";
  } else if (dirtyN > 0) {
    status = dict.settingsDirtyN.replace("%s", String(dirtyN));
    statusTone = "dirty";
  }

  const field = (s: Setting, compact: boolean) => (
    <Field
      key={s.key}
      dict={dict}
      s={s}
      form={form}
      compact={compact}
      chipList={chips(s, byKey, form, dict)}
      note={fieldMeta[s.key]?.atrNote && !form.cleared[s.key] && atrActive(byKey, form, dict) ? dict.settingsAtrNote : ""}
      disabled={busy}
      onChange={(v) => setVal(s.key, v)}
      onClear={() => clear(s.key)}
      onUndo={() => undo(s.key)}
    />
  );

  return (
    <div className="settings-view">
      <div className="settings-header-copy">
        <h1>{dict.settingsTitle}</h1>
        <p>{dict.settingsIntro}</p>
      </div>

      <div className="settings-savebar">
        <div className={`settings-status ${statusTone}`} role="status" aria-live="polite">
          {busy && phase !== "done" && <span className="settings-spinner" aria-hidden="true" />}
          {(statusTone === "dirty" || statusTone === "err") && <span className="settings-dot" aria-hidden="true" />}
          <span>{status}</span>
          {phase === "restarting" && <span className="settings-status-sub">{dict.settingsRestartingSub}</span>}
        </div>
        {phase === "idle" && dirtyN > 0 && (
          <button type="button" onClick={() => setForm({ vals: initialVals(settings), cleared: {} })}>
            {dict.settingsDiscard}
          </button>
        )}
        {phase === "done" ? (
          <button type="button" className="btn-primary" onClick={() => window.location.reload()}>
            {dict.settingsReload}
          </button>
        ) : (
          <button type="button" className="btn-primary" disabled={!canSave} onClick={save}>
            {dict.settingsSave}
          </button>
        )}
      </div>
      {error && phase !== "done" && <div className="settings-save-error">{error}</div>}

      {groups.map((g) => {
        const items = settings.filter((s) => s.group === g.id && visible(s));
        if (items.length === 0) return null;
        const paired = new Set(fieldPairs.flatMap((p) => p.keys));
        const rows: ReactElement[] = [];
        for (const s of items) {
          const pair = fieldPairs.find((p) => p.keys[0] === s.key);
          if (pair && byKey[pair.keys[1]]) {
            rows.push(
              <div className="settings-row settings-pair" key={s.key}>
                <h3>{dict[pair.title] as string}</h3>
                <div className="settings-pair-grid">
                  {pair.keys.map((k) => (
                    <div className="settings-pair-cell" key={k}>
                      {field(byKey[k], true)}
                    </div>
                  ))}
                </div>
              </div>,
            );
          } else if (!paired.has(s.key)) {
            const nested = !!fieldMeta[s.key]?.showIf;
            rows.push(
              <div className={`settings-row${nested ? " settings-nested" : ""}`} key={s.key}>
                {field(s, false)}
              </div>,
            );
          }
        }
        return (
          <section className="settings-group" key={g.id} aria-labelledby={`settings-${g.id}`}>
            <header>
              <h2 id={`settings-${g.id}`}>{dict[g.title] as string}</h2>
              {g.hint && <p>{dict[g.hint] as string}</p>}
            </header>
            {rows}
            {g.foot && <div className="settings-foot">{dict[g.foot] as string}</div>}
          </section>
        );
      })}

      <div className="settings-live-divider">
        <h2>{dict.settingsLiveTitle}</h2>
        <p>{dict.settingsLiveSub}</p>
      </div>
      <NewsSourceBlock dict={dict} onUnauthorized={onUnauthorized} />

      {wealth && <WealthResetZone dict={dict} onUnauthorized={onUnauthorized} />}
    </div>
  );
}

interface FieldProps {
  dict: Dictionary;
  s: Setting;
  form: FormState;
  compact: boolean;
  chipList: Array<{ text: string; tone: string }>;
  note: string;
  disabled: boolean;
  onChange: (v: string) => void;
  onClear: () => void;
  onUndo: () => void;
}

function Field({ dict, s, form, compact, chipList, note, disabled, onChange, onClear, onUndo }: FieldProps) {
  const info = settingFieldText[dict.langId]?.[s.key];
  const name = compact ? (s.key.endsWith("_TW") ? dict.settingsCellTW : dict.settingsCellUS) : (info?.name ?? s.key);
  const value = form.vals[s.key] ?? "";
  const cleared = !!form.cleared[s.key];
  const err = validate(s, form, dict);
  const id = `fld-${s.key}`;
  const errId = `err-${s.key}`;
  const unit = fieldMeta[s.key]?.unit;
  const hasValue = s.kind === "secret" ? s.isSet || value !== "" : s.value !== "" || value !== "";
  const canClear = !cleared && hasValue && ["secret", "text", "number", "int", "discount"].includes(s.kind);
  const placeholder =
    s.kind === "secret"
      ? dict.settingsKeepBlank
      : s.default !== undefined && s.default !== ""
        ? dict.settingsDefPh.replace("%s", s.default)
        : s.key === "SEC_USER_AGENT"
          ? "argus you@example.com"
          : "";
  const inputProps = {
    id,
    value,
    placeholder,
    disabled,
    autoComplete: "off",
    "aria-invalid": !!err,
    "aria-describedby": err ? errId : undefined,
    onChange: (e: { target: { value: string } }) => onChange(e.target.value),
  };

  let control: ReactElement;
  if (s.kind === "toggle") {
    const on = value === "true";
    control = (
      <button
        id={id}
        type="button"
        role="switch"
        aria-checked={on}
        className={`settings-switch${on ? " on" : ""}`}
        disabled={disabled}
        onClick={() => onChange(on ? "false" : "true")}
      >
        <span className="settings-switch-track" aria-hidden="true">
          <span />
        </span>
        {on ? dict.settingsOn : dict.settingsOff}
      </button>
    );
  } else if (s.kind === "model") {
    const current = value || s.default || "";
    // A full model ID already in .env stays selectable, or opening the page
    // would quietly offer to replace it.
    const options = current && !claudeModels.includes(current) ? [...claudeModels, current] : claudeModels;
    control = (
      <select id={id} value={current} disabled={disabled} onChange={(e) => onChange(e.target.value)}>
        {options.map((o) => (
          <option key={o} value={o}>
            {o === s.default ? o + dict.settingsDefSuffix : o}
          </option>
        ))}
      </select>
    );
  } else if (s.kind === "number" || s.kind === "discount" || s.kind === "int") {
    control = (
      <div className={`settings-number${err ? " invalid" : ""}`}>
        <input type="text" inputMode={s.kind === "int" ? "numeric" : "decimal"} {...inputProps} />
        {unit && <span aria-hidden="true">{unit === "pct" ? "%" : dict.settingsUnitMult}</span>}
      </div>
    );
  } else {
    control = (
      <input
        type={s.kind === "secret" ? "password" : "text"}
        spellCheck={false}
        className={err ? "invalid" : undefined}
        {...inputProps}
        autoComplete={s.kind === "secret" ? "new-password" : "off"}
      />
    );
  }

  const clearedText =
    s.default !== undefined && s.default !== ""
      ? dict.settingsClearedDef.replace("%s", withUnit(s.key, s.default, dict))
      : dict.settingsCleared;
  const dirty = isDirty(s, form);

  return (
    <div className={`settings-field${compact ? " compact" : ""}`}>
      <div className="settings-field-info">
        <div className="settings-field-name">
          <label htmlFor={id}>{name}</label>
          <code>{s.key}</code>
          {dirty && <span className="settings-unsaved">{dict.settingsUnsaved}</span>}
        </div>
        {info && <p>{info.desc}</p>}
      </div>
      <div className="settings-field-ctl">
        {cleared ? (
          <div className="settings-cleared" role="status">
            <span>{clearedText}</span>
            <button type="button" disabled={disabled} onClick={onUndo}>
              {dict.settingsUndo}
            </button>
          </div>
        ) : (
          <div className="settings-ctl-row">
            <div className="settings-ctl-main">{control}</div>
            {canClear && (
              <button
                type="button"
                className="settings-clear"
                disabled={disabled}
                aria-label={`${dict.settingsClear} ${info?.name ?? s.key}`}
                title={`${dict.settingsClear} ${info?.name ?? s.key}`}
                onClick={onClear}
              >
                ×
              </button>
            )}
          </div>
        )}
        {chipList.length > 0 && (
          <div className="settings-chips">
            {chipList.map((c) => (
              <span key={c.text} className={`settings-chip ${c.tone}`}>
                {c.text}
              </span>
            ))}
          </div>
        )}
        {err && (
          <div id={errId} className="settings-field-error" role="alert">
            {err}
          </div>
        )}
        {note && <div className="settings-field-note">{note}</div>}
      </div>
    </div>
  );
}
