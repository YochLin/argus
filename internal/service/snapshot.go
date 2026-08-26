package service

import (
	"math"
	"sort"
	"time"

	"argus/internal/db"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/signals"
)

// staleQuoteAge is RunClosingSnapshot's holiday guard: a quote timestamped
// older than this is the previous session's data (a market holiday still
// fires the cron), and saving it would file stale data under today's date.
const staleQuoteAge = 12 * time.Hour

// SnapshotStore is the persistence boundary for closing-snapshot data —
// per-ticker daily bars, recent closes for the cumulative-decline check,
// and net-worth totals.
type SnapshotStore interface {
	SaveSnapshot(db.DailySnapshot) error
	GetRecentCloses(ticker string, n int) ([]float64, error)
	GetPositions() ([]db.Position, error)
	SaveNetWorthSnapshot(date string, m market.MarketID, total float64) error
}

// Snapshotter is the narrower boundary SnapshotBenchmarkTo needs — just
// enough to target either the live db (b.db) or the paper-trading db
// (b.paperDB) with the exact same fetch/stale-guard logic, mirroring
// bot.snapshotBenchmarkTo's target *db.DB param.
type Snapshotter interface {
	SaveSnapshot(db.DailySnapshot) error
}

// SnapshotService is Phase 24 Stage 1's Report & Snapshot Service slice:
// the pure quote-fetch/stale-guard/price-event-detect/save core of
// RunClosingSnapshot, plus the benchmark and net-worth snapshot helpers it
// shares with paper-trading's own closing job (runPaperClose). The LLM
// writeup + Telegram push half of price-event handling (recordPriceEvents)
// stays in bot.go — formatting/channel side effects belong in the adapter.
type SnapshotService struct {
	store  SnapshotStore
	quotes RiskQuoteReader
}

func NewSnapshotService(store SnapshotStore, quotes RiskQuoteReader) *SnapshotService {
	return &SnapshotService{store: store, quotes: quotes}
}

// ClosingQuotes is FetchClosingQuotes' result: today's validated closing
// prices, keyed by ticker, plus any price events detected along the way.
type ClosingQuotes struct {
	Prices      map[string]float64
	PriceEvents []signals.PriceEvent
}

