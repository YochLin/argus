package bot

import (
	"sort"
	"strings"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/llm"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/service"
)

// stopCandidateATRMult is the ATR multiplier for the volatility-adaptive
const stopCandidateATRMult = service.StopCandidateATRMult

// stopSuggestion bundles Phase 3.11's three candidate stop-loss reference
// prices (see /stop and the /buy suggestion line) plus the latest close they
// were computed against. Low10/Low20/ATRBased are 0 when there isn't enough
// history for that particular candidate (same "0 = not enough data"
// sentinel signals.MA already uses) — callers must skip rendering a
// zero-valued candidate rather than showing a misleading $0.00.
type stopSuggestion = service.StopSuggestion

// maxRecentLessons caps how many cross-ticker general lessons
// loadRecentLessons feeds into every /recommend/daily-report prompt (Phase
// 3.9's "跨 ticker 最近 N 筆通用教訓" — see
// docs/research-tradingagents.md). A plain const, not an env var, same
// reasoning as maxExploreNominations: this is a prompt-size/relevance
// trade-off, not a user preference, and ACP's Pro/Max auth means there's no
// per-call billing pressure to tune it against.
const maxRecentLessons = 5

func dbToDataSnapshot(s *db.FundamentalSnapshot) *data.FundamentalSnapshot {
	if s == nil {
		return nil
	}
	return &data.FundamentalSnapshot{
		Ticker: s.Ticker, EPSAnnual: s.EPSAnnual, PERatio: s.PERatio, PEPercentile: s.PEPercentile,
		OCF: s.OCF, NetIncome: s.NetIncome, CashFlowQuality: s.CashFlowQuality, AsOfFiscalYearEnd: s.AsOfFiscalYearEnd,
	}
}

// computeTechnicals is a thin wrapper around service.ComputeTechnicals
// (Phase 24 Stage 1 Recommendation Pipeline Service extraction).
func (b *Bot) computeTechnicals(ticker string, spyCloses []float64) (*llm.Technicals, []data.Candle, []llm.StrategyHitInfo) {
	return service.ComputeTechnicals(b.history, ticker, spyCloses)
}

// computeStopSuggestion fetches ticker's OHLCV history and reduces it to
// stopSuggestion's three candidates, all computed against LatestClose.
func (b *Bot) computeStopSuggestion(ticker string) (stopSuggestion, bool) {
	if b.risks() == nil {
		return stopSuggestion{}, false
	}
	return b.risks().ComputeStopSuggestion(ticker)
}

// accountValue is Phase 3.11's account-size input for paper.SuggestShares: m's
// latest recorded net worth (position market value as of the last closing
// snapshot) plus m's declared cash — the same cash source /insight and
// RunWeeklyReview already use (see loadCash), not a separate live
// computation. Phase 6 makes this per-market (never summed across TWD/USD,
// see docs/phase-6-tw-market.md §3.2). ok is false only when there's no net
// worth snapshot on record at all for m (e.g. before the first closing
// snapshot has ever run for that market); missing cash just leaves it out
// rather than failing the whole lookup, same as loadCash's own callers
// already tolerate "never set".
func (b *Bot) accountValue(m market.MarketID) (float64, bool) {
	_, total, ok, err := b.db.GetLatestNetWorth(m)
	if err != nil {
		logger.Errorf("account value: net worth: %v", err)
	}
	if !ok {
		return 0, false
	}
	if cash, cashOK, err := b.loadCash(m); err != nil {
		logger.Errorf("account value: cash: %v", err)
	} else if cashOK {
		total += cash
	}
	return total, true
}

// computeMarketRegime is a thin wrapper around service.ComputeMarketRegime
// (Phase 24 Stage 1 Recommendation Pipeline Service extraction) — benchTicker/
// vixTicker resolution (SPY/0050, ^VIX) stays a bot-layer policy choice, same
// reasoning as rankAndTruncateCandidates' benchTicker param.
func (b *Bot) computeMarketRegime(m market.MarketID) *llm.MarketContext {
	return service.ComputeMarketRegime(b.history, b.provider, m, service.BenchmarkFor(m), vixTicker)
}

