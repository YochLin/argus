package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"gopkg.in/natefinch/lumberjack.v2"
	"argus/internal/bot"
	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/llm"
	"argus/internal/mcptools"
	"argus/internal/scheduler"
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

	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, reading env from environment")
	}

	telegramToken := mustEnv("TELEGRAM_BOT_TOKEN")
	chatIDStr := mustEnv("TELEGRAM_CHAT_ID")
	finnhubKey := os.Getenv("FINNHUB_API_KEY")
	recommendModel := envOr("CLAUDE_RECOMMEND_MODEL", "opus")
	checkModel := envOr("CLAUDE_CHECK_MODEL", "sonnet")
	chatModel := envOr("CLAUDE_CHAT_MODEL", "sonnet")
	dbPath := envOr("DB_PATH", "data/argus.db")
	lang := i18n.Parse(envOr("BOT_LANGUAGE", "zh"))
	// Phase 3.8 exit-discipline thresholds: positive percentages, 0 disables
	// the corresponding daily-report check entirely. Defaults are a starting
	// point, not backed by any backtest yet — see PLAN.md's Phase 3.8 note
	// that /track is the intended feedback loop for tuning these later.
	stopLossPct := envOrFloat("STOP_LOSS_PCT", 10)
	trailingStopPct := envOrFloat("TRAILING_STOP_PCT", 15)

	chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		log.Fatalf("invalid TELEGRAM_CHAT_ID: %v", err)
	}

	// Ensure DB directory exists
	if err := os.MkdirAll("data", 0o755); err != nil {
		log.Fatalf("create data dir: %v", err)
	}

	// Re-export DB_PATH as an absolute path so llm.argusMCPServer (which
	// reads it back via os.Getenv when a chat session spins up the MCP
	// subprocess) sees the right file regardless of DB_PATH's default vs.
	// explicit-in-.env origin. This matters because that subprocess is
	// launched by claude-agent-acp from os.TempDir(), not this process's
	// cwd (see acp_provider.go's startClaudeSession) — a relative path here
	// would resolve against the wrong directory once handed to the child.
	if absDBPath, err := filepath.Abs(dbPath); err != nil {
		log.Printf("warning: could not resolve absolute DB_PATH from %q: %v", dbPath, err)
	} else {
		os.Setenv("DB_PATH", absDBPath)
	}

	// Log to both stdout (visible via `docker logs`/systemd journal) and a
	// daily-rotated file (registered below on the scheduler) so a VPS
	// deployment has something to grep after the fact — lumberjack only
	// rotates on size by itself; MaxAge+MaxBackups here cap it at roughly a
	// week of history so the log can't slowly fill the disk.
	logFile := &lumberjack.Logger{
		Filename:   envOr("LOG_PATH", "data/argus.log"),
		MaxBackups: 7,
		MaxAge:     7,
		Compress:   true,
	}
	defer logFile.Close()
	log.SetOutput(io.MultiWriter(os.Stdout, logFile))

	database, err := db.New(dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer database.Close()

	// Set up multi-provider data layer (Finnhub primary, Yahoo fallback).
	// Fundamentals/financial statements, the earnings calendar, and general
	// market news are all Finnhub-only (Yahoo's fundamentals equivalent
	// requires a crumb/cookie handshake we don't implement, and has no
	// earnings-calendar or general-news-category endpoint at all), so all
	// three providers stay nil when no Finnhub key is configured. Historical
	// closes (for RSI/MACD) go the other way: Finnhub's free tier blocks
	// /stock/candle entirely, so history is Yahoo-only.
	var providers []data.Provider
	var fundamentalsProvider data.FundamentalsProvider
	var earningsProvider data.EarningsProvider
	var marketNewsProvider data.MarketNewsProvider
	if finnhubKey != "" {
		finnhub := data.NewFinnhub(finnhubKey)
		providers = append(providers, finnhub)
		fundamentalsProvider = finnhub
		earningsProvider = finnhub
		marketNewsProvider = finnhub
	}
	yahoo := data.NewYahoo()
	providers = append(providers, yahoo)
	provider := data.NewMulti(providers...)

	llmClient := llm.NewClient(recommendModel, checkModel, chatModel, lang)
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

	telegramBot, err := bot.New(telegramToken, chatID, database, provider, fundamentalsProvider, earningsProvider, marketNewsProvider, yahoo, llmClient, lang, stopLossPct, trailingStopPct)
	if err != nil {
		log.Fatalf("init bot: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	sched := scheduler.New()
	sched.AddDailyReport(ctx, func(ctx context.Context) {
		telegramBot.RunDailyReport(ctx)
	})
	sched.AddClosingSnapshot(ctx, func(ctx context.Context) {
		telegramBot.RunClosingSnapshot(ctx)
	})
	sched.AddUniverseScan(ctx, func(ctx context.Context) {
		telegramBot.RunUniverseScan(ctx)
	})
	sched.AddLogRotation(func() {
		if err := logFile.Rotate(); err != nil {
			log.Printf("log rotation: %v", err)
		}
	})
	backupDir := envOr("BACKUP_DIR", "data/backups")
	backupRetentionDays := envOrInt("BACKUP_RETENTION_DAYS", 14)
	sched.AddBackup(func() {
		runBackup(database, backupDir, backupRetentionDays)
	})
	sched.Start()
	defer sched.Stop()

	log.Println("stock trader bot started")
	telegramBot.Run(ctx)
	log.Println("bot stopped")
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
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, reading env from environment")
	}
	finnhubKey := os.Getenv("FINNHUB_API_KEY")
	lang := i18n.Parse(envOr("BOT_LANGUAGE", "zh"))

	// Same provider construction as main(): Finnhub-only tools
	// (fundamentals/statements/earnings) stay nil without a key, same as
	// Bot.fundamentals — mcptools.NewServer skips registering their tools
	// in that case rather than registering a tool that always fails.
	var providers []data.Provider
	var fundamentalsProvider data.FundamentalsProvider
	var earningsProvider data.EarningsProvider
	if finnhubKey != "" {
		finnhub := data.NewFinnhub(finnhubKey)
		providers = append(providers, finnhub)
		fundamentalsProvider = finnhub
		earningsProvider = finnhub
	}
	yahoo := data.NewYahoo()
	providers = append(providers, yahoo)
	provider := data.NewMulti(providers...)

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
		log.Printf("mcp: open read-only db: %v", err)
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
		log.Printf("mcp: open writable db: %v", err)
		writeDatabase = nil
	} else {
		defer writeDatabase.Close()
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := mcptools.Run(ctx, lang, provider, yahoo, fundamentalsProvider, earningsProvider, database, writeDatabase); err != nil {
		log.Fatalf("mcp server: %v", err)
	}
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required env var %s is not set", key)
	}
	return v
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envOrFloat(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		log.Printf("invalid %s=%q, using default %v", key, v, fallback)
		return fallback
	}
	return n
}

func envOrInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		log.Printf("invalid %s=%q, using default %d", key, v, fallback)
		return fallback
	}
	return n
}

// runBackup writes a dated SQLite backup (via db.Backup's VACUUM INTO) into
// dir and prunes backup files older than retentionDays. transactions/
// positions are irreplaceable personal financial data with no other backup
// path on a single VPS, hence a daily on-disk copy — see PLAN.md.
func runBackup(database *db.DB, dir string, retentionDays int) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("backup: create dir: %v", err)
		return
	}

	dest := filepath.Join(dir, fmt.Sprintf("argus-%s.db", time.Now().In(cst).Format("2006-01-02")))
	if err := database.Backup(dest); err != nil {
		log.Printf("backup: %v", err)
		return
	}
	log.Printf("backup: wrote %s", dest)

	pruneOldBackups(dir, retentionDays)
}

func pruneOldBackups(dir string, retentionDays int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Printf("backup: prune: read dir: %v", err)
		return
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			log.Printf("backup: prune: stat %s: %v", e.Name(), err)
			continue
		}
		if info.ModTime().After(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
			log.Printf("backup: prune: remove %s: %v", e.Name(), err)
		}
	}
}
