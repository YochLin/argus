package web

import (
	"sort"
	"time"

	"argus/internal/db"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/service"
)

// paperResponse is /api/paper's body — one market's live paper account
// (Phase 11 PR3/PR4, docs/phase-11-paper-account.md §7): KPIs, open
// positions, closed rounds, and three overlayable curves. No new
// computation engine — every number here comes from pnl.go/benchmark.go's
// existing pure functions (the same ones buildDashboard already uses) fed
// paper.db's data instead of the real db, or from segmentRounds (rounds.go,
// Phase 8 PR4), which already does exactly the BUY/SELL round-pairing this
// endpoint's closed-position list needs.
type paperResponse struct {
	KPIs      paperKPIs               `json:"kpis"`
	Positions []paperPositionResponse `json:"positions"`
	Closed    []paperClosedResponse   `json:"closed"`
	Curve     []DateValue             `json:"curve"`
	Benchmark []DateValue             `json:"benchmark"`
	Drawdown  []DateValue             `json:"drawdown"`
}

type paperKPIs struct {
	Cash               float64 `json:"cash"`
	PositionsValue     float64 `json:"positionsValue"`
	Equity             float64 `json:"equity"`
	InitialCash        float64 `json:"initialCash"`
	SinceDate          string  `json:"sinceDate"`
	RealizedPnL        float64 `json:"realizedPnL"`
	UnrealizedPnL      float64 `json:"unrealizedPnL"`
	TotalReturnPct     float64 `json:"totalReturnPct"`
	BenchmarkReturnPct float64 `json:"benchmarkReturnPct"`
	AlphaPct           float64 `json:"alphaPct"`
	MaxDrawdownPct     float64 `json:"maxDrawdownPct"`
	WinRate            float64 `json:"winRate"`
	ProfitFactor       float64 `json:"profitFactor"`
	Expectancy         float64 `json:"expectancy"`
}

type paperPositionResponse struct {
	Ticker        string  `json:"ticker"`
	EntryDate     string  `json:"entryDate"`
	AvgCost       float64 `json:"avgCost"`
	Shares        float64 `json:"shares"`
	StopPrice     float64 `json:"stopPrice"`
	Price         float64 `json:"price"`
	UnrealizedPnL float64 `json:"unrealizedPnL"`
	UnrealizedPct float64 `json:"unrealizedPct"`
	DistToStopPct float64 `json:"distToStopPct"`
}

type paperClosedResponse struct {
	Ticker      string  `json:"ticker"`
	EntryDate   string  `json:"entryDate"`
	ExitDate    string  `json:"exitDate"`
	AvgCost     float64 `json:"avgCost"`
	ExitPrice   float64 `json:"exitPrice"`
	Shares      float64 `json:"shares"`
	RealizedPnL float64 `json:"realizedPnL"`
	RealizedPct float64 `json:"realizedPct"`
	ExitReason  string  `json:"exitReason"` // "stop" | "llm_sell" — see closedPositionFromRound
}

// paperInitialCashFor mirrors internal/bot's Bot.paperInitialCashFor — not
// yet extracted into internal/service, unlike service.CashSettingKey/
// service.BenchmarkFor elsewhere in this package.
func paperInitialCashFor(m market.MarketID, usd, twd float64) float64 {
	if m == market.TW {
		return twd
	}
	return usd
}

