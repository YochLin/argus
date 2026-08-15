package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/paper"
)

// callbackConfirmPrefix/callbackRejectPrefix identify a Telegram inline
// keyboard button's callback_data as "confirm/reject pending action <id>" —
// Telegram's callback_data cap is 64 bytes, and "pa_confirm:<id>" is well
// within it even for a large id.
const (
	callbackConfirmPrefix = "pa_confirm:"
	callbackRejectPrefix  = "pa_reject:"
)

// tradePayload mirrors internal/mcptools' own copy (trade_write_tools.go) —
// same deliberate small duplication as this codebase's other
// can't-share-an-import cases (see that file's doc comment), since bot
// doesn't import mcptools and mcptools can't import bot.
type tradePayload struct {
	Ticker string   `json:"ticker"`
	Shares float64  `json:"shares"`
	Price  float64  `json:"price"`
	Fee    *float64 `json:"fee"`
	Date   string   `json:"date"`
	ExtID  string   `json:"ext_id,omitempty"`
}

func decodeTradePayload(payload string) (tradePayload, bool) {
	var p tradePayload
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return tradePayload{}, false
	}
	return p, true
}

// resolvePendingFee is an MCP-proposed trade's Fee: an explicit value if the
// model set one, otherwise auto-calculated via paper.FeeFor exactly like
// ExecuteBuy/ExecuteSell's nil-fee branch — the tool schema promises "omit
// to auto-calculate", so this must match, not silently default to 0 (which
// used to skip TW's brokerage fee entirely on an MCP-confirmed trade).
func (b *Bot) resolvePendingFee(p tradePayload, side string) float64 {
	if p.Fee != nil {
		return *p.Fee
	}
	return paper.FeeFor(market.Of(p.Ticker), side, p.Shares*p.Price, b.twFeeDiscount)
}

// sendPendingActionPrompts checks for any db.PendingAction rows a chat tool
// call (record_buy/record_sell, running in the MCP subprocess — see
// internal/mcptools' trade_write_tools.go) left in "pending" status during
// the LLM turn that just completed, and sends each one a Telegram
// confirmation message with Confirm/Reject inline buttons — marking it
// "sent" so a later call doesn't show it again. The MCP subprocess has no
// Telegram bot of its own to do this with; this is the bridge back to the
// one process that does. Called from handleChat/handleChatArticle right
// after the LLM reply is sent, since that's the only point in the chat flow
// where a tool call could have run.
func (b *Bot) sendPendingActionPrompts() {
	actions, err := b.db.GetPendingActionsByStatus(db.PendingActionStatusPending)
	if err != nil {
		logger.Errorf("pending actions: query pending: %v", err)
		return
	}
	for _, a := range actions {
		text, ok := b.describePendingAction(a)
		if !ok {
			logger.Errorf("pending actions: could not describe action id %d (type %q)", a.ID, a.ActionType)
			continue
		}
		if err := b.sendPendingActionConfirmation(a.ID, text); err != nil {
			logger.Errorf("pending actions: send confirmation for id %d: %v", a.ID, err)
			continue
		}
		if _, err := b.db.MarkPendingActionSent(a.ID); err != nil {
			logger.Errorf("pending actions: mark sent id %d: %v", a.ID, err)
		}
	}
}

// describePendingAction renders the human-readable confirmation text for a
// pending action, or ok=false if its action_type/payload can't be
// understood (defensive only — every action_type this build can create has
// a case below).
func (b *Bot) describePendingAction(a db.PendingAction) (string, bool) {
	switch a.ActionType {
	case db.PendingActionRecordBuy:
		p, ok := decodeTradePayload(a.Payload)
		if !ok {
			return "", false
		}
		text := i18n.T(b.lang, i18n.KeyPendingBuyConfirm, p.Ticker, p.Shares, b.money(p.Ticker, p.Price), b.money(p.Ticker, b.resolvePendingFee(p, "BUY")), p.Date)
		return text + sinopacSourceNote(b.lang, p.ExtID), true
	case db.PendingActionRecordSell:
		p, ok := decodeTradePayload(a.Payload)
		if !ok {
			return "", false
		}
		text := i18n.T(b.lang, i18n.KeyPendingSellConfirm, p.Ticker, p.Shares, b.money(p.Ticker, p.Price), b.money(p.Ticker, b.resolvePendingFee(p, "SELL")), p.Date)
		return text + sinopacSourceNote(b.lang, p.ExtID), true
	default:
		return "", false
	}
}

// sinopacSourceNote annotates a pending action's confirmation text when it
// came from Phase 16's Shioaji sync (extID set) rather than an LLM tool
// call, so the user knows it's a broker-observed trade, not a proposal.
func sinopacSourceNote(lang i18n.Lang, extID string) string {
	if extID == "" {
		return ""
	}
	return i18n.T(lang, i18n.KeyPendingActionFromSinopac)
}

// sendPendingActionConfirmation sends text with a Confirm/Reject inline
// keyboard (these messages are always short, hand-composed templates, never
// user-scale content needing Send's chunking).
func (b *Bot) sendPendingActionConfirmation(id int64, text string) error {
	buttons := []Button{
		{Label: i18n.T(b.lang, i18n.KeyConfirmButton), Data: fmt.Sprintf("%s%d", callbackConfirmPrefix, id)},
		{Label: i18n.T(b.lang, i18n.KeyRejectButton), Data: fmt.Sprintf("%s%d", callbackRejectPrefix, id)},
	}
	return b.channel.SendWithButtons(text, buttons)
}

