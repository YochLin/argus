package bot

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/llm"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/render"
	"argus/internal/service"
	"argus/internal/webfetch"
)

func (b *Bot) handleAdd(ticker string) {
	watchlist := b.watchlists()
	if watchlist == nil {
		b.Send(i18n.T(b.lang, i18n.KeyAddFailed, errors.New("watchlist service unavailable")))
		return
	}
	normalized, err := watchlist.Add(ticker)
	if errors.Is(err, service.ErrInvalidTicker) {
		b.Send(i18n.T(b.lang, i18n.KeyAddUsage))
		return
	}
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyAddFailed, err))
		return
	}
	b.Send(i18n.T(b.lang, i18n.KeyAddSuccess, b.tickerLabel(normalized)))
}

func (b *Bot) handleRemove(ticker string) {
	watchlist := b.watchlists()
	if watchlist == nil {
		b.Send(i18n.T(b.lang, i18n.KeyRemoveFailed, errors.New("watchlist service unavailable")))
		return
	}
	normalized, err := watchlist.Remove(ticker)
	if errors.Is(err, service.ErrInvalidTicker) {
		b.Send(i18n.T(b.lang, i18n.KeyRemoveUsage))
		return
	}
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyRemoveFailed, err))
		return
	}
	b.Send(i18n.T(b.lang, i18n.KeyRemoveSuccess, b.tickerLabel(normalized)))
}

func (b *Bot) handleList() {
	watchlist := b.watchlists()
	if watchlist == nil {
		b.Send(i18n.T(b.lang, i18n.KeyQueryFailed, errors.New("watchlist service unavailable")))
		return
	}
	tickers, err := watchlist.GetWatchlist()
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyQueryFailed, err))
		return
	}
	if len(tickers) == 0 {
		b.Send(i18n.T(b.lang, i18n.KeyWatchlistEmptyHint))
		return
	}
	labels := make([]string, len(tickers))
	for i, t := range tickers {
		labels[i] = b.tickerLabel(t)
	}
	b.Send(i18n.T(b.lang, i18n.KeyWatchlistTitle) + strings.Join(labels, "\n"))
}

func (b *Bot) handleStatus(ticker string) {
	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	if ticker == "" {
		watchlist := b.watchlists()
		if watchlist == nil {
			b.Send(i18n.T(b.lang, i18n.KeyQueryFailed, errors.New("watchlist service unavailable")))
			return
		}
		tickers, err := watchlist.GetWatchlist()
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
				sb.WriteString(i18n.T(b.lang, i18n.KeyQuoteUnavailable, b.tickerLabel(t)))
				continue
			}
			sb.WriteString(formatQuote(b.lang, q, b.tickerLabel(t)))
			sb.WriteString("\n\n")
		}
		b.Send(sb.String())
		return
	}

	q, err := b.provider.GetQuote(ticker)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyQuoteFailed, b.tickerLabel(ticker), err))
		return
	}
	b.Send(formatQuote(b.lang, q, b.tickerLabel(ticker)))
}

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
		b.runRecommend(ctx, m)
	}
}

