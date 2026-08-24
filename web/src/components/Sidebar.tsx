import { useEffect, useRef, useState, type MouseEvent, type ReactNode } from "react";
import type { Dictionary } from "../i18n";
import { currencySymbol, type Market, type Status } from "../api";
import { formatValue } from "./KpiCard";

// Persisted the same way as App.tsx's langStorageKey/themeStorageKey — a
// viewer preference, not server state.
const devModeStorageKey = "argus-dev-mode";

interface Props {
  path: string;
  onNavigate: (path: string) => void;
  dict: Dictionary;
  market: Market;
  // status backs the bottom account-overview card (Figma reference layout)
  // — null while /api/status hasn't resolved yet, same "render nothing"
  // degrade App.tsx's own StatusBar placeholder already uses.
  status: Status | null;
  // writable (Phase 5 §B) gates the /import nav link — same "hidden
  // entirely, not just disabled" convention as every other write-only
  // entry point (TopBar's "+ Trade", ChartListView's remove button).
  writable: boolean;
  // paperEnabled (Phase 11 PR4) gates the /paper nav link the same way —
  // hidden entirely when PAPER_DB_PATH isn't configured server-side.
  paperEnabled: boolean;
  // llmAuditEnabled (Phase 19, WEB_LLM_AUDIT) gates the /llm nav link the
  // same way — hidden entirely when unset server-side.
  llmAuditEnabled: boolean;
}

// /round (the detail page reached by clicking a row in /rounds) has no nav
// link of its own — same reasoning as a chart drill-down page not
// appearing in a top nav — see App.tsx's comment on why this is a
// hand-rolled path/popstate router rather than a routing library for an
// app this size. isActive additionally treats /round as belonging to the
// Rounds link, fixing NavBar's old exact-match-only behavior that left all
// three links un-highlighted on the round detail page.
const links: Array<{ path: string; label: (dict: Dictionary) => string; icon: ReactNode }> = [
  { path: "/", label: (d) => d.navDashboard, icon: <DashboardIcon /> },
  { path: "/risk", label: (d) => d.navRisk, icon: <RiskIcon /> },
  { path: "/calendar", label: (d) => d.navCalendar, icon: <CalendarIcon /> },
  { path: "/options", label: (d) => d.navOptions, icon: <OptionsIcon /> },
  { path: "/rounds", label: (d) => d.navRounds, icon: <RoundsIcon /> },
  { path: "/flow", label: (d) => d.navFlow, icon: <FlowIcon /> },
  { path: "/chart", label: (d) => d.navChart, icon: <ChartIcon /> },
  { path: "/reports", label: (d) => d.navReports, icon: <ReportsIcon /> },
  { path: "/recs", label: (d) => d.navRecs, icon: <RecsIcon /> },
];

const importLink = { path: "/import", label: (d: Dictionary) => d.navImport, icon: <ImportIcon /> };
const paperLink = { path: "/paper", label: (d: Dictionary) => d.navPaper, icon: <PaperIcon /> };
const llmLink = { path: "/llm", label: (d: Dictionary) => d.navLlm, icon: <LlmIcon /> };

function isActive(linkPath: string, path: string): boolean {
  return linkPath === path;
}

export function Sidebar({ path, onNavigate, dict, market, status, writable, paperEnabled, llmAuditEnabled }: Props) {
  const navLinks = [
    ...links,
    ...(paperEnabled ? [paperLink] : []),
    ...(llmAuditEnabled ? [llmLink] : []),
    ...(writable ? [importLink] : []),
  ];
  return (
    <div className="sidebar">
      <div className="sidebar-wordmark">
        ARGUS <span className="cursor">▮</span>
      </div>
      <nav className="sidebar-nav">
        {navLinks.map((link) => (
          <a
            key={link.path}
            href={link.path}
            className={`side-link${isActive(link.path, path) ? " active" : ""}`}
            onClick={(e: MouseEvent) => {
              e.preventDefault();
              onNavigate(link.path);
            }}
          >
            <span className="side-link-icon">{link.icon}</span>
            <span className="side-link-label">{link.label(dict)}</span>
          </a>
        ))}
      </nav>
      {status && (
        <AccountMenu dict={dict} market={market} status={status} writable={writable} onNavigate={onNavigate} />
      )}
    </div>
  );
}

