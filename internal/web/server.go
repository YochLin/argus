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
	Lang     i18n.Lang
	// CompanyNames resolves TW tickers' Chinese short names for display
	// (/api/company-names). nil when FINMIND_TOKEN isn't configured — the
	// endpoint then returns an empty map and the frontend shows bare
	// tickers, same optionality as bot.Config.CompanyNames.
	CompanyNames data.CompanyNameProvider
}

// Server is Argus's read-only web dashboard (Phase 5 PR1 — see
// docs/phase-5-web-dashboard.md). WEB_ADDR-gated; intended for VPS-private
// access only (Tailscale/SSH tunnel), so it deliberately has no auth/HTTPS
// of its own.
type Server struct {
	db           dbReader
	quotes       quoteGetter
	history      data.HistoryProvider
	lang         i18n.Lang
	companyNames data.CompanyNameProvider
	mux          *http.ServeMux
}

func New(cfg Config) *Server {
	s := &Server{
		db:           cfg.DB,
		quotes:       newQuoteCache(cfg.Provider),
		history:      cfg.History,
		lang:         cfg.Lang,
		companyNames: cfg.CompanyNames,
	}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("GET /api/config", s.handleConfig)
	s.mux.HandleFunc("GET /api/status", s.handleStatus)
	s.mux.HandleFunc("GET /api/dashboard", s.handleDashboard)
	s.mux.HandleFunc("GET /api/calendar", s.handleCalendar)
	s.mux.HandleFunc("GET /api/rounds", s.handleRounds)
	s.mux.HandleFunc("GET /api/round-detail", s.handleRoundDetail)
	s.mux.HandleFunc("GET /api/reports", s.handleReports)
	s.mux.HandleFunc("GET /api/chart", s.handleChart)
	s.mux.HandleFunc("GET /api/tickers", s.handleTickers)
	s.mux.HandleFunc("GET /api/company-names", s.handleCompanyNames)
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
