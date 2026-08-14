package bot

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/logger"
	"argus/internal/option"
)

// parseOptionTradeArgs parses /obuy's and /osell's
// "<OCC symbol> <contracts> <premium> [fee] [date]" arguments — same
// optional trailing fee/date shape as parseTradeArgs, but with an OCC
// symbol instead of a ticker and per-share premium instead of a share
// price. Deliberately doesn't accept a bare ticker + shorthand expiry —
// only a full OCC symbol (docs/phase-12-options.md's own worked examples
// never use anything else). ponytail: add a compact-strike shorthand if
// typing the full symbol out proves annoying in practice.
func parseOptionTradeArgs(args string) (symbol string, contracts, premium, fee float64, date string, err error) {
	fields := strings.Fields(args)
	if len(fields) < 3 || len(fields) > 5 {
		return "", 0, 0, 0, "", fmt.Errorf("expected <OCC symbol> <contracts> <premium> [fee] [date]")
	}
	symbol = strings.ToUpper(fields[0])
	if !option.IsOCC(symbol) {
		return "", 0, 0, 0, "", fmt.Errorf("invalid OCC symbol %q", fields[0])
	}
	if contracts, err = strconv.ParseFloat(fields[1], 64); err != nil || contracts <= 0 {
		return "", 0, 0, 0, "", fmt.Errorf("invalid contracts %q", fields[1])
	}
	if premium, err = strconv.ParseFloat(fields[2], 64); err != nil || premium < 0 {
		return "", 0, 0, 0, "", fmt.Errorf("invalid premium %q", fields[2])
	}

	feeSet := false
	for _, f := range fields[3:] {
		if tradeDateRe.MatchString(f) {
			if date != "" {
				return "", 0, 0, 0, "", fmt.Errorf("duplicate date %q", f)
			}
			if _, perr := time.Parse("2006-01-02", f); perr != nil {
				return "", 0, 0, 0, "", fmt.Errorf("invalid date %q", f)
			}
			date = f
			continue
		}
		if feeSet {
			return "", 0, 0, 0, "", fmt.Errorf("unexpected argument %q", f)
		}
		if fee, err = strconv.ParseFloat(f, 64); err != nil || fee < 0 {
			return "", 0, 0, 0, "", fmt.Errorf("invalid fee %q", f)
		}
		feeSet = true
	}
	return symbol, contracts, premium, fee, date, nil
}

func (b *Bot) handleOBuy(args string) {
	symbol, contracts, premium, fee, date, err := parseOptionTradeArgs(args)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyOptionUsage))
		return
	}
	if date == "" {
		date = todayDate()
	}
	b.Send(b.recordOption(symbol, "BUY", contracts, premium, fee, date))
}

func (b *Bot) handleOSell(args string) {
	symbol, contracts, premium, fee, date, err := parseOptionTradeArgs(args)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyOptionUsage))
		return
	}
	if date == "" {
		date = todayDate()
	}
	b.Send(b.recordOption(symbol, "SELL", contracts, premium, fee, date))
}

// recordOption is handleOBuy/handleOSell's shared core — see recordBuy's
// doc comment for why this split exists (the expiry-scan pending-action
// executor doesn't reuse this one, since ASSIGNED/EXPIRED/EXERCISED go
// through resolveOption instead).
func (b *Bot) recordOption(symbol, side string, contracts, premium, fee float64, date string) string {
	pos, realizedPnL, err := b.db.RecordOption(symbol, side, contracts, premium, fee, date)
	if err != nil {
		if errors.Is(err, db.ErrCrossesZero) {
			return i18n.T(b.lang, i18n.KeyOptionCrossesZero, symbol)
		}
		return i18n.T(b.lang, i18n.KeyOptionTradeFailed, err)
	}
	msg := i18n.T(b.lang, i18n.KeyOptionTradeSuccess, side, symbol, contracts, premium, fee, realizedPnL, pos.Contracts, pos.AvgPremium)
	if side == "SELL" {
		msg += b.nakedCallWarning(pos)
	}
	return msg
}

