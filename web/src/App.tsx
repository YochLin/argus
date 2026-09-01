import { useEffect, useState } from "react";
import { fetchCompanyNames, fetchConfig, fetchStatus, marketOf, type Market, type Status } from "./api";
import { getDictionary, normalizeLang, type Lang } from "./i18n";
import { Sidebar } from "./components/Sidebar";
import { StatusBar } from "./components/StatusBar";
import { TopBar } from "./components/TopBar";
import { DashboardView } from "./components/DashboardView";
import { CalendarView } from "./components/CalendarView";
import { RoundsListView } from "./components/RoundsListView";
import { ChartListView } from "./components/ChartListView";
import { ChartView } from "./components/ChartView";
import { ReportsView } from "./components/ReportsView";
import { RiskView } from "./components/RiskView";
import { RecsView } from "./components/RecsView";
import { PaperView } from "./components/PaperView";
import { OptionsView } from "./components/OptionsView";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { TradeModal, type TradeMode } from "./components/TradeModal";
import { LoginModal } from "./components/LoginModal";
import { ImportView } from "./components/ImportView";
import { SettingsView } from "./components/SettingsView";
import { SectorFlowView } from "./components/SectorFlowView";
import { LlmRunsView } from "./components/LlmRunsView";

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
// Same pattern for the dark/light toggle (TopBar, Figma reference layout) —
// a per-browser display choice, no server default to fall back to. Absent
// means dark, matching every pre-existing screenshot/expectation.
const themeStorageKey = "argus-theme";
// Same pattern again for the sidebar account menu's dev-mode toggle
// (Sidebar.tsx's AccountMenu) — lifted up here rather than kept as
// Sidebar-local state because it now also gates the /llm route below, not
// just the nav link. devMode's purpose is hiding features that exist
// (server-enabled) but aren't meant for the everyday view — /llm is the
// first one; llmAuditEnabled (server) and devMode (client) are both
// required, so turning this on client-side can't reveal a feature the
// operator hasn't enabled server-side.
const devModeStorageKey = "argus-dev-mode";

