package bot

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
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
)

type Bot struct {
	api          *tgbotapi.BotAPI
	db           *db.DB
	provider     data.Provider
	fundamentals data.FundamentalsProvider // nil if FINNHUB_API_KEY isn't set
	earnings     data.EarningsProvider     // nil if FINNHUB_API_KEY isn't set
	history      data.HistoryProvider
	llm          *llm.Client
	detector     *signals.Detector
	chatID       int64
	lang         i18n.Lang

	// chatQueue feeds chatWorker, which answers plain-text messages one at a
	// time and in the order they arrived — unlike commands, chat shares one
	// persistent LLM session, so processing it concurrently could let a
	// later message reach that conversation history before an earlier one.
	// 32 is generous slack for a single-user bot; if it ever fills up, the
	// user has 32 unanswered messages backlogged, and blocking is the right
	// thing to do at that point anyway.
	chatQueue chan *tgbotapi.Message
}

func New(token string, chatID int64, database *db.DB, provider data.Provider, fundamentals data.FundamentalsProvider, earnings data.EarningsProvider, history data.HistoryProvider, llmClient *llm.Client, lang i18n.Lang) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("telegram: %w", err)
	}
	log.Printf("Telegram bot authorized: @%s", api.Self.UserName)
	return &Bot{
		api:          api,
		db:           database,
		provider:     provider,
		fundamentals: fundamentals,
		earnings:     earnings,
		history:      history,
		llm:          llmClient,
		detector:     signals.NewDetector(lang),
		chatID:       chatID,
		lang:         lang,
		chatQueue:    make(chan *tgbotapi.Message, 32),
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
	case "dailyreport":
		b.RunDailyReport(ctx)
	case "fundamentals":
		b.handleFundamentals(args)
	case "universe":
		b.handleUniverse(args)
	case "reset":
		b.handleReset()
	default:
		b.Send(i18n.T(b.lang, i18n.KeyUnknownCommand))
	}
}

func (b *Bot) handleAdd(ticker string) {
	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	if ticker == "" {
		b.Send(i18n.T(b.lang, i18n.KeyAddUsage))
		return
	}
	if err := b.db.AddTicker(ticker); err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyAddFailed, err))
		return
	}
	b.Send(i18n.T(b.lang, i18n.KeyAddSuccess, ticker))
}

func (b *Bot) handleRemove(ticker string) {
	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	if ticker == "" {
		b.Send(i18n.T(b.lang, i18n.KeyRemoveUsage))
		return
	}
	if err := b.db.RemoveTicker(ticker); err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyRemoveFailed, err))
		return
	}
	b.Send(i18n.T(b.lang, i18n.KeyRemoveSuccess, ticker))
}

func (b *Bot) handleList() {
	tickers, err := b.db.GetWatchlist()
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyQueryFailed, err))
		return
	}
	if len(tickers) == 0 {
		b.Send(i18n.T(b.lang, i18n.KeyWatchlistEmptyHint))
		return
	}
	b.Send(i18n.T(b.lang, i18n.KeyWatchlistTitle) + strings.Join(tickers, "\n"))
}

func (b *Bot) handleStatus(ticker string) {
	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	if ticker == "" {
		tickers, err := b.db.GetWatchlist()
		if err != nil {
			b.Send(i18n.T(b.lang, i18n.KeyQueryFailed, err))
			return
		}
		if len(tickers) == 0 {
			b.Send(i18n.T(b.lang, i18n.KeyWatchlistEmpty))
			return
		}
		var sb strings.Builder
		sb.WriteString(i18n.T(b.lang, i18n.KeyMarketStatusTitle))
		for _, t := range tickers {
			q, err := b.provider.GetQuote(t)
			if err != nil {
				sb.WriteString(i18n.T(b.lang, i18n.KeyQuoteUnavailable, t))
				continue
			}
			sb.WriteString(formatQuote(b.lang, q))
			sb.WriteString("\n\n")
		}
		b.Send(sb.String())
		return
	}

	q, err := b.provider.GetQuote(ticker)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyQuoteFailed, ticker, err))
		return
	}
	b.Send(formatQuote(b.lang, q))
}

func (b *Bot) handleRecommend(ctx context.Context) {
	b.Send(i18n.T(b.lang, i18n.KeyAnalyzing))

	tickers, err := b.db.GetWatchlist()
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyWatchlistQueryFailed, err))
		return
	}

	candidateTickers, err := b.provider.GetMarketMovers()
	if err != nil {
		log.Printf("market movers: %v", err)
	}
	scanHits := b.loadScanHits()
	dedupedCandidates := mergeCandidates(candidateTickers, scanHits, tickers)

	positions := b.loadPositions()
	earnings := b.loadEarnings(append(append([]string{}, tickers...), dedupedCandidates...))

	watchlist := b.fetchStockData(tickers, true, positions, earnings, nil)
	candidates := b.fetchStockData(dedupedCandidates, false, positions, earnings, scanHits)

	recs, err := b.llm.GenerateRecommendations(ctx, watchlist, candidates)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyLLMFailed, err))
		return
	}

	if len(recs) == 0 {
		b.Send(i18n.T(b.lang, i18n.KeyNoRecommendations))
		return
	}

	b.sendAndSaveRecommendations(recs, watchlist, candidates)
}

