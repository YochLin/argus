package db

import (
	"database/sql"

	"argus/internal/market"
)

// SaveNetWorthSnapshot upserts the total position value for (date, market) —
// see net_worth_snapshots' migration-12 PK rebuild (Phase 6): the two
// markets' totals are always tracked as separate rows, never summed, since
// TWD and USD don't convert.
func (d *DB) SaveNetWorthSnapshot(date string, m market.MarketID, total float64) error {
	_, err := d.conn.Exec(`
		INSERT INTO net_worth_snapshots (date, market, total_value)
		VALUES (?, ?, ?)
		ON CONFLICT(date, market) DO UPDATE SET total_value = excluded.total_value`,
		date, string(m), total,
	)
	return err
}

// GetLatestNetWorth returns the most recent net_worth_snapshots row for
// market regardless of date, or ok=false if none exists yet — same "most
// recent regardless of date" shape as GetLatestSnapshot, for the weekly
// review's net-worth line (Phase 3.6 PR2) and Phase 3.11's accountValue.
func (d *DB) GetLatestNetWorth(m market.MarketID) (date string, total float64, ok bool, err error) {
	err = d.conn.QueryRow(
		`SELECT date, total_value FROM net_worth_snapshots WHERE market = ? ORDER BY date DESC LIMIT 1`,
		string(m),
	).Scan(&date, &total)
	if err == sql.ErrNoRows {
		return "", 0, false, nil
	}
	if err != nil {
		return "", 0, false, err
	}
	return date, total, true, nil
}

// GetNetWorthOnOrBefore returns the most recent net_worth_snapshots row for
// market with date <= the given date, or ok=false if none exists — used to
// find a baseline "about a week ago" even when that exact date wasn't a
// trading day (weekend/holiday), same reasoning as GetPeakClose's date-range
// query.
func (d *DB) GetNetWorthOnOrBefore(date string, m market.MarketID) (float64, bool, error) {
	var total float64
	err := d.conn.QueryRow(
		`SELECT total_value FROM net_worth_snapshots WHERE market = ? AND date <= ? ORDER BY date DESC LIMIT 1`,
		string(m), date,
	).Scan(&total)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return total, true, nil
}

// NetWorthPoint is one date/total pair from net_worth_snapshots, returned by
// GetNetWorthRange for Phase 3.6 追加項's monthly report (see
// docs/phase-3.6-monthly-report.md) — the first reader to want a whole
// range of points rather than a single latest/on-or-before value.
type NetWorthPoint struct {
	Date  string
	Total float64
}

// GetNetWorthRange returns every net_worth_snapshots row for market with
// date in [from, to] inclusive, date ascending — the monthly report's raw
// input for its sparkline/drawdown calculations (both pure functions over
// this slice, see bot.sparkline/bot.maxDrawdownPct).
func (d *DB) GetNetWorthRange(from, to string, m market.MarketID) ([]NetWorthPoint, error) {
	rows, err := d.conn.Query(
		`SELECT date, total_value FROM net_worth_snapshots WHERE market = ? AND date BETWEEN ? AND ? ORDER BY date`,
		string(m), from, to,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []NetWorthPoint
	for rows.Next() {
		var p NetWorthPoint
		if err := rows.Scan(&p.Date, &p.Total); err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	return points, rows.Err()
}
