package main

import (
	"fmt"
	"sort"

	"argus/internal/data"
	"argus/internal/market"
	"argus/internal/paper"
)

// Phase 25 §8.5: cross-sectional native-form momentum — see
// docs/phase-25-new-strategy-candidates.md §8.5. Every screen in
// strategies.go asks "is THIS ticker a signal today"; this asks a
// structurally different question — "rank the WHOLE pool and hold the top
// decile" — which is why it needed its own engine rather than slotting into
// runPortfolioBacktest's per-signal entry loop (that loop only ever sees
// tickers a screen already picked out, never the full universe). Reuses
// portfolio.go's tickerHist/equityPoint/maxDrawdownPct/sharpeAnnualized/
// annualizedReturnPct/writeEquityCurveCSV; everything specific to this
// engine (ranking, monthly rebalance, equal-weight sizing, no exit logic)
// lives here.
//
// Deliberately has NO stop-loss/trailing-stop/target: a position's only
// exit is falling out of the top decile at the next rebalance. Mixing in an
// exit-shape mechanism here would confound the ranking question this file
// tests with §8.4's exit-timing questions, already studied separately.
//
// momentumLookback/momentumSkip are the standard Jegadeesh-Titman 12-1
// construction (skip the most recent month — short-term reversal would
// otherwise contaminate the ranking); momentumDecile (top 1/10 of the
// ranked, history-eligible pool) and monthly rebalancing are pre-registered
// per §4.4, not swept post hoc.
//
// Known characteristic, not a bug: a ticker only becomes rank-eligible once
// it has momentumLookback trading days of history, so the strategy sits in
// cash (rebalanceTargets returns nil) until enough of the pool clears that
// bar — for the §4.4 holdout slice (2016-11-01 start, ~10y cache), that's
// roughly the slice's first year. runEqualWeightBuyHold's control has no
// such warm-up, so the early months of any comparison favor the control on
// construction, not on the ranking's merit — same shape as volMultiplierAt's
// "no full trailing pool yet -> neutral" fallback in portfolio.go.
//
// Measured 2026-09-03 via -momentum-backtest (US S&P 500, 10y cache,
// -momentum-cash=100000, default), acceptance metric Sharpe+max-drawdown
// per §3/§8.3/§8.4②/§8.6's convention for a portfolio-layer comparison
// (strategy vs. its own control, not per-trade return):
//
//	slice              Sharpe (ctrl -> strat)   MaxDD% (ctrl -> strat)   Ann.Ret% (ctrl -> strat)
//	holdout 16-21     1.08 -> 0.96             37.22 -> 38.39            21.86 -> 22.76
//	in-sample 21-26   0.76 -> 1.10             20.78 -> 29.88            12.25 -> 29.16
//
// NO-SHIP: fails in both slices, on different halves of the bar. Holdout:
// the ranking is worse than the passive control on BOTH metrics (lower
// Sharpe, deeper drawdown) — 12-1 momentum simply didn't work on this pool
// in this window. In-sample: Sharpe improves sharply (0.76 -> 1.10, and
// annualized return nearly triples), but max drawdown gets WORSE (20.78% ->
// 29.88%) — concentrating into a top decile trades the control's diversified
// smoothness for a narrower, more volatile bet, and that trade-off shows up
// as drawdown even where the return more than compensates for it on Sharpe.
// Neither slice clears "both metrics improve," the same bar (and the same
// rejection shape) as §3's vol-target, §8.3's regime gate, §8.4③'s breakeven
// stop, and §8.6's correlation gate. This does not contradict the
// 2026-08-26 rejection of 12-1 momentum/252d RS as a ranking factor on
// screen-filtered candidates (see PLAN.md) — that tested momentum as an
// add-on to an already-filtered signal set; this tests it in native
// cross-sectional form, a different construction, and it also doesn't clear
// the bar. No flag exists to ship (this is a standalone backtest mode, not
// wired into any live path); -momentum-backtest stays as a measurement tool
// only. Not a parameter to re-tune post hoc (§4.4) — a different
// lookback/skip/decile would need its own pre-registered run.
const (
	momentumLookback = 252 // ~12 months of trading days
	momentumSkip     = 21  // ~1 month, excluded (short-term reversal)
	momentumDecile   = 10  // top 1/N of the ranked, history-eligible pool
)

