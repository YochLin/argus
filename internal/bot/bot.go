package bot

import (
	"context"
	"fmt"
	"log"
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

type Bot struct {
	api          *tgbotapi.BotAPI
	db           *db.DB
	provider     data.Provider
	fundamentals data.FundamentalsProvider // nil if FINNHUB_API_KEY isn't set
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

func New(token string, chatID int64, database *db.DB, provider data.Provider, fundamentals data.FundamentalsProvider, history data.HistoryProvider, llmClient *llm.Client, lang i18n.Lang) (*Bot, error) {
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
	case "dailyreport":
		b.RunDailyReport(ctx)
	case "fundamentals":
		b.handleFundamentals(args)
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

	watchlist := b.fetchStockData(tickers, true)

	candidateTickers, err := b.provider.GetMarketMovers()
	if err != nil {
		log.Printf("market movers: %v", err)
	}
	candidates := b.fetchStockData(dedup(candidateTickers, tickers), false)

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

// handleChat replies to a plain-text (non-command) message using the LLM
// client's persistent session, so the agent remembers earlier turns in this
// conversation — unlike /recommend and /check, which are one-shot analysis
// calls with no memory between requests.
func (b *Bot) handleChat(ctx context.Context, text string) {
	b.Send(i18n.T(b.lang, i18n.KeyThinking))

	reply, err := b.llm.Chat(ctx, text)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyChatFailed, err))
		return
	}
	b.Send(reply)
}

// handleReset clears the persistent chat session so the next message starts
// a fresh conversation with no memory of earlier turns.
func (b *Bot) handleReset() {
	b.llm.ResetChat()
	b.Send(i18n.T(b.lang, i18n.KeyResetDone))
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
	tickers, err := b.db.GetWatchlist()
	if err != nil {
		log.Printf("closing snapshot: watchlist: %v", err)
		return
	}

	date := time.Now().In(cst).AddDate(0, 0, -1).Format("2006-01-02")
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
}

// RunDailyReport fetches data, detects signals, generates LLM recommendations,
// and sends the daily report. Called by the scheduler.
func (b *Bot) RunDailyReport(ctx context.Context) {
	b.Send(i18n.T(b.lang, i18n.KeyDailyReportStart))

	tickers, err := b.db.GetWatchlist()
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyWatchlistQueryFailed, err))
		return
	}

	// Detect signals on watchlist
	var allSignals []signals.Signal
	watchlist := b.fetchStockData(tickers, true)
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

	candidateTickers, err := b.provider.GetMarketMovers()
	if err != nil {
		log.Printf("market movers: %v", err)
	}
	candidates := b.fetchStockData(dedup(candidateTickers, tickers), false)

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

// fetchStockData fetches quote+news for each ticker. Fundamentals are only
// attached when includeFundamentals is set (watchlist tickers, not the
// broad market-mover candidate list) to stay well under Finnhub's free-tier
// 60-requests/minute limit when a candidate list has a dozen-plus tickers.
func (b *Bot) fetchStockData(tickers []string, includeFundamentals bool) []llm.StockData {
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
		result = append(result, stock)
	}
	return result
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