// sendAndSaveRecommendations formats LLM recommendations for Telegram and
// persists them dated today, each with its ticker's current price looked up
// from the already-fetched stock data — /track later compares that stored
// price against the price on review day. Shared by /recommend and
// RunDailyReport, which otherwise mirror each other.
func (b *Bot) sendAndSaveRecommendations(recs []llm.Recommendation, stockLists ...[]llm.StockData) {
	prices := make(map[string]float64)
	for _, list := range stockLists {
		for _, s := range list {
			if s.Quote != nil {
				prices[s.Quote.Ticker] = s.Quote.Price
			}
		}
	}

	var sb strings.Builder
	sb.WriteString(i18n.T(b.lang, i18n.KeyRecommendationsTitle))
	for i, r := range recs {
		if r.Action != "" {
			fmt.Fprintf(&sb, "%d. *%s* — %s\n%s\n\n", i+1, r.Ticker, r.Action, r.Reason)
		} else {
			fmt.Fprintf(&sb, "%d. *%s*\n%s\n\n", i+1, r.Ticker, r.Reason)
		}
	}
	b.Send(sb.String())

	var dbRecs []db.Recommendation
	for _, r := range recs {
		dbRecs = append(dbRecs, db.Recommendation{
			Ticker: r.Ticker,
			Action: r.Action,
			Reason: r.Reason,
			Price:  prices[r.Ticker],
		})
	}
	if err := b.db.SaveRecommendations(todayDate(), dbRecs); err != nil {
		log.Printf("save recommendations: %v", err)
	}
}

func (b *Bot) handleCheck(ctx context.Context, ticker string) {
	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	if ticker == "" {
		b.Send(i18n.T(b.lang, i18n.KeyCheckUsage))
		return
	}

	b.Send(i18n.T(b.lang, i18n.KeyAnalyzingTicker, ticker))

	q, err := b.provider.GetQuote(ticker)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyQuoteFailed, ticker, err))
		return
	}
	news, _ := b.provider.GetNews(ticker, 5)

	stock := llm.StockData{Quote: q, News: news}
	if b.fundamentals != nil {
		if fd, err := b.fundamentals.GetFundamentals(ticker); err != nil {
			log.Printf("fundamentals %s: %v", ticker, err)
		} else {
			stock.Fundamentals = fd
		}
		if st, err := b.fundamentals.GetFinancialStatements(ticker, "annual"); err != nil {
			log.Printf("financial statements %s: %v", ticker, err)
		} else {
			stock.Statement = st
		}
	}

	result, err := b.llm.CheckStock(ctx, stock)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyLLMFailed, err))
		return
	}

	b.Send(i18n.T(b.lang, i18n.KeyCheckResultTitle, ticker, result))
}

// handleTrack reviews recommendations from the past N days (default 7)
// against today's prices, so recommendation quality is verifiable instead of
// write-only. A BUY counts as a hit if the price rose since the
// recommendation, a SELL if it fell; HOLDs are listed but excluded from the
// hit rate. The baseline price is the one stored at recommendation time;
// rows from before that column existed fall back to the ticker's
// daily_snapshots close on the recommendation date, if the post-close job
// captured one.
func (b *Bot) handleTrack(daysArg string) {
	days := 7
	if daysArg != "" {
		n, err := strconv.Atoi(daysArg)
		if err != nil || n < 1 || n > 90 {
			b.Send(i18n.T(b.lang, i18n.KeyTrackUsage))
			return
		}
		days = n
	}

	fromDate := time.Now().In(cst).AddDate(0, 0, -days).Format("2006-01-02")
	recs, err := b.db.GetRecommendationsSince(fromDate)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyQueryFailed, err))
		return
	}
	if len(recs) == 0 {
		b.Send(i18n.T(b.lang, i18n.KeyTrackEmpty, days))
		return
	}

	// One quote per distinct ticker, however often it was recommended.
	quotes := make(map[string]*data.Quote)

	var sb strings.Builder
	sb.WriteString(i18n.T(b.lang, i18n.KeyTrackTitle, days))
	hits, evaluated := 0, 0
	for _, r := range recs {
		action := r.Action
		if action == "" {
			action = "—"
		}

		base := r.Price
		if base == 0 {
			if c, ok, err := b.db.GetSnapshotClose(r.Ticker, r.Date); err == nil && ok {
				base = c
			}
		}
		if base == 0 {
			sb.WriteString(i18n.T(b.lang, i18n.KeyTrackLineNoPrice, r.Date, r.Ticker, action))
			continue
		}

		q, seen := quotes[r.Ticker]
		if !seen {
			var err error
			q, err = b.provider.GetQuote(r.Ticker)
			if err != nil {
				log.Printf("track: quote %s: %v", r.Ticker, err)
				q = nil
			}
			quotes[r.Ticker] = q
		}
		if q == nil {
			sb.WriteString(i18n.T(b.lang, i18n.KeyQuoteUnavailable, r.Ticker))
			continue
		}

		changePct := (q.Price - base) / base * 100
		verdict := ""
		switch r.Action {
		case "BUY":
			evaluated++
			verdict = "❌"
			if changePct > 0 {
				hits++
				verdict = "✅"
			}
		case "SELL":
			evaluated++
			verdict = "❌"
			if changePct < 0 {
				hits++
				verdict = "✅"
			}
		}
		sb.WriteString(i18n.T(b.lang, i18n.KeyTrackLine, r.Date, r.Ticker, action, base, q.Price, changePct, verdict))
	}
	if evaluated > 0 {
		sb.WriteString(i18n.T(b.lang, i18n.KeyTrackSummary, hits, evaluated, float64(hits)/float64(evaluated)*100))
	}
	b.Send(sb.String())
}

