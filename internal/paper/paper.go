// Package paper is the pure rule engine shared by the two Phase 11 paths —
// argus backtest's historical replay (cmd/server/backtest.go) and the live
// paper account's forward accumulation (internal/bot/paper.go). Same
// "no DB/network/Telegram" discipline as internal/signals/internal/receval:
// Account.ApplySignal/MarkClose/Equity are the entire trading rulebook, fed
// plain values by both callers, so a divergence between backtest and live
// behavior is structurally impossible — see docs/phase-11-paper-account.md
// §1 for why that sharing is this package's only reason to exist.
package paper

import (
	"math"
	"sort"

	"argus/internal/market"
)

// Config parameterizes one account's sizing/exit rules. Market picks the fee
// model (FeeFor) — the two books (US/TW) never share a Config since their
// currencies and fee schedules differ.
type Config struct {
	InitialCash       float64
	RiskPct           float64 // max % of equity risked per trade (suggestShares budget)
	MaxPositionPct    float64 // single-position notional cap, % of equity; <=0 = no cap
	StopATRMult       float64 // ATR(14) multiple below entry price for the initial stop
	StopLossPct       float64 // fixed % fallback when ATR is unavailable
	TrailingPct       float64 // fixed trailing-stop distance, %; 0 = disabled
	TrailingATRMult   float64 // ATR-based trailing distance multiple; <=0 = fixed % only
	TakeProfitATRMult float64 // ATR(14) multiple above entry for the take-profit target; <=0 = disabled
	Market            market.MarketID
	FeeDiscount       float64 // TW brokerage discount on the commission leg only (statutory tax never discounts); 1.0 = no discount. Unused for US.

	// BreakevenAtR and PartialExitAtR are Phase 25 §8.4②③'s exit-shape
	// overlays, both <=0 by default (off) — see MarkClose for the mechanism.
	// R is the initial per-share risk, AvgCost-Stop as set at entry; both are
	// read against that same initial R even after BreakevenAtR has moved
	// Stop, so turning both on in the same run is well-defined but was never
	// how either was measured — §4.4 requires each pre-registered and tested
	// independently, at R=1 (the textbook "1R" choice; not swept — a sweep
	// over several R values before picking the best is exactly the multiple-
	// comparisons problem §8.3's regime-gate result was written up to avoid).
	//
	// BreakevenAtR: measured 2026-09-02 via cmd/strategyscan -breakeven-at-r,
	// paired against the SAME baseline (random-entry) trades at R=0 vs R=1,
	// -dump-trades + breakeven_study.py's bootstrap paired diff (400
	// resamples), both pre-registered §4.4 slices:
	//
	//	slice              n       mean ExitRet (off -> on)   win% (off -> on)   paired diff    sigma
	//	holdout 16-21     58893   +2.89% -> +2.60%            43.6 -> 34.7       -0.29pp        16.8
	//	in-sample 21-26   59588   +0.97% -> +0.88%            34.6 -> 27.0       -0.09pp         4.9
	//
	// NO-SHIP, overwhelmingly: mean return and win rate both fall in BOTH
	// slices, at 16.8σ/4.9σ — moving the stop to breakeven at 1R cuts off
	// exactly the trades that pull back through cost before continuing to a
	// real winner, which this data shows happens often enough to cost more
	// than it protects. Flag stays default off.
	//
	// PartialExitAtR: measured 2026-09-02 via cmd/strategyscan
	// -portfolio-backtest -partial-exit-at-r=1 vs =0 (US S&P 500, 10y cache,
	// -portfolio-cash=100000 -portfolio-risk-pct=1.0
	// -portfolio-max-position-pct=25 — same settings as §3/§8.3), acceptance
	// metric Sharpe+max-drawdown per PLAN.md's §8.4② note (per-trade return
	// is the wrong instrument here — see doc §1.2 gap B — the overlay's
	// value if any is in reduced drawdown, not return):
	//
	//	slice              Sharpe (off -> on)   MaxDD% (off -> on)   Ann.Ret% (off -> on)   trades
	//	holdout 16-21     0.50 -> 0.62         28.51 -> 23.45        +6.36 -> +8.37          145 -> 586
	//	in-sample 21-26   0.69 -> 0.62         24.05 -> 22.79       +13.46 -> +10.75          236 -> 642
	//
	// NO-SHIP: max drawdown improves in BOTH slices, and the holdout clears
	// the full bar (Sharpe and MaxDD both better) — but in-sample Sharpe gets
	// WORSE (0.69 -> 0.62), so it fails §3.5's "both slices must improve"
	// requirement on the primary metric, the same shape of rejection as §3's
	// vol-target and §8.3's regime gate. The trade-count jump (freed-up cash
	// from early partial exits lets more later entries fill — same mechanism
	// closedPortfolioTrade's doc comment describes for -vol-target) is
	// expected, not a bug. Flag stays default off; this is not a parameter to
	// re-tune post hoc (§4.4) — a different R would need its own
	// pre-registered run.
	BreakevenAtR   float64 // once Peak reaches AvgCost + this many R, Stop moves up to AvgCost (breakeven); never moves back down
	PartialExitAtR float64 // once Peak reaches AvgCost + this many R, sell half the position (floored) at that close, clear Target, and let the remainder run on the trailing stop alone
}