// The design-canvas reference's account-switcher: a click-to-open menu
// anchored above the account card, holding a Settings entry and a dev-mode
// toggle. Real multi-account switching (the reference's account list) isn't
// built yet, so this menu only ever has these two static rows — Settings
// moved here from the main nav list to match the reference's placement.
function AccountMenu({
  dict,
  market,
  status,
  writable,
  onNavigate,
}: {
  dict: Dictionary;
  market: Market;
  status: Status;
  writable: boolean;
  onNavigate: (path: string) => void;
}) {
  const [open, setOpen] = useState(false);
  // ponytail: devMode is a bare viewer-side toggle with nothing wired to it
  // yet — no debug view exists to gate on it. Add that behavior (and read
  // this flag) when there's an actual debug view to show.
  const [devMode, setDevMode] = useState(() => localStorage.getItem(devModeStorageKey) === "1");
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const handler = (e: globalThis.MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [open]);

  function toggleDevMode() {
    setDevMode((v) => {
      const next = !v;
      localStorage.setItem(devModeStorageKey, next ? "1" : "0");
      return next;
    });
  }

  return (
    <div className="sidebar-account-wrap" ref={ref}>
      {open && (
        <div className="sidebar-account-menu">
          {writable && (
            <a
              href="/settings"
              className="sidebar-account-menu-item"
              onClick={(e: MouseEvent) => {
                e.preventDefault();
                setOpen(false);
                onNavigate("/settings");
              }}
            >
              <span className="sidebar-account-menu-icon">
                <SettingsIcon />
              </span>
              <span className="sidebar-account-menu-text">
                <span>{dict.navSettings}</span>
                <span className="sidebar-account-menu-note">{dict.setNavNote}</span>
              </span>
            </a>
          )}
          <button className="sidebar-account-menu-item" onClick={toggleDevMode} aria-pressed={devMode}>
            <span className="sidebar-account-menu-icon">
              <DevModeIcon />
            </span>
            <span className="sidebar-account-menu-text">
              <span>{dict.acctDevMode}</span>
              <span className="sidebar-account-menu-note">{dict.acctDevNote}</span>
            </span>
            <span className={`sidebar-switch${devMode ? " on" : ""}`}>
              <span className="sidebar-switch-knob" />
            </span>
          </button>
        </div>
      )}
      <button className="card sidebar-account" onClick={() => setOpen((v) => !v)} aria-expanded={open}>
        <div className="eyebrow">{dict.accountValue}</div>
        <div className="sidebar-account-value mono">
          {currencySymbol(market)}
          {status.accountValue.toLocaleString(undefined, { maximumFractionDigits: 0 })}
        </div>
        <div className={`sidebar-account-pnl mono ${status.netPnL >= 0 ? "profit" : "loss"}`}>
          {formatValue(status.netPnL, "currency", currencySymbol(market))} {dict.netPnL}
        </div>
        <div className="sidebar-account-stats">
          {status.tradeCount} {dict.trades} · {dict.winRate} {(status.winRate * 100).toFixed(1)}%
        </div>
      </button>
    </div>
  );
}

const iconProps = {
  width: 16,
  height: 16,
  viewBox: "0 0 16 16",
  stroke: "currentColor",
  strokeWidth: 1.5,
  fill: "none",
};

function DashboardIcon() {
  return (
    <svg {...iconProps} aria-hidden="true">
      <rect x="2" y="2" width="5" height="5" rx="1" />
      <rect x="9" y="2" width="5" height="5" rx="1" />
      <rect x="2" y="9" width="5" height="5" rx="1" />
      <rect x="9" y="9" width="5" height="5" rx="1" />
    </svg>
  );
}

function FlowIcon() {
  return (
    <svg {...iconProps} aria-hidden="true">
      <rect x="2" y="8" width="4" height="6" rx="0.5" />
      <rect x="7" y="4" width="4" height="10" rx="0.5" />
      <rect x="12" y="6" width="2" height="8" rx="0.5" />
    </svg>
  );
}