// handleCallbackQuery processes a tap on any inline keyboard button this
// bot sends — pending-action confirm/reject (below) and per-ticker quick
// actions (quick_actions.go's callbackCheckPrefix/callbackBuyPrefix/
// callbackSellPrefix, checked first since they're a different callback_data
// namespace). Both always answer the callback query first (clears the
// channel's loading spinner, if it has one) regardless of outcome.
// Pending-action taps then edit the original message in place to show the
// result instead of sending a new one, so the chat doesn't accumulate a
// stray "confirmed"/"rejected" line under the original proposal; quick
// actions instead send a fresh message (a per-ticker message that already
// has its own buttons stays untouched, so it can still be tapped again).
func (b *Bot) handleCallbackQuery(ctx context.Context, cb InCallback) {
	if ticker, ok := strings.CutPrefix(cb.Data, callbackCheckPrefix); ok {
		b.channel.AnswerCallback(cb.ID)
		go b.handleCheck(ctx, ticker)
		return
	}
	if ticker, ok := strings.CutPrefix(cb.Data, callbackBuyPrefix); ok {
		b.channel.AnswerCallback(cb.ID)
		b.Send(i18n.T(b.lang, i18n.KeyBuyCommandTemplate, ticker))
		return
	}
	if ticker, ok := strings.CutPrefix(cb.Data, callbackSellPrefix); ok {
		b.channel.AnswerCallback(cb.ID)
		b.Send(i18n.T(b.lang, i18n.KeySellCommandTemplate, ticker))
		return
	}

	id, confirm, ok := parseCallbackData(cb.Data)
	if !ok {
		return
	}

	b.channel.AnswerCallback(cb.ID)

	resultText := b.resolvePendingAction(ctx, id, confirm)
	if err := b.channel.EditMessage(cb.MsgRef, resultText); err != nil {
		logger.Errorf("pending actions: edit confirmation message: %v", err)
	}
}

// parseCallbackData splits a button's callback_data back into the pending
// action id and whether it was the confirm or reject button.
func parseCallbackData(data string) (id int64, confirm bool, ok bool) {
	var idStr string
	switch {
	case strings.HasPrefix(data, callbackConfirmPrefix):
		idStr, confirm = strings.TrimPrefix(data, callbackConfirmPrefix), true
	case strings.HasPrefix(data, callbackRejectPrefix):
		idStr, confirm = strings.TrimPrefix(data, callbackRejectPrefix), false
	default:
		return 0, false, false
	}
	parsed, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return 0, false, false
	}
	return parsed, confirm, true
}

// resolvePendingAction atomically claims the pending action (guarding
// against a double-tap or a tap on an already-resolved message — see
// db.ResolvePendingAction) and, if confirmed, executes it. Returns the text
// to show in place of the original confirmation message.
func (b *Bot) resolvePendingAction(ctx context.Context, id int64, confirm bool) string {
	action, ok, err := b.db.GetPendingAction(id)
	if err != nil {
		return i18n.T(b.lang, i18n.KeyQueryFailed, err)
	}
	if !ok || action.Status != db.PendingActionStatusSent {
		return i18n.T(b.lang, i18n.KeyPendingActionAlreadyResolved)
	}

	targetStatus := db.PendingActionStatusRejected
	if confirm {
		targetStatus = db.PendingActionStatusConfirmed
	}
	resolved, err := b.db.ResolvePendingAction(id, targetStatus)
	if err != nil {
		return i18n.T(b.lang, i18n.KeyQueryFailed, err)
	}
	if !resolved {
		// Lost a race with a concurrent tap (or a second tap on the same
		// button) — someone else already resolved this row.
		return i18n.T(b.lang, i18n.KeyPendingActionAlreadyResolved)
	}

	if !confirm {
		return i18n.T(b.lang, i18n.KeyPendingActionRejected)
	}
	return b.executePendingAction(ctx, action)
}

// executePendingAction runs the confirmed action, reusing exactly the same
// code path (and confirmation text) /buy and /sell themselves use — see
// recordBuy/recordSell in handlers.go. A confirmed record_sell that fully
// closes the position triggers Phase 3.8's sell-review exactly like /sell
// itself does, so a chat-confirmed sell gets the same follow-up as a manual
// one.
func (b *Bot) executePendingAction(ctx context.Context, action db.PendingAction) string {
	switch action.ActionType {
	case db.PendingActionRecordBuy:
		p, ok := decodeTradePayload(action.Payload)
		if !ok {
			return i18n.T(b.lang, i18n.KeyPendingActionExecFailed)
		}
		msg, _ := b.recordBuy(p.Ticker, p.Shares, p.Price, b.resolvePendingFee(p, "BUY"), p.Fee == nil, p.Date, p.ExtID)
		if p.ExtID != "" && b.sinopac != nil {
			b.syncSinopacCashBalance(ctx)
		}
		return msg
	case db.PendingActionRecordSell:
		p, ok := decodeTradePayload(action.Payload)
		if !ok {
			return i18n.T(b.lang, i18n.KeyPendingActionExecFailed)
		}
		msg, closed, stopPrice, _ := b.recordSell(p.Ticker, p.Shares, p.Price, b.resolvePendingFee(p, "SELL"), p.Fee == nil, p.Date, p.ExtID)
		if p.ExtID != "" && b.sinopac != nil {
			b.syncSinopacCashBalance(ctx)
		}
		if closed {
			go b.reviewClosedTrade(ctx, p.Ticker, stopPrice)
		}
		return msg
	case db.PendingActionOptionExpiry:
		p, ok := decodeOptionExpiryPayload(action.Payload)
		if !ok {
			return i18n.T(b.lang, i18n.KeyPendingActionExecFailed)
		}
		return b.resolveOption(p.Symbol, p.Date, p.ProposedAction)
	default:
		return i18n.T(b.lang, i18n.KeyPendingActionExecFailed)
	}
}