// runRecommend is handleRecommend's per-market body — the interactive
// counterpart of runDailyReport's LLM-call tail, without the signal
// detection/exit-discipline/exploration steps that are runDailyReport-only
// (see that function's own doc comment for why).
func (b *Bot) runRecommend(ctx context.Context, m market.MarketID) {
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
	ticker = strings.ToUpper(strings.TrimSpace(ticker))
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

	q, err := b.provider.GetQuote(ticker)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyQuoteFailed, label, err))
		return
	}
	fetched, _ := b.provider.GetNews(ticker, tickerNewsFetch)
	news := (&newsPicker{}).pick(fetched, tickerNewsSlots)

	stock := llm.StockData{Quote: q, News: news, CompanyName: b.companyName(ticker)}
	if b.fundamentals != nil {
		if fd, err := b.fundamentals.GetFundamentals(ticker); err != nil {
			logger.Errorf("fundamentals %s: %v", ticker, err)
		} else {
			stock.Fundamentals = fd
		}
		if st, err := b.fundamentals.GetFinancialStatements(ticker, "annual"); err != nil {
			logger.Errorf("financial statements %s: %v", ticker, err)
		} else {
			stock.Statement = st
		}
	}
	if b.analystRating != nil {
		if ar, err := b.analystRating.GetAnalystRating(ticker); err != nil {
			logger.Errorf("analyst rating %s: %v", ticker, err)
		} else {
			stock.AnalystRating = ar
		}
	}
	// RS63 needs the right benchmark for ticker's own market (Phase 6): SPY
	// for a US ticker, 0050 for a TW one — /check works on either since
	// handleCheck, unlike gatherRecommendationInputs, isn't restricted to a
	// US-only watchlist (see docs/phase-6-tw-market.md §4.3).
	var benchCloses []float64
	if benchCandles, err := b.history.GetHistory(benchmarkFor(market.Of(ticker)), "1y"); err == nil {
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

// These aliases and wrappers keep the Telegram adapter's formatting helpers
// compact while the tracking rule and aggregation live in internal/service.
type trackRow = service.RecommendationTrackRow
type trackSourceStats = service.RecommendationTrackStats

func trackHit(action string, tickerChangePct, spyChangePct float64, haveSPY bool) bool {
	return service.TrackHit(action, tickerChangePct, spyChangePct, haveSPY)
}

func summarizeTrack(rows []trackRow) (overall trackSourceStats, bySource map[string]trackSourceStats, byMarket map[market.MarketID]trackSourceStats) {
	summary := service.SummarizeTrack(rows)
	return summary.Overall, summary.BySource, summary.ByMarket
}

func displaySource(source string) string {
	return service.DisplaySource(source)
}

func sortedSourceKeys(bySource map[string]trackSourceStats) []string {
	return service.SortedSourceKeys(bySource)
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
// feeSet distinguishes "no fee argument typed" (auto-calculate via
// paper.FeeFor) from "explicitly typed 0" (Phase 13 §3.2) — a plain float64
// can't tell those apart since both are 0.
func parseTradeArgs(args string) (ticker string, shares, price, fee float64, feeSet bool, date string, err error) {
	fields := strings.Fields(args)
	if len(fields) < 3 || len(fields) > 5 {
		return "", 0, 0, 0, false, "", fmt.Errorf("expected <ticker> <shares> <price> [fee] [date]")
	}
	ticker = strings.ToUpper(fields[0])
	if shares, err = strconv.ParseFloat(fields[1], 64); err != nil || shares <= 0 {
		return "", 0, 0, 0, false, "", fmt.Errorf("invalid shares %q", fields[1])
	}
	if price, err = strconv.ParseFloat(fields[2], 64); err != nil || price <= 0 {
		return "", 0, 0, 0, false, "", fmt.Errorf("invalid price %q", fields[2])
	}

	for _, f := range fields[3:] {
		if tradeDateRe.MatchString(f) {
			if date != "" {
				return "", 0, 0, 0, false, "", fmt.Errorf("duplicate date %q", f)
			}
			if _, perr := time.Parse("2006-01-02", f); perr != nil {
				return "", 0, 0, 0, false, "", fmt.Errorf("invalid date %q", f)
			}
			date = f
			continue
		}
		if feeSet {
			return "", 0, 0, 0, false, "", fmt.Errorf("unexpected argument %q", f)
		}
		if fee, err = strconv.ParseFloat(f, 64); err != nil || fee < 0 {
			return "", 0, 0, 0, false, "", fmt.Errorf("invalid fee %q", f)
		}
		feeSet = true
	}
	return ticker, shares, price, fee, feeSet, date, nil
}

// parseStopArgs parses /stop's "<ticker> [price]" arguments — price is
// optional (omitted means "show me the current setting and candidates"),
// mirroring parseTradeArgs' shape for a single optional numeric field.
func parseStopArgs(args string) (ticker string, price float64, hasPrice bool, err error) {
	fields := strings.Fields(args)
	if len(fields) < 1 || len(fields) > 2 {
		return "", 0, false, fmt.Errorf("expected <ticker> [price]")
	}
	ticker = strings.ToUpper(fields[0])
	if len(fields) == 1 {
		return ticker, 0, false, nil
	}
	if price, err = strconv.ParseFloat(fields[1], 64); err != nil || price <= 0 {
		return "", 0, false, fmt.Errorf("invalid price %q", fields[1])
	}
	return ticker, price, true, nil
}

// handleStop is Phase 3.11 PR1's /stop TICKER [PRICE] (see
// docs/phase-3.11-trade-risk-management.md §3.2): with a price, sets that
// ticker's per-trade stop-loss (db.Position.StopPrice); without one, shows
// the current setting plus three candidate reference prices computed from
// existing history — no new data source. A long position's stop must sit
// below the latest close, so it's rejected (not silently accepted) when it
// doesn't — computeStopSuggestion's LatestClose is the same number the
// candidates themselves were computed against, so this validates against
// exactly what the user is being shown. computeStopSuggestion also degrades
// to a live quote when history is unavailable; only when that also fails is
// there truly nothing to validate against, in which case the set is
// rejected rather than accepted blind.
func (b *Bot) handleStop(args string) {
	ticker, price, hasPrice, err := parseStopArgs(args)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyStopUsage))
		return
	}

	if !hasPrice {
		pos, ok, err := b.db.GetPosition(ticker)
		if err != nil {
			b.Send(i18n.T(b.lang, i18n.KeyQueryFailed, err))
			return
		}
		if !ok {
			b.Send(i18n.T(b.lang, i18n.KeyStopNoPosition, b.tickerLabel(ticker)))
			return
		}
		b.showStop(ticker, pos)
		return
	}

	msg, _ := b.setStop(ticker, price)
	b.Send(msg)
}

// setStop is handleStop's price-setting core (the "<ticker> <price>"
// branch), pulled out so ExecuteSetStop (internal/web's POST /api/stop, see
// docs/phase-10-web-trade-input.md §4.2) validates identically instead of
// duplicating the "must sit below latest close" check — that close is the
// same one computeStopSuggestion already computes for the candidate display,
// so both entry points reject against exactly the number the user would see.
func (b *Bot) setStop(ticker string, price float64) (string, error) {
	r, err := b.risksOrErr()
	if err != nil {
		return i18n.T(b.lang, i18n.KeyQueryFailed, err), err
	}
	res, err := r.SetStop(service.SetStopInput{Ticker: ticker, Price: price})
	if err != nil {
		if errors.Is(err, db.ErrNoPosition) {
			return i18n.T(b.lang, i18n.KeyStopNoPosition, b.tickerLabel(ticker)), err
		}
		var invalidStopErr *service.InvalidStopPriceError
		if errors.As(err, &invalidStopErr) {
			return i18n.T(b.lang, i18n.KeyStopInvalidPrice, b.money(ticker, price), b.money(ticker, invalidStopErr.LatestClose)), err
		}
		return i18n.T(b.lang, i18n.KeyQueryFailed, err), err
	}

	return i18n.T(b.lang, i18n.KeyStopSet, b.tickerLabel(res.Ticker), b.money(res.Ticker, res.StopPrice), res.DistPct, b.money(res.Ticker, res.RiskPerShare)), nil
}

// showStop renders /stop TICKER's no-price branch: the current setting (or
// a note that it falls back to the global STOP_LOSS_PCT) plus the three
// candidate reference prices — each skipped individually when
// computeStopSuggestion couldn't derive it (0 = not enough history), same
// degrade-per-field convention writeStockSection's MA lines already use.
func (b *Bot) showStop(ticker string, pos db.Position) {
	var sb strings.Builder
	if pos.StopPrice > 0 {
		sb.WriteString(i18n.T(b.lang, i18n.KeyStopShow, b.tickerLabel(ticker), b.money(ticker, pos.StopPrice)))
	} else {
		stopLossPct := b.stopLossPct
		if market.Of(ticker) == market.TW {
			stopLossPct = b.stopLossPctTW
		}
		sb.WriteString(i18n.T(b.lang, i18n.KeyStopNotSet, b.tickerLabel(ticker), stopLossPct))
	}

	suggestion, ok := b.computeStopSuggestion(ticker)
	if !ok {
		b.Send(sb.String())
		return
	}
	sb.WriteString(i18n.T(b.lang, i18n.KeyStopCandidatesHeader))
	if suggestion.Low10 > 0 {
		sb.WriteString(i18n.T(b.lang, i18n.KeyStopCandidateLine, i18n.T(b.lang, i18n.KeyStopLow10Label), b.money(ticker, suggestion.Low10)))
	}
	if suggestion.Low20 > 0 {
		sb.WriteString(i18n.T(b.lang, i18n.KeyStopCandidateLine, i18n.T(b.lang, i18n.KeyStopLow20Label), b.money(ticker, suggestion.Low20)))
	}
	if suggestion.ATRBased > 0 {
		sb.WriteString(i18n.T(b.lang, i18n.KeyStopCandidateLine, i18n.T(b.lang, i18n.KeyStopATRLabel), b.money(ticker, suggestion.ATRBased)))
	}
	b.Send(sb.String())
}