// tradeDateRe matches an optional trailing YYYY-MM-DD date argument to
// /buy and /sell, for backdating a trade entered after the fact (e.g.
// migrating cost basis from a broker/spreadsheet) instead of recording it
// under today's date.
var tradeDateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// parseTradeArgs parses the "<ticker> <shares> <price> [fee] [date]"
// arguments shared by /buy and /sell. fee and date are both optional and
// order-independent relative to each other (distinguished by shape: a
// YYYY-MM-DD token is the date, any other numeric token is the fee), so
// "10 200 1.5 2026-01-15" and "10 200 2026-01-15" (no fee) both parse. date
// is returned as "" when omitted; the caller defaults that to today.
func parseTradeArgs(args string) (ticker string, shares, price, fee float64, date string, err error) {
	fields := strings.Fields(args)
	if len(fields) < 3 || len(fields) > 5 {
		return "", 0, 0, 0, "", fmt.Errorf("expected <ticker> <shares> <price> [fee] [date]")
	}
	ticker = strings.ToUpper(fields[0])
	if shares, err = strconv.ParseFloat(fields[1], 64); err != nil || shares <= 0 {
		return "", 0, 0, 0, "", fmt.Errorf("invalid shares %q", fields[1])
	}
	if price, err = strconv.ParseFloat(fields[2], 64); err != nil || price <= 0 {
		return "", 0, 0, 0, "", fmt.Errorf("invalid price %q", fields[2])
	}

	feeSet := false
	for _, f := range fields[3:] {
		if tradeDateRe.MatchString(f) {
			if date != "" {
				return "", 0, 0, 0, "", fmt.Errorf("duplicate date %q", f)
			}
			if _, perr := time.Parse("2006-01-02", f); perr != nil {
				return "", 0, 0, 0, "", fmt.Errorf("invalid date %q", f)
			}
			date = f
			continue
		}
		if feeSet {
			return "", 0, 0, 0, "", fmt.Errorf("unexpected argument %q", f)
		}
		if fee, err = strconv.ParseFloat(f, 64); err != nil || fee < 0 {
			return "", 0, 0, 0, "", fmt.Errorf("invalid fee %q", f)
		}
		feeSet = true
	}
	return ticker, shares, price, fee, date, nil
}

// handleBuy records a purchase and folds it into the ticker's position
// (weighted-average cost). The ticker is also added to the watchlist —
// see the "持倉自動納入 watchlist" PLAN.md item — so a bought position is
// never silently unmonitored. An explicit date backdates the trade (for
// migrating historical cost basis); note weighted-average cost is
// order-independent for buys, but backdated sells should still be entered
// oldest-first so realized P&L is computed against the cost basis as it
// actually stood at the time.
func (b *Bot) handleBuy(args string) {
	ticker, shares, price, fee, date, err := parseTradeArgs(args)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyBuyUsage))
		return
	}
	if date == "" {
		date = todayDate()
	}

	pos, err := b.db.RecordBuy(ticker, shares, price, fee, date)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyBuyFailed, err))
		return
	}
	if err := b.db.AddTicker(ticker); err != nil {
		log.Printf("buy: add %s to watchlist: %v", ticker, err)
	}
	b.Send(i18n.T(b.lang, i18n.KeyBuySuccess, ticker, shares, price, fee, pos.Shares, pos.AvgCost))
}

// handleSell records a sale against an existing position and reports the
// realized P&L. It does not remove the ticker from the watchlist even when
// the position is fully closed out — the user may still want to watch it.
func (b *Bot) handleSell(args string) {
	ticker, shares, price, fee, date, err := parseTradeArgs(args)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeySellUsage))
		return
	}
	if date == "" {
		date = todayDate()
	}

	pos, realizedPnL, err := b.db.RecordSell(ticker, shares, price, fee, date)
	if err != nil {
		switch {
		case errors.Is(err, db.ErrNoPosition):
			b.Send(i18n.T(b.lang, i18n.KeySellNoPosition, ticker))
		case errors.Is(err, db.ErrInsufficientShares):
			b.Send(i18n.T(b.lang, i18n.KeySellInsufficientShares, ticker))
		default:
			b.Send(i18n.T(b.lang, i18n.KeySellFailed, err))
		}
		return
	}
	b.Send(i18n.T(b.lang, i18n.KeySellSuccess, ticker, shares, price, fee, realizedPnL, pos.Shares))
}

// handlePortfolio shows every open position's current market value and
// unrealized P&L against a live quote, plus cumulative realized P&L across
// all past sells.
func (b *Bot) handlePortfolio() {
	positions, err := b.db.GetPositions()
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyQueryFailed, err))
		return
	}
	if len(positions) == 0 {
		b.Send(i18n.T(b.lang, i18n.KeyPortfolioEmpty))
		return
	}

	realizedTotal, err := b.db.GetRealizedPnL()
	if err != nil {
		log.Printf("portfolio: realized pnl: %v", err)
	}

	var sb strings.Builder
	sb.WriteString(i18n.T(b.lang, i18n.KeyPortfolioTitle))
	var totalValue float64
	for _, p := range positions {
		q, err := b.provider.GetQuote(p.Ticker)
		if err != nil {
			sb.WriteString(i18n.T(b.lang, i18n.KeyQuoteUnavailable, p.Ticker))
			continue
		}
		marketValue := p.Shares * q.Price
		unrealized := (q.Price - p.AvgCost) * p.Shares
		unrealizedPct := (q.Price - p.AvgCost) / p.AvgCost * 100
		totalValue += marketValue
		sb.WriteString(i18n.T(b.lang, i18n.KeyPortfolioLine, p.Ticker, p.Shares, p.AvgCost, q.Price, marketValue, unrealized, unrealizedPct))
	}
	sb.WriteString(i18n.T(b.lang, i18n.KeyPortfolioSummary, totalValue, realizedTotal))
	b.Send(sb.String())
}

// handleFundamentals shows raw fundamentals/financial-statement data
// directly, without going through the LLM.
func (b *Bot) handleFundamentals(ticker string) {
	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	if ticker == "" {
		b.Send(i18n.T(b.lang, i18n.KeyFundamentalsUsage))
		return
	}
	if b.fundamentals == nil {
		b.Send(i18n.T(b.lang, i18n.KeyFundamentalsNoKey))
		return
	}

	fd, fdErr := b.fundamentals.GetFundamentals(ticker)
	st, stErr := b.fundamentals.GetFinancialStatements(ticker, "annual")
	if fdErr != nil && stErr != nil {
		b.Send(i18n.T(b.lang, i18n.KeyFundamentalsFailed, ticker, fdErr))
		return
	}

	var sb strings.Builder
	sb.WriteString(i18n.T(b.lang, i18n.KeyFundamentalsTitle, ticker))
	if fd != nil {
		sb.WriteString(formatFundamentals(b.lang, fd))
		sb.WriteString("\n\n")
	}
	if st != nil {
		sb.WriteString(formatFinancialStatement(b.lang, st))
	}
	b.Send(sb.String())
}