// FetchClosingQuotes fetches, stale-guards, and saves a daily_snapshots row
// for each of tickers, detecting single-day and cumulative-decline price
// events along the way (Phase 20). A single ticker's fetch/save failure only
// logs and moves on to the next — same "one bad ticker doesn't block the
// rest" convention the rest of this job uses. Moved verbatim from
// RunClosingSnapshot's per-ticker loop.
func (s *SnapshotService) FetchClosingQuotes(tickers []string, m market.MarketID, date string) ClosingQuotes {
	thresholds := signals.DefaultEventThresholds(m)
	prices := make(map[string]float64, len(tickers))
	var hits []signals.PriceEvent
	for _, t := range tickers {
		q, err := s.quotes.GetQuote(t)
		if err != nil {
			logger.Errorf("closing snapshot: quote %s: %v", t, err)
			continue
		}
		if time.Since(q.Timestamp) > staleQuoteAge {
			logger.Warnf("closing snapshot: %s quote is stale (%s), skipping (holiday?)", t, q.Timestamp.Format(time.RFC3339))
			continue
		}
		prices[t] = q.Price
		var windowAgoClose float64
		if closes, err := s.store.GetRecentCloses(t, thresholds.CumulativeWindowDays); err != nil {
			logger.Errorf("closing snapshot: recent closes %s: %v", t, err)
		} else if len(closes) == thresholds.CumulativeWindowDays {
			windowAgoClose = closes[0]
		}
		if ev := signals.CheckPriceEvent(q, windowAgoClose, thresholds); ev != nil {
			hits = append(hits, *ev)
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
		if err := s.store.SaveSnapshot(snap); err != nil {
			logger.Errorf("closing snapshot: save %s: %v", t, err)
		}
	}
	return ClosingQuotes{Prices: prices, PriceEvents: hits}
}

// SnapshotBenchmarkTo records benchTicker's closing price into target under
// date, so /track's relative-to-market hit rate (Phase 3.8) has same-day
// benchmark data without ever needing to replay history through a live API
// call. target varies (b.db for the real dashboard, b.paperDB for the paper
// account's BenchmarkReplay, Phase 11 PR3/PR4) but the quote source doesn't
// — both go through the same fetch/stale-guard code path. Moved verbatim
// from bot.snapshotBenchmarkTo.
func (s *SnapshotService) SnapshotBenchmarkTo(target Snapshotter, benchTicker, date string) {
	q, err := s.quotes.GetQuote(benchTicker)
	if err != nil {
		logger.Errorf("closing snapshot: benchmark %s: %v", benchTicker, err)
		return
	}
	if time.Since(q.Timestamp) > staleQuoteAge {
		logger.Warnf("closing snapshot: benchmark %s quote is stale (%s), skipping (holiday?)", benchTicker, q.Timestamp.Format(time.RFC3339))
		return
	}
	snap := db.DailySnapshot{
		Ticker:        benchTicker,
		Date:          date,
		Open:          q.Open,
		Close:         q.Price,
		High:          q.High,
		Low:           q.Low,
		Volume:        q.Volume,
		ChangePercent: q.ChangePercent,
	}
	if err := target.SaveSnapshot(snap); err != nil {
		logger.Errorf("closing snapshot: save benchmark %s: %v", benchTicker, err)
	}
}

// RecordNetWorthSnapshot totals market m's open positions' value as of the
// closing snapshot and stores it dated the same day (Phase 6: per-market
// row, never summed across markets). prices reuses the quotes
// FetchClosingQuotes already fetched for watchlist tickers (positions are
// auto-added to the watchlist on /buy, so this covers the common case); any
// position ticker missing from it gets a direct quote fetch as a fallback.
// Moved verbatim from bot.recordNetWorthSnapshot.
func (s *SnapshotService) RecordNetWorthSnapshot(m market.MarketID, date string, prices map[string]float64) {
	positions, err := s.store.GetPositions()
	if err != nil {
		logger.Errorf("net worth snapshot: positions: %v", err)
		return
	}

	var total float64
	var haveAny bool
	for _, p := range positions {
		if market.Of(p.Ticker) != m {
			continue
		}
		haveAny = true
		price, ok := s.priceFor(p.Ticker, prices)
		if !ok {
			continue
		}
		total += p.Shares * price
	}
	if !haveAny {
		return
	}
	if err := s.store.SaveNetWorthSnapshot(date, m, total); err != nil {
		logger.Errorf("net worth snapshot: save: %v", err)
	}
}

func (s *SnapshotService) priceFor(ticker string, prices map[string]float64) (float64, bool) {
	if p, ok := prices[ticker]; ok {
		return p, true
	}
	q, err := s.quotes.GetQuote(ticker)
	if err != nil {
		logger.Errorf("quote %s: %v", ticker, err)
		return 0, false
	}
	return q.Price, true
}

// priceEventMoveSize is recordPriceEvents' writeup-priority ranking key —
// the largest magnitude among the thresholds that actually fired, so a
// ticker with only a big cumulative decline still competes fairly against
// one with a large single-day move. Only triggered numbers count: since
// Phase 20 後續 PR3 a PriceEvent also carries the day's non-triggering
// values, and ranking on those would let an untriggered move decide who
// gets one of the run's LLM writeups. Exported for bot.recordPriceEvents'
// ranking step, which stays in the adapter (Telegram/LLM side effects).
func PriceEventMoveSize(ev signals.PriceEvent) float64 {
	var size float64
	if ev.GapTriggered {
		size = math.Max(size, math.Abs(ev.GapPct))
	}
	if ev.ChangeTriggered {
		size = math.Max(size, math.Abs(ev.ChangePct))
	}
	if ev.CumulativeTriggered {
		size = math.Max(size, math.Abs(ev.CumulativePct))
	}
	return size
}

// SortPriceEventsByMoveSize orders events by PriceEventMoveSize descending,
// in place — bot.recordPriceEvents' ranking step before the writeup cap.
func SortPriceEventsByMoveSize(events []signals.PriceEvent) {
	sort.Slice(events, func(i, j int) bool {
		return PriceEventMoveSize(events[i]) > PriceEventMoveSize(events[j])
	})
}
