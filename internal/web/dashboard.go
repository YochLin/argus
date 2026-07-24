package web

import (
	"log"
	"time"

	"argus/internal/db"
	"argus/internal/market"
)

// spyTicker/twBenchmarkTicker mirror internal/bot's own benchmarkTicker/
// benchmarkFor — SPY/0050 are deliberately never added to the watchlist
// table (they're not holdings), so daily_snapshots is the only place either
// is recorded; internal/web can't import internal/bot's unexported
// constant/function, so this is duplicated the same way formatFundamentals/
// commaf are elsewhere in the project rather than restructuring package
// boundaries for one string literal.
const (
	spyTicker         = "SPY"
	twBenchmarkTicker = "0050"
)

// benchmarkFor returns m's daily-snapshot benchmark ticker.
func benchmarkFor(m market.MarketID) string {
	if m == market.TW {
		return twBenchmarkTicker
	}
	return spyTicker
}

// filterByMarket returns the subset of tickers belonging to market m.
func filterByMarket(tickers []string, m market.MarketID) []string {
	out := make([]string, 0, len(tickers))
	for _, t := range tickers {
		if market.Of(t) == m {
			out = append(out, t)
		}
	}
	return out
}

// filterTransactionsByMarket returns the subset of txs belonging to market
// m, preserving order.
func filterTransactionsByMarket(txs []db.Transaction, m market.MarketID) []db.Transaction {
	out := make([]db.Transaction, 0, len(txs))
	for _, t := range txs {
		if market.Of(t.Ticker) == m {
			out = append(out, t)
		}
	}
	return out
}

// filterPositionsByMarket returns the subset of positions belonging to
// market m, preserving order.
func filterPositionsByMarket(positions []db.Position, m market.MarketID) []db.Position {
	out := make([]db.Position, 0, len(positions))
	for _, p := range positions {
		if market.Of(p.Ticker) == m {
			out = append(out, p)
		}
	}
	return out
}

// dbReader is the subset of *db.DB the dashboard needs — narrow enough to
// fake in tests without a real SQLite file.
type dbReader interface {
	GetPositions() ([]db.Position, error)
	GetAllTransactions() ([]db.Transaction, error)
	GetDailySnapshotsForTickers(tickers []string, from, to string) ([]db.DailySnapshot, error)
	GetWatchlist() ([]string, error)
	GetLatestSnapshot(ticker string) (db.DailySnapshot, bool, error)
	GetNetWorthOnOrBefore(date string, m market.MarketID) (float64, bool, error)
	GetNetWorthRange(from, to string, m market.MarketID) ([]db.NetWorthPoint, error)
}

// netWorthBaseline resolves the capital base for a period starting at
// periodStart: the net worth as of the day before periodStart, or — if the
// account's history doesn't reach that far back — the earliest net worth
// snapshot on record within the period itself. That fallback mirrors
// internal/bot/jobs.go's buildMonthlyReportBlock (same "baseline, haveBaseline
// = values[0], true" idiom), not a new convention. A query failure logs and
// is treated the same as "no data," matching every other single-query
// degrade in this file.
func netWorthBaseline(database dbReader, periodStart string, m market.MarketID, today string) (float64, bool) {
	dayBefore, err := time.Parse("2006-01-02", periodStart)
	if err != nil {
		log.Printf("web: dashboard: parse period start %q: %v", periodStart, err)
		return 0, false
	}
	dayBeforeStr := dayBefore.AddDate(0, 0, -1).Format("2006-01-02")

	if baseline, ok, err := database.GetNetWorthOnOrBefore(dayBeforeStr, m); err != nil {
		log.Printf("web: dashboard: get net worth on or before %s: %v", dayBeforeStr, err)
	} else if ok {
		return baseline, true
	}

	points, err := database.GetNetWorthRange(periodStart, today, m)
	if err != nil {
		log.Printf("web: dashboard: get net worth range: %v", err)
		return 0, false
	}
	if len(points) == 0 {
		return 0, false
	}
	return points[0].Total, true
}

// buildStatus assembles the /api/status response — the shell-level status
// bar shown on every page (Phase 5 sidebar layout, see
// docs/phase-5-sidebar-layout.md), not just the dashboard. Like
// buildDashboard, a single failed query never fails the whole response: it
// logs and leaves that field at its zero value. m selects which market's
// watchlist count/benchmark to show (Phase 6, see
// docs/phase-6-tw-market.md §4.4).
func buildStatus(database dbReader, m market.MarketID) statusResponse {
	var status statusResponse
	watchlist, err := database.GetWatchlist()
	if err != nil {
		log.Printf("web: status: get watchlist: %v", err)
	} else {
		status.WatchingCount = len(filterByMarket(watchlist, m))
	}
	if bench, ok, err := database.GetLatestSnapshot(benchmarkFor(m)); err != nil {
		log.Printf("web: status: get benchmark snapshot: %v", err)
	} else if ok {
		status.SPYChangePct = bench.ChangePercent
		status.LastCloseDate = bench.Date
	}
	return status
}