// handleUniverse manages Phase 2.6's candidate scan pool. With no
// arguments it shows a count summary (never the full ~500-ticker list,
// which would blow past Telegram's message size for no reason); "add"/
// "remove" manage the manual tier — the S&P 500 seed tier is read-only
// from here (see db.seedSP500).
func (b *Bot) handleUniverse(args string) {
	fields := strings.Fields(args)
	if len(fields) == 0 {
		b.sendUniverseSummary()
		return
	}

	sub := strings.ToLower(fields[0])
	ticker := ""
	if len(fields) > 1 {
		ticker = strings.ToUpper(strings.TrimSpace(fields[1]))
	}

	switch sub {
	case "add":
		if ticker == "" {
			b.Send(i18n.T(b.lang, i18n.KeyUniverseAddUsage))
			return
		}
		if err := b.db.AddUniverseTicker(ticker, "manual"); err != nil {
			b.Send(i18n.T(b.lang, i18n.KeyUniverseAddFailed, ticker, err))
			return
		}
		b.Send(i18n.T(b.lang, i18n.KeyUniverseAddSuccess, ticker))
	case "remove":
		if ticker == "" {
			b.Send(i18n.T(b.lang, i18n.KeyUniverseRemoveUsage))
			return
		}
		if err := b.db.RemoveUniverseTicker(ticker); err != nil {
			b.Send(i18n.T(b.lang, i18n.KeyUniverseRemoveFailed, ticker, err))
			return
		}
		b.Send(i18n.T(b.lang, i18n.KeyUniverseRemoveSuccess, ticker))
	default:
		b.Send(i18n.T(b.lang, i18n.KeyUniverseUsage))
	}
}

func (b *Bot) sendUniverseSummary() {
	entries, err := b.db.GetUniverse()
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyQueryFailed, err))
		return
	}

	bySource := make(map[string]int)
	for _, e := range entries {
		bySource[e.Source]++
	}

	var sb strings.Builder
	sb.WriteString(i18n.T(b.lang, i18n.KeyUniverseSummary, len(entries)))
	for _, source := range []string{"sp500", "manual"} {
		if n := bySource[source]; n > 0 {
			sb.WriteString(i18n.T(b.lang, i18n.KeyUniverseSourceLine, source, n))
		}
	}
	b.Send(sb.String())
}

// handleChat replies to a plain-text (non-command) message using the LLM
// client's persistent session, so the agent remembers earlier turns in this
// conversation — unlike /recommend and /check, which are one-shot analysis
// calls with no memory between requests. Every message is prefixed with a
// read-only summary of the watchlist/positions (see buildChatContext) so
// free-form questions like "我自選股裡最近跌最多的是哪檔" are answerable
// without giving the ACP session any tools.
func (b *Bot) handleChat(ctx context.Context, text string) {
	b.Send(i18n.T(b.lang, i18n.KeyThinking))

	if ctxBlock := b.buildChatContext(); ctxBlock != "" {
		text = ctxBlock + text
	}

	reply, err := b.llm.Chat(ctx, text)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyChatFailed, err))
		return
	}
	b.Send(reply)
}

// buildChatContext composes formatChatContext's input from the DB: the
// union of watchlist and position tickers (a position ticker should always
// already be on the watchlist via /buy's auto-add, but this covers a
// position added before that existed), each one's latest closing snapshot,
// and every open position. It deliberately reads local snapshots instead of
// fetching live quotes — adding a round of network calls to every chat
// message would make free-form chat feel sluggish, and "as of last close"
// is a fine trade for conversational context (use /status or /portfolio for
// real-time prices). Returns "" if there's nothing to show.
func (b *Bot) buildChatContext() string {
	watchlist, err := b.db.GetWatchlist()
	if err != nil {
		log.Printf("chat context: watchlist: %v", err)
	}
	positions, err := b.db.GetPositions()
	if err != nil {
		log.Printf("chat context: positions: %v", err)
	}

	tickerSet := make(map[string]bool, len(watchlist))
	tickers := make([]string, 0, len(watchlist)+len(positions))
	for _, t := range watchlist {
		tickerSet[t] = true
		tickers = append(tickers, t)
	}
	posByTicker := make(map[string]db.Position, len(positions))
	for _, p := range positions {
		posByTicker[p.Ticker] = p
		if !tickerSet[p.Ticker] {
			tickerSet[p.Ticker] = true
			tickers = append(tickers, p.Ticker)
		}
	}

	snapshots := make(map[string]db.DailySnapshot, len(tickers))
	for _, t := range tickers {
		snap, ok, err := b.db.GetLatestSnapshot(t)
		if err != nil {
			log.Printf("chat context: snapshot %s: %v", t, err)
			continue
		}
		if ok {
			snapshots[t] = snap
		}
	}

	return formatChatContext(b.lang, tickers, posByTicker, snapshots)
}

// formatChatContext renders the read-only background block prefixed to
// chat messages: each ticker's most recent close plus, for tickers actually
// held, cost basis and unrealized P&L against that close. tickers is the
// order to render in; positions/snapshots are keyed by ticker. Returns ""
// for an empty tickers list so callers can skip prefixing entirely.
func formatChatContext(lang i18n.Lang, tickers []string, positions map[string]db.Position, snapshots map[string]db.DailySnapshot) string {
	if len(tickers) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(i18n.T(lang, i18n.KeyChatContextHeader))
	for _, t := range tickers {
		snap, ok := snapshots[t]
		if !ok {
			sb.WriteString(i18n.T(lang, i18n.KeyChatContextTickerNoData, t))
			continue
		}
		if p, held := positions[t]; held {
			unrealizedPct := (snap.Close - p.AvgCost) / p.AvgCost * 100
			sb.WriteString(i18n.T(lang, i18n.KeyChatContextPositionLine, t, snap.Date, snap.Close, snap.ChangePercent, p.Shares, p.AvgCost, unrealizedPct))
		} else {
			sb.WriteString(i18n.T(lang, i18n.KeyChatContextWatchLine, t, snap.Date, snap.Close, snap.ChangePercent))
		}
	}
	sb.WriteString(i18n.T(lang, i18n.KeyChatContextFooter))
	return sb.String()
}

