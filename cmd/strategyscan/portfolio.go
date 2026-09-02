package main

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"strconv"

	"argus/internal/data"
	"argus/internal/paper"
	"argus/internal/signals"
)

// This file is Phase 25 §3.3's "portfolio-layer measurement infra": every
// other number cmd/strategyscan prints is a per-trade average from
// simulateTrade's disposable one-ticker account (huge cash, MaxPositionPct
// forced to 0 so the simulated BUY always fills — see simulatedAccountCash's
// doc comment). That answers "is this signal's expected value positive" but
// cannot answer "what would the equity curve have looked like" — no shared
// cash, no concurrent-position limit, no drawdown, no Sharpe. Any strategy
// whose value is risk reduction rather than return (§3's vol-target overlay
// being the motivating case) is structurally invisible to the rest of this
// tool, which is exactly what -portfolio-backtest exists to fix.
//
// It is off by default and additive: nothing here runs, and nothing else in
// this file is imported, unless -portfolio-backtest is passed. The default
// run's output is therefore untouched byte-for-byte (TestPortfolioBacktest
// in portfolio_test.go pins the vol-target half of that claim; the "doesn't
// run at all" half is structural — see main.go's `if tickerHists != nil`
// guards around every write into this file's types).

// tickerHist is one ticker's full candle series plus O(1) date lookup and
// precomputed highs/lows/closes, so runPortfolioBacktest's per-day ATR reads
// don't re-copy the whole history on every call the way simulateTrade's
// atrAt already tolerates for a single trade's bounded hold — a
// chronological multi-year, multi-ticker replay calls this far more often.
type tickerHist struct {
	candles             []data.Candle
	idxByDate           map[string]int
	highs, lows, closes []float64
}

func newTickerHist(candles []data.Candle) tickerHist {
	idx := make(map[string]int, len(candles))
	for i, c := range candles {
		idx[c.Date.Format("2006-01-02")] = i
	}
	return tickerHist{
		candles:   candles,
		idxByDate: idx,
		highs:     data.Highs(candles),
		lows:      data.Lows(candles),
		closes:    data.Closes(candles),
	}
}

// closeAndATR returns date's close and ATR(14) as of that bar plus its
// index. ok is false when the ticker has no bar on that date (a fetch gap,
// or a day it didn't trade) — same degrade-by-omission convention
// paper.Account.MarkClose already uses for a missing price, rather than
// treating a hole in one ticker's history as a reason to fail the whole
// replay.
func (h tickerHist) closeAndATR(date string) (close, atr float64, idx int, ok bool) {
	i, ok := h.idxByDate[date]
	if !ok {
		return 0, 0, 0, false
	}
	return h.candles[i].Close, signals.ATR(h.highs[:i+1], h.lows[:i+1], h.closes[:i+1], atrPeriod), i, true
}

// equityPoint is one day of the portfolio backtest's equity curve.
type equityPoint struct {
	Date          string
	Equity, Cash  float64
	PositionsHeld int
}

// closedPortfolioTrade is one round trip in the chronological replay,
// pairing a BUY with the SELL that later closed it — the replay never sends
// an explicit SELL signal, every exit is paper.Account.MarkClose's own
// stop/trailing/target, so every close can be paired to the entry that
// opened it. ReturnPct is the raw price return, independent of Shares — the
// field docs/phase-25-new-strategy-candidates.md §3.5's invariant checks
// stays IDENTICAL between -vol-target off and on; only Shares may differ.
//
// One nuance found running this for real (2016-2026, S&P 500): the two runs'
// TRADE SETS are not identical, even though every trade both runs DO share
// matches exactly on ticker/EntryDate/ExitDate/ReturnPct (live-verified, 0
// mismatches across 256 shared trades in both time slices). This is not a
// bug — it's Part A's shared, finite cash pool doing its job. A smaller
// vol-target position leaves more cash for a LATER entry that the other
// run's larger position would have crowded out (and vice versa), so which
// entries can even afford to fill genuinely differs. The invariant is "a
// trade's mechanics never depend on its size," not "the two runs trade
// identically" — the latter would only hold in an account with unlimited
// cash, which defeats the point of Part A's portfolio-layer cash sharing.
type closedPortfolioTrade struct {
	Ticker                string
	EntryDate, ExitDate   string
	EntryPrice, ExitPrice float64
	Shares                float64
	ReturnPct             float64
	ExitReason            string
}