// buyStopSuggestion renders the same three candidates as a one-shot
// suggestion line appended to a successful /buy confirmation (§3.2) — never
// blocking or altering the trade confirmation itself, same
// degrade-by-omission convention as computeTechnicals feeding
// fetchStockData. addOnNote is appended when this buy topped up an existing
// position that already has a stop price set, since RecordBuy deliberately
// never adjusts an existing stop automatically (see Position.StopPrice).
func (b *Bot) buyStopSuggestion(ticker string, existingStopPrice float64) string {
	suggestion, ok := b.computeStopSuggestion(ticker)
	if !ok {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(i18n.T(b.lang, i18n.KeyBuyStopSuggestion))
	if suggestion.Low10 > 0 {
		sb.WriteString(i18n.T(b.lang, i18n.KeyStopCandidateLine, i18n.T(b.lang, i18n.KeyStopLow10Label), b.money(ticker, suggestion.Low10)))
	}
	if suggestion.Low20 > 0 {
		sb.WriteString(i18n.T(b.lang, i18n.KeyStopCandidateLine, i18n.T(b.lang, i18n.KeyStopLow20Label), b.money(ticker, suggestion.Low20)))
	}
	if suggestion.ATRBased > 0 {
		sb.WriteString(i18n.T(b.lang, i18n.KeyStopCandidateLine, i18n.T(b.lang, i18n.KeyStopATRLabel), b.money(ticker, suggestion.ATRBased)))
	}
	if existingStopPrice > 0 {
		sb.WriteString(i18n.T(b.lang, i18n.KeyBuyStopAddOnNote, ticker, b.money(ticker, existingStopPrice)))
	}
	return sb.String()
}

// buyAlertDirection infers which side of price a new buy alert should watch
// by comparing it to the live quote at set time — a target at or below the
// current price is a dip watch (db.BuyAlertBelow), one above it is a
// breakout watch (db.BuyAlertAbove). This is what lets /buyalert take a
// single price argument and still support both "notify me on a dip" and
// "notify me on a breakout" without an extra direction flag.
func buyAlertDirection(price, currentPrice float64) string {
	return service.BuyAlertDirection(price, currentPrice)
}

func (b *Bot) buyAlertDirPhrase(direction string) string {
	if direction == db.BuyAlertAbove {
		return i18n.T(b.lang, i18n.KeyBuyAlertDirAbove)
	}
	return i18n.T(b.lang, i18n.KeyBuyAlertDirBelow)
}

// parseBuyAlertArgs parses /buyalert's "<ticker> [price | remove <price>]"
// arguments — mirrors parseStopArgs' shape (price optional, meaning "show me
// the current alerts"), plus a "remove <price>" form since a ticker can carry
// several alerts and /stop's single-column semantics don't need one.
func parseBuyAlertArgs(args string) (ticker string, remove bool, price float64, hasPrice bool, err error) {
	fields := strings.Fields(args)
	if len(fields) < 1 || len(fields) > 3 {
		return "", false, 0, false, fmt.Errorf("expected <ticker> [price | remove <price>]")
	}
	ticker = strings.ToUpper(fields[0])
	if len(fields) == 1 {
		return ticker, false, 0, false, nil
	}
	if len(fields) == 3 {
		if strings.ToLower(fields[1]) != "remove" {
			return "", false, 0, false, fmt.Errorf("expected 'remove' before the price")
		}
		if price, err = strconv.ParseFloat(fields[2], 64); err != nil || price <= 0 {
			return "", false, 0, false, fmt.Errorf("invalid price %q", fields[2])
		}
		return ticker, true, price, true, nil
	}
	if price, err = strconv.ParseFloat(fields[1], 64); err != nil || price <= 0 {
		return "", false, 0, false, fmt.Errorf("invalid price %q", fields[1])
	}
	return ticker, false, price, true, nil
}

// handleBuyAlert is /buyalert TICKER [PRICE | remove PRICE] (see setStop's
// doc comment for the parallel /stop design): with no price, lists the
// ticker's current buy alerts; with a price, adds one (direction inferred,
// see buyAlertDirection); with "remove PRICE", deletes the matching one.
// Unlike /stop, no open position is required — a buy alert's whole purpose
// is watching a ticker the user doesn't hold yet.
func (b *Bot) handleBuyAlert(args string) {
	ticker, remove, price, hasPrice, err := parseBuyAlertArgs(args)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyBuyAlertUsage))
		return
	}

	if !hasPrice {
		b.showBuyAlerts(ticker)
		return
	}

	if remove {
		b.removeBuyAlert(ticker, price)
		return
	}

	msg, _ := b.addBuyAlert(ticker, price)
	b.Send(msg)
}

// addBuyAlert is handleBuyAlert's price-setting core, pulled out so
// ExecuteAddBuyAlert (internal/web's POST /api/buy-alerts/add) validates and
// infers direction identically instead of duplicating the quote fetch —
// same split as setStop/ExecuteSetStop.
func (b *Bot) addBuyAlert(ticker string, price float64) (string, error) {
	r, err := b.risksOrErr()
	if err != nil {
		return i18n.T(b.lang, i18n.KeyBuyAlertQueryFailed, err), err
	}
	res, err := r.AddBuyAlert(service.BuyAlertInput{Ticker: ticker, Price: price})
	if err != nil {
		return i18n.T(b.lang, i18n.KeyBuyAlertQueryFailed, err), err
	}

	return i18n.T(b.lang, i18n.KeyBuyAlertSet, b.tickerLabel(res.Ticker), b.money(res.Ticker, res.Price), b.buyAlertDirPhrase(res.Direction)), nil
}

