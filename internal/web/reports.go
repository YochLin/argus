package web

import (
	"fmt"
	"sort"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/market"
)

// minSampleSize is the "low sample" honesty threshold (design doc's own
// words: "低頻交易者的分組統計很容易變成雜訊，UI 不能假裝它有統計意義") — a
// group with fewer than this many sells is still shown (never hidden — a
// user with only 2 trades on a ticker still wants to see them), just flagged
// via ReportGroup.LowSample so the frontend can grey it out instead of
// implying the same confidence as a 30-sample group. Same reasoning that got
// Zella Score cut from PR1.
const minSampleSize = 5

// sellRecord is one SELL leg annotated with the round context it belongs to
// — the "以每筆 SELL 與持倉回合為基礎" unit of analysis the design doc
// specifies (docs/phase-5-web-dashboard.md §A1): grouping dimensions live at
// the round level (a round has one entry, so one entry month/weekday), but
// realized P&L and holding days are per-sell, since a round can be exited in
// more than one partial sell, each with its own holding period and its own
// realized_pnl already computed by db.RecordSell.
type sellRecord struct {
	Ticker       string
	HoldingDays  int
	EntryMonth   time.Month
	EntryWeekday time.Weekday
	Sell         db.Transaction
}

// buildSellRecords walks txs (any order — grouped and re-sorted by ticker
// internally) into one sellRecord per SELL leg, via the same round
// segmentation rounds.go's segmentRounds already uses for the round detail
// page. A round's still-open trailing segment (no closing SELL) contributes
// nothing here — every sellRecord necessarily belongs to a SELL leg, and an
// open round's legs so far are all BUYs by definition of segmentRounds.
func buildSellRecords(txs []db.Transaction) ([]sellRecord, error) {
	byTicker := make(map[string][]db.Transaction)
	for _, t := range txs {
		byTicker[t.Ticker] = append(byTicker[t.Ticker], t)
	}

	var records []sellRecord
	for ticker, tickerTxs := range byTicker {
		for _, r := range segmentRounds(tickerTxs) {
			startT, err := time.Parse("2006-01-02", r.StartDate)
			if err != nil {
				return nil, fmt.Errorf("web: parse round start %s: %w", r.StartDate, err)
			}
			for _, leg := range r.Legs {
				if leg.Side != "SELL" {
					continue
				}
				sellT, err := time.Parse("2006-01-02", leg.Date)
				if err != nil {
					return nil, fmt.Errorf("web: parse sell date %s: %w", leg.Date, err)
				}
				records = append(records, sellRecord{
					Ticker:       ticker,
					HoldingDays:  int(sellT.Sub(startT).Hours() / 24),
					EntryMonth:   startT.Month(),
					EntryWeekday: startT.Weekday(),
					Sell:         leg,
				})
			}
		}
	}
	return records, nil
}

// sellReturnPct backs out the sell's return% from db.RecordSell's own
// formula (realizedPnL = (price-avgCost)*shares - fee) rather than needing
// avgCost as a separate input — transactions never stores avgCost directly.
// costBasis = avgCost*shares = price*shares - fee - realizedPnL; the return
// is realizedPnL over that cost basis, expressed net of fee (the same
// headline figure /portfolio and /track already surface), not the
// fee-blind (price-avgCost)/avgCost gross figure internal/bot uses for a
// still-open position's unrealized%. ok is false when costBasis isn't
// positive (shouldn't happen for a real trade, but a divide-by-zero guard
// costs nothing).
func sellReturnPct(t db.Transaction) (pct float64, ok bool) {
	costBasis := t.Price*t.Shares - t.Fee - t.RealizedPnL
	if costBasis <= 0 {
		return 0, false
	}
	return t.RealizedPnL / costBasis * 100, true
}

// ReportGroup is one row of a grouped performance report — one bucket along
// one dimension (ticker / holding-days / entry month / entry weekday).
type ReportGroup struct {
	Key              string  `json:"key"`
	N                int     `json:"n"`
	WinRate          float64 `json:"winRate"`      // fraction 0-1, same convention as kpisResponse.WinRate
	ProfitFactor     float64 `json:"profitFactor"`
	AvgReturnPct     float64 `json:"avgReturnPct"` // already-scaled percent (12.3 means 12.3%)
	TotalRealizedPnL float64 `json:"totalRealizedPnL"`
	AvgHoldingDays   float64 `json:"avgHoldingDays"`
	LowSample        bool    `json:"lowSample"` // n < minSampleSize
}

// holdingDaysBucket sorts a sell's holding period into one of four fixed
// bands, chosen for a low-frequency swing trader's own time scale (Argus's
// stated positioning — "低頻交易導向") rather than a day-trading-oriented
// scale: same-week, few-weeks-to-a-month, a couple months, or a genuine
// long hold.
func holdingDaysBucket(days int) string {
	switch {
	case days <= 5:
		return "0-5d"
	case days <= 20:
		return "6-20d"
	case days <= 60:
		return "21-60d"
	default:
		return "60d+"
	}
}