export default function App() {
  // serverLang is /api/config's BOT_LANGUAGE default; userLang is the
  // toggle's override, persisted in localStorage. The override wins when
  // present — the server value only decides the first-visit default.
  const [serverLang, setServerLang] = useState<Lang>("zh");
  // paperEnabled (Phase 11 PR4) gates the sidebar's Paper Account link —
  // mirrors /api/config's paperEnabled (PAPER_DB_PATH configured
  // server-side). Defaults false so a slow/failed /api/config fetch never
  // briefly shows a link to a page that would 404.
  const [paperEnabled, setPaperEnabled] = useState(false);
  // llmAuditEnabled (Phase 19, WEB_LLM_AUDIT) gates the /llm nav link and
  // route the same way — hidden entirely when unset server-side.
  const [llmAuditEnabled, setLlmAuditEnabled] = useState(false);
  const [devMode, setDevMode] = useState<boolean>(() => localStorage.getItem(devModeStorageKey) === "1");
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
  // Remembers each market's last-viewed /chart route (ticker + round), so
  // toggling US/TW away from a ticker detail page and back restores it
  // instead of leaving a US ticker on screen while the toggle reads TW (or
  // vice versa). Keyed by the ticker's own market (marketOf), not the
  // toggle's position at save time, since those two only agree when we
  // actually navigate away below.
  const [chartRouteByMarket, setChartRouteByMarket] = useState<Record<Market, string>>({
    us: "/chart",
    tw: "/chart",
  });
  const [isDark, setIsDark] = useState<boolean>(() => localStorage.getItem(themeStorageKey) !== "light");
  // Phase 10 write-input state (docs/phase-10-web-trade-input.md §4.3): one
  // shared TradeModal instance for every entry point (PositionsTable's row
  // buttons, TopBar's global "+ Trade"), one shared LoginModal for any 401
  // any of them hits, and a refreshSignal bumped after a successful write
  // so the currently-visible view's own data fetch picks up the change —
  // simpler than threading a refetch callback through every view.
  const [tradeModal, setTradeModal] = useState<{
    mode: TradeMode;
    ticker: string;
    editableTicker?: boolean;
    prefillPrice?: number;
  } | null>(null);
  const [authRetry, setAuthRetry] = useState<(() => void) | null>(null);
  const [refreshSignal, setRefreshSignal] = useState(0);

  const openTrade = (mode: TradeMode, ticker: string, prefillPrice?: number) =>
    setTradeModal({ mode, ticker, prefillPrice });

  const lang = userLang ?? serverLang;
  const dict = getDictionary(lang);

  const changeLang = (next: Lang) => {
    localStorage.setItem(langStorageKey, next);
    setUserLang(next);
  };

  const toggleTheme = () => {
    const next = !isDark;
    localStorage.setItem(themeStorageKey, next ? "dark" : "light");
    setIsDark(next);
  };

  const toggleDevMode = () => {
    const next = !devMode;
    localStorage.setItem(devModeStorageKey, next ? "1" : "0");
    setDevMode(next);
  };

  // :root.light (theme.css) carries every light-mode token override — see
  // its own doc comment for what's ported from the Figma reference and what
  // known corner (lightweight-charts' fixed dark grid/axis colors) is cut.
  useEffect(() => {
    document.documentElement.classList.toggle("light", !isDark);
  }, [isDark]);

  useEffect(() => {
    // /api/config's failure isn't fatal — the page still works with the zh
    // default dictionary.
    fetchConfig()
      .then((cfg) => {
        setServerLang(normalizeLang(cfg.lang));
        setPaperEnabled(cfg.paperEnabled);
        setLlmAuditEnabled(cfg.llmAuditEnabled);
      })
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

  const handleMarketChange = (next: Market) => {
    if (next === market) return;
    const currentTicker = path === "/chart" ? params.get("ticker") : null;
    if (currentTicker) {
      // Save where we're leaving so switching back restores it.
      setChartRouteByMarket((byMarket) => ({ ...byMarket, [marketOf(currentTicker)]: route }));
      if (marketOf(currentTicker) !== next) {
        navigate(chartRouteByMarket[next]);
      }
    }
    setMarket(next);
  };

  let body;
  if (path === "/calendar") {
    body = (
      <CalendarView
        dict={dict}
        market={market}
        names={names}
        writable={status?.writable ?? false}
        onUnauthorized={(retry) => setAuthRetry(() => retry)}
      />
    );
  } else if (path === "/rounds") {
    body = (
      <RoundsListView
        dict={dict}
        market={market}
        names={names}
        onOpenRound={(ticker, start) =>
          navigate(`/chart?ticker=${encodeURIComponent(ticker)}&round=${encodeURIComponent(start)}`)
        }
      />
    );
  } else if (path === "/reports") {
    body = <ReportsView dict={dict} market={market} names={names} />;
  } else if (path === "/recs") {
    body = <RecsView dict={dict} market={market} names={names} />;
  } else if (path === "/paper") {
    body = paperEnabled ? (
      <PaperView
        dict={dict}
        market={market}
        names={names}
        onTickerClick={(t) => navigate(`/chart?ticker=${encodeURIComponent(t)}`)}
      />
    ) : null;
  } else if (path === "/options") {
    body = (
      <OptionsView
        dict={dict}
        market={market}
        writable={status?.writable ?? false}
        onUnauthorized={(retry) => setAuthRetry(() => retry)}
      />
    );
  } else if (path === "/llm") {
    body = llmAuditEnabled && devMode ? (
      <LlmRunsView
        dict={dict}
        names={names}
        writable={status?.writable ?? false}
        onUnauthorized={(retry) => setAuthRetry(() => retry)}
        runId={params.get("id") ? Number(params.get("id")) : null}
        onOpenRun={(id) => navigate(`/llm?id=${id}`)}
      />
    ) : null;
  } else if (path === "/settings") {
    // Same writable gate as /import: /api/settings 404s without WEB_PASSWORD
    // (requireWritable), so the page would have nothing to show anyway.
    body = status?.writable ? (
      <SettingsView dict={dict} onUnauthorized={(retry) => setAuthRetry(() => retry)} />
    ) : null;
  } else if (path === "/import") {
    body = status?.writable ? (
      <ImportView
        dict={dict}
        onUnauthorized={(retry) => setAuthRetry(() => retry)}
        onSuccess={() => setRefreshSignal((n) => n + 1)}
      />
    ) : null;
  } else if (path === "/flow") {
    body = (
      <SectorFlowView
        dict={dict}
        lang={lang}
        market={market}
        names={names}
        onTickerClick={(t) => navigate(`/chart?ticker=${encodeURIComponent(t)}`)}
      />
    );
  } else if (path === "/risk") {
    body = (
      <RiskView
        dict={dict}
        market={market}
        names={names}
        onTickerClick={(t) => navigate(`/chart?ticker=${encodeURIComponent(t)}`)}
      />
    );
  } else if (path === "/chart") {
    const ticker = params.get("ticker");
    const roundStart = params.get("round") ?? undefined;
    body = ticker ? (
      <ChartView
        dict={dict}
        ticker={ticker}
        initialRoundStart={roundStart}
        names={names}
        writable={status?.writable ?? false}
        onTrade={status?.writable ? openTrade : undefined}
        onUnauthorized={(retry) => setAuthRetry(() => retry)}
        onBack={() => navigate("/chart")}
      />
    ) : (
      <ChartListView
        dict={dict}
        market={market}
        names={names}
        onOpenTicker={(t) => navigate(`/chart?ticker=${encodeURIComponent(t)}`)}
        writable={status?.writable ?? false}
        onUnauthorized={(retry) => setAuthRetry(() => retry)}
        onSuccess={() => setRefreshSignal((n) => n + 1)}
      />
    );
  } else {
    body = (
      <DashboardView
        dict={dict}
        market={market}
        names={names}
        onTickerClick={(t) => navigate(`/chart?ticker=${encodeURIComponent(t)}`)}
        onTrade={status?.writable ? openTrade : undefined}
        refreshSignal={refreshSignal}
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
        status={status}
        writable={status?.writable ?? false}
        paperEnabled={paperEnabled}
        llmAuditEnabled={llmAuditEnabled}
        devMode={devMode}
        onToggleDevMode={toggleDevMode}
      />
      <div className="app-main">
        <TopBar
          market={market}
          onMarketChange={handleMarketChange}
          isDark={isDark}
          onToggleTheme={toggleTheme}
          lang={lang}
          onLangChange={changeLang}
          dict={dict}
          writable={status?.writable ?? false}
          onAddTrade={() => setTradeModal({ mode: "buy", ticker: "", editableTicker: true })}
        />
        {status ? (
          <StatusBar status={status} dict={dict} market={market} />
        ) : (
          <div className="status-bar" />
        )}
        <div className="content">
          <ErrorBoundary key={`${path}:${market}`} message={dict.error}>
            {body}
          </ErrorBoundary>
        </div>
      </div>
      {tradeModal && (
        <TradeModal
          dict={dict}
          mode={tradeModal.mode}
          ticker={tradeModal.ticker}
          editableTicker={tradeModal.editableTicker}
          prefillPrice={tradeModal.prefillPrice}
          onClose={() => setTradeModal(null)}
          onSuccess={() => setRefreshSignal((n) => n + 1)}
          onUnauthorized={(retry) => setAuthRetry(() => retry)}
        />
      )}
      {authRetry && (
        <LoginModal
          dict={dict}
          onClose={() => setAuthRetry(null)}
          onSuccess={() => {
            const retry = authRetry;
            setAuthRetry(null);
            retry();
          }}
        />
      )}
    </div>
  );
}
