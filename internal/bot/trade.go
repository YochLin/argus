package bot

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/logger"
	"argus/internal/service"
)

// tradeDateRe matches an optional trailing YYYY-MM-DD date argument to
// /buy and /sell, for backdating a trade entered after the fact (e.g.
// migrating cost basis from a broker/spreadsheet) instead of recording it
// under today's date.
var tradeDateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// parseTradeArgs parses the "<ticker> <shares> <price> [fee] [date]"
// arguments shared by /buy and /sell. fee and date are both optional and
// order-independent relative to each other (distinguished by shape: a
// YYYY-MM-DD token is the date, any other numeric token is the fee), so
// "10 200 1.5 2026-01-15" and "10 200 2026-01-15" (no fee) both parse. date
// is returned as "" when omitted; the caller defaults that to today.
// feeSet distinguishes "no fee argument typed" (auto-calculate via
// paper.FeeFor) from "explicitly typed 0" (Phase 13 §3.2) — a plain float64
// can't tell those apart since both are 0.
func parseTradeArgs(args string) (ticker string, shares, price, fee float64, feeSet bool, date string, err error) {
	fields := strings.Fields(args)
	if len(fields) < 3 || len(fields) > 5 {
		return "", 0, 0, 0, false, "", fmt.Errorf("expected <ticker> <shares> <price> [fee] [date]")
	}
	ticker = strings.ToUpper(fields[0])
	if shares, err = strconv.ParseFloat(fields[1], 64); err != nil || shares <= 0 {
		return "", 0, 0, 0, false, "", fmt.Errorf("invalid shares %q", fields[1])
	}
	if price, err = strconv.ParseFloat(fields[2], 64); err != nil || price <= 0 {
		return "", 0, 0, 0, false, "", fmt.Errorf("invalid price %q", fields[2])
	}

	for _, f := range fields[3:] {
		if tradeDateRe.MatchString(f) {
			if date != "" {
				return "", 0, 0, 0, false, "", fmt.Errorf("duplicate date %q", f)
			}
			if _, perr := time.Parse("2006-01-02", f); perr != nil {
				return "", 0, 0, 0, false, "", fmt.Errorf("invalid date %q", f)
			}
			date = f
			continue
		}
		if feeSet {
			return "", 0, 0, 0, false, "", fmt.Errorf("unexpected argument %q", f)
		}
		if fee, err = strconv.ParseFloat(f, 64); err != nil || fee < 0 {
			return "", 0, 0, 0, false, "", fmt.Errorf("invalid fee %q", f)
		}
		feeSet = true
	}
	return ticker, shares, price, fee, feeSet, date, nil
}

// handleBuy records a purchase and folds it into the ticker's position
// (weighted-average cost). The ticker is also added to the watchlist —
// see the "持倉自動納入 watchlist" PLAN.md item — so a bought position is
// never silently unmonitored. An explicit date backdates the trade (for
// migrating historical cost basis); note weighted-average cost is
// order-independent for buys, but backdated sells should still be entered
// oldest-first so realized P&L is computed against the cost basis as it
// actually stood at the time.
func (b *Bot) handleBuy(args string) {
	ticker, shares, price, fee, feeSet, date, err := parseTradeArgs(args)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyBuyUsage))
		return
	}
	if date == "" {
		date = todayDate()
	}
	feeAuto := !feeSet
	msg, _ := b.recordBuy(ticker, shares, price, fee, feeAuto, date, "")
	b.Send(msg)
}