// handleReset clears the persistent chat session so the next message starts
// a fresh conversation with no memory of earlier turns.
func (b *Bot) handleReset() {
	b.llm.ResetChat()
	b.Send(i18n.T(b.lang, i18n.KeyResetDone))
}

// recoverJobPanic recovers from a panic inside a scheduler-invoked job
// (RunDailyReport/RunClosingSnapshot), logging it and alerting the user over
// Telegram. Without this, a panic in either job would kill that goroutine
// silently — the bot keeps running and answering commands, but the VPS is
// unattended, so a failed daily report or closing snapshot would otherwise
// go completely unnoticed. job names the job for the log line and alert.
func (b *Bot) recoverJobPanic(job string) {
	if r := recover(); r != nil {
		log.Printf("%s: panic: %v", job, r)
		b.Send(i18n.T(b.lang, i18n.KeyJobPanic, job, r))
	}
}

// SendSignalAlert sends signal notifications to the chat.
func (b *Bot) SendSignalAlert(sigs []signals.Signal) {
	if len(sigs) == 0 {
		return
	}
	var sb strings.Builder
	sb.WriteString(i18n.T(b.lang, i18n.KeySignalAlertTitle))
	for _, s := range sigs {
		sb.WriteString("• " + s.Message + "\n")
	}
	b.Send(sb.String())
}

// RunClosingSnapshot records the just-closed US session's OHLCV for every
// watchlist ticker into daily_snapshots. Called by the scheduler at 05:30
// CST — after the US close — so unlike the pre-open daily report this
// captures genuine closing data. At that hour the US trading date is
// Taiwan's "yesterday", which is why the snapshot is dated one day back.
// It's a silent background job: results go to the DB and errors to the log,
// not to Telegram.
func (b *Bot) RunClosingSnapshot(ctx context.Context) {
	defer b.recoverJobPanic("closing snapshot")

	tickers, err := b.db.GetWatchlist()
	if err != nil {
		log.Printf("closing snapshot: watchlist: %v", err)
		b.Send(i18n.T(b.lang, i18n.KeyWatchlistQueryFailed, err))
		return
	}

	date := time.Now().In(cst).AddDate(0, 0, -1).Format("2006-01-02")
	prices := make(map[string]float64, len(tickers))
	for _, t := range tickers {
		q, err := b.provider.GetQuote(t)
		if err != nil {
			log.Printf("closing snapshot: quote %s: %v", t, err)
			continue
		}
		// On a US market holiday the cron still fires but providers return
		// the previous session's quote; its timestamp is then a full day
		// old, and saving it would file old data under the wrong date.
		if time.Since(q.Timestamp) > 12*time.Hour {
			log.Printf("closing snapshot: %s quote is stale (%s), skipping (US holiday?)", t, q.Timestamp.Format(time.RFC3339))
			continue
		}
		prices[t] = q.Price
		snap := db.DailySnapshot{
			Ticker:        t,
			Date:          date,
			Open:          q.Open,
			Close:         q.Price,
			High:          q.High,
			Low:           q.Low,
			Volume:        q.Volume,
			ChangePercent: q.ChangePercent,
		}
		if err := b.db.SaveSnapshot(snap); err != nil {
			log.Printf("closing snapshot: save %s: %v", t, err)
		}
	}
	log.Printf("closing snapshot: done for %s (%d tickers)", date, len(tickers))

	b.recordNetWorthSnapshot(date, prices)
}

// recordNetWorthSnapshot totals every open position's value as of the
// closing snapshot and stores it dated the same day, so a net worth curve
// can be drawn later. prices reuses the quotes RunClosingSnapshot already
// fetched for watchlist tickers (positions are auto-added to the watchlist
// on /buy, so this covers the common case); any position ticker missing
// from it gets a direct quote fetch as a fallback.
func (b *Bot) recordNetWorthSnapshot(date string, prices map[string]float64) {
	positions, err := b.db.GetPositions()
	if err != nil {
		log.Printf("net worth snapshot: positions: %v", err)
		return
	}
	if len(positions) == 0 {
		return
	}

	var total float64
	for _, p := range positions {
		price, ok := prices[p.Ticker]
		if !ok {
			q, err := b.provider.GetQuote(p.Ticker)
			if err != nil {
				log.Printf("net worth snapshot: quote %s: %v", p.Ticker, err)
				continue
			}
			price = q.Price
		}
		total += p.Shares * price
	}
	if err := b.db.SaveNetWorthSnapshot(date, total); err != nil {
		log.Printf("net worth snapshot: save: %v", err)
	}
}

// RunDailyReport fetches data, detects signals, generates LLM recommendations,
// and sends the daily report. Called by the scheduler.
func (b *Bot) RunDailyReport(ctx context.Context) {
	defer b.recoverJobPanic("daily report")

	b.Send(i18n.T(b.lang, i18n.KeyDailyReportStart))

	tickers, err := b.db.GetWatchlist()
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyWatchlistQueryFailed, err))
		return
	}

	candidateTickers, err := b.provider.GetMarketMovers()
	if err != nil {
		log.Printf("market movers: %v", err)
	}
	scanHits := b.loadScanHits()
	dedupedCandidates := mergeCandidates(candidateTickers, scanHits, tickers)

	positions := b.loadPositions()
	earnings := b.loadEarnings(append(append([]string{}, tickers...), dedupedCandidates...))
	b.checkEarningsAlerts(tickers, earnings)

	// Detect signals on watchlist
	var allSignals []signals.Signal
	watchlist := b.fetchStockData(tickers, true, positions, earnings, nil)
	for _, s := range watchlist {
		if s.Quote != nil {
			allSignals = append(allSignals, b.detector.CheckQuote(s.Quote)...)
		}
	}
	for _, t := range tickers {
		closes, err := b.history.GetHistory(t)
		if err != nil {
			log.Printf("history %s: %v", t, err)
			continue
		}
		allSignals = append(allSignals, b.checkStatefulSignals(t, closes)...)
	}
	if len(allSignals) > 0 {
		b.SendSignalAlert(allSignals)
	}

	candidates := b.fetchStockData(dedupedCandidates, false, positions, earnings, scanHits)

	recs, err := b.llm.GenerateRecommendations(ctx, watchlist, candidates)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyLLMFailed, err))
		return
	}

	if len(recs) == 0 {
		b.Send(i18n.T(b.lang, i18n.KeyNoRecommendationsToday))
		return
	}

	b.sendAndSaveRecommendations(recs, watchlist, candidates)
}

