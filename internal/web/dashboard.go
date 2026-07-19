package web

import (
	"log"
	"time"

	"argus/internal/db"
)

// spyTicker mirrors internal/bot's own benchmarkTicker constant — SPY is
// deliberately never added to the watchlist table (it's not a holding),
// so daily_snapshots is the only place it's recorded; internal/web can't
// import internal/bot's unexported constant, so this is duplicated the
// same way formatFundamentals/commaf are elsewhere in the project rather
// than restructuring package boundaries for one string literal.
const spyTicker = "SPY"

// dbReader is the subset of *db.DB the dashboard needs — narrow enough to
// fake in tests without a real SQLite file.
type dbReader interface {
	GetPositions() ([]db.Position, error)
	GetAllTransactions() ([]db.Transaction, error)
	GetDailySnapshotsForTickers(tickers []string, from, to string) ([]db.DailySnapshot, error)
	GetWatchlist() ([]string, error)
	GetLatestSnapshot(ticker string) (db.DailySnapshot, bool, error)
}

// buildDashboard assembles the /api/dashboard response: KPIs and the
// cumulative P&L curve (both from the DailyPnL replay engine in pnl.go),
// the live-quote-enriched positions list, and the status-bar fields.
// Nothing here aborts the whole response on a partial failure — a single
// bad quote just leaves that position's price fields at 0 (logged), same
// "attach what's available" degrade convention internal/bot's
// fetchStockData uses for optional prompt fields.
func buildDashboard(database dbReader, quotes quoteGetter) (dashboardResponse, error) {
	positions, err := database.GetPositions()
	if err != nil {
		return dashboardResponse{}, err
	}
	txs, err := database.GetAllTransactions()
	if err != nil {
		return dashboardResponse{}, err
	}

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
		to := time.Now().Format("2006-01-02")

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

	watchlist, err := database.GetWatchlist()
	if err != nil {
		log.Printf("web: dashboard: get watchlist: %v", err)
	} else {
		resp.Status.WatchingCount = len(watchlist)
	}
	if spy, ok, err := database.GetLatestSnapshot(spyTicker); err != nil {
		log.Printf("web: dashboard: get SPY snapshot: %v", err)
	} else if ok {
		resp.Status.SPYChangePct = spy.ChangePercent
		resp.Status.LastCloseDate = spy.Date
	}

	return resp, nil
}
