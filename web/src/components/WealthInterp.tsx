import { useState } from "react";
import type { Dictionary } from "../i18n";

// The rules of thumb the interpretation lines read against. They are the same
// numbers as internal/service/wealth_checks.go's constants (the home page's
// 本月該做的事), so a page and its action item never disagree; change both.
export const RULE = {
  emergencyMonths: 6,
  emergencyMonthsDeps: 9,
  emergencyFloor: 3, // below this the line turns red
  debtRatioMax: 40,
  coverageMin: 80,
  retirementMin: 80,
  savingsRateMin: 20,
  riskLow: 25,
  riskHigh: 40,
};

export type Tone = "ok" | "warn" | "bad" | "data";

// Interp is the one line under a page's headline figure saying whether it is
// on target (the design's wIp text): ✓ on target, ! off it, ? not enough data.
export function Interp({ tone, text }: { tone: Tone; text: string }) {
  return (
    <div className={`ip-line ip-${tone}`}>
      <span className="ip-glyph" aria-hidden="true">
        {tone === "ok" ? "✓" : tone === "data" ? "?" : "!"}
      </span>
      <span>{text}</span>
    </div>
  );
}

// TermTip is the ⓘ beside a headline figure: a click opens what the term
// means. A click outside or Escape closes it.
export function TermTip({ dict, term, def }: { dict: Dictionary; term: string; def: string }) {
  const [open, setOpen] = useState(false);
  const label = dict.ipWhatIs.replace("{term}", term);
  return (
    <span className="ip-tip" onKeyDown={(e) => e.key === "Escape" && setOpen(false)}>
      <button type="button" className="ip-tip-btn" aria-label={label} aria-expanded={open} title={label} onClick={() => setOpen(!open)}>
        <svg width="13" height="13" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.4" aria-hidden="true">
          <circle cx="8" cy="8" r="6.5" />
          <path d="M8 7.2v3.8" />
          <circle cx="8" cy="5" r=".6" fill="currentColor" />
        </svg>
      </button>
      {open && (
        <>
          <button type="button" tabIndex={-1} aria-hidden="true" className="ip-tip-scrim" onClick={() => setOpen(false)} />
          <span role="dialog" aria-label={term} className="ip-tip-pop">
            <span className="ip-tip-head">
              <strong>{term}</strong>
              <button type="button" aria-label={dict.close} className="ip-tip-x" onClick={() => setOpen(false)}>
                ×
              </button>
            </span>
            <span>{def}</span>
          </span>
        </>
      )}
    </span>
  );
}