// loadPositions returns every open position keyed by ticker, for attaching
// cost-basis context to LLM prompts. A query failure logs and degrades to an
// empty map rather than failing the caller — recommendations without cost
// basis are still useful.
func (b *Bot) loadPositions() map[string]db.Position {
	positions, err := b.db.GetPositions()
	if err != nil {
		logger.Errorf("load positions: %v", err)
		return nil
	}
	out := make(map[string]db.Position, len(positions))
	for _, p := range positions {
		out[p.Ticker] = p
	}
	return out
}

// loadPrevRecs returns each ticker's most recent recommendation on record
// (across any past date), keyed by ticker, for Phase 3.8's recommendation
// continuity (see llm.StockData.PrevRec). Degrades to nil on a query failure
// or an empty ticker list — same optional-data pattern as
// fundamentals/earnings/positions.
func (b *Bot) loadPrevRecs(tickers []string) map[string]db.Recommendation {
	if len(tickers) == 0 {
		return nil
	}
	recs, err := b.db.GetLatestRecommendations(tickers)
	if err != nil {
		logger.Errorf("load prev recommendations: %v", err)
		return nil
	}
	return recs
}

// loadPastLessons returns every trade-review lesson on record for any
// ticker in tickers, keyed by ticker — the "same ticker: bring all of them"
// half of Phase 3.9's feedback loop (see docs/research-tradingagents.md and
// llm.StockData.PastLessons). Degrades to nil (not an error) on a query
// failure or an empty ticker list — same optional-data pattern as
// fundamentals/earnings/positions; a prompt without past-lesson context is
// still useful.
func (b *Bot) loadPastLessons(tickers []string) map[string][]db.Lesson {
	if len(tickers) == 0 {
		return nil
	}
	lessons, err := b.db.GetLessonsForTickers(tickers)
	if err != nil {
		logger.Errorf("load past lessons: %v", err)
		return nil
	}
	return lessons
}

// loadRecentLessons returns the most recent maxRecentLessons trade-review
// lessons across every ticker — the "cross ticker: recent N, general" half
// of Phase 3.9's feedback loop (see loadPastLessons above and
// llm.GenerateRecommendations' recentLessons parameter). Degrades to nil on
// a query failure, same optional-data pattern as the rest of this file.
func (b *Bot) loadRecentLessons() []llm.PastLesson {
	rows, err := b.db.GetRecentLessons(maxRecentLessons)
	if err != nil {
		logger.Errorf("load recent lessons: %v", err)
		return nil
	}
	out := make([]llm.PastLesson, len(rows))
	for i, l := range rows {
		out[i] = llm.PastLesson{Ticker: l.Ticker, Date: l.Date, Lesson: l.Lesson}
	}
	return out
}

// portfolioStocks assembles the per-position StockData that the
// portfolio-level LLM calls (/insight and the weekly review) both feed their
// prompts, scoped to a single market: quotes/technicals/fundamentals via
// fetchStockData, plus each position's earnings date, holding thesis and
// vs-benchmark return. Returns nil, nil when m has no positions at all, so
// callers skip that market's message entirely rather than sending an empty
// one. Market-scoped because mixing markets into one prompt sums TWD and USD
// position values into a single meaningless total (see
// llm.buildWeeklyReviewPrompt).
func (b *Bot) portfolioStocks(m market.MarketID, positions []db.Position) ([]llm.StockData, map[string]data.EarningsEvent) {
	var tickers []string
	positionsMap := make(map[string]db.Position, len(positions))
	for _, p := range positions {
		if market.Of(p.Ticker) != m {
			continue
		}
		tickers = append(tickers, p.Ticker)
		positionsMap[p.Ticker] = p
	}
	if len(tickers) == 0 {
		return nil, nil
	}

	earnings := b.loadEarnings(tickers)
	stocks := b.fetchStockData(tickers, true, positionsMap, earnings, nil, nil, nil)

	theses := b.loadTheses(tickers)
	vsBench := b.loadVsSPY(stocks, positionsMap)
	for i := range stocks {
		ticker := stocks[i].Quote.Ticker
		if th, ok := theses[ticker]; ok {
			stocks[i].Thesis = &th
		}
		if v, ok := vsBench[ticker]; ok {
			stocks[i].VsSPY = &v
		}
	}
	return stocks, earnings
}

