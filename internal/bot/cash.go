package bot

import (
	"fmt"
	"strconv"
	"strings"

	"argus/internal/i18n"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/service"
)

// cashSettingKey/cashSettingKeyTWD are the db.settings keys /cash reads/
// writes — see db.GetSetting/SetSetting's Phase 3.6 doc comment.
// cashSettingKey (USD) predates Phase 6 and keeps its original key name for
// zero-migration backward compatibility; cashSettingKeyTWD is Phase 6's new
// second book (see docs/phase-6-tw-market.md §3.2 — TWD/USD never convert,
// so this is a second independent value, not a currency-converted view of
// the first).
const (
	cashSettingKey    = service.CashSettingKeyUSD
	cashSettingKeyTWD = service.CashSettingKeyTWD
)

// handleCash manages the user's manually-declared cash balance, one per
// market (Phase 3.6, extended by Phase 6 to two books — see §3.2). With no
// argument it reports both currencies' current values (omitting whichever
// isn't set); `/cash <amount>` sets USD (backward compatible with the
// pre-Phase-6 single-currency form); `/cash usd <amount>`/`/cash twd
// <amount>` set a currency explicitly. /buy and /sell nudge this value via
// adjustCash once it's been declared here at least once (see adjustCash) —
// deposits/withdrawals/dividends never flow through the bot, so this stays a
// user-corrected reference value, not a ledger; re-run /cash to resync it
// against reality whenever it drifts. Fed into /insight/週報 (see
// handleInsight) — never into /recommend, so the model doesn't see idle cash
// and start nudging toward "put it to work."
func (b *Bot) handleCash(args string) {
	args = strings.TrimSpace(args)
	if args == "" {
		b.Send(b.renderCashStatus())
		return
	}

	m, amount, err := parseCashArgs(args)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyCashUsage))
		return
	}
	if err := b.db.SetSetting(service.CashSettingKey(m), strconv.FormatFloat(amount, 'f', 2, 64)); err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyCashSetFailed, err))
		return
	}
	if m == market.TW {
		b.Send(i18n.T(b.lang, i18n.KeyCashSetSuccessTWD, amount))
	} else {
		b.Send(i18n.T(b.lang, i18n.KeyCashSetSuccess, amount))
	}
}

// parseCashArgs parses handleCash's non-empty argument string: a bare
// amount (backward compatible, defaults to USD), or "usd <amount>"/
// "twd <amount>" to pick a currency explicitly (see §3.2 of
// docs/phase-6-tw-market.md). Pure and separately tested, same convention
// as parseTradeArgs/parseStopArgs — the caller (handleCash) owns rendering
// the usage message on error.
func parseCashArgs(args string) (m market.MarketID, amount float64, err error) {
	fields := strings.Fields(args)
	amountArg := args
	m = market.US

	switch len(fields) {
	case 1:
		amountArg = fields[0]
	case 2:
		switch strings.ToLower(fields[0]) {
		case "twd":
			m = market.TW
			amountArg = fields[1]
		case "usd":
			m = market.US
			amountArg = fields[1]
		default:
			return "", 0, fmt.Errorf("unknown currency %q", fields[0])
		}
	default:
		return "", 0, fmt.Errorf("cash: unexpected argument shape %q", args)
	}

	amount, err = strconv.ParseFloat(amountArg, 64)
	if err != nil {
		return "", 0, err
	}
	if amount < 0 {
		return "", 0, fmt.Errorf("cash: negative amount %v", amount)
	}
	return m, amount, nil
}

// renderCashStatus renders /cash's no-argument reply: one line per currency
// that's actually been set, or KeyCashNotSet if neither has.
func (b *Bot) renderCashStatus() string {
	usd, usdOK, errUSD := b.loadCash(market.US)
	if errUSD != nil {
		return i18n.T(b.lang, i18n.KeyQueryFailed, errUSD)
	}
	twd, twdOK, errTWD := b.loadCash(market.TW)
	if errTWD != nil {
		return i18n.T(b.lang, i18n.KeyQueryFailed, errTWD)
	}
	if !usdOK && !twdOK {
		return i18n.T(b.lang, i18n.KeyCashNotSet)
	}
	var sb strings.Builder
	if usdOK {
		sb.WriteString(i18n.T(b.lang, i18n.KeyCashCurrent, usd))
	}
	if twdOK {
		if usdOK {
			sb.WriteString("\n")
		}
		sb.WriteString(i18n.T(b.lang, i18n.KeyCashCurrentTWD, twd))
	}
	return sb.String()
}

// loadCash returns m's declared cash balance, or ok=false if /cash has
// never set it.
func (b *Bot) loadCash(m market.MarketID) (float64, bool, error) {
	return service.LoadCash(b.db, m)
}

// adjustCash nudges ticker's market cash balance by delta — negative for a
// buy's cost (shares*price+fee), positive for a sell's proceeds
// (shares*price-fee) — but only when the user has already declared a
// balance via /cash; with no declared baseline there's nothing sane to
// adjust from, so this is a no-op rather than inventing one at delta. Errors
// are log-only, same as thesisNudge/AddTicker: a trade confirmation
// shouldn't fail because cash bookkeeping did.
func (b *Bot) adjustCash(ticker string, delta float64) {
	m := market.Of(ticker)
	cash, ok, err := b.loadCash(m)
	if err != nil {
		logger.Errorf("adjust cash for %s: load: %v", ticker, err)
		return
	}
	if !ok {
		return
	}
	if err := b.db.SetSetting(service.CashSettingKey(m), strconv.FormatFloat(cash+delta, 'f', 2, 64)); err != nil {
		logger.Errorf("adjust cash for %s: save: %v", ticker, err)
	}
}