// recordBuy is handleBuy's core, pulled out so a confirmed Phase 4
// pending-action proposal (record_buy, see internal/mcptools'
// trade_write_tools.go) can execute the exact same logic and produce the
// exact same confirmation text as typing /buy directly — see
// executePendingAction. err is nil on success, purely for a caller that
// needs to distinguish success/failure programmatically (ExecuteBuy, see
// docs/phase-10-web-trade-input.md §4.2) without re-parsing msg's already
// i18n-rendered text. extID is "" for every caller except Phase 16's
// Shioaji-synced pending actions — see db.RecordBuyExt.
func (b *Bot) recordBuy(ticker string, shares, price, fee float64, feeAuto bool, date, extID string) (string, error) {
	// Read any stop price already on the position before the buy — the trade
	// service deliberately doesn't touch it, but buyStopSuggestion's add-on
	// note needs to know it was there.
	//
	// Read this before executing because a buy that tops up an existing
	// position deliberately keeps its existing stop price.
	var existingStopPrice float64
	if prevPos, ok, err := b.db.GetPosition(ticker); err != nil {
		logger.Errorf("buy %s: get existing position: %v", ticker, err)
	} else if ok {
		existingStopPrice = prevPos.StopPrice
	}

	trades := b.trading()
	if trades == nil {
		err := errors.New("trade service unavailable")
		return i18n.T(b.lang, i18n.KeyBuyFailed, err), err
	}
	var feeInput *float64
	if !feeAuto {
		feeInput = &fee
	}
	result, err := trades.Buy(service.BuyInput{
		Ticker: ticker,
		Shares: shares,
		Price:  price,
		Fee:    feeInput,
		Date:   date,
		ExtID:  extID,
	})
	if err != nil {
		return i18n.T(b.lang, i18n.KeyBuyFailed, err), err
	}
	b.adjustCash(result.Ticker, -(shares*price + result.Fee))
	go b.captureFillNews(result.Ticker, date)
	msg := i18n.T(b.lang, i18n.KeyBuySuccess, result.Ticker, shares, b.money(result.Ticker, price), b.money(result.Ticker, result.Fee), result.Position.Shares, b.money(result.Ticker, result.Position.AvgCost))
	if feeAuto {
		msg += i18n.T(b.lang, i18n.KeyFeeAutoNote)
	}
	msg += b.thesisNudge(result.Ticker)
	return msg + b.buyStopSuggestion(result.Ticker, existingStopPrice), nil
}

// ExecuteBuy is ExecuteSell/ExecuteSetStop's sibling: internal/web's POST
// /api/trade/buy calls this instead of db.RecordBuy directly, so a web
// order gets the exact same watchlist auto-add /buy itself produces (see
// docs/phase-10-web-trade-input.md §4.2's "one write path" rule).
// Validation mirrors parseTradeArgs' semantics; date is expected already
// resolved (defaulted to today) by the caller, the same "date defaults in
// the handler, not the shared function" convention parseTradeArgs' own
// callers follow. Unlike before Phase 24 tech debt 3, this no longer pushes
// a Telegram confirmation itself — the caller decides whether to notify (see
// Notify), so a web trade's Telegram push is now internal/web's explicit
// choice rather than baked into every Execute* call.
func (b *Bot) ExecuteBuy(ticker string, shares, price float64, fee *float64, date string) (string, error) {
	ticker = normalizeTicker(ticker)
	if ticker == "" || shares <= 0 || price <= 0 || (fee != nil && *fee < 0) {
		msg := i18n.T(b.lang, i18n.KeyBuyUsage)
		return msg, fmt.Errorf("invalid buy arguments")
	}
	feeAuto := fee == nil
	feeVal := 0.0
	if !feeAuto {
		feeVal = *fee
	}
	return b.recordBuy(ticker, shares, price, feeVal, feeAuto, date, "")
}

// Notify pushes msg to Telegram on the caller's behalf. It exists so
// internal/web's write handlers (ExecuteBuy/ExecuteSell/ExecuteSetStop
// callers) can still surface the same confirmation Telegram would show for
// the equivalent slash command — the "Telegram 照常同步" decision from
// docs/phase-10-web-trade-input.md §4.2 — without that push being an
// unconditional side effect of Execute* itself (Phase 24 tech debt 3).
func (b *Bot) Notify(msg string) {
	b.Send(msg)
}

// thesisNudge returns a one-line nudge to record a holding thesis when
// ticker doesn't have one yet, or "" when it already does (or the lookup
// fails — a nudge is a courtesy, not worth failing the trade confirmation
// over). Called only from handleBuy, never blocking the trade itself — see
// PLAN.md's Phase 3.6 expansion "論點日誌" item.
func (b *Bot) thesisNudge(ticker string) string {
	_, ok, err := service.CurrentThesis(b.db, ticker)
	if err != nil {
		logger.Errorf("buy: check thesis %s: %v", ticker, err)
		return ""
	}
	if ok {
		return ""
	}
	return i18n.T(b.lang, i18n.KeyBuyThesisNudge, ticker, ticker)
}

// handleSell records a sale against an existing position and reports the
// realized P&L. It does not remove the ticker from the watchlist even when
// the position is fully closed out — the user may still want to watch it.
// When the sell fully closes the position, it kicks off Phase 3.8's
// sell-review (reviewClosedTrade) as a second message after the success
// reply — in its own goroutine so the (slow) LLM call never delays the
// immediate confirmation, and log-only on failure since the user already has
// their sell confirmation and doesn't need a second alert about the review
// itself failing.
func (b *Bot) handleSell(ctx context.Context, args string) {
	ticker, shares, price, fee, feeSet, date, err := parseTradeArgs(args)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeySellUsage))
		return
	}
	if date == "" {
		date = todayDate()
	}
	feeAuto := !feeSet
	msg, closed, stopPrice, _ := b.recordSell(ticker, shares, price, fee, feeAuto, date, "")
	b.Send(msg)
	if closed {
		go b.reviewClosedTrade(ctx, ticker, stopPrice)
	}
}

