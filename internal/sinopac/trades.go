package sinopac

import (
	"fmt"
	"sort"
)

// Trade is one reconstructed manual buy/sell event, ready to seed a
// pending_action. ExtID is Shioaji's own per-deal dseq — the real
// idempotency key (see internal/db migration 17's ext_id column) —
// falling back to a synthesized key (see Trades) when dseq is empty.
//
// Price's per-share-vs-total-amount semantics are unconfirmed against a
// real brokerage statement (live testing showed a value that looks more
// like a total trade amount than a per-share price for at least one
// sample) — Price is passed through here exactly as the API returns it.
// Do not build a pending_action from this without first reconciling one
// sync's dry-run output against the statement; see internal/bot/sinopac.go.
type Trade struct {
	ExtID     string
	Date      string
	Ticker    string
	Side      string // "BUY" or "SELL"
	Shares    float64
	Price     float64
	Synthetic bool // true when ExtID was synthesized, not a real dseq
}

// Trades reconstructs individual buy/sell events for auto-bookkeeping.
// Buys come from still-open position lots (details, one row per
// still-open lot with direction "Buy") — a lot that's since been fully
// re-sold within the lookback window won't appear there, an accepted gap
// for swing/position trading rather than day-trading reconciliation.
// Sells come from realized sells (pnl). skip excludes tickers the caller
// knows are periodic/定期定額 investment purchases (SINOPAC_SKIP_TICKERS),
// which this deliberately does not try to detect heuristically — the user
// already knows which tickers those are.
//
// A row with an empty dseq (observed on the oldest history) falls back to
// a synthesized key (date+ticker+side+shares+price) so it can still be
// deduped against transactions.ext_id, tagged Synthetic so callers can log
// it as lower-confidence.
func Trades(details []PositionDetail, pnl []ProfitLoss, skip map[string]bool) []Trade {
	var out []Trade
	for _, d := range details {
		if skip[d.Code] {
			continue
		}
		out = append(out, newTrade(d.Date, d.Code, "BUY", float64(d.Quantity), d.Price, d.Dseq))
	}
	for _, p := range pnl {
		if skip[p.Code] {
			continue
		}
		out = append(out, newTrade(p.Date, p.Code, "SELL", float64(p.Quantity), p.Price, p.Dseq))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out
}

func newTrade(date, ticker, side string, shares, price float64, dseq string) Trade {
	t := Trade{ExtID: dseq, Date: date, Ticker: ticker, Side: side, Shares: shares, Price: price}
	if dseq == "" {
		t.ExtID = fmt.Sprintf("synthetic:%s:%s:%s:%g:%g", date, ticker, side, shares, price)
		t.Synthetic = true
	}
	return t
}
