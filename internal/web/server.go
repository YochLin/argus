package web

import (
	"cmp"
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/logger"
	"argus/internal/notification"
	"argus/internal/service"
)

// dist holds the built frontend (web/'s Vite project, configured to build
// straight into this directory — see web/vite.config.ts — since go:embed
// patterns can't reach outside the package directory with a "..", the SPA
// source has to live at the repo root but its build output has to land
// here). A placeholder index.html is committed so a fresh clone's
// `go build ./...`/`go test ./...` never depends on Node having run first;
// CI and deploy.yml both run `npm run build` before the Go build step,
// which overwrites this directory with the real SPA.
//
//go:embed all:dist
var dist embed.FS

// Config wires the dashboard's dependencies — the same *db.DB and
// data.Provider chain the rest of the bot process already uses (this is
// an in-process HTTP server, not a subprocess like internal/mcptools, so
// there's no need for a second db.OpenReadOnly connection: database/sql
// connections are already safe for concurrent use from other goroutines).
type Config struct {
	DB       *db.DB
	Provider data.Provider
	History  data.HistoryProvider
	// Earnings backs the Calendar page's earnings-event indicators
	// (/api/calendar). nil when FINNHUB_API_KEY isn't set — Calendar then
	// simply shows no earnings dots, same optionality as CompanyNames.
	Earnings data.EarningsProvider
	Lang     i18n.Lang
	// CompanyNames resolves TW tickers' Chinese short names for display
	// (/api/company-names). nil when FINMIND_TOKEN isn't configured — the
	// endpoint then returns an empty map and the frontend shows bare
	// tickers, same optionality as bot.Config.CompanyNames.
	CompanyNames data.CompanyNameProvider
	// OptionChain backs Phase 12 PR4's /api/options — the same
	// data.OptionChainProvider (Yahoo) internal/bot's /option and /portfolio
	// options section use. nil disables live mark/greeks on that page (the
	// ledger data itself still renders); no other page reads this field.
	OptionChain data.OptionChainProvider
	// RiskHeatPct is the /risk page's portfolio-heat warning threshold (env
	// RISK_HEAT_PCT, Phase 8 PR1) — <=0 means "don't draw a warning line,"
	// same convention as internal/bot's STOP_LOSS_PCT. main.go defaults it
	// to 6.0 when the env var is unset.
	RiskHeatPct float64
	// Password (env WEB_PASSWORD, Phase 10) gates the write endpoints —
	// empty means writes are off entirely (login + all five write routes
	// simply aren't registered, so they 404), same presence-of-config
	// convention as WEB_ADDR itself. See docs/phase-10-web-trade-input.md
	// §4.1.
	Password string
	// Trade is the write-endpoint seam onto *bot.Bot (buy/sell/stop) — nil
	// whenever Password is empty, since New only wires the write routes up
	// in that case. It is no longer nil when Telegram is unconfigured
	// (Phase 24 Stage 3's headless bot is a working TradeExecutor); nil now
	// only means no seam was injected at all, which requireTrade answers
	// with a 409.
	Trade TradeExecutor
	// Recommend backs POST /api/v1/recommendations/trigger; nil (the state
	// every test builds) makes that one route 409 and changes nothing else.
	Recommend Recommender
	// EnvPath is the .env file /api/settings edits (Phase 17 PR2). main.go
	// always passes ".env": godotenv.Load() reads exactly that path relative
	// to the working directory, so anything else here would edit a file the
	// process doesn't read. It's a field only so settings_test.go can point
	// at a temp file. Empty defaults to ".env".
	EnvPath string
	// PaperDB is Phase 11 PR3's live paper account's own database — nil
	// (PAPER_DB_PATH unset) disables /api/paper entirely (404), same
	// presence-of-config convention as Password/Trade above. Physically
	// separate from DB, so nothing on this read-only page can ever touch
	// real trading data. PaperInitialCashUSD/TWD mirror the same env vars
	// internal/bot/paper.go seeds the account with (PAPER_INITIAL_CASH_USD/
	// TWD) — needed here only to render the KPI denominator for "return
	// since inception."
	PaperDB             *db.DB
	PaperInitialCashUSD float64
	PaperInitialCashTWD float64
	// Sector backs Phase 18's /api/sectorflow US classification (Finnhub's
	// finnhubIndustry, see internal/data/finnhub.go's GetSector). nil when
	// FINNHUB_API_KEY isn't set — RunSectorFlowScan then skips the US scan
	// entirely, same presence-of-config convention as Earnings/CompanyNames.
	Sector data.SectorProvider
	// IndustryMap backs /api/sectorflow's TW classification (FinMind's
	// whole-market GetIndustryMap) — nil when FINMIND_TOKEN isn't set.
	IndustryMap data.IndustryMapProvider
	// JWTSecret (env JWT_SECRET, Phase 24 Stage 4) signs the /api/v1 access
	// and refresh tokens. Empty disables the whole /api/v1 surface — see
	// registerAPIV1 — which is the right default: an API with no signing key
	// has no safe degraded mode. Rotating it invalidates every issued token,
	// which is also how you log every client out.
	JWTSecret string
	// APIKey (env API_KEY) is the alternative credential for personal
	// scripts and cron jobs, sent as X-API-Key. Empty means only JWT is
	// accepted; it is never matched against an empty header.
	APIKey string
	// Events is Phase 24 Stage 4 Step 4.3's live-alert fan-out, registered on
	// the process's notification.Dispatcher by internal/app. nil leaves
	// /api/v1/ws unregistered — the dashboard and the REST surface don't
	// depend on it.
	Events *notification.WebSocketHub
	// LLMAudit gates Phase 19's /llm page (env WEB_LLM_AUDIT, §8.4) — unlike
	// Paper/OptionChain/etc.'s "always register, 404/empty at request time"
	// convention, this one skips route registration entirely when off, so
	// /api/llm-runs and /api/news-sources/* fall through to spaHandler's
	// catch-all like any other unregistered path. Argus is single-user, so
	// there's no "someone else might see this" reason to gate at request
	// time instead — a deploy-time env flag matches every other optional
	// feature's presence-of-config convention (WEB_ADDR, Password, etc.).
	LLMAudit bool
}