// momentumAt returns ticker h's 12-1 momentum as of bar idx: the return from
// momentumLookback trading days ago to momentumSkip trading days ago. ok is
// false when h doesn't have momentumLookback bars of history yet as of idx.
func momentumAt(h tickerHist, idx int) (mom float64, ok bool) {
	if idx < momentumLookback {
		return 0, false
	}
	start := h.closes[idx-momentumLookback]
	end := h.closes[idx-momentumSkip]
	if start <= 0 || end <= 0 {
		return 0, false
	}
	return end/start - 1, true
}

// rebalanceTargets ranks every ticker in hists with a bar on date AND enough
// trailing history for momentumAt by descending 12-1 momentum, returning the
// top decile (at least 1, when the eligible pool is non-empty; nil when it
// isn't — a legal "stay in cash" result, not an error).
func rebalanceTargets(hists map[string]tickerHist, date string) []string {
	type scored struct {
		ticker string
		mom    float64
	}
	var pool []scored
	for ticker, h := range hists {
		idx, ok := h.idxByDate[date]
		if !ok {
			continue
		}
		mom, ok := momentumAt(h, idx)
		if !ok {
			continue
		}
		pool = append(pool, scored{ticker, mom})
	}
	if len(pool) == 0 {
		return nil
	}
	sort.Slice(pool, func(i, j int) bool { return pool[i].mom > pool[j].mom })
	n := len(pool) / momentumDecile
	if n < 1 {
		n = 1
	}
	out := make([]string, n)
	for i := range out {
		out[i] = pool[i].ticker
	}
	return out
}

// priceOn returns ticker's close on date, falling back to the last price
// seen (lastPrice, updated in place) when the ticker has no bar that day —
// same degrade-by-omission convention paper.Account.Equity uses for a
// missing quote, rather than zeroing a holding on a one-off data gap.
func priceOn(hists map[string]tickerHist, lastPrice map[string]float64, ticker, date string) (float64, bool) {
	if h, ok := hists[ticker]; ok {
		if idx, ok := h.idxByDate[date]; ok {
			p := h.closes[idx]
			lastPrice[ticker] = p
			return p, true
		}
	}
	p, ok := lastPrice[ticker]
	return p, ok
}

// buyEqualWeight opens shares of each ticker at date, equal-weight by
// equity, applying paper.FeeFor per fill (0 for US — this study's only
// market so far; a real deduction if this is ever pointed at TW). A ticker
// with no price on date is skipped; its share of equity stays in cash
// rather than being redistributed among the rest.
func buyEqualWeight(hists map[string]tickerHist, lastPrice map[string]float64, tickers []string, equity float64, date string, m market.MarketID) (map[string]float64, float64) {
	holdings := make(map[string]float64, len(tickers))
	cash := equity
	if len(tickers) == 0 {
		return holdings, cash
	}
	perTicker := equity / float64(len(tickers))
	for _, ticker := range tickers {
		price, ok := priceOn(hists, lastPrice, ticker, date)
		if !ok || price <= 0 {
			continue
		}
		shares := perTicker / price
		notional := shares * price
		fee := paper.FeeFor(m, "BUY", notional, 1.0)
		holdings[ticker] = shares
		cash -= notional + fee
	}
	return holdings, cash
}

// markToMarket is cash plus every holding's value at date, via priceOn's
// carry-forward fallback.
func markToMarket(hists map[string]tickerHist, lastPrice map[string]float64, holdings map[string]float64, cash float64, date string) float64 {
	equity := cash
	for ticker, shares := range holdings {
		if p, ok := priceOn(hists, lastPrice, ticker, date); ok && p > 0 {
			equity += p * shares
		}
	}
	return equity
}