// showBuyAlerts renders /buyalert TICKER's no-price branch: every alert
// currently set on ticker, oldest first.
func (b *Bot) showBuyAlerts(ticker string) {
	r, err := b.risksOrErr()
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyBuyAlertQueryFailed, err))
		return
	}
	alerts, err := r.GetBuyAlerts(ticker)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyBuyAlertQueryFailed, err))
		return
	}
	if len(alerts) == 0 {
		b.Send(i18n.T(b.lang, i18n.KeyBuyAlertEmpty, b.tickerLabel(ticker)))
		return
	}

	var sb strings.Builder
	sb.WriteString(i18n.T(b.lang, i18n.KeyBuyAlertListHeader, b.tickerLabel(ticker)))
	for _, a := range alerts {
		sb.WriteString(i18n.T(b.lang, i18n.KeyBuyAlertLine, b.money(ticker, a.Price), b.buyAlertDirPhrase(a.Direction)))
	}
	b.Send(sb.String())
}

// removeBuyAlert deletes the alert(s) on ticker matching price exactly —
// there's no id exposed over Telegram, so price is the only handle the user
// has (same tradeoff as most chat commands operating on human-entered
// numbers rather than internal ids).
func (b *Bot) removeBuyAlert(ticker string, price float64) {
	r, err := b.risksOrErr()
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyBuyAlertQueryFailed, err))
		return
	}
	removed, err := r.RemoveBuyAlertByPrice(ticker, price)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyBuyAlertQueryFailed, err))
		return
	}
	if !removed {
		b.Send(i18n.T(b.lang, i18n.KeyBuyAlertNotFound, b.tickerLabel(ticker), b.money(ticker, price)))
		return
	}
	b.Send(i18n.T(b.lang, i18n.KeyBuyAlertRemoved, b.tickerLabel(ticker), b.money(ticker, price)))
}

// ExecuteAddBuyAlert is internal/web's POST /api/buy-alerts/add entry point
// (see docs/phase-10-web-trade-input.md §4.2's "one write path" rule,
// mirrored by TradeExecutor.ExecuteSetStop) — a thin wrapper around
// addBuyAlert so the web handler never duplicates the quote-fetch/direction-
// inference logic.
func (b *Bot) ExecuteAddBuyAlert(ticker string, price float64) (string, error) {
	return b.addBuyAlert(ticker, price)
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
	ticker, shares, price, fee, feeSet, date, err := parseTradeArgs(args)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyBuyUsage))
		return
	}
	if date == "" {
		date = todayDate()
	}
	feeAuto := !feeSet
	msg, _ := b.recordBuy(ticker, shares, price, fee, feeAuto, date, "")
	b.Send(msg)
}

// recordBuy is handleBuy's core, pulled out so a confirmed Phase 4
// pending-action proposal (record_buy, see internal/mcptools'
// trade_write_tools.go) can execute the exact same logic and produce the
// exact same confirmation text as typing /buy directly — see
// executePendingAction. err is nil on success, purely for a caller that
// needs to distinguish success/failure programmatically (ExecuteBuy, see
// docs/phase-10-web-trade-input.md §4.2) without re-parsing msg's already
// i18n-rendered text. extID is "" for every caller except Phase 16's
// Shioaji-synced pending actions — see db.RecordBuyExt.
func (b *Bot) recordBuy(ticker string, shares, price, fee float64, feeAuto bool, date, extID string) (string, error) {
	// Read any stop price already on the position before the buy — the trade
	// service deliberately doesn't touch it, but buyStopSuggestion's add-on
	// note needs to know it was there.
	//
	// Read this before executing because a buy that tops up an existing
	// position deliberately keeps its existing stop price.
	var existingStopPrice float64
	if prevPos, ok, err := b.db.GetPosition(ticker); err != nil {
		logger.Errorf("buy %s: get existing position: %v", ticker, err)
	} else if ok {
		existingStopPrice = prevPos.StopPrice
	}

	trades := b.trading()
	if trades == nil {
		err := errors.New("trade service unavailable")
		return i18n.T(b.lang, i18n.KeyBuyFailed, err), err
	}
	var feeInput *float64
	if !feeAuto {
		feeInput = &fee
	}
	result, err := trades.Buy(service.BuyInput{
		Ticker: ticker,
		Shares: shares,
		Price:  price,
		Fee:    feeInput,
		Date:   date,
		ExtID:  extID,
	})
	if err != nil {
		return i18n.T(b.lang, i18n.KeyBuyFailed, err), err
	}
	b.adjustCash(result.Ticker, -(shares*price + result.Fee))
	msg := i18n.T(b.lang, i18n.KeyBuySuccess, result.Ticker, shares, b.money(result.Ticker, price), b.money(result.Ticker, result.Fee), result.Position.Shares, b.money(result.Ticker, result.Position.AvgCost))
	if feeAuto {
		msg += i18n.T(b.lang, i18n.KeyFeeAutoNote)
	}
	msg += b.thesisNudge(result.Ticker)
	return msg + b.buyStopSuggestion(result.Ticker, existingStopPrice), nil
}

// ExecuteBuy is ExecuteSell/ExecuteSetStop's sibling: internal/web's POST
// /api/trade/buy calls this instead of db.RecordBuy directly, so a web
// order gets the exact same watchlist auto-add /buy itself produces (see
// docs/phase-10-web-trade-input.md §4.2's "one write path" rule).
// Validation mirrors parseTradeArgs' semantics; date is expected already
// resolved (defaulted to today) by the caller, the same "date defaults in
// the handler, not the shared function" convention parseTradeArgs' own
// callers follow. Unlike before Phase 24 tech debt 3, this no longer pushes
// a Telegram confirmation itself — the caller decides whether to notify (see
// Notify), so a web trade's Telegram push is now internal/web's explicit
// choice rather than baked into every Execute* call.
func (b *Bot) ExecuteBuy(ticker string, shares, price float64, fee *float64, date string) (string, error) {
	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	if ticker == "" || shares <= 0 || price <= 0 || (fee != nil && *fee < 0) {
		msg := i18n.T(b.lang, i18n.KeyBuyUsage)
		return msg, fmt.Errorf("invalid buy arguments")
	}
	feeAuto := fee == nil
	feeVal := 0.0
	if !feeAuto {
		feeVal = *fee
	}
	return b.recordBuy(ticker, shares, price, feeVal, feeAuto, date, "")
}

