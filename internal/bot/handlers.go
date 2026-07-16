package bot

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/llm"
	"argus/internal/render"
	"argus/internal/webfetch"
)

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

	in, err := b.gatherRecommendationInputs()
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyWatchlistQueryFailed, err))
		return
	}

	summary, recs, err := b.llm.GenerateRecommendations(ctx, in.watchlist, in.candidates, in.marketNews)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyLLMFailed, err))
		return
	}

	if len(recs) == 0 {
		b.Send(i18n.T(b.lang, i18n.KeyNoRecommendations))
		return
	}

	sources := recommendationSources(in.watchlistTickers, in.candidateTickers, in.scanHits)
	b.sendAndSaveRecommendations(summary, recs, sources, in.watchlist, in.candidates)
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
	if b.analystRating != nil {
		if ar, err := b.analystRating.GetAnalystRating(ticker); err != nil {
			log.Printf("analyst rating %s: %v", ticker, err)
		} else {
			stock.AnalystRating = ar
		}
	}
	stock.Technicals = b.computeTechnicals(ticker)

	result, err := b.llm.CheckStock(ctx, stock)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyLLMFailed, err))
		return
	}

	b.Send(i18n.T(b.lang, i18n.KeyCheckResultTitle, ticker, result))
}

// handleTrack reviews recommendations from the past N days (default 7)
// against today's prices, so recommendation quality is verifiable instead of
// write-only. Hit criteria (Phase 3.8): when a same-period benchmarkTicker
// (SPY) close is on record (see snapshotBenchmark), BUY only counts as a hit
// if the ticker beat SPY's move over the same window and SELL only if it
// underperformed SPY — "up in a broad rally" no longer counts as a good BUY
// call on its own (see trackHit). Recommendations predating the SPY
// snapshot job (or any date SPY has no snapshot for) fall back to the
// absolute-direction rule: BUY hits if price rose, SELL if it fell. The
// baseline price is the one stored at recommendation time; rows from before
// that column existed fall back to the ticker's daily_snapshots close on
// the recommendation date, if the post-close job captured one. The summary
// footer adds average BUY/SELL magnitude and, when more than one candidate
// source appears in the window, a hit-rate breakdown by source
// (watchlist/movers/scan) — see summarizeTrack.
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

	rows, lines, ok, err := b.computeTrackRows(days)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyQueryFailed, err))
		return
	}
	if !ok {
		b.Send(i18n.T(b.lang, i18n.KeyTrackEmpty, days))
		return
	}

	var sb strings.Builder
	sb.WriteString(i18n.T(b.lang, i18n.KeyTrackTitle, days))
	for _, l := range lines {
		sb.WriteString(l)
	}

	overall, bySource := summarizeTrack(rows)
	sb.WriteString(renderTrackSummary(b.lang, overall, bySource))
	b.Send(sb.String())
}

// trackHit implements Phase 3.8's relative-to-SPY hit rule: when a
// same-period SPY change is available, BUY only counts as a hit if the
// ticker beat it and SELL only if it underperformed it; otherwise it falls
// back to the pre-Phase-3.8 absolute-direction rule (BUY counts if price
// rose, SELL if it fell). Only meaningful for action == "BUY"/"SELL" —
// anything else (HOLD, "") always returns false, since handleTrack never
// scores those.
func trackHit(action string, tickerChangePct, spyChangePct float64, haveSPY bool) bool {
	switch action {
	case "BUY":
		if haveSPY {
			return tickerChangePct > spyChangePct
		}
		return tickerChangePct > 0
	case "SELL":
		if haveSPY {
			return tickerChangePct < spyChangePct
		}
		return tickerChangePct < 0
	default:
		return false
	}
}

// trackRow is one BUY/SELL recommendation reduced to what /track's summary
// needs, computed by computeTrackRows (which has the live quotes/SPY data)
// so the aggregation below stays a pure pass over plain values.
type trackRow struct {
	Action    string // "BUY" or "SELL" only
	Source    string // already normalized via displaySource
	ChangePct float64
	Hit       bool
}

// trackSourceStats accumulates hit-rate and average-magnitude stats for one
// group of trackRows (either every row, or one source's rows).
type trackSourceStats struct {
	Hits, Evaluated int
	BuySum          float64
	BuyCount        int
	SellSum         float64
	SellCount       int
}

