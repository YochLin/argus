package bot

import (
	"context"
	"math"
	"strings"
	"time"

	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/notification"
	"argus/internal/service"
	"argus/internal/signals"
)

// priceEventWriteupCap caps how many of a single RunClosingSnapshot run's
// triggered price events get an LLM writeup + Telegram push, chosen by move
// size descending (docs/phase-20-price-event-log.md §2 "單次觸發上限"). A
// starting-point constant per that doc's §6 risk note, not calibrated
// against real trigger frequency yet — revisit if it's routinely maxed out.
const priceEventWriteupCap = 3

// RunClosingSnapshot records the just-closed session's OHLCV for every
// watchlist ticker in market m into daily_snapshots. Called by the
// scheduler once per market: 05:30 CST for US (after the US close) and
// 14:30 CST Mon-Fri for TW (after the TW close) — see
// docs/phase-6-tw-market.md §3.3. Date semantics differ by market: at
// 05:30 CST the US trading date is still Taiwan's "yesterday" (hence the
// -1 day), but the TW closing snapshot runs the same afternoon as the TW
// session it's recording, so it's dated today.
// It's a silent background job: results go to the DB and errors to the log,
// not to Telegram.
func (b *Bot) RunClosingSnapshot(ctx context.Context, m market.MarketID) {
	defer b.recoverJobPanic("closing snapshot")

	tickers, err := b.db.GetWatchlistByMarket(m)
	if err != nil {
		logger.Errorf("closing snapshot: watchlist: %v", err)
		b.Send(i18n.T(b.lang, i18n.KeyWatchlistQueryFailed, err))
		return
	}

	now := time.Now().In(cst)
	date := now.Format("2006-01-02")
	if m == market.US {
		date = now.AddDate(0, 0, -1).Format("2006-01-02")
	}
	closing := b.snapshots().FetchClosingQuotes(tickers, m, date)
	prices := closing.Prices
	logger.Infof("closing snapshot: done for %s market=%s (%d tickers)", date, m, len(tickers))

	b.recordPriceEvents(ctx, closing.PriceEvents, m, date)

	b.snapshotBenchmark(date, m)
	b.recordNetWorthSnapshot(date, m, prices)
	b.runPaperClose(m, date, prices)

	// Buy alerts get checked here too, not just from runDailyReport — see
	// checkBuyAlerts' doc comment for why a ticker the user doesn't hold
	// yet warrants both of a market's daily checkpoints rather than riding
	// along with the position-only exit-discipline sweep.
	if buyAlerts, err := b.db.GetBuyAlertsByMarket(m); err != nil {
		logger.Errorf("closing snapshot: buy alerts: %v", err)
	} else {
		b.checkBuyAlerts(buyAlerts, prices)
	}

	// Phase 26: post-sell follow-up review, 5 trading days after a full
	// close (see docs/phase-26-sell-followup.md) — rides the closing
	// snapshot for both markets rather than a new cron entry (§4.1).
	b.checkSellFollowups(ctx, m, date)

	// Phase 12: US-only (options.go's OptionChainProvider is US-only, see
	// its own doc comment) — both the expiry-scan confirm-flow and the
	// daily ATM IV snapshot ride the US closing snapshot rather than
	// getting a cron entry of their own.
	if m == market.US {
		b.checkOptionExpiry(prices, date)
		b.recordDailyATMIV(tickers, prices, date)
	}
	if m == market.TW {
		b.syncDCAFundValues(ctx, date)
	}
}

