package main

import (
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"

	"argus/internal/i18n"
	"argus/internal/logger"
)

// Config holds every env-var-derived setting the Telegram bot subcommand
// (main(), not "mcp"/"eval"/"backtest" — see their own smaller env reads)
// needs before it starts wiring up providers/bot/web. loadConfig is the only
// place that should call os.Getenv for these; everything downstream just
// reads the struct.
type Config struct {
	// TelegramToken/ChatID are optional (Phase 17 PR1, see
	// docs/phase-17-web-settings.md §3): an unset or unparseable pair leaves
	// them zero-valued and main.go simply never constructs the bot, rather
	// than the whole process dying before the web dashboard can come up.
	// That's the whole point — the dashboard's Settings page is where these
	// get filled in, so it can't be downstream of them being set already.
	TelegramToken string
	ChatID        int64
	FinnhubKey    string
	FinMindToken  string
	// SECUserAgent (Phase 23 PR6) gates the SEC EDGAR fundamentals source,
	// same presence-of-config convention as FinnhubKey — empty disables it
	// entirely rather than falling back to some default UA, since SEC's edge
	// filter requires the UA to look like a real contact email (see
	// internal/data/sec.go's doc comment) and there's no safe default for
	// that.
	SECUserAgent string
	// ShioajiAddr (Phase 16, internal/sinopac) points at the local Shioaji
	// gRPC daemon — empty disables the Sinopac provider/sync entirely, same
	// presence-of-config convention as FinnhubKey.
	ShioajiAddr string
	// SinopacSkip (SINOPAC_SKIP_TICKERS) lists periodic-investment tickers
	// the Shioaji sync should never propose — see parseSinopacSkip.
	SinopacSkip map[string]bool
	// SinopacSyncLive (SINOPAC_SYNC_LIVE) opts the daily sync into writing
	// trades directly instead of going through pending_actions confirmation.
	SinopacSyncLive bool

	RecommendModel string
	CheckModel     string
	ChatModel      string

	DBPath string
	Lang   i18n.Lang

	// WebAddr gates the read-only dashboard (Phase 5 PR1, see
	// docs/phase-5-web-dashboard.md) — empty means off, same
	// presence-of-config convention as FinnhubKey, not envOr's give-a-default
	// shape, since binding a port at all is the thing that needs to be opted
	// into.
	WebAddr string
	// WebPassword (Phase 10, docs/phase-10-web-trade-input.md §4.1) gates the
	// dashboard's write endpoints — empty disables writes entirely, same
	// presence-of-config convention as WebAddr itself. Meaningless without
	// WebAddr also set, but no extra validation beyond that: an unreachable
	// server's password setting is simply inert.
	WebPassword string
	// WebLLMAudit (WEB_LLM_AUDIT, Phase 19 PR1, docs/phase-19-llm-transparency.md
	// §8.4) gates the /llm audit-trail page's API routes — llm_runs is still
	// written on every GenerateRecommendations call regardless (that's the
	// audit trail's whole point), this only controls whether anything can
	// read it back over HTTP, checked at route-registration time rather than
	// request time like Paper/Options' feature flags.
	WebLLMAudit bool

	// Phase 3.8 exit-discipline thresholds: positive percentages, 0 disables
	// the corresponding daily-report check entirely. Defaults are a starting
	// point, not backed by any backtest yet — see PLAN.md's Phase 3.8 note
	// that /track is the intended feedback loop for tuning these later.
	StopLossPct     float64
	TrailingStopPct float64
	// StopLossPctTW/TrailingStopPctTW (Phase 13 §7) default to TW's wider
	// calibration (a single ±10% daily-limit move shouldn't itself trigger
	// the US-calibrated 10%/15% thresholds above) — 0 is reserved as "not
	// set" (bot.NewWithChannel falls back to StopLossPct/TrailingStopPct when
	// 0), so an operator who explicitly wants TW's check disabled must still
	// go through StopLossPct/TrailingStopPct's own STOP_LOSS_PCT=0 today;
	// disabling only TW independently isn't supported yet.
	StopLossPctTW     float64
	TrailingStopPctTW float64
	// TrailingStopATRMult defaults to 0 (disabled): opt-in, not
	// presence-gated, since it changes when a real-money alert fires — see
	// docs/phase-3.8-atr-trailing-stop.md.
	TrailingStopATRMult float64
	// RiskPctPerTrade (Phase 3.11 PR1) defaults to 0 (disabled): sizing
	// suggestions change what a BUY recommendation tells the user to do with
	// real money, so — same reasoning as TrailingStopATRMult above — it's
	// opt-in rather than shipped with a default like StopLossPct/
	// TrailingStopPct's 10/15.
	RiskPctPerTrade float64
	// TWFeeDiscount (Phase 13 §3) defaults to 1.0 (no discount) so an
	// unconfigured deployment's TW fee auto-calc matches the statutory rate;
	// common e-brokerage plans are 0.6/0.28.
	TWFeeDiscount float64
	// RiskHeatPct (Phase 8 PR1) is the /risk page's portfolio-heat warning
	// threshold — defaults to 6.0 (a textbook-common figure, not backtested)
	// when unset; <=0 disables the frontend's warning line entirely, same
	// "<=0 means disabled" convention as StopLossPct/TrailingStopPct.
	RiskHeatPct float64

	// PaperDBPath (Phase 11 PR3, docs/phase-11-paper-account.md §6.6) gates
	// the live paper account — empty disables it entirely, same
	// presence-of-config convention as WebAddr/FinnhubKey. The other three
	// Paper* fields are meaningless without it and simply inert if set alone.
	PaperDBPath         string
	PaperInitialCashUSD float64
	PaperInitialCashTWD float64
	PaperMaxPositionPct float64
	// PaperTakeProfitATRMult (2026-08-08 strategy tuning,
	// docs/phase-11-paper-strategy-tuning.md §3) — <=0 (the default) disables
	// take-profit entirely, same convention as TrailingStopATRMult.
	PaperTakeProfitATRMult float64

	LogPath             string
	BackupDir           string
	BackupRetentionDays int
}

