package web

import (
	"net/http"
	"time"

	"argus/internal/db"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/service"
)

// dashboardResponse is /api/dashboard's body. Only raw numbers/dates/
// tickers — no display strings — per docs/phase-5-web-dashboard.md's UI
// language decision: the frontend picks zh/en display text itself from
// /api/config's lang, so internal/i18n never needs touching for this API.
type dashboardResponse struct {
	KPIs  kpisResponse `json:"kpis"`
	Curve []DateValue  `json:"curve"`
	// Drawdown is Curve's underwater series (pnl.go's DrawdownSeries, Phase 8
	// PR2) — always <= 0, same length/date axis as Curve, "[]" not "null"
	// when there's no history yet.
	Drawdown []DateValue `json:"drawdown"`
	// Benchmark is the same-cash-flow SPY/0050 replay (benchmark.go's
	// BenchmarkReplay, Phase 8 PR2) in the same cumulative-P&L-amount units
	// as Curve, directly overlayable on the same chart/axis. Its date axis
	// starts wherever daily_snapshots' benchmark history begins, which may
	// be later than Curve's own start — see docs/phase-8-trader-analytics.md
	// §7's accepted gap. "[]" not "null" when there's no benchmark history
	// in range yet.
	Benchmark []DateValue        `json:"benchmark"`
	Positions []positionResponse `json:"positions"`
}

type kpisResponse struct {
	NetPnL       float64 `json:"netPnL"`
	WinRate      float64 `json:"winRate"`
	ProfitFactor float64 `json:"profitFactor"`
	Expectancy   float64 `json:"expectancy"`
	MaxDrawdown  float64 `json:"maxDrawdown"`
	// YTDReturnPct/QTDReturnPct/HTDReturnPct are *float64 (not omitempty) so
	// "no usable baseline yet" (see pnl.go's PeriodReturnPct) serializes as
	// an explicit JSON null, never an omitted key or a misleading 0 —
	// docs/phase-5-web-dashboard.md's "don't pad missing data with zero"
	// principle.
	YTDReturnPct *float64 `json:"ytdReturnPct"`
	QTDReturnPct *float64 `json:"qtdReturnPct"`
	HTDReturnPct *float64 `json:"htdReturnPct"`
	// BenchmarkAlpha is Curve's terminal value minus Benchmark's terminal
	// value (Phase 8 PR2) — nil when there's no benchmark curve in range yet
	// (same "explicit null over a misleading zero" convention as the
	// period-return fields above).
	BenchmarkAlpha *float64 `json:"benchmarkAlpha"`
}

type positionResponse struct {
	Ticker           string  `json:"ticker"`
	Shares           float64 `json:"shares"`
	AvgCost          float64 `json:"avgCost"`
	Price            float64 `json:"price"`
	MarketValue      float64 `json:"marketValue"`
	UnrealizedPnL    float64 `json:"unrealizedPnL"`
	UnrealizedPnLPct float64 `json:"unrealizedPnLPct"`
	// StopPrice (Phase 10, docs/phase-10-web-trade-input.md §4.3) is
	// db.Position.StopPrice passed straight through — 0 means unset, same
	// sentinel convention as the DB column itself; the frontend renders that
	// as "—" rather than a misleading $0.00.
	StopPrice float64 `json:"stopPrice"`
}

type statusResponse struct {
	WatchingCount int     `json:"watchingCount"`
	SPYChangePct  float64 `json:"spyChangePct"`
	LastCloseDate string  `json:"lastCloseDate"`
	// AccountValue/NetPnL/WinRate/TradeCount back the sidebar's
	// account-overview card (see buildStatus's doc comment for each field's
	// exact definition).
	AccountValue float64 `json:"accountValue"`
	NetPnL       float64 `json:"netPnL"`
	WinRate      float64 `json:"winRate"`
	TradeCount   int     `json:"tradeCount"`
	// Writable (Phase 10, docs/phase-10-web-trade-input.md §4.1) mirrors
	// whether WEB_PASSWORD is configured — the frontend uses this to decide
	// whether to render any trade-input UI at all, not just disable it.
	Writable bool `json:"writable"`
}

