package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"gopkg.in/natefinch/lumberjack.v2"

	"argus/internal/bot"
	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/llm"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/mcptools"
	"argus/internal/scheduler"
	"argus/internal/web"
)

var cst = time.FixedZone("CST", 8*3600)

func main() {
	// "mcp" runs this same binary as an MCP server over stdio instead of the
	// Telegram bot (see internal/mcptools) — branch before any of the
	// Telegram/env setup below, since an MCP subprocess (spawned by an ACP
	// chat session via os.Executable(), not by a human) needs none of it.
	if len(os.Args) > 1 && os.Args[1] == "mcp" {
		runMCPServer()
		return
	}

	// "eval" runs the offline recommendation-history scorer (see
	// docs/offline-rec-eval.md, cmd/bot/eval.go) instead of the Telegram
	// bot — a manual, one-shot terminal tool, same never-needs-Telegram-env
	// branch shape as "mcp" above.
	if len(os.Args) > 1 && os.Args[1] == "eval" {
		runEval()
		return
	}

	// "backtest" replays the recommendations table through internal/paper —
	// the live paper account's forward rule engine — instead of the Telegram
	// bot (see docs/phase-11-paper-account.md §5, cmd/bot/backtest.go). Same
	// never-needs-Telegram-env branch shape as "eval" above.
	if len(os.Args) > 1 && os.Args[1] == "backtest" {
		runBacktest()
		return
	}

	cfg := loadConfig()

	// Ensure DB directory exists
	if err := os.MkdirAll("data", 0o755); err != nil {
		logger.Fatalf("create data dir: %v", err)
	}

	// Re-export DB_PATH as an absolute path so llm.argusMCPServer (which
	// reads it back via os.Getenv when a chat session spins up the MCP
	// subprocess) sees the right file regardless of DB_PATH's default vs.
	// explicit-in-.env origin. This matters because that subprocess is
	// launched by claude-agent-acp from os.TempDir(), not this process's
	// cwd (see acp_provider.go's startClaudeSession) — a relative path here
	// would resolve against the wrong directory once handed to the child.
	if absDBPath, err := filepath.Abs(cfg.DBPath); err != nil {
		logger.Warnf("warning: could not resolve absolute DB_PATH from %q: %v", cfg.DBPath, err)
	} else {
		os.Setenv("DB_PATH", absDBPath)
	}

	// Log to both stdout (visible via `docker logs`/systemd journal) and a
	// daily-rotated file (registered below on the scheduler) so a VPS
	// deployment has something to grep after the fact — lumberjack only
	// rotates on size by itself; MaxAge+MaxBackups here cap it at roughly a
	// week of history so the log can't slowly fill the disk.
	logFile := &lumberjack.Logger{
		Filename:   cfg.LogPath,
		MaxBackups: 7,
		MaxAge:     7,
	}
	defer logFile.Close()
	logger.Configure(io.MultiWriter(os.Stdout, logFile), logger.ParseLevel(os.Getenv("LOG_LEVEL")))

	database, err := db.New(cfg.DBPath)
	if err != nil {
		logger.Fatalf("open database: %v", err)
	}
	defer database.Close()

	// paperDatabase is Phase 11 PR3's live paper account — a second,
	// physically separate SQLite file opened via the same db.New (so it gets
	// the same migrations), never a second connection to dbPath. nil when
	// PAPER_DB_PATH is unset, which bot.Config's PaperDB field treats as
	// "feature off" (see internal/bot/paper.go).
	var paperDatabase *db.DB
	if cfg.PaperDBPath != "" {
		paperDatabase, err = db.New(cfg.PaperDBPath)
		if err != nil {
			logger.Fatalf("open paper database: %v", err)
		}
		defer paperDatabase.Close()
	}

	// newsBlocked backs Phase 19 PR2's global news-source blacklist (see
	// docs/phase-19-llm-transparency.md §5) — a closure reading straight from
	// the DB, not a snapshot, so a block/unblock via the /llm page takes
	// effect on the very next fetch, and so internal/data never needs an
	// internal/db import. Wrapping multi/marketNewsProvider/twMarketNews from
	// the outside (rather than filtering inside Multi.GetNews) keeps Multi's
	// provider-fallback semantics untouched: a filtered-to-empty result is
	// still a success, never a trigger to fall through to the next provider.
	newsBlocked := func(source string) bool {
		blocked, err := database.IsNewsSourceBlocked(source)
		if err != nil {
			logger.Errorf("news filter: check blocked source %q: %v", source, err)
			return false
		}
		return blocked
	}

	// Set up the multi-provider data layer (Finnhub primary, Yahoo fallback)
	// shared with runMCPServer() — see newCoreProviders (Phase 24 tech debt
	// 6). Fundamentals/financial statements, the earnings calendar, and
	// general market news are all Finnhub-only (Yahoo's fundamentals
	// equivalent requires a crumb/cookie handshake we don't implement, and
	// has no earnings-calendar or general-news-category endpoint at all), so
	// all three providers stay nil when no Finnhub key is configured.
	// Historical closes (for RSI/MACD) go the other way: Finnhub's free tier
	// blocks /stock/candle entirely, so history is Yahoo-only.
	core := newCoreProviders(cfg.FinnhubKey, cfg.FinMindToken, cfg.ShioajiAddr, newsBlocked)
	provider := core.Provider
	yahoo := core.Yahoo
	sinopacClient := core.Sinopac
	fundamentalsProvider := core.Fundamentals
	companyNameProvider := core.CompanyNames
	insiderTxProvider := core.InsiderTx
	earningsProvider := core.Earnings

	// analystRatingProvider/marketNewsProvider/sectorProvider/
	// earningsSurpriseProvider are Finnhub fields runMCPServer() has no use
	// for, so newCoreProviders doesn't return them — set here directly off
	// core.Finnhub instead. sectorProvider/industryMapProvider back Phase
	// 18's /api/sectorflow (US via Finnhub per-ticker, TW via FinMind's
	// whole-market map, see internal/web/sectorflow.go) — only web.Config
	// reads these, the bot itself has no sector-flow surface (web-only, see
	// PLAN.md's Phase 18 entry).
	var analystRatingProvider data.AnalystRatingProvider
	var marketNewsProvider data.MarketNewsProvider
	var sectorProvider data.SectorProvider
	var earningsSurpriseProvider data.EarningsSurpriseProvider
	if core.Finnhub != nil {
		analystRatingProvider = core.Finnhub
		marketNewsProvider = core.Finnhub
		sectorProvider = core.Finnhub
		earningsSurpriseProvider = core.Finnhub // Phase 23 PR8
	}
	// trustNetProvider/twValuationProvider are FinMind fields runMCPServer()
	// has no use for, same reasoning as the Finnhub block above.
	var trustNetProvider data.TrustNetProvider
	var industryMapProvider data.IndustryMapProvider
	var twValuationProvider data.FundamentalHistoryProvider
	if core.FinMind != nil {
		trustNetProvider = core.FinMind
		industryMapProvider = core.FinMind
		twValuationProvider = core.FinMind // Phase 23 PR7: TaiwanStockPER-derived valuation percentile
	}
	if marketNewsProvider != nil {
		marketNewsProvider = data.NewMarketNewsFilter(marketNewsProvider, newsBlocked)
	}

	// SEC EDGAR (Phase 23 PR6) is US-only and needs a real contact-email UA
	// (see internal/data/sec.go) — presence-gated same as Finnhub/FinMind
	// above, left nil (not constructed at all) when SEC_USER_AGENT is unset.
	var secFundamentalsProvider data.FundamentalHistoryProvider
	if cfg.SECUserAgent != "" {
		secFundamentalsProvider = data.NewSEC(cfg.SECUserAgent)
	}

	// twMovers/twMarketNews (2026-07-28 TW data-gap PR) need no API key —
	// TWSE's OpenAPI and cnyes are both free/keyless — so unlike
	// fundamentalsProvider/earningsProvider above they're constructed
	// unconditionally, same as yahoo/History.
	twMovers := data.NewTWSE()
	twMarketNews := data.MarketNewsProvider(data.NewMarketNewsFilter(data.NewCnyes(), newsBlocked))
	var twMoversProvider data.TWMarketMoversProvider = twMovers
	if sinopacClient != nil {
		// Scanner's AmountRank covers TPEx-listed movers too, which
		// TWSE.GetMarketMovers (TWSE-exchange-only) never could — falls
		// back to twMovers on any daemon error.
		twMoversProvider = data.NewShioajiMovers(sinopacClient, twMovers)
	}

	llmClient := llm.NewClient(cfg.RecommendModel, cfg.CheckModel, cfg.ChatModel, cfg.Lang)
	// Antigravity fallback is opt-in, not presence-of-config-gated like
	// Finnhub above: agy has no read-only mode for non-interactive calls (see
	// AntigravityProvider's doc comment and PLAN.md's architecture-debt
	// entry), so wiring it in is a deliberate risk the operator accepts via
	// ANTIGRAVITY_ENABLED, not something that should turn on just because a
	// model name happened to be set.
	if os.Getenv("ANTIGRAVITY_ENABLED") == "true" {
		antigravityModel := os.Getenv("ANTIGRAVITY_MODEL")
		llmClient.AddFallback(llm.AntigravityProvider{}, antigravityModel, antigravityModel, antigravityModel)
	}
	defer llmClient.Close() // kills any still-open persistent chat session's subprocess

	// Telegram is optional as of Phase 17 PR1 (docs/phase-17-web-settings.md
	// §3): without a token/chat id — or with a pair Telegram itself rejects,
	// which bot.New finds out synchronously via getMe — telegramBot stays nil
	// and the process still comes up to serve the web dashboard, whose
	// Settings page is how the operator fills these in. Every use of
	// telegramBot below is therefore nil-guarded.
	var telegramBot *bot.Bot
	if cfg.TelegramToken == "" || cfg.ChatID == 0 {
		logger.Info("TELEGRAM_BOT_TOKEN/TELEGRAM_CHAT_ID not configured: Telegram disabled, set them on the web dashboard's Settings page")
	} else if telegramBot, err = bot.New(bot.Config{
		Token:                  cfg.TelegramToken,
		ChatID:                 cfg.ChatID,
		DB:                     database,
		Provider:               provider,
		Fundamentals:           fundamentalsProvider,
		AnalystRating:          analystRatingProvider,
		InsiderTx:              insiderTxProvider,
		EarningsSurprise:       earningsSurpriseProvider,
		Institutional:          twMovers,
		Earnings:               earningsProvider,
		MarketNews:             marketNewsProvider,
		TWMarketNews:           twMarketNews,
		TWMovers:               twMoversProvider,
		CompanyNames:           companyNameProvider,
		TrustNet:               trustNetProvider,
		OptionChain:            yahoo,
		SECFundamentals:        secFundamentalsProvider,
		TWValuation:            twValuationProvider,
		History:                yahoo,
		Sinopac:                sinopacClient,
		SinopacSkip:            cfg.SinopacSkip,
		SinopacSyncLive:        cfg.SinopacSyncLive,
		LLM:                    llmClient,
		Lang:                   cfg.Lang,
		StopLossPct:            cfg.StopLossPct,
		TrailingStopPct:        cfg.TrailingStopPct,
		StopLossPctTW:          cfg.StopLossPctTW,
		TrailingStopPctTW:      cfg.TrailingStopPctTW,
		TrailingStopATRMult:    cfg.TrailingStopATRMult,
		RiskPctPerTrade:        cfg.RiskPctPerTrade,
		TWFeeDiscount:          cfg.TWFeeDiscount,
		PaperDB:                paperDatabase,
		PaperInitialCashUSD:    cfg.PaperInitialCashUSD,
		PaperInitialCashTWD:    cfg.PaperInitialCashTWD,
		PaperMaxPositionPct:    cfg.PaperMaxPositionPct,
		PaperTakeProfitATRMult: cfg.PaperTakeProfitATRMult,
	}); err != nil {
		// A rejected token (bot.New's getMe call) must not be fatal either:
		// the recovery path for a typo'd token is the Settings page, which
		// only exists if this process keeps running.
		logger.Errorf("init bot: %v (Telegram disabled, dashboard still available)", err)
		telegramBot = nil
	}

	// Phase 2.6 追加項's S&P 500 refresh (see docs/phase-2.6-universe-refresh.md):
	// runs once per process start, right after seedSP500's fresh-install
	// path (inside bot.New -> db.New) has already had its chance to run.
	if telegramBot != nil {
		telegramBot.SyncUniverse()
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Constructed here (not at its original spot below) so the Phase 18
	// sector-flow jobs can be registered on it from inside the WebAddr block
	// right below, right alongside webServer's own construction — cron
	// registration order doesn't matter to robfig/cron, so moving this up
	// changes nothing about the jobs registered further down.
	sched := scheduler.New()

	// trade must be assigned only when telegramBot is non-nil: handing a nil
	// *bot.Bot straight to web.Config.Trade would produce a non-nil interface
	// wrapping a nil pointer, which every `s.trade == nil` check in
	// internal/web would then miss (server.go's paperDB comment names the
	// same trap).
	var trade web.TradeExecutor
	if telegramBot != nil {
		trade = telegramBot
	}

	if cfg.WebAddr != "" {
		// In-process, not a subcommand like "mcp": the dashboard needs live
		// quotes (data.Provider) alongside DB reads, and shares this
		// process's *db.DB connection directly rather than opening a
		// second db.OpenReadOnly one — unlike the MCP subprocess, this
		// isn't a separate process, and database/sql connections already
		// support concurrent use from other goroutines.
		webServer := web.New(web.Config{
			DB:           database,
			Provider:     provider,
			History:      yahoo,
			Earnings:     earningsProvider,
			Lang:         cfg.Lang,
			CompanyNames: companyNameProvider,
			OptionChain:  yahoo,
			RiskHeatPct:  cfg.RiskHeatPct,
			Password:     cfg.WebPassword,
			Trade:        trade,
			// Phase 17 PR2: the file godotenv.Load() already read at startup
			// — not configurable, since editing any other file would have no
			// effect on the next boot.
			EnvPath:             ".env",
			PaperDB:             paperDatabase,
			PaperInitialCashUSD: cfg.PaperInitialCashUSD,
			PaperInitialCashTWD: cfg.PaperInitialCashTWD,
			Sector:              sectorProvider,
			IndustryMap:         industryMapProvider,
			LLMAudit:            cfg.WebLLMAudit,
		})
		go func() {
			if err := webServer.Run(ctx, cfg.WebAddr); err != nil {
				logger.Errorf("web: server error: %v", err)
			}
		}()
		// Phase 18's sector money-flow scan only has a consumer when the
		// dashboard is running, so it's registered here rather than
		// unconditionally alongside the other jobs below.
		sched.AddSectorFlowScan(ctx, func(ctx context.Context) {
			webServer.RunSectorFlowScan(ctx, market.US)
		})
		sched.AddTWSectorFlowScan(ctx, func(ctx context.Context) {
			webServer.RunSectorFlowScan(ctx, market.TW)
		})
	}

	// Every job below reports to Telegram, so the whole block is registered
	// as one unit rather than nil-guarding twelve closures — see
	// docs/phase-17-web-settings.md §3.1. Log rotation and backups are
	// deliberately outside it: they're the two jobs that still matter on a
	// Telegram-less process.
	if telegramBot != nil {
		sched.AddDailyReport(ctx, func(ctx context.Context) {
			telegramBot.RunDailyReport(ctx)
		})
		sched.AddMorningBriefing(ctx, func(ctx context.Context) {
			telegramBot.RunUSMorningBriefing(ctx)
		})
		sched.AddTWDailyReport(ctx, func(ctx context.Context) {
			telegramBot.RunTWDailyReport(ctx)
		})
		sched.AddTWMorningBriefing(ctx, func(ctx context.Context) {
			telegramBot.RunTWMorningBriefing(ctx)
		})
		sched.AddClosingSnapshot(ctx, func(ctx context.Context) {
			telegramBot.RunClosingSnapshot(ctx, market.US)
		})
		sched.AddTWClosingSnapshot(ctx, func(ctx context.Context) {
			telegramBot.RunClosingSnapshot(ctx, market.TW)
		})
		sched.AddUniverseScan(ctx, func(ctx context.Context) {
			telegramBot.RunUniverseScan(ctx)
		})
		sched.AddTWUniverseScan(ctx, func(ctx context.Context) {
			telegramBot.RunTWUniverseScan(ctx)
		})
		sched.AddWeeklyReview(ctx, func(ctx context.Context) {
			telegramBot.RunWeeklyReview(ctx)
		})
		sched.AddSinopacSync(ctx, func(ctx context.Context) {
			if sinopacClient == nil {
				return
			}
			if msg, found := telegramBot.RunSinopacSync(ctx, !cfg.SinopacSyncLive); found {
				telegramBot.Send(msg)
			}
		})
		sched.AddMonthlyReport(ctx, func(ctx context.Context) {
			telegramBot.RunMonthlyReport(ctx)
		})
	}

	sched.AddLogRotation(func() {
		if err := logFile.Rotate(); err != nil {
			logger.Errorf("log rotation: %v", err)
		}
	})
	sched.AddBackup(func() {
		runBackup(database, cfg.BackupDir, cfg.BackupRetentionDays, "argus")
		if paperDatabase != nil {
			runBackup(paperDatabase, cfg.BackupDir, cfg.BackupRetentionDays, "argus-paper")
		}
	})
	sched.Start()
	defer sched.Stop()

	logger.Info("stock trader bot started")
	if telegramBot != nil {
		telegramBot.Run(ctx)
	} else {
		// Nothing to long-poll, but the web server, log rotation and backups
		// are all still running on their own goroutines — block until a
		// signal (or the Settings page's os.Exit) ends the process.
		<-ctx.Done()
	}
	logger.Info("bot stopped")
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
	lang := i18n.Parse(envOr("BOT_LANGUAGE", "zh"))

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
	dbPath := envOr("DB_PATH", "data/argus.db")
	database, err := db.OpenReadOnly(dbPath)
	if err != nil {
		logger.Errorf("mcp: open read-only db: %v", err)
		database = nil
	} else {
		defer database.Close()
	}

	// Same provider construction as main() (see newCoreProviders, Phase 24
	// tech debt 6): Finnhub-only tools (statements/earnings) stay nil
	// without a key, same as Bot's — and fundamentalsProvider is the same
	// US/TW FundamentalsRouter as main() (Phase 6 PR3) — mcptools.NewServer's
	// nil-check on it only skips registering get_fundamentals/
	// get_financial_statements when *both* keys are absent, not just
	// Finnhub's. database is nil when the read-only open above failed, in
	// which case newsBlocked never reports anything blocked rather than
	// every get_news call erroring out.
	core := newCoreProviders(finnhubKey, finmindToken, shioajiAddr, func(source string) bool {
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
	provider := core.Provider
	yahoo := core.Yahoo
	fundamentalsProvider := core.Fundamentals
	earningsProvider := core.Earnings
	insiderTxProvider := core.InsiderTx
	// twInstitutional (TWSE T86), like main()'s twMovers, needs no API key.
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
	if err := mcptools.Run(ctx, lang, provider, yahoo, fundamentalsProvider, earningsProvider, insiderTxProvider, twInstitutional, database, writeDatabase); err != nil {
		logger.Fatalf("mcp server: %v", err)
	}
}

// runBackup writes a dated SQLite backup (via db.Backup's VACUUM INTO) into
// dir and prunes backup files older than retentionDays. transactions/
// positions are irreplaceable personal financial data with no other backup
// path on a single VPS, hence a daily on-disk copy — see PLAN.md. prefix
// distinguishes the real ("argus") and paper-account ("argus-paper", Phase
// 11 PR3) backup files sharing one dir — pruneOldBackups doesn't filter by
// prefix, so calling this for both databases just prunes the shared dir
// twice, which is harmless at once-a-day cron frequency.
func runBackup(database *db.DB, dir string, retentionDays int, prefix string) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logger.Errorf("backup: create dir: %v", err)
		return
	}

	dest := filepath.Join(dir, fmt.Sprintf("%s-%s.db", prefix, time.Now().In(cst).Format("2006-01-02")))
	if err := database.Backup(dest); err != nil {
		logger.Errorf("backup: %v", err)
		return
	}
	logger.Infof("backup: wrote %s", dest)

	pruneOldBackups(dir, retentionDays)
}

func pruneOldBackups(dir string, retentionDays int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		logger.Errorf("backup: prune: read dir: %v", err)
		return
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			logger.Errorf("backup: prune: stat %s: %v", e.Name(), err)
			continue
		}
		if info.ModTime().After(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
			logger.Errorf("backup: prune: remove %s: %v", e.Name(), err)
		}
	}
}