// Notify pushes msg to Telegram on the caller's behalf. It exists so
// internal/web's write handlers (ExecuteBuy/ExecuteSell/ExecuteSetStop
// callers) can still surface the same confirmation Telegram would show for
// the equivalent slash command — the "Telegram 照常同步" decision from
// docs/phase-10-web-trade-input.md §4.2 — without that push being an
// unconditional side effect of Execute* itself (Phase 24 tech debt 3).
func (b *Bot) Notify(msg string) {
	b.Send(msg)
}

// thesisNudge returns a one-line nudge to record a holding thesis when
// ticker doesn't have one yet, or "" when it already does (or the lookup
// fails — a nudge is a courtesy, not worth failing the trade confirmation
// over). Called only from handleBuy, never blocking the trade itself — see
// PLAN.md's Phase 3.6 expansion "論點日誌" item.
func (b *Bot) thesisNudge(ticker string) string {
	_, ok, err := b.db.GetThesis(ticker)
	if err != nil {
		logger.Errorf("buy: check thesis %s: %v", ticker, err)
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
	ticker, shares, price, fee, feeSet, date, err := parseTradeArgs(args)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeySellUsage))
		return
	}
	if date == "" {
		date = todayDate()
	}
	feeAuto := !feeSet
	msg, closed, stopPrice, _ := b.recordSell(ticker, shares, price, fee, feeAuto, date, "")
	b.Send(msg)
	if closed {
		go b.reviewClosedTrade(ctx, ticker, stopPrice)
	}
}

// recordSell is handleSell's core, pulled out for the same reason as
// recordBuy — a confirmed Phase 4 pending-action proposal (record_sell)
// reuses this instead of duplicating the RecordSell call and error mapping.
// closed reports whether this sell fully closed out the position (shares
// returned to 0), so callers can decide whether to trigger a sell-review;
// it's always false on an error path, since nothing was recorded. stopPrice
// is the position's stop price as it stood right before this sell (Phase
// 3.11 PR1 §3.5) — read via GetPosition *before* calling db.RecordSell,
// since a full close deletes the positions row and takes the stop price
// with it; always 0 on an error path or when no stop had ever been set. err
// is nil on success, same "for a programmatic caller, not for re-parsing
// msg" purpose as recordBuy's (see ExecuteSell). extID is "" for every
// caller except Phase 16's Shioaji-synced pending actions.
func (b *Bot) recordSell(ticker string, shares, price, fee float64, feeAuto bool, date, extID string) (msg string, closed bool, stopPrice float64, err error) {
	if prevPos, ok, gerr := b.db.GetPosition(ticker); gerr != nil {
		logger.Errorf("sell %s: get position for stop price: %v", ticker, gerr)
	} else if ok {
		stopPrice = prevPos.StopPrice
	}

	trades := b.trading()
	if trades == nil {
		err = errors.New("trade service unavailable")
		return i18n.T(b.lang, i18n.KeySellFailed, err), false, 0, err
	}
	var feeInput *float64
	if !feeAuto {
		feeInput = &fee
	}
	result, err := trades.Sell(service.SellInput{
		Ticker: ticker,
		Shares: shares,
		Price:  price,
		Fee:    feeInput,
		Date:   date,
		ExtID:  extID,
	})
	if err != nil {
		switch {
		case errors.Is(err, db.ErrNoPosition):
			return i18n.T(b.lang, i18n.KeySellNoPosition, ticker), false, 0, err
		case errors.Is(err, db.ErrInsufficientShares):
			return i18n.T(b.lang, i18n.KeySellInsufficientShares, ticker), false, 0, err
		default:
			return i18n.T(b.lang, i18n.KeySellFailed, err), false, 0, err
		}
	}
	b.adjustCash(result.Ticker, shares*price-result.Fee)
	msg = i18n.T(b.lang, i18n.KeySellSuccess, result.Ticker, shares, b.money(result.Ticker, price), b.money(result.Ticker, result.Fee), result.RealizedPnL, result.Position.Shares)
	if feeAuto {
		msg += i18n.T(b.lang, i18n.KeyFeeAutoNote)
	}
	return msg, result.Closed, result.StopPrice, nil
}

// ExecuteSell is ExecuteBuy's sibling for internal/web's POST
// /api/trade/sell (see docs/phase-10-web-trade-input.md §4.2) — same
// validation shape (Telegram parity now via Notify, not built in — see
// ExecuteBuy), plus triggering the same sell-review goroutine handleSell
// kicks off on a closing sell.
func (b *Bot) ExecuteSell(ctx context.Context, ticker string, shares, price float64, fee *float64, date string) (string, error) {
	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	if ticker == "" || shares <= 0 || price <= 0 || (fee != nil && *fee < 0) {
		msg := i18n.T(b.lang, i18n.KeySellUsage)
		return msg, fmt.Errorf("invalid sell arguments")
	}
	feeAuto := fee == nil
	feeVal := 0.0
	if !feeAuto {
		feeVal = *fee
	}
	msg, closed, stopPrice, err := b.recordSell(ticker, shares, price, feeVal, feeAuto, date, "")
	if closed {
		go b.reviewClosedTrade(ctx, ticker, stopPrice)
	}
	return msg, err
}

