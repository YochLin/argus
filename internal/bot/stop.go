package bot

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/market"
	"argus/internal/service"
)

// parseStopArgs parses /stop's "<ticker> [price]" arguments — price is
// optional (omitted means "show me the current setting and candidates"),
// mirroring parseTradeArgs' shape for a single optional numeric field.
func parseStopArgs(args string) (ticker string, price float64, hasPrice bool, err error) {
	fields := strings.Fields(args)
	if len(fields) < 1 || len(fields) > 2 {
		return "", 0, false, fmt.Errorf("expected <ticker> [price]")
	}
	ticker = strings.ToUpper(fields[0])
	if len(fields) == 1 {
		return ticker, 0, false, nil
	}
	if price, err = strconv.ParseFloat(fields[1], 64); err != nil || price <= 0 {
		return "", 0, false, fmt.Errorf("invalid price %q", fields[1])
	}
	return ticker, price, true, nil
}

// handleStop is Phase 3.11 PR1's /stop TICKER [PRICE] (see
// docs/phase-3.11-trade-risk-management.md §3.2): with a price, sets that
// ticker's per-trade stop-loss (db.Position.StopPrice); without one, shows
// the current setting plus three candidate reference prices computed from
// existing history — no new data source. A long position's stop must sit
// below the latest close, so it's rejected (not silently accepted) when it
// doesn't — computeStopSuggestion's LatestClose is the same number the
// candidates themselves were computed against, so this validates against
// exactly what the user is being shown. computeStopSuggestion also degrades
// to a live quote when history is unavailable; only when that also fails is
// there truly nothing to validate against, in which case the set is
// rejected rather than accepted blind.
func (b *Bot) handleStop(args string) {
	ticker, price, hasPrice, err := parseStopArgs(args)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyStopUsage))
		return
	}

	if !hasPrice {
		pos, ok, err := b.db.GetPosition(ticker)
		if err != nil {
			b.Send(i18n.T(b.lang, i18n.KeyQueryFailed, err))
			return
		}
		if !ok {
			b.Send(i18n.T(b.lang, i18n.KeyStopNoPosition, b.tickerLabel(ticker)))
			return
		}
		b.showStop(ticker, pos)
		return
	}

	msg, _ := b.setStop(ticker, price)
	b.Send(msg)
}

// setStop is handleStop's price-setting core (the "<ticker> <price>"
// branch), pulled out so ExecuteSetStop (internal/web's POST /api/stop, see
// docs/phase-10-web-trade-input.md §4.2) validates identically instead of
// duplicating the "must sit below latest close" check — that close is the
// same one computeStopSuggestion already computes for the candidate display,
// so both entry points reject against exactly the number the user would see.
func (b *Bot) setStop(ticker string, price float64) (string, error) {
	r, err := b.risksOrErr()
	if err != nil {
		return i18n.T(b.lang, i18n.KeyQueryFailed, err), err
	}
	res, err := r.SetStop(service.SetStopInput{Ticker: ticker, Price: price})
	if err != nil {
		if errors.Is(err, db.ErrNoPosition) {
			return i18n.T(b.lang, i18n.KeyStopNoPosition, b.tickerLabel(ticker)), err
		}
		var invalidStopErr *service.InvalidStopPriceError
		if errors.As(err, &invalidStopErr) {
			return i18n.T(b.lang, i18n.KeyStopInvalidPrice, b.money(ticker, price), b.money(ticker, invalidStopErr.LatestClose)), err
		}
		return i18n.T(b.lang, i18n.KeyQueryFailed, err), err
	}

	return i18n.T(b.lang, i18n.KeyStopSet, b.tickerLabel(res.Ticker), b.money(res.Ticker, res.StopPrice), res.DistPct, b.money(res.Ticker, res.RiskPerShare)), nil
}

