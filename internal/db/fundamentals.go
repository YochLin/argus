package db

import "database/sql"

// FundamentalSnapshot is one ticker's cached Phase 23 PR6 valuation/cash-flow
// summary (migration 21, fundamental_snapshots) — derived numbers only, not
// SEC's raw companyfacts JSON (discarded once these are extracted, see
// docs/phase-23-strategy-data-uplift.md §3.5). PEPercentile/CashFlowQuality
// mirror data.FundamentalSnapshot's own nil-means-not-computed convention.
type FundamentalSnapshot struct {
	Ticker            string
	EPSAnnual         float64
	PERatio           float64
	PEPercentile      *float64
	OCF               float64
	NetIncome         float64
	CashFlowQuality   *float64
	AsOfFiscalYearEnd string
	FetchedAt         string
}

// GetFundamentalSnapshot returns ticker's cached row, or nil, nil if there
// isn't one yet — callers (bot.cachedSECSnapshot) compare FetchedAt against
// the 90-day TTL themselves rather than this method silently expiring it,
// since a stale-but-present row is still useful as a fallback on a fresh
// fetch's error (see that call site).
func (d *DB) GetFundamentalSnapshot(ticker string) (*FundamentalSnapshot, error) {
	var s FundamentalSnapshot
	var pePercentile, cashFlowQuality sql.NullFloat64
	err := d.conn.QueryRow(
		`SELECT ticker, eps_annual, pe_ratio, pe_percentile, ocf, net_income, cash_flow_quality, as_of_fiscal_year_end, fetched_at
		 FROM fundamental_snapshots WHERE ticker = ?`, ticker,
	).Scan(&s.Ticker, &s.EPSAnnual, &s.PERatio, &pePercentile, &s.OCF, &s.NetIncome, &cashFlowQuality, &s.AsOfFiscalYearEnd, &s.FetchedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if pePercentile.Valid {
		s.PEPercentile = &pePercentile.Float64
	}
	if cashFlowQuality.Valid {
		s.CashFlowQuality = &cashFlowQuality.Float64
	}
	return &s, nil
}

// SaveFundamentalSnapshot upserts one ticker's row, stamping fetched_at to
// now regardless of what the caller passed in FetchedAt (the column exists
// so GetFundamentalSnapshot can report it, not so callers can backdate it).
func (d *DB) SaveFundamentalSnapshot(s FundamentalSnapshot) error {
	var pePercentile, cashFlowQuality any
	if s.PEPercentile != nil {
		pePercentile = *s.PEPercentile
	}
	if s.CashFlowQuality != nil {
		cashFlowQuality = *s.CashFlowQuality
	}
	_, err := d.conn.Exec(
		`INSERT INTO fundamental_snapshots (ticker, eps_annual, pe_ratio, pe_percentile, ocf, net_income, cash_flow_quality, as_of_fiscal_year_end, fetched_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(ticker) DO UPDATE SET
			eps_annual = excluded.eps_annual,
			pe_ratio = excluded.pe_ratio,
			pe_percentile = excluded.pe_percentile,
			ocf = excluded.ocf,
			net_income = excluded.net_income,
			cash_flow_quality = excluded.cash_flow_quality,
			as_of_fiscal_year_end = excluded.as_of_fiscal_year_end,
			fetched_at = CURRENT_TIMESTAMP`,
		s.Ticker, s.EPSAnnual, s.PERatio, pePercentile, s.OCF, s.NetIncome, cashFlowQuality, s.AsOfFiscalYearEnd,
	)
	return err
}
