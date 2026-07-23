package mcptools

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/market"
)

// benchmarkTicker mirrors internal/bot's own benchmarkTicker (SPY) —
// duplicated rather than imported for the same package-boundary reason as
// formatFundamentals/commaf in tools.go. Never written to any DB table
// itself; only used to look up same-period daily_snapshots closes that
// RunClosingSnapshot's snapshotBenchmark already records there.
const benchmarkTicker = "SPY"

const (
	// trackDefaultDays/trackMaxDays mirror /track's own defaults
	// (handleTrack in internal/bot) — a tool caller gets the same look-back
	// window a human typing /track would.
	trackDefaultDays = 7
	trackMaxDays     = 90

	// recentRecsMaxRows caps get_recent_recommendations output — each row
	// carries a full LLM-written reason paragraph, so an uncapped 90-day
	// dump could flood the chat model's context.
	recentRecsMaxRows = 50
)

// registerDBTools adds the Phase 3.5 "追加項" read-only DB query tools —
// get_watchlist/get_portfolio/get_recommendation_stats/
// get_recent_recommendations/get_universe_summary — when ts.db is non-nil
// (see db.OpenReadOnly's doc comment for why this package is now allowed to
// hold a DB connection at all, and NewServer's doc comment for the
// nil-degrade contract: a failed open takes down these five tools only, not
// the whole MCP server).
func registerDBTools(s *mcp.Server, ts *toolset) {
	if ts.db == nil {
		return
	}

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_watchlist",
		Description: "Get every ticker on the user's watchlist.",
	}, ts.getWatchlist)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_portfolio",
		Description: "Get the user's current stock positions — shares held, average cost, live price, market value, and unrealized P&L per ticker — plus all-time cumulative realized P&L.",
	}, ts.getPortfolio)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_recommendation_stats",
		Description: "Get hit-rate and average-return statistics for past BUY/SELL recommendations over a look-back window (same scoring /track uses: relative to SPY's same-period move when available), broken down by candidate source (watchlist/movers/scan) when more than one appears in the window.",
	}, ts.getRecommendationStats)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_recent_recommendations",
		Description: "List the individual BUY/SELL/HOLD recommendations the bot itself has made — date, ticker, action, price at recommendation time, candidate source, and the full reasoning — over a look-back window, newest first, optionally filtered to one ticker. Use this to answer what was recommended and why; for hit-rate/return scoring of those calls use get_recommendation_stats instead.",
	}, ts.getRecentRecommendations)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_universe_summary",
		Description: "Get a count summary of the candidate scan pool (the universe table) by source — S&P 500 seed vs manually added via /universe add.",
	}, ts.getUniverseSummary)
}

type recommendationStatsInput struct {
	Days int `json:"days,omitempty" jsonschema:"look-back window in days (default 7, max 90)"`
}

func (ts *toolset) getWatchlist(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, any, error) {
	result, err := ts.withCache(ctx, "get_watchlist", longCacheTTL, func() (*mcp.CallToolResult, error) {
		tickers, err := ts.db.GetWatchlist()
		if err != nil {
			return nil, ts.mcpErr(i18n.KeyQueryFailed, err)
		}
		if len(tickers) == 0 {
			return nil, ts.mcpErr(i18n.KeyWatchlistEmptyHint)
		}
		text := i18n.T(ts.lang, i18n.KeyWatchlistTitle) + strings.Join(tickers, "\n")
		return textResult(text), nil
	})
	return result, nil, err
}

// getPortfolio mirrors internal/bot's handlePortfolio/sendPortfolioSection
// (core logic only, not the Telegram-send tail) — one live quote per
// position for market value/unrealized P&L, plus each market's own
// all-time realized P&L (Phase 6: money never sums across markets, see
// docs/phase-6-tw-market.md §3.2 and CLAUDE.md's can't-share-an-import note
// on why this is its own copy rather than an internal/bot import). Kept in
// quoteCacheTTL (not longCacheTTL) since, like get_quote, its numbers are
// live prices a chat model might reasonably re-check within the same
// conversation.
func (ts *toolset) getPortfolio(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, any, error) {
	result, err := ts.withCache(ctx, "get_portfolio", quoteCacheTTL, func() (*mcp.CallToolResult, error) {
		positions, err := ts.db.GetPositions()
		if err != nil {
			return nil, ts.mcpErr(i18n.KeyQueryFailed, err)
		}
		if len(positions) == 0 {
			return nil, ts.mcpErr(i18n.KeyPortfolioEmpty)
		}

		var sb strings.Builder
		sb.WriteString(i18n.T(ts.lang, i18n.KeyPortfolioTitle))
		ts.writePortfolioSection(&sb, market.US, positions)
		ts.writePortfolioSection(&sb, market.TW, positions)
		return textResult(sb.String()), nil
	})
	return result, nil, err
}

