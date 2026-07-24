import type { MouseEvent, ReactNode } from "react";
import type { Dictionary, Lang } from "../i18n";
import type { Market } from "../api";

interface Props {
  path: string;
  onNavigate: (path: string) => void;
  dict: Dictionary;
  market: Market;
  onMarketChange: (market: Market) => void;
  lang: Lang;
  onLangChange: (lang: Lang) => void;
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
  { path: "/calendar", label: (d) => d.navCalendar, icon: <CalendarIcon /> },
  { path: "/rounds", label: (d) => d.navRounds, icon: <RoundsIcon /> },
  { path: "/chart", label: (d) => d.navChart, icon: <ChartIcon /> },
  { path: "/reports", label: (d) => d.navReports, icon: <ReportsIcon /> },
];

function isActive(linkPath: string, path: string): boolean {
  return linkPath === path || (linkPath === "/rounds" && path === "/round");
}

// Phase 6's US/TW toggle (docs/phase-6-tw-market.md §4.4: "tab 或
// select,樣式從簡") lives here rather than as its own component — it's two
// buttons, not worth a dedicated file, and the sidebar is already the
// shell-level chrome every page shares (same reasoning as the wordmark
// living here instead of on each view).
export function Sidebar({ path, onNavigate, dict, market, onMarketChange, lang, onLangChange }: Props) {
  return (
    <div className="sidebar">
      <div className="sidebar-wordmark">
        ARGUS <span className="cursor">▮</span>
      </div>
      <div className="market-toggle" role="group" aria-label="market">
        <button
          className={`market-toggle-btn${market === "us" ? " active" : ""}`}
          onClick={() => onMarketChange("us")}
        >
          US
        </button>
        <button
          className={`market-toggle-btn${market === "tw" ? " active" : ""}`}
          onClick={() => onMarketChange("tw")}
        >
          TW
        </button>
      </div>
      {/* Language toggle — same two-button shape and CSS as the market
          toggle above. BOT_LANGUAGE only sets the first-visit default; this
          persists the user's pick in localStorage (see App.tsx). Labels stay
          in their own language (中文 always in Chinese, EN always in
          English) so each is readable exactly when it's the one you need. */}
      <div className="market-toggle lang-toggle" role="group" aria-label="language">
        <button
          className={`market-toggle-btn${lang === "zh" ? " active" : ""}`}
          onClick={() => onLangChange("zh")}
        >
          中文
        </button>
        <button
          className={`market-toggle-btn${lang === "en" ? " active" : ""}`}
          onClick={() => onLangChange("en")}
        >
          EN
        </button>
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
