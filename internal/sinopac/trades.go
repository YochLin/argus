package sinopac

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// Trade is one reconstructed manual buy/sell event, ready to seed a
// pending_action. ExtID is Shioaji's own per-deal dseq — the real
// idempotency key (see internal/db migration 17's ext_id column) —
// falling back to a synthesized key (see Trades) when dseq is empty.
type Trade struct {
	ExtID     string
	Date      string
	Ticker    string
	Side      string // "BUY" or "SELL"
	Shares    float64
	Price     float64
	Synthetic bool // true when ExtID was synthesized, not a real dseq
}

// Trades reconstructs today's buy/sell events for auto-bookkeeping. Only
// today's: position_detail can't describe an older buy (see below), and an
// earlier day's trades are the ones the user has most likely already
// entered by hand — those carry no ext_id, so dedup can't catch them.
//
// Sells come from pnl (profit_loss with unit=Share), already per-share.
//
// Buys come from today's position_detail lots, whose fields are NOT what
// their names say (live-verified 2026-10-15): quantity is in 張, so an odd
// lot reads 0, and price is the lot's remaining cost including fee, not a
// per-share price. Today's bought shares are instead the position's
// quantity minus yd_quantity (yesterday's), plus whatever sold today; the
// per-share price is the lots' cost minus fee over the shares still held
// from them (sells consume the oldest lots first, so that's at most the
// whole position). Several of today's lots in one ticker become one Trade.
// A buy fully sold the same day leaves no position and is missed — an
// accepted gap for swing trading rather than day-trading reconciliation.
//
// skip excludes tickers the caller knows are periodic/定期定額 investment
// purchases (SINOPAC_SKIP_TICKERS), which this deliberately does not try to
// detect heuristically — the user already knows which tickers those are.
//
// A row with an empty dseq falls back to a synthesized key
// (date+ticker+side+shares+price) so it can still be deduped against
// transactions.ext_id, tagged Synthetic so callers can log it as
// lower-confidence.
func Trades(positions []Position, details []PositionDetail, pnl []ProfitLoss, skip map[string]bool, today string) []Trade {
	var out []Trade
	soldToday := map[string]int{}
	for _, p := range pnl {
		if skip[p.Code] || p.Date != today {
			continue
		}
		soldToday[p.Code] += p.Quantity
		out = append(out, newTrade(p.Date, p.Code, "SELL", float64(p.Quantity), p.Price, p.Dseq))
	}

	type lots struct {
		cost  float64
		dseqs []string
	}
	todayLots := map[string]*lots{}
	for _, d := range details {
		if skip[d.Code] || d.Date != today {
			continue
		}
		l := todayLots[d.Code]
		if l == nil {
			l = &lots{}
			todayLots[d.Code] = l
		}
		l.cost += d.Price - d.Fee
		l.dseqs = append(l.dseqs, d.Dseq)
	}
	for _, p := range positions {
		l := todayLots[p.Code]
		if l == nil {
			continue
		}
		bought := p.Quantity - p.YdQuantity + soldToday[p.Code]
		if bought <= 0 {
			continue
		}
		dseq := strings.Join(l.dseqs, "+")
		if slices.Contains(l.dseqs, "") {
			dseq = ""
		}
		out = append(out, newTrade(today, p.Code, "BUY", float64(bought), l.cost/float64(min(bought, p.Quantity)), dseq))
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Ticker < out[j].Ticker })
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
