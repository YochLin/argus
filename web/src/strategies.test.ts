import { describe, expect, it } from "vitest";
import type { StrategyAlert } from "./api";
import { getDictionary } from "./i18n";
import { stratChannelNote, stratKey, stratText, stratValid } from "./strategies";

const dict = getDictionary("zh");

const alert = (over: Partial<StrategyAlert> = {}): StrategyAlert => ({
  type: "strategy_squeeze_breakout",
  date: "2026-09-04",
  channel: "watchlist",
  message: "m",
  verdict: null,
  ...over,
});

describe("stratValid", () => {
  it("every screen but the daily-weekly cross is not validated", () => {
    for (const type of ["strategy_squeeze_breakout", "strategy_box_bottom", "strategy_trend_pullback", "strategy_trust_follow"]) {
      const v = stratValid(dict, type, "AAPL");
      expect(v.tone).toBe("warn");
      expect(v.note).toBe(dict.stratNote_warn);
    }
  });
  it("the trend breakout carries its own sample note", () => {
    expect(stratValid(dict, "strategy_trend_breakout", "AAPL")).toMatchObject({ tone: "warn", note: dict.stratNote_breakout });
  });
  it("the daily-weekly cross is positive on TW and a contrarian reading on US", () => {
    expect(stratValid(dict, "strategy_mtf_cross", "2330")).toMatchObject({ tone: "ok", label: dict.stratValid_ok, note: dict.stratNote_mtfTw });
    expect(stratValid(dict, "strategy_mtf_cross", "AAPL")).toMatchObject({ tone: "bad", label: dict.stratValid_bad, note: dict.stratNote_mtfUs });
  });
});

describe("stratText / stratKey / stratChannelNote", () => {
  it("names the six screens", () => {
    expect(stratText(dict, "strategy_squeeze_breakout")).toEqual({ code: dict.stratCode_squeeze, name: dict.stratName_squeeze });
    expect(stratText(dict, "strategy_mtf_cross").code).toBe(dict.stratCode_mtf);
  });
  it("shows an unknown type as itself instead of blank", () => {
    expect(stratText(dict, "strategy_new_thing")).toEqual({ code: "strategy_new_thing", name: "strategy_new_thing" });
  });
  it("keys an alert like a pattern: type and date", () => {
    expect(stratKey(alert())).toBe("strategy_squeeze_breakout:2026-09-04");
  });
  it("says where it was pushed, with the signal date", () => {
    expect(stratChannelNote(dict, alert())).toBe(dict.stratPushedWatchlist.replace("%s", "2026-09-04"));
    expect(stratChannelNote(dict, alert({ channel: "scan" }))).toBe(dict.stratPushedScan.replace("%s", "2026-09-04"));
  });
});