// nakedCallWarning fires the moment a /osell leaves the underlying with more
// short-call-locked shares than it actually holds — an uncapped-risk
// position the user may not have noticed they created (§3.5). "" when the
// position isn't a naked call (long, or a covered short) — the naked check
// only makes sense for calls; a short put's collateral is cash, not shares,
// and CSP collateral just needs a warning-free /risk display, not a
// write-time alert (nothing about it can go wrong silently the way naked
// share coverage can).
func (b *Bot) nakedCallWarning(pos db.OptionPosition) string {
	if pos.Right != "C" || pos.Contracts >= 0 {
		return ""
	}
	shortCallPositions, err := b.db.GetOptionPositionsByUnderlying(pos.Underlying)
	if err != nil {
		logger.Errorf("naked call check %s: %v", pos.Underlying, err)
		return ""
	}
	var lockedShares float64
	for _, p := range shortCallPositions {
		if p.Right == "C" && p.Contracts < 0 {
			lockedShares += -p.Contracts * float64(p.Multiplier)
		}
	}
	stockPos, ok, err := b.db.GetPosition(pos.Underlying)
	if err != nil {
		logger.Errorf("naked call check %s: get stock position: %v", pos.Underlying, err)
		return ""
	}
	held := 0.0
	if ok {
		held = stockPos.Shares
	}
	if lockedShares <= held {
		return ""
	}
	return i18n.T(b.lang, i18n.KeyOptionNakedCallWarning, pos.Underlying, lockedShares, held)
}

// parseOptionResolutionArgs parses /oassign's and /oexercise's
// "<OCC symbol> [date]" arguments.
func parseOptionResolutionArgs(args string) (symbol, date string, err error) {
	fields := strings.Fields(args)
	if len(fields) < 1 || len(fields) > 2 {
		return "", "", fmt.Errorf("expected <OCC symbol> [date]")
	}
	symbol = strings.ToUpper(fields[0])
	if !option.IsOCC(symbol) {
		return "", "", fmt.Errorf("invalid OCC symbol %q", fields[0])
	}
	if len(fields) == 2 {
		if !tradeDateRe.MatchString(fields[1]) {
			return "", "", fmt.Errorf("invalid date %q", fields[1])
		}
		if _, perr := time.Parse("2006-01-02", fields[1]); perr != nil {
			return "", "", fmt.Errorf("invalid date %q", fields[1])
		}
		date = fields[1]
	}
	return symbol, date, nil
}

func (b *Bot) handleOAssign(args string) {
	symbol, date, err := parseOptionResolutionArgs(args)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyOptionResolutionUsage))
		return
	}
	if date == "" {
		date = todayDate()
	}
	b.Send(b.resolveOption(symbol, date, db.OptionActionAssigned))
}

func (b *Bot) handleOExercise(args string) {
	symbol, date, err := parseOptionResolutionArgs(args)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyOptionResolutionUsage))
		return
	}
	if date == "" {
		date = todayDate()
	}
	b.Send(b.resolveOption(symbol, date, db.OptionActionExercised))
}

// resolveOption closes symbol's remaining position at premium 0 (the
// premium is either fully kept or fully forfeited, §3.4) and, for
// ASSIGNED/EXERCISED, generates the matching stock trade through the
// existing RecordBuy/RecordSell — no new stock-side write path, so
// positions/transactions/net_worth_snapshots stay exactly as trustworthy as
// they were before this feature existed. Also the pending-action executor
// for the daily expiry scan (jobs.go), which always resolves as
// EXPIRED/ASSIGNED — EXERCISED is manual-only (/oexercise), since a buyer
// choosing to exercise is a deliberate decision a scan can't infer.
func (b *Bot) resolveOption(symbol, date, action string) string {
	pos, ok, err := b.db.GetOptionPosition(symbol)
	if err != nil {
		return i18n.T(b.lang, i18n.KeyOptionResolveFailed, err)
	}
	if !ok {
		return i18n.T(b.lang, i18n.KeyOptionNoPosition, symbol)
	}
	if action == db.OptionActionAssigned && pos.Contracts >= 0 {
		return i18n.T(b.lang, i18n.KeyOptionAssignRequiresShort, symbol)
	}
	if action == db.OptionActionExercised && pos.Contracts <= 0 {
		return i18n.T(b.lang, i18n.KeyOptionExerciseRequiresLong, symbol)
	}

	_, realizedPnL, err := b.db.ResolveOption(symbol, action, date)
	if err != nil {
		return i18n.T(b.lang, i18n.KeyOptionResolveFailed, err)
	}
	msg := i18n.T(b.lang, i18n.KeyOptionResolveSuccess, action, symbol, realizedPnL)

	shares := math.Abs(pos.Contracts) * float64(pos.Multiplier)
	var stockMsg string
	switch {
	case action == db.OptionActionAssigned && pos.Right == "P": // short put assigned -> buy the stock
		stockMsg, _ = b.recordBuy(pos.Underlying, shares, pos.Strike, 0, false, date)
	case action == db.OptionActionAssigned && pos.Right == "C": // short call assigned -> sell the stock
		stockMsg, _, _, _ = b.recordSell(pos.Underlying, shares, pos.Strike, 0, false, date)
	case action == db.OptionActionExercised && pos.Right == "C": // long call exercised -> buy the stock at strike
		stockMsg, _ = b.recordBuy(pos.Underlying, shares, pos.Strike, 0, false, date)
	case action == db.OptionActionExercised && pos.Right == "P": // long put exercised -> sell the stock at strike
		stockMsg, _, _, _ = b.recordSell(pos.Underlying, shares, pos.Strike, 0, false, date)
	}
	if stockMsg != "" {
		msg += "\n" + stockMsg
	}
	return msg
}