// checkStatefulSignals runs the RSI/MACD checks that diff against the last
// state persisted in signal_states: RSI only alerts when it newly enters an
// extreme zone (no repeat alert while it stays there on consecutive days),
// and MACD only alerts on an actual golden/death cross rather than every day
// a trend holds. A failed state read falls back to "" — worst case one
// duplicate alert, better than dropping the check entirely.
func (b *Bot) checkStatefulSignals(ticker string, closes []float64) []signals.Signal {
	var out []signals.Signal

	prevRSI, err := b.db.GetSignalState(ticker, signals.FamilyRSI)
	if err != nil {
		log.Printf("signal state %s/%s: %v", ticker, signals.FamilyRSI, err)
	}
	sig, newRSI := b.detector.CheckRSIState(ticker, closes, prevRSI)
	if sig != nil {
		out = append(out, *sig)
	}
	if newRSI != prevRSI {
		if err := b.db.SetSignalState(ticker, signals.FamilyRSI, newRSI); err != nil {
			log.Printf("signal state %s/%s: %v", ticker, signals.FamilyRSI, err)
		}
	}

	prevMACD, err := b.db.GetSignalState(ticker, signals.FamilyMACD)
	if err != nil {
		log.Printf("signal state %s/%s: %v", ticker, signals.FamilyMACD, err)
	}
	sig, newMACD := b.detector.CheckMACDCross(ticker, closes, prevMACD)
	if sig != nil {
		out = append(out, *sig)
	}
	if newMACD != prevMACD {
		if err := b.db.SetSignalState(ticker, signals.FamilyMACD, newMACD); err != nil {
			log.Printf("signal state %s/%s: %v", ticker, signals.FamilyMACD, err)
		}
	}

	return out
}

// scanChunkCount and universeScanRequestDelay govern Phase 2.6's daily
// candidate-pool scan: the universe (~500 S&P 500 + manual tickers) is split
// into scanChunkCount rotating slices — matching the closing-snapshot cadence
// of Tue–Sat, 5 trading days/week — so a full pass covers roughly 100
// tickers/day. universeScanRequestDelay throttles Yahoo history requests
// within a chunk, per PLAN.md's explicit note not to hammer it.
const (
	scanChunkCount           = 5
	universeScanRequestDelay = 300 * time.Millisecond
)

// universeScanChunk returns the slice of tickers to scan for dayIndex (an
// ever-increasing day counter, e.g. time.Now().YearDay()), rotating through
// all of tickers over chunkCount calls. Pure and stateless — no persisted
// scan cursor needed — so coverage is deterministic given the same tickers
// and dayIndex, at the cost of chunk boundaries shifting slightly as the
// universe's membership changes day to day (harmless: PLAN.md tolerates
// staleness on the order of months for this data).
func universeScanChunk(tickers []string, chunkCount, dayIndex int) []string {
	if len(tickers) == 0 || chunkCount <= 0 {
		return nil
	}
	size := (len(tickers) + chunkCount - 1) / chunkCount
	idx := dayIndex % chunkCount
	if idx < 0 {
		idx += chunkCount
	}
	start := idx * size
	if start >= len(tickers) {
		return nil
	}
	end := start + size
	if end > len(tickers) {
		end = len(tickers)
	}
	return tickers[start:end]
}

// RunUniverseScan is Phase 2.6's chunked candidate-pool scan: it checks
// today's rotating slice of the universe (excluding anything already on the
// watchlist, which gets a full RSI/MACD check daily anyway) for a fresh
// RSI/MACD signal via the same checkStatefulSignals used for the watchlist —
// safe to share signal_states with it since the two ticker sets never
// overlap. Any hit is logged to scan_hits for RunDailyReport/handleRecommend
// to pick up the same day and upgrade into an LLM candidate. Silent
// background job like RunClosingSnapshot: results go to the DB/log, not
// Telegram — the eventual daily report is the user-facing surface.
func (b *Bot) RunUniverseScan(ctx context.Context) {
	defer b.recoverJobPanic("universe scan")

	entries, err := b.db.GetUniverse()
	if err != nil {
		log.Printf("universe scan: universe: %v", err)
		return
	}
	watchlist, err := b.db.GetWatchlist()
	if err != nil {
		log.Printf("universe scan: watchlist: %v", err)
		return
	}
	watchSet := make(map[string]bool, len(watchlist))
	for _, t := range watchlist {
		watchSet[t] = true
	}

	var tickers []string
	for _, e := range entries {
		if !watchSet[e.Ticker] {
			tickers = append(tickers, e.Ticker)
		}
	}

	chunk := universeScanChunk(tickers, scanChunkCount, time.Now().In(cst).YearDay())
	date := todayDate()
	hits := 0
	for i, t := range chunk {
		select {
		case <-ctx.Done():
			log.Printf("universe scan: cancelled after %d/%d tickers", i, len(chunk))
			return
		default:
		}

		closes, err := b.history.GetHistory(t)
		if err != nil {
			log.Printf("universe scan: history %s: %v", t, err)
			continue
		}
		for _, sig := range b.checkStatefulSignals(t, closes) {
			if err := b.db.SaveScanHit(t, date, sig.Message); err != nil {
				log.Printf("universe scan: save hit %s: %v", t, err)
				continue
			}
			hits++
		}

		if i < len(chunk)-1 {
			time.Sleep(universeScanRequestDelay)
		}
	}
	log.Printf("universe scan: checked %d tickers, %d hits", len(chunk), hits)
}

