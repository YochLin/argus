package web

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
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
	// in that case.
	Trade TradeExecutor
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
	history          data.HistoryProvider
	earnings         data.EarningsProvider
	lang             i18n.Lang
	companyNames     data.CompanyNameProvider
	heatThresholdPct float64
	recPerf          *recPerfStore
	password         string
	trade            TradeExecutor
	csvDB            csvWriter
	mux              *http.ServeMux
}

func New(cfg Config) *Server {
	s := &Server{
		db:               cfg.DB,
		watchlistDB:      cfg.DB,
		buyAlertDB:       cfg.DB,
		quotes:           newQuoteCache(cfg.Provider),
		history:          cfg.History,
		earnings:         cfg.Earnings,
		lang:             cfg.Lang,
		companyNames:     cfg.CompanyNames,
		heatThresholdPct: cfg.RiskHeatPct,
		password:         cfg.Password,
		trade:            cfg.Trade,
		csvDB:            cfg.DB,
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
	// Write routes (Phase 10) are always registered, but requireWritable
	// 404s every one of them when no password is configured (see Config.
	// Password's doc comment) — registering unconditionally, rather than
	// skipping registration, is what makes that a real 404 instead of
	// falling through to spaHandler's SPA-fallback 200. See
	// docs/phase-10-web-trade-input.md §4.1.
	s.mux.HandleFunc("POST /api/login", s.requireWritable(s.handleLogin))
	s.mux.HandleFunc("POST /api/trade/buy", s.requireWritable(s.requireAuth(s.handleTradeBuy)))
	s.mux.HandleFunc("POST /api/trade/sell", s.requireWritable(s.requireAuth(s.handleTradeSell)))
	s.mux.HandleFunc("POST /api/stop", s.requireWritable(s.requireAuth(s.handleSetStop)))
	s.mux.HandleFunc("POST /api/watchlist/add", s.requireWritable(s.requireAuth(s.handleWatchlistAdd)))
	s.mux.HandleFunc("POST /api/watchlist/remove", s.requireWritable(s.requireAuth(s.handleWatchlistRemove)))
	s.mux.HandleFunc("POST /api/buy-alerts/add", s.requireWritable(s.requireAuth(s.handleBuyAlertAdd)))
	s.mux.HandleFunc("POST /api/buy-alerts/remove", s.requireWritable(s.requireAuth(s.handleBuyAlertRemove)))
	// /api/import (Phase 5 §B, optional CSV backfill) reuses the same
	// requireWritable/requireAuth gate as every other write route — a bulk
	// transaction write is no less a write than a single /buy.
	s.mux.HandleFunc("POST /api/import", s.requireWritable(s.requireAuth(s.handleImport)))
	s.mux.Handle("/", spaHandler())
	return s
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
		log.Fatalf("web: embedded dist: %v", err)
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

	errCh := make(chan error, 1)
	go func() {
		log.Printf("web: dashboard listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
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