// Signal is one recommendation to apply: BUY/SELL/HOLD/"" at Price on Date.
type Signal struct {
	Date, Ticker, Action string
	Price                float64
}

// Holding is one open position. Peak tracks the highest close seen since
// entry (updated by MarkClose) for the trailing stop; Stop is the fixed
// initial stop set at entry and never tightened — the trailing exit is a
// separate check against Peak, not a moving Stop. Target is the take-profit
// price, also set once at entry and never moved; 0 means take-profit is
// disabled for this holding (no ATR at entry, or TakeProfitATRMult<=0).
type Holding struct {
	Shares, AvgCost, Stop, Peak, Target float64
	EntryDate                           string
	PartialExited                       bool // true once §8.4②'s one partial exit has fired for this holding, so a still-climbing Peak doesn't sell it again
}

// Account is one book's live state — either a backtest replay's running
// position or the live paper account loaded from paper.db.
type Account struct {
	Cash     float64
	Holdings map[string]Holding
}

// NewAccount starts an account with cash and no positions.
func NewAccount(cash float64) *Account {
	return &Account{Cash: cash, Holdings: make(map[string]Holding)}
}

// Trade is one executed BUY or SELL, shaped to convert 1:1 into a
// db.Transaction (Ticker/Side/Shares/Price/Fee/Date/RealizedPnL/StopPrice
// line up by name) so internal/bot/paper.go's persistPaperTrade and
// cmd/server/backtest.go's CSV writer don't need a translation layer. Reason is
// "llm_buy"/"llm_sell"/"stop"/"target"/"trailing" — the exit-reason breakdown backtest
// reports and the web dashboard both key off it (the web side re-derives an
// equivalent from the persisted stop_price snapshot instead, see
// docs/phase-11-paper-account.md §7.1).
type Trade struct {
	Date, Ticker, Side                    string
	Shares, Price, Fee, RealizedPnL, Stop float64
	Reason                                string
}

// twMinFee is TWSE brokers' standard commission floor — most brokers charge
// a flat NT$20 on any fill whose 0.1425%*discount commission would otherwise
// round below it. The 0.3% securities transaction tax (sell side) is a
// statutory rate and never discounted or floored.
const twMinFee = 20.0

// feeRate returns the round-trip-relevant one-sided commission rate (tax
// excluded): 0 for US (commission-free assumption, matching the rest of this
// codebase), and Taiwan's brokerage rate net of discount otherwise. discount
// is a broker's fraction of the statutory 0.1425% commission (1.0 = no
// discount, e.g. 0.28 for a 72%-off e-brokerage plan) — the securities
// transaction tax is not a commission and is applied separately in FeeFor.
func feeRate(m market.MarketID, discount float64) float64 {
	if m != market.TW {
		return 0
	}
	return 0.001425 * discount
}