var holdingDaysBucketOrder = []string{"0-5d", "6-20d", "21-60d", "60d+"}

var monthOrder = []string{"01", "02", "03", "04", "05", "06", "07", "08", "09", "10", "11", "12"}

// weekdayOrder is Monday-first (trading-week order) rather than Go's
// Sunday-first time.Weekday zero value, and omits Saturday/Sunday from the
// fixed ordering — a backdated /buy can technically land on a weekend, but
// no real trading day does, so any weekend key that does appear (a
// backdating typo, most likely) sorts after every real weekday rather than
// needing its own slot.
var weekdayOrder = []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}

// groupReport aggregates records into report rows keyed by keyFn, ordered by
// order (any key present in the data but absent from order sorts after every
// ordered key, alphabetically among themselves — a defensive fallback, not
// an expected path for the four dimensions this package actually uses).
func groupReport(records []sellRecord, keyFn func(sellRecord) string, order []string) []ReportGroup {
	type bucket struct {
		sells       []db.Transaction
		holdingDays []int
	}
	buckets := make(map[string]*bucket)
	for _, r := range records {
		key := keyFn(r)
		b := buckets[key]
		if b == nil {
			b = &bucket{}
			buckets[key] = b
		}
		b.sells = append(b.sells, r.Sell)
		b.holdingDays = append(b.holdingDays, r.HoldingDays)
	}

	rank := make(map[string]int, len(order))
	for i, k := range order {
		rank[k] = i
	}

	keys := make([]string, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		ri, iok := rank[keys[i]]
		rj, jok := rank[keys[j]]
		if iok && jok {
			return ri < rj
		}
		if iok != jok {
			return iok // ranked keys sort before unranked ones
		}
		return keys[i] < keys[j]
	})

	out := make([]ReportGroup, 0, len(keys))
	for _, key := range keys {
		b := buckets[key]
		var totalReturnPct float64
		var returnPctN int
		var totalHoldingDays int
		for i, s := range b.sells {
			if pct, ok := sellReturnPct(s); ok {
				totalReturnPct += pct
				returnPctN++
			}
			totalHoldingDays += b.holdingDays[i]
		}
		g := ReportGroup{
			Key:            key,
			N:              len(b.sells),
			WinRate:        WinRate(b.sells),
			ProfitFactor:   ProfitFactor(b.sells),
			AvgHoldingDays: float64(totalHoldingDays) / float64(len(b.sells)),
			LowSample:      len(b.sells) < minSampleSize,
		}
		for _, s := range b.sells {
			g.TotalRealizedPnL += s.RealizedPnL
		}
		if returnPctN > 0 {
			g.AvgReturnPct = totalReturnPct / float64(returnPctN)
		}
		out = append(out, g)
	}
	return out
}

// FeeSummary is the "順手項" fee rollup (design doc §A1) — fee had been
// recorded on every transaction since Phase 2 but never surfaced in
// aggregate anywhere.
type FeeSummary struct {
	TotalFees        float64 `json:"totalFees"`
	PctOfRealizedPnL float64 `json:"pctOfRealizedPnL"` // 0 when total realized P&L is 0
}

