import type { MouseEvent, ReactNode } from "react";
import type { Dictionary } from "../i18n";
import { currencySymbol, type Market, type Status } from "../api";
import { formatValue } from "./KpiCard";

interface Props {
  path: string;
  onNavigate: (path: string) => void;
  dict: Dictionary;
  market: Market;
  // status backs the bottom account-overview card (Figma reference layout)
  // — null while /api/status hasn't resolved yet, same "render nothing"
  // degrade App.tsx's own StatusBar placeholder already uses.
  status: Status | null;
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
  { path: "/rounds", label: (d) => d.navRounds, icon: <RoundsIcon /> },
  { path: "/chart", label: (d) => d.navChart, icon: <ChartIcon /> },
  { path: "/reports", label: (d) => d.navReports, icon: <ReportsIcon /> },
  { path: "/recs", label: (d) => d.navRecs, icon: <RecsIcon /> },
];

function isActive(linkPath: string, path: string): boolean {
  return linkPath === path || (linkPath === "/rounds" && path === "/round");
}

export function Sidebar({ path, onNavigate, dict, market, status }: Props) {
  return (
    <div className="sidebar">
      <div className="sidebar-wordmark">
        ARGUS <span className="cursor">▮</span>
      </div>
      <nav className="sidebar-nav">
        {links.map((link) => (
          <a
            key={link.path}
            href={link.path}
            className={`side-link${isActive(link.path, path) ? " active" : ""}`}
            onClick={(e: MouseEvent) => {
              e.preventDefault();
              onNavigate(link.path);
            }}
          >
            {link.icon}
            {link.label(dict)}
          </a>
        ))}
      </nav>
      {status && (
        <div className="card sidebar-account">
          <div className="eyebrow">{dict.accountValue}</div>
          <div className="sidebar-account-value mono">
            {formatValue(status.accountValue, "currency", currencySymbol(market))}
          </div>
          <div className={`sidebar-account-pnl mono ${status.netPnL >= 0 ? "profit" : "loss"}`}>
            {formatValue(status.netPnL, "currency", currencySymbol(market))} {dict.netPnL}
          </div>
          <div className="sidebar-account-stats">
            {status.tradeCount} {dict.trades} · {dict.winRate} {(status.winRate * 100).toFixed(1)}%
          </div>
        </div>
      )}
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