// recordSell is handleSell's core, pulled out for the same reason as
// recordBuy — a confirmed Phase 4 pending-action proposal (record_sell)
// reuses this instead of duplicating the RecordSell call and error mapping.
// closed reports whether this sell fully closed out the position (shares
// returned to 0), so callers can decide whether to trigger a sell-review;
// it's always false on an error path, since nothing was recorded. stopPrice
// is the position's stop price as it stood right before this sell (Phase
// 3.11 PR1 §3.5) — read via GetPosition *before* calling db.RecordSell,
// since a full close deletes the positions row and takes the stop price
// with it; always 0 on an error path or when no stop had ever been set. err
// is nil on success, same "for a programmatic caller, not for re-parsing
// msg" purpose as recordBuy's (see ExecuteSell). extID is "" for every
// caller except Phase 16's Shioaji-synced pending actions.
func (b *Bot) recordSell(ticker string, shares, price, fee float64, feeAuto bool, date, extID string) (msg string, closed bool, stopPrice float64, err error) {
	if prevPos, ok, gerr := b.db.GetPosition(ticker); gerr != nil {
		logger.Errorf("sell %s: get position for stop price: %v", ticker, gerr)
	} else if ok {
		stopPrice = prevPos.StopPrice
	}

	trades := b.trading()
	if trades == nil {
		err = errors.New("trade service unavailable")
		return i18n.T(b.lang, i18n.KeySellFailed, err), false, 0, err
	}
	var feeInput *float64
	if !feeAuto {
		feeInput = &fee
	}
	result, err := trades.Sell(service.SellInput{
		Ticker: ticker,
		Shares: shares,
		Price:  price,
		Fee:    feeInput,
		Date:   date,
		ExtID:  extID,
	})
	if err != nil {
		switch {
		case errors.Is(err, db.ErrNoPosition):
			return i18n.T(b.lang, i18n.KeySellNoPosition, ticker), false, 0, err
		case errors.Is(err, db.ErrInsufficientShares):
			return i18n.T(b.lang, i18n.KeySellInsufficientShares, ticker), false, 0, err
		default:
			return i18n.T(b.lang, i18n.KeySellFailed, err), false, 0, err
		}
	}
	b.adjustCash(result.Ticker, shares*price-result.Fee)
	go b.captureFillNews(result.Ticker, date)
	msg = i18n.T(b.lang, i18n.KeySellSuccess, result.Ticker, shares, b.money(result.Ticker, price), b.money(result.Ticker, result.Fee), result.RealizedPnL, result.Position.Shares)
	if feeAuto {
		msg += i18n.T(b.lang, i18n.KeyFeeAutoNote)
	}
	return msg, result.Closed, result.StopPrice, nil
}

// ExecuteSell is ExecuteBuy's sibling for internal/web's POST
// /api/trade/sell (see docs/phase-10-web-trade-input.md §4.2) — same
// validation shape (Telegram parity now via Notify, not built in — see
// ExecuteBuy), plus triggering the same sell-review goroutine handleSell
// kicks off on a closing sell.
func (b *Bot) ExecuteSell(ctx context.Context, ticker string, shares, price float64, fee *float64, date string) (string, error) {
	ticker = normalizeTicker(ticker)
	if ticker == "" || shares <= 0 || price <= 0 || (fee != nil && *fee < 0) {
		msg := i18n.T(b.lang, i18n.KeySellUsage)
		return msg, fmt.Errorf("invalid sell arguments")
	}
	feeAuto := fee == nil
	feeVal := 0.0
	if !feeAuto {
		feeVal = *fee
	}
	msg, closed, stopPrice, err := b.recordSell(ticker, shares, price, feeVal, feeAuto, date, "")
	if closed {
		go b.reviewClosedTrade(ctx, ticker, stopPrice)
	}
	return msg, err
}

