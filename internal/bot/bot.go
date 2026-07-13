package bot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/llm"
	"argus/internal/signals"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
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
)

type Bot struct {
	api           *tgbotapi.BotAPI
	db            *db.DB
	provider      data.Provider
	fundamentals  data.FundamentalsProvider  // nil if FINNHUB_API_KEY isn't set
	analystRating data.AnalystRatingProvider // nil if FINNHUB_API_KEY isn't set
	earnings      data.EarningsProvider      // nil if FINNHUB_API_KEY isn't set
	marketNews    data.MarketNewsProvider    // nil if FINNHUB_API_KEY isn't set
	history       data.HistoryProvider
	llm           *llm.Client
	detector      *signals.Detector
	chatID        int64
	lang          i18n.Lang

	// stopLossPct/trailingStopPct (STOP_LOSS_PCT/TRAILING_STOP_PCT env,
	// Phase 3.8) are positive percentage thresholds for RunDailyReport's
	// rule-based exit-discipline checks (checkStopLossAlerts/
	// checkTrailingStopAlerts) — 0 disables the corresponding check entirely.
	stopLossPct     float64
	trailingStopPct float64

	// chatQueue feeds chatWorker, which answers plain-text messages one at a
	// time and in the order they arrived — unlike commands, chat shares one
	// persistent LLM session, so processing it concurrently could let a
	// later message reach that conversation history before an earlier one.
	// 32 is generous slack for a single-user bot; if it ever fills up, the
	// user has 32 unanswered messages backlogged, and blocking is the right
	// thing to do at that point anyway.
	chatQueue chan *tgbotapi.Message
}

// Config bundles New's construction inputs. Replaces New's former 12
// positional parameters (grown by one or two per phase — stopLossPct/
// trailingStopPct most recently), which had gotten hard to read at call
// sites and error-prone to extend — see docs/refactor-internal-bot.md. This
// is deliberately not the 架構債 "設定整理" item (centralized env parsing/
// validation in main.go); it only fixes this constructor's signature.
type Config struct {
	Token           string
	ChatID          int64
	DB              *db.DB
	Provider        data.Provider
	Fundamentals    data.FundamentalsProvider  // nil if FINNHUB_API_KEY isn't set
	AnalystRating   data.AnalystRatingProvider // nil if FINNHUB_API_KEY isn't set
	Earnings        data.EarningsProvider      // nil if FINNHUB_API_KEY isn't set
	MarketNews      data.MarketNewsProvider    // nil if FINNHUB_API_KEY isn't set
	History         data.HistoryProvider
	LLM             *llm.Client
	Lang            i18n.Lang
	StopLossPct     float64 // STOP_LOSS_PCT env; 0 disables the check
	TrailingStopPct float64 // TRAILING_STOP_PCT env; 0 disables the check
}

func New(cfg Config) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil {
		return nil, fmt.Errorf("telegram: %w", err)
	}
	log.Printf("Telegram bot authorized: @%s", api.Self.UserName)
	return &Bot{
		api:             api,
		db:              cfg.DB,
		provider:        cfg.Provider,
		fundamentals:    cfg.Fundamentals,
		analystRating:   cfg.AnalystRating,
		earnings:        cfg.Earnings,
		marketNews:      cfg.MarketNews,
		history:         cfg.History,
		llm:             cfg.LLM,
		detector:        signals.NewDetector(cfg.Lang),
		chatID:          cfg.ChatID,
		lang:            cfg.Lang,
		stopLossPct:     cfg.StopLossPct,
		trailingStopPct: cfg.TrailingStopPct,
		chatQueue:       make(chan *tgbotapi.Message, 32),
	}, nil
}

func (b *Bot) Send(text string) {
	msg := tgbotapi.NewMessage(b.chatID, text)
	msg.ParseMode = "Markdown"
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("send error: %v", err)
	}
}

func (b *Bot) Run(ctx context.Context) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := b.api.GetUpdatesChan(u)

	go b.chatWorker(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case update, ok := <-updates:
			if !ok {
				return
			}
			if update.Message == nil {
				continue
			}
			b.dispatch(ctx, update.Message)
		}
	}
}

// dispatch routes an incoming message. Commands are independent one-shot
// requests, so each runs in its own goroutine and a slow one (e.g.
// /recommend) doesn't block a quick one (e.g. /status) sent right after it.
// Plain-text chat messages instead go on chatQueue, so chatWorker answers
// them one at a time in arrival order — see the chatQueue field comment.
func (b *Bot) dispatch(ctx context.Context, msg *tgbotapi.Message) {
	if msg.Command() != "" {
		go b.handleMessage(ctx, msg)
		return
	}
	if strings.TrimSpace(msg.Text) == "" {
		return
	}
	select {
	case b.chatQueue <- msg:
	case <-ctx.Done():
	}
}

// chatWorker drains chatQueue on a single goroutine so chat replies stay in
// the order the user sent them, sharing the LLM client's one persistent
// session.
func (b *Bot) chatWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-b.chatQueue:
			b.handleChat(ctx, strings.TrimSpace(msg.Text))
		}
	}
}

func (b *Bot) handleMessage(ctx context.Context, msg *tgbotapi.Message) {
	cmd := msg.Command()
	args := strings.TrimSpace(msg.CommandArguments())

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
		b.handleRecommend(ctx)
	case "check":
		b.handleCheck(ctx, args)
	case "track":
		b.handleTrack(args)
	case "buy":
		b.handleBuy(args)
	case "sell":
		b.handleSell(args)
	case "portfolio":
		b.handlePortfolio()
	case "insight":
		b.handleInsight(ctx)
	case "cash":
		b.handleCash(args)
	case "dailyreport":
		b.RunDailyReport(ctx)
	case "fundamentals":
		b.handleFundamentals(args)
	case "universe":
		b.handleUniverse(args)
	case "thesis":
		b.handleThesis(args)
	case "reset":
		b.handleReset()
	default:
		b.Send(i18n.T(b.lang, i18n.KeyUnknownCommand))
	}
}