// Server is Argus's read-only web dashboard (Phase 5 PR1 — see
// docs/phase-5-web-dashboard.md). WEB_ADDR-gated; intended for VPS-private
// access only (Tailscale/SSH tunnel), so it deliberately has no auth/HTTPS
// of its own.
type Server struct {
	db               dbReader
	watchlistDB      watchlistWriter
	buyAlertDB       buyAlertWriter
	quotes           quoteGetter
	portfolio        *service.PortfolioService
	history          data.HistoryProvider
	earnings         data.EarningsProvider
	lang             i18n.Lang
	companyNames     data.CompanyNameProvider
	optionChain      data.OptionChainProvider
	heatThresholdPct float64
	recPerf          *recPerfStore
	password         string
	trade            TradeExecutor
	recommender      Recommender
	// recRunning is the one-at-a-time gate on
	// handleAPIRecommendationsTrigger's background run.
	recRunning atomic.Bool
	envPath    string
	csvDB      csvWriter
	thesisDB   thesisWriter
	// paperDB stays *db.DB (not dbReader) so nil-checking it in
	// handlePaper can't fall into the classic "non-nil interface wrapping
	// a nil pointer" trap — it's passed into buildPaper's dbReader
	// parameter at each call site instead.
	paperDB             *db.DB
	paperInitialCashUSD float64
	paperInitialCashTWD float64
	sector              data.SectorProvider
	industryMap         data.IndustryMapProvider
	sectorFlow          *sectorFlowCache
	newsSourceDB        newsSourceWriter
	llmAudit            bool
	jwtSecret           string
	apiKey              string
	// apiDB is a narrow interface (not folded into dbReader: these three
	// methods are only ever reached from /api/v1, and dbReader is already
	// implemented by a pile of hand-built test values that have no reason
	// to grow them) — assigned in New only when cfg.DB != nil, same
	// nil-typed-pointer-in-an-interface trap paperDB's comment above warns
	// about, just solved the other way since this field's callers need an
	// interface, not a concrete *db.DB.
	apiDB  apiV1Store
	events wsHub
	mux    *http.ServeMux
}