// portfolioResult is one full chronological replay's output.
type portfolioResult struct {
	Curve               []equityPoint
	Trades              []closedPortfolioTrade
	MaxDrawdownPct      float64
	SharpeAnnualized    float64
	AnnualizedReturnPct float64
	FinalEquity         float64
}

// Phase 25 §3's market-level exposure overlay: SPY's trailing volRVWindow-day
// realized volatility, ranked against its own trailing volPctLookback days,
// mapped LINEARLY onto [volMultFloor, volMultCeil] — the top of its own
// trailing-year range shrinks sizing to volMultFloor, the bottom grows it to
// volMultCeil, the median leaves it neutral at 1.0. These four numbers are
// pre-registered per §3.5/the workstream brief — chosen before any backtest
// was run, not tuned after seeing a result.
//
// Measured 2026-08-27 (US, S&P 500, 10y cache, -portfolio-cash=100000
// -portfolio-risk-pct=1.0 -portfolio-max-position-pct=25, both matching the
// live paper account's own defaults), both halves of the standard 2021-11
// split, -vol-target off vs on:
//
//	slice              Sharpe (off -> on)   MaxDD% (off -> on)   Ann.Ret% (off -> on)   trades
//	holdout 16-21     0.54 -> 0.50         25.74 -> 25.76        +6.90 -> +6.32          135 -> 165
//	in-sample 21-26   0.81 -> 0.74         24.05 -> 23.60        +13.59 -> +11.88        234 -> 252
//
// NO-SHIP: Sharpe — the pre-registered primary metric — gets WORSE with the
// overlay on, in BOTH slices. Max drawdown is a wash (0.02pt worse in the
// holdout, 0.45pt better in-sample) — nowhere near the "both slices improve"
// bar §3.5 requires either way. The flag stays default OFF; this is not a
// parameter to re-tune post hoc (see the workstream's pre-registration rule)
// — a different lookback/floor/ceiling would need its own pre-registered
// sweep, not a retry against these numbers.
//
// This does not touch any entry-signal or exit-parameter conclusion from
// earlier Phase 25/23 studies: it changes position SIZE only, and every
// trade shared between an off/on pair of runs matches exactly on
// ticker/EntryDate/ExitDate/ReturnPct (see closedPortfolioTrade's doc
// comment) — the mechanism this workstream was built to isolate is working
// as designed, it just doesn't clear the bar on this market/window.
const (
	volRVWindow    = 20  // realized-vol lookback, trading days
	volPctLookback = 252 // trailing pool the percentile is ranked against, ~1y
	volMultFloor   = 0.5
	volMultCeil    = 1.5
)

// computeRV20 returns SPY's annualized volRVWindow-day realized volatility
// (population stdev of daily log returns over the trailing window x
// sqrt(252)) aligned 1:1 with closes. Entries before there's a full window,
// or spanning a non-positive close, are NaN.
func computeRV20(closes []float64) []float64 {
	out := make([]float64, len(closes))
	for i := range out {
		out[i] = math.NaN()
		if i < volRVWindow {
			continue
		}
		rets := make([]float64, 0, volRVWindow)
		ok := true
		for j := i - volRVWindow + 1; j <= i; j++ {
			if closes[j-1] <= 0 || closes[j] <= 0 {
				ok = false
				break
			}
			rets = append(rets, math.Log(closes[j]/closes[j-1]))
		}
		if !ok {
			continue
		}
		m := mean(rets)
		var ss float64
		for _, r := range rets {
			ss += (r - m) * (r - m)
		}
		out[i] = math.Sqrt(ss/float64(len(rets))) * math.Sqrt(252)
	}
	return out
}

// volMultiplierAt returns the RiskPct multiplier for benchmark index i.
// Returns neutral 1.0 whenever there isn't a full trailing volPctLookback
// pool yet, rather than ranking against a short/biased sample.
func volMultiplierAt(rv20 []float64, i int) float64 {
	if i < 0 || i >= len(rv20) || math.IsNaN(rv20[i]) {
		return 1.0
	}
	start := i - volPctLookback
	if start < 0 {
		return 1.0
	}
	var below, n int
	for j := start; j <= i; j++ {
		if math.IsNaN(rv20[j]) {
			continue
		}
		n++
		if rv20[j] <= rv20[i] {
			below++
		}
	}
	if n == 0 {
		return 1.0
	}
	pct := float64(below) / float64(n)
	return volMultCeil - pct*(volMultCeil-volMultFloor)
}