func (s trackSourceStats) HitRate() float64 {
	if s.Evaluated == 0 {
		return 0
	}
	return float64(s.Hits) / float64(s.Evaluated) * 100
}

func (s trackSourceStats) AvgBuyPct() float64 {
	if s.BuyCount == 0 {
		return 0
	}
	return s.BuySum / float64(s.BuyCount)
}

func (s trackSourceStats) AvgSellPct() float64 {
	if s.SellCount == 0 {
		return 0
	}
	return s.SellSum / float64(s.SellCount)
}

// summarizeTrack aggregates trackRows into overall stats and a per-source
// breakdown (see trackSourceStats), for /track's summary footer: hit rate,
// average BUY/SELL magnitude, and — when more than one source is present —
// the same broken down by candidate-sourcing path (Phase 2.6's
// deferred-until-Phase-3.8 "成效對照").
func summarizeTrack(rows []trackRow) (overall trackSourceStats, bySource map[string]trackSourceStats) {
	bySource = make(map[string]trackSourceStats)
	for _, r := range rows {
		accumulateTrackRow(&overall, r)
		s := bySource[r.Source]
		accumulateTrackRow(&s, r)
		bySource[r.Source] = s
	}
	return overall, bySource
}

func accumulateTrackRow(s *trackSourceStats, r trackRow) {
	s.Evaluated++
	if r.Hit {
		s.Hits++
	}
	switch r.Action {
	case "BUY":
		s.BuySum += r.ChangePct
		s.BuyCount++
	case "SELL":
		s.SellSum += r.ChangePct
		s.SellCount++
	}
}

// displaySource normalizes a stored db.Recommendation.Source for display:
// rows saved before the source column existed have "" and read as
// "watchlist" (see the migration's doc comment in internal/db).
func displaySource(source string) string {
	if source == "" {
		return "watchlist"
	}
	return source
}

// sortedSourceKeys returns bySource's keys in alphabetical order, so
// /track's per-source breakdown renders in a stable order instead of Go's
// randomized map iteration.
func sortedSourceKeys(bySource map[string]trackSourceStats) []string {
	keys := make([]string, 0, len(bySource))
	for k := range bySource {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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
	b.Send(b.recordBuy(ticker, shares, price, fee, date))
}

// recordBuy is handleBuy's core, pulled out so a confirmed Phase 4
// pending-action proposal (record_buy, see internal/mcptools'
// trade_write_tools.go) can execute the exact same logic and produce the
// exact same confirmation text as typing /buy directly — see
// executePendingAction.
func (b *Bot) recordBuy(ticker string, shares, price, fee float64, date string) string {
	pos, err := b.db.RecordBuy(ticker, shares, price, fee, date)
	if err != nil {
		return i18n.T(b.lang, i18n.KeyBuyFailed, err)
	}
	if err := b.db.AddTicker(ticker); err != nil {
		log.Printf("buy: add %s to watchlist: %v", ticker, err)
	}
	return i18n.T(b.lang, i18n.KeyBuySuccess, ticker, shares, price, fee, pos.Shares, pos.AvgCost) + b.thesisNudge(ticker)
}

// thesisNudge returns a one-line nudge to record a holding thesis when
// ticker doesn't have one yet, or "" when it already does (or the lookup
// fails — a nudge is a courtesy, not worth failing the trade confirmation
// over). Called only from handleBuy, never blocking the trade itself — see
// PLAN.md's Phase 3.6 expansion "論點日誌" item.
func (b *Bot) thesisNudge(ticker string) string {
	_, ok, err := b.db.GetThesis(ticker)
	if err != nil {
		log.Printf("buy: check thesis %s: %v", ticker, err)
		return ""
	}
	if ok {
		return ""
	}
	return i18n.T(b.lang, i18n.KeyBuyThesisNudge, ticker, ticker)
}

// handleSell records a sale against an existing position and reports the
// realized P&L. It does not remove the ticker from the watchlist even when
// the position is fully closed out — the user may still want to watch it.
// When the sell fully closes the position, it kicks off Phase 3.8's
// sell-review (reviewClosedTrade) as a second message after the success
// reply — in its own goroutine so the (slow) LLM call never delays the
// immediate confirmation, and log-only on failure since the user already has
// their sell confirmation and doesn't need a second alert about the review
// itself failing.
func (b *Bot) handleSell(ctx context.Context, args string) {
	ticker, shares, price, fee, date, err := parseTradeArgs(args)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeySellUsage))
		return
	}
	if date == "" {
		date = todayDate()
	}
	msg, closed := b.recordSell(ticker, shares, price, fee, date)
	b.Send(msg)
	if closed {
		go b.reviewClosedTrade(ctx, ticker)
	}
}