// buildPaper assembles /api/paper's response for market m against paperDB —
// a completely separate *db.DB from the real dashboard's, so nothing here
// ever touches real trading data. initialCashUSD/TWD are PAPER_INITIAL_CASH_
// USD/TWD (the same env-driven values internal/bot/paper.go seeds the
// account with), needed here only to render KPIs.InitialCash/TotalReturnPct
// — paperDB's cash setting already reflects the live, auto-adjusted balance
// (Phase 11 PR3 §6.1), this is just the fixed denominator for "return since
// inception."
func buildPaper(paperDB dbReader, quotes quoteGetter, m market.MarketID, initialCashUSD, initialCashTWD float64) (paperResponse, error) {
	initialCash := paperInitialCashFor(m, initialCashUSD, initialCashTWD)

	allPositions, err := paperDB.GetPositions()
	if err != nil {
		return paperResponse{}, err
	}
	positions := filterPositionsByMarket(allPositions, m)

	allTxs, err := paperDB.GetAllTransactions()
	if err != nil {
		return paperResponse{}, err
	}
	txs := filterTransactionsByMarket(allTxs, m)

	cash, err := loadCash(paperDB, m)
	if err != nil {
		logger.Errorf("web: paper: load cash for %s: %v", m, err)
	}
	realizedTotal, err := paperDB.GetRealizedPnL(m)
	if err != nil {
		logger.Errorf("web: paper: realized pnl for %s: %v", m, err)
	}

	byTicker := make(map[string][]db.Transaction, len(positions))
	for _, t := range txs {
		byTicker[t.Ticker] = append(byTicker[t.Ticker], t)
	}

	resp := paperResponse{
		Positions: []paperPositionResponse{},
		Closed:    []paperClosedResponse{},
		Curve:     []DateValue{},
		Benchmark: []DateValue{},
		Drawdown:  []DateValue{},
		KPIs: paperKPIs{
			Cash:        cash,
			InitialCash: initialCash,
			RealizedPnL: realizedTotal,
		},
	}
	if len(txs) > 0 {
		resp.KPIs.SinceDate = txs[0].Date // date-ordered, see GetAllTransactions
	}

	sells := FilterSells(txs)
	resp.KPIs.WinRate = WinRate(sells)
	resp.KPIs.ProfitFactor = ProfitFactor(sells)
	resp.KPIs.Expectancy = Expectancy(sells)

	// entryDateByTicker resolves each still-open position's own entry date
	// (segmentRounds' trailing open round, EndDate "") rather than
	// GetEarliestBuyDate's first-ever-BUY, which would be wrong for a
	// ticker paper-traded, closed, and re-entered.
	entryDateByTicker := make(map[string]string, len(positions))
	for ticker, tickerTxs := range byTicker {
		for _, r := range segmentRounds(tickerTxs) {
			if r.EndDate == "" {
				entryDateByTicker[ticker] = r.StartDate
			} else {
				resp.Closed = append(resp.Closed, closedPositionFromRound(ticker, r))
			}
		}
	}
	sort.Slice(resp.Closed, func(i, j int) bool { return resp.Closed[i].ExitDate > resp.Closed[j].ExitDate })

	posTickers := make([]string, len(positions))
	for i, p := range positions {
		posTickers[i] = p.Ticker
	}
	quoteMap := fetchQuotes(quotes, posTickers, "paper")

	for _, p := range positions {
		pr := paperPositionResponse{
			Ticker: p.Ticker, EntryDate: entryDateByTicker[p.Ticker],
			AvgCost: p.AvgCost, Shares: p.Shares, StopPrice: p.StopPrice,
		}
		if q, ok := quoteMap[p.Ticker]; ok {
			pr.Price = q.Price
			pr.UnrealizedPnL = (q.Price - p.AvgCost) * p.Shares
			if p.AvgCost != 0 {
				pr.UnrealizedPct = (q.Price - p.AvgCost) / p.AvgCost * 100
			}
			if q.Price != 0 {
				pr.DistToStopPct = (q.Price - p.StopPrice) / q.Price * 100
			}
			resp.KPIs.PositionsValue += q.Price * p.Shares
			resp.KPIs.UnrealizedPnL += pr.UnrealizedPnL
		}
		resp.Positions = append(resp.Positions, pr)
	}
	resp.KPIs.Equity = cash + resp.KPIs.PositionsValue
	if initialCash > 0 {
		resp.KPIs.TotalReturnPct = (resp.KPIs.Equity - initialCash) / initialCash * 100
	}

	if len(txs) > 0 {
		tickerSet := make(map[string]bool, len(txs))
		for _, t := range txs {
			tickerSet[t.Ticker] = true
		}
		benchTicker := service.BenchmarkFor(m)
		snapshotTickers := make([]string, 0, len(tickerSet)+1)
		for t := range tickerSet {
			snapshotTickers = append(snapshotTickers, t)
		}
		if !tickerSet[benchTicker] {
			snapshotTickers = append(snapshotTickers, benchTicker)
		}
		to := time.Now().Format("2006-01-02")
		snapshots, err := paperDB.GetDailySnapshotsForTickers(snapshotTickers, txs[0].Date, to)
		if err != nil {
			return paperResponse{}, err
		}

		pnlCurve := CumulativeCurve(DailyPnL(txs, snapshots))
		resp.Curve = equityCurveFrom(pnlCurve, initialCash)
		resp.Drawdown = DrawdownSeries(resp.Curve)
		resp.KPIs.MaxDrawdownPct = maxDrawdownPct(resp.Curve)

		benchCloses := make(map[string]float64)
		for _, s := range snapshots {
			if s.Ticker == benchTicker {
				benchCloses[s.Date] = s.Close
			}
		}
		if benchCurve := BenchmarkReplay(txs, benchCloses); benchCurve != nil {
			resp.Benchmark = equityCurveFrom(benchCurve, initialCash)
			if initialCash > 0 {
				resp.KPIs.BenchmarkReturnPct = (resp.Benchmark[len(resp.Benchmark)-1].Value - initialCash) / initialCash * 100
			}
			resp.KPIs.AlphaPct = resp.KPIs.TotalReturnPct - resp.KPIs.BenchmarkReturnPct
		}
	}

	return resp, nil
}