// writePortfolioSection renders one market's block into sb — see
// bot.sendPortfolioSection's doc comment for the shared logic this mirrors.
// Writes nothing at all when m has no open positions.
func (ts *toolset) writePortfolioSection(sb *strings.Builder, m market.MarketID, positions []db.Position) {
	var marketPositions []db.Position
	for _, p := range positions {
		if market.Of(p.Ticker) == m {
			marketPositions = append(marketPositions, p)
		}
	}
	if len(marketPositions) == 0 {
		return
	}

	realizedTotal, err := ts.db.GetRealizedPnL(m)
	if err != nil {
		log.Printf("mcptools: get_portfolio realized pnl (%s): %v", m, err)
	}

	sectionTitle := i18n.KeyPortfolioSectionUS
	if m == market.TW {
		sectionTitle = i18n.KeyPortfolioSectionTW
	}
	sb.WriteString(i18n.T(ts.lang, sectionTitle))

	var totalValue float64
	for _, p := range marketPositions {
		q, err := ts.provider.GetQuote(p.Ticker)
		if err != nil {
			sb.WriteString(i18n.T(ts.lang, i18n.KeyQuoteUnavailable, p.Ticker))
			continue
		}
		marketValue := p.Shares * q.Price
		unrealized := (q.Price - p.AvgCost) * p.Shares
		unrealizedPct := (q.Price - p.AvgCost) / p.AvgCost * 100
		totalValue += marketValue
		sb.WriteString(i18n.T(ts.lang, i18n.KeyPortfolioLine, p.Ticker, p.Shares, p.AvgCost, q.Price, marketValue, unrealized, unrealizedPct))
	}
	if m == market.TW {
		sb.WriteString(i18n.T(ts.lang, i18n.KeyPortfolioSummaryTWD, totalValue, realizedTotal))
	} else {
		sb.WriteString(i18n.T(ts.lang, i18n.KeyPortfolioSummary, totalValue, realizedTotal))
	}
	sb.WriteString("\n\n")
}

