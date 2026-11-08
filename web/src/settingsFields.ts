import type { Setting } from "./api";
import type { Dictionary } from "./i18n";

// Layout and meaning the server's /api/settings doesn't carry: units, what a
// 0 means, and which fields sit side by side. Keyed by env var; a key the
// server adds without an entry here still renders, as a plain field.
interface FieldMeta {
  unit?: "pct" | "mult";
  // What 0 means: the check is off, the TW field falls back to the US one
  // (bot.NewWithChannel), or the ATR multiple leaves only the fixed %.
  zero?: "off" | "inherit" | "fixed";
  from?: string;
  // The trailing-stop % fields: the ATR multiple replaces them when > 0.
  atrNote?: boolean;
  // Shown only while this toggle is on, nested under it.
  showIf?: string;
}

export const fieldMeta: Record<string, FieldMeta> = {
  STOP_LOSS_PCT: { unit: "pct", zero: "off" },
  STOP_LOSS_PCT_TW: { unit: "pct", zero: "inherit", from: "STOP_LOSS_PCT" },
  TRAILING_STOP_PCT: { unit: "pct", zero: "off", atrNote: true },
  TRAILING_STOP_PCT_TW: { unit: "pct", zero: "inherit", from: "TRAILING_STOP_PCT", atrNote: true },
  TRAILING_STOP_ATR_MULT: { unit: "mult", zero: "fixed" },
  RISK_PCT_PER_TRADE: { unit: "pct", zero: "off" },
  RISK_HEAT_PCT: { unit: "pct", zero: "off" },
  ANTIGRAVITY_MODEL: { showIf: "ANTIGRAVITY_ENABLED" },
};

// US|TW pairs rendered as one row with two cells.
export const fieldPairs: Array<{ keys: [string, string]; title: keyof Dictionary }> = [
  { keys: ["STOP_LOSS_PCT", "STOP_LOSS_PCT_TW"], title: "settingsPairStop" },
  { keys: ["TRAILING_STOP_PCT", "TRAILING_STOP_PCT_TW"], title: "settingsPairTrailing" },
];

export const claudeModels = ["opus", "sonnet", "haiku"];

export interface FormState {
  // What the user has in each input. Secrets start "" (the value never
  // reaches the browser); everything else starts at the loaded value.
  vals: Record<string, string>;
  cleared: Record<string, boolean>;
}

export function initialVals(settings: Setting[]): Record<string, string> {
  return Object.fromEntries(settings.map((s) => [s.key, s.kind === "secret" ? "" : s.value]));
}

const isToggleOn = (v: string) => v === "true";

export function isDirty(s: Setting, st: FormState): boolean {
  if (st.cleared[s.key]) return true;
  const v = st.vals[s.key] ?? "";
  if (s.kind === "secret") return v !== "";
  if (s.kind === "toggle") return isToggleOn(v) !== isToggleOn(s.value);
  // A model left on its default isn't a change, whether .env says it or not.
  if (s.kind === "model") return (v || s.default || "") !== (s.value || s.default || "");
  return v.trim() !== s.value;
}

// The value the process will run with after a save.
export function effective(s: Setting, st: FormState): string {
  if (st.cleared[s.key]) return s.default ?? "";
  const v = (st.vals[s.key] ?? "").trim();
  return v === "" ? (s.default ?? "") : v;
}

// Mirrors internal/web/settings.go's validateSetting, so a bad value is
// caught at the field instead of by a 400 for the whole save.
export function validate(s: Setting, st: FormState, dict: Dictionary): string {
  if (st.cleared[s.key]) return "";
  const raw = (st.vals[s.key] ?? "").trim();
  if (raw === "") return "";
  if (s.kind === "int") return /^-?\d+$/.test(raw) ? "" : dict.settingsErrInt;
  if (s.kind === "number" || s.kind === "discount") {
    if (!/^\d*\.?\d+$/.test(raw)) return dict.settingsErrNum;
    const n = parseFloat(raw);
    if (s.kind === "discount") return n > 0 && n <= 1 ? "" : dict.settingsErrDisc;
    const max = fieldMeta[s.key]?.unit === "mult" ? 20 : 100;
    if (n > max) return max === 20 ? dict.settingsErrMult : dict.settingsErrPct;
  }
  if (s.key === "SEC_USER_AGENT" && !/^\S+\s+\S+@\S+\.\S+$/.test(raw)) return dict.settingsErrSec;
  return "";
}

// The POST body: only what changed. A non-secret field emptied by hand means
// the same as Clear (null) — the server would otherwise read "" as "keep".
export function payload(settings: Setting[], st: FormState): Record<string, string | null> {
  const out: Record<string, string | null> = {};
  for (const s of settings) {
    if (!isDirty(s, st)) continue;
    const v = (st.vals[s.key] ?? "").trim();
    if (st.cleared[s.key]) out[s.key] = null;
    else if (s.kind === "toggle") out[s.key] = isToggleOn(v) ? "true" : "false";
    else if (s.kind === "model") out[s.key] = v || (s.default ?? "");
    else if (v === "" && s.kind !== "secret") out[s.key] = null;
    else out[s.key] = v;
  }
  return out;
}

export function withUnit(key: string, v: string, dict: Dictionary): string {
  const unit = fieldMeta[key]?.unit;
  if (unit === "pct") return `${v}%`;
  if (unit === "mult") return `${v} ${dict.settingsUnitMult}`;
  return v;
}

export interface Chip {
  text: string;
  tone: "accent" | "ok" | "muted";
}

const fmt2 = (n: number) => String(Math.round(n * 100) / 100);

export function chips(s: Setting, byKey: Record<string, Setting>, st: FormState, dict: Dictionary): Chip[] {
  if (st.cleared[s.key]) return [];
  if (s.kind === "secret") {
    if ((st.vals[s.key] ?? "") !== "") return [{ text: dict.settingsWillUpdate, tone: "accent" }];
    return [s.isSet ? { text: dict.settingsIsSet, tone: "ok" } : { text: dict.settingsNotSet, tone: "muted" }];
  }
  if (validate(s, st, dict)) return [];
  const n = parseFloat(effective(s, st));
  if (s.kind === "discount" && !Number.isNaN(n)) {
    return [{ text: n === 1 ? dict.settingsDiscNone : dict.settingsDisc.replace("%s", fmt2(n * parseFloat(dict.settingsDiscScale))), tone: "muted" }];
  }
  const meta = fieldMeta[s.key];
  if (!meta?.zero || n !== 0) return [];
  if (meta.zero === "off") return [{ text: dict.settingsZeroOff, tone: "muted" }];
  if (meta.zero === "fixed") return [{ text: dict.settingsFixedOnly, tone: "muted" }];
  const from = meta.from ? byKey[meta.from] : undefined;
  const us = from ? effective(from, st) : "";
  return [
    {
      text: parseFloat(us) === 0 || us === "" ? dict.settingsInheritOff : dict.settingsInherit.replace("%s", withUnit(from!.key, us, dict)),
      tone: "accent",
    },
  ];
}

export function atrActive(byKey: Record<string, Setting>, st: FormState, dict: Dictionary): boolean {
  const atr = byKey.TRAILING_STOP_ATR_MULT;
  if (!atr || validate(atr, st, dict)) return false;
  return parseFloat(effective(atr, st)) > 0;
}
