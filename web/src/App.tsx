import { useEffect, useState } from "react";
import { fetchConfig } from "./api";
import { getDictionary, type Dictionary } from "./i18n";
import { NavBar } from "./components/NavBar";
import { DashboardView } from "./components/DashboardView";
import { CalendarView } from "./components/CalendarView";

// Two client-side routes (dashboard, calendar) don't justify pulling in a
// routing library — a hand-rolled path + popstate listener is simpler and
// matches this project's "no abstraction beyond what's needed" convention.
// server.go's spaHandler already falls back to index.html for any path
// that isn't a real file in the build, so /calendar loads directly too
// (a hard refresh on it doesn't 404).
function useRoute(): [string, (path: string) => void] {
  const [path, setPath] = useState(window.location.pathname);

  useEffect(() => {
    const onPopState = () => setPath(window.location.pathname);
    window.addEventListener("popstate", onPopState);
    return () => window.removeEventListener("popstate", onPopState);
  }, []);

  const navigate = (next: string) => {
    if (next !== window.location.pathname) {
      window.history.pushState(null, "", next);
    }
    setPath(next);
  };

  return [path, navigate];
}

export default function App() {
  const [dict, setDict] = useState<Dictionary>(getDictionary("zh"));
  const [path, navigate] = useRoute();

  useEffect(() => {
    // /api/config's failure isn't fatal — the page still works with the
    // zh default dictionary already in state, just possibly the wrong
    // language.
    fetchConfig()
      .then((cfg) => setDict(getDictionary(cfg.lang)))
      .catch(() => {});
  }, []);

  return (
    <div className="app">
      <NavBar path={path} onNavigate={navigate} dict={dict} />
      {path === "/calendar" ? <CalendarView dict={dict} /> : <DashboardView dict={dict} />}
    </div>
  );
}
