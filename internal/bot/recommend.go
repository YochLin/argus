package bot

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"argus/internal/data"
	"argus/internal/i18n"
	"argus/internal/llm"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/service"
)

// These aliases and wrapper keep the Telegram adapter's formatting helpers
// compact while the tracking rule and aggregation live in internal/service.
type trackRow = service.RecommendationTrackRow

type trackSourceStats = service.RecommendationTrackStats

// parseRecommendMarketArg parses /recommend's optional [tw|us] argument
// (Phase 6 PR2 §5.3), case-insensitive: "" means "both markets, US first"
// (matching this project's existing US-first convention elsewhere — e.g.
// sendPortfolioSection), "us"/"tw" means just that one. ok is false for
// anything else, telling the caller to show the usage message rather than
// guessing. Pure and separately tested, same convention as
// parseTradeArgs/parseStopArgs/parseCashArgs.
func parseRecommendMarketArg(args string) (markets []market.MarketID, ok bool) {
	switch strings.ToLower(strings.TrimSpace(args)) {
	case "":
		return []market.MarketID{market.US, market.TW}, true
	case "us":
		return []market.MarketID{market.US}, true
	case "tw":
		return []market.MarketID{market.TW}, true
	default:
		return nil, false
	}
}

// handleRecommend is /recommend [tw|us]'s command handler: each market in
// parseRecommendMarketArg's result runs its own placeholder-then-report flow
// in sequence.
func (b *Bot) handleRecommend(ctx context.Context, args string) {
	markets, ok := parseRecommendMarketArg(args)
	if !ok {
		b.Send(i18n.T(b.lang, i18n.KeyRecommendUsage))
		return
	}
	for _, m := range markets {
		b.RunRecommend(ctx, m)
	}
}

// RunRecommend is handleRecommend's per-market body — the interactive
// counterpart of runDailyReport's LLM-call tail, without the signal
// detection/exit-discipline/exploration steps that are runDailyReport-only
// (see that function's own doc comment for why).
//
// Exported for POST /api/v1/recommendations/trigger (Phase 24 Stage 4 Step
// 4.2), which reaches it through web.Recommender rather than importing this
// package. The panic guard is here rather than at either call site for the
// same reason: both run it on a goroutine of their own (dispatch's `go
// b.handleMessage`, the trigger handler's), where an unrecovered panic takes
// the whole process down instead of one request.
func (b *Bot) RunRecommend(ctx context.Context, m market.MarketID) {
	defer b.recoverJobPanic("recommend")
	b.Send(i18n.T(b.lang, i18n.KeyAnalyzing))

	in, err := b.gatherRecommendationInputs(m)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyWatchlistQueryFailed, err))
		return
	}

	summary, recs, raw, model, latencyMs, err := b.llm.GenerateRecommendations(ctx, in.watchlist, in.candidates, in.marketNews, in.marketContext, in.recentLessons, m == market.TW)
	b.recordLLMRun("recommend", m, in, raw, model, latencyMs)
	if err != nil {
		if errors.Is(err, llm.ErrRecommendationParseFailed) {
			b.Send(i18n.T(b.lang, i18n.KeyRecParseFailed, err))
		} else {
			b.Send(i18n.T(b.lang, i18n.KeyLLMFailed, err))
		}
		return
	}

	if len(recs) == 0 {
		b.Send(i18n.T(b.lang, i18n.KeyNoRecommendations))
		return
	}

	// explore is nil: two-stage LLM exploration only runs from RunDailyReport
	// (see exploreCandidates and docs/phase-2.6-two-stage-llm-exploration.md)
	// — an interactive /recommend doesn't get a second one-shot LLM call
	// tacked onto its latency.
	sources := service.RecommendationSources(in.watchlistTickers, in.candidateTickers, in.scanHits, nil)
	b.sendAndSaveRecommendations(summary, recs, sources, m, len(in.candidateTickers), in.watchlist, in.candidates)
}

