import { useEffect, useState } from "react";
import { ApiError, fetchSettings, saveSettings, type Setting } from "../api";
import type { Dictionary } from "../i18n";

interface Props {
  dict: Dictionary;
  onUnauthorized: (retry: () => void) => void;
}

// group ids come from internal/web/settings.go's settingKeys; an id with no
// entry here falls back to the raw id rather than rendering nothing, so a new
// server-side group is visible (if unpolished) before this map catches up.
const groupLabelKey: Record<string, keyof Dictionary> = {
  telegram: "settingsGroupTelegram",
  data: "settingsGroupData",
  sinopac: "settingsGroupSinopac",
};

// Phase 17 PR3 (docs/phase-17-web-settings.md §5): the connection/credential
// settings page. Field labels are the env var names themselves — they're what
// the operator already knows from .env and from every error message, and it
// keeps this component generic over whatever /api/settings returns.
//
// Saving is a one-way door within the page's lifetime: the server writes .env
// and exits so its supervisor restarts it, so there's nothing to re-fetch
// afterwards and the form stays disabled behind a reload button.
export function SettingsView({ dict, onUnauthorized }: Props) {
  const [settings, setSettings] = useState<Setting[] | null>(null);
  // edits holds only what the user typed. Untouched fields are never sent, so
  // a blank secret input can't wipe a working token (see saveSettings).
  const [edits, setEdits] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function load() {
    fetchSettings()
      .then((r) => setSettings(r.settings))
      .catch((e) => {
        if (e instanceof ApiError && e.status === 401) {
          onUnauthorized(load);
        } else {
          setError(e instanceof ApiError ? e.message : dict.error);
        }
      });
  }

  useEffect(load, []);

  async function save() {
    setBusy(true);
    setError(null);
    try {
      await saveSettings(edits);
      setSaved(true);
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        onUnauthorized(save);
      } else {
        setError(e instanceof ApiError ? e.message : dict.error);
      }
    } finally {
      setBusy(false);
    }
  }

  if (settings === null) {
    return <div className="card">{error ?? dict.loading}</div>;
  }

  const groups = settings.reduce<Array<{ group: string; items: Setting[] }>>((acc, item) => {
    const last = acc[acc.length - 1];
    if (last && last.group === item.group) last.items.push(item);
    else acc.push({ group: item.group, items: [item] });
    return acc;
  }, []);

  const dirty = Object.values(edits).some((v) => v.trim() !== "");

  return (
    <div className="settings-view">
      <div className="card">
        <div className="eyebrow">{dict.settingsTitle}</div>
        <p>{dict.settingsIntro}</p>
      </div>

      {groups.map(({ group, items }) => (
        <div className="card" key={group}>
          <div className="eyebrow">
            {groupLabelKey[group] ? dict[groupLabelKey[group]] : group}
          </div>
          {items.map((item) => (
            <label className="form-field" key={item.key}>
              <span className="mono">{item.key}</span>
              <input
                className="mono"
                type={item.secret ? "password" : "text"}
                autoComplete="off"
                disabled={saved}
                placeholder={item.secret ? (item.isSet ? dict.settingsSecretSet : dict.settingsSecretUnset) : ""}
                value={edits[item.key] ?? (item.secret ? "" : item.value)}
                onChange={(e) => setEdits((prev) => ({ ...prev, [item.key]: e.target.value }))}
              />
            </label>
          ))}
          {group === "sinopac" && <p className="mono settings-note">{dict.settingsSinopacDaemonNote}</p>}
        </div>
      ))}

      <div className="card">
        {error && <div className="error-message">{error}</div>}
        {saved ? (
          <div className="modal-actions">
            <div className="success-message">{dict.settingsRestarting}</div>
            <button className="btn-primary" onClick={() => window.location.reload()}>
              {dict.settingsReload}
            </button>
          </div>
        ) : (
          <div className="modal-actions">
            <button className="btn-primary" disabled={busy || !dirty} onClick={save}>
              {dict.settingsSave}
            </button>
          </div>
        )}
      </div>
    </div>
  );
}