function CalendarIcon() {
  return (
    <svg {...iconProps} aria-hidden="true">
      <rect x="2" y="3" width="12" height="11" rx="1" />
      <line x1="2" y1="6" x2="14" y2="6" />
      <line x1="5" y1="1.5" x2="5" y2="4" />
      <line x1="11" y1="1.5" x2="11" y2="4" />
    </svg>
  );
}

function RoundsIcon() {
  return (
    <svg {...iconProps} aria-hidden="true">
      <line x1="4" y1="2" x2="4" y2="14" />
      <rect x="2.5" y="5" width="3" height="5" />
      <line x1="8" y1="1" x2="8" y2="15" />
      <rect x="6.5" y="3" width="3" height="7" />
      <line x1="12" y1="3" x2="12" y2="13" />
      <rect x="10.5" y="6" width="3" height="4" />
    </svg>
  );
}

function RiskIcon() {
  return (
    <svg {...iconProps} aria-hidden="true">
      <path d="M8 1.5 14 4.5 V9 C14 12 11.5 14 8 14.5 C4.5 14 2 12 2 9 V4.5 Z" />
      <line x1="8" y1="5.5" x2="8" y2="9" />
      <line x1="8" y1="10.5" x2="8" y2="11" />
    </svg>
  );
}

function ChartIcon() {
  return (
    <svg {...iconProps} aria-hidden="true">
      <polyline points="2,11 6,6 9,8.5 14,3" />
      <line x1="2" y1="14" x2="14" y2="14" />
    </svg>
  );
}

function ReportsIcon() {
  return (
    <svg {...iconProps} aria-hidden="true">
      <rect x="2" y="1.5" width="12" height="13" rx="1" />
      <line x1="4.5" y1="5" x2="11.5" y2="5" />
      <line x1="4.5" y1="8" x2="11.5" y2="8" />
      <line x1="4.5" y1="11" x2="9" y2="11" />
    </svg>
  );
}

function RecsIcon() {
  return (
    <svg {...iconProps} aria-hidden="true">
      <circle cx="8" cy="8" r="6" />
      <path d="M8 5 V8 L10.5 9.5" />
    </svg>
  );
}

function OptionsIcon() {
  return (
    <svg {...iconProps} aria-hidden="true">
      <circle cx="8" cy="8" r="6.5" />
      <path d="M6 6.5 C6 5.4 6.9 4.5 8 4.5 S10 5.4 10 6.3 C10 7.6 8 7.8 8 9" />
      <line x1="8" y1="10.8" x2="8" y2="11.2" />
    </svg>
  );
}

function PaperIcon() {
  return (
    <svg {...iconProps} aria-hidden="true">
      <rect x="3" y="2" width="10" height="12" rx="1" />
      <line x1="5.5" y1="5.5" x2="10.5" y2="5.5" />
      <line x1="5.5" y1="8" x2="10.5" y2="8" />
      <line x1="5.5" y1="10.5" x2="8.5" y2="10.5" />
    </svg>
  );
}

function LlmIcon() {
  return (
    <svg {...iconProps} aria-hidden="true">
      <circle cx="6.5" cy="6.5" r="4.5" />
      <line x1="9.8" y1="9.8" x2="14" y2="14" />
    </svg>
  );
}

function SettingsIcon() {
  return (
    <svg {...iconProps} aria-hidden="true">
      <circle cx="8" cy="8" r="2.5" />
      <path d="M8 1.5 V3 M8 13 V14.5 M1.5 8 H3 M13 8 H14.5 M3.4 3.4 L4.5 4.5 M11.5 11.5 L12.6 12.6 M12.6 3.4 L11.5 4.5 M4.5 11.5 L3.4 12.6" />
    </svg>
  );
}

function DevModeIcon() {
  return (
    <svg {...iconProps} aria-hidden="true">
      <path d="M5.5 5 L2.5 8 L5.5 11" />
      <path d="M10.5 5 L13.5 8 L10.5 11" />
    </svg>
  );
}

function ImportIcon() {
  return (
    <svg {...iconProps} aria-hidden="true">
      <path d="M8 1.5 V10" />
      <path d="M5 7 L8 10 L11 7" />
      <path d="M2.5 12 V13.5 H13.5 V12" />
    </svg>
  );
}
