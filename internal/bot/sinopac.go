package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/logger"
	"argus/internal/sinopac"
)

// sinopacSyncLookbackDays is how far RunSinopacSync re-checks on every run —
// cheap since dedup is by transactions.ext_id/pending_actions payload, not
// by "have I run since date X", so a missed day (bot downtime, daemon
// session drop) gets picked up for free on the next run.
const sinopacSyncLookbackDays = 7

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

	trades, err := b.fetchSinopacTrades(ctx)
	if err != nil {
		logger.Errorf("sinopac sync: fetch trades: %v", err)
		return i18n.T(b.lang, i18n.KeySinopacSyncFailed, err), true
	}

	queued, err := b.queuedSinopacExtIDs()
	if err != nil {
		logger.Errorf("sinopac sync: queued ext_ids: %v", err)
		return i18n.T(b.lang, i18n.KeySinopacSyncFailed, err), true
	}

	var lines []string
	created := 0
	for _, t := range trades {
		if queued[t.ExtID] {
			continue
		}
		exists, err := b.db.TransactionExtIDExists(t.ExtID)
		if err != nil {
			logger.Errorf("sinopac sync: check ext_id %s: %v", t.ExtID, err)
			continue
		}
		if exists {
			continue
		}

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

	if !dryRun {
		b.syncSinopacCashBalance(ctx)
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

// fetchSinopacTrades pulls the current open-position lot detail (buys) and
// the lookback window's realized sells (profit_loss) and reconstructs Trade
// events — see internal/sinopac.Trades' doc comment for the reconstruction
// logic and its known "still-open lots only" gap on the buy side.
func (b *Bot) fetchSinopacTrades(ctx context.Context) ([]sinopac.Trade, error) {
	positions, err := b.sinopac.PositionUnit(ctx)
	if err != nil {
		return nil, fmt.Errorf("position_unit: %w", err)
	}
	var details []sinopac.PositionDetail
	for _, p := range positions {
		d, err := b.sinopac.PositionDetail(ctx, p.ID)
		if err != nil {
			return nil, fmt.Errorf("position_detail %d: %w", p.ID, err)
		}
		details = append(details, d...)
	}

	end := time.Now().In(cst)
	begin := end.AddDate(0, 0, -sinopacSyncLookbackDays)
	pnl, err := b.sinopac.ProfitLoss(ctx, begin.Format("2006-01-02"), end.Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("profit_loss: %w", err)
	}

	return sinopac.Trades(details, pnl, b.sinopacSkip), nil
}

// queuedSinopacExtIDs collects the ExtID of every trade already sitting in
// pending_actions (status pending or sent) — a second dedup layer alongside
// TransactionExtIDExists, needed because a trade proposed but not yet
// confirmed hasn't reached transactions yet.
func (b *Bot) queuedSinopacExtIDs() (map[string]bool, error) {
	out := make(map[string]bool)
	for _, status := range []string{db.PendingActionStatusPending, db.PendingActionStatusSent} {
		actions, err := b.db.GetPendingActionsByStatus(status)
		if err != nil {
			return nil, err
		}
		for _, a := range actions {
			if a.ActionType != db.PendingActionRecordBuy && a.ActionType != db.PendingActionRecordSell {
				continue
			}
			if p, ok := decodeTradePayload(a.Payload); ok && p.ExtID != "" {
				out[p.ExtID] = true
			}
		}
	}
	return out, nil
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
// withdrawals too. Log-only on failure: cash balance is a reference value
// (see cashSettingKeyFor's doc comment), not worth failing the whole sync
// reply over.
func (b *Bot) syncSinopacCashBalance(ctx context.Context) {
	bal, err := b.sinopac.AccountBalance(ctx)
	if err != nil {
		logger.Errorf("sinopac sync: account_balance: %v", err)
		return
	}
	if err := b.db.SetSetting(cashSettingKeyTWD, strconv.FormatFloat(bal.AccBalance, 'f', 2, 64)); err != nil {
		logger.Errorf("sinopac sync: set cash balance: %v", err)
	}
}
