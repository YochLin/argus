package bot

import (
	"log"

	"argus/internal/i18n"
)

// callbackCheckPrefix/callbackBuyPrefix/callbackSellPrefix identify a
// per-ticker quick-action button's callback_data — a different namespace
// from pending_actions.go's callbackConfirmPrefix/callbackRejectPrefix, so
// both live side by side in handleCallbackQuery without collision (an
// unrecognized prefix already falls through harmlessly).
const (
	callbackCheckPrefix = "act_check:"
	callbackBuyPrefix   = "act_buy:"
	callbackSellPrefix  = "act_sell:"
)

// tickerActionButtons is the [Check]/[Buy]/[Sell] row attached to every
// per-ticker message in Daily Report/`/recommend`/`/portfolio` (UX quick
// win — see PLAN.md). Buy/Sell can't actually prefill Telegram's message
// input box: switch_inline_query_current_chat needs Inline Mode enabled via
// BotFather and sends its chosen result immediately on tap rather than
// leaving it editable for the user to add shares/price (verified against
// the live Bot API, not assumed) — so those two reply with a copy-pasteable
// command template instead (see handleBuyQuickAction/handleSellQuickAction).
func tickerActionButtons(lang i18n.Lang, ticker string) []Button {
	return []Button{
		{Label: i18n.T(lang, i18n.KeyCheckButton), Data: callbackCheckPrefix + ticker},
		{Label: i18n.T(lang, i18n.KeyBuyButton), Data: callbackBuyPrefix + ticker},
		{Label: i18n.T(lang, i18n.KeySellButton), Data: callbackSellPrefix + ticker},
	}
}

// sendWithTickerActions sends text with tickerActionButtons attached — every
// caller here is a single per-ticker line, never long enough to need
// Send's chunking.
func (b *Bot) sendWithTickerActions(ticker, text string) {
	if err := b.channel.SendWithButtons(text, tickerActionButtons(b.lang, ticker)); err != nil {
		log.Printf("send with ticker actions: %v", err)
	}
}