func New(cfg Config) *Server {
	quotes := newQuoteCache(cfg.Provider)
	s := &Server{
		db:                  cfg.DB,
		watchlistDB:         service.NewWatchlistService(cfg.DB),
		buyAlertDB:          cfg.DB,
		quotes:              quotes,
		portfolio:           service.NewPortfolioService(cfg.DB, quotes),
		history:             cfg.History,
		earnings:            cfg.Earnings,
		lang:                cfg.Lang,
		companyNames:        cfg.CompanyNames,
		optionChain:         cfg.OptionChain,
		heatThresholdPct:    cfg.RiskHeatPct,
		password:            cfg.Password,
		trade:               cfg.Trade,
		recommender:         cfg.Recommend,
		envPath:             cmp.Or(cfg.EnvPath, ".env"),
		csvDB:               cfg.DB,
		thesisDB:            cfg.DB,
		paperDB:             cfg.PaperDB,
		paperInitialCashUSD: cfg.PaperInitialCashUSD,
		paperInitialCashTWD: cfg.PaperInitialCashTWD,
		sector:              cfg.Sector,
		industryMap:         cfg.IndustryMap,
		sectorFlow:          newSectorFlowCache(),
		newsSourceDB:        cfg.DB,
		llmAudit:            cfg.LLMAudit,
		jwtSecret:           cfg.JWTSecret,
		apiKey:              cfg.APIKey,
	}
	if cfg.DB != nil {
		s.apiDB = cfg.DB
	}
	s.recPerf = newRecPerfStore(s.db, s.history)
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("GET /api/config", s.handleConfig)
	s.mux.HandleFunc("GET /api/status", s.handleStatus)
	s.mux.HandleFunc("GET /api/dashboard", s.handleDashboard)
	s.mux.HandleFunc("GET /api/calendar", s.handleCalendar)
	s.mux.HandleFunc("GET /api/rounds", s.handleRounds)
	s.mux.HandleFunc("GET /api/round-detail", s.handleRoundDetail)
	s.mux.HandleFunc("GET /api/reports", s.handleReports)
	s.mux.HandleFunc("GET /api/monthly", s.handleMonthly)
	s.mux.HandleFunc("GET /api/distributions", s.handleDistributions)
	s.mux.HandleFunc("GET /api/chart", s.handleChart)
	s.mux.HandleFunc("GET /api/tickers", s.handleTickers)
	s.mux.HandleFunc("GET /api/watchlist-summary", s.handleWatchlistSummary)
	s.mux.HandleFunc("GET /api/company-names", s.handleCompanyNames)
	s.mux.HandleFunc("GET /api/risk", s.handleRisk)
	s.mux.HandleFunc("GET /api/rec-performance", s.handleRecPerformance)
	s.mux.HandleFunc("GET /api/paper", s.handlePaper)
	s.mux.HandleFunc("GET /api/options", s.handleOptions)
	s.mux.HandleFunc("GET /api/events", s.handleEvents)
	s.mux.HandleFunc("GET /api/sectorflow", s.handleSectorFlow)
	s.mux.HandleFunc("POST /api/sectorflow/refresh", s.handleSectorFlowRefresh)
	// Phase 19's /llm page (§4.4/§5.3) — see Config.LLMAudit's doc comment
	// for why this is the one feature gated by skipping registration
	// entirely rather than 404ing/degrading at request time.
	if cfg.LLMAudit {
		s.mux.HandleFunc("GET /api/llm-runs", s.handleLLMRuns)
		s.mux.HandleFunc("GET /api/llm-runs/{id}", s.handleLLMRunDetail)
		s.mux.HandleFunc("GET /api/news-sources/blocked", s.handleBlockedNewsSources)
		s.mux.HandleFunc("POST /api/news-sources/block", s.requireWritable(s.requireAuth(s.handleNewsSourceBlock)))
		s.mux.HandleFunc("POST /api/news-sources/unblock", s.requireWritable(s.requireAuth(s.handleNewsSourceUnblock)))
	}
	// Write routes (Phase 10) are always registered, but requireWritable
	// 404s every one of them when no password is configured (see Config.
	// Password's doc comment) — registering unconditionally, rather than
	// skipping registration, is what makes that a real 404 instead of
	// falling through to spaHandler's SPA-fallback 200. See
	// docs/phase-10-web-trade-input.md §4.1.
	s.mux.HandleFunc("POST /api/login", s.requireWritable(s.handleLogin))
	// The TradeExecutor-backed routes carry requireTrade on top of the
	// shared gate — Trade is nil whenever Telegram isn't configured (Phase 17
	// PR1), which is now a reachable state for an otherwise-working
	// dashboard.
	s.mux.HandleFunc("POST /api/trade/buy", s.requireWritable(s.requireAuth(s.requireTrade(s.handleTradeBuy))))
	s.mux.HandleFunc("POST /api/trade/sell", s.requireWritable(s.requireAuth(s.requireTrade(s.handleTradeSell))))
	s.mux.HandleFunc("POST /api/trade/delete", s.requireWritable(s.requireAuth(s.requireTrade(s.handleDeleteTransaction))))
	s.mux.HandleFunc("POST /api/stop", s.requireWritable(s.requireAuth(s.requireTrade(s.handleSetStop))))
	s.mux.HandleFunc("POST /api/watchlist/add", s.requireWritable(s.requireAuth(s.handleWatchlistAdd)))
	s.mux.HandleFunc("POST /api/watchlist/remove", s.requireWritable(s.requireAuth(s.handleWatchlistRemove)))
	s.mux.HandleFunc("POST /api/buy-alerts/add", s.requireWritable(s.requireAuth(s.requireTrade(s.handleBuyAlertAdd))))
	s.mux.HandleFunc("POST /api/buy-alerts/remove", s.requireWritable(s.requireAuth(s.handleBuyAlertRemove)))
	// /api/thesis (Phase 21) is a DB write like any other above — no reason
	// to give it a looser gate than /api/trade/*.
	s.mux.HandleFunc("POST /api/thesis", s.requireWritable(s.requireAuth(s.handleThesisSet)))
	// /api/settings (Phase 17 PR2) sits behind the same gate as every write
	// route, GET included: the read side reports which credentials are
	// configured, which is not something to hand out unauthenticated.
	s.mux.HandleFunc("GET /api/settings", s.requireWritable(s.requireAuth(s.handleSettingsGet)))
	s.mux.HandleFunc("POST /api/settings", s.requireWritable(s.requireAuth(s.handleSettingsSave)))
	// /api/import (Phase 5 §B, optional CSV backfill) reuses the same
	// requireWritable/requireAuth gate as every other write route — a bulk
	// transaction write is no less a write than a single /buy.
	s.mux.HandleFunc("POST /api/import", s.requireWritable(s.requireAuth(s.handleImport)))
	if cfg.Events != nil {
		s.events = cfg.Events
	}
	s.registerAPIV1()
	s.mux.Handle("/", spaHandler())
	return s
}