// fetchStockData fetches quote+news for each ticker. Fundamentals are only
// attached when includeFundamentals is set (watchlist tickers, not the
// broad market-mover candidate list) to stay well under Finnhub's free-tier
// 60-requests/minute limit when a candidate list has a dozen-plus tickers.
// positions (ticker -> open position) is looked up via loadPositions and
// attaches cost-basis context for any ticker the user actually holds;
// earnings (ticker -> upcoming earnings) is looked up via loadEarnings and
// attaches an earnings-date warning for any ticker reporting soon.
// scanReasons (ticker -> joined signal message) is looked up via
// db.GetScanHits and attaches why a Phase 2.6 universe-scan candidate was
// surfaced. Pass nil for any of the three if there's nothing to attach.
func (b *Bot) fetchStockData(tickers []string, includeFundamentals bool, positions map[string]db.Position, earnings map[string]data.EarningsEvent, scanReasons map[string]string) []llm.StockData {
	var result []llm.StockData
	for _, t := range tickers {
		q, err := b.provider.GetQuote(t)
		if err != nil {
			log.Printf("quote %s: %v", t, err)
			continue
		}
		news, _ := b.provider.GetNews(t, 5)
		stock := llm.StockData{Quote: q, News: news}
		if includeFundamentals && b.fundamentals != nil {
			if fd, err := b.fundamentals.GetFundamentals(t); err != nil {
				log.Printf("fundamentals %s: %v", t, err)
			} else {
				stock.Fundamentals = fd
			}
		}
		if p, ok := positions[t]; ok {
			stock.Position = &llm.Position{Shares: p.Shares, AvgCost: p.AvgCost}
		}
		if e, ok := earnings[t]; ok {
			stock.Earnings = &llm.Earnings{Date: e.Date, DaysUntil: daysUntil(e.Date)}
		}
		if r, ok := scanReasons[t]; ok {
			stock.ScanReason = &r
		}
		result = append(result, stock)
	}
	return result
}

// loadPositions returns every open position keyed by ticker, for attaching
// cost-basis context to LLM prompts. A query failure logs and degrades to an
// empty map rather than failing the caller — recommendations without cost
// basis are still useful.
func (b *Bot) loadPositions() map[string]db.Position {
	positions, err := b.db.GetPositions()
	if err != nil {
		log.Printf("load positions: %v", err)
		return nil
	}
	out := make(map[string]db.Position, len(positions))
	for _, p := range positions {
		out[p.Ticker] = p
	}
	return out
}

// loadEarnings returns each ticker's next scheduled earnings date within
// earningsPromptWindowDays, keyed by ticker. Degrades to nil (not an error)
// when Finnhub isn't configured or the request fails — same optional-data
// pattern as fundamentals.
func (b *Bot) loadEarnings(tickers []string) map[string]data.EarningsEvent {
	if b.earnings == nil || len(tickers) == 0 {
		return nil
	}
	events, err := b.earnings.GetUpcomingEarnings(tickers, earningsPromptWindowDays)
	if err != nil {
		log.Printf("earnings calendar: %v", err)
		return nil
	}
	return events
}

// loadScanHits returns today's Phase 2.6 universe-scan hits keyed by ticker
// (joined reason string per ticker) via db.GetScanHits. Degrades to nil
// (not an error) on a query failure — candidates without a scan reason still
// go through movers as before.
func (b *Bot) loadScanHits() map[string]string {
	hits, err := b.db.GetScanHits(todayDate())
	if err != nil {
		log.Printf("scan hits: %v", err)
		return nil
	}
	return hits
}

// checkEarningsAlerts sends one batched Telegram message warning about
// watchlist tickers (positions are always on the watchlist via /buy's
// auto-add, so this covers held positions too) with earnings due within
// earningsAlertDays. Deduped via signal_states (family "earnings", state =
// the earnings date string) so it fires once per reporting date rather than
// every day the ticker sits inside the alert window.
func (b *Bot) checkEarningsAlerts(tickers []string, earnings map[string]data.EarningsEvent) {
	var lines []string
	for _, t := range tickers {
		e, ok := earnings[t]
		if !ok {
			continue
		}
		days := daysUntil(e.Date)
		if days < 0 || days > earningsAlertDays {
			continue
		}

		prev, err := b.db.GetSignalState(t, earningsSignalFamily)
		if err != nil {
			log.Printf("earnings alert state %s: %v", t, err)
		}
		if prev == e.Date {
			continue
		}

		lines = append(lines, i18n.T(b.lang, i18n.KeyEarningsAlertLine, t, e.Date, days))
		if err := b.db.SetSignalState(t, earningsSignalFamily, e.Date); err != nil {
			log.Printf("earnings alert state %s: %v", t, err)
		}
	}
	if len(lines) == 0 {
		return
	}

	var sb strings.Builder
	sb.WriteString(i18n.T(b.lang, i18n.KeyEarningsAlertTitle))
	for _, l := range lines {
		sb.WriteString(l)
	}
	b.Send(sb.String())
}

// daysUntil returns the whole number of days from today (Taiwan time) until
// dateStr (YYYY-MM-DD), which may be negative for a past date. Both sides
// are compared as date-only values (not instants) so it's not sensitive to
// what time of day it's called.
func daysUntil(dateStr string) int {
	target, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return 0
	}
	today, _ := time.Parse("2006-01-02", time.Now().In(cst).Format("2006-01-02"))
	return int(target.Sub(today).Hours() / 24)
}

func formatQuote(lang i18n.Lang, q *data.Quote) string {
	arrow := "▲"
	if q.ChangePercent < 0 {
		arrow = "▼"
	}
	return i18n.T(lang, i18n.KeyQuoteLine, q.Ticker, q.Price, arrow, q.ChangePercent, q.Open, q.High, q.Low)
}