// loadEarnings returns each ticker's next scheduled earnings date within
// earningsPromptWindowDays, keyed by ticker: Finnhub's real per-company date
// for a US ticker, and (2026-07-28 TW data-gap PR) data.GetTWUpcomingEarnings'
// statutory-deadline proxy for a TW one — the latter needs no API/nil-check,
// it's a pure calendar function, and a TW ticker simply never matches
// Finnhub's US-only calendar so both sources can be queried unconditionally
// without any market branching here. Finnhub's half still degrades to
// nothing (not an error) when it isn't configured or the request fails —
// same optional-data pattern as fundamentals.
func (b *Bot) loadEarnings(tickers []string) map[string]data.EarningsEvent {
	if len(tickers) == 0 {
		return nil
	}
	out := data.GetTWUpcomingEarnings(tickers, earningsPromptWindowDays, b.now())

	if b.earnings != nil {
		events, err := b.earnings.GetUpcomingEarnings(tickers, earningsPromptWindowDays)
		if err != nil {
			logger.Errorf("earnings calendar: %v", err)
		} else if out == nil {
			out = events
		} else {
			for t, e := range events {
				out[t] = e
			}
		}
	}
	return out
}

// loadTheses returns each ticker's recorded holding thesis (see /thesis,
// handleThesis), keyed by ticker — only tickers with one on record appear in
// the map. A per-ticker query failure logs and skips that ticker rather than
// aborting the whole call; unlike fundamentals/earnings this hits local
// SQLite, not a rate-limited external API, so a plain loop (not a batched
// query) is fine at the handful-of-positions scale /insight runs at.
func (b *Bot) loadTheses(tickers []string) map[string]string {
	out := make(map[string]string, len(tickers))
	for _, t := range tickers {
		thesis, ok, err := service.CurrentThesis(b.db, t)
		if err != nil {
			logger.Errorf("load thesis %s: %v", t, err)
			continue
		}
		if ok {
			out[t] = thesis
		}
	}
	return out
}

// computeVsSPY is the pure percentage math behind the Phase 3.6 expansion's
// "逐檔 vs SPY" item: a position's own holding-period return next to SPY's
// over the same period. Split out from loadVsSPY (which owns the DB/quote
// lookups) so the arithmetic is independently testable, same shape as
// service.BreachAlertDecision.
func computeVsSPY(currentPrice, avgCost, spyPrice, spyEntryClose float64, bench string) llm.VsSPYReturn {
	return llm.VsSPYReturn{
		TickerPct: (currentPrice - avgCost) / avgCost * 100,
		SPYPct:    (spyPrice - spyEntryClose) / spyEntryClose * 100,
		Bench:     bench,
	}
}

// loadVsSPY computes computeVsSPY for every position in stocks that has both
// a BUY date on record (db.GetEarliestBuyDate) and a same-date benchmark
// close in daily_snapshots (populated by snapshotBenchmark since Phase 3.8)
// — a position missing either is simply omitted from the result, not an
// error (e.g. a pre-Phase-3.8 buy predates the benchmark ever being
// snapshotted). Reuses stocks' already-fetched Quote.Price rather than a
// second GetQuote call per ticker. Phase 6 PR2: stocks can now span both
// markets (RunWeeklyReview/handleInsight pass every held position), so the
// benchmark is selected per position via market.Of (SPY for US, 0050 for
// TW) rather than a single shared SPY quote — benchQuotes lazily fetches and
// caches each market's current quote, same "only pay for what's actually
// used" shape as fetchStockData's loadBenchCloses.
func (b *Bot) loadVsSPY(stocks []llm.StockData, positions map[string]db.Position) map[string]llm.VsSPYReturn {
	benchQuotes := make(map[market.MarketID]*data.Quote)
	loadBenchQuote := func(m market.MarketID) *data.Quote {
		if q, ok := benchQuotes[m]; ok {
			return q
		}
		ticker := service.BenchmarkFor(m)
		q, err := b.provider.GetQuote(ticker)
		if err != nil {
			logger.Errorf("vs-spy: benchmark %s quote: %v", ticker, err)
			q = nil
		}
		benchQuotes[m] = q
		return q
	}

	out := make(map[string]llm.VsSPYReturn, len(stocks))
	for _, s := range stocks {
		ticker := s.Quote.Ticker
		p, ok := positions[ticker]
		if !ok || p.AvgCost == 0 {
			continue
		}
		m := market.Of(ticker)
		benchQuote := loadBenchQuote(m)
		if benchQuote == nil {
			continue
		}
		buyDate, ok, err := b.db.GetEarliestBuyDate(ticker)
		if err != nil {
			logger.Errorf("vs-spy: earliest buy %s: %v", ticker, err)
			continue
		}
		if !ok {
			continue
		}
		benchEntryClose, ok, err := b.db.GetSnapshotClose(service.BenchmarkFor(m), buyDate)
		if err != nil {
			logger.Errorf("vs-spy: benchmark snapshot %s: %v", ticker, err)
			continue
		}
		if !ok || benchEntryClose == 0 {
			continue
		}
		out[ticker] = computeVsSPY(s.Quote.Price, p.AvgCost, benchQuote.Price, benchEntryClose, service.BenchmarkFor(m))
	}
	return out
}

