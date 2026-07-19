import type { Status } from "../api";
import type { Dictionary } from "../i18n";

interface Props {
  status: Status;
  dict: Dictionary;
}

// The dashboard's one signature element (docs/phase-5-web-dashboard.md):
// "ARGUS ▮ WATCHING 14 · SPY +0.42% · LAST CLOSE 2026-07-15" — a hundred-
// eyed watcher rendered as a terminal status line, and genuinely live data
// rather than pure decoration. The blinking cursor block respects
// prefers-reduced-motion via .cursor's CSS rule in theme.css.
export function StatusBar({ status, dict }: Props) {
  const sign = status.spyChangePct >= 0 ? "+" : "";
  return (
    <div className="status-bar">
      <span>ARGUS</span>
      <span className="cursor">▮</span>
      <span>
        {dict.watching} {status.watchingCount}
      </span>
      <span>·</span>
      <span className={status.spyChangePct >= 0 ? "profit" : "loss"}>
        SPY {sign}
        {status.spyChangePct.toFixed(2)}%
      </span>
      {status.lastCloseDate !== "" && (
        <>
          <span>·</span>
          <span>
            {dict.lastClose} {status.lastCloseDate}
          </span>
        </>
      )}
    </div>
  );
}