// sendPortfolioOptionsSection renders /portfolio's options block — nothing
// at all (not even a title) when there are no open contracts, same
// no-dangling-empty-section convention as sendPortfolioSection.
func (b *Bot) sendPortfolioOptionsSection(positions []db.OptionPosition) {
	if len(positions) == 0 {
		return
	}
	b.Send(i18n.T(b.lang, i18n.KeyPortfolioOptionsSection))
	for _, p := range positions {
		mark, dte, err := b.optionMark(p)
		if err != nil {
			b.Send(i18n.T(b.lang, i18n.KeyPortfolioOptionUnavailable, p.ContractSymbol, err))
			continue
		}
		marketValue := mark * p.Contracts * float64(p.Multiplier)
		b.Send(i18n.T(b.lang, i18n.KeyPortfolioOptionLine, p.ContractSymbol, p.Contracts, p.AvgPremium, mark, marketValue, p.Expiry, dte))
	}
}

// optionMark fetches p's live chain and returns the mark price (option.Mark)
// and days-to-expiry for its specific contract. There's no single-contract
// quote endpoint — Yahoo only serves a whole expiry's chain — so this costs
// one request per distinct (underlying, expiry) position; fine at
// single-user, few-position scale (ponytail: cache per-request if
// /portfolio ever holds enough contracts to make that slow).
func (b *Bot) optionMark(p db.OptionPosition) (mark float64, dte int, err error) {
	if b.optionChain == nil {
		return 0, 0, fmt.Errorf("option chain provider unavailable")
	}
	expiry, err := time.Parse("2006-01-02", p.Expiry)
	if err != nil {
		return 0, 0, err
	}
	quotes, err := b.optionChain.GetOptionChain(p.Underlying, expiry)
	if err != nil {
		return 0, 0, err
	}
	for _, q := range quotes {
		if q.ContractSymbol == p.ContractSymbol {
			dte = int(math.Ceil(time.Until(expiry).Hours() / 24))
			return option.Mark(q.Bid, q.Ask, q.LastPrice), dte, nil
		}
	}
	return 0, 0, fmt.Errorf("contract not found in chain")
}

// optionExpiryPayload is the pending_actions.payload shape for
// db.PendingActionOptionExpiry — decoded by executePendingAction
// (pending_actions.go) when the user taps Confirm.
type optionExpiryPayload struct {
	Symbol         string `json:"symbol"`
	ProposedAction string `json:"proposed_action"`
	Date           string `json:"date"`
}

func decodeOptionExpiryPayload(payload string) (optionExpiryPayload, bool) {
	var p optionExpiryPayload
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return optionExpiryPayload{}, false
	}
	return p, true
}