// portfolios lazily preserves compatibility with small hand-built Server
// values in tests while production construction injects the shared service.
func (s *Server) portfolios() *service.PortfolioService {
	if s.portfolio == nil && s.db != nil {
		s.portfolio = service.NewPortfolioService(s.db, s.quotes)
	}
	return s.portfolio
}

// spaHandler serves the embedded SPA build, falling back to index.html for
// any path that isn't a real file in dist — client-side routing (React
// Router et al.) needs the server to hand back the app shell for routes
// like /calendar that only exist in the browser, not on disk.
func spaHandler() http.Handler {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		// Only possible if the embed directive itself is wrong (a build-time
		// concern, not a runtime one) — the placeholder/real dist directory
		// always exists by construction.
		logger.Fatalf("web: embedded dist: %v", err)
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(sub, path); err != nil {
			// Not a real file in the build (e.g. a client-side route like
			// /positions) — hand back the SPA shell instead of a 404 so
			// the frontend router can take over.
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

// Run starts the HTTP server on addr and blocks until ctx is cancelled,
// then shuts down gracefully — the same "runs until ctx cancellation"
// shape as scheduler.Scheduler and bot.Bot.Run, so main.go can start it as
// just another goroutine alongside those.
func (s *Server) Run(ctx context.Context, addr string) error {
	srv := &http.Server{Addr: addr, Handler: s.mux}

	// net.Listen up front (not srv.ListenAndServe, which binds inside the
	// goroutine below) so the "listening" log line only fires once the bind
	// has actually succeeded — otherwise a bind failure logs "listening"
	// followed immediately by the error, which reads backwards.
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	logger.Infof("web: dashboard listening on %s", addr)

	errCh := make(chan error, 1)
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("web: shutdown: %w", err)
		}
		return nil
	case err := <-errCh:
		return err
	}
}
