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

type openPosition struct {
	EntryDate  string
	EntryPrice float64
}

// regimeGate is §8.3's single switch, applied uniformly across every
// strategy's pooled entriesByDate (never per-strategy — see main.go's
// -regime-gate flag doc comment on the multiple-comparisons objection this
// avoids): "bull-only" skips every entry signal on a day marketRegimeAt
// scores "bear", using the SAME benchCandles index the day's own MarkClose
// already reads, so there's no separate lookahead-prone lookup. Exits are
// never gated — a stop/trailing/target must still fire in a bear day, only
// NEW positions are withheld. "off" (default) runs unchanged.
//
// Measured 2026-09-02 (US, S&P 500, same config as volMultiplierAt's own
// measured run above: 10y cache, -portfolio-cash=100000
// -portfolio-risk-pct=1.0 -portfolio-max-position-pct=25), both halves of
// the standard 2021-11 split, -regime-gate=off vs bull-only:
//
//	slice              Sharpe (off -> on)   MaxDD% (off -> on)   Ann.Ret% (off -> on)   trades
//	holdout 16-21     0.50 -> 0.53         28.51 -> 30.61        +6.36 -> +7.02          145 -> 147
//	in-sample 21-26   0.69 -> 0.43         24.05 -> 22.29        +13.46 -> +6.13         236 -> 164
//
// NO-SHIP: §8.3.4's bar is Sharpe AND max drawdown BOTH improving in BOTH
// slices. Neither slice clears it, and the two metrics don't even agree
// with each other within a slice: the holdout's Sharpe improves but its
// drawdown gets WORSE (28.51 -> 30.61); the in-sample's drawdown improves
// but its Sharpe collapses (0.69 -> 0.43, driven by trade count dropping
// 236 -> 164 — bear days withheld more of the in-sample's best trades than
// they protected against). This is the same shape of result as
// volMultiplierAt's rejection above (mechanism sound, doesn't clear the bar
// on this market/window) and confirms §8.3.4's own "反對理由②samples"
// worry: gating out roughly half the calendar days measurably thins the
// trade set without the drawdown benefit gate-on-regime is supposed to buy.
// The flag stays default OFF — not a parameter to re-tune post hoc (a
// different regime definition, e.g. MA200, would need its own
// pre-registered run, not a retry against these numbers).
//
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
func runPortfolioBacktest(hists map[string]tickerHist, entriesByDate map[string][]string, benchCandles []data.Candle, cfg paper.Config, initialCash float64, volTarget bool, regimeGate string, fromDate, toDate string) portfolioResult {
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
			// §8.4②: a "partial_target" fill (paper.Config.PartialExitAtR)
			// doesn't close the position — the ticker is still in
			// acct.Holdings afterward — so it isn't a round trip yet; leave
			// `open`'s original entry alone for whenever the remainder does
			// fully close.
			if _, stillOpen := acct.Holdings[sell.Ticker]; stillOpen {
				continue
			}
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

		gated := regimeGate == "bull-only" && marketRegimeAt(benchCandles, i) == "bear"
		if !gated {
			for _, ticker := range entriesByDate[date] {
				h, ok := hists[ticker]
				if !ok {
					continue
				}
				price, atr, _, ok := h.closeAndATR(date)
				if !ok || price <= 0 {
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

func printPortfolioResult(initialCash float64, r portfolioResult, volTarget bool, regimeGate string, partialExitAtR float64) {
	fmt.Printf("\n=======================================================\n")
	fmt.Printf(" 組合回測（Phase 25 §3.3 portfolio-layer backtest, vol-target=%v, regime-gate=%s, partial-exit-at-r=%.2f）\n", volTarget, regimeGate, partialExitAtR)
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