// ExecuteSetStop is ExecuteBuy's sibling for internal/web's POST /api/stop
// (see docs/phase-10-web-trade-input.md §4.2), reusing setStop's validation
// so the web form rejects a stop price at/above the latest close the exact
// same way /stop TICKER PRICE does.
func (b *Bot) ExecuteSetStop(ticker string, price float64) (string, error) {
	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	if ticker == "" || price <= 0 {
		msg := i18n.T(b.lang, i18n.KeyStopUsage)
		return msg, fmt.Errorf("invalid stop arguments")
	}
	return b.setStop(ticker, price)
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
	}

	if high, low, ok, err := b.db.GetCloseExtremes(ticker, round.StartDate, round.EndDate); err != nil {
		logger.Errorf("review %s: close extremes: %v", ticker, err)
	} else if ok {
		trade.PeriodHigh = high
		trade.PeriodLow = low
	}

	if entryPrice := weightedAvgPrice(round.Legs, "BUY"); entryPrice > 0 {
		exitPrice := weightedAvgPrice(round.Legs, "SELL")
		spyStart, startOK, startErr := b.db.GetSnapshotClose(benchmarkTicker, round.StartDate)
		spyEnd, endOK, endErr := b.db.GetSnapshotClose(benchmarkTicker, round.EndDate)
		if startErr != nil || endErr != nil {
			logger.Errorf("review %s: spy close: start err=%v end err=%v", ticker, startErr, endErr)
		} else if startOK && endOK {
			vs := computeVsSPY(exitPrice, entryPrice, spyEnd, spyStart, benchmarkTicker)
			trade.VsSPY = &vs
		}
	}

	if thesis, ok, err := b.db.GetThesis(ticker); err != nil {
		logger.Errorf("review %s: thesis: %v", ticker, err)
	} else if ok {
		trade.Thesis = &thesis
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

// eventsListLimit/eventsTickerLimit cap /events' two forms — starting-point
// constants per docs/phase-20-price-event-log.md §6, same "not calibrated,
// revisit if it matters" caveat as priceEventWriteupCap (jobs.go).
const (
	eventsListLimit   = 20
	eventsTickerLimit = 20
)

// handleEvents is Phase 20's /events [TICKER] — with no argument, the most
// recent price events across every ticker; with one, that ticker's full
// history. Both forms must reply even when the result is empty (same UX
// lesson as /recommend's empty-result fix, see docs/phase-20-price-event-log.md
// §4.4) rather than going silent.
func (b *Bot) handleEvents(args string) {
	ticker := strings.ToUpper(strings.TrimSpace(args))

	var events []db.PriceEvent
	var err error
	var title string
	fullSummary := ticker != ""
	if ticker == "" {
		events, err = b.db.GetRecentPriceEvents(eventsListLimit)
		title = i18n.T(b.lang, i18n.KeyEventsListTitle)
	} else {
		events, err = b.db.GetPriceEventsForTicker(ticker, eventsTickerLimit)
		title = i18n.T(b.lang, i18n.KeyEventsTickerTitle, ticker)
	}
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyQueryFailed, err))
		return
	}
	if len(events) == 0 {
		b.Send(i18n.T(b.lang, i18n.KeyEventsEmpty))
		return
	}

	var sb strings.Builder
	sb.WriteString(title)
	for _, ev := range events {
		summary := ev.Summary
		switch {
		case summary == "":
			summary = i18n.T(b.lang, i18n.KeyEventsNoSummary)
		case !fullSummary:
			// The no-argument form is a cross-ticker digest — show only the
			// summary's first line/sentence so one event doesn't push the
			// rest off the message (see handleEvents' doc comment).
			if i := strings.IndexByte(summary, '\n'); i >= 0 {
				summary = summary[:i]
			}
		}
		sb.WriteString(i18n.T(b.lang, i18n.KeyEventsLine, ev.Date, ev.Ticker, ev.GapPct, ev.ChangePct, ev.CumulativePct, summary))
	}
	b.Send(sb.String())
}

// handlePortfolio shows every open position's current market value and
// unrealized P&L against a live quote, plus cumulative realized P&L across
// all past sells. Each position goes out as its own message (same reasoning
// as sendAndSaveRecommendations — see that doc comment) so its
// [Check]/[Buy]/[Sell] quick-action row (quick_actions.go) attaches to that
// ticker specifically.
func (b *Bot) handlePortfolio() {
	portfolio := b.portfolios()
	if portfolio == nil {
		b.Send(i18n.T(b.lang, i18n.KeyQueryFailed, errors.New("portfolio service unavailable")))
		return
	}
	usSnapshot, err := portfolio.Snapshot(market.US)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyQueryFailed, err))
		return
	}
	twSnapshot, err := portfolio.Snapshot(market.TW)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyQueryFailed, err))
		return
	}
	optionPositions, err := b.db.GetOptionPositions()
	if err != nil {
		logger.Errorf("portfolio: option positions: %v", err)
	}
	if len(usSnapshot.Positions) == 0 && len(twSnapshot.Positions) == 0 && len(optionPositions) == 0 {
		b.Send(i18n.T(b.lang, i18n.KeyPortfolioEmpty))
		return
	}

	b.Send(i18n.T(b.lang, i18n.KeyPortfolioTitle))
	b.sendPortfolioSection(usSnapshot)
	b.sendPortfolioSection(twSnapshot)
	b.sendPortfolioOptionsSection(optionPositions)
}

// sendPortfolioSection renders /portfolio's per-market block (Phase 6, see
// docs/phase-6-tw-market.md §4.3): a section title tagged with the market's
// currency, each of that market's positions, and a market-scoped subtotal
// (GetRealizedPnL(m) — realized P&L never sums across markets). Sends
// nothing at all — not even the section title — when m has no open
// positions, so a single-market portfolio doesn't show a dangling empty
// block for the other market.
func (b *Bot) sendPortfolioSection(snapshot service.PortfolioSnapshot) {
	if len(snapshot.Positions) == 0 {
		return
	}

	m := snapshot.Market
	if snapshot.RealizedPnLErr != nil {
		logger.Errorf("portfolio: realized pnl (%s): %v", m, snapshot.RealizedPnLErr)
	}

	b.Send(i18n.T(b.lang, portfolioSectionTitleKey(m)))
	for _, valuation := range snapshot.Positions {
		p := valuation.Position
		if valuation.QuoteErr != nil || valuation.Quote == nil {
			if valuation.QuoteErr != nil {
				logger.Errorf("portfolio: quote %s: %v", p.Ticker, valuation.QuoteErr)
			}
			b.sendWithTickerActions(p.Ticker, i18n.T(b.lang, i18n.KeyQuoteUnavailable, b.tickerLabel(p.Ticker)))
			continue
		}
		line := i18n.T(b.lang, i18n.KeyPortfolioLine, b.tickerLabel(p.Ticker), p.Shares, b.money(p.Ticker, p.AvgCost), b.money(p.Ticker, valuation.Quote.Price), b.money(p.Ticker, valuation.MarketValue), valuation.UnrealizedPnL, valuation.UnrealizedPnLPct)
		if note := lotSuffix(b.lang, m, p.Shares); note != "" {
			// KeyPortfolioLine ends "...%%)\n\n" — splice the note onto that
			// last content line rather than appending after the trailing
			// blank line, so it reads as part of the same position summary.
			line = strings.TrimSuffix(line, "\n\n") + note + "\n\n"
		}
		b.sendWithTickerActions(p.Ticker, line)
	}
	if snapshot.CashErr != nil {
		logger.Errorf("portfolio: load cash (%s): %v", m, snapshot.CashErr)
	}
	cash, haveCash := snapshot.Cash, snapshot.HasCash
	realizedTotal := snapshot.RealizedPnL
	totalValue := snapshot.TotalMarketValue

	if m == market.TW {
		if haveCash {
			b.Send(i18n.T(b.lang, i18n.KeyPortfolioSummaryWithCashTWD, totalValue, realizedTotal, totalValue+cash, cash))
		} else {
			b.Send(i18n.T(b.lang, i18n.KeyPortfolioSummaryTWD, totalValue, realizedTotal))
		}
		return
	}
	if haveCash {
		b.Send(i18n.T(b.lang, i18n.KeyPortfolioSummaryWithCash, totalValue, realizedTotal, totalValue+cash, cash))
	} else {
		b.Send(i18n.T(b.lang, i18n.KeyPortfolioSummary, totalValue, realizedTotal))
	}
}

