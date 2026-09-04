package bot

import (
	"context"
	"sort"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/llm"
	"argus/internal/logger"
	"argus/internal/market"
)

// Phase 26's post-sell follow-up review constants (see
// docs/phase-26-sell-followup.md §6) — starting-point values, not
// calibrated against real trigger frequency yet, same caveat convention as
// priceEventWriteupCap (jobs.go).
const (
	// sellFollowupTradingDays is both the "how long to wait" trigger and
	// the fixed evaluation window size (§4.2): the follow-up always looks
	// at exactly the first 5 trading days after the exit, even if a retry
	// means it actually runs later — so a late run reaches the same
	// conclusion an on-time run would have.
	sellFollowupTradingDays = 5
	// sellFollowupMinAgeDays/sellFollowupMaxAgeDays bound the calendar-day
	// coarse filter (§4.2) used before ever fetching a candidate's history:
	// 5 calendar days is the minimum for 5 trading days to have elapsed at
	// all; 30 is the give-up line (and, on first deploy, the anti-backfill
	// gate that keeps a year of old closes from all firing at once).
	sellFollowupMinAgeDays = 5
	sellFollowupMaxAgeDays = 30
	// sellFollowupPerRunCap caps how many candidates a single
	// checkSellFollowups call processes, same shape as priceEventWriteupCap.
	sellFollowupPerRunCap = 3
	// sellFollowupVerdictThresholdUS/TW are followupVerdict's classification
	// bands (§4.4), on a 5-trading-day scale — TW's wider band accounts for
	// its generally higher single-name volatility, same reasoning as the
	// existing US/TW threshold splits elsewhere in internal/signals.
	sellFollowupVerdictThresholdUS = 5.0
	sellFollowupVerdictThresholdTW = 7.0
)

// followupVerdict classifies a closed trade's 5-trading-day-later outcome
// from pctSinceExit (the 5th trading day's close vs the exit's own
// weighted-average price) into one of three labels, pure and independent of
// any LLM call so the follow-up message stays meaningful even if the LLM
// call itself fails: "sold_early" (price kept rising — money left on the
// table), "good_exit" (price fell — the exit, or stop-loss, was right), or
// "neutral" (roughly a wash).
func followupVerdict(pctSinceExit float64, m market.MarketID) string {
	threshold := sellFollowupVerdictThresholdUS
	if m == market.TW {
		threshold = sellFollowupVerdictThresholdTW
	}
	switch {
	case pctSinceExit >= threshold:
		return "sold_early"
	case pctSinceExit <= -threshold:
		return "good_exit"
	default:
		return "neutral"
	}
}

// verdictLabel renders a followupVerdict result as one localized sentence
// for the Telegram result title.
func verdictLabel(lang i18n.Lang, verdict string) string {
	switch verdict {
	case "sold_early":
		return i18n.T(lang, i18n.KeyTradeFollowupVerdictSoldEarly)
	case "good_exit":
		return i18n.T(lang, i18n.KeyTradeFollowupVerdictGoodExit)
	default:
		return i18n.T(lang, i18n.KeyTradeFollowupVerdictNeutral)
	}
}

// postExitWindow finds the n daily candles strictly after exitDate (candles
// is expected oldest-first, as data.HistoryProvider.GetHistory returns it)
// and returns that window's last close plus its high/low, or ok=false if
// fewer than n such candles exist yet. Because daily candles only exist for
// sessions that actually traded, "n candles after exitDate" is exactly "n
// trading days after the exit" — US holidays, TW multi-day breaks, and
// individual-ticker trading halts are all automatically correct with no
// calendar of their own (§4.2). Always takes the *first* n candles after
// exitDate (not the most recent n), so a follow-up that runs late — after a
// retry — evaluates the same fixed window an on-time run would have.
func postExitWindow(candles []data.Candle, exitDate string, n int) (last, high, low float64, ok bool) {
	var post []data.Candle
	for _, c := range candles {
		if c.Date.Format("2006-01-02") > exitDate {
			post = append(post, c)
		}
	}
	if len(post) < n {
		return 0, 0, 0, false
	}
	window := post[:n]
	high, low = window[0].High, window[0].Low
	for _, c := range window {
		if c.High > high {
			high = c.High
		}
		if c.Low < low {
			low = c.Low
		}
	}
	return window[n-1].Close, high, low, true
}

// dateOffset shifts a "2006-01-02" date string by days (negative moves
// back), returning dateStr unchanged if it doesn't parse — this only ever
// receives dates checkSellFollowups itself formats, so that fallback never
// actually triggers in practice.
func dateOffset(dateStr string, days int) string {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return dateStr
	}
	return t.AddDate(0, 0, days).Format("2006-01-02")
}