// checkOptionExpiry finds option positions past expiry (date, the trading
// day RunClosingSnapshot just closed out) and proposes a resolution through
// Phase 4's pending-action confirm/reject flow — never resolved
// automatically (§3.4: an ITM expiry is an assignment/exercise, not a
// zero, and getting that wrong silently misrecords a whole leg's P&L).
// ITM/OTM is judged off the underlying's closing price (prices, falling
// back to a live quote via priceFor same as the rest of this job); a
// symbol whose underlying quote isn't available this run is simply
// skipped and picked up by tomorrow's scan instead of guessing.
func (b *Bot) checkOptionExpiry(prices map[string]float64, date string) {
	positions, err := b.db.GetOptionPositions()
	if err != nil {
		logger.Errorf("option expiry scan: positions: %v", err)
		return
	}
	if len(positions) == 0 {
		return
	}

	outstanding, err := b.outstandingOptionExpirySymbols()
	if err != nil {
		logger.Errorf("option expiry scan: outstanding proposals: %v", err)
		return
	}

	for _, p := range positions {
		if p.Expiry >= date || outstanding[p.ContractSymbol] {
			continue
		}
		spot, ok := b.priceFor(p.Underlying, prices)
		if !ok {
			continue
		}

		itm := (p.Right == "C" && spot > p.Strike) || (p.Right == "P" && spot < p.Strike)
		proposedAction := db.OptionActionExpired
		switch {
		case itm && p.Contracts < 0:
			proposedAction = db.OptionActionAssigned // short + ITM: the counterparty exercised against us
		case itm && p.Contracts > 0:
			proposedAction = db.OptionActionExercised // long + ITM: we're the one who'd exercise
		}

		payload, err := json.Marshal(optionExpiryPayload{Symbol: p.ContractSymbol, ProposedAction: proposedAction, Date: date})
		if err != nil {
			logger.Errorf("option expiry scan: marshal payload %s: %v", p.ContractSymbol, err)
			continue
		}
		id, err := b.db.CreatePendingAction(db.PendingActionOptionExpiry, string(payload))
		if err != nil {
			logger.Errorf("option expiry scan: create pending action %s: %v", p.ContractSymbol, err)
			continue
		}
		text := i18n.T(b.lang, i18n.KeyOptionExpiryConfirm, p.ContractSymbol, spot, p.Strike, proposedAction)
		if err := b.sendPendingActionConfirmation(id, text); err != nil {
			logger.Errorf("option expiry scan: send confirmation %s: %v", p.ContractSymbol, err)
			continue
		}
		if _, err := b.db.MarkPendingActionSent(id); err != nil {
			logger.Errorf("option expiry scan: mark sent %s: %v", p.ContractSymbol, err)
		}
	}
}

// outstandingOptionExpirySymbols returns the set of contract symbols that
// already have a "sent" (awaiting a Telegram tap) expiry proposal, so a
// daily rescan doesn't nag about the same contract every day until the user
// acts on it. Only "sent" counts, not "pending" — a row that somehow never
// made it to "sent" (e.g. a crash between the two steps) was never shown to
// the user, so it isn't really outstanding; tomorrow's scan creating a
// fresh one is harmless.
func (b *Bot) outstandingOptionExpirySymbols() (map[string]bool, error) {
	actions, err := b.db.GetPendingActionsByStatus(db.PendingActionStatusSent)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool)
	for _, a := range actions {
		if a.ActionType != db.PendingActionOptionExpiry {
			continue
		}
		if p, ok := decodeOptionExpiryPayload(a.Payload); ok {
			out[p.Symbol] = true
		}
	}
	return out, nil
}

// recordDailyATMIV snapshots one ATM-ish implied volatility reading per
// watchlist ticker that has open option interest available — see migration
// 15's doc comment for why this starts accumulating now despite having no
// reader for 6-12 months. Scoped to tickers (the US watchlist
// RunClosingSnapshot already fetched quotes for) rather than every
// optionable US stock: this is for tickers the user is actually watching,
// and each one costs a live chain request. Silent, best-effort per ticker —
// same "never block the closing snapshot" convention as the rest of this
// job.
func (b *Bot) recordDailyATMIV(tickers []string, prices map[string]float64, date string) {
	if b.optionChain == nil {
		return
	}
	for _, ticker := range tickers {
		spot, ok := prices[ticker]
		if !ok || spot <= 0 {
			continue
		}
		expirations, err := b.optionChain.GetOptionExpirations(ticker)
		if err != nil || len(expirations) == 0 {
			continue
		}
		expiry := expirations[0]
		quotes, err := b.optionChain.GetOptionChain(ticker, expiry)
		if err != nil {
			continue
		}

		var atmIV float64
		bestDiff := math.MaxFloat64
		for _, q := range quotes {
			if q.Right != "C" {
				continue
			}
			if diff := math.Abs(q.Strike - spot); diff < bestDiff {
				bestDiff = diff
				atmIV = q.ImpliedVolatility
			}
		}
		if atmIV <= 0 {
			continue
		}
		dte := int(math.Ceil(time.Until(expiry).Hours() / 24))
		if err := b.db.SaveATMIV(ticker, date, atmIV, dte); err != nil {
			logger.Errorf("record ATM IV %s: %v", ticker, err)
		}
	}
}

