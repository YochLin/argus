import type { MouseEvent } from "react";
import type { Dictionary } from "../i18n";

interface Props {
  path: string;
  onNavigate: (path: string) => void;
  dict: Dictionary;
}

// Two routes today (dashboard, calendar); a third (round-detail, PR3) will
// slot in here without touching the router itself — see App.tsx's comment
// on why this is a hand-rolled path/popstate router rather than a routing
// library for an app this size.
const links: Array<{ path: string; label: (dict: Dictionary) => string }> = [
  { path: "/", label: (d) => d.navDashboard },
  { path: "/calendar", label: (d) => d.navCalendar },
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