// checkSellFollowups is Phase 26's RunClosingSnapshot tail step (see
// docs/phase-26-sell-followup.md §4.1/§4.3): find every ticker in market m
// whose most recent fully closed round trip (lastClosedRound) exited within
// the last sellFollowupMinAgeDays..sellFollowupMaxAgeDays calendar days,
// hasn't already been followed up on (sell_followups' (ticker, exit_date)
// unique index), and hasn't been bought back into since — then hand the
// oldest-exit-first sellFollowupPerRunCap of them to followupClosedTrade.
//
// Deliberately reuses GetAllTransactions + the existing lastClosedRound
// pure function instead of adding a date-range query (§4.3): transactions
// has no date index and a single-user bot's whole table is a few hundred
// rows at most, so grouping in memory costs nothing extra here.
//
// Like recordPriceEvents, this is a silent background job: every failure
// just logs and moves on to the next candidate, since it runs unattended
// off the scheduler.
func (b *Bot) checkSellFollowups(ctx context.Context, m market.MarketID, today string) {
	if b.llm == nil {
		return
	}
	txs, err := b.db.GetAllTransactions()
	if err != nil {
		logger.Errorf("sell followup: all transactions: %v", err)
		return
	}

	byTicker := make(map[string][]db.Transaction)
	for _, tx := range txs {
		if market.Of(tx.Ticker) != m {
			continue
		}
		byTicker[tx.Ticker] = append(byTicker[tx.Ticker], tx)
	}

	minExit, maxExit := dateOffset(today, -sellFollowupMaxAgeDays), dateOffset(today, -sellFollowupMinAgeDays)

	type candidate struct {
		ticker string
		round  tradeRound
	}
	var candidates []candidate
	for ticker, ttxs := range byTicker {
		round, ok := lastClosedRound(ttxs)
		if !ok || round.EndDate < minExit || round.EndDate > maxExit {
			continue
		}
		reentered := false
		for _, tx := range ttxs {
			if tx.Date > round.EndDate {
				reentered = true
				break
			}
		}
		if reentered {
			logger.Infof("sell followup: %s skipped, re-entered after exit %s", ticker, round.EndDate)
			continue
		}
		has, err := b.db.HasSellFollowup(ticker, round.EndDate)
		if err != nil {
			logger.Errorf("sell followup: dedup check %s: %v", ticker, err)
			continue
		}
		if has {
			continue
		}
		candidates = append(candidates, candidate{ticker, round})
	}

	sort.Slice(candidates, func(i, j int) bool { return candidates[i].round.EndDate < candidates[j].round.EndDate })
	if len(candidates) > sellFollowupPerRunCap {
		candidates = candidates[:sellFollowupPerRunCap]
	}

	for _, c := range candidates {
		b.followupClosedTrade(ctx, c.ticker, c.round, m, today)
	}
}

// followupClosedTrade is checkSellFollowups' per-ticker worker: pull the
// last sellFollowupTradingDays of price action since round's exit
// (postExitWindow over b.history.GetHistory — deliberately not
// daily_snapshots, which stops updating once a closed-out ticker drops off
// the watchlist, see §4.2), compute the exit-vs-now percentage and its
// followupVerdict, attach both to buildClosedTradeReview's usual sell-review
// input as an llm.TradeFollowup, and run it through the existing
// llm.Client.ReviewTrade/saveLesson pipeline. sell_followups only gets
// written after the Telegram send succeeds (§4.3), so any earlier failure —
// not enough trading-day history yet, the LLM call itself failing — simply
// leaves no row and checkSellFollowups retries the same ticker on the next
// closing snapshot, up to its own age cutoff.
func (b *Bot) followupClosedTrade(ctx context.Context, ticker string, round tradeRound, m market.MarketID, today string) {
	candles, err := b.history.GetHistory(ticker, "3mo")
	if err != nil {
		logger.Errorf("sell followup %s: history: %v", ticker, err)
		return
	}
	priceAfter, high, low, ok := postExitWindow(candles, round.EndDate, sellFollowupTradingDays)
	if !ok {
		logger.Infof("sell followup %s: fewer than %d trading days since exit %s, will retry next close", ticker, sellFollowupTradingDays, round.EndDate)
		return
	}

	exitPrice := weightedAvgPrice(round.Legs, "SELL")
	if exitPrice == 0 {
		logger.Errorf("sell followup %s: closed round has no SELL legs (unexpected)", ticker)
		return
	}
	pctSinceExit := (priceAfter - exitPrice) / exitPrice * 100
	verdict := followupVerdict(pctSinceExit, m)

	trade, ok, err := b.buildClosedTradeReview(ticker, 0)
	if err != nil {
		logger.Errorf("sell followup %s: %v", ticker, err)
		return
	}
	if !ok {
		logger.Errorf("sell followup %s: no closed round found (unexpected, checkSellFollowups just found one)", ticker)
		return
	}
	trade.Followup = &llm.TradeFollowup{
		TradingDaysAfterExit: sellFollowupTradingDays,
		ExitPrice:            exitPrice,
		PriceAfter:           priceAfter,
		HighAfter:            high,
		LowAfter:             low,
		PctSinceExit:         pctSinceExit,
		Verdict:              verdict,
	}

	result, lesson, err := b.llm.ReviewTrade(ctx, trade)
	if err != nil {
		logger.Errorf("sell followup %s: LLM: %v", ticker, err)
		return
	}
	b.saveLesson(ticker, lesson)
	b.Send(i18n.T(b.lang, i18n.KeyTradeFollowupResultTitle, ticker, sellFollowupTradingDays, verdictLabel(b.lang, verdict), result))

	f := db.SellFollowup{
		Ticker: ticker, Market: string(m), ExitDate: round.EndDate, ReviewDate: today,
		ExitPrice: exitPrice, PriceAtReview: priceAfter, PctSinceExit: pctSinceExit, Verdict: verdict,
	}
	if err := b.db.SaveSellFollowup(f); err != nil {
		logger.Errorf("sell followup %s: save: %v", ticker, err)
	}
}