type transactionResponse struct {
	ID          int64   `json:"id"`
	Date        string  `json:"date"`
	Ticker      string  `json:"ticker"`
	Side        string  `json:"side"`
	Shares      float64 `json:"shares"`
	Price       float64 `json:"price"`
	Fee         float64 `json:"fee"`
	RealizedPnL float64 `json:"realizedPnL"`
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
		logger.Errorf("web: dashboard: parse period start %q: %v", periodStart, err)
		return 0, false
	}
	dayBeforeStr := dayBefore.AddDate(0, 0, -1).Format("2006-01-02")

	if baseline, ok, err := database.GetNetWorthOnOrBefore(dayBeforeStr, m); err != nil {
		logger.Errorf("web: dashboard: get net worth on or before %s: %v", dayBeforeStr, err)
	} else if ok {
		return baseline, true
	}

	points, err := database.GetNetWorthRange(periodStart, today, m)
	if err != nil {
		logger.Errorf("web: dashboard: get net worth range: %v", err)
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
//
// AccountValue/NetPnL/WinRate/TradeCount (added for the sidebar's account-
// overview card, mirroring Figma reference layout) reuse the exact same
// definitions buildDashboard/buildRisk already use elsewhere — live position
// value + declared cash for AccountValue (buildRisk's accountValue), the
// all-time realized-SELL total for NetPnL (db.GetRealizedPnL, same figure
// internal/bot's /portfolio shows), and pnl.go's FilterSells/WinRate for the
// win-rate/trade-count pair — no new "what counts as account value" logic
// gets invented here.
func buildStatus(database dbReader, quotes quoteGetter, m market.MarketID) statusResponse {
	return buildStatusWithPortfolio(database, service.NewPortfolioService(database, quotes), m)
}

func buildStatusWithPortfolio(database dbReader, portfolio *service.PortfolioService, m market.MarketID) statusResponse {
	var status statusResponse
	watchlist, err := database.GetWatchlist()
	if err != nil {
		logger.Errorf("web: status: get watchlist: %v", err)
	} else {
		status.WatchingCount = len(filterByMarket(watchlist, m))
	}
	if bench, ok, err := database.GetLatestSnapshot(service.BenchmarkFor(m)); err != nil {
		logger.Errorf("web: status: get benchmark snapshot: %v", err)
	} else if ok {
		status.SPYChangePct = bench.ChangePercent
		status.LastCloseDate = bench.Date
	}

	snapshot, err := portfolio.Snapshot(m)
	if err != nil {
		logger.Errorf("web: status: get positions: %v", err)
	} else {
		if snapshot.CashErr != nil {
			logger.Errorf("web: status: load cash for %s: %v", m, snapshot.CashErr)
		}
		status.AccountValue = snapshot.AccountValue
	}

	if netPnL, err := database.GetRealizedPnL(m); err != nil {
		logger.Errorf("web: status: get realized pnl: %v", err)
	} else {
		status.NetPnL = netPnL
	}

	if txs, err := database.GetAllTransactions(); err != nil {
		logger.Errorf("web: status: get transactions: %v", err)
	} else {
		sells := FilterSells(filterTransactionsByMarket(txs, m))
		status.TradeCount = len(sells)
		status.WinRate = WinRate(sells)
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
	return buildDashboardWithPortfolio(database, service.NewPortfolioService(database, quotes), m)
}

func buildDashboardWithPortfolio(database dbReader, portfolio *service.PortfolioService, m market.MarketID) (dashboardResponse, error) {
	snapshot, err := portfolio.Snapshot(m)
	if err != nil {
		return dashboardResponse{}, err
	}

	allTxs, err := database.GetAllTransactions()
	if err != nil {
		return dashboardResponse{}, err
	}
	txs := filterTransactionsByMarket(allTxs, m)

	sells := FilterSells(txs)
	resp := dashboardResponse{
		Curve:     []DateValue{}, // never nil — JSON "[]", not "null", when there's no history yet
		Drawdown:  []DateValue{},
		Benchmark: []DateValue{},
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

		// benchTicker rides along in the same batched snapshot query rather
		// than a second round-trip — daily_snapshots already carries it
		// (snapshotBenchmark writes it every closing-snapshot run) and
		// DailyPnL safely ignores a ticker with no matching transactions
		// (openingShares is 0 for it, so its snapshot rows never contribute
		// P&L), so including it here costs nothing extra.
		benchTicker := service.BenchmarkFor(m)
		snapshotTickers := tickers
		if !tickerSet[benchTicker] {
			snapshotTickers = append(snapshotTickers, benchTicker)
		}
		snapshots, err := database.GetDailySnapshotsForTickers(snapshotTickers, from, to)
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
		if dd := DrawdownSeries(curve); dd != nil {
			resp.Drawdown = dd
		}

		benchCloses := make(map[string]float64)
		for _, s := range snapshots {
			if s.Ticker == benchTicker {
				benchCloses[s.Date] = s.Close
			}
		}
		benchCurve := BenchmarkReplay(txs, benchCloses)
		if benchCurve != nil {
			resp.Benchmark = benchCurve
			if len(curve) > 0 {
				alpha := curve[len(curve)-1].Value - benchCurve[len(benchCurve)-1].Value
				resp.KPIs.BenchmarkAlpha = &alpha
			}
		}

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

	resp.Positions = make([]positionResponse, 0, len(snapshot.Positions))
	for _, valuation := range snapshot.Positions {
		p := valuation.Position
		pr := positionResponse{Ticker: p.Ticker, Shares: p.Shares, AvgCost: p.AvgCost, StopPrice: p.StopPrice}
		if valuation.QuoteErr != nil {
			logger.Errorf("web: dashboard: get quote for %s: %v", p.Ticker, valuation.QuoteErr)
		}
		q := valuation.Quote
		if q == nil {
			resp.Positions = append(resp.Positions, pr)
			continue
		}
		pr.Price = q.Price
		pr.MarketValue = valuation.MarketValue
		pr.UnrealizedPnL = valuation.UnrealizedPnL
		pr.UnrealizedPnLPct = valuation.UnrealizedPnLPct
		resp.Positions = append(resp.Positions, pr)
	}

	return resp, nil
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	resp := buildStatusWithPortfolio(s.db, s.portfolios(), marketParam(r))
	resp.Writable = s.password != ""
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	resp, err := buildDashboardWithPortfolio(s.db, s.portfolios(), marketParam(r))
	if err != nil {
		logger.Errorf("web: build dashboard: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to build dashboard")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
