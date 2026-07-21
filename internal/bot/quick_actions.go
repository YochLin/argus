package bot

import (
	"log"

	"argus/internal/i18n"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
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

// tickerActionKeyboard is the [Check]/[Buy]/[Sell] row attached to every
// per-ticker message in Daily Report/`/recommend`/`/portfolio` (UX quick
// win — see PLAN.md). Buy/Sell can't actually prefill Telegram's message
// input box: switch_inline_query_current_chat needs Inline Mode enabled via
// BotFather and sends its chosen result immediately on tap rather than
// leaving it editable for the user to add shares/price (verified against
// the live Bot API, not assumed) — so those two reply with a copy-pasteable
// command template instead (see handleBuyQuickAction/handleSellQuickAction).
func tickerActionKeyboard(lang i18n.Lang, ticker string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, i18n.KeyCheckButton), callbackCheckPrefix+ticker),
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, i18n.KeyBuyButton), callbackBuyPrefix+ticker),
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, i18n.KeySellButton), callbackSellPrefix+ticker),
		),
	)
}

// sendWithTickerActions sends text with tickerActionKeyboard attached,
// bypassing Send's chunking helper — every caller here is a single
// per-ticker line, never long enough to need splitting, and a chunked
// message would leave the buttons attached to just the last fragment.
func (b *Bot) sendWithTickerActions(ticker, text string) {
	msg := tgbotapi.NewMessage(b.chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tickerActionKeyboard(b.lang, ticker)
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("send with ticker actions: %v", err)
	}
}