// handleUndo backs /undo <ticker> — the typo-recovery path: it looks up
// ticker's most recent transaction (GetTransactions is already date,id
// ascending, so the last element is it) and deletes it via undoTransaction.
// Anything older than the latest is out of scope by design — see
// db.DB.DeleteTransaction's doc comment.
func (b *Bot) handleUndo(args string) {
	ticker := normalizeTicker(args)
	if ticker == "" {
		b.Send(i18n.T(b.lang, i18n.KeyUndoUsage))
		return
	}
	txs, err := b.db.GetTransactions(ticker)
	if err != nil {
		logger.Errorf("undo %s: get transactions: %v", ticker, err)
		b.Send(i18n.T(b.lang, i18n.KeyUndoFailed, err))
		return
	}
	if len(txs) == 0 {
		b.Send(i18n.T(b.lang, i18n.KeyUndoNoTx, b.tickerLabel(ticker)))
		return
	}
	msg, _ := b.undoTransaction(txs[len(txs)-1].ID)
	b.Send(msg)
}

// undoTransaction is handleUndo's core, pulled out so ExecuteDeleteTransaction
// (internal/web's per-row delete button) produces the exact same
// confirmation text and cash reversal as typing /undo directly — same
// convention as recordBuy/recordSell. The cash delta undoes exactly what
// recordBuy/recordSell applied when the deleted transaction was first
// recorded (see their own adjustCash calls): a deleted BUY gives back
// shares*price+fee, a deleted SELL takes back shares*price-fee.
func (b *Bot) undoTransaction(id int64) (string, error) {
	trades := b.trading()
	if trades == nil {
		err := errors.New("trade service unavailable")
		return i18n.T(b.lang, i18n.KeyUndoFailed, err), err
	}
	t, err := trades.Undo(id)
	if err != nil {
		switch {
		case errors.Is(err, db.ErrTransactionNotFound):
			return i18n.T(b.lang, i18n.KeyUndoNotFound), err
		case errors.Is(err, db.ErrNotLatestTransaction):
			return i18n.T(b.lang, i18n.KeyUndoNotLatest), err
		default:
			return i18n.T(b.lang, i18n.KeyUndoFailed, err), err
		}
	}

	var cashDelta float64
	if t.Side == "BUY" {
		cashDelta = t.Shares*t.Price + t.Fee
	} else {
		cashDelta = -(t.Shares*t.Price - t.Fee)
	}
	b.adjustCash(t.Ticker, cashDelta)

	if t.Side == "BUY" {
		return i18n.T(b.lang, i18n.KeyUndoBuySuccess, b.tickerLabel(t.Ticker), t.Shares, b.money(t.Ticker, t.Price)), nil
	}
	return i18n.T(b.lang, i18n.KeyUndoSellSuccess, b.tickerLabel(t.Ticker), t.Shares, b.money(t.Ticker, t.Price)), nil
}

// ExecuteDeleteTransaction is ExecuteBuy/ExecuteSell's sibling for
// internal/web's POST /api/trade/delete — the dashboard's per-row delete
// button, which already knows the specific transaction id (unlike /undo,
// which resolves it from a ticker).
func (b *Bot) ExecuteDeleteTransaction(id int64) (string, error) {
	return b.undoTransaction(id)
}

// handleThesis manages the holding thesis: "/thesis TICKER" alone queries
// the current round's recorded rationale, "/thesis TICKER free text"
// overwrites it wholesale (Phase 27 P1b: one text per round — the open
// round's, or a pre-buy draft when the ticker isn't held; see
// service.CurrentThesis). Deliberately fed only into /insight
// (see handleInsight's loadTheses call) — never /recommend, so the model
// challenges the user's stated thesis instead of confirming it.
func (b *Bot) handleThesis(args string) {
	args = strings.TrimSpace(args)
	if args == "" {
		b.Send(i18n.T(b.lang, i18n.KeyThesisUsage))
		return
	}

	parts := strings.SplitN(args, " ", 2)
	ticker := normalizeTicker(parts[0])
	if ticker == "" {
		b.Send(i18n.T(b.lang, i18n.KeyThesisUsage))
		return
	}

	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		thesis, ok, err := service.CurrentThesis(b.db, ticker)
		if err != nil {
			b.Send(i18n.T(b.lang, i18n.KeyQueryFailed, err))
			return
		}
		if !ok {
			b.Send(i18n.T(b.lang, i18n.KeyThesisNotSet, b.tickerLabel(ticker), ticker))
			return
		}
		b.Send(i18n.T(b.lang, i18n.KeyThesisCurrent, b.tickerLabel(ticker), thesis))
		return
	}

	thesis := strings.TrimSpace(parts[1])
	if err := service.SetCurrentThesis(b.db, ticker, thesis); err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyThesisSetFailed, b.tickerLabel(ticker), err))
		return
	}
	b.Send(i18n.T(b.lang, i18n.KeyThesisSetSuccess, b.tickerLabel(ticker), thesis))
}
