package data

import (
	"fmt"

	"argus/internal/market"
)

// InsiderTransaction is a single SEC Form 4 insider buy/sell/grant, from
// Finnhub's /stock/insider-transactions (free tier, live-verified 2026-08-02
// — unlike /stock/ownership this one isn't premium-gated).
type InsiderTransaction struct {
	Ticker           string
	Name             string // filer's name, e.g. "COOK TIMOTHY D"
	Share            int64  // shares held after this transaction
	Change           int64  // signed delta vs. the filer's prior holding; negative is a sale/disposal
	TransactionDate  string // YYYY-MM-DD
	TransactionCode  string // SEC Form 4 code: "P" open-market buy, "S" open-market sale, "M" option exercise, "G" gift, "A" grant/award, "F" tax withholding, etc.
	TransactionPrice float64
}

// InsiderTransactionProvider is implemented only by Finnhub — same reasoning
// as AnalystRatingProvider: no Yahoo equivalent, and TW isn't in scope (no
// Form-4-equivalent filing regime).
type InsiderTransactionProvider interface {
	GetInsiderTransactions(ticker string, limit int) ([]InsiderTransaction, error)
	GetInsiderTransactionsRange(ticker, from, to string) ([]InsiderTransaction, error)
}

type finnhubInsiderTransaction struct {
	Name             string  `json:"name"`
	Share            int64   `json:"share"`
	Change           int64   `json:"change"`
	TransactionDate  string  `json:"transactionDate"`
	TransactionCode  string  `json:"transactionCode"`
	TransactionPrice float64 `json:"transactionPrice"`
}

// GetInsiderTransactions fetches Finnhub's insider-transactions feed for
// ticker, most-recent-filing-first (Finnhub's own order — see
// TestFinnhubTWGuard's sibling methods for the same trust-the-response-order
// choice /stock/recommendation makes), capped at limit rows.
func (f *Finnhub) GetInsiderTransactions(ticker string, limit int) ([]InsiderTransaction, error) {
	if market.Of(ticker) == market.TW {
		return nil, errTWNotSupported
	}
	var result struct {
		Data []finnhubInsiderTransaction `json:"data"`
	}
	if err := f.get(fmt.Sprintf("/stock/insider-transactions?symbol=%s", ticker), &result); err != nil {
		return nil, err
	}
	return buildInsiderTransactions(ticker, result.Data, limit), nil
}

// GetInsiderTransactionsRange is GetInsiderTransactions' sister method for
// Phase 25 §8.2's backtest cache (cmd/strategyscan/insider_cache.go) — the
// live path (LLM briefing, Phase 19 audit page) only ever wants "most
// recent N", but a backtest needs a specific historical window. Finnhub's
// free tier does support from/to on this endpoint (live-verified
// 2026-08-29: /stock/insider-transactions?symbol=AAPL&from=2016-01-01&
// to=2016-12-31 returns real 2016 filings, not an empty/truncated page —
// unlike EarningsSurpriseProvider's /stock/earnings, which silently caps at
// the trailing 4 quarters regardless of from/to, this endpoint honors the
// range). No limit param: a backtest wants the whole window, not a capped
// "most recent" page.
func (f *Finnhub) GetInsiderTransactionsRange(ticker, from, to string) ([]InsiderTransaction, error) {
	if market.Of(ticker) == market.TW {
		return nil, errTWNotSupported
	}
	var result struct {
		Data []finnhubInsiderTransaction `json:"data"`
	}
	if err := f.get(fmt.Sprintf("/stock/insider-transactions?symbol=%s&from=%s&to=%s", ticker, from, to), &result); err != nil {
		return nil, err
	}
	return buildInsiderTransactions(ticker, result.Data, 0), nil
}

// buildInsiderTransactions maps Finnhub's raw rows to InsiderTransaction,
// capped at limit (<= 0 means no cap) — split out from
// GetInsiderTransactions so this mapping/capping logic is testable without a
// live HTTP round-trip.
func buildInsiderTransactions(ticker string, rows []finnhubInsiderTransaction, limit int) []InsiderTransaction {
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	out := make([]InsiderTransaction, len(rows))
	for i, r := range rows {
		out[i] = InsiderTransaction{
			Ticker:           ticker,
			Name:             r.Name,
			Share:            r.Share,
			Change:           r.Change,
			TransactionDate:  r.TransactionDate,
			TransactionCode:  r.TransactionCode,
			TransactionPrice: r.TransactionPrice,
		}
	}
	return out
}