// portfolioSectionTitleKey selects /portfolio's per-market section title.
func portfolioSectionTitleKey(m market.MarketID) i18n.Key {
	if m == market.TW {
		return i18n.KeyPortfolioSectionTW
	}
	return i18n.KeyPortfolioSectionUS
}

// twLotSize is one 台股 board lot (張) — 1,000 shares, the unit Taiwanese
// investors actually think in even though this project records TW positions
// in raw shares like US ones (§3 of docs/phase-6-tw-market.md: "單位一律
// 股").
const twLotSize = 1000

// lotSuffix appends a "(= N 張)" note to a TW portfolio line when shares is
// an exact multiple of a board lot (1,000) — a non-round share count (e.g.
// a partial fill) simply gets no note, same as any other "not enough data"
// omission in this codebase. Always "" for US, where board lots aren't a
// concept.
func lotSuffix(lang i18n.Lang, m market.MarketID, shares float64) string {
	if m != market.TW || shares < twLotSize {
		return ""
	}
	lots := shares / twLotSize
	if lots != math.Trunc(lots) {
		return ""
	}
	return i18n.T(lang, i18n.KeyPortfolioLotSuffix, int(lots))
}

// handleInsight is Phase 3.6's portfolio-level analysis command: unlike
// /recommend (a per-candidate scanning view) or /check (a single ticker),
// this steps back to look at the whole set of holdings together —
// concentration risk, whether each position's original thesis still holds,
// add/reduce/stop-loss suggestions. Reuses fetchStockData/loadEarnings
// exactly like /recommend and RunDailyReport do (same attach-and-render
// StockData fields: technicals, fundamentals, earnings, cost basis) rather
// than building a separate data-gathering path.
//
// One message per market, each from its own LLM call over only that market's
// holdings and cash — same reasoning as RunWeeklyReview's split: a combined
// call weighed TWD position sizes against USD ones and summed both into one
// "total assets" number.
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

	for _, m := range []market.MarketID{market.US, market.TW} {
		b.insightMarket(ctx, m, positions)
	}
}

// insightMarket is handleInsight's per-market half: one LLM call and one
// message covering market m only, silently skipped when m holds nothing.
func (b *Bot) insightMarket(ctx context.Context, m market.MarketID, positions []db.Position) {
	stocks, _ := b.portfolioStocks(m, positions)
	if len(stocks) == 0 {
		return
	}

	cash, haveCash, err := b.loadCash(m)
	if err != nil {
		logger.Errorf("insight: load cash (%s): %v", m, err)
	}

	result, err := b.llm.InsightPortfolio(ctx, stocks, cash, haveCash, m == market.TW)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyLLMFailed, err))
		return
	}

	titleKey := i18n.KeyInsightResultTitleUS
	if m == market.TW {
		titleKey = i18n.KeyInsightResultTitleTW
	}
	b.Send(i18n.T(b.lang, titleKey, result))
}

// cashSettingKey/cashSettingKeyTWD are the db.settings keys /cash reads/
// writes — see db.GetSetting/SetSetting's Phase 3.6 doc comment.
// cashSettingKey (USD) predates Phase 6 and keeps its original key name for
// zero-migration backward compatibility; cashSettingKeyTWD is Phase 6's new
// second book (see docs/phase-6-tw-market.md §3.2 — TWD/USD never convert,
// so this is a second independent value, not a currency-converted view of
// the first).
const (
	cashSettingKey    = service.CashSettingKeyUSD
	cashSettingKeyTWD = service.CashSettingKeyTWD
)

// cashSettingKeyFor returns the settings key backing m's cash balance.
func cashSettingKeyFor(m market.MarketID) string {
	return service.CashSettingKey(m)
}

// handleCash manages the user's manually-declared cash balance, one per
// market (Phase 3.6, extended by Phase 6 to two books — see §3.2). With no
// argument it reports both currencies' current values (omitting whichever
// isn't set); `/cash <amount>` sets USD (backward compatible with the
// pre-Phase-6 single-currency form); `/cash usd <amount>`/`/cash twd
// <amount>` set a currency explicitly. /buy and /sell nudge this value via
// adjustCash once it's been declared here at least once (see adjustCash) —
// deposits/withdrawals/dividends never flow through the bot, so this stays a
// user-corrected reference value, not a ledger; re-run /cash to resync it
// against reality whenever it drifts. Fed into /insight/週報 (see
// handleInsight) — never into /recommend, so the model doesn't see idle cash
// and start nudging toward "put it to work."
func (b *Bot) handleCash(args string) {
	args = strings.TrimSpace(args)
	if args == "" {
		b.Send(b.renderCashStatus())
		return
	}

	m, amount, err := parseCashArgs(args)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyCashUsage))
		return
	}
	if err := b.db.SetSetting(cashSettingKeyFor(m), strconv.FormatFloat(amount, 'f', 2, 64)); err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyCashSetFailed, err))
		return
	}
	if m == market.TW {
		b.Send(i18n.T(b.lang, i18n.KeyCashSetSuccessTWD, amount))
	} else {
		b.Send(i18n.T(b.lang, i18n.KeyCashSetSuccess, amount))
	}
}