// recordPriceEvents is RunClosingSnapshot's Phase 20 tail step (see
// docs/phase-20-price-event-log.md §4.4): dedup hits against price_events'
// (ticker, date) uniqueness (HasPriceEvent — guards a rerun from
// re-triggering the same day), rank the rest by move size, LLM-writeup and
// push the top priceEventWriteupCap, and store everything past that cap with
// no summary plus one overflow notice. Called after every watchlist
// ticker's quote is already saved to daily_snapshots, so an LLM call's
// latency or failure never delays that write — same reasoning
// RunClosingSnapshot's own doc comment gives for keeping this out of the
// per-ticker fetch loop. A single ticker's LLM/save failure only logs and
// moves on, same "one bad ticker doesn't block the rest" convention as the
// snapshot loop above it.
func (b *Bot) recordPriceEvents(ctx context.Context, hits []signals.PriceEvent, m market.MarketID, date string) {
	if len(hits) == 0 {
		return
	}

	var pending []signals.PriceEvent
	for _, ev := range hits {
		has, err := b.db.HasPriceEvent(ev.Ticker, date)
		if err != nil {
			logger.Errorf("price events: dedup check %s: %v", ev.Ticker, err)
			continue
		}
		if has {
			continue
		}
		pending = append(pending, ev)
	}
	if len(pending) == 0 {
		return
	}
	service.SortPriceEventsByMoveSize(pending)

	writeup, overflow := pending, []signals.PriceEvent(nil)
	if len(pending) > priceEventWriteupCap {
		writeup, overflow = pending[:priceEventWriteupCap], pending[priceEventWriteupCap:]
	}

	for _, ev := range writeup {
		news, _ := b.provider.GetNews(ev.Ticker, service.EventNewsFetch)
		news = (&service.NewsPicker{}).Pick(service.FilterNewsNearDate(news, date), service.EventNewsSlots)
		facts := service.PriceEventFacts(ev)
		facts.ATRMultiple, facts.VolumeRatio = b.priceEventScale(ev.Ticker, ev.ChangePct)
		summary, model, latencyMs, err := b.llm.ExplainPriceEvent(ctx, facts, news)
		b.recordPriceEventLLMRun(ev, m, news, summary, model, latencyMs)
		if err != nil {
			logger.Errorf("price events: LLM %s: %v", ev.Ticker, err)
			continue
		}
		if err := b.db.SavePriceEvent(db.PriceEvent{Ticker: ev.Ticker, Market: string(m), Date: date, GapPct: ev.GapPct, ChangePct: ev.ChangePct, CumulativePct: ev.CumulativePct, Summary: summary}); err != nil {
			logger.Errorf("price events: save %s: %v", ev.Ticker, err)
			continue
		}
		b.publishAlert("price_event", notification.LevelInfo, i18n.T(b.lang, i18n.KeyPriceEventResultTitle, ev.Ticker, summary))
	}

	if len(overflow) == 0 {
		return
	}
	var sb strings.Builder
	for _, ev := range overflow {
		if err := b.db.SavePriceEvent(db.PriceEvent{Ticker: ev.Ticker, Market: string(m), Date: date, GapPct: ev.GapPct, ChangePct: ev.ChangePct, CumulativePct: ev.CumulativePct}); err != nil {
			logger.Errorf("price events: save %s: %v", ev.Ticker, err)
			continue
		}
		sb.WriteString(i18n.T(b.lang, i18n.KeyPriceEventOverflowTickerLine, ev.Ticker, ev.GapPct, ev.ChangePct, ev.CumulativePct))
	}
	b.publishAlert("price_event", notification.LevelInfo, i18n.T(b.lang, i18n.KeyPriceEventOverflowLine, sb.String()))
}

// priceEventScale gives the event summary a sense of how unusual the move
// actually is: the day's change as a multiple of ATR(14), and volume against
// its 20-day average (Phase 20 後續 PR5). 7% is a routine day for NVDA and a
// once-a-year one for KO, and a percentage alone can't say which this was.
//
// The day's change is the numerator even when the cumulative-decline
// threshold is what fired — ATR measures a single session's range, so
// comparing a five-day slide against it would be a category error. Costs one
// GetHistory per writeup ticker, at most priceEventWriteupCap a day, and
// reuses computeTechnicals rather than recomputing ATR here (spyCloses is
// nil: relative strength isn't part of this). Returns 0, 0 on any missing
// piece — the prompt then simply omits the line rather than printing a
// multiple derived from a partial series.
func (b *Bot) priceEventScale(ticker string, changePct float64) (atrMultiple, volumeRatio float64) {
	t, candles, _ := b.computeTechnicals(ticker, nil)
	if t == nil || len(candles) == 0 {
		return 0, 0
	}
	if close := candles[len(candles)-1].Close; t.ATR14 > 0 && close > 0 {
		atrMultiple = math.Abs(changePct) / (t.ATR14 / close * 100)
	}
	return atrMultiple, t.VolumeRatio
}

// snapshotBenchmark records service.BenchmarkFor(m)'s (SPY/0050) closing price into
// daily_snapshots under the same date as the watchlist snapshot, so /track's
// relative-to-market hit rate (Phase 3.8) has same-day benchmark data to
// compare against without ever needing to replay history through a live API
// call. Thin wrapper around snapshotBenchmarkTo targeting b.db — see that
// function for the actual logic.
func (b *Bot) snapshotBenchmark(date string, m market.MarketID) {
	b.snapshotBenchmarkTo(b.db, date, m)
}

// snapshotBenchmarkTo is snapshotBenchmark's logic parameterized over which
// *db.DB to write into — Phase 11 PR3's runPaperClose calls this with
// b.paperDB so the live paper account's benchmark-overlay data (BenchmarkReplay,
// PR4) comes from the exact same fetch/stale-quote-guard code path as the
// real dashboard's, instead of a second implementation that could drift.
// Thin wrapper around service.SnapshotService.SnapshotBenchmarkTo (Phase 24
// Stage 1 Report & Snapshot Service extraction).
func (b *Bot) snapshotBenchmarkTo(target *db.DB, date string, m market.MarketID) {
	b.snapshots().SnapshotBenchmarkTo(target, service.BenchmarkFor(m), date)
}

// recordNetWorthSnapshot is a thin wrapper around
// service.SnapshotService.RecordNetWorthSnapshot (Phase 24 Stage 1 Report &
// Snapshot Service extraction).
func (b *Bot) recordNetWorthSnapshot(date string, m market.MarketID, prices map[string]float64) {
	b.snapshots().RecordNetWorthSnapshot(m, date, prices)
}