// buildDashboard assembles the /api/dashboard response: KPIs and the
// cumulative P&L curve (both from the DailyPnL replay engine in pnl.go)
// and the live-quote-enriched positions list, restricted to market m
// (Phase 6, see docs/phase-6-tw-market.md §4.4 — TWD and USD transactions
// must never be replayed together, or the curve/KPIs come out as
// meaningless mixed-currency numbers). The replay engine itself
// (DailyPnL/KPIs in pnl.go) is unchanged — it's a pure function over
// whatever transactions/snapshots it's given, so filtering happens here,
// before that call, not inside it. Nothing here aborts the whole response
// on a partial failure — a single bad quote just leaves that position's
// price fields at 0 (logged), same "attach what's available" degrade
// convention internal/bot's fetchStockData uses for optional prompt fields.
func buildDashboard(database dbReader, quotes quoteGetter, m market.MarketID) (dashboardResponse, error) {
	allPositions, err := database.GetPositions()
	if err != nil {
		return dashboardResponse{}, err
	}
	positions := filterPositionsByMarket(allPositions, m)

	allTxs, err := database.GetAllTransactions()
	if err != nil {
		return dashboardResponse{}, err
	}
	txs := filterTransactionsByMarket(allTxs, m)

	sells := FilterSells(txs)
	resp := dashboardResponse{
		Curve: []DateValue{}, // never nil — JSON "[]", not "null", when there's no history yet
		KPIs: kpisResponse{
			WinRate:      WinRate(sells),
			ProfitFactor: ProfitFactor(sells),
			Expectancy:   Expectancy(sells),
		},
	}

	if len(txs) > 0 {
		tickerSet := make(map[string]bool, len(txs))
		for _, t := range txs {
			tickerSet[t.Ticker] = true
		}
		tickers := make([]string, 0, len(tickerSet))
		for t := range tickerSet {
			tickers = append(tickers, t)
		}
		from := txs[0].Date // GetAllTransactions is date-ordered; the first row is the earliest.
		now := time.Now()
		to := now.Format("2006-01-02")

		snapshots, err := database.GetDailySnapshotsForTickers(tickers, from, to)
		if err != nil {
			return dashboardResponse{}, err
		}

		daily := DailyPnL(txs, snapshots)
		curve := CumulativeCurve(daily)
		if curve != nil {
			resp.Curve = curve
		}
		if len(curve) > 0 {
			resp.KPIs.NetPnL = curve[len(curve)-1].Value
		}
		resp.KPIs.MaxDrawdown = MaxDrawdownAbs(curve)

		ytdStart := YTDStart(now)
		if base, ok := netWorthBaseline(database, ytdStart, m, to); ok {
			if pct, ok := PeriodReturnPct(curve, ytdStart, base, ok); ok {
				resp.KPIs.YTDReturnPct = &pct
			}
		}
		qtdStart := QTDStart(now)
		if base, ok := netWorthBaseline(database, qtdStart, m, to); ok {
			if pct, ok := PeriodReturnPct(curve, qtdStart, base, ok); ok {
				resp.KPIs.QTDReturnPct = &pct
			}
		}
		htdStart := HTDStart(now)
		if base, ok := netWorthBaseline(database, htdStart, m, to); ok {
			if pct, ok := PeriodReturnPct(curve, htdStart, base, ok); ok {
				resp.KPIs.HTDReturnPct = &pct
			}
		}
	}

	resp.Positions = make([]positionResponse, 0, len(positions))
	for _, p := range positions {
		pr := positionResponse{Ticker: p.Ticker, Shares: p.Shares, AvgCost: p.AvgCost}
		q, err := quotes.GetQuote(p.Ticker)
		if err != nil {
			log.Printf("web: dashboard: get quote for %s: %v", p.Ticker, err)
			resp.Positions = append(resp.Positions, pr)
			continue
		}
		pr.Price = q.Price
		pr.MarketValue = q.Price * p.Shares
		pr.UnrealizedPnL = (q.Price - p.AvgCost) * p.Shares
		if p.AvgCost != 0 {
			pr.UnrealizedPnLPct = (q.Price - p.AvgCost) / p.AvgCost * 100
		}
		resp.Positions = append(resp.Positions, pr)
	}

	return resp, nil
}