// loadConfig reads .env (if present) and every env var main()'s bot
// subcommand needs, applying defaults/validation. Nothing here is fatal
// any more (Phase 17 PR1): a missing/unparseable TELEGRAM_CHAT_ID logs and
// leaves Telegram off instead of killing the process, so a deployment with
// only WEB_ADDR/WEB_PASSWORD set still boots far enough to serve the
// Settings page that configures the rest.
func loadConfig() Config {
	envErr := godotenv.Load()
	logger.Configure(os.Stderr, logger.ParseLevel(os.Getenv("LOG_LEVEL")))
	if envErr != nil {
		logger.Info("no .env file found, reading env from environment")
	}

	// An empty TELEGRAM_CHAT_ID is a legitimate "Telegram not configured
	// yet" state and stays silent; a non-empty unparseable one is a typo
	// worth an error line, since the symptom (bot never starts) is otherwise
	// indistinguishable from not having set it at all.
	var chatID int64
	if raw := os.Getenv("TELEGRAM_CHAT_ID"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			logger.Errorf("invalid TELEGRAM_CHAT_ID %q: Telegram stays disabled until it's a number", raw)
		}
		chatID = parsed
	}

	return Config{
		TelegramToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		ChatID:        chatID,
		FinnhubKey:    os.Getenv("FINNHUB_API_KEY"),
		FinMindToken:  os.Getenv("FINMIND_TOKEN"),
		SECUserAgent:  os.Getenv("SEC_USER_AGENT"),

		ShioajiAddr:     os.Getenv("SHIOAJI_ADDR"),
		SinopacSkip:     parseSinopacSkip(os.Getenv("SINOPAC_SKIP_TICKERS")),
		SinopacSyncLive: os.Getenv("SINOPAC_SYNC_LIVE") == "true",

		RecommendModel: envOr("CLAUDE_RECOMMEND_MODEL", "opus"),
		CheckModel:     envOr("CLAUDE_CHECK_MODEL", "sonnet"),
		ChatModel:      envOr("CLAUDE_CHAT_MODEL", "sonnet"),

		DBPath: envOr("DB_PATH", "data/argus.db"),
		Lang:   i18n.Parse(envOr("BOT_LANGUAGE", "zh")),

		WebAddr:     os.Getenv("WEB_ADDR"),
		WebPassword: os.Getenv("WEB_PASSWORD"),
		WebLLMAudit: os.Getenv("WEB_LLM_AUDIT") == "true",

		StopLossPct:         envOrFloat("STOP_LOSS_PCT", 10),
		TrailingStopPct:     envOrFloat("TRAILING_STOP_PCT", 15),
		StopLossPctTW:       envOrFloat("STOP_LOSS_PCT_TW", 12),
		TrailingStopPctTW:   envOrFloat("TRAILING_STOP_PCT_TW", 18),
		TrailingStopATRMult: envOrFloat("TRAILING_STOP_ATR_MULT", 0),
		RiskPctPerTrade:     envOrFloat("RISK_PCT_PER_TRADE", 0),
		TWFeeDiscount:       envOrFloat("TW_BROKER_FEE_DISCOUNT", 1.0),
		RiskHeatPct:         envOrFloat("RISK_HEAT_PCT", 6.0),

		PaperDBPath:            os.Getenv("PAPER_DB_PATH"),
		PaperInitialCashUSD:    envOrFloat("PAPER_INITIAL_CASH_USD", 100000),
		PaperInitialCashTWD:    envOrFloat("PAPER_INITIAL_CASH_TWD", 1000000),
		PaperMaxPositionPct:    envOrFloat("PAPER_MAX_POSITION_PCT", 25),
		PaperTakeProfitATRMult: envOrFloat("PAPER_TAKE_PROFIT_ATR_MULT", 0),

		LogPath:             envOr("LOG_PATH", "data/argus.log"),
		BackupDir:           envOr("BACKUP_DIR", "data/backups"),
		BackupRetentionDays: envOrInt("BACKUP_RETENTION_DAYS", 14),
	}
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
		logger.Warnf("invalid %s=%q, using default %v", key, v, fallback)
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
		logger.Warnf("invalid %s=%q, using default %d", key, v, fallback)
		return fallback
	}
	return n
}

// parseSinopacSkip splits SINOPAC_SKIP_TICKERS (comma-separated, e.g.
// "0050,006208") into a lookup set for internal/sinopac.Trades — periodic
// investment tickers the Shioaji sync should never propose. nil (not an
// empty map) when unset, same "absent means nothing to check" shape as
// every other optional bot.Config field.
func parseSinopacSkip(raw string) map[string]bool {
	if raw == "" {
		return nil
	}
	out := make(map[string]bool)
	for _, t := range strings.Split(raw, ",") {
		if t = strings.TrimSpace(t); t != "" {
			out[t] = true
		}
	}
	return out
}
