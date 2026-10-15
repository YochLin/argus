package bot

import (
	"fmt"
	"strconv"
	"strings"

	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/service"
)

func (b *Bot) buyAlertDirPhrase(direction string) string {
	if direction == db.BuyAlertAbove {
		return i18n.T(b.lang, i18n.KeyBuyAlertDirAbove)
	}
	return i18n.T(b.lang, i18n.KeyBuyAlertDirBelow)
}

// parseBuyAlertArgs parses /buyalert's "<ticker> [price | remove <price>]"
// arguments — mirrors parseStopArgs' shape (price optional, meaning "show me
// the current alerts"), plus a "remove <price>" form since a ticker can carry
// several alerts and /stop's single-column semantics don't need one.
func parseBuyAlertArgs(args string) (ticker string, remove bool, price float64, hasPrice bool, err error) {
	fields := strings.Fields(args)
	if len(fields) < 1 || len(fields) > 3 {
		return "", false, 0, false, fmt.Errorf("expected <ticker> [price | remove <price>]")
	}
	ticker = strings.ToUpper(fields[0])
	if len(fields) == 1 {
		return ticker, false, 0, false, nil
	}
	if len(fields) == 3 {
		if strings.ToLower(fields[1]) != "remove" {
			return "", false, 0, false, fmt.Errorf("expected 'remove' before the price")
		}
		if price, err = strconv.ParseFloat(fields[2], 64); err != nil || price <= 0 {
			return "", false, 0, false, fmt.Errorf("invalid price %q", fields[2])
		}
		return ticker, true, price, true, nil
	}
	if price, err = strconv.ParseFloat(fields[1], 64); err != nil || price <= 0 {
		return "", false, 0, false, fmt.Errorf("invalid price %q", fields[1])
	}
	return ticker, false, price, true, nil
}

// handleBuyAlert is /buyalert TICKER [PRICE | remove PRICE] (see setStop's
// doc comment for the parallel /stop design): with no price, lists the
// ticker's current buy alerts; with a price, adds one (direction inferred,
// see buyAlertDirection); with "remove PRICE", deletes the matching one.
// Unlike /stop, no open position is required — a buy alert's whole purpose
// is watching a ticker the user doesn't hold yet.
func (b *Bot) handleBuyAlert(args string) {
	ticker, remove, price, hasPrice, err := parseBuyAlertArgs(args)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyBuyAlertUsage))
		return
	}

	if !hasPrice {
		b.showBuyAlerts(ticker)
		return
	}

	if remove {
		b.removeBuyAlert(ticker, price)
		return
	}

	msg, _ := b.addBuyAlert(ticker, price)
	b.Send(msg)
}

// addBuyAlert is handleBuyAlert's price-setting core, pulled out so
// ExecuteAddBuyAlert (internal/web's POST /api/buy-alerts/add) validates and
// infers direction identically instead of duplicating the quote fetch —
// same split as setStop/ExecuteSetStop.
func (b *Bot) addBuyAlert(ticker string, price float64) (string, error) {
	r, err := b.risksOrErr()
	if err != nil {
		return i18n.T(b.lang, i18n.KeyBuyAlertQueryFailed, err), err
	}
	res, err := r.AddBuyAlert(service.BuyAlertInput{Ticker: ticker, Price: price})
	if err != nil {
		return i18n.T(b.lang, i18n.KeyBuyAlertQueryFailed, err), err
	}

	return i18n.T(b.lang, i18n.KeyBuyAlertSet, b.tickerLabel(res.Ticker), b.money(res.Ticker, res.Price), b.buyAlertDirPhrase(res.Direction)), nil
}

// showBuyAlerts renders /buyalert TICKER's no-price branch: every alert
// currently set on ticker, oldest first.
func (b *Bot) showBuyAlerts(ticker string) {
	r, err := b.risksOrErr()
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyBuyAlertQueryFailed, err))
		return
	}
	alerts, err := r.GetBuyAlerts(ticker)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyBuyAlertQueryFailed, err))
		return
	}
	if len(alerts) == 0 {
		b.Send(i18n.T(b.lang, i18n.KeyBuyAlertEmpty, b.tickerLabel(ticker)))
		return
	}

	var sb strings.Builder
	sb.WriteString(i18n.T(b.lang, i18n.KeyBuyAlertListHeader, b.tickerLabel(ticker)))
	for _, a := range alerts {
		sb.WriteString(i18n.T(b.lang, i18n.KeyBuyAlertLine, b.money(ticker, a.Price), b.buyAlertDirPhrase(a.Direction)))
	}
	b.Send(sb.String())
}

// removeBuyAlert deletes the alert(s) on ticker matching price exactly —
// there's no id exposed over Telegram, so price is the only handle the user
// has (same tradeoff as most chat commands operating on human-entered
// numbers rather than internal ids).
func (b *Bot) removeBuyAlert(ticker string, price float64) {
	r, err := b.risksOrErr()
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyBuyAlertQueryFailed, err))
		return
	}
	removed, err := r.RemoveBuyAlertByPrice(ticker, price)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyBuyAlertQueryFailed, err))
		return
	}
	if !removed {
		b.Send(i18n.T(b.lang, i18n.KeyBuyAlertNotFound, b.tickerLabel(ticker), b.money(ticker, price)))
		return
	}
	b.Send(i18n.T(b.lang, i18n.KeyBuyAlertRemoved, b.tickerLabel(ticker), b.money(ticker, price)))
}

// ExecuteAddBuyAlert is internal/web's POST /api/buy-alerts/add entry point
// (see docs/phase-10-web-trade-input.md §4.2's "one write path" rule,
// mirrored by TradeExecutor.ExecuteSetStop) — a thin wrapper around
// addBuyAlert so the web handler never duplicates the quote-fetch/direction-
// inference logic.
func (b *Bot) ExecuteAddBuyAlert(ticker string, price float64) (string, error) {
	return b.addBuyAlert(ticker, price)
}
