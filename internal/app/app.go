package app

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/natefinch/lumberjack.v2"

	"argus/internal/bot"
	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/llm"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/notification"
	"argus/internal/scheduler"
	"argus/internal/service"
	"argus/internal/signals"
	"argus/internal/web"
)

// DefaultWebAddr is the address cmd/server falls back to when WEB_ADDR is
// unset. cmd/bot deliberately does not use it: there, an empty WEB_ADDR still
// means "no dashboard" (Config.WebAddr's presence-of-config convention), and
// silently binding a port on an existing deployment that never asked for one
// would be a behavior change. cmd/server is the inverse by definition — a
// server with no HTTP listener is not a server — so it defaults instead of
// gating. No new env var: WEB_ADDR already names exactly this thing.
//
// Loopback-only, not "0.0.0.0:8080": internal/web/server.go says outright
// that the dashboard "deliberately has no auth/HTTPS of its own" beyond
// WEB_PASSWORD gating writes — every read route (portfolio, P&L, positions)
// is wide open. cmd/bot never bound a port the operator hadn't explicitly
// asked for; cmd/server binding all interfaces by default on a VPS with a
// routable public IP would hand that same open read surface to anyone who
// port-scans it. An operator who wants it reachable sets WEB_ADDR=:8080
// themselves — a decision this default shouldn't make for them.
const DefaultWebAddr = "127.0.0.1:8080"

// App is everything Boot wired up: the long-running pieces Run drives and the
// resources Close releases. Fields are exported so an entrypoint can reach
// past Run for a one-shot (cmd/bot's SyncUniverse, say) without Boot needing
// a callback parameter for each such case.
type App struct {
	cfg     Config
	DB      *db.DB
	PaperDB *db.DB
	LLM     *llm.Client
	// Bot is nil when Telegram isn't configured (Phase 17 PR1) — every use
	// of it, here and in the entrypoints, must stay nil-guarded.
	Bot *bot.Bot
	// Web is nil only when cfg.WebAddr is empty (cmd/bot's dashboard-off
	// state); cmd/server always has one, see DefaultWebAddr.
	Web       *web.Server
	Scheduler *scheduler.Scheduler
	// Notifier is the process-wide event bus (Phase 24 Stage 2). Built here
	// rather than inside the bot as of Stage 3 Step 3.2, because scheduler
	// jobs that no longer go through *Bot still need somewhere to publish —
	// and because Stage 4's WebSocketNotifier has no *Bot to hang off at all.
	Notifier *notification.Dispatcher
	// Events is the live-alert fan-out behind /api/v1/ws (Stage 4 Step 4.3)
	// — Stage 2's deferred WebSocketNotifier, now that there's a subscriber.
	Events *notification.WebSocketHub
	// Scan runs the universe scan for the scheduler. Separate from the
	// ScanService the bot builds for its own daily-report path: they share
	// their dedup state through signal_states, not through a struct.
	Scan    *service.ScanService
	logFile *lumberjack.Logger
}

