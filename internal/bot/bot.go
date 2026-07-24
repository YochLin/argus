package bot

import (
	"context"
	"strings"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/llm"
	"argus/internal/signals"
)

var cst = time.FixedZone("CST", 8*3600)

// earningsPromptWindowDays bounds how far ahead we look for upcoming
// earnings when feeding it into LLM prompts — wide enough to matter for a
// low-frequency trader deciding whether to enter a position now or wait.
// earningsAlertDays is the (narrower) window for the proactive Telegram
// reminder, close enough to actually be actionable that week.
const (
	earningsPromptWindowDays = 14
	earningsAlertDays        = 3
	earningsSignalFamily     = "earnings"

	// marketNewsLimit is double the per-ticker news limit (5, see
	// fetchStockData) since it covers the whole market rather than one name.
	marketNewsLimit = 10

	// benchmarkTicker is snapshotted alongside the watchlist on every closing
	// snapshot (see snapshotBenchmark) so /track can compare a recommendation
	// against the same-period broad-market move instead of just its own
	// absolute direction. It's intentionally not added to the watchlist table
	// — it's not a holding, and /list shouldn't show it.
	benchmarkTicker = "SPY"

	// vixTicker feeds computeMarketRegime's VIX line (Phase 3.7 追加項, see
	// docs/phase-3.7-market-regime.md) via the ordinary quote path
	// (b.provider.GetQuote) — not a history/watchlist ticker, same as
	// benchmarkTicker never touching the watchlist table.
	vixTicker = "^VIX"
)

type Bot struct {
	channel       Channel
	db            *db.DB
	provider      data.Provider
	fundamentals  data.FundamentalsProvider  // nil if FINNHUB_API_KEY isn't set
	analystRating data.AnalystRatingProvider // nil if FINNHUB_API_KEY isn't set
	earnings      data.EarningsProvider      // nil if FINNHUB_API_KEY isn't set
	marketNews    data.MarketNewsProvider    // nil if FINNHUB_API_KEY isn't set
	companyNames  data.CompanyNameProvider   // nil if FINMIND_TOKEN isn't set
	history       data.HistoryProvider
	llm           *llm.Client
	detector      *signals.Detector
	lang          i18n.Lang

	// stopLossPct/trailingStopPct (STOP_LOSS_PCT/TRAILING_STOP_PCT env,
	// Phase 3.8) are positive percentage thresholds for RunDailyReport's
	// rule-based exit-discipline checks (checkStopLossAlerts/
	// checkTrailingStopAlerts) — 0 disables the corresponding check entirely.
	// trailingStopATRMult (TRAILING_STOP_ATR_MULT env, Phase 3.8 追加項) is an
	// opt-in ATR(14) multiplier that tightens the trailing-stop distance for
	// lower-volatility tickers — see trailingStopThreshold and
	// docs/phase-3.8-atr-trailing-stop.md; <= 0 (the default) leaves
	// trailingStopPct as the sole threshold, unchanged from before this field
	// existed.
	stopLossPct         float64
	trailingStopPct     float64
	trailingStopATRMult float64

	// riskPctPerTrade (RISK_PCT_PER_TRADE env, Phase 3.11 PR1) is the max
	// account-value percentage a single new BUY should risk, used by
	// buildSizingLines to compute a suggested share count — <= 0 (the
	// default) disables the whole feature, same convention as
	// stopLossPct/trailingStopPct.
	riskPctPerTrade float64

	// chatQueue feeds chatWorker, which answers plain-text messages one at a
	// time and in the order they arrived — unlike commands, chat shares one
	// persistent LLM session, so processing it concurrently could let a
	// later message reach that conversation history before an earlier one.
	// 32 is generous slack for a single-user bot; if it ever fills up, the
	// user has 32 unanswered messages backlogged, and blocking is the right
	// thing to do at that point anyway. Holds trimmed message text — dispatch
	// does the trimming once, before enqueueing.
	chatQueue chan string

	// now returns the current time; RunDailyReport's market.IsTradingDay
	// guard reads through this instead of calling time.Now() directly so a
	// test can pin it to a known trading (or holiday/weekend) date instead
	// of depending on whatever real date it happens to run on — otherwise
	// an E2E test exercising RunDailyReport's full path would intermittently
	// take the "market closed" short-circuit whenever CI ran on an actual
	// weekend. New sets this to time.Now; nil is only safe here because no
	// other command/job in this package calls it.
	now func() time.Time
}

// Config bundles New's construction inputs. Replaces New's former 12
// positional parameters (grown by one or two per phase — stopLossPct/
// trailingStopPct most recently), which had gotten hard to read at call
// sites and error-prone to extend — see docs/refactor-internal-bot.md. This
// is deliberately not the 架構債 "設定整理" item (centralized env parsing/
// validation in main.go); it only fixes this constructor's signature.
type Config struct {
	Token               string
	ChatID              int64
	DB                  *db.DB
	Provider            data.Provider
	Fundamentals        data.FundamentalsProvider  // nil if FINNHUB_API_KEY isn't set
	AnalystRating       data.AnalystRatingProvider // nil if FINNHUB_API_KEY isn't set
	Earnings            data.EarningsProvider      // nil if FINNHUB_API_KEY isn't set
	MarketNews          data.MarketNewsProvider    // nil if FINNHUB_API_KEY isn't set
	CompanyNames        data.CompanyNameProvider   // nil if FINMIND_TOKEN isn't set
	History             data.HistoryProvider
	LLM                 *llm.Client
	Lang                i18n.Lang
	StopLossPct         float64 // STOP_LOSS_PCT env; 0 disables the check
	TrailingStopPct     float64 // TRAILING_STOP_PCT env; 0 disables the check
	TrailingStopATRMult float64 // TRAILING_STOP_ATR_MULT env; <= 0 disables the ATR-based distance
	RiskPctPerTrade     float64 // RISK_PCT_PER_TRADE env; <= 0 disables the sizing suggestion

	// APIEndpoint overrides tgbotapi's default https://api.telegram.org
	// target — empty (the only value any real deployment ever sets) keeps
	// tgbotapi.NewBotAPI's normal behavior. Its only purpose today is
	// pointing a test's *Bot at an httptest fake Telegram server (see the
	// daily-report E2E test) instead of making a real network call.
	APIEndpoint string
}

