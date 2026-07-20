package bot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode/utf8"

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

	// vixTicker feeds computeMarketRegime's VIX line (Phase 3.7 追加項, see
	// docs/phase-3.7-market-regime.md) via the ordinary quote path
	// (b.provider.GetQuote) — not a history/watchlist ticker, same as
	// benchmarkTicker never touching the watchlist table.
	vixTicker = "^VIX"
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
	Token               string
	ChatID              int64
	DB                  *db.DB
	Provider            data.Provider
	Fundamentals        data.FundamentalsProvider  // nil if FINNHUB_API_KEY isn't set
	AnalystRating       data.AnalystRatingProvider // nil if FINNHUB_API_KEY isn't set
	Earnings            data.EarningsProvider      // nil if FINNHUB_API_KEY isn't set
	MarketNews          data.MarketNewsProvider    // nil if FINNHUB_API_KEY isn't set
	History             data.HistoryProvider
	LLM                 *llm.Client
	Lang                i18n.Lang
	StopLossPct         float64 // STOP_LOSS_PCT env; 0 disables the check
	TrailingStopPct     float64 // TRAILING_STOP_PCT env; 0 disables the check
	TrailingStopATRMult float64 // TRAILING_STOP_ATR_MULT env; <= 0 disables the ATR-based distance
	RiskPctPerTrade     float64 // RISK_PCT_PER_TRADE env; <= 0 disables the sizing suggestion
}

func New(cfg Config) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil {
		return nil, fmt.Errorf("telegram: %w", err)
	}
	log.Printf("Telegram bot authorized: @%s", api.Self.UserName)
	return &Bot{
		api:                 api,
		db:                  cfg.DB,
		provider:            cfg.Provider,
		fundamentals:        cfg.Fundamentals,
		analystRating:       cfg.AnalystRating,
		earnings:            cfg.Earnings,
		marketNews:          cfg.MarketNews,
		history:             cfg.History,
		llm:                 cfg.LLM,
		detector:            signals.NewDetector(cfg.Lang),
		chatID:              cfg.ChatID,
		lang:                cfg.Lang,
		stopLossPct:         cfg.StopLossPct,
		trailingStopPct:     cfg.TrailingStopPct,
		trailingStopATRMult: cfg.TrailingStopATRMult,
		riskPctPerTrade:     cfg.RiskPctPerTrade,
		chatQueue:           make(chan *tgbotapi.Message, 32),
	}, nil
}

// telegramMaxMessageLen is a conservative cap on outgoing message length.
// Telegram's actual sendMessage limit is 4096 characters; this project stays
// well under it since splitMessage counts runes (not the UTF-16 code units
// Telegram's limit is really specified in — astral-plane emoji like 📊 need
// two of those per rune) and BotAPI.Send returns "message is too long" as a
// plain error that Send only logs, never surfaces to the user. /track is the
// command most likely to hit this: its length grows with
// watchlist-size × lookback-days (see handleTrack), so even a modest
// watchlist can produce a multi-thousand-character report after a week of
// daily reports.
const telegramMaxMessageLen = 3500

func (b *Bot) Send(text string) {
	for _, chunk := range splitMessage(text, telegramMaxMessageLen) {
		msg := tgbotapi.NewMessage(b.chatID, chunk)
		msg.ParseMode = "Markdown"
		if _, err := b.api.Send(msg); err != nil {
			log.Printf("send error: %v", err)
		}
	}
}

// splitMessage breaks text into chunks of at most limit runes, splitting
// only at line boundaries so a Markdown entity opened and closed within a
// single line (e.g. "*AAPL*") never gets split across two messages — every
// i18n line template in this package opens and closes its own markdown
// within one line, so this preserves valid Markdown per chunk. A single line
// longer than limit on its own (shouldn't happen with today's templates) is
// hard-split by rune as a last resort, so content is never silently dropped.
func splitMessage(text string, limit int) []string {
	if utf8.RuneCountInString(text) <= limit {
		return []string{text}
	}

	var chunks []string
	var current strings.Builder
	currentLen := 0
	flush := func() {
		if current.Len() > 0 {
			chunks = append(chunks, current.String())
			current.Reset()
			currentLen = 0
		}
	}

	for _, line := range strings.SplitAfter(text, "\n") {
		if line == "" {
			continue
		}
		lineLen := utf8.RuneCountInString(line)
		if lineLen > limit {
			flush()
			runes := []rune(line)
			for len(runes) > 0 {
				n := limit
				if n > len(runes) {
					n = len(runes)
				}
				chunks = append(chunks, string(runes[:n]))
				runes = runes[n:]
			}
			continue
		}
		if currentLen+lineLen > limit {
			flush()
		}
		current.WriteString(line)
		currentLen += lineLen
	}
	flush()
	return chunks
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
			if update.CallbackQuery != nil {
				go b.handleCallbackQuery(ctx, update.CallbackQuery)
				continue
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