// recordSell is handleSell's core, pulled out for the same reason as
// recordBuy — a confirmed Phase 4 pending-action proposal (record_sell)
// reuses this instead of duplicating the RecordSell call and error mapping.
// closed reports whether this sell fully closed out the position (shares
// returned to 0), so callers can decide whether to trigger a sell-review;
// it's always false on an error path, since nothing was recorded.
func (b *Bot) recordSell(ticker string, shares, price, fee float64, date string) (msg string, closed bool) {
	pos, realizedPnL, err := b.db.RecordSell(ticker, shares, price, fee, date)
	if err != nil {
		switch {
		case errors.Is(err, db.ErrNoPosition):
			return i18n.T(b.lang, i18n.KeySellNoPosition, ticker), false
		case errors.Is(err, db.ErrInsufficientShares):
			return i18n.T(b.lang, i18n.KeySellInsufficientShares, ticker), false
		default:
			return i18n.T(b.lang, i18n.KeySellFailed, err), false
		}
	}
	return i18n.T(b.lang, i18n.KeySellSuccess, ticker, shares, price, fee, realizedPnL, pos.Shares), pos.Shares == 0
}

// tradeRound is a fully closed round trip in a ticker's transaction history:
// the share balance went from 0 up to some positive amount (via one or more
// BUYs) and back down to 0 (via one or more SELLs), possibly with several
// buys and partial sells interleaved. Legs holds every transaction in that
// round, oldest first.
type tradeRound struct {
	Legs      []db.Transaction
	StartDate string // the first BUY's date
	EndDate   string // the date the balance returned to 0
}

// lastClosedRound segments txs (expected sorted oldest-first, as
// db.GetTransactions returns them) into round trips by walking the running
// share balance, and returns the most recent one that closed back to 0 —
// deliberately not db.GetEarliestBuyDate's all-time MIN(date), which would
// anchor to an earlier round if the ticker was fully closed out and later
// re-entered (see docs/phase-3.8-sell-review.md's note on this exact
// conflict). ok is false when there's no closed round at all (never traded,
// or the only round on record is still open). A round still open at the end
// of txs is simply not returned — /review reviews what's actually finished,
// not an in-progress position. Balances within 1e-9 of 0 count as closed,
// the same float-dust threshold db.RecordSell uses to decide whether a sell
// fully closes a position.
func lastClosedRound(txs []db.Transaction) (tradeRound, bool) {
	var last tradeRound
	found := false

	balance := 0.0
	start := -1
	for i, tx := range txs {
		if start == -1 {
			start = i
		}
		switch tx.Side {
		case "BUY":
			balance += tx.Shares
		case "SELL":
			balance -= tx.Shares
		}
		if math.Abs(balance) < 1e-9 {
			last = tradeRound{
				Legs:      append([]db.Transaction{}, txs[start:i+1]...),
				StartDate: txs[start].Date,
				EndDate:   tx.Date,
			}
			found = true
			start = -1
			balance = 0
		}
	}
	return last, found
}

// weightedAvgPrice returns the shares-weighted average price across every
// leg in legs matching side ("BUY" or "SELL"), or 0 if there are none — the
// same weighted-average shape db.RecordBuy uses for cost basis, just over a
// fixed slice of legs instead of an incremental running update. Used to
// reduce a multi-leg round trip's entry/exit down to single reference prices
// for the vs-SPY comparison.
func weightedAvgPrice(legs []db.Transaction, side string) float64 {
	var shares, cost float64
	for _, l := range legs {
		if l.Side == side {
			shares += l.Shares
			cost += l.Shares * l.Price
		}
	}
	if shares == 0 {
		return 0
	}
	return cost / shares
}

