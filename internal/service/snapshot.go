package service

import (
	"math"
	"sort"
	"strings"
	"time"
	"unicode"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/llm"
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

// PriceEventFacts converts a detected event into the fact sheet the LLM
// prompt takes. Two structs rather than one shared type is deliberate:
// internal/llm doesn't import internal/signals (see llm.StrategyHitInfo for
// the same split), so this three-line copy is what keeps that boundary.
func PriceEventFacts(ev signals.PriceEvent) llm.PriceEventFacts {
	return llm.PriceEventFacts{
		Ticker:              ev.Ticker,
		GapPct:              ev.GapPct,
		ChangePct:           ev.ChangePct,
		CumulativePct:       ev.CumulativePct,
		GapTriggered:        ev.GapTriggered,
		ChangeTriggered:     ev.ChangeTriggered,
		CumulativeTriggered: ev.CumulativeTriggered,
	}
}

// EventNewsWindow is how far from the event's own trading date a news item
// may be published and still be offered to ExplainPriceEvent as a possible
// cause. One day either side, not a tighter window: a US event dated in
// Taiwan-yesterday terms has its session news spread across two CST
// calendar days (the US open is 21:30 CST, the close 04:00 CST the next
// morning), so anything narrower would throw away news from the very
// session being explained.
const EventNewsWindow = 24 * time.Hour

// EventNewsSlots is how many news items an event summary is given, and
// EventNewsFetch how many are pulled to fill them. The gap between the two
// is the room the date filter and the syndication dedupe need: asking for
// exactly five and then throwing three away leaves the model with two, so
// the fetch is widened rather than the filters loosened. Both filters run
// before the truncation, so the five that survive are the five freshest
// distinct stories.
const (
	EventNewsSlots = 5
	EventNewsFetch = 10
)

// FilterNewsNearDate keeps only the news published within EventNewsWindow of
// date (a CST "2006-01-02" trading date), dropping anything older — Phase 20
// 後續 PR2's fix for the summaries' main quality problem. The feed is
// correctly ordered newest-first, but a ticker's top-5 routinely contained
// 4–7-day-old stories, and the prompt asks the model to infer a cause from
// whatever news it's given, so it dutifully blamed last week's headline for
// today's gap. Filtering to nothing is the intended outcome for a move with
// no news behind it: the prompt already has a "no related news, cause
// unknown" branch that until now was almost never reached.
//
// Items with no publish timestamp are dropped rather than kept — an undated
// item can't be shown to be recent, and this is the one place where being
// wrong about recency is the whole failure mode. An unparseable date leaves
// news untouched (a filter bug must not silently starve the prompt).
func FilterNewsNearDate(news []data.NewsItem, date string) []data.NewsItem {
	day, err := time.ParseInLocation("2006-01-02", date, cst)
	if err != nil {
		return news
	}
	var out []data.NewsItem
	for _, n := range news {
		if n.PublishedAt.IsZero() {
			continue
		}
		p := n.PublishedAt.In(cst)
		published := time.Date(p.Year(), p.Month(), p.Day(), 0, 0, 0, 0, cst)
		if diff := published.Sub(day); diff <= EventNewsWindow && diff >= -EventNewsWindow {
			out = append(out, n)
		}
	}
	return out
}

// headlineDupThreshold is how similar two normalized headlines must be
// (Dice coefficient over rune bigrams) before the later one is treated as a
// syndicated copy of the earlier. 0.8 is calibrated for the case that
// motivated this — the same wire story carried by several outlets, differing
// by an outlet tag or a couple of characters — and deliberately sits well
// above where two genuinely different stories about the same company land.
const headlineDupThreshold = 0.8

// DedupeHeadlines drops news whose headline is a near-copy of an earlier
// one's, keeping the first (the feed is newest-first, so the first is the
// freshest). Phase 20 後續 PR4: TW news comes from Google News RSS, which
// returns the same story from several outlets — one of ten sampled TW
// tickers had the same headline three times inside its top five — and TW
// items carry no Summary at all (KeyNewsSummaryLine never fires there, 50/50
// sampled items were empty), so the headline is the only thing the model
// gets. With only five slots and nothing but titles in them, spending three
// on one story is most of the ticker's evidence gone.
//
// Not TW-gated: a duplicate headline is noise in any market, and the check
// is a few string comparisons over at most a handful of items.
func DedupeHeadlines(news []data.NewsItem) []data.NewsItem {
	var out []data.NewsItem
	var kept []string
	for _, n := range news {
		norm := normalizeHeadline(n.Headline)
		dup := false
		for _, k := range kept {
			if diceSimilarity(norm, k) >= headlineDupThreshold {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		out = append(out, n)
		kept = append(kept, norm)
	}
	return out
}

// normalizeHeadline strips a headline down to what two syndications of the
// same story share: the trailing " - 媒體名" outlet tag Google News RSS
// appends, then punctuation, spacing and case. What's left is letters and
// digits, which for a CJK headline is the characters themselves.
func normalizeHeadline(h string) string {
	if i := strings.LastIndex(h, " - "); i > 0 {
		h = h[:i]
	}
	var sb strings.Builder
	for _, r := range strings.ToLower(h) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// diceSimilarity is the Dice coefficient over the two strings' rune bigram
// sets — 1 for identical, 0 for nothing in common. Chosen over an edit
// distance because it needs no allocation-heavy matrix and behaves the same
// on CJK (where a bigram is two characters, already a meaningful unit) as on
// English; strings under two runes fall back to equality, having no bigrams.
func diceSimilarity(a, b string) float64 {
	ar, br := []rune(a), []rune(b)
	if len(ar) < 2 || len(br) < 2 {
		return boolToFloat(a == b)
	}
	seen := make(map[string]bool, len(ar))
	for i := 0; i+1 < len(ar); i++ {
		seen[string(ar[i:i+2])] = true
	}
	matches := 0
	other := make(map[string]bool, len(br))
	for i := 0; i+1 < len(br); i++ {
		bg := string(br[i : i+2])
		if other[bg] {
			continue
		}
		other[bg] = true
		if seen[bg] {
			matches++
		}
	}
	return 2 * float64(matches) / float64(len(seen)+len(other))
}

func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
