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
	"argus/internal/sinopac"
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

	// Set up multi-provider data layer (Finnhub primary, Yahoo fallback).
	// Fundamentals/financial statements, the earnings calendar, and general
	// market news are all Finnhub-only (Yahoo's fundamentals equivalent
	// requires a crumb/cookie handshake we don't implement, and has no
	// earnings-calendar or general-news-category endpoint at all), so all
	// three providers stay nil when no Finnhub key is configured. Historical
	// closes (for RSI/MACD) go the other way: Finnhub's free tier blocks
	// /stock/candle entirely, so history is Yahoo-only.
	var providers []data.Provider
	var analystRatingProvider data.AnalystRatingProvider
	var insiderTxProvider data.InsiderTransactionProvider
	var earningsProvider data.EarningsProvider
	var marketNewsProvider data.MarketNewsProvider
	var companyNameProvider data.CompanyNameProvider
	var trustNetProvider data.TrustNetProvider
	// sectorProvider/industryMapProvider back Phase 18's /api/sectorflow —
	// US via Finnhub (per-ticker), TW via FinMind's whole-market map (see
	// internal/web/sectorflow.go). Only web.Config reads these; the bot
	// itself has no sector-flow surface (web-only, see PLAN.md's Phase 18
	// entry).
	var sectorProvider data.SectorProvider
	var industryMapProvider data.IndustryMapProvider
	fundamentalsRouter := &data.FundamentalsRouter{}
	if cfg.FinnhubKey != "" {
		finnhub := data.NewFinnhub(cfg.FinnhubKey)
		providers = append(providers, finnhub)
		fundamentalsRouter.US = finnhub
		analystRatingProvider = finnhub
		insiderTxProvider = finnhub
		earningsProvider = finnhub
		marketNewsProvider = finnhub
		sectorProvider = finnhub
	}
	// FINMIND_TOKEN gates TW fundamentals the same way FINNHUB_API_KEY gates
	// US (Phase 6 PR3) — FinMind has no Yahoo-style Multi fallback to sit
	// behind, it's the only TW fundamentals source there is (see
	// internal/data/finmind.go). Reused as the CompanyNameProvider too (its
	// TaiwanStockInfo lookup, see finmind.go's GetCompanyName) — same client,
	// not a second one, since there's nothing FinMind-specific about that
	// call that would justify a separate instance.
	if cfg.FinMindToken != "" {
		finmind := data.NewFinMind(cfg.FinMindToken)
		fundamentalsRouter.TW = finmind
		companyNameProvider = finmind
		trustNetProvider = finmind
		industryMapProvider = finmind
	}
	// fundamentalsProvider stays nil (not a router wrapping two nil fields)
	// when neither key is set, preserving every existing `if b.fundamentals
	// != nil` nil-check's pre-Phase-6-PR3 behavior exactly.
	var fundamentalsProvider data.FundamentalsProvider
	if fundamentalsRouter.US != nil || fundamentalsRouter.TW != nil {
		fundamentalsProvider = fundamentalsRouter
	}
	// Shioaji sits between Finnhub and GoogleNews when SHIOAJI_ADDR is
	// configured — broker-grade TW quotes ahead of Yahoo's unofficial,
	// suffix-guessing chart API (see internal/data/shioaji.go). Left nil
	// (no reference anywhere else) when unset, matching the presence-gated
	// convention every other optional provider here follows; any daemon
	// error/session drop falls straight through to GoogleNews/Yahoo since
	// Multi tries providers in order until one succeeds.
	var sinopacClient *sinopac.Client
	if cfg.ShioajiAddr != "" {
		sinopacClient = sinopac.New(cfg.ShioajiAddr)
		providers = append(providers, data.NewShioaji(sinopacClient))
	}
	// Google News sits between Finnhub and Yahoo so TW tickers get
	// Chinese-language per-ticker news: Finnhub's /company-news is US-only,
	// and Yahoo's search API answers a .TW symbol with mostly English wire
	// coverage. It answers TW tickers only (errGoogleNewsNotTW otherwise), so
	// the US path stays Finnhub-then-Yahoo exactly as before at the cost of
	// one no-network method call per US news lookup. Constructed after the
	// FinMind block because it takes companyNameProvider — with a token it
	// searches "台積電", without one the bare "2330" (see GoogleNews.GetNews
	// for why that's materially worse but still worth having).
	providers = append(providers, data.NewGoogleNews(companyNameProvider))
	yahoo := data.NewYahoo()
	providers = append(providers, yahoo)
	provider := data.NewMulti(providers...)

	// twMovers/twMarketNews (2026-07-28 TW data-gap PR) need no API key —
	// TWSE's OpenAPI and cnyes are both free/keyless — so unlike
	// fundamentalsProvider/earningsProvider above they're constructed
	// unconditionally, same as yahoo/History.
	twMovers := data.NewTWSE()
	twMarketNews := data.NewCnyes()
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

	telegramBot, err := bot.New(bot.Config{
		Token:                  cfg.TelegramToken,
		ChatID:                 cfg.ChatID,
		DB:                     database,
		Provider:               provider,
		Fundamentals:           fundamentalsProvider,
		AnalystRating:          analystRatingProvider,
		InsiderTx:              insiderTxProvider,
		Institutional:          twMovers,
		Earnings:               earningsProvider,
		MarketNews:             marketNewsProvider,
		TWMarketNews:           twMarketNews,
		TWMovers:               twMoversProvider,
		CompanyNames:           companyNameProvider,
		TrustNet:               trustNetProvider,
		OptionChain:            yahoo,
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
	})
	if err != nil {
		logger.Fatalf("init bot: %v", err)
	}

	// Phase 2.6 追加項's S&P 500 refresh (see docs/phase-2.6-universe-refresh.md):
	// runs once per process start, right after seedSP500's fresh-install
	// path (inside bot.New -> db.New) has already had its chance to run.
	telegramBot.SyncUniverse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Constructed here (not at its original spot below) so the Phase 18
	// sector-flow jobs can be registered on it from inside the WebAddr block
	// right below, right alongside webServer's own construction — cron
	// registration order doesn't matter to robfig/cron, so moving this up
	// changes nothing about the jobs registered further down.
	sched := scheduler.New()

	if cfg.WebAddr != "" {
		// In-process, not a subcommand like "mcp": the dashboard needs live
		// quotes (data.Provider) alongside DB reads, and shares this
		// process's *db.DB connection directly rather than opening a
		// second db.OpenReadOnly one — unlike the MCP subprocess, this
		// isn't a separate process, and database/sql connections already
		// support concurrent use from other goroutines.
		webServer := web.New(web.Config{
			DB:                  database,
			Provider:            provider,
			History:             yahoo,
			Earnings:            earningsProvider,
			Lang:                cfg.Lang,
			CompanyNames:        companyNameProvider,
			OptionChain:         yahoo,
			RiskHeatPct:         cfg.RiskHeatPct,
			Password:            cfg.WebPassword,
			Trade:               telegramBot,
			PaperDB:             paperDatabase,
			PaperInitialCashUSD: cfg.PaperInitialCashUSD,
			PaperInitialCashTWD: cfg.PaperInitialCashTWD,
			Sector:              sectorProvider,
			IndustryMap:         industryMapProvider,
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
	telegramBot.Run(ctx)
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

	// Same provider construction as main(): Finnhub-only tools
	// (statements/earnings) stay nil without a key, same as Bot's — and
	// fundamentalsProvider is the same US/TW FundamentalsRouter as main()
	// (Phase 6 PR3) — mcptools.NewServer's nil-check on it only skips
	// registering get_fundamentals/get_financial_statements when *both*
	// keys are absent, not just Finnhub's.
	var providers []data.Provider
	var earningsProvider data.EarningsProvider
	var insiderTxProvider data.InsiderTransactionProvider
	fundamentalsRouter := &data.FundamentalsRouter{}
	if finnhubKey != "" {
		finnhub := data.NewFinnhub(finnhubKey)
		providers = append(providers, finnhub)
		fundamentalsRouter.US = finnhub
		earningsProvider = finnhub
		insiderTxProvider = finnhub
	}
	var companyNameProvider data.CompanyNameProvider
	if finmindToken != "" {
		finmind := data.NewFinMind(finmindToken)
		fundamentalsRouter.TW = finmind
		companyNameProvider = finmind
	}
	var fundamentalsProvider data.FundamentalsProvider
	if fundamentalsRouter.US != nil || fundamentalsRouter.TW != nil {
		fundamentalsProvider = fundamentalsRouter
	}
	// Same Shioaji-before-GoogleNews ordering as main(), for the same
	// reason: get_quote on a TW ticker should reach broker-grade data
	// rather than Yahoo's suffix-guessing chart API.
	if shioajiAddr != "" {
		providers = append(providers, data.NewShioaji(sinopac.New(shioajiAddr)))
	}
	// Same Google-News-before-Yahoo ordering as main(), for the same reason:
	// get_news on a TW ticker should reach Chinese coverage rather than
	// Yahoo's English wire stories.
	providers = append(providers, data.NewGoogleNews(companyNameProvider))
	yahoo := data.NewYahoo()
	providers = append(providers, yahoo)
	provider := data.NewMulti(providers...)
	// twInstitutional (TWSE T86), like main()'s twMovers, needs no API key.
	twInstitutional := data.NewTWSE()

	// Read-only DB connection for get_watchlist/get_portfolio/
	// get_recommendation_stats/get_universe_summary (Phase 3.5 "追加項" —
	// see db.OpenReadOnly's doc comment for how read-only is actually
	// enforced). A failure here degrades exactly like a missing Finnhub
	// key: those four tools are simply not registered (mcptools.NewServer's
	// nil-check), everything else still works. DB_PATH falls back to the
	// same default as main() for the case where this subcommand is run
	// directly (e.g. manual testing from the repo root) rather than spawned
	// as a chat session's MCP server, where main() always exports an
	// absolute DB_PATH before the subprocess is launched.
	dbPath := envOr("DB_PATH", "data/argus.db")
	database, err := db.OpenReadOnly(dbPath)
	if err != nil {
		logger.Errorf("mcp: open read-only db: %v", err)
		database = nil
	} else {
		defer database.Close()
	}

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
