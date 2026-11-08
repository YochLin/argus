import { getJSONAuthed, postJSON } from "./http";
import { movesNetWorth } from "./netWorth";

// --- Phase 17: the settings page (docs/phase-17-web-settings.md) ---
// Mirrors internal/web/settings.go: the server owns the key list, each key's
// kind (how to edit and validate it) and its default; SettingsView adds the
// names, explanations and layout.

export type SettingKind = "secret" | "text" | "number" | "int" | "toggle" | "model" | "discount";

export interface Setting {
  key: string;
  // group is an id ("telegram"/"data"/"sinopac"/"risk"/"ai"), not a heading.
  group: string;
  kind: SettingKind;
  // secret keys never send their value back; read isSet instead.
  value: string;
  isSet: boolean;
  // What the process uses while the key is unset or cleared.
  default?: string;
}

// The settings page's 重設整個管理帳戶 (Phase 28 D): when it last ran (null:
// never), and where the server writes the backup it takes first.
export interface WealthResetInfo {
  lastResetAt: string | null;
  backupDir?: string;
  backup?: string;
}

export function fetchWealthReset(): Promise<WealthResetInfo> {
  return getJSONAuthed("/api/wealth/reset");
}

export function resetWealth(): Promise<WealthResetInfo> {
  return movesNetWorth(postJSON("/api/wealth/reset", {}));
}

export function fetchSettings(): Promise<{ settings: Setting[] }> {
  return getJSONAuthed("/api/settings");
}

// A key omitted here (or sent empty) keeps its current value — that's what
// lets the secret inputs render blank without a save wiping them; null clears
// it. Success means the server has written .env and is about to exit for a
// restart.
export function saveSettings(updates: Record<string, string | null>): Promise<{ message: string }> {
  return postJSON("/api/settings", updates);
}