// FeeFor is the dollar fee for one fill of notional value on side ("BUY" or
// "SELL") in market m, with a broker commission discount (1.0 = none; unused
// for US). TW sells additionally pay the 0.3% securities transaction tax
// (never discounted), and any TW commission below twMinFee is raised to it
// before the tax is added — the NT$20 floor is a brokerage commission floor,
// not a floor on commission+tax combined, so it must apply before the tax
// leg is added or a small sell's fee is underestimated. ponytail: discount
// and twMinFee are broker-specific (vary by plan); the 0.1425%/0.3% rates
// are statutory and shouldn't move with them — keep that split if this ever
// grows a per-broker fee table.
func FeeFor(m market.MarketID, side string, notional, discount float64) float64 {
	if m != market.TW {
		return 0
	}
	commission := notional * feeRate(m, discount)
	if commission < twMinFee {
		commission = twMinFee
	}
	if side == "SELL" {
		commission += notional * 0.003
	}
	return commission
}

// SuggestShares is internal/bot/pipeline.go's sizing formula, moved here so
// the live pipeline's display-only sizing line and this engine's actual
// order sizing can never drift into two implementations (pipeline.go now
// calls this instead — see docs/phase-11-paper-account.md §4.2). Risk-based:
// shares such that a stop-out loses at most equity*riskPct/100.
func SuggestShares(equity, riskPct, price, stop float64) int {
	if equity <= 0 || riskPct <= 0 || price <= 0 || stop <= 0 || stop >= price {
		return 0
	}
	riskBudget := equity * riskPct / 100
	perShareRisk := price - stop
	shares := int(riskBudget / perShareRisk)
	if shares < 0 {
		return 0
	}
	return shares
}

// TrailingStopThreshold is internal/bot/jobs.go's trailing-stop distance
// formula, moved here for the same no-drift reason as SuggestShares — see
// jobs.go's checkTrailingStopAlerts, which now calls this instead. atrMult
// <=0 disables the ATR-based leg; when both legs are usable the tighter one
// wins (see the original doc comment this carries forward).
func TrailingStopThreshold(fixedPct, atrMult, atr, peak float64) (thresholdPct float64, atrBased, ok bool) {
	atrPct := 0.0
	atrOK := atrMult > 0 && atr > 0 && peak > 0
	if atrOK {
		atrPct = atrMult * atr / peak * 100
	}

	switch {
	case fixedPct > 0 && atrOK:
		if atrPct < fixedPct {
			return atrPct, true, true
		}
		return fixedPct, false, true
	case fixedPct > 0:
		return fixedPct, false, true
	case atrOK:
		return atrPct, true, true
	default:
		return 0, false, false
	}
}

// Equity is cash plus the mark-to-market value of every holding. A holding
// missing from prices (or priced <=0) falls back to its AvgCost — same
// degrade-by-omission convention used throughout this codebase (e.g.
// buildSizingLines) rather than treating a missing quote as zero value.
func (a *Account) Equity(prices map[string]float64) float64 {
	equity := a.Cash
	for t, h := range a.Holdings {
		if p, ok := prices[t]; ok && p > 0 {
			equity += p * h.Shares
		} else {
			equity += h.AvgCost * h.Shares
		}
	}
	return equity
}