// showStop renders /stop TICKER's no-price branch: the current setting (or
// a note that it falls back to the global STOP_LOSS_PCT) plus the three
// candidate reference prices — each skipped individually when
// computeStopSuggestion couldn't derive it (0 = not enough history), same
// degrade-per-field convention writeStockSection's MA lines already use.
func (b *Bot) showStop(ticker string, pos db.Position) {
	var sb strings.Builder
	if pos.StopPrice > 0 {
		sb.WriteString(i18n.T(b.lang, i18n.KeyStopShow, b.tickerLabel(ticker), b.money(ticker, pos.StopPrice)))
	} else {
		stopLossPct := b.stopLossPct
		if market.Of(ticker) == market.TW {
			stopLossPct = b.stopLossPctTW
		}
		sb.WriteString(i18n.T(b.lang, i18n.KeyStopNotSet, b.tickerLabel(ticker), stopLossPct))
	}

	suggestion, ok := b.computeStopSuggestion(ticker)
	if !ok {
		b.Send(sb.String())
		return
	}
	sb.WriteString(i18n.T(b.lang, i18n.KeyStopCandidatesHeader))
	if suggestion.Low10 > 0 {
		sb.WriteString(i18n.T(b.lang, i18n.KeyStopCandidateLine, i18n.T(b.lang, i18n.KeyStopLow10Label), b.money(ticker, suggestion.Low10)))
	}
	if suggestion.Low20 > 0 {
		sb.WriteString(i18n.T(b.lang, i18n.KeyStopCandidateLine, i18n.T(b.lang, i18n.KeyStopLow20Label), b.money(ticker, suggestion.Low20)))
	}
	if suggestion.ATRBased > 0 {
		sb.WriteString(i18n.T(b.lang, i18n.KeyStopCandidateLine, i18n.T(b.lang, i18n.KeyStopATRLabel), b.money(ticker, suggestion.ATRBased)))
	}
	b.Send(sb.String())
}

// buyStopSuggestion renders the same three candidates as a one-shot
// suggestion line appended to a successful /buy confirmation (§3.2) — never
// blocking or altering the trade confirmation itself, same
// degrade-by-omission convention as computeTechnicals feeding
// fetchStockData. addOnNote is appended when this buy topped up an existing
// position that already has a stop price set, since RecordBuy deliberately
// never adjusts an existing stop automatically (see Position.StopPrice).
func (b *Bot) buyStopSuggestion(ticker string, existingStopPrice float64) string {
	suggestion, ok := b.computeStopSuggestion(ticker)
	if !ok {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(i18n.T(b.lang, i18n.KeyBuyStopSuggestion))
	if suggestion.Low10 > 0 {
		sb.WriteString(i18n.T(b.lang, i18n.KeyStopCandidateLine, i18n.T(b.lang, i18n.KeyStopLow10Label), b.money(ticker, suggestion.Low10)))
	}
	if suggestion.Low20 > 0 {
		sb.WriteString(i18n.T(b.lang, i18n.KeyStopCandidateLine, i18n.T(b.lang, i18n.KeyStopLow20Label), b.money(ticker, suggestion.Low20)))
	}
	if suggestion.ATRBased > 0 {
		sb.WriteString(i18n.T(b.lang, i18n.KeyStopCandidateLine, i18n.T(b.lang, i18n.KeyStopATRLabel), b.money(ticker, suggestion.ATRBased)))
	}
	if existingStopPrice > 0 {
		sb.WriteString(i18n.T(b.lang, i18n.KeyBuyStopAddOnNote, ticker, b.money(ticker, existingStopPrice)))
	}
	return sb.String()
}

// ExecuteSetStop is ExecuteBuy's sibling for internal/web's POST /api/stop
// (see docs/phase-10-web-trade-input.md §4.2), reusing setStop's validation
// so the web form rejects a stop price at/above the latest close the exact
// same way /stop TICKER PRICE does.
func (b *Bot) ExecuteSetStop(ticker string, price float64) (string, error) {
	ticker = normalizeTicker(ticker)
	if ticker == "" || price <= 0 {
		msg := i18n.T(b.lang, i18n.KeyStopUsage)
		return msg, fmt.Errorf("invalid stop arguments")
	}
	return b.setStop(ticker, price)
}
