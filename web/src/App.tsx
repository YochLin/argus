import { useEffect, useState } from "react";
import { fetchConfig } from "./api";
import { getDictionary, type Dictionary } from "./i18n";
import { NavBar } from "./components/NavBar";
import { DashboardView } from "./components/DashboardView";
import { CalendarView } from "./components/CalendarView";
import { RoundsListView } from "./components/RoundsListView";
import { RoundDetailView } from "./components/RoundDetailView";

// Four client-side routes (dashboard, calendar, round list, round detail)
// don't justify pulling in a routing library — a hand-rolled route
// (pathname + search) + popstate listener is simpler and matches this
// project's "no abstraction beyond what's needed" convention. server.go's
// spaHandler already falls back to index.html for any path that isn't a
// real file in the build, so /calendar, /rounds, /round all load directly
// too (a hard refresh on any of them doesn't 404).
function useRoute(): [string, (route: string) => void] {
  const [route, setRoute] = useState(window.location.pathname + window.location.search);

  useEffect(() => {
    const onPopState = () => setRoute(window.location.pathname + window.location.search);
    window.addEventListener("popstate", onPopState);
    return () => window.removeEventListener("popstate", onPopState);
  }, []);

  const navigate = (next: string) => {
    if (next !== window.location.pathname + window.location.search) {
      window.history.pushState(null, "", next);
    }
    setRoute(next);
  };

  return [route, navigate];
}

export default function App() {
  const [dict, setDict] = useState<Dictionary>(getDictionary("zh"));
  const [route, navigate] = useRoute();

  useEffect(() => {
    // /api/config's failure isn't fatal — the page still works with the
    // zh default dictionary already in state, just possibly the wrong
    // language.
    fetchConfig()
      .then((cfg) => setDict(getDictionary(cfg.lang)))
      .catch(() => {});
  }, []);

  const [path, search] = route.split("?");
  const params = new URLSearchParams(search ?? "");

  let body;
  if (path === "/calendar") {
    body = <CalendarView dict={dict} />;
  } else if (path === "/rounds") {
    body = (
      <RoundsListView
        dict={dict}
        onOpenRound={(ticker, start) =>
          navigate(`/round?ticker=${encodeURIComponent(ticker)}&start=${encodeURIComponent(start)}`)
        }
      />
    );
  } else if (path === "/round") {
    body = (
      <RoundDetailView
        dict={dict}
        ticker={params.get("ticker") ?? ""}
        start={params.get("start") ?? ""}
        onBack={() => navigate("/rounds")}
      />
    );
  } else {
    body = <DashboardView dict={dict} />;
  }

  return (
    <div className="app">
      <NavBar path={path} onNavigate={navigate} dict={dict} />
      {body}
    </div>
  );
}