// getRecommendationStats mirrors internal/bot's handleTrack (core logic
// only) — same relative-to-SPY hit rule, same per-source breakdown. The
// scoring helpers below (trackHit/trackRow/trackSourceStats/summarizeTrack/
// accumulateTrackRow/displaySource/sortedSourceKeys) are duplicated from
// bot.go rather than imported, for the same package-boundary reason as
// formatFundamentals — they're unexported there and this package can't
// import internal/bot anyway. days is clamped rather than rejected on an
// out-of-range value (unlike /track's usage-error reply) since a tool
// caller has no natural way to "retype the command" — the closest sane
// value is more useful than a hard failure.
func (ts *toolset) getRecommendationStats(ctx context.Context, _ *mcp.CallToolRequest, in recommendationStatsInput) (*mcp.CallToolResult, any, error) {
	days := in.Days
	if days <= 0 {
		days = trackDefaultDays
	}
	if days > trackMaxDays {
		days = trackMaxDays
	}

	key := fmt.Sprintf("get_recommendation_stats:%d", days)
	result, err := ts.withCache(ctx, key, shortCacheTTL, func() (*mcp.CallToolResult, error) {
		fromDate := time.Now().In(cst).AddDate(0, 0, -days).Format("2006-01-02")
		recs, err := ts.db.GetRecommendationsSince(fromDate)
		if err != nil {
			return nil, ts.mcpErr(i18n.KeyQueryFailed, err)
		}
		if len(recs) == 0 {
			return nil, ts.mcpErr(i18n.KeyTrackEmpty, days)
		}

		quotes := make(map[string]*data.Quote)
		spyQuote, err := ts.provider.GetQuote(benchmarkTicker)
		if err != nil {
			log.Printf("mcptools: get_recommendation_stats benchmark %s quote: %v", benchmarkTicker, err)
			spyQuote = nil
		}

		var sb strings.Builder
		sb.WriteString(i18n.T(ts.lang, i18n.KeyTrackTitle, days))
		var rows []trackRow
		for _, r := range recs {
			action := r.Action
			if action == "" {
				action = "—"
			}

			base := r.Price
			if base == 0 {
				if c, ok, err := ts.db.GetSnapshotClose(r.Ticker, r.Date); err == nil && ok {
					base = c
				}
			}
			if base == 0 {
				sb.WriteString(i18n.T(ts.lang, i18n.KeyTrackLineNoPrice, r.Date, r.Ticker, action))
				continue
			}

			q, seen := quotes[r.Ticker]
			if !seen {
				var err error
				q, err = ts.provider.GetQuote(r.Ticker)
				if err != nil {
					log.Printf("mcptools: get_recommendation_stats quote %s: %v", r.Ticker, err)
					q = nil
				}
				quotes[r.Ticker] = q
			}
			if q == nil {
				sb.WriteString(i18n.T(ts.lang, i18n.KeyQuoteUnavailable, r.Ticker))
				continue
			}

			changePct := (q.Price - base) / base * 100

			var spyChangePct float64
			haveSPY := false
			if spyQuote != nil {
				if spyBase, ok, err := ts.db.GetSnapshotClose(benchmarkTicker, r.Date); err == nil && ok && spyBase != 0 {
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
				sb.WriteString(i18n.T(ts.lang, i18n.KeyTrackLineVsSPY, r.Date, r.Ticker, action, base, q.Price, changePct, spyChangePct, verdict))
			} else {
				sb.WriteString(i18n.T(ts.lang, i18n.KeyTrackLine, r.Date, r.Ticker, action, base, q.Price, changePct, verdict))
			}
		}

		overall, bySource := summarizeTrack(rows)
		if overall.Evaluated > 0 {
			sb.WriteString(i18n.T(ts.lang, i18n.KeyTrackSummary, overall.Hits, overall.Evaluated, overall.HitRate()))
			sb.WriteString(i18n.T(ts.lang, i18n.KeyTrackAvgReturnLine, overall.AvgBuyPct(), overall.BuyCount, overall.AvgSellPct(), overall.SellCount))

			if len(bySource) > 1 {
				sb.WriteString(i18n.T(ts.lang, i18n.KeyTrackBySourceHeader))
				for _, source := range sortedSourceKeys(bySource) {
					s := bySource[source]
					sb.WriteString(i18n.T(ts.lang, i18n.KeyTrackBySourceLine, source, s.Hits, s.Evaluated, s.HitRate()))
				}
			}
		}
		return textResult(sb.String()), nil
	})
	return result, nil, err
}

type recentRecommendationsInput struct {
	Days   int    `json:"days,omitempty" jsonschema:"look-back window in days (default 7, max 90)"`
	Ticker string `json:"ticker,omitempty" jsonschema:"optional ticker to filter by, e.g. AAPL"`
}

// getRecentRecommendations lists the raw recommendations rows —
// get_recommendation_stats' unaggregated counterpart, and the chat-side
// answer to "what did you recommend and why" (the /recommend path already
// sees this history via llm.StockData.PrevRec; the chat session otherwise
// has no view of it at all). days is clamped, not rejected, for the same
// no-way-to-retype reason as getRecommendationStats. Output is rendered
// newest first and capped at recentRecsMaxRows (reasons are full LLM
// paragraphs — see the const's doc comment).
func (ts *toolset) getRecentRecommendations(ctx context.Context, _ *mcp.CallToolRequest, in recentRecommendationsInput) (*mcp.CallToolResult, any, error) {
	days := in.Days
	if days <= 0 {
		days = trackDefaultDays
	}
	if days > trackMaxDays {
		days = trackMaxDays
	}
	ticker := strings.ToUpper(strings.TrimSpace(in.Ticker))

	key := fmt.Sprintf("get_recent_recommendations:%d:%s", days, ticker)
	result, err := ts.withCache(ctx, key, shortCacheTTL, func() (*mcp.CallToolResult, error) {
		fromDate := time.Now().In(cst).AddDate(0, 0, -days).Format("2006-01-02")
		recs, err := ts.db.GetRecommendationsSince(fromDate)
		if err != nil {
			return nil, ts.mcpErr(i18n.KeyQueryFailed, err)
		}
		if ticker != "" {
			filtered := recs[:0]
			for _, r := range recs {
				if r.Ticker == ticker {
					filtered = append(filtered, r)
				}
			}
			recs = filtered
		}
		if len(recs) == 0 {
			if ticker != "" {
				return nil, ts.mcpErr(i18n.KeyMCPRecentRecsEmptyTicker, ticker, days)
			}
			return nil, ts.mcpErr(i18n.KeyTrackEmpty, days)
		}

		total := len(recs)
		if total > recentRecsMaxRows {
			recs = recs[total-recentRecsMaxRows:] // GetRecommendationsSince is oldest-first; keep the newest
		}

		var sb strings.Builder
		if ticker != "" {
			sb.WriteString(i18n.T(ts.lang, i18n.KeyMCPRecentRecsTitleTicker, ticker, days))
		} else {
			sb.WriteString(i18n.T(ts.lang, i18n.KeyMCPRecentRecsTitle, days))
		}
		if len(recs) < total {
			sb.WriteString(i18n.T(ts.lang, i18n.KeyMCPRecentRecsTruncated, len(recs), total))
		}
		for i := len(recs) - 1; i >= 0; i-- {
			r := recs[i]
			action := r.Action
			if action == "" {
				action = "—"
			}
			if r.Price != 0 {
				sb.WriteString(i18n.T(ts.lang, i18n.KeyMCPRecentRecLine, r.Date, r.Ticker, action, r.Price, displaySource(r.Source), r.Reason))
			} else {
				sb.WriteString(i18n.T(ts.lang, i18n.KeyMCPRecentRecLineNoPrice, r.Date, r.Ticker, action, displaySource(r.Source), r.Reason))
			}
		}
		return textResult(sb.String()), nil
	})
	return result, nil, err
}

func (ts *toolset) getUniverseSummary(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, any, error) {
	result, err := ts.withCache(ctx, "get_universe_summary", longCacheTTL, func() (*mcp.CallToolResult, error) {
		entries, err := ts.db.GetUniverse()
		if err != nil {
			return nil, ts.mcpErr(i18n.KeyQueryFailed, err)
		}

		bySource := make(map[string]int)
		for _, e := range entries {
			bySource[e.Source]++
		}

		var sb strings.Builder
		sb.WriteString(i18n.T(ts.lang, i18n.KeyUniverseSummary, len(entries)))
		for _, source := range []string{"sp500", "manual"} {
			if n := bySource[source]; n > 0 {
				sb.WriteString(i18n.T(ts.lang, i18n.KeyUniverseSourceLine, source, n))
			}
		}
		return textResult(sb.String()), nil
	})
	return result, nil, err
}

// --- /track scoring, duplicated from internal/bot/bot.go (unexported
// there, and this package can't import internal/bot — see this file's and
// tools.go's package-boundary doc comments) ---

// trackHit implements /track's relative-to-SPY hit rule: when a same-period
// SPY change is available, BUY only counts as a hit if the ticker beat it
// and SELL only if it underperformed it; otherwise it falls back to the
// pre-Phase-3.8 absolute-direction rule (BUY counts if price rose, SELL if
// it fell).
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

// trackRow is one BUY/SELL recommendation reduced to what the summary
// needs.
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
// "watchlist".
func displaySource(source string) string {
	if source == "" {
		return "watchlist"
	}
	return source
}

// sortedSourceKeys returns bySource's keys in alphabetical order for
// deterministic output instead of Go's randomized map iteration.
func sortedSourceKeys(bySource map[string]trackSourceStats) []string {
	keys := make([]string, 0, len(bySource))
	for k := range bySource {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