// ApplySignal executes one BUY/SELL recommendation (HOLD/"" is a no-op) per
// the rules in docs/phase-11-paper-account.md §4.1: SELL closes a held
// position in full (no-op if not held — this engine never shorts); BUY on an
// already-held ticker is skipped (see the doc's rationale — the daily report
// re-recommends BUY on every still-bullish watchlist ticker, so "skip, don't
// add" is what keeps position size meaningful). ok is false for every no-op
// case, including a BUY sized down to 0 shares by insufficient cash — a
// caller that pre-filters already-held BUYs (as both PR2 and PR3 do) can
// therefore treat a false return as "shares were unaffordable" without extra
// bookkeeping in this package.
//
// Equity for sizing is computed from just this ticker's live price (other
// holdings value at cost) — Equity(map[string]float64{s.Ticker: price}) —
// since ApplySignal only ever receives one ticker's price/atr, not a full
// market snapshot; the AvgCost fallback for other holdings is the same
// degrade-by-omission Equity always uses.
func (a *Account) ApplySignal(s Signal, price, atr float64, cfg Config) (Trade, bool) {
	switch s.Action {
	case "SELL":
		h, held := a.Holdings[s.Ticker]
		if !held {
			return Trade{}, false
		}
		return a.sell(s.Date, s.Ticker, h, price, cfg, "llm_sell"), true
	case "BUY":
		return a.buy(s, price, atr, cfg)
	default:
		return Trade{}, false
	}
}

func (a *Account) buy(s Signal, price, atr float64, cfg Config) (Trade, bool) {
	if _, held := a.Holdings[s.Ticker]; held {
		return Trade{}, false
	}
	if price <= 0 {
		return Trade{}, false
	}

	stop := price - cfg.StopATRMult*atr
	if atr <= 0 {
		stop = price * (1 - cfg.StopLossPct/100)
	}

	// Target has no fixed-%% fallback (unlike stop's StopLossPct) — its
	// whole meaning is an R-multiple off ATR, and without ATR there's no R
	// to be a multiple of, so it's simply disabled for that entry.
	var target float64
	if atr > 0 && cfg.TakeProfitATRMult > 0 {
		target = price + cfg.TakeProfitATRMult*atr
	}

	equity := a.Equity(map[string]float64{s.Ticker: price})
	shares := SuggestShares(equity, cfg.RiskPct, price, stop)

	if cfg.MaxPositionPct > 0 {
		if cap := int(equity * cfg.MaxPositionPct / 100 / price); cap < shares {
			shares = cap
		}
	}
	// Cash cap must budget for FeeFor's twMinFee floor, not just the
	// percentage rate — a naive price*(1+feeRate) estimate underbudgets any
	// TW fill small enough that the percentage fee would round below
	// twMinFee, which can push a.Cash negative once FeeFor's actual floor
	// applies. minFeeBudget is 0 for US, matching FeeFor's own 0 there.
	minFeeBudget := 0.0
	if cfg.Market == market.TW {
		minFeeBudget = twMinFee
	}
	if cap := int((a.Cash - minFeeBudget) / (price * (1 + feeRate(cfg.Market, cfg.FeeDiscount)))); cap < shares {
		shares = cap
	}
	if shares < 1 {
		return Trade{}, false
	}

	notional := price * float64(shares)
	fee := FeeFor(cfg.Market, "BUY", notional, cfg.FeeDiscount)
	a.Cash -= notional + fee
	a.Holdings[s.Ticker] = Holding{
		Shares:    float64(shares),
		AvgCost:   price,
		Stop:      stop,
		Peak:      price,
		Target:    target,
		EntryDate: s.Date,
	}
	return Trade{
		Date: s.Date, Ticker: s.Ticker, Side: "BUY",
		Shares: float64(shares), Price: price, Fee: fee, Stop: stop,
		Reason: "llm_buy",
	}, true
}

func (a *Account) sell(date, ticker string, h Holding, price float64, cfg Config, reason string) Trade {
	notional := price * h.Shares
	fee := FeeFor(cfg.Market, "SELL", notional, cfg.FeeDiscount)
	proceeds := notional - fee
	realized := proceeds - h.AvgCost*h.Shares
	a.Cash += proceeds
	delete(a.Holdings, ticker)
	return Trade{
		Date: date, Ticker: ticker, Side: "SELL",
		Shares: h.Shares, Price: price, Fee: fee, RealizedPnL: realized, Stop: h.Stop,
		Reason: reason,
	}
}