// equityCurveFrom shifts a cumulative-P&L curve (pnl.go's CumulativeCurve/
// BenchmarkReplay both return "P&L relative to 0") into an equity-level
// curve starting near initialCash — docs/phase-11-paper-account.md §7.1
// asks for "curve"/"benchmark" as the equity curve itself, directly
// overlayable, rather than a P&L delta.
func equityCurveFrom(pnlCurve []DateValue, initialCash float64) []DateValue {
	out := make([]DateValue, len(pnlCurve))
	for i, v := range pnlCurve {
		out[i] = DateValue{Date: v.Date, Value: v.Value + initialCash}
	}
	return out
}

// maxDrawdownPct is the largest peak-to-trough decline in curve as a
// percentage of the running peak. DrawdownSeries/MaxDrawdownAbs (pnl.go)
// only give a dollar figure — meaningful for a cumulative-P&L curve
// starting at 0, but docs/phase-11-paper-account.md §7.1 asks for a
// percentage here, so this walks the same peak logic and divides by peak
// instead. Shifting curve by a constant (equityCurveFrom) doesn't change
// this result, since both peak and current value shift by the same amount.
func maxDrawdownPct(curve []DateValue) float64 {
	if len(curve) < 2 {
		return 0
	}
	peak := curve[0].Value
	var maxDD float64
	for _, v := range curve[1:] {
		if v.Value > peak {
			peak = v.Value
			continue
		}
		if peak <= 0 {
			continue
		}
		if dd := (peak - v.Value) / peak * 100; dd > maxDD {
			maxDD = dd
		}
	}
	return maxDD
}

// closedPositionFromRound converts one of segmentRounds' closed rounds
// (r.EndDate != "") into a paperClosedResponse. Weighted-average
// AvgCost/ExitPrice across legs rather than assuming exactly one BUY/one
// SELL — Phase 11's engine never partial-sells or adds (§9's 不做清單), so
// in practice every paper.db round is exactly two legs, but this doesn't
// assume that. ExitReason needs zero schema changes: the SELL leg's own
// StopPrice is already the position's stop snapshot at sale time
// (migration 13, RecordSell) — exitPrice <= stopPrice means the stop
// triggered the exit, otherwise it was a plain LLM SELL recommendation
// (docs/phase-11-paper-account.md §7.1).
func closedPositionFromRound(ticker string, r round) paperClosedResponse {
	var buyNotional, buyShares, sellNotional, sellShares, realized float64
	var lastSell db.Transaction
	for _, leg := range r.Legs {
		switch leg.Side {
		case "BUY":
			buyNotional += leg.Price * leg.Shares
			buyShares += leg.Shares
		case "SELL":
			sellNotional += leg.Price * leg.Shares
			sellShares += leg.Shares
			realized += leg.RealizedPnL
			lastSell = leg
		}
	}

	cr := paperClosedResponse{
		Ticker: ticker, EntryDate: r.StartDate, ExitDate: r.EndDate,
		Shares: buyShares, RealizedPnL: realized, ExitReason: "llm_sell",
	}
	if buyShares > 0 {
		cr.AvgCost = buyNotional / buyShares
	}
	if sellShares > 0 {
		cr.ExitPrice = sellNotional / sellShares
	}
	if buyNotional > 0 {
		cr.RealizedPct = realized / buyNotional * 100
	}
	if lastSell.StopPrice > 0 && lastSell.Price <= lastSell.StopPrice {
		cr.ExitReason = "stop"
	}
	return cr
}
