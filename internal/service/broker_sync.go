package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"argus/internal/db"
	"argus/internal/logger"
	"argus/internal/sinopac"
)

// SinopacSyncLookbackDays is how far FetchTrades re-checks on every run —
// cheap since dedup is by transactions.ext_id/pending_actions payload, not
// by "have I run since date X", so a missed day (bot downtime, daemon
// session drop) gets picked up for free on the next run.
const SinopacSyncLookbackDays = 7

// SinopacClient is the subset of *sinopac.Client BrokerSyncService needs —
// narrow so tests can fake it without a real Shioaji server.
type SinopacClient interface {
	PositionUnit(ctx context.Context) ([]sinopac.Position, error)
	PositionDetail(ctx context.Context, detailID int) ([]sinopac.PositionDetail, error)
	ProfitLoss(ctx context.Context, begin, end string) ([]sinopac.ProfitLoss, error)
	AccountBalance(ctx context.Context) (sinopac.AccountBalance, error)
}

// BrokerSyncStore is the persistence boundary BrokerSyncService needs from
// *db.DB.
type BrokerSyncStore interface {
	GetPendingActionsByStatus(status string) ([]db.PendingAction, error)
	TransactionExtIDExists(extID string) (bool, error)
	SetSetting(key, value string) error
}

type BrokerSyncService struct {
	client SinopacClient
	store  BrokerSyncStore
}

func NewBrokerSyncService(client SinopacClient, store BrokerSyncStore) *BrokerSyncService {
	return &BrokerSyncService{client: client, store: store}
}

// FetchTrades pulls the current open-position lot detail (buys) and the
// lookback window's realized sells (profit_loss) and reconstructs Trade
// events — see internal/sinopac.Trades' doc comment for the reconstruction
// logic and its known "still-open lots only" gap on the buy side.
func (s *BrokerSyncService) FetchTrades(ctx context.Context, skip map[string]bool, now time.Time) ([]sinopac.Trade, error) {
	positions, err := s.client.PositionUnit(ctx)
	if err != nil {
		return nil, fmt.Errorf("position_unit: %w", err)
	}
	var details []sinopac.PositionDetail
	for _, p := range positions {
		d, err := s.client.PositionDetail(ctx, p.ID)
		if err != nil {
			return nil, fmt.Errorf("position_detail %d: %w", p.ID, err)
		}
		details = append(details, d...)
	}

	end := now
	begin := end.AddDate(0, 0, -SinopacSyncLookbackDays)
	pnl, err := s.client.ProfitLoss(ctx, begin.Format("2006-01-02"), end.Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("profit_loss: %w", err)
	}

	return sinopac.Trades(details, pnl, skip), nil
}

// QueuedExtIDs collects the ExtID of every trade already sitting in
// pending_actions (status pending or sent) — a second dedup layer alongside
// NewTrades' TransactionExtIDExists check, needed because a trade proposed
// but not yet confirmed hasn't reached transactions yet. extractExtID
// decodes a pending_action's payload — its shape (bot's tradePayload /
// mcptools' own copy) is a caller concern, not this service's.
func (s *BrokerSyncService) QueuedExtIDs(extractExtID func(payload string) (extID string, ok bool)) (map[string]bool, error) {
	out := make(map[string]bool)
	for _, status := range []string{db.PendingActionStatusPending, db.PendingActionStatusSent} {
		actions, err := s.store.GetPendingActionsByStatus(status)
		if err != nil {
			return nil, err
		}
		for _, a := range actions {
			if a.ActionType != db.PendingActionRecordBuy && a.ActionType != db.PendingActionRecordSell {
				continue
			}
			if extID, ok := extractExtID(a.Payload); ok && extID != "" {
				out[extID] = true
			}
		}
	}
	return out, nil
}

// NewTrades filters trades down to ones not already queued or recorded. A
// per-trade TransactionExtIDExists error is logged and that trade skipped,
// not fatal to the whole batch — matches the original bot.RunSinopacSync
// loop's behavior.
func (s *BrokerSyncService) NewTrades(trades []sinopac.Trade, queued map[string]bool) []sinopac.Trade {
	var out []sinopac.Trade
	for _, t := range trades {
		if queued[t.ExtID] {
			continue
		}
		exists, err := s.store.TransactionExtIDExists(t.ExtID)
		if err != nil {
			logger.Errorf("broker sync: check ext_id %s: %v", t.ExtID, err)
			continue
		}
		if exists {
			continue
		}
		out = append(out, t)
	}
	return out
}

// SyncCashBalance overwrites the TWD cash-balance setting with Shioaji's own
// account_balance — more accurate than buy/sell-only bookkeeping, since it
// reflects dividends/deposits/withdrawals too. Log-only on failure: cash
// balance is a reference value, not worth failing a confirmation over.
func (s *BrokerSyncService) SyncCashBalance(ctx context.Context) {
	bal, err := s.client.AccountBalance(ctx)
	if err != nil {
		logger.Errorf("sinopac sync: account_balance: %v", err)
		return
	}
	if err := s.store.SetSetting(CashSettingKeyTWD, strconv.FormatFloat(bal.AccBalance, 'f', 2, 64)); err != nil {
		logger.Errorf("sinopac sync: set cash balance: %v", err)
	}
}