// Phase 25 §8.6's diversification gate: `paper.Config.MaxPositionPct` caps a
// single position's size but has no notion of "these N concurrently-held
// positions are really the same bet" (same sector/factor moving together).
// Rather than building sector classification (TW has one via FinMind, US
// doesn't — see docs/phase-25-new-strategy-candidates.md §8.6), this uses
// trailing correlationWindow-day daily-return correlation between a BUY
// candidate and each currently-held ticker as a market-agnostic proxy: two
// tickers that have been moving together ARE the same bet, whatever the
// reason. correlationWindow=60 (~3 months) and the 0.7 threshold tested below
// are pre-registered per §4.4, not swept — this is a portfolio-only overlay
// (like -vol-target), never wired into paper.Config/simulateTrade, since it
// only means anything once more than one position can be held at once.
//
// Measured 2026-09-02 via -portfolio-backtest -max-correlation-at-entry=0.7
// vs =0 (US S&P 500, 10y cache, -portfolio-cash=100000 -portfolio-risk-pct=1.0
// -portfolio-max-position-pct=25 — same settings as §3/§8.3/§8.4②),
// acceptance metric Sharpe+max-drawdown per the doc's §8.6 note (same
// instrument as §3/§8.4②, not per-trade return):
//
//	slice              Sharpe (off -> on)   MaxDD% (off -> on)   Ann.Ret% (off -> on)   trades
//	holdout 16-21     0.50 -> 0.36         28.51 -> 26.90        +6.36 -> +4.18          145 -> 141
//	in-sample 21-26   0.69 -> 0.68         24.05 -> 23.81       +13.46 -> +12.55          236 -> 215
//
// NO-SHIP: max drawdown improves in both slices, but Sharpe — the
// pre-registered primary metric — gets WORSE in both, sharply so in the
// holdout (0.50 -> 0.36). Skipping a correlated entry doesn't just cut the
// crowded-bet risk the gate targets; on this signal set it disproportionately
// skips entries that would have been winners, so the return given up costs
// more than the smoother ride is worth. Fails the "both metrics improve in
// both slices" bar, the same rejection shape as §3's vol-target, §8.3's
// regime gate, and §8.4③'s breakeven stop. Flag stays default off; this is
// not a parameter to re-tune post hoc (§4.4) — a different threshold/window
// would need its own pre-registered run.
const correlationWindow = 60

// pearson is the standard product-moment correlation coefficient. Returns 0
// when either series has no variance (a flat run), rather than NaN.
func pearson(a, b []float64) float64 {
	n := len(a)
	if n == 0 {
		return 0
	}
	ma, mb := mean(a), mean(b)
	var cov, va, vb float64
	for i := 0; i < n; i++ {
		da, db := a[i]-ma, b[i]-mb
		cov += da * db
		va += da * da
		vb += db * db
	}
	if va == 0 || vb == 0 {
		return 0
	}
	return cov / math.Sqrt(va*vb)
}

// trailingCorrelation pairs hA's and hB's daily log returns over the
// correlationWindow trading days ending at hA's bar idxA, matched by
// calendar date (hB may have gaps hA doesn't, e.g. a later listing date) —
// ok is false when fewer than half the window's days align, too thin a
// sample to trust.
func trailingCorrelation(hA, hB tickerHist, idxA int) (corr float64, ok bool) {
	if idxA < correlationWindow {
		return 0, false
	}
	retsA := make([]float64, 0, correlationWindow)
	retsB := make([]float64, 0, correlationWindow)
	for i := idxA - correlationWindow + 1; i <= idxA; i++ {
		date := hA.candles[i].Date.Format("2006-01-02")
		prevDate := hA.candles[i-1].Date.Format("2006-01-02")
		jb, okB := hB.idxByDate[date]
		jbPrev, okBPrev := hB.idxByDate[prevDate]
		if !okB || !okBPrev {
			continue
		}
		if hA.closes[i-1] <= 0 || hA.closes[i] <= 0 || hB.closes[jbPrev] <= 0 || hB.closes[jb] <= 0 {
			continue
		}
		retsA = append(retsA, math.Log(hA.closes[i]/hA.closes[i-1]))
		retsB = append(retsB, math.Log(hB.closes[jb]/hB.closes[jbPrev]))
	}
	if len(retsA) < correlationWindow/2 {
		return 0, false
	}
	return pearson(retsA, retsB), true
}

// tooCorrelated reports whether ticker h (at bar idx) is correlated at or
// above threshold with any currently-held position — the gate
// runPortfolioBacktest's entry loop applies before opening a new position.
func tooCorrelated(hists map[string]tickerHist, holdings map[string]paper.Holding, h tickerHist, idx int, threshold float64) bool {
	for heldTicker := range holdings {
		hb, ok := hists[heldTicker]
		if !ok {
			continue
		}
		if corr, ok := trailingCorrelation(h, hb, idx); ok && corr >= threshold {
			return true
		}
	}
	return false
}

