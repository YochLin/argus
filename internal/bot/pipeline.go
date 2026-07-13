package bot

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/llm"
	"argus/internal/signals"
)

// recommendationInputs bundles everything handleRecommend and RunDailyReport
// both need to build their llm.GenerateRecommendations call — assembled once
// by gatherRecommendationInputs so a new prompt input gets wired in exactly
// one place instead of both call sites having to stay in lockstep by hand
// (see docs/refactor-internal-bot.md). Each caller's own middle (daily
// report's earnings/signal/stop-loss checks) reads straight off these
// fields rather than this struct performing them, since only RunDailyReport
// wants them.
type recommendationInputs struct {
	watchlistTickers []string
	candidateTickers []string // deduped, watchlist tickers excluded
	scanHits         map[string]string
	positions        map[string]db.Position
	earnings         map[string]data.EarningsEvent
	marketNews       []data.NewsItem
	prevRecs         map[string]db.Recommendation
	watchlist        []llm.StockData // fetchStockData output for watchlistTickers
	candidates       []llm.StockData // fetchStockData output for candidateTickers
}

// gatherRecommendationInputs assembles the watchlist ∪ market-mover/scan-hit
// candidate set, the positions/earnings/market-news/prior-recommendation
// context that feeds the LLM prompt, and the resulting []llm.StockData for
// both ticker sets. Returns the db.GetWatchlist error verbatim (both callers
// render it via the same KeyWatchlistQueryFailed message and abort).
func (b *Bot) gatherRecommendationInputs() (recommendationInputs, error) {
	tickers, err := b.db.GetWatchlist()
	if err != nil {
		return recommendationInputs{}, err
	}

	candidateTickers, err := b.provider.GetMarketMovers()
	if err != nil {
		log.Printf("market movers: %v", err)
	}
	scanHits := b.loadScanHits()
	dedupedCandidates := mergeCandidates(candidateTickers, scanHits, tickers)
	allTickers := append(append([]string{}, tickers...), dedupedCandidates...)

	positions := b.loadPositions()
	earnings := b.loadEarnings(allTickers)
	marketNews := b.loadMarketNews()
	prevRecs := b.loadPrevRecs(allTickers)

	watchlist := b.fetchStockData(tickers, true, positions, earnings, nil, prevRecs)
	candidates := b.fetchStockData(dedupedCandidates, false, positions, earnings, scanHits, prevRecs)

	return recommendationInputs{
		watchlistTickers: tickers,
		candidateTickers: dedupedCandidates,
		scanHits:         scanHits,
		positions:        positions,
		earnings:         earnings,
		marketNews:       marketNews,
		prevRecs:         prevRecs,
		watchlist:        watchlist,
		candidates:       candidates,
	}, nil
}

// sendAndSaveRecommendations formats LLM recommendations for Telegram and
// persists them dated today, each with its ticker's current price looked up
// from the already-fetched stock data — /track later compares that stored
// price against the price on review day. sources (ticker -> "watchlist"/
// "movers"/"scan", see recommendationSources) is persisted alongside so
// /track can break its hit rate down by candidate-sourcing path (Phase 3.8).
// Shared by /recommend and RunDailyReport, which otherwise mirror each other.
func (b *Bot) sendAndSaveRecommendations(newsSummary string, recs []llm.Recommendation, sources map[string]string, stockLists ...[]llm.StockData) {
	if newsSummary != "" {
		b.Send(i18n.T(b.lang, i18n.KeyMarketNewsSummaryTitle) + newsSummary)
	}

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
			Source: sources[r.Ticker],
		})
	}
	if err := b.db.SaveRecommendations(todayDate(), dbRecs); err != nil {
		log.Printf("save recommendations: %v", err)
	}
}