// buildClosedTradeReview assembles Phase 3.8 追加項's sell-review input (see
// docs/phase-3.8-sell-review.md) for ticker's most recent fully closed round
// trip (lastClosedRound) — realized P&L, holding days, the period's own
// high/low, a vs-SPY comparison (nil if either end's SPY close is missing),
// the recorded thesis (nil if none), and every recommendation issued during
// the holding window. ok is false when there's no closed round to review at
// all (never traded, or still open); every other per-field lookup degrades
// individually (logged, left at its zero value) rather than failing the
// whole review — same "attach what's available" convention as
// fetchStockData's optional StockData fields.
func (b *Bot) buildClosedTradeReview(ticker string) (llm.ClosedTrade, bool, error) {
	txs, err := b.db.GetTransactions(ticker)
	if err != nil {
		return llm.ClosedTrade{}, false, err
	}
	round, ok := lastClosedRound(txs)
	if !ok {
		return llm.ClosedTrade{}, false, nil
	}

	legs := make([]llm.TradeLeg, len(round.Legs))
	var realizedPnL float64
	for i, tx := range round.Legs {
		legs[i] = llm.TradeLeg{Side: tx.Side, Shares: tx.Shares, Price: tx.Price, Date: tx.Date}
		realizedPnL += tx.RealizedPnL
	}

	holdingDays := 0
	if start, serr := time.Parse("2006-01-02", round.StartDate); serr == nil {
		if end, eerr := time.Parse("2006-01-02", round.EndDate); eerr == nil {
			holdingDays = int(end.Sub(start).Hours() / 24)
		}
	}

	trade := llm.ClosedTrade{
		Ticker:      ticker,
		Legs:        legs,
		RealizedPnL: realizedPnL,
		HoldingDays: holdingDays,
	}

	if high, low, ok, err := b.db.GetCloseExtremes(ticker, round.StartDate, round.EndDate); err != nil {
		log.Printf("review %s: close extremes: %v", ticker, err)
	} else if ok {
		trade.PeriodHigh = high
		trade.PeriodLow = low
	}

	if entryPrice := weightedAvgPrice(round.Legs, "BUY"); entryPrice > 0 {
		exitPrice := weightedAvgPrice(round.Legs, "SELL")
		spyStart, startOK, startErr := b.db.GetSnapshotClose(benchmarkTicker, round.StartDate)
		spyEnd, endOK, endErr := b.db.GetSnapshotClose(benchmarkTicker, round.EndDate)
		if startErr != nil || endErr != nil {
			log.Printf("review %s: spy close: start err=%v end err=%v", ticker, startErr, endErr)
		} else if startOK && endOK {
			vs := computeVsSPY(exitPrice, entryPrice, spyEnd, spyStart)
			trade.VsSPY = &vs
		}
	}

	if thesis, ok, err := b.db.GetThesis(ticker); err != nil {
		log.Printf("review %s: thesis: %v", ticker, err)
	} else if ok {
		trade.Thesis = &thesis
	}

	if recs, err := b.db.GetRecommendationsForTicker(ticker, round.StartDate, round.EndDate); err != nil {
		log.Printf("review %s: recommendations: %v", ticker, err)
	} else {
		for _, r := range recs {
			trade.Recommendations = append(trade.Recommendations, llm.TradeRecommendation{Date: r.Date, Action: r.Action, Reason: r.Reason})
		}
	}

	return trade, true, nil
}

// reviewClosedTrade is the automatic sell-review path, triggered by
// handleSell/executePendingAction right after a sell fully closes a
// position. Log-only on any failure — the user already has their sell
// confirmation, so a second failure alert about the review itself would be
// noise for something that isn't the trade record. See handleReview for the
// manual /review TICKER path, which reports failures to the user instead.
func (b *Bot) reviewClosedTrade(ctx context.Context, ticker string) {
	trade, ok, err := b.buildClosedTradeReview(ticker)
	if err != nil {
		log.Printf("review %s: %v", ticker, err)
		return
	}
	if !ok {
		log.Printf("review %s: no closed round found right after closing (unexpected)", ticker)
		return
	}
	result, err := b.llm.ReviewTrade(ctx, trade)
	if err != nil {
		log.Printf("review %s: LLM: %v", ticker, err)
		return
	}
	b.Send(i18n.T(b.lang, i18n.KeyTradeReviewResultTitle, ticker, result))
}

