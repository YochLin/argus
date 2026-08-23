package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/logger"
	"argus/internal/sinopac"
)

// handleSinopac backs /sinopac (dry-run listing — status, effectively,
// since a dry run is always cheap and idempotent to recompute) and
// /sinopac sync (creates real pending actions, ignoring sinopacSyncLive —
// an explicit user-typed command already is the confirmation the
// scheduled job's default-off gate exists to wait for).
func (b *Bot) handleSinopac(ctx context.Context, args string) {
	dryRun := !strings.EqualFold(strings.TrimSpace(args), "sync")
	msg, _ := b.RunSinopacSync(ctx, dryRun)
	b.Send(msg)
}

// RunSinopacSync is Phase 16's TW-only auto-bookkeeping job: reconstructs
// manual (non-定期定額) trades from the last sinopacSyncLookbackDays of
// Shioaji portfolio data and files each new one as a pending_action for the
// user to confirm via Telegram, exactly like an LLM-proposed trade — see
// internal/sinopac's Trade doc comment for why this never writes directly.
// dryRun lists what would be proposed without creating anything, which the
// scheduled job (AddSinopacSync) uses unless SINOPAC_SYNC_LIVE is set — the
// mandated safety net until PositionDetail/ProfitLoss's price-field
// semantics have been reconciled against a real brokerage statement (see
// the Phase 16 plan's risk section). found reports whether there was
// anything to show, so the scheduled job (unlike /sinopac, which always
// replies since the user explicitly asked) can stay silent on an
// uneventful day instead of pinging "nothing new" every weekday.
func (b *Bot) RunSinopacSync(ctx context.Context, dryRun bool) (msg string, found bool) {
	if b.sinopac == nil {
		return i18n.T(b.lang, i18n.KeySinopacNotConfigured), true
	}

	svc := b.brokerSync()
	trades, err := svc.FetchTrades(ctx, b.sinopacSkip, time.Now().In(cst))
	if err != nil {
		logger.Errorf("sinopac sync: fetch trades: %v", err)
		return i18n.T(b.lang, i18n.KeySinopacSyncFailed, err), true
	}

	queued, err := svc.QueuedExtIDs(func(payload string) (string, bool) {
		p, ok := decodeTradePayload(payload)
		return p.ExtID, ok
	})
	if err != nil {
		logger.Errorf("sinopac sync: queued ext_ids: %v", err)
		return i18n.T(b.lang, i18n.KeySinopacSyncFailed, err), true
	}

	var lines []string
	created := 0
	for _, t := range svc.NewTrades(trades, queued) {
		note := ""
		if t.Synthetic {
			note = " " + i18n.T(b.lang, i18n.KeySinopacSyncNoDseq)
		}
		lines = append(lines, fmt.Sprintf("%s %s %s %g @ %g%s", t.Date, t.Side, t.Ticker, t.Shares, t.Price, note))

		if dryRun {
			continue
		}
		if err := b.createSinopacPendingAction(t); err != nil {
			logger.Errorf("sinopac sync: create pending action for %s: %v", t.ExtID, err)
			continue
		}
		created++
	}

	if len(lines) == 0 {
		return i18n.T(b.lang, i18n.KeySinopacSyncNone), false
	}

	title := i18n.T(b.lang, i18n.KeySinopacSyncDryRunTitle)
	if !dryRun {
		title = i18n.T(b.lang, i18n.KeySinopacSyncTitle, created)
	}
	var sb strings.Builder
	sb.WriteString(title)
	for _, l := range lines {
		sb.WriteString("\n• " + l)
	}
	return sb.String(), true
}

// createSinopacPendingAction files t as a pending_action, exactly the
// shape internal/mcptools' record_buy/record_sell tools produce (plus
// ExtID) — sendPendingActionPrompts (chat's own pending-action bridge)
// only looks for "pending" status rows, so this alone is enough to surface
// a Telegram confirmation next time that runs; RunSinopacSync itself is
// called outside the chat flow, so it sends the prompt directly instead of
// waiting for sendPendingActionPrompts' next pass.
func (b *Bot) createSinopacPendingAction(t sinopac.Trade) error {
	actionType := db.PendingActionRecordBuy
	if t.Side == "SELL" {
		actionType = db.PendingActionRecordSell
	}
	payload, err := json.Marshal(tradePayload{Ticker: t.Ticker, Shares: t.Shares, Price: t.Price, Date: t.Date, ExtID: t.ExtID})
	if err != nil {
		return err
	}
	id, err := b.db.CreatePendingAction(actionType, string(payload))
	if err != nil {
		return err
	}
	action, ok, err := b.db.GetPendingAction(id)
	if err != nil || !ok {
		return err
	}
	text, ok := b.describePendingAction(action)
	if !ok {
		return fmt.Errorf("sinopac: could not describe pending action %d", id)
	}
	if err := b.sendPendingActionConfirmation(id, text); err != nil {
		return err
	}
	_, err = b.db.MarkPendingActionSent(id)
	return err
}

// syncSinopacCashBalance overwrites the TWD cash-balance setting with
// Shioaji's own account_balance — more accurate than adjustCash's
// buy/sell-only bookkeeping, since it reflects dividends/deposits/
// withdrawals too. Only called from executePendingAction after the user has
// actually confirmed a Sinopac-sourced trade (ExtID != "") — never from
// RunSinopacSync itself, so a dry-run or an as-yet-unconfirmed pending
// action can never overwrite this value, matching the same "never write
// without confirmation" rule Phase 16 applies to trades. Log-only on
// failure: cash balance is a reference value (see cashSettingKeyFor's doc
// comment), not worth failing the whole confirmation over.
func (b *Bot) syncSinopacCashBalance(ctx context.Context) {
	b.brokerSync().SyncCashBalance(ctx)
}
