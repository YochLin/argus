import { describe, it, expect } from "vitest";
import type { Setting } from "./api";
import { getDictionary } from "./i18n";
import { chips, initialVals, payload, validate, type FormState } from "./settingsFields";

const dict = getDictionary("zh");

const settings: Setting[] = [
  { key: "FINNHUB_API_KEY", group: "data", kind: "secret", value: "", isSet: true },
  { key: "SINOPAC_SKIP_TICKERS", group: "sinopac", kind: "text", value: "0050", isSet: true },
  { key: "SINOPAC_SYNC_LIVE", group: "sinopac", kind: "toggle", value: "", isSet: false },
  { key: "STOP_LOSS_PCT", group: "risk", kind: "number", value: "10", isSet: true, default: "10" },
  { key: "STOP_LOSS_PCT_TW", group: "risk", kind: "number", value: "0", isSet: true, default: "12" },
  { key: "TW_BROKER_FEE_DISCOUNT", group: "risk", kind: "discount", value: "0.28", isSet: true, default: "1" },
  { key: "CLAUDE_CHAT_MODEL", group: "ai", kind: "model", value: "", isSet: false, default: "sonnet" },
];
const byKey = Object.fromEntries(settings.map((s) => [s.key, s]));
const form = (vals: Record<string, string> = {}, cleared: Record<string, boolean> = {}): FormState => ({
  vals: { ...initialVals(settings), ...vals },
  cleared,
});

describe("settings payload", () => {
  it("sends only what changed", () => {
    expect(payload(settings, form())).toEqual({});
    // Picking the model that is already the default isn't a change.
    expect(payload(settings, form({ CLAUDE_CHAT_MODEL: "sonnet" }))).toEqual({});
  });

  it("clears with null, including a non-secret field emptied by hand", () => {
    expect(payload(settings, form({ SINOPAC_SKIP_TICKERS: "" }, { STOP_LOSS_PCT: true }))).toEqual({
      SINOPAC_SKIP_TICKERS: null,
      STOP_LOSS_PCT: null,
    });
  });

  it("sends a typed secret and a toggle as true/false", () => {
    expect(payload(settings, form({ FINNHUB_API_KEY: " new ", SINOPAC_SYNC_LIVE: "true" }))).toEqual({
      FINNHUB_API_KEY: "new",
      SINOPAC_SYNC_LIVE: "true",
    });
  });
});

describe("settings validation and chips", () => {
  it("rejects what the server would", () => {
    expect(validate(byKey.STOP_LOSS_PCT, form({ STOP_LOSS_PCT: "101" }), dict)).toBe(dict.settingsErrPct);
    expect(validate(byKey.STOP_LOSS_PCT, form({ STOP_LOSS_PCT: "1e3" }), dict)).toBe(dict.settingsErrNum);
    expect(validate(byKey.TW_BROKER_FEE_DISCOUNT, form({ TW_BROKER_FEE_DISCOUNT: "0" }), dict)).toBe(dict.settingsErrDisc);
    expect(validate(byKey.STOP_LOSS_PCT, form(), dict)).toBe("");
  });

  it("says what a 0 means and what the discount comes to", () => {
    expect(chips(byKey.STOP_LOSS_PCT_TW, byKey, form(), dict)).toEqual([{ text: "沿用美股：10%", tone: "accent" }]);
    expect(chips(byKey.STOP_LOSS_PCT, byKey, form({ STOP_LOSS_PCT: "0" }), dict)).toEqual([
      { text: dict.settingsZeroOff, tone: "muted" },
    ]);
    expect(chips(byKey.TW_BROKER_FEE_DISCOUNT, byKey, form(), dict)).toEqual([{ text: "＝ 2.8 折", tone: "muted" }]);
    expect(chips(byKey.TW_BROKER_FEE_DISCOUNT, byKey, form(), getDictionary("en"))[0].text).toBe("Pays 28% of the full fee");
  });
});