// fetchStockData fetches quote+news for each ticker. Fundamentals and
// AnalystRating (Phase 3.7) are only attached when includeFundamentals is set
// (watchlist tickers, not the broad market-mover candidate list) to stay well
// under Finnhub's free-tier 60-requests/minute limit when a candidate list
// has a dozen-plus tickers — /stock/recommendation is a per-ticker call just
// like /stock/metric, so it shares the same gate rather than getting its own.
// Technicals (RSI/MACD/moving averages, via computeTechnicals) has no such
// gate — Yahoo's history endpoint carries no rate-limit concern, and
// candidates are exactly where the model most needs trend context before
// calling a fresh BUY. positions (ticker -> open position) is looked up via
// loadPositions and attaches cost-basis context for any ticker the user
// actually holds; earnings (ticker -> upcoming earnings) is looked up via
// loadEarnings and attaches an earnings-date warning for any ticker
// reporting soon. scanReasons (ticker -> joined signal message) is looked up
// via db.GetScanHits and attaches why a Phase 2.6 universe-scan candidate
// was surfaced. prevRecs (ticker -> last recommendation on record) is looked
// up via loadPrevRecs and attaches Phase 3.8's recommendation-continuity
// line; a row with an empty Action (pre-Phase-1 data, or a call the model
// omitted) is skipped rather than rendering a blank line. Pass nil for any
// of the four if there's nothing to attach.
func (b *Bot) fetchStockData(tickers []string, includeFundamentals bool, positions map[string]db.Position, earnings map[string]data.EarningsEvent, scanReasons map[string]string, prevRecs map[string]db.Recommendation) []llm.StockData {
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
		if includeFundamentals && b.analystRating != nil {
			if ar, err := b.analystRating.GetAnalystRating(t); err != nil {
				log.Printf("analyst rating %s: %v", t, err)
			} else {
				stock.AnalystRating = ar
			}
		}
		stock.Technicals = b.computeTechnicals(t)
		if p, ok := positions[t]; ok {
			stock.Position = &llm.Position{Shares: p.Shares, AvgCost: p.AvgCost}
		}
		if e, ok := earnings[t]; ok {
			stock.Earnings = &llm.Earnings{Date: e.Date, DaysUntil: daysUntil(e.Date)}
		}
		if r, ok := scanReasons[t]; ok {
			stock.ScanReason = &r
		}
		if pr, ok := prevRecs[t]; ok && pr.Action != "" {
			stock.PrevRec = &llm.PrevRecommendation{Action: pr.Action, Date: pr.Date, Price: pr.Price, DaysAgo: -daysUntil(pr.Date)}
		}
		result = append(result, stock)
	}
	return result
}

// computeTechnicals fetches ticker's closing-price history and reduces it to
// the RSI/MACD/moving-average values an LLM prompt needs (see
// llm.Technicals). Returns nil (not an error) on a history-fetch failure, so
// callers degrade the same way the fundamentals fetch above does. This
// duplicates the GetHistory call RunDailyReport's signal-check loop already
// makes for watchlist tickers (see checkStatefulSignals) — the two serve
// different purposes (stateful alert dedup vs. raw values for the prompt)
// and don't share a data structure, and Yahoo's history endpoint has no
// rate-limit concern like Finnhub's, so the duplicate call is an accepted
// trade-off rather than an oversight.
func (b *Bot) computeTechnicals(ticker string) *llm.Technicals {
	closes, err := b.history.GetHistory(ticker)
	if err != nil {
		log.Printf("history %s: %v", ticker, err)
		return nil
	}
	return &llm.Technicals{
		RSI14:     signals.RSI(closes, 14),
		MACDTrend: signals.MACDTrend(closes),
		MA20:      signals.MA(closes, 20),
		MA50:      signals.MA(closes, 50),
		MA200:     signals.MA(closes, 200),
	}
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
		log.Printf("load prev recommendations: %v", err)
		return nil
	}
	return recs
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