// parseCashArgs parses handleCash's non-empty argument string: a bare
// amount (backward compatible, defaults to USD), or "usd <amount>"/
// "twd <amount>" to pick a currency explicitly (see §3.2 of
// docs/phase-6-tw-market.md). Pure and separately tested, same convention
// as parseTradeArgs/parseStopArgs — the caller (handleCash) owns rendering
// the usage message on error.
func parseCashArgs(args string) (m market.MarketID, amount float64, err error) {
	fields := strings.Fields(args)
	amountArg := args
	m = market.US

	switch len(fields) {
	case 1:
		amountArg = fields[0]
	case 2:
		switch strings.ToLower(fields[0]) {
		case "twd":
			m = market.TW
			amountArg = fields[1]
		case "usd":
			m = market.US
			amountArg = fields[1]
		default:
			return "", 0, fmt.Errorf("unknown currency %q", fields[0])
		}
	default:
		return "", 0, fmt.Errorf("cash: unexpected argument shape %q", args)
	}

	amount, err = strconv.ParseFloat(amountArg, 64)
	if err != nil {
		return "", 0, err
	}
	if amount < 0 {
		return "", 0, fmt.Errorf("cash: negative amount %v", amount)
	}
	return m, amount, nil
}

// renderCashStatus renders /cash's no-argument reply: one line per currency
// that's actually been set, or KeyCashNotSet if neither has.
func (b *Bot) renderCashStatus() string {
	usd, usdOK, errUSD := b.loadCash(market.US)
	if errUSD != nil {
		return i18n.T(b.lang, i18n.KeyQueryFailed, errUSD)
	}
	twd, twdOK, errTWD := b.loadCash(market.TW)
	if errTWD != nil {
		return i18n.T(b.lang, i18n.KeyQueryFailed, errTWD)
	}
	if !usdOK && !twdOK {
		return i18n.T(b.lang, i18n.KeyCashNotSet)
	}
	var sb strings.Builder
	if usdOK {
		sb.WriteString(i18n.T(b.lang, i18n.KeyCashCurrent, usd))
	}
	if twdOK {
		if usdOK {
			sb.WriteString("\n")
		}
		sb.WriteString(i18n.T(b.lang, i18n.KeyCashCurrentTWD, twd))
	}
	return sb.String()
}

// loadCash returns m's declared cash balance, or ok=false if /cash has
// never set it.
func (b *Bot) loadCash(m market.MarketID) (float64, bool, error) {
	raw, ok, err := b.db.GetSetting(cashSettingKeyFor(m))
	if err != nil || !ok {
		return 0, ok, err
	}
	amount, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false, err
	}
	return amount, true, nil
}

// adjustCash nudges ticker's market cash balance by delta — negative for a
// buy's cost (shares*price+fee), positive for a sell's proceeds
// (shares*price-fee) — but only when the user has already declared a
// balance via /cash; with no declared baseline there's nothing sane to
// adjust from, so this is a no-op rather than inventing one at delta. Errors
// are log-only, same as thesisNudge/AddTicker: a trade confirmation
// shouldn't fail because cash bookkeeping did.
func (b *Bot) adjustCash(ticker string, delta float64) {
	m := market.Of(ticker)
	cash, ok, err := b.loadCash(m)
	if err != nil {
		logger.Errorf("adjust cash for %s: load: %v", ticker, err)
		return
	}
	if !ok {
		return
	}
	if err := b.db.SetSetting(cashSettingKeyFor(m), strconv.FormatFloat(cash+delta, 'f', 2, 64)); err != nil {
		logger.Errorf("adjust cash for %s: save: %v", ticker, err)
	}
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
		b.Send(i18n.T(b.lang, i18n.KeyFundamentalsFailed, b.tickerLabel(ticker), fdErr))
		return
	}

	var sb strings.Builder
	sb.WriteString(i18n.T(b.lang, i18n.KeyFundamentalsTitle, b.tickerLabel(ticker)))
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
	for _, source := range []string{"sp500", "tw", "manual"} {
		if n := bySource[source]; n > 0 {
			sb.WriteString(i18n.T(b.lang, i18n.KeyUniverseSourceLine, source, n))
		}
	}
	b.Send(sb.String())
}

// SyncUniverse is Phase 2.6 追加項's S&P 500 refresh entry point (see
// docs/phase-2.6-universe-refresh.md), called once from main() right after
// New() (after seedSP500's fresh-install path has already run, so a brand
// new DB syncs to a no-op immediately). New constituents are inserted
// silently — log-only, no Telegram — since there's nothing for the user to
// decide about a new index addition worth scanning. Constituents dropped
// from the embedded list are never auto-removed (being cut from the index
// isn't the same as delisted/dead), so they're surfaced as one consolidated
// Telegram suggestion instead; a query failure also just logs, since this
// runs unattended at startup with no command context to report an error
// against. A fully quiet run (nothing added, nothing delisted) is the
// overwhelmingly common case — the daily-scheduled deploy restarts this
// process far more often than the embedded ticker list actually changes —
// so silence here is a hard requirement, not an edge case: anything else
// would mean a near-daily Telegram notification about nothing.
func (b *Bot) SyncUniverse() {
	added, delisted, err := b.db.SyncSP500()
	if err != nil {
		logger.Errorf("sync universe: %v", err)
		return
	}
	if len(added) > 0 {
		logger.Infof("sync universe: added %d new S&P 500 ticker(s): %s", len(added), strings.Join(added, ", "))
	}
	if len(delisted) == 0 {
		return
	}

	var sb strings.Builder
	sb.WriteString(i18n.T(b.lang, i18n.KeyUniverseDelistedTitle))
	for _, t := range delisted {
		sb.WriteString(i18n.T(b.lang, i18n.KeyUniverseDelistedLine, t, t))
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
			b.Send(i18n.T(b.lang, i18n.KeyThesisNotSet, b.tickerLabel(ticker), ticker))
			return
		}
		b.Send(i18n.T(b.lang, i18n.KeyThesisCurrent, b.tickerLabel(ticker), thesis))
		return
	}

	thesis := strings.TrimSpace(parts[1])
	if err := b.db.SetThesis(ticker, thesis); err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyThesisSetFailed, b.tickerLabel(ticker), err))
		return
	}
	b.Send(i18n.T(b.lang, i18n.KeyThesisSetSuccess, b.tickerLabel(ticker), thesis))
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
		logger.Errorf("chat: article fetch %s: %v", url, err)
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
		logger.Errorf("chat context: watchlist: %v", err)
	}
	positions, err := b.db.GetPositions()
	if err != nil {
		logger.Errorf("chat context: positions: %v", err)
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
			logger.Errorf("chat context: snapshot %s: %v", t, err)
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
