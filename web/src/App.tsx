import { useEffect, useState } from "react";
import { fetchConfig, fetchStatus, type Market, type Status } from "./api";
import { getDictionary, type Dictionary } from "./i18n";
import { Sidebar } from "./components/Sidebar";
import { StatusBar } from "./components/StatusBar";
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
  const [status, setStatus] = useState<Status | null>(null);
  const [route, navigate] = useRoute();
  // Phase 6's US/TW toggle (docs/phase-6-tw-market.md §4.4) — lifted here
  // (not per-view state) since it's shell-level chrome shared by every page,
  // same reasoning as Sidebar/StatusBar living above the routed body.
  const [market, setMarket] = useState<Market>("us");

  useEffect(() => {
    // /api/config's failure isn't fatal — the page still works with the zh
    // default dictionary.
    fetchConfig()
      .then((cfg) => setDict(getDictionary(cfg.lang)))
      .catch(() => {});
  }, []);

  useEffect(() => {
    // /api/status's failure isn't fatal either — an empty status-bar shell
    // is a fine degrade. Refetches whenever the market toggle changes, since
    // the watching-count/benchmark line is market-scoped (see
    // internal/web/dashboard.go's buildStatus).
    setStatus(null);
    fetchStatus(market)
      .then(setStatus)
      .catch(() => {});
  }, [market]);

  const [path, search] = route.split("?");
  const params = new URLSearchParams(search ?? "");

  let body;
  if (path === "/calendar") {
    body = <CalendarView dict={dict} market={market} />;
  } else if (path === "/rounds") {
    body = (
      <RoundsListView
        dict={dict}
        market={market}
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
    body = <DashboardView dict={dict} market={market} />;
  }

  return (
    <div className="app-shell">
      <Sidebar path={path} onNavigate={navigate} dict={dict} market={market} onMarketChange={setMarket} />
      <div className="app-main">
        {status ? (
          <StatusBar status={status} dict={dict} market={market} />
        ) : (
          <div className="status-bar" />
        )}
        <div className="content">{body}</div>
      </div>
    </div>
  );
}