// loadTheses returns each ticker's recorded holding thesis (see /thesis,
// handleThesis), keyed by ticker — only tickers with one on record appear in
// the map. A per-ticker query failure logs and skips that ticker rather than
// aborting the whole call; unlike fundamentals/earnings this hits local
// SQLite, not a rate-limited external API, so a plain loop (not a batched
// query) is fine at the handful-of-positions scale /insight runs at.
func (b *Bot) loadTheses(tickers []string) map[string]string {
	out := make(map[string]string, len(tickers))
	for _, t := range tickers {
		thesis, ok, err := b.db.GetThesis(t)
		if err != nil {
			log.Printf("load thesis %s: %v", t, err)
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
// breachAlertDecision.
func computeVsSPY(currentPrice, avgCost, spyPrice, spyEntryClose float64) llm.VsSPYReturn {
	return llm.VsSPYReturn{
		TickerPct: (currentPrice - avgCost) / avgCost * 100,
		SPYPct:    (spyPrice - spyEntryClose) / spyEntryClose * 100,
	}
}

// loadVsSPY computes computeVsSPY for every position in stocks that has both
// a BUY date on record (db.GetEarliestBuyDate) and a same-date SPY close in
// daily_snapshots (populated by snapshotBenchmark since Phase 3.8) — a
// position missing either is simply omitted from the result, not an error
// (e.g. a pre-Phase-3.8 buy predates SPY ever being snapshotted). Reuses
// stocks' already-fetched Quote.Price rather than a second GetQuote call per
// ticker, and fetches the current SPY quote once up front since every
// position compares against the same value.
func (b *Bot) loadVsSPY(stocks []llm.StockData, positions map[string]db.Position) map[string]llm.VsSPYReturn {
	spyQuote, err := b.provider.GetQuote(benchmarkTicker)
	if err != nil {
		log.Printf("vs-spy: benchmark %s quote: %v", benchmarkTicker, err)
		return nil
	}

	out := make(map[string]llm.VsSPYReturn, len(stocks))
	for _, s := range stocks {
		ticker := s.Quote.Ticker
		p, ok := positions[ticker]
		if !ok || p.AvgCost == 0 {
			continue
		}
		buyDate, ok, err := b.db.GetEarliestBuyDate(ticker)
		if err != nil {
			log.Printf("vs-spy: earliest buy %s: %v", ticker, err)
			continue
		}
		if !ok {
			continue
		}
		spyEntryClose, ok, err := b.db.GetSnapshotClose(benchmarkTicker, buyDate)
		if err != nil {
			log.Printf("vs-spy: benchmark snapshot %s: %v", ticker, err)
			continue
		}
		if !ok || spyEntryClose == 0 {
			continue
		}
		out[ticker] = computeVsSPY(s.Quote.Price, p.AvgCost, spyQuote.Price, spyEntryClose)
	}
	return out
}

// loadMarketNews returns up to marketNewsLimit general market/macro news
// items for the recommendation prompt's market-news summary section.
// Degrades to nil (not an error) when Finnhub isn't configured or the
// request fails — same optional-data pattern as fundamentals/earnings; a nil
// result means GenerateRecommendations simply omits the summary.
func (b *Bot) loadMarketNews() []data.NewsItem {
	if b.marketNews == nil {
		return nil
	}
	items, err := b.marketNews.GetMarketNews(marketNewsLimit)
	if err != nil {
		log.Printf("market news: %v", err)
		return nil
	}
	return items
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

// computeTrackRows re-runs /track's core computation for the given lookback
// window: for each recommendation since then, look up the price at
// recommendation time and now, score BUY/SELL hits against the same-period
// SPY move (trackHit), and render each into a display line. Shared by
// handleTrack (its own full display) and RunWeeklyReview's strategy-feedback
// block (which only needs rows, to summarize via summarizeTrack — lines
// go unused there, same "compute once, let callers use what they need"
// shape as fetchStockData). ok=false means there were no recommendations in
// the window at all (not an error) — callers render that as "nothing to
// report" rather than an empty summary.
func (b *Bot) computeTrackRows(days int) (rows []trackRow, lines []string, ok bool, err error) {
	fromDate := time.Now().In(cst).AddDate(0, 0, -days).Format("2006-01-02")
	recs, err := b.db.GetRecommendationsSince(fromDate)
	if err != nil {
		return nil, nil, false, err
	}
	if len(recs) == 0 {
		return nil, nil, false, nil
	}

	// One quote per distinct ticker, however often it was recommended.
	quotes := make(map[string]*data.Quote)
	spyQuote, err := b.provider.GetQuote(benchmarkTicker)
	if err != nil {
		log.Printf("track: benchmark %s quote: %v", benchmarkTicker, err)
		spyQuote = nil
	}

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
			lines = append(lines, i18n.T(b.lang, i18n.KeyTrackLineNoPrice, r.Date, r.Ticker, action))
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
			lines = append(lines, i18n.T(b.lang, i18n.KeyQuoteUnavailable, r.Ticker))
			continue
		}

		changePct := (q.Price - base) / base * 100

		var spyChangePct float64
		haveSPY := false
		if spyQuote != nil {
			if spyBase, ok, err := b.db.GetSnapshotClose(benchmarkTicker, r.Date); err == nil && ok && spyBase != 0 {
				spyChangePct = (spyQuote.Price - spyBase) / spyBase * 100
				haveSPY = true
			}
		}

		verdict := ""
		if r.Action == "BUY" || r.Action == "SELL" {
			hit := trackHit(r.Action, changePct, spyChangePct, haveSPY)
			verdict = "❌"
			if hit {
				verdict = "✅"
			}
			rows = append(rows, trackRow{
				Action:    r.Action,
				Source:    displaySource(r.Source),
				ChangePct: changePct,
				Hit:       hit,
			})
		}

		if haveSPY {
			lines = append(lines, i18n.T(b.lang, i18n.KeyTrackLineVsSPY, r.Date, r.Ticker, action, base, q.Price, changePct, spyChangePct, verdict))
		} else {
			lines = append(lines, i18n.T(b.lang, i18n.KeyTrackLine, r.Date, r.Ticker, action, base, q.Price, changePct, verdict))
		}
	}

	return rows, lines, true, nil
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
	}
	var entries []entry
	for t, e := range earnings {
		d := daysUntil(e.Date)
		if d < 0 || d > days {
			continue
		}
		entries = append(entries, entry{ticker: t, date: e.Date, daysUntil: d})
	}
	if len(entries) == 0 {
		return ""
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].daysUntil < entries[j].daysUntil })

	var sb strings.Builder
	sb.WriteString(i18n.T(lang, i18n.KeyWeeklyEarningsPreviewTitle))
	for _, e := range entries {
		sb.WriteString(i18n.T(lang, i18n.KeyWeeklyEarningsPreviewLine, e.ticker, e.date, e.daysUntil))
	}
	return sb.String()
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

// recommendationSources maps every ticker eligible for today's LLM call to
// where it came from ("watchlist"/"scan"/"movers"), for Phase 3.8's /track
// breakdown by candidate-sourcing path. candidates is the already-deduped
// list returned by mergeCandidates; a ticker present in both scanHits and
// that list is attributed to "scan" rather than "movers" — that's the more
// specific signal that actually surfaced it with a stated reason (see
// llm.StockData.ScanReason), even if it also happened to be trending.
func recommendationSources(watchlist, candidates []string, scanHits map[string]string) map[string]string {
	out := make(map[string]string, len(watchlist)+len(candidates))
	for _, t := range watchlist {
		out[t] = "watchlist"
	}
	for _, t := range candidates {
		// mergeCandidates already excludes watchlist tickers from candidates
		// in normal use, so this shouldn't fire in practice — kept as a
		// defensive guard so "watchlist" always wins over "movers"/"scan"
		// for a ticker present in both, rather than depending on which loop
		// ran last.
		if out[t] == "watchlist" {
			continue
		}
		if _, ok := scanHits[t]; ok {
			out[t] = "scan"
		} else {
			out[t] = "movers"
		}
	}
	return out
}
