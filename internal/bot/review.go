package bot

import (
	"context"
	"strings"
	"time"

	"argus/internal/i18n"
	"argus/internal/llm"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/service"
)

// tradeRound is a fully closed round trip in a ticker's transaction history.
// Re-exported from internal/service for bot backwards compatibility.
type tradeRound = service.Round

var (
	lastClosedRound  = service.LastClosedRound
	weightedAvgPrice = service.WeightedAvgPrice
)

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
// buildClosedTradeReview assembles the most recent fully closed round trip
// in ticker into an llm.ClosedTrade. stopPrice (Phase 3.11 PR1 §3.5) is the
// position's stop price at the moment it closed, supplied by the caller —
// this function has no way to recover it itself once the positions row is
// gone, so reviewClosedTrade (the automatic post-sell path) passes what
// recordSell captured right before the close, while handleReview (the
// manual /review path, which can run long after a ticker closed) passes 0:
// there is no way to recover a historical stop price after the fact, so a
// manually reviewed trade simply renders without the R-multiple line.
func (b *Bot) buildClosedTradeReview(ticker string, stopPrice float64) (llm.ClosedTrade, bool, error) {
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
		StopPrice:   stopPrice,
		Snapshots:   b.reviewSnapshots(ticker, round),
	}

	if high, low, ok, err := b.db.GetCloseExtremes(ticker, round.StartDate, round.EndDate); err != nil {
		logger.Errorf("review %s: close extremes: %v", ticker, err)
	} else if ok {
		trade.PeriodHigh = high
		trade.PeriodLow = low
	}

	if entryPrice := weightedAvgPrice(round.Legs, "BUY"); entryPrice > 0 {
		exitPrice := weightedAvgPrice(round.Legs, "SELL")
		bench := service.BenchmarkFor(market.Of(ticker))
		spyStart, startOK, startErr := b.db.GetSnapshotClose(bench, round.StartDate)
		spyEnd, endOK, endErr := b.db.GetSnapshotClose(bench, round.EndDate)
		if startErr != nil || endErr != nil {
			logger.Errorf("review %s: %s close: start err=%v end err=%v", ticker, bench, startErr, endErr)
		} else if startOK && endOK {
			vs := computeVsSPY(exitPrice, entryPrice, spyEnd, spyStart, bench)
			trade.VsSPY = &vs
		}
	}

	// The reviewed round's own thesis (Phase 27 P1b) — not the ticker's latest,
	// which for an older round would be a later round's.
	if thesis, ok, err := service.RoundThesis(b.db, ticker, service.SegmentRounds(txs), round.StartDate); err != nil {
		logger.Errorf("review %s: thesis: %v", ticker, err)
	} else if ok {
		trade.Thesis = &thesis.Text
	}

	if recs, err := b.db.GetRecommendationsForTicker(ticker, round.StartDate, round.EndDate); err != nil {
		logger.Errorf("review %s: recommendations: %v", ticker, err)
	} else {
		for _, r := range recs {
			trade.Recommendations = append(trade.Recommendations, llm.TradeRecommendation{Date: r.Date, Action: r.Action, Reason: r.Reason})
		}
	}

	return trade, true, nil
}

// reviewClosedTrade is the automatic sell-review path, triggered by
// handleSell/executePendingAction/ExecuteSell right after a sell fully
// closes a position, always via "go b.reviewClosedTrade(ctx, ...)" so the
// caller's own handler can return immediately. ctx is detached from the
// caller's cancellation with context.WithoutCancel — ExecuteSell in
// particular passes an http.Request's context, which net/http cancels the
// instant the handler returns, well before this goroutine's (multi-minute)
// LLM call finishes; the callTimeout budget inside llm.Client.prompt still
// bounds it, just no longer tied to a request/dispatch context that dies
// first. Log-only on any failure — the user already has their sell
// confirmation, so a second failure alert about the review itself would be
// noise for something that isn't the trade record. See handleReview for the
// manual /review TICKER path, which reports failures to the user instead
// (and is also the way to backfill a review that got dropped this way).
func (b *Bot) reviewClosedTrade(ctx context.Context, ticker string, stopPrice float64) {
	// ExecuteSell fires this in its own goroutine on every full close, with
	// no synchronization back to the caller — a test Bot built without an
	// llm client (most don't need one) can still trigger a full-close sell
	// and leak this goroutine, which then panics on b.llm.ReviewTrade's nil
	// receiver at some later, unrelated point in the same test binary.
	// Live-observed 2026-08-29 CI (internal/bot, SIGSEGV in
	// llm.(*Client).ReviewTrade). b.llm is never nil in production — Boot
	// constructs it unconditionally — so this guard only ever fires in
	// exactly that test scenario.
	if b.llm == nil {
		return
	}
	ctx = context.WithoutCancel(ctx)
	trade, ok, err := b.buildClosedTradeReview(ticker, stopPrice)
	if err != nil {
		logger.Errorf("review %s: %v", ticker, err)
		return
	}
	if !ok {
		logger.Errorf("review %s: no closed round found right after closing (unexpected)", ticker)
		return
	}
	result, lesson, err := b.llm.ReviewTrade(ctx, trade)
	if err != nil {
		logger.Errorf("review %s: LLM: %v", ticker, err)
		return
	}
	b.saveLesson(ticker, lesson)
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

	trade, ok, err := b.buildClosedTradeReview(ticker, 0)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyQueryFailed, err))
		return
	}
	if !ok {
		b.Send(i18n.T(b.lang, i18n.KeyReviewNoClosedTrade, ticker))
		return
	}

	b.Send(i18n.T(b.lang, i18n.KeyAnalyzingTicker, ticker))
	result, lesson, err := b.llm.ReviewTrade(ctx, trade)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyLLMFailed, err))
		return
	}
	b.saveLesson(ticker, lesson)
	b.Send(i18n.T(b.lang, i18n.KeyTradeReviewResultTitle, ticker, result))
}

// saveLesson persists Phase 3.9's parsed trade-review takeaway (see
// llm.Client.ReviewTrade's lesson return value) under today's date. A no-op
// when lesson is "" (the model omitted the marker) rather than storing an
// empty row; a save failure only logs — the user already has the full
// review text either way, so a second Telegram message about a storage
// failure would be noise about something that doesn't affect what they
// just read.
func (b *Bot) saveLesson(ticker, lesson string) {
	if lesson == "" {
		return
	}
	if err := b.db.SaveLesson(ticker, todayDate(), lesson); err != nil {
		logger.Errorf("save lesson %s: %v", ticker, err)
	}
}
