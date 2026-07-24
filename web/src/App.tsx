import { useEffect, useState } from "react";
import { fetchCompanyNames, fetchConfig, fetchStatus, type Market, type Status } from "./api";
import { getDictionary, normalizeLang, type Lang } from "./i18n";
import { Sidebar } from "./components/Sidebar";
import { StatusBar } from "./components/StatusBar";
import { DashboardView } from "./components/DashboardView";
import { CalendarView } from "./components/CalendarView";
import { RoundsListView } from "./components/RoundsListView";
import { RoundDetailView } from "./components/RoundDetailView";
import { ChartListView } from "./components/ChartListView";
import { ChartView } from "./components/ChartView";
import { ReportsView } from "./components/ReportsView";

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

// localStorage key for the user's language override. Absent means "follow
// the server's BOT_LANGUAGE default"; the toggle writes it and it wins over
// /api/config from then on. Language preference is a per-browser display
// choice, so localStorage (not a server setting) is the right home for it.
const langStorageKey = "argus-lang";

export default function App() {
  // serverLang is /api/config's BOT_LANGUAGE default; userLang is the
  // toggle's override, persisted in localStorage. The override wins when
  // present — the server value only decides the first-visit default.
  const [serverLang, setServerLang] = useState<Lang>("zh");
  const [userLang, setUserLang] = useState<Lang | null>(() => {
    const stored = localStorage.getItem(langStorageKey);
    return stored === null ? null : normalizeLang(stored);
  });
  const [status, setStatus] = useState<Status | null>(null);
  // names is /api/company-names' TW ticker -> Chinese short name map — see
  // internal/web/companynames.go. Fetched once at the shell level (not
  // per-market, since the endpoint itself isn't market-scoped: it covers
  // every TW ticker the user has any relationship with regardless of
  // toggle position) and threaded down to every view that renders a
  // ticker. Defaults to {} so every view can index it unconditionally
  // before this resolves, same degrade as a failed/absent FINMIND_TOKEN.
  const [names, setNames] = useState<Record<string, string>>({});
  const [route, navigate] = useRoute();
  // Phase 6's US/TW toggle (docs/phase-6-tw-market.md §4.4) — lifted here
  // (not per-view state) since it's shell-level chrome shared by every page,
  // same reasoning as Sidebar/StatusBar living above the routed body.
  const [market, setMarket] = useState<Market>("us");

  const lang = userLang ?? serverLang;
  const dict = getDictionary(lang);

  const changeLang = (next: Lang) => {
    localStorage.setItem(langStorageKey, next);
    setUserLang(next);
  };

  useEffect(() => {
    // /api/config's failure isn't fatal — the page still works with the zh
    // default dictionary.
    fetchConfig()
      .then((cfg) => setServerLang(normalizeLang(cfg.lang)))
      .catch(() => {});
  }, []);

  useEffect(() => {
    // /api/company-names' failure isn't fatal either — every view already
    // defaults `names` to {} and falls back to a bare ticker.
    fetchCompanyNames()
      .then((r) => setNames(r.names))
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
    body = <CalendarView dict={dict} market={market} names={names} />;
  } else if (path === "/rounds") {
    body = (
      <RoundsListView
        dict={dict}
        market={market}
        names={names}
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
        names={names}
        onBack={() => navigate("/rounds")}
      />
    );
  } else if (path === "/reports") {
    body = <ReportsView dict={dict} market={market} names={names} />;
  } else if (path === "/chart") {
    const ticker = params.get("ticker");
    body = ticker ? (
      <ChartView dict={dict} ticker={ticker} names={names} onBack={() => navigate("/chart")} />
    ) : (
      <ChartListView
        dict={dict}
        market={market}
        names={names}
        onOpenTicker={(t) => navigate(`/chart?ticker=${encodeURIComponent(t)}`)}
      />
    );
  } else {
    body = (
      <DashboardView
        dict={dict}
        market={market}
        names={names}
        onTickerClick={(t) => navigate(`/chart?ticker=${encodeURIComponent(t)}`)}
      />
    );
  }

  return (
    <div className="app-shell">
      <Sidebar
        path={path}
        onNavigate={navigate}
        dict={dict}
        market={market}
        onMarketChange={setMarket}
        lang={lang}
        onLangChange={changeLang}
      />
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
