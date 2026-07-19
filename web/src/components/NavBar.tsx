import type { MouseEvent } from "react";
import type { Dictionary } from "../i18n";

interface Props {
  path: string;
  onNavigate: (path: string) => void;
  dict: Dictionary;
}

// /round (the detail page reached by clicking a row in /rounds) has no nav
// link of its own — same reasoning as a chart drill-down page not
// appearing in a top nav — see App.tsx's comment on why this is a
// hand-rolled path/popstate router rather than a routing library for an
// app this size.
const links: Array<{ path: string; label: (dict: Dictionary) => string }> = [
  { path: "/", label: (d) => d.navDashboard },
  { path: "/calendar", label: (d) => d.navCalendar },
  { path: "/rounds", label: (d) => d.navRounds },
];

export function NavBar({ path, onNavigate, dict }: Props) {
  return (
    <nav className="nav-bar">
      {links.map((link) => (
        <a
          key={link.path}
          href={link.path}
          className={`nav-link${path === link.path ? " active" : ""}`}
          onClick={(e: MouseEvent) => {
            e.preventDefault();
            onNavigate(link.path);
          }}
        >
          {link.label(dict)}
        </a>
      ))}
    </nav>
  );
}