// handleReview is /review TICKER's manual entry point: review the most
// recent fully closed round trip for ticker, regardless of when it closed
// (unlike the automatic path, which only fires right after a closing sell).
// Mirrors /check's placeholder-then-result shape since this is also a
// one-shot LLM call.
func (b *Bot) handleReview(ctx context.Context, args string) {
	ticker := strings.ToUpper(strings.TrimSpace(args))
	if ticker == "" {
		b.Send(i18n.T(b.lang, i18n.KeyReviewUsage))
		return
	}

	trade, ok, err := b.buildClosedTradeReview(ticker)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyQueryFailed, err))
		return
	}
	if !ok {
		b.Send(i18n.T(b.lang, i18n.KeyReviewNoClosedTrade, ticker))
		return
	}

	b.Send(i18n.T(b.lang, i18n.KeyAnalyzingTicker, ticker))
	result, err := b.llm.ReviewTrade(ctx, trade)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyLLMFailed, err))
		return
	}
	b.Send(i18n.T(b.lang, i18n.KeyTradeReviewResultTitle, ticker, result))
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

// handleInsight is Phase 3.6's portfolio-level analysis command: unlike
// /recommend (a per-candidate scanning view) or /check (a single ticker),
// this steps back to look at the whole set of holdings together —
// concentration risk, whether each position's original thesis still holds,
// add/reduce/stop-loss suggestions. Reuses fetchStockData/loadEarnings
// exactly like /recommend and RunDailyReport do (same attach-and-render
// StockData fields: technicals, fundamentals, earnings, cost basis) rather
// than building a separate data-gathering path.
func (b *Bot) handleInsight(ctx context.Context) {
	positions, err := b.db.GetPositions()
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyQueryFailed, err))
		return
	}
	if len(positions) == 0 {
		b.Send(i18n.T(b.lang, i18n.KeyPortfolioEmpty))
		return
	}

	b.Send(i18n.T(b.lang, i18n.KeyAnalyzing))

	tickers := make([]string, len(positions))
	positionsMap := make(map[string]db.Position, len(positions))
	for i, p := range positions {
		tickers[i] = p.Ticker
		positionsMap[p.Ticker] = p
	}

	earnings := b.loadEarnings(tickers)
	stocks := b.fetchStockData(tickers, true, positionsMap, earnings, nil, nil)

	theses := b.loadTheses(tickers)
	vsSPY := b.loadVsSPY(stocks, positionsMap)
	for i := range stocks {
		ticker := stocks[i].Quote.Ticker
		if th, ok := theses[ticker]; ok {
			stocks[i].Thesis = &th
		}
		if v, ok := vsSPY[ticker]; ok {
			stocks[i].VsSPY = &v
		}
	}

	cash, haveCash, err := b.loadCash()
	if err != nil {
		log.Printf("insight: load cash: %v", err)
	}

	result, err := b.llm.InsightPortfolio(ctx, stocks, cash, haveCash)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyLLMFailed, err))
		return
	}
	b.Send(i18n.T(b.lang, i18n.KeyInsightResultTitle, result))
}

// cashSettingKey is the db.settings key /cash reads/writes — see
// db.GetSetting/SetSetting's Phase 3.6 doc comment.
const cashSettingKey = "cash_balance"

// handleCash manages the user's manually-declared cash balance (Phase 3.6).
// With no argument it reports the current value; with one, it sets it.
// Deliberately never touched by /buy or /sell (see PLAN.md's Phase 3.6
// "現金水位" item) — transactions don't record where the money came from,
// so auto-adjusting cash from them would drift from reality quickly. This
// is a purely user-maintained reference value, fed only into /insight (see
// handleInsight) — never into /recommend, so the model doesn't see idle
// cash and start nudging toward "put it to work."
func (b *Bot) handleCash(args string) {
	args = strings.TrimSpace(args)
	if args == "" {
		cash, ok, err := b.loadCash()
		if err != nil {
			b.Send(i18n.T(b.lang, i18n.KeyQueryFailed, err))
			return
		}
		if !ok {
			b.Send(i18n.T(b.lang, i18n.KeyCashNotSet))
			return
		}
		b.Send(i18n.T(b.lang, i18n.KeyCashCurrent, cash))
		return
	}

	amount, err := strconv.ParseFloat(args, 64)
	if err != nil || amount < 0 {
		b.Send(i18n.T(b.lang, i18n.KeyCashUsage))
		return
	}
	if err := b.db.SetSetting(cashSettingKey, strconv.FormatFloat(amount, 'f', 2, 64)); err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyCashSetFailed, err))
		return
	}
	b.Send(i18n.T(b.lang, i18n.KeyCashSetSuccess, amount))
}