type openPosition struct {
	EntryDate  string
	EntryPrice float64
}

// runPortfolioBacktest replays a single paper.Account chronologically across
// the benchmark's own trading calendar, applying entriesByDate's BUY signals
// (built from every strategy's deduped hits — never the baseline, which is a
// random-sampling control, not a set of positions anyone would have taken)
// and settling every open holding against the day's close first
// (paper.Account.MarkClose's own documented order — "yesterday's positions
// face today's stop first"). cfg.RiskPct/MaxPositionPct/Market/exit knobs are
// the caller's; cfg.RiskPct is scaled per entry by volMultiplierAt when
// volTarget is set — multiplying, not replacing, SuggestShares' own
// ATR-based per-stock sizing (§3.4). fromDate/toDate bound the calendar walk
// the same way -date-from/-date-to bound which triggers get recorded
// upstream, so a time-sliced run's curve doesn't include days outside the
// slice it's studying.
func runPortfolioBacktest(hists map[string]tickerHist, entriesByDate map[string][]string, benchCandles []data.Candle, cfg paper.Config, initialCash float64, volTarget bool, maxCorrelationAtEntry float64, fromDate, toDate string) portfolioResult {
	rv20 := computeRV20(data.Closes(benchCandles))
	acct := paper.NewAccount(initialCash)
	open := make(map[string]openPosition)

	var curve []equityPoint
	var trades []closedPortfolioTrade

	for i, bc := range benchCandles {
		date := bc.Date.Format("2006-01-02")
		if fromDate != "" && date < fromDate {
			continue
		}
		if toDate != "" && date > toDate {
			break
		}

		closes := make(map[string]float64, len(acct.Holdings))
		atrs := make(map[string]float64, len(acct.Holdings))
		for t := range acct.Holdings {
			h, ok := hists[t]
			if !ok {
				continue
			}
			if c, a, _, ok := h.closeAndATR(date); ok {
				closes[t], atrs[t] = c, a
			}
		}
		for _, sell := range acct.MarkClose(date, closes, atrs, cfg) {
			if e, ok := open[sell.Ticker]; ok {
				trades = append(trades, closedPortfolioTrade{
					Ticker: sell.Ticker, EntryDate: e.EntryDate, ExitDate: date,
					EntryPrice: e.EntryPrice, ExitPrice: sell.Price, Shares: sell.Shares,
					ReturnPct:  (sell.Price - e.EntryPrice) / e.EntryPrice * 100,
					ExitReason: sell.Reason,
				})
				delete(open, sell.Ticker)
			}
		}

		for _, ticker := range entriesByDate[date] {
			h, ok := hists[ticker]
			if !ok {
				continue
			}
			price, atr, idx, ok := h.closeAndATR(date)
			if !ok || price <= 0 {
				continue
			}
			if maxCorrelationAtEntry > 0 && tooCorrelated(hists, acct.Holdings, h, idx, maxCorrelationAtEntry) {
				continue
			}
			entryCfg := cfg
			if volTarget {
				entryCfg.RiskPct = cfg.RiskPct * volMultiplierAt(rv20, i)
			}
			trade, filled := acct.ApplySignal(paper.Signal{Date: date, Ticker: ticker, Action: "BUY", Price: price}, price, atr, entryCfg)
			if filled {
				open[ticker] = openPosition{EntryDate: date, EntryPrice: trade.Price}
			}
		}

		dayPrices := make(map[string]float64, len(acct.Holdings))
		for t := range acct.Holdings {
			if h, ok := hists[t]; ok {
				if c, _, _, ok := h.closeAndATR(date); ok {
					dayPrices[t] = c
				}
			}
		}
		curve = append(curve, equityPoint{
			Date: date, Equity: acct.Equity(dayPrices), Cash: acct.Cash, PositionsHeld: len(acct.Holdings),
		})
	}

	result := portfolioResult{
		Curve:               curve,
		Trades:              trades,
		MaxDrawdownPct:      maxDrawdownPct(curve),
		SharpeAnnualized:    sharpeAnnualized(curve),
		AnnualizedReturnPct: annualizedReturnPct(curve),
		FinalEquity:         initialCash,
	}
	if len(curve) > 0 {
		result.FinalEquity = curve[len(curve)-1].Equity
	}
	return result
}