// partialSell is §8.4②'s "1R out half" — unlike sell, it leaves the position
// open (mutates h in place, doesn't touch a.Holdings/delete) so the caller
// can keep running the remainder under the normal stop/trailing rules.
func (a *Account) partialSell(date, ticker string, h *Holding, sellShares, price float64, cfg Config) Trade {
	notional := price * sellShares
	fee := FeeFor(cfg.Market, "SELL", notional, cfg.FeeDiscount)
	proceeds := notional - fee
	realized := proceeds - h.AvgCost*sellShares
	a.Cash += proceeds
	h.Shares -= sellShares
	return Trade{
		Date: date, Ticker: ticker, Side: "SELL",
		Shares: sellShares, Price: price, Fee: fee, RealizedPnL: realized, Stop: h.Stop,
		Reason: "partial_target",
	}
}

// MarkClose settles every open holding against date's closing prices before
// that day's signals are applied (docs/phase-11-paper-account.md §4.3/§5.1:
// "yesterday's positions face today's stop first"). A holding missing from
// closes is left untouched (no price to judge it against, same
// degrade-by-omission convention as elsewhere). Order: fixed stop first
// (close <= Stop, exits at the close — ponytail: no gap-down simulation,
// switch to daily Low if that matters more than simplicity), then target,
// then peak update, then §8.4②③'s breakeven-stop/partial-exit overlays
// (both off by default), then the trailing-stop distance
// (TrailingStopThreshold). A FULL exit (stop/target/trailing) can only
// happen once per call, same as before §8.4②; a PartialExitAtR fill does not
// `continue`, so on the same day it can be followed by a trailing exit on
// what's left of the position — two trades, one holding, one call. Iterates
// tickers in sorted order purely for deterministic output (trade order never
// affects any one ticker's outcome, since each ticker's exit depends only on
// its own state).
func (a *Account) MarkClose(date string, closes, atrs map[string]float64, cfg Config) []Trade {
	tickers := make([]string, 0, len(a.Holdings))
	for t := range a.Holdings {
		tickers = append(tickers, t)
	}
	sort.Strings(tickers)

	var trades []Trade
	for _, t := range tickers {
		h := a.Holdings[t]
		close, ok := closes[t]
		if !ok || close <= 0 {
			continue
		}

		if close <= h.Stop {
			trades = append(trades, a.sell(date, t, h, close, cfg, "stop"))
			continue
		}

		if h.Target > 0 && close >= h.Target {
			trades = append(trades, a.sell(date, t, h, close, cfg, "target"))
			continue
		}

		if close > h.Peak {
			h.Peak = close
		}

		// §8.4②③: both read R off the Stop as it stood at entry, before
		// either overlay might move it — computed once here so PartialExitAtR
		// doesn't see a degenerate R=0 on a day BreakevenAtR has already
		// moved Stop up to AvgCost.
		r := h.AvgCost - h.Stop
		if cfg.BreakevenAtR > 0 && r > 0 && h.Stop < h.AvgCost && h.Peak >= h.AvgCost+cfg.BreakevenAtR*r {
			h.Stop = h.AvgCost
		}
		if cfg.PartialExitAtR > 0 && !h.PartialExited && r > 0 && h.Peak >= h.AvgCost+cfg.PartialExitAtR*r {
			if half := math.Trunc(h.Shares / 2); half >= 1 {
				trades = append(trades, a.partialSell(date, t, &h, half, close, cfg))
				h.PartialExited = true
				h.Target = 0
			}
		}

		if thresholdPct, _, ok := TrailingStopThreshold(cfg.TrailingPct, cfg.TrailingATRMult, atrs[t], h.Peak); ok {
			drawdownPct := (h.Peak - close) / h.Peak * 100
			if drawdownPct >= thresholdPct {
				trades = append(trades, a.sell(date, t, h, close, cfg, "trailing"))
				continue
			}
		}
		a.Holdings[t] = h
	}
	return trades
}