func (b *Bot) handleCheck(ctx context.Context, ticker string) {
	ticker = normalizeTicker(ticker)
	if ticker == "" {
		b.Send(i18n.T(b.lang, i18n.KeyCheckUsage))
		return
	}

	// Send the placeholder on the bare ticker before any lookup — tickerLabel
	// makes a live FinMind network call for a TW ticker (US tickers skip it,
	// see companyName), so computing the label first could delay this
	// "still working" reply the same way the slow LLM call itself would.
	b.Send(i18n.T(b.lang, i18n.KeyAnalyzingTicker, ticker))
	label := b.tickerLabel(ticker)

	stock, err := service.CheckStockData(b.provider, b.companyNames, b.fundamentals, b.analystRating, b.db, ticker)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyQuoteFailed, label, err))
		return
	}
	// RS63 needs the right benchmark for ticker's own market (Phase 6): SPY
	// for a US ticker, 0050 for a TW one — /check works on either since
	// handleCheck, unlike gatherRecommendationInputs, isn't restricted to a
	// US-only watchlist (see docs/phase-6-tw-market.md §4.3).
	var benchCloses []float64
	if benchCandles, err := b.history.GetHistory(service.BenchmarkFor(market.Of(ticker)), "1y"); err == nil {
		benchCloses = data.Closes(benchCandles)
	}
	stock.Technicals, stock.Candles, stock.StrategyHits = b.computeTechnicals(ticker, benchCloses)

	result, err := b.llm.CheckStock(ctx, stock)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyLLMFailed, err))
		return
	}

	b.Send(i18n.T(b.lang, i18n.KeyCheckResultTitle, label, result))
}

// handleTrack reviews recommendations from the past N days (default 7)
// against today's prices, so recommendation quality is verifiable instead of
// write-only. Hit criteria (Phase 3.8): when a same-period market benchmark
// close is on record (see snapshotBenchmark), BUY only counts as a hit if the
// ticker beat its market benchmark's move over the same window and SELL only
// if it underperformed — "up in a broad rally" no longer counts as a good BUY
// call on its own (see service.TrackHit). Recommendations predating the
// benchmark snapshot job (or any date the benchmark has no snapshot for) fall back to the
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

	overall, bySource, byMarket := summarizeTrack(rows)
	sb.WriteString(renderTrackSummary(b.lang, overall, bySource, byMarket))
	b.Send(sb.String())
}

func summarizeTrack(rows []trackRow) (overall trackSourceStats, bySource map[string]trackSourceStats, byMarket map[market.MarketID]trackSourceStats) {
	summary := service.SummarizeTrack(rows)
	return summary.Overall, summary.BySource, summary.ByMarket
}

// computeTrackRows re-runs /track's core computation for the given lookback
// window: for each recommendation since then, look up the price at
// recommendation time and now, score BUY/SELL hits against the same-period
// market benchmark (TrackHit), and render each into a display line. Shared by
// handleTrack (its own full display) and RunWeeklyReview's strategy-feedback
// block (which only needs rows, to summarize via summarizeTrack — lines
// go unused there, same "compute once, let callers use what they need"
// shape as fetchStockData). ok=false means there were no recommendations in
// the window at all (not an error) — callers render that as "nothing to
// report" rather than an empty summary.
func (b *Bot) computeTrackRows(days int) (rows []trackRow, lines []string, ok bool, err error) {
	tracker := b.recommendations()
	if tracker == nil {
		return nil, nil, false, fmt.Errorf("recommendation service unavailable")
	}
	report, err := tracker.Track(days)
	if err != nil {
		return nil, nil, false, err
	}
	if !report.HasRecommendations {
		return nil, nil, false, nil
	}

	loggedBenchmarkErrors := make(map[string]bool)
	for _, detail := range report.Details {
		if !detail.HasBasePrice {
			lines = append(lines, i18n.T(b.lang, i18n.KeyTrackLineNoPrice, detail.Date, detail.Ticker, detail.Action))
			continue
		}
		if !detail.HasQuote {
			if detail.QuoteErr != nil {
				logger.Errorf("track: quote %s: %v", detail.Ticker, detail.QuoteErr)
			}
			lines = append(lines, i18n.T(b.lang, i18n.KeyQuoteUnavailable, detail.Ticker))
			continue
		}
		if detail.BenchmarkErr != nil && !loggedBenchmarkErrors[detail.BenchmarkTicker] {
			logger.Errorf("track: benchmark %s: %v", detail.BenchmarkTicker, detail.BenchmarkErr)
			loggedBenchmarkErrors[detail.BenchmarkTicker] = true
		}

		verdict := ""
		if detail.Action == "BUY" || detail.Action == "SELL" {
			verdict = "❌"
			if detail.Hit {
				verdict = "✅"
			}
		}
		if detail.HasBenchmark {
			lines = append(lines, i18n.T(b.lang, i18n.KeyTrackLineVsSPY, detail.Date, detail.Ticker, detail.Action, b.money(detail.Ticker, detail.BasePrice), b.money(detail.Ticker, detail.CurrentPrice), detail.ChangePct, detail.BenchmarkTicker, detail.BenchmarkChangePct, verdict))
		} else {
			lines = append(lines, i18n.T(b.lang, i18n.KeyTrackLine, detail.Date, detail.Ticker, detail.Action, b.money(detail.Ticker, detail.BasePrice), b.money(detail.Ticker, detail.CurrentPrice), detail.ChangePct, verdict))
		}
	}

	return report.Rows, lines, true, nil
}
