// Command server is Argus's daemon (Phase 24 Stage 3, see
// docs/architecture/server-refactor-plan.md §Stage 3): the HTTP/API server
// and cron engine are the process, and the Telegram bot is one optional
// adapter attached to it rather than the thing that owns main().
//
// This is the only entrypoint. It used to share internal/app.Boot with a
// second one, cmd/bot, kept while the inversion was in progress; once
// internal/app stopped gating anything but the Telegram long-poll loop on a
// token, the two differed only in what an empty WEB_ADDR meant, and a second
// deployable binary that behaves the same is just a second thing to keep in
// sync. Deploy units, the Dockerfile and the docs all build this one.
//
// The mcp/eval/backtest subcommands below branch before any of the daemon
// setup. "mcp" in particular has to live in *this* binary and no other:
// llm.argusMCPServer spawns a chat session's MCP server as
// `os.Executable() mcp`, so whatever gets deployed must answer to it — a
// deployed binary that ignored the argument would boot a second copy of the
// whole daemon against the live database instead.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"

	"argus/internal/app"
	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/logger"
	"argus/internal/mcptools"
)

func main() {
	// Subcommands run this same binary as something other than the daemon,
	// and none of them needs the .env/Telegram/HTTP setup below:
	//   mcp      — an MCP server over stdio (internal/mcptools), spawned by
	//              an ACP chat session via os.Executable(), never by a human.
	//   eval     — the offline recommendation-history scorer (eval.go, see
	//              docs/offline-rec-eval.md), a manual one-shot terminal tool.
	//   backtest — replays the recommendations table through internal/paper,
	//              the live paper account's forward rule engine (backtest.go,
	//              docs/phase-11-paper-account.md §5).
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "mcp":
			runMCPServer()
			return
		case "eval":
			runEval()
			return
		case "backtest":
			runBacktest()
			return
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cfg := app.Load()
	// A server always listens. WEB_ADDR's presence-of-config convention
	// elsewhere means "unset = feature off", but a server with no HTTP
	// listener is not a server, so here an unset value means "you didn't
	// pick a port" and we pick one. Deliberately not a new env var —
	// WEB_ADDR already names this, and a second one would just be a way for
	// the two to disagree.
	if cfg.WebAddr == "" {
		cfg.WebAddr = app.DefaultWebAddr
	}

	a, err := app.Boot(ctx, cfg)
	if err != nil {
		logger.Fatalf("boot: %v", err)
	}
	defer a.Close()

	// Phase 2.6 追加項's S&P 500 refresh (see docs/phase-2.6-universe-refresh.md):
	// runs once per process start, right after seedSP500's fresh-install
	// path (inside bot.New -> db.New) has already had its chance to run.
	// It's a DB refresh, not a Telegram concern, but it lives on *bot.Bot
	// until Stage 1's remaining service extractions move it. No nil guard
	// since Boot's headless fallback: it runs on a Telegram-less server
	// too, which is the whole point — the universe table going stale was
	// never a Telegram problem.
	a.Bot.SyncUniverse()

	logger.Info("argus server started")
	a.Run(ctx)
	logger.Info("argus server stopped")
}

// runMCPServer runs argus as an MCP server over stdio (see internal/mcptools)
// until ctx is cancelled or the connection closes. Invoked via the "mcp"
// subcommand, never directly by a human — but it still needs its own
// godotenv.Load()/FINNHUB_API_KEY/BOT_LANGUAGE read, since main() skips all
// of that for this subcommand (see the branch in main()). When launched as a
// chat session's MCP server (llm.argusMCPServer), godotenv.Load() here finds
// no .env at its cwd (os.TempDir(), not this repo) and silently no-ops, but
// FINNHUB_API_KEY/BOT_LANGUAGE still resolve correctly: argusMCPServer passes
// them through explicitly via the MCP server's env config. log output here
// goes to log's default stderr, not stdout — stdout is reserved for the MCP
// JSON-RPC stream (mcp.StdioTransport).
func runMCPServer() {
	envErr := godotenv.Load()
	logger.Configure(os.Stderr, logger.ParseLevel(os.Getenv("LOG_LEVEL")))
	if envErr != nil {
		logger.Info("no .env file found, reading env from environment")
	}
	finnhubKey := os.Getenv("FINNHUB_API_KEY")
	finmindToken := os.Getenv("FINMIND_TOKEN")
	shioajiAddr := os.Getenv("SHIOAJI_ADDR")
	lang := i18n.Parse(app.EnvOr("BOT_LANGUAGE", "zh"))

	// Read-only DB connection for get_watchlist/get_portfolio/
	// get_recommendation_stats/get_universe_summary (Phase 3.5 "追加項" —
	// see db.OpenReadOnly's doc comment for how read-only is actually
	// enforced), and also (Phase 19 PR2) the source for the news-blacklist
	// closure below — opened before provider construction for that reason,
	// unlike main()'s ordering. A failure here degrades exactly like a
	// missing Finnhub key: those four tools are simply not registered
	// (mcptools.NewServer's nil-check), everything else still works, and the
	// news filter just passes everything through (see newsBlocked below).
	// DB_PATH falls back to the same default as main() for the case where
	// this subcommand is run directly (e.g. manual testing from the repo
	// root) rather than spawned as a chat session's MCP server, where main()
	// always exports an absolute DB_PATH before the subprocess is launched.
	dbPath := app.EnvOr("DB_PATH", "data/argus.db")
	database, err := db.OpenReadOnly(dbPath)
	if err != nil {
		logger.Errorf("mcp: open read-only db: %v", err)
		database = nil
	} else {
		defer database.Close()
	}

	// Same provider construction as app.Boot (see NewCoreProviders, Phase 24
	// tech debt 6): Finnhub-only tools (statements/earnings) stay nil
	// without a key, same as Bot's — and fundamentalsProvider is the same
	// US/TW FundamentalsRouter as Boot (Phase 6 PR3) — mcptools.NewServer's
	// nil-check on it only skips registering get_fundamentals/
	// get_financial_statements when *both* keys are absent, not just
	// Finnhub's. database is nil when the read-only open above failed, in
	// which case newsBlocked never reports anything blocked rather than
	// every get_news call erroring out.
	core := app.NewCoreProviders(finnhubKey, finmindToken, shioajiAddr, func(source string) bool {
		if database == nil {
			return false
		}
		blocked, err := database.IsNewsSourceBlocked(source)
		if err != nil {
			logger.Errorf("news filter: check blocked source %q: %v", source, err)
			return false
		}
		return blocked
	})
	// twInstitutional (TWSE T86), like Boot's twMovers, needs no API key.
	twInstitutional := data.NewTWSE()

	// Writable DB connection for add_to_watchlist/remove_from_watchlist
	// (Phase 3.5 "watchlist 寫入工具" pilot — see db.OpenForWrites' doc
	// comment for why this is a distinct connection from the read-only one
	// above rather than the same one with query_only left off). Same
	// nil-degrade contract: a failure here only takes down these two tools.
	writeDatabase, err := db.OpenForWrites(dbPath)
	if err != nil {
		logger.Errorf("mcp: open writable db: %v", err)
		writeDatabase = nil
	} else {
		defer writeDatabase.Close()
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := mcptools.Run(ctx, lang, core.Provider, core.Yahoo, core.Fundamentals, core.Earnings, core.InsiderTx, twInstitutional, database, writeDatabase); err != nil {
		logger.Fatalf("mcp server: %v", err)
	}
}