// Boot wires the whole process up in dependency order — logging, DB, data
// providers, LLM, Telegram adapter, HTTP server, cron jobs — and returns
// without starting anything long-running (that's Run). Splitting construction
// from starting is what lets cmd/server bring the HTTP server up as the
// primary process with Telegram attached behind it, while cmd/bot keeps its
// original Telegram-first shape, off one copy of this wiring.
//
// ctx is only used for cron-job registration (scheduler.Add* takes the ctx
// each job body will run under); nothing here blocks on it.
func Boot(ctx context.Context, cfg Config) (a *App, err error) {
	a = &App{cfg: cfg}
	// Named return + this defer let every early-return below just bail with
	// `return` instead of `return nil, err`: whatever a already holds (DB
	// open but PaperDB failed, say) gets released instead of leaked, and
	// Close is already nil-safe field-by-field for exactly this partial case.
	defer func() {
		if err != nil {
			a.Close()
		}
	}()

	// cfg.DBPath's directory, not a hardcoded "data" — db.New doesn't create
	// it (internal/db/db.go's sql.Open expects the path to already resolve),
	// and a DB_PATH outside ./data (e.g. a VPS's /var/lib/argus) would
	// otherwise silently create an unused ./data and then fail to open.
	if err = os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		return
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
	a.logFile = &lumberjack.Logger{
		Filename:   cfg.LogPath,
		MaxBackups: 7,
		MaxAge:     7,
	}
	logger.Configure(io.MultiWriter(os.Stdout, a.logFile), logger.ParseLevel(os.Getenv("LOG_LEVEL")))

	database, err := db.New(cfg.DBPath)
	if err != nil {
		return
	}
	a.DB = database

	// PaperDB is Phase 11 PR3's live paper account — a second, physically
	// separate SQLite file opened via the same db.New (so it gets the same
	// migrations), never a second connection to DBPath. nil when
	// PAPER_DB_PATH is unset, which bot.Config's PaperDB field treats as
	// "feature off" (see internal/bot/paper.go).
	if cfg.PaperDBPath != "" {
		a.PaperDB, err = db.New(cfg.PaperDBPath)
		if err != nil {
			return
		}
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
	// shared with runMCPServer() — see NewCoreProviders (Phase 24 tech debt
	// 6). Fundamentals/financial statements, the earnings calendar, and
	// general market news are all Finnhub-only (Yahoo's fundamentals
	// equivalent requires a crumb/cookie handshake we don't implement, and
	// has no earnings-calendar or general-news-category endpoint at all), so
	// all three providers stay nil when no Finnhub key is configured.
	// Historical closes (for RSI/MACD) go the other way: Finnhub's free tier
	// blocks /stock/candle entirely, so history is Yahoo-only.
	core := NewCoreProviders(cfg.FinnhubKey, cfg.FinMindToken, cfg.ShioajiAddr, newsBlocked)

	// analystRatingProvider/marketNewsProvider/sectorProvider/
	// earningsSurpriseProvider are Finnhub fields runMCPServer() has no use
	// for, so NewCoreProviders doesn't return them — set here directly off
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
	if core.Sinopac != nil {
		// Scanner's AmountRank covers TPEx-listed movers too, which
		// TWSE.GetMarketMovers (TWSE-exchange-only) never could — falls
		// back to twMovers on any daemon error.
		twMoversProvider = data.NewShioajiMovers(core.Sinopac, twMovers)
	}

	a.LLM = llm.NewClient(cfg.RecommendModel, cfg.CheckModel, cfg.ChatModel, cfg.Lang)
	// Antigravity fallback is opt-in, not presence-of-config-gated like
	// Finnhub above: agy has no read-only mode for non-interactive calls (see
	// AntigravityProvider's doc comment and PLAN.md's architecture-debt
	// entry), so wiring it in is a deliberate risk the operator accepts via
	// ANTIGRAVITY_ENABLED, not something that should turn on just because a
	// model name happened to be set.
	if os.Getenv("ANTIGRAVITY_ENABLED") == "true" {
		antigravityModel := os.Getenv("ANTIGRAVITY_MODEL")
		a.LLM.AddFallback(llm.AntigravityProvider{}, antigravityModel, antigravityModel, antigravityModel)
	}

	// The Dispatcher and the scheduler's ScanService are both built before
	// the bot, and neither depends on it: that's the Stage 3 inversion in one
	// place. The bot registers its own TelegramNotifier onto this Dispatcher
	// when it's constructed below.
	// Events is registered on the Dispatcher unconditionally, not gated on
	// cfg.WebAddr: a hub with no subscribers costs one map lookup per
	// published event, and gating it would mean the /api/v1/ws route's
	// existence depended on two separate settings instead of one.
	a.Events = notification.NewWebSocketHub()
	a.Notifier = notification.NewDispatcher(notification.NewInAppNotificationStore(database), a.Events)
	// Restricted is TW-only and needs the Shioaji daemon; core.Sinopac is a
	// typed nil-safe pointer, so it's assigned through the interface only
	// when actually constructed (the coreProviders footgun again).
	var restricted service.RestrictedProvider
	if core.Sinopac != nil {
		restricted = core.Sinopac
	}
	a.Scan = service.NewScanService(service.ScanConfig{
		Store:    database,
		Detector: signals.NewDetector(cfg.Lang),
		TrustNet: trustNetProvider,
		// Uncached, unlike the bot's own ScanService: this reader only backs
		// the revenue-growth gate, which is short-circuited to a handful of
		// TW tickers a day (docs/phase-14-strategy-screens-2.md §4.2c), so a
		// cache in front of it would save single-digit requests. No explicit
		// rate limiting here either — RunUniverseScan's own 1s
		// per-ticker delay already paces every request this reader makes,
		// same as it paces the history/quote calls around it.
		Fundamentals: fundamentalsReader(core.Fundamentals),
		History:      core.Yahoo,
		Quotes:       core.Provider,
		Restricted:   restricted,
		Lang:         cfg.Lang,
	})

	// Telegram is optional as of Phase 17 PR1 (docs/phase-17-web-settings.md
	// §3): without a token/chat id — or with a pair Telegram itself rejects,
	// which bot.New finds out synchronously via getMe — a.Bot stays nil and
	// the process still comes up to serve the web dashboard, whose Settings
	// page is how the operator fills these in. Every use of a.Bot is
	// therefore nil-guarded.
	if cfg.TelegramToken == "" || cfg.ChatID == 0 {
		logger.Info("TELEGRAM_BOT_TOKEN/TELEGRAM_CHAT_ID not configured: Telegram disabled, set them on the web dashboard's Settings page")
	} else if a.Bot, err = bot.New(bot.Config{
		Token:                  cfg.TelegramToken,
		ChatID:                 cfg.ChatID,
		DB:                     database,
		Provider:               core.Provider,
		Fundamentals:           core.Fundamentals,
		AnalystRating:          analystRatingProvider,
		InsiderTx:              core.InsiderTx,
		EarningsSurprise:       earningsSurpriseProvider,
		Institutional:          twMovers,
		Earnings:               core.Earnings,
		MarketNews:             marketNewsProvider,
		TWMarketNews:           twMarketNews,
		TWMovers:               twMoversProvider,
		CompanyNames:           core.CompanyNames,
		TrustNet:               trustNetProvider,
		OptionChain:            core.Yahoo,
		SECFundamentals:        secFundamentalsProvider,
		TWValuation:            twValuationProvider,
		History:                core.Yahoo,
		Sinopac:                core.Sinopac,
		SinopacSkip:            cfg.SinopacSkip,
		SinopacSyncLive:        cfg.SinopacSyncLive,
		LLM:                    a.LLM,
		Lang:                   cfg.Lang,
		StopLossPct:            cfg.StopLossPct,
		TrailingStopPct:        cfg.TrailingStopPct,
		StopLossPctTW:          cfg.StopLossPctTW,
		TrailingStopPctTW:      cfg.TrailingStopPctTW,
		TrailingStopATRMult:    cfg.TrailingStopATRMult,
		RiskPctPerTrade:        cfg.RiskPctPerTrade,
		TWFeeDiscount:          cfg.TWFeeDiscount,
		PaperDB:                a.PaperDB,
		PaperInitialCashUSD:    cfg.PaperInitialCashUSD,
		PaperInitialCashTWD:    cfg.PaperInitialCashTWD,
		PaperMaxPositionPct:    cfg.PaperMaxPositionPct,
		PaperTakeProfitATRMult: cfg.PaperTakeProfitATRMult,
		Notifier:               a.Notifier,
	}); err != nil {
		// A rejected token (bot.New's getMe call) must not be fatal either:
		// the recovery path for a typo'd token is the Settings page, which
		// only exists if this process keeps running.
		logger.Errorf("init bot: %v (Telegram disabled, dashboard still available)", err)
		a.Bot = nil
	}

	a.Scheduler = scheduler.New()

	// trade must be assigned only when a.Bot is non-nil: handing a nil
	// *bot.Bot straight to web.Config.Trade would produce a non-nil interface
	// wrapping a nil pointer, which every `s.trade == nil` check in
	// internal/web would then miss (server.go's paperDB comment names the
	// same trap).
	var trade web.TradeExecutor
	if a.Bot != nil {
		trade = a.Bot
	}

	if cfg.WebAddr != "" {
		// In-process, not a subcommand like "mcp": the dashboard needs live
		// quotes (data.Provider) alongside DB reads, and shares this
		// process's *db.DB connection directly rather than opening a
		// second db.OpenReadOnly one — unlike the MCP subprocess, this
		// isn't a separate process, and database/sql connections already
		// support concurrent use from other goroutines.
		a.Web = web.New(web.Config{
			DB:           database,
			Provider:     core.Provider,
			History:      core.Yahoo,
			Earnings:     core.Earnings,
			Lang:         cfg.Lang,
			CompanyNames: core.CompanyNames,
			OptionChain:  core.Yahoo,
			RiskHeatPct:  cfg.RiskHeatPct,
			Password:     cfg.WebPassword,
			Trade:        trade,
			// Phase 17 PR2: the file godotenv.Load() already read at startup
			// — not configurable, since editing any other file would have no
			// effect on the next boot.
			EnvPath:             ".env",
			PaperDB:             a.PaperDB,
			PaperInitialCashUSD: cfg.PaperInitialCashUSD,
			PaperInitialCashTWD: cfg.PaperInitialCashTWD,
			Sector:              sectorProvider,
			IndustryMap:         industryMapProvider,
			LLMAudit:            cfg.WebLLMAudit,
			JWTSecret:           cfg.JWTSecret,
			APIKey:              cfg.APIKey,
			Events:              a.Events,
		})
		// Phase 18's sector money-flow scan only has a consumer when the
		// dashboard is running, so it's registered here rather than
		// unconditionally alongside the other jobs below.
		a.Scheduler.AddSectorFlowScan(ctx, func(ctx context.Context) {
			a.Web.RunSectorFlowScan(ctx, market.US)
		})
		a.Scheduler.AddTWSectorFlowScan(ctx, func(ctx context.Context) {
			a.Web.RunSectorFlowScan(ctx, market.TW)
		})
	}

	a.registerJobs(ctx)
	return a, nil
}

// registerJobs registers every cron job on a.Scheduler. The Telegram block is
// registered as one unit rather than nil-guarding twelve closures — every job
// in it reports to Telegram, see docs/phase-17-web-settings.md §3.1. Log
// rotation and backups are deliberately outside it: they're the two jobs that
// still matter on a Telegram-less process.
//
// Known limitation (Stage 3.2, not yet done — see
// docs/architecture/server-refactor-plan.md §4 Step 3.2): every job here
// still computes its result and sends it via Telegram in one method on
// *bot.Bot, so a Telegram-less cmd/server skips all of them, not just the
// Telegram send. The one that bites hardest is AddClosingSnapshot: its real
// job is writing daily_snapshots/benchmark rows, which the dashboard's whole
// P&L curve replays — a Telegram-less server silently accumulates no history
// and the dashboard just looks broken. AddUniverseScan (scan_hits) has the
// same problem. Fixing this means splitting each *Bot method into a
// compute-and-persist half callable without Telegram and a
// format-and-send half, job by job — deferred to a follow-up PR, not done
// here.
func (a *App) registerJobs(ctx context.Context) {
	if b := a.Bot; b != nil {
		a.Scheduler.AddDailyReport(ctx, func(ctx context.Context) {
			b.RunDailyReport(ctx)
		})
		a.Scheduler.AddMorningBriefing(ctx, func(ctx context.Context) {
			b.RunUSMorningBriefing(ctx)
		})
		a.Scheduler.AddTWDailyReport(ctx, func(ctx context.Context) {
			b.RunTWDailyReport(ctx)
		})
		a.Scheduler.AddTWMorningBriefing(ctx, func(ctx context.Context) {
			b.RunTWMorningBriefing(ctx)
		})
		a.Scheduler.AddClosingSnapshot(ctx, func(ctx context.Context) {
			b.RunClosingSnapshot(ctx, market.US)
		})
		a.Scheduler.AddTWClosingSnapshot(ctx, func(ctx context.Context) {
			b.RunClosingSnapshot(ctx, market.TW)
		})
		a.Scheduler.AddWeeklyReview(ctx, func(ctx context.Context) {
			b.RunWeeklyReview(ctx)
		})
		a.Scheduler.AddSinopacSync(ctx, func(ctx context.Context) {
			if a.cfg.ShioajiAddr == "" {
				return
			}
			if msg, found := b.RunSinopacSync(ctx, !a.cfg.SinopacSyncLive); found {
				b.Send(msg)
			}
		})
		a.Scheduler.AddMonthlyReport(ctx, func(ctx context.Context) {
			b.RunMonthlyReport(ctx)
		})
	}

	// The universe scan is Phase 24 Stage 3 Step 3.2's first fully inverted
	// job: the scheduler calls a service and gets a DTO back, with no *Bot in
	// the path at all. It's outside the Telegram block above on purpose — the
	// job has never sent a Telegram message (its output is scan_hits rows the
	// daily report reads later), so there was never a reason for it to be
	// gated on Telegram being configured, only an accident of where it lived.
	a.Scheduler.AddUniverseScan(ctx, func(ctx context.Context) {
		a.runScan(ctx, market.US)
	})
	a.Scheduler.AddTWUniverseScan(ctx, func(ctx context.Context) {
		a.runScan(ctx, market.TW)
	})

	a.Scheduler.AddLogRotation(func() {
		if err := a.logFile.Rotate(); err != nil {
			logger.Errorf("log rotation: %v", err)
		}
	})
	a.Scheduler.AddBackup(func() {
		runBackup(a.DB, a.cfg.BackupDir, a.cfg.BackupRetentionDays, "argus")
		if a.PaperDB != nil {
			runBackup(a.PaperDB, a.cfg.BackupDir, a.cfg.BackupRetentionDays, "argus-paper")
		}
	})
}

// runScan is the universe scan's scheduler entry point: call the service,
// log the DTO. recoverJob replaces the *Bot method that used to guard this
// (bot.recoverJobPanic) — same log line, same job_panic alert, published
// through the process-wide Dispatcher instead of through the bot.
func (a *App) runScan(ctx context.Context, m market.MarketID) {
	defer a.recoverJob("universe scan")
	res, err := a.Scan.RunUniverseScan(ctx, m)
	if err != nil && !errors.Is(err, context.Canceled) {
		logger.Errorf("universe scan: market=%s: %v", m, err)
		return
	}
	if !res.Skipped {
		logger.Infof("universe scan: market=%s checked %d tickers, %d hits", m, res.Scanned, res.Hits)
	}
}

// recoverJob recovers from a panic inside a scheduler-invoked job, logging
// it and alerting the user. Without this, a panic in a job would kill that
// goroutine silently — the process keeps running and answering requests, but
// the VPS is unattended, so a failed job would otherwise go completely
// unnoticed. Deferred by the job wrapper, not by the service: a service
// shouldn't have to know it's being run by cron.
func (a *App) recoverJob(job string) {
	if r := recover(); r != nil {
		logger.Errorf("%s: panic: %v", job, r)
		a.Notifier.Publish(context.Background(), notification.Event{
			Type:  "job_panic",
			Text:  i18n.T(a.cfg.Lang, i18n.KeyJobPanic, job, r),
			Level: notification.LevelCritical,
		})
	}
}

// Run starts the HTTP server, the cron scheduler and (when configured) the
// Telegram long-poll loop, then blocks until ctx is cancelled and everything
// it started has stopped. Shutdown order is HTTP → Telegram → scheduler:
// web.Server.Run and bot.Bot.Run both return on ctx cancellation on their
// own, so the WaitGroup below *is* the ordering guarantee that no request or
// command handler is still touching the DB when Close runs.
//
// A local, cancellable ctx (not the caller's directly) is what a listener
// failure needs to actually stop the process: cmd/server's HTTP listener is
// the reason the process exists, so if it fails to bind, staying up with
// Telegram/cron still running would leave a supervisor believing the process
// is healthy while its port serves nothing and never gets retried.
func (a *App) Run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	a.Scheduler.Start()

	var wg sync.WaitGroup
	if a.Web != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := a.Web.Run(ctx, a.cfg.WebAddr); err != nil {
				logger.Errorf("web: server error: %v", err)
				cancel()
			}
		}()
	}
	if a.Bot != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a.Bot.Run(ctx)
		}()
	}

	<-ctx.Done()
	wg.Wait()
	a.Scheduler.Stop()
}

// Close releases what Boot opened. AGENTS.md's "key behaviors" only requires
// that llmClient.Close() gets called before the process exits — not that it
// go last — so this ordering (DBs, then LLM, then the log file) is a choice
// made here, not one the doc mandates; llm.Client never touches the DB
// itself (the MCP tool surface is a separate process with its own
// connection), so there's no correctness reason to order it any differently.
func (a *App) Close() {
	if a.PaperDB != nil {
		a.PaperDB.Close()
	}
	if a.DB != nil {
		a.DB.Close()
	}
	if a.LLM != nil {
		a.LLM.Close()
	}
	if a.logFile != nil {
		a.logFile.Close()
	}
}

// fundamentalsReader adapts a data.FundamentalsProvider to the plain function
// ScanConfig wants, staying nil when there's no provider — the gate then
// fails closed, same as the pre-extraction nil-check in internal/bot.
func fundamentalsReader(p data.FundamentalsProvider) func(string) (*data.Fundamentals, error) {
	if p == nil {
		return nil
	}
	return p.GetFundamentals
}