// loadMarketNews returns up to marketNewsLimit general market/macro news
// items for market m's recommendation prompt news-summary section — Finnhub
// (b.marketNews) for US, cnyes (b.twMarketNews) for TW (2026-07-28 TW
// data-gap PR; TW previously nil-degraded here unconditionally). Degrades to
// nil (not an error) when the relevant provider isn't configured/nil or the
// request fails — same optional-data pattern as fundamentals/earnings; a nil
// result means GenerateRecommendations simply omits the summary.
func (b *Bot) loadMarketNews(m market.MarketID) []data.NewsItem {
	provider := b.marketNews
	if m == market.TW {
		provider = b.twMarketNews
	}
	if provider == nil {
		return nil
	}
	items, err := provider.GetMarketNews(marketNewsLimit)
	if err != nil {
		logger.Errorf("market news: %v", err)
		return nil
	}
	return items
}

// loadScanHits returns today's Phase 2.6 universe-scan hits keyed by ticker
// (joined reason string per ticker) via db.GetScanHits, filtered to market m
// — scan_hits carries no market column of its own (see internal/db's Phase 6
// migration note: every reader that has the ticker in hand can derive it),
// so this filters client-side via market.Of, same pattern
// recordNetWorthSnapshot uses for db.GetPositions(). Degrades to nil (not an
// error) on a query failure — candidates without a scan reason still go
// through movers as before.
func (b *Bot) loadScanHits(m market.MarketID) map[string]string {
	hits, err := b.db.GetScanHits(todayDate())
	if err != nil {
		logger.Errorf("scan hits: %v", err)
		return nil
	}
	out := make(map[string]string, len(hits))
	for t, reason := range hits {
		if market.Of(t) == m {
			out[t] = reason
		}
	}
	return out
}

// renderEarningsPreview formats a consolidated "upcoming earnings" list from
// a ticker->EarningsEvent map (as returned by loadEarnings), limited to
// events within days days out and sorted soonest-first — RunWeeklyReview's
// Sunday-planning view (PLAN.md's Phase 3.6 PR2), distinct from
// checkEarningsAlerts' narrower 3-day proactive alert and from each
// position's own per-ticker earnings line already embedded by
// writeStockSection via fetchStockData. Returns "" when nothing falls in
// the window, so RunWeeklyReview can skip the block entirely.
func renderEarningsPreview(lang i18n.Lang, earnings map[string]data.EarningsEvent, days int) string {
	type entry struct {
		ticker    string
		date      string
		daysUntil int
		estimated bool
	}
	var entries []entry
	for t, e := range earnings {
		d := daysUntil(e.Date)
		if d < 0 || d > days {
			continue
		}
		entries = append(entries, entry{ticker: t, date: e.Date, daysUntil: d, estimated: e.Estimated})
	}
	if len(entries) == 0 {
		return ""
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].daysUntil < entries[j].daysUntil })

	var sb strings.Builder
	sb.WriteString(i18n.T(lang, i18n.KeyWeeklyEarningsPreviewTitle))
	for _, e := range entries {
		key := i18n.KeyWeeklyEarningsPreviewLine
		if e.estimated {
			key = i18n.KeyWeeklyEarningsPreviewLineEstimated
		}
		sb.WriteString(i18n.T(lang, key, e.ticker, e.date, e.daysUntil))
	}
	return sb.String()
}