// optionSelectMaxResults caps /option's reply — a liquid large-cap chain can
// pass the liquidity+delta+DTE screen with a dozen strikes across a couple
// expiries; nobody reads past the first handful in a Telegram message.
const optionSelectMaxResults = 8

// optionProfileFor maps /option's kind argument to a option.Profile — see
// parseOptionSelectArgs.
func optionProfileFor(kind string) (option.Profile, bool) {
	switch kind {
	case "", "call":
		return option.LongCall, true
	case "put":
		return option.LongPut, true
	case "csp":
		return option.CSP, true
	case "cc":
		return option.CoveredCall, true
	default:
		return option.Profile{}, false
	}
}

// parseOptionSelectArgs parses /option's "<TICKER> [call|put|csp|cc]"
// arguments — kind defaults to "call" (LongCall) when omitted.
func parseOptionSelectArgs(args string) (ticker string, profile option.Profile, err error) {
	fields := strings.Fields(args)
	if len(fields) < 1 || len(fields) > 2 {
		return "", option.Profile{}, fmt.Errorf("expected <ticker> [call|put|csp|cc]")
	}
	ticker = strings.ToUpper(fields[0])
	kind := ""
	if len(fields) == 2 {
		kind = strings.ToLower(fields[1])
	}
	profile, ok := optionProfileFor(kind)
	if !ok {
		return "", option.Profile{}, fmt.Errorf("invalid kind %q", fields[1])
	}
	return ticker, profile, nil
}

func (b *Bot) handleOption(args string) {
	ticker, profile, err := parseOptionSelectArgs(args)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyOptionSelectUsage))
		return
	}
	if b.optionChain == nil {
		b.Send(i18n.T(b.lang, i18n.KeyOptionSelectFailed, fmt.Errorf("option chain provider unavailable")))
		return
	}

	spotQuote, err := b.provider.GetQuote(ticker)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyOptionSelectFailed, err))
		return
	}

	candidates, err := b.gatherOptionCandidates(ticker, spotQuote.Price, profile)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyOptionSelectFailed, err))
		return
	}
	if len(candidates) == 0 {
		b.Send(i18n.T(b.lang, i18n.KeyOptionSelectNoCandidates, ticker, profile.Name))
		return
	}

	for i, c := range candidates {
		if i >= optionSelectMaxResults {
			break
		}
		spreadPct := c.SpreadPct * 100
		b.Send(i18n.T(b.lang, i18n.KeyOptionSelectLine,
			c.Quote.ContractSymbol, c.Mark, c.Greeks.Delta, c.Quote.ImpliedVolatility*100,
			c.Quote.OpenInterest, spreadPct, c.DTE))
	}
}

// gatherOptionCandidates fetches every expiry within profile's DTE band
// (one chain request each — Yahoo's endpoint is per-expiry, see
// optionMark's doc comment for the same constraint) and runs
// option.Select over the merged result.
func (b *Bot) gatherOptionCandidates(ticker string, spot float64, profile option.Profile) ([]option.Candidate, error) {
	expirations, err := b.optionChain.GetOptionExpirations(ticker)
	if err != nil {
		return nil, err
	}
	now := time.Now()

	var chain []data.OptionQuote
	for _, expiry := range expirations {
		dte := int(math.Ceil(time.Until(expiry).Hours() / 24))
		if dte < profile.DTEMin || dte > profile.DTEMax {
			continue
		}
		quotes, err := b.optionChain.GetOptionChain(ticker, expiry)
		if err != nil {
			logger.Errorf("option select %s @ %s: %v", ticker, expiry.Format("2006-01-02"), err)
			continue
		}
		chain = append(chain, quotes...)
	}
	return option.Select(chain, spot, now, profile), nil
}
