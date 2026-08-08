// Package paper is the pure rule engine shared by the two Phase 11 paths —
// argus backtest's historical replay (cmd/bot/backtest.go) and the live
// paper account's forward accumulation (internal/bot/paper.go). Same
// "no DB/network/Telegram" discipline as internal/signals/internal/receval:
// Account.ApplySignal/MarkClose/Equity are the entire trading rulebook, fed
// plain values by both callers, so a divergence between backtest and live
// behavior is structurally impossible — see docs/phase-11-paper-account.md
// §1 for why that sharing is this package's only reason to exist.
package paper

import (
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
// cmd/bot/backtest.go's CSV writer don't need a translation layer. Reason is
// "llm_buy"/"llm_sell"/"stop"/"target"/"trailing" — the exit-reason breakdown backtest
// reports and the web dashboard both key off it (the web side re-derives an
// equivalent from the persisted stop_price snapshot instead, see
// docs/phase-11-paper-account.md §7.1).
type Trade struct {
	Date, Ticker, Side                    string
	Shares, Price, Fee, RealizedPnL, Stop float64
	Reason                                string
}

// feeRate returns the round-trip-relevant one-sided fee rate: 0 for US
// (commission-free assumption, matching the rest of this codebase), and
// Taiwan's brokerage/tax schedule otherwise. ponytail: flat statutory rates,
// no broker discount — promote to a Config field if that precision matters.
func feeRate(m market.MarketID, side string) float64 {
	if m != market.TW {
		return 0
	}
	if side == "SELL" {
		return 0.001425 + 0.003
	}
	return 0.001425
}

// FeeFor is the dollar fee for one fill of notional value on side ("BUY" or
// "SELL") in market m.
func FeeFor(m market.MarketID, side string, notional float64) float64 {
	return notional * feeRate(m, side)
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
	if cap := int(a.Cash / (price * (1 + feeRate(cfg.Market, "BUY")))); cap < shares {
		shares = cap
	}
	if shares < 1 {
		return Trade{}, false
	}

	notional := price * float64(shares)
	fee := FeeFor(cfg.Market, "BUY", notional)
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
	fee := FeeFor(cfg.Market, "SELL", notional)
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

// MarkClose settles every open holding against date's closing prices before
// that day's signals are applied (docs/phase-11-paper-account.md §4.3/§5.1:
// "yesterday's positions face today's stop first"). A holding missing from
// closes is left untouched (no price to judge it against, same
// degrade-by-omission convention as elsewhere). Order: fixed stop first
// (close <= Stop, exits at the close — ponytail: no gap-down simulation,
// switch to daily Low if that matters more than simplicity), then peak
// update, then the trailing-stop distance (TrailingStopThreshold); a holding
// can only exit once per call. Iterates tickers in sorted order purely for
// deterministic output (trade order never affects any one ticker's outcome,
// since each ticker's exit depends only on its own state).
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
