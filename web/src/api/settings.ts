import { getJSONAuthed, postJSON } from "./http";
import { movesNetWorth } from "./netWorth";

// --- Phase 17: connection/credential settings (docs/phase-17-web-settings.md) ---
// Mirrors internal/web/settings.go. The server owns the whole key list —
// SettingsView renders whatever comes back, grouped by `group`, so adding a
// variable to settingKeys there needs no change here.

export interface Setting {
  key: string;
  // group is an id ("telegram"/"data"/"sinopac"), not a heading — i18n/
  // holds the display text, same split as calendar events' `kind`.
  group: string;
  // secret keys never send their value back; read isSet instead.
  secret: boolean;
  value: string;
  isSet: boolean;
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
// lets the secret inputs render blank without a save wiping them. Success
// means the server has written .env and is about to exit for a restart, so
// there is no fresh state to re-fetch afterwards.
export function saveSettings(updates: Record<string, string>): Promise<{ message: string }> {
  return postJSON("/api/settings", updates);
}
