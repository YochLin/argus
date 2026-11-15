package db

import "database/sql"

// SaveFXRate upserts one day's exchange rate for pair (e.g. "USDTWD") —
// migration 29's fx_rates table exists so a historical net-worth conversion
// never gets silently rewritten by today's rate (docs/phase-9-asset-
// platform.md §8.2).
func (d *DB) SaveFXRate(date, pair string, rate float64) error {
	_, err := d.conn.Exec(`
		INSERT INTO fx_rates (date, pair, rate) VALUES (?, ?, ?)
		ON CONFLICT(date, pair) DO UPDATE SET rate = excluded.rate`,
		date, pair, rate,
	)
	return err
}

// GetFXRateOnOrBefore is GetFXRate with a bounded lookback: the newest rate
// recorded on date or up to maxDays before it. fx_rates only gains a row on a
// day someone opened a wealth page (the live fetch is the only writer), so an
// exact-date lookup misses most past days. Only the net-worth trend asks for
// this — every other history metric keeps GetFXRate's exact-date rule — and
// the cap keeps a months-old rate from standing in for a day it never priced.
func (d *DB) GetFXRateOnOrBefore(date, pair string, maxDays int) (float64, bool, error) {
	var rate float64
	err := d.conn.QueryRow(`
		SELECT rate FROM fx_rates
		WHERE pair = ? AND date <= ? AND date >= date(?, '-' || ? || ' days')
		ORDER BY date DESC LIMIT 1`,
		pair, date, date, maxDays,
	).Scan(&rate)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return rate, true, nil
}

// GetFXRate returns pair's rate on exactly date, or ok=false if none is
// recorded. No closest-date fallback on purpose: a wealth-page metric that
// needs a rate for a date this table doesn't have renders "—" rather than
// guess with a nearby day's rate (§8.17.1's "don't fabricate a number from
// an incomplete period" rule applies here too).
func (d *DB) GetFXRate(date, pair string) (float64, bool, error) {
	var rate float64
	err := d.conn.QueryRow(`SELECT rate FROM fx_rates WHERE date = ? AND pair = ?`, date, pair).Scan(&rate)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return rate, true, nil
}