func maxDrawdownPct(curve []equityPoint) float64 {
	var peak, maxDD float64
	for _, p := range curve {
		if p.Equity > peak {
			peak = p.Equity
		}
		if peak > 0 {
			if dd := (peak - p.Equity) / peak * 100; dd > maxDD {
				maxDD = dd
			}
		}
	}
	return maxDD
}

// sharpeAnnualized is mean(daily return)/stdev(daily return) x sqrt(252),
// risk-free rate assumed 0 (this backtest has no cash-yield model, matching
// paper.Account's own no-interest-on-cash behavior). 0 when there's too
// little history or the curve never moved (stdev 0), rather than +-Inf/NaN.
func sharpeAnnualized(curve []equityPoint) float64 {
	if len(curve) < 2 {
		return 0
	}
	rets := make([]float64, 0, len(curve)-1)
	for i := 1; i < len(curve); i++ {
		if curve[i-1].Equity <= 0 {
			continue
		}
		rets = append(rets, curve[i].Equity/curve[i-1].Equity-1)
	}
	if len(rets) < 2 {
		return 0
	}
	m := mean(rets)
	var ss float64
	for _, r := range rets {
		ss += (r - m) * (r - m)
	}
	sd := math.Sqrt(ss / float64(len(rets)-1))
	if sd == 0 {
		return 0
	}
	return m / sd * math.Sqrt(252)
}

// annualizedReturnPct is CAGR over the curve's own length in trading days
// (252/yr, same convention stop_study.py's "R/yr" column already uses).
func annualizedReturnPct(curve []equityPoint) float64 {
	if len(curve) < 2 || curve[0].Equity <= 0 {
		return 0
	}
	years := float64(len(curve)-1) / 252.0
	if years <= 0 {
		return 0
	}
	return (math.Pow(curve[len(curve)-1].Equity/curve[0].Equity, 1/years) - 1) * 100
}

func printPortfolioResult(initialCash float64, r portfolioResult, volTarget bool, maxCorrelationAtEntry float64) {
	fmt.Printf("\n=======================================================\n")
	fmt.Printf(" 組合回測（Phase 25 §3.3 portfolio-layer backtest, vol-target=%v, max-correlation-at-entry=%.2f）\n", volTarget, maxCorrelationAtEntry)
	fmt.Printf("=======================================================\n")
	fmt.Printf("起始資金 $%.0f -> 期末權益 $%.0f（%d 個交易日、%d 筆平倉交易）\n",
		initialCash, r.FinalEquity, len(r.Curve), len(r.Trades))
	fmt.Printf("年化報酬: %+.2f%%\n", r.AnnualizedReturnPct)
	fmt.Printf("最大回撤: %.2f%%\n", r.MaxDrawdownPct)
	fmt.Printf("Sharpe（年化, rf=0）: %.2f\n", r.SharpeAnnualized)
}

func writeEquityCurveCSV(path string, curve []equityPoint) {
	f, err := os.Create(path)
	if err != nil {
		fmt.Printf("Error creating equity curve CSV: %v\n", err)
		return
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	w.Write([]string{"Date", "Equity", "Cash", "PositionsHeld"})
	for _, p := range curve {
		w.Write([]string{
			p.Date,
			strconv.FormatFloat(p.Equity, 'f', 2, 64),
			strconv.FormatFloat(p.Cash, 'f', 2, 64),
			strconv.Itoa(p.PositionsHeld),
		})
	}
	fmt.Printf("Saved equity curve CSV to %s\n", path)
}

func writePortfolioTradesCSV(path string, trades []closedPortfolioTrade) {
	f, err := os.Create(path)
	if err != nil {
		fmt.Printf("Error creating portfolio trades CSV: %v\n", err)
		return
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	w.Write([]string{"Ticker", "EntryDate", "ExitDate", "EntryPrice", "ExitPrice", "Shares", "ReturnPct", "ExitReason"})
	for _, t := range trades {
		w.Write([]string{
			t.Ticker, t.EntryDate, t.ExitDate,
			strconv.FormatFloat(t.EntryPrice, 'f', 4, 64),
			strconv.FormatFloat(t.ExitPrice, 'f', 4, 64),
			strconv.FormatFloat(t.Shares, 'f', 4, 64),
			strconv.FormatFloat(t.ReturnPct, 'f', 6, 64),
			t.ExitReason,
		})
	}
	fmt.Printf("Saved portfolio trades CSV to %s (%d trades)\n", path, len(trades))
}