// loadCash returns the user's declared cash balance, or ok=false if /cash
// has never been run.
func (b *Bot) loadCash() (float64, bool, error) {
	raw, ok, err := b.db.GetSetting(cashSettingKey)
	if err != nil || !ok {
		return 0, ok, err
	}
	amount, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false, err
	}
	return amount, true, nil
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
		sb.WriteString(render.Fundamentals(b.lang, fd))
		sb.WriteString("\n\n")
	}
	if st != nil {
		sb.WriteString(render.FinancialStatement(b.lang, st))
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

// handleThesis manages the Phase 3.6 expansion's holding-thesis journal:
// "/thesis TICKER" alone queries the currently recorded rationale, "/thesis
// TICKER free text" sets/overwrites it wholesale (see db.SetThesis's doc
// comment for why there's no history). Deliberately fed only into /insight
// (see handleInsight's loadTheses call) — never /recommend, so the model
// challenges the user's stated thesis instead of confirming it.
func (b *Bot) handleThesis(args string) {
	args = strings.TrimSpace(args)
	if args == "" {
		b.Send(i18n.T(b.lang, i18n.KeyThesisUsage))
		return
	}

	parts := strings.SplitN(args, " ", 2)
	ticker := strings.ToUpper(strings.TrimSpace(parts[0]))
	if ticker == "" {
		b.Send(i18n.T(b.lang, i18n.KeyThesisUsage))
		return
	}

	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		thesis, ok, err := b.db.GetThesis(ticker)
		if err != nil {
			b.Send(i18n.T(b.lang, i18n.KeyQueryFailed, err))
			return
		}
		if !ok {
			b.Send(i18n.T(b.lang, i18n.KeyThesisNotSet, ticker, ticker))
			return
		}
		b.Send(i18n.T(b.lang, i18n.KeyThesisCurrent, ticker, thesis))
		return
	}

	thesis := strings.TrimSpace(parts[1])
	if err := b.db.SetThesis(ticker, thesis); err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyThesisSetFailed, ticker, err))
		return
	}
	b.Send(i18n.T(b.lang, i18n.KeyThesisSetSuccess, ticker, thesis))
}

// handleChat replies to a plain-text (non-command) message using the LLM
// client's persistent session, so the agent remembers earlier turns in this
// conversation — unlike /recommend and /check, which are one-shot analysis
// calls with no memory between requests. Every message is prefixed with a
// read-only summary of the watchlist/positions (see buildChatContext) so
// free-form questions like "我自選股裡最近跌最多的是哪檔" are answerable
// without giving the ACP session any tools.
func (b *Bot) handleChat(ctx context.Context, text string) {
	if url, ok := webfetch.ExtractURL(text); ok {
		b.handleChatArticle(ctx, text, url)
		return
	}

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
	b.sendPendingActionPrompts()
}

// handleChatArticle is handleChat's "article digestion" path (Phase 3): the
// user pasted a URL, possibly alongside their own comment/question in text.
// The page is fetched and its text extracted bot-side (the ACP chat session
// has no tools of its own — see internal/llm's acp_provider.go), then
// wrapped in KeyArticleTaskBlock and sent through the same persistent chat
// session as an ordinary message, so the digestion happens inline in the
// conversation rather than as a separate one-shot analysis call. A fetch
// failure (dead link, paywall, JS-only page — see webfetch.Fetch) is
// reported directly rather than forwarded to the LLM, since there's nothing
// useful for it to reason about without the article text.
func (b *Bot) handleChatArticle(ctx context.Context, text, url string) {
	b.Send(i18n.T(b.lang, i18n.KeyFetchingArticle))

	article, err := webfetch.Fetch(ctx, url)
	if err != nil {
		log.Printf("chat: article fetch %s: %v", url, err)
		b.Send(i18n.T(b.lang, i18n.KeyArticleFetchFailed, err))
		return
	}

	b.Send(i18n.T(b.lang, i18n.KeyThinking))

	prompt := i18n.T(b.lang, i18n.KeyArticleTaskBlock, article.Title, url, article.Text, text)
	if ctxBlock := b.buildChatContext(); ctxBlock != "" {
		prompt = ctxBlock + prompt
	}

	reply, err := b.llm.Chat(ctx, prompt)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyChatFailed, err))
		return
	}
	b.Send(reply)
	b.sendPendingActionPrompts()
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

// handleReset clears the persistent chat session so the next message starts
// a fresh conversation with no memory of earlier turns.
func (b *Bot) handleReset() {
	b.llm.ResetChat()
	b.Send(i18n.T(b.lang, i18n.KeyResetDone))
}