// runMomentumBacktest replays an equal-weight, monthly-rebalanced top-decile
// 12-1 momentum portfolio over hists' whole pool, on benchCandles' own
// trading calendar (same calendar every other portfolio backtest in this
// package walks). Each rebalance liquidates the prior holdings and re-buys
// the fresh top decile in one step — no separate SELL-leg fee is charged
// (US's FeeFor is 0 either way; a TW reuse would need to add it explicitly).
// fromDate/toDate bound the walk the same way runPortfolioBacktest's do.
func runMomentumBacktest(hists map[string]tickerHist, benchCandles []data.Candle, initialCash float64, m market.MarketID, fromDate, toDate string) portfolioResult {
	holdings := make(map[string]float64)
	lastPrice := make(map[string]float64)
	cash := initialCash
	currentMonth := ""
	var curve []equityPoint

	for _, bc := range benchCandles {
		date := bc.Date.Format("2006-01-02")
		if fromDate != "" && date < fromDate {
			continue
		}
		if toDate != "" && date > toDate {
			break
		}

		if month := date[:7]; month != currentMonth {
			currentMonth = month
			equity := markToMarket(hists, lastPrice, holdings, cash, date)
			holdings, cash = buyEqualWeight(hists, lastPrice, rebalanceTargets(hists, date), equity, date, m)
		}

		curve = append(curve, equityPoint{
			Date: date, Cash: cash, PositionsHeld: len(holdings),
			Equity: markToMarket(hists, lastPrice, holdings, cash, date),
		})
	}
	return finishMomentumResult(curve, initialCash)
}

// runEqualWeightBuyHold is §8.5's control: every ticker with a bar on the
// walk's first date, bought once equal-weight and held flat — no rebalance,
// no ranking, no exits, no history-eligibility warm-up. The baseline
// runMomentumBacktest's result has to beat for the ranking to have added
// anything.
func runEqualWeightBuyHold(hists map[string]tickerHist, benchCandles []data.Candle, initialCash float64, m market.MarketID, fromDate, toDate string) portfolioResult {
	holdings := make(map[string]float64)
	lastPrice := make(map[string]float64)
	cash := initialCash
	bought := false
	var curve []equityPoint

	for _, bc := range benchCandles {
		date := bc.Date.Format("2006-01-02")
		if fromDate != "" && date < fromDate {
			continue
		}
		if toDate != "" && date > toDate {
			break
		}

		if !bought {
			bought = true
			var tickers []string
			for ticker, h := range hists {
				if _, ok := h.idxByDate[date]; ok {
					tickers = append(tickers, ticker)
				}
			}
			sort.Strings(tickers) // deterministic order only; equal-weight either way
			holdings, cash = buyEqualWeight(hists, lastPrice, tickers, cash, date, m)
		}

		curve = append(curve, equityPoint{
			Date: date, Cash: cash, PositionsHeld: len(holdings),
			Equity: markToMarket(hists, lastPrice, holdings, cash, date),
		})
	}
	return finishMomentumResult(curve, initialCash)
}

func finishMomentumResult(curve []equityPoint, initialCash float64) portfolioResult {
	result := portfolioResult{
		Curve:               curve,
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

func printMomentumResult(initialCash float64, strat, ctrl portfolioResult) {
	fmt.Printf("\n=======================================================\n")
	fmt.Printf(" 橫斷面動量回測（Phase 25 §8.5：月頻再平衡、全池前十分位 12-1 動量 vs 同池等權買進持有）\n")
	fmt.Printf("=======================================================\n")
	fmt.Printf("起始資金 $%.0f\n", initialCash)
	fmt.Printf("%-14s %12s %10s %10s %8s\n", "", "期末權益", "年化報酬", "最大回撤", "Sharpe")
	fmt.Printf("%-14s %12.0f %9.2f%% %9.2f%% %8.2f\n", "動量策略", strat.FinalEquity, strat.AnnualizedReturnPct, strat.MaxDrawdownPct, strat.SharpeAnnualized)
	fmt.Printf("%-14s %12.0f %9.2f%% %9.2f%% %8.2f\n", "等權買進持有", ctrl.FinalEquity, ctrl.AnnualizedReturnPct, ctrl.MaxDrawdownPct, ctrl.SharpeAnnualized)
}
