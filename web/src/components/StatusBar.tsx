import type { Status } from "../api";
import type { Dictionary } from "../i18n";

interface Props {
  status: Status;
  dict: Dictionary;
}

// The shell-level status line rendered on every page (Phase 5 sidebar
// layout, see docs/phase-5-sidebar-layout.md): "WATCHING 14 · SPY +0.42% ·
// LAST CLOSE 2026-07-15" — genuinely live data, not decoration. The
// "ARGUS ▮" wordmark used to live here but moved to Sidebar's top, since
// the status bar itself is now shared shell chrome rather than a
// dashboard-only element.
export function StatusBar({ status, dict }: Props) {
  const sign = status.spyChangePct >= 0 ? "+" : "";
  return (
    <div className="status-bar">
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