// buildFeeSummary sums fee across every transaction (BUY and SELL both —
// a buy's fee is folded into avg_cost and so already drags down the
// eventual sell's realized_pnl, but it's still a real dollar cost worth
// showing directly rather than only as an invisible component of avg_cost).
// PctOfRealizedPnL is fees against the *net* (already fee-inclusive)
// realized P&L total, the same headline figure /portfolio and this
// package's own KPIs already report — not a fee-added-back gross figure.
func buildFeeSummary(txs []db.Transaction) FeeSummary {
	var totalFees, totalRealizedPnL float64
	for _, t := range txs {
		totalFees += t.Fee
		if t.Side == "SELL" {
			totalRealizedPnL += t.RealizedPnL
		}
	}
	summary := FeeSummary{TotalFees: totalFees}
	if totalRealizedPnL != 0 {
		summary.PctOfRealizedPnL = totalFees / abs(totalRealizedPnL) * 100
	}
	return summary
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// StreakStats is the design doc's A5 "便宜 KPI 補完" — best/worst trade and
// longest win/loss streak, all a single scan of the same sells slice the
// existing WinRate/ProfitFactor/Expectancy KPIs already read, folded into
// the report page per the doc's "implementation-time" placement call rather
// than PR1's KPI cards (avoids widening that grid for a PR1 that already
// shipped).
type StreakStats struct {
	BestTradePnL      float64 `json:"bestTradePnL"`
	WorstTradePnL     float64 `json:"worstTradePnL"`
	AvgWinPnL         float64 `json:"avgWinPnL"`
	AvgLossPnL        float64 `json:"avgLossPnL"`
	LongestWinStreak  int     `json:"longestWinStreak"`
	LongestLossStreak int     `json:"longestLossStreak"`
}

// buildStreakStats assumes sells is already in chronological order (the
// order db.GetAllTransactions/GetTransactions return rows in) — streaks are
// meaningless computed over an arbitrarily-ordered slice.
func buildStreakStats(sells []db.Transaction) StreakStats {
	var s StreakStats
	if len(sells) == 0 {
		return s
	}
	s.BestTradePnL = sells[0].RealizedPnL
	s.WorstTradePnL = sells[0].RealizedPnL

	var winSum, lossSum float64
	var winN, lossN int
	var curWin, curLoss int
	for _, t := range sells {
		if t.RealizedPnL > s.BestTradePnL {
			s.BestTradePnL = t.RealizedPnL
		}
		if t.RealizedPnL < s.WorstTradePnL {
			s.WorstTradePnL = t.RealizedPnL
		}
		if t.RealizedPnL > 0 {
			winSum += t.RealizedPnL
			winN++
			curWin++
			curLoss = 0
		} else {
			lossSum += t.RealizedPnL
			lossN++
			curLoss++
			curWin = 0
		}
		if curWin > s.LongestWinStreak {
			s.LongestWinStreak = curWin
		}
		if curLoss > s.LongestLossStreak {
			s.LongestLossStreak = curLoss
		}
	}
	if winN > 0 {
		s.AvgWinPnL = winSum / float64(winN)
	}
	if lossN > 0 {
		s.AvgLossPnL = lossSum / float64(lossN)
	}
	return s
}

// reportsResponse is /api/reports' body — see handlers.go for the JSON
// contract this mirrors on the frontend.
type reportsResponse struct {
	ByTicker       []ReportGroup `json:"byTicker"`
	ByHoldingDays  []ReportGroup `json:"byHoldingDays"`
	ByEntryMonth   []ReportGroup `json:"byEntryMonth"`
	ByEntryWeekday []ReportGroup `json:"byEntryWeekday"`
	Fees           FeeSummary    `json:"fees"`
	Streaks        StreakStats   `json:"streaks"`
	MAEMFE         MAEMFESummary `json:"maeMfe"`
}

// buildReports assembles /api/reports: the grouped performance breakdown
// (design doc §A1), fee summary, cheap KPI completions (§A5), and the MAE/
// MFE captured-% aggregate (§A2, maefe.go) — restricted to market m for the
// same currency-mixing reason every other market-scoped builder in this
// package is (buildDashboard/buildCalendar/buildRounds). A single ticker's
// history fetch failing (for the MAE/MFE aggregate only) degrades that
// ticker's rounds out of the average rather than failing the whole
// response — see buildMAEMFESummary.
func buildReports(database dbReader, history data.HistoryProvider, m market.MarketID) (reportsResponse, error) {
	allTxs, err := database.GetAllTransactions()
	if err != nil {
		return reportsResponse{}, err
	}
	txs := filterTransactionsByMarket(allTxs, m)

	records, err := buildSellRecords(txs)
	if err != nil {
		return reportsResponse{}, err
	}

	resp := reportsResponse{
		ByTicker:       groupReport(records, func(r sellRecord) string { return r.Ticker }, nil),
		ByHoldingDays:  groupReport(records, func(r sellRecord) string { return holdingDaysBucket(r.HoldingDays) }, holdingDaysBucketOrder),
		ByEntryMonth:   groupReport(records, func(r sellRecord) string { return fmt.Sprintf("%02d", int(r.EntryMonth)) }, monthOrder),
		ByEntryWeekday: groupReport(records, func(r sellRecord) string { return r.EntryWeekday.String() }, weekdayOrder),
		Fees:           buildFeeSummary(txs),
		Streaks:        buildStreakStats(FilterSells(txs)),
	}
	// byTicker is sorted by total realized P&L, most profitable first —
	// unlike the other three dimensions there's no natural fixed order for
	// ticker symbols, and "which ticker makes money" is the report's main
	// question.
	sort.Slice(resp.ByTicker, func(i, j int) bool {
		if resp.ByTicker[i].TotalRealizedPnL != resp.ByTicker[j].TotalRealizedPnL {
			return resp.ByTicker[i].TotalRealizedPnL > resp.ByTicker[j].TotalRealizedPnL
		}
		return resp.ByTicker[i].Key < resp.ByTicker[j].Key
	})

	resp.MAEMFE = buildMAEMFESummary(txs, history)

	return resp, nil
}