func formatFundamentals(lang i18n.Lang, fd *data.Fundamentals) string {
	var sb strings.Builder
	sb.WriteString(i18n.T(lang, i18n.KeyValuationHeader))
	sb.WriteString(i18n.T(lang, i18n.KeyPE, fd.PE))
	sb.WriteString(i18n.T(lang, i18n.KeyPB, fd.PB))
	sb.WriteString(i18n.T(lang, i18n.KeyPS, fd.PS))
	sb.WriteString(i18n.T(lang, i18n.KeyMarketCap, commaf(fd.MarketCapMillion)))
	sb.WriteString(i18n.T(lang, i18n.KeyBeta, fd.Beta))
	sb.WriteString(i18n.T(lang, i18n.Key52Week, fd.Week52High, fd.Week52Low))

	sb.WriteString(i18n.T(lang, i18n.KeyProfitabilityHeader))
	sb.WriteString(i18n.T(lang, i18n.KeyROE, fd.ROE))
	sb.WriteString(i18n.T(lang, i18n.KeyROA, fd.ROA))
	sb.WriteString(i18n.T(lang, i18n.KeyGrossMargin, fd.GrossMarginPct))
	sb.WriteString(i18n.T(lang, i18n.KeyOperatingMargin, fd.OperatingMarginPct))
	sb.WriteString(i18n.T(lang, i18n.KeyNetMargin, fd.NetMarginPct))

	sb.WriteString(i18n.T(lang, i18n.KeyFinStructureHeader))
	sb.WriteString(i18n.T(lang, i18n.KeyDebtToEquity, fd.DebtToEquity))
	sb.WriteString(i18n.T(lang, i18n.KeyCurrentRatio, fd.CurrentRatio))
	sb.WriteString(i18n.T(lang, i18n.KeyQuickRatio, fd.QuickRatio))

	sb.WriteString(i18n.T(lang, i18n.KeyGrowthHeader))
	sb.WriteString(i18n.T(lang, i18n.KeyRevenueGrowth, fd.RevenueGrowthYoY))
	sb.WriteString(i18n.T(lang, i18n.KeyEPSGrowth, fd.EPSGrowthYoY))
	sb.WriteString(i18n.T(lang, i18n.KeyEPS, fd.EPS))
	sb.WriteString(i18n.T(lang, i18n.KeyBookValue, fd.BookValuePerShare))
	sb.WriteString(i18n.T(lang, i18n.KeyDividendYield, fd.DividendYieldPct))
	return sb.String()
}

func formatFinancialStatement(lang i18n.Lang, st *data.FinancialStatement) string {
	var sb strings.Builder
	sb.WriteString(i18n.T(lang, i18n.KeyStatementTitle, st.Form, st.FiscalYear, st.PeriodEnd))

	sb.WriteString(i18n.T(lang, i18n.KeyIncomeStatementHeader))
	sb.WriteString(i18n.T(lang, i18n.KeyRevenue, commaf(st.Revenue/1e6)))
	sb.WriteString(i18n.T(lang, i18n.KeyGrossProfit, commaf(st.GrossProfit/1e6)))
	sb.WriteString(i18n.T(lang, i18n.KeyOperatingIncome, commaf(st.OperatingIncome/1e6)))
	sb.WriteString(i18n.T(lang, i18n.KeyNetIncome, commaf(st.NetIncome/1e6)))
	sb.WriteString(i18n.T(lang, i18n.KeyDilutedEPS, st.DilutedEPS))

	sb.WriteString(i18n.T(lang, i18n.KeyBalanceSheetHeader))
	sb.WriteString(i18n.T(lang, i18n.KeyTotalAssets, commaf(st.TotalAssets/1e6)))
	sb.WriteString(i18n.T(lang, i18n.KeyTotalLiabilities, commaf(st.TotalLiabilities/1e6)))
	sb.WriteString(i18n.T(lang, i18n.KeyTotalEquity, commaf(st.TotalEquity/1e6)))

	sb.WriteString(i18n.T(lang, i18n.KeyCashFlowHeader))
	sb.WriteString(i18n.T(lang, i18n.KeyOperatingCashFlow, commaf(st.OperatingCashFlow/1e6)))
	sb.WriteString(i18n.T(lang, i18n.KeyCapEx, commaf(st.CapEx/1e6)))
	sb.WriteString(i18n.T(lang, i18n.KeyFreeCashFlow, commaf(st.FreeCashFlow/1e6)))
	return sb.String()
}

// commaf formats a float as a rounded integer with thousands separators
// (e.g. 4321020 -> "4,321,020"), for human-facing Telegram output.
func commaf(v float64) string {
	n := int64(v + 0.5)
	if v < 0 {
		n = int64(v - 0.5)
	}
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.FormatInt(n, 10)
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

func todayDate() string {
	return time.Now().In(cst).Format("2006-01-02")
}

// dedup returns tickers in a that are not present in b.
func dedup(a, b []string) []string {
	set := make(map[string]bool, len(b))
	for _, t := range b {
		set[t] = true
	}
	var out []string
	for _, t := range a {
		if !set[t] {
			out = append(out, t)
		}
	}
	return out
}

// mergeCandidates combines the market-movers list with today's Phase 2.6
// universe-scan hits into the final candidate ticker list: movers first
// (existing behavior preserved), then any scan-hit ticker not already
// present, finally excluding anything already on the watchlist (exclude).
func mergeCandidates(movers []string, scanHits map[string]string, exclude []string) []string {
	seen := make(map[string]bool, len(movers)+len(scanHits))
	excluded := make(map[string]bool, len(exclude))
	for _, t := range exclude {
		excluded[t] = true
	}

	var out []string
	add := func(t string) {
		if seen[t] || excluded[t] {
			return
		}
		seen[t] = true
		out = append(out, t)
	}
	for _, t := range movers {
		add(t)
	}
	for t := range scanHits {
		add(t)
	}
	return out
}