func New(cfg Config) (*Bot, error) {
	channel, err := NewTelegramChannel(cfg.Token, cfg.APIEndpoint, cfg.ChatID)
	if err != nil {
		return nil, err
	}
	return NewWithChannel(channel, cfg), nil
}

// NewWithChannel builds a Bot against an already-constructed Channel,
// bypassing New's Telegram-specific construction (NewTelegramChannel) — the
// seam a future second messaging channel (its own package implementing
// Channel, per CLAUDE.md's 訊息通道介面 note) or a test would use instead of
// New. Mirrors internal/llm's NewClient/NewClientWithProvider split for the
// same reason: every real call site still goes through New.
func NewWithChannel(channel Channel, cfg Config) *Bot {
	return &Bot{
		channel:             channel,
		db:                  cfg.DB,
		provider:            cfg.Provider,
		fundamentals:        cfg.Fundamentals,
		analystRating:       cfg.AnalystRating,
		earnings:            cfg.Earnings,
		marketNews:          cfg.MarketNews,
		companyNames:        cfg.CompanyNames,
		history:             cfg.History,
		llm:                 cfg.LLM,
		detector:            signals.NewDetector(cfg.Lang),
		lang:                cfg.Lang,
		stopLossPct:         cfg.StopLossPct,
		trailingStopPct:     cfg.TrailingStopPct,
		trailingStopATRMult: cfg.TrailingStopATRMult,
		riskPctPerTrade:     cfg.RiskPctPerTrade,
		chatQueue:           make(chan string, 32),
		now:                 time.Now,
	}
}

func (b *Bot) Send(text string) {
	b.channel.Send(text)
}

func (b *Bot) Run(ctx context.Context) {
	go b.chatWorker(ctx)

	b.channel.Listen(ctx, func(u Update) {
		if u.Callback != nil {
			go b.handleCallbackQuery(ctx, *u.Callback)
			return
		}
		if u.Message != nil {
			b.dispatch(ctx, u.Message)
		}
	})
}

// dispatch routes an incoming message. Commands are independent one-shot
// requests, so each runs in its own goroutine and a slow one (e.g.
// /recommend) doesn't block a quick one (e.g. /status) sent right after it.
// Plain-text chat messages instead go on chatQueue, so chatWorker answers
// them one at a time in arrival order — see the chatQueue field comment.
func (b *Bot) dispatch(ctx context.Context, msg *InMessage) {
	if cmd, args, ok := parseCommand(msg.Text); ok {
		go b.handleMessage(ctx, cmd, args)
		return
	}
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}
	select {
	case b.chatQueue <- text:
	case <-ctx.Done():
	}
}

// parseCommand splits a channel-agnostic inbound message into a command and
// its arguments, mirroring tgbotapi.Message's Command()/CommandArguments()
// closely enough for this project's actual usage: a leading "/word",
// optionally suffixed "@botname" (Telegram appends this in group chats),
// then everything after the first space as args. Working over plain text
// rather than a specific channel's inbound event type is what lets dispatch
// stay channel-agnostic.
func parseCommand(text string) (cmd, args string, ok bool) {
	if !strings.HasPrefix(text, "/") {
		return "", "", false
	}
	fields := strings.SplitN(text, " ", 2)
	cmd = strings.TrimPrefix(fields[0], "/")
	if at := strings.IndexByte(cmd, '@'); at != -1 {
		cmd = cmd[:at]
	}
	if cmd == "" {
		return "", "", false
	}
	if len(fields) == 2 {
		args = strings.TrimSpace(fields[1])
	}
	return cmd, args, true
}

// chatWorker drains chatQueue on a single goroutine so chat replies stay in
// the order the user sent them, sharing the LLM client's one persistent
// session.
func (b *Bot) chatWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case text := <-b.chatQueue:
			b.handleChat(ctx, text)
		}
	}
}

func (b *Bot) handleMessage(ctx context.Context, cmd, args string) {
	switch cmd {
	case "add":
		b.handleAdd(args)
	case "remove":
		b.handleRemove(args)
	case "list":
		b.handleList()
	case "status":
		b.handleStatus(args)
	case "recommend":
		b.handleRecommend(ctx, args)
	case "check":
		b.handleCheck(ctx, args)
	case "track":
		b.handleTrack(args)
	case "buy":
		b.handleBuy(args)
	case "sell":
		b.handleSell(ctx, args)
	case "stop":
		b.handleStop(args)
	case "portfolio":
		b.handlePortfolio()
	case "insight":
		b.handleInsight(ctx)
	case "cash":
		b.handleCash(args)
	case "dailyreport":
		b.RunDailyReport(ctx)
	case "monthlyreport":
		b.RunMonthlyReport(ctx)
	case "fundamentals":
		b.handleFundamentals(args)
	case "universe":
		b.handleUniverse(args)
	case "thesis":
		b.handleThesis(args)
	case "review":
		b.handleReview(ctx, args)
	case "reset":
		b.handleReset()
	default:
		b.Send(i18n.T(b.lang, i18n.KeyUnknownCommand))
	}
}
