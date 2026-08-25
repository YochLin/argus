// Command server is Argus's primary daemon (Phase 24 Stage 3 Step 3.1, see
// docs/architecture/server-refactor-plan.md §Stage 3): the HTTP/API server
// and cron engine are the process, and the Telegram bot is one optional
// adapter attached to it rather than the thing that owns main().
//
// It shares all of its wiring with cmd/bot via internal/app.Boot; the only
// differences are the two below — HTTP is always on, and nothing runs in the
// foreground on Telegram's behalf. cmd/bot stays as-is for deployments that
// still point at it (deploy/argus.service, docker-compose.yml, and every
// `go run ./cmd/bot` in the docs).
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"argus/internal/app"
	"argus/internal/logger"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cfg := app.Load()
	// The one config difference from cmd/bot: a server always listens. An
	// unset WEB_ADDR means "off" there (Config.WebAddr's presence-of-config
	// convention); here it means "you didn't pick a port", so we pick one.
	// Deliberately not a new env var — WEB_ADDR already names this, and a
	// second one would just be a way for the two to disagree.
	if cfg.WebAddr == "" {
		cfg.WebAddr = app.DefaultWebAddr
	}

	a, err := app.Boot(ctx, cfg)
	if err != nil {
		logger.Fatalf("boot: %v", err)
	}
	defer a.Close()

	// Phase 2.6 追加項's S&P 500 refresh — same once-per-process-start call
	// cmd/bot makes; it's a DB refresh, not a Telegram concern, but it lives
	// on *bot.Bot until Stage 1's remaining service extractions move it.
	if a.Bot != nil {
		a.Bot.SyncUniverse()
	}

	logger.Info("argus server started")
	a.Run(ctx)
	logger.Info("argus server stopped")
}
