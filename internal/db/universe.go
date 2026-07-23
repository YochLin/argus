package db

import (
	_ "embed"
	"sort"
	"strings"
	"time"
)

// sp500Tickers is the embedded seed list for the universe table's 'sp500'
// source tier — one ticker symbol per line, extracted from the Symbol column
// of https://github.com/datasets/s-and-p-500-companies (which itself syncs
// Wikipedia's S&P 500 constituents list). Only the ticker is ever needed, so
// this is a plain symbol list rather than the full CSV (sector/HQ/etc.,
// which also has quoting complications like `"Saint Paul, Minnesota"`).
//
//go:embed sp500_tickers.txt
var sp500Tickers string

// tw150Tickers is the embedded seed list for the universe table's 'tw'
// source tier (Phase 6 PR2, see docs/phase-6-tw-market.md §5.2) — a
// hand-compiled approximation of 0050 (Taiwan 50)'s large-cap constituents
// plus a selection of well-known 台灣中型100 (mid-cap) names, ~120 tickers
// total. Unlike sp500Tickers there is no single stable public CSV export for
// either index's constituents (both etfinfo.tw and moneydj paginate their
// holdings tables), so this list was compiled by hand from those sites'
// visible holdings plus well-established TW market knowledge as of 2026-07 —
// it is a known-approximate seed, not an authoritative index snapshot.
// Refreshing it is a manual, human-driven process repeated in a future PR
// (see docs/phase-6-tw-market.md §7's "tw150 清單無自動刷新" known gap), same
// staleness trade-off as sp500Tickers' own refresh story but without an
// automated SyncSP500-style diff — a scan pool that's missing a newer
// mid-cap constituent or still carries a name that fell out of the index
// just scans a slightly different set than the live index; it doesn't
// produce wrong data for any ticker it does scan.
//
//go:embed tw150_tickers.txt
var tw150Tickers string

// UniverseEntry is one ticker in the Phase 2.6 candidate scan pool.
type UniverseEntry struct {
	Ticker  string
	Source  string // "sp500" (seeded) or "manual" (/universe add)
	AddedAt time.Time
}

// seedSP500 bulk-inserts the embedded S&P 500 ticker list with source='sp500'
// the first time it's ever called against a given database (checked via a
// count query, not a migration step, since it's data not schema). It's
// deliberately *not* re-synced on every startup — a user's manual
// `/universe remove` of a seeded ticker should stick; refreshing against a
// newer embedded list is SyncSP500's job, called separately from main(). The
// count query deliberately doesn't filter out tombstoned (removed=1) rows —
// a tombstoned row still counts as "already seeded once," and TestSeedSP500
// depends on that to keep a removed ticker from coming back here.
func (d *DB) seedSP500() error {
	var count int
	if err := d.conn.QueryRow(`SELECT COUNT(*) FROM universe WHERE source = 'sp500'`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, line := range strings.Split(strings.TrimSpace(sp500Tickers), "\n") {
		ticker := strings.TrimSpace(line)
		if ticker == "" {
			continue
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO universe (ticker, source) VALUES (?, 'sp500')`, ticker); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// seedTW150 is seedSP500's Phase 6 PR2 mirror for the 'tw' source tier: same
// count-query-not-migration gate (checked separately from 'sp500', so a
// fresh install seeds both tiers independently), same deliberately-not-
// re-synced-on-every-startup behavior so a manual /universe remove of a
// seeded TW ticker sticks. There is no TW equivalent of SyncSP500 in this
// phase (see tw150Tickers' doc comment) — refreshing the embedded list is a
// manual re-generate-and-PR process, not an automatic startup diff.
func (d *DB) seedTW150() error {
	var count int
	if err := d.conn.QueryRow(`SELECT COUNT(*) FROM universe WHERE source = 'tw'`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, line := range strings.Split(strings.TrimSpace(tw150Tickers), "\n") {
		ticker := strings.TrimSpace(line)
		if ticker == "" {
			continue
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO universe (ticker, source) VALUES (?, 'tw')`, ticker); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// AddUniverseTicker adds ticker to the scan pool under source (e.g. "manual"
// for /universe add). A ticker already present keeps its existing source.
// Re-adding a previously tombstoned (removed=1) ticker clears the tombstone
// so it becomes active again — without the ON CONFLICT clause, the ticker's
// existing row would make this a silent no-op (Add would report success
// while the ticker stayed absent from GetUniverse's active pool).
func (d *DB) AddUniverseTicker(ticker, source string) error {
	_, err := d.conn.Exec(`
		INSERT INTO universe (ticker, source) VALUES (?, ?)
		ON CONFLICT(ticker) DO UPDATE SET removed = 0`,
		ticker, source,
	)
	return err
}

// RemoveUniverseTicker soft-deletes ticker (sets removed=1) rather than
// hard-deleting the row, regardless of source — see docs/phase-2.6-universe-
// refresh.md. A tombstoned row is what lets SyncSP500 tell "the user
// removed this" apart from "this ticker was never seeded" once a future
// embedded-list refresh no longer contains it; a hard DELETE would erase
// that distinction. Applying this uniformly to 'manual' tickers too (rather
// than branching on source) costs nothing — no reader treats a tombstoned
// manual row specially — and avoids a code path with no behavioral payoff.
func (d *DB) RemoveUniverseTicker(ticker string) error {
	_, err := d.conn.Exec(`UPDATE universe SET removed = 1 WHERE ticker = ?`, ticker)
	return err
}

// GetUniverse returns every active (non-tombstoned) ticker in the scan
// pool, ordered by ticker. RunUniverseScan and /universe's summary are its
// only two readers, and both want "today's actual scan pool" — a
// removed=1 row must not appear in either.
func (d *DB) GetUniverse() ([]UniverseEntry, error) {
	rows, err := d.conn.Query(`SELECT ticker, source, added_at FROM universe WHERE removed = 0 ORDER BY ticker`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []UniverseEntry
	for rows.Next() {
		var e UniverseEntry
		if err := rows.Scan(&e.Ticker, &e.Source, &e.AddedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// SyncSP500 diffs the embedded S&P 500 ticker list against universe's
// existing rows (see docs/phase-2.6-universe-refresh.md), called once at
// startup (after seedSP500, which only runs on a fresh install) rather than
// on a cron — a re-synced embedded list only ever changes via a merged PR,
// and the daily-scheduled deploy already restarts the process on every
// such merge, so an on-restart diff is bound by deploy latency (at most a
// day) without needing a dedicated schedule of its own.
//
// added: an embedded ticker entirely absent from universe (any source) is
// auto-inserted with source='sp500' and returned here — nothing for the
// user to decide, a new index constituent is unambiguously worth scanning.
//
// delisted: an un-removed 'sp500' row whose ticker is no longer in the
// embedded list is returned here but the row itself is never touched —
// dropped from the index isn't the same as delisted/dead, and whether to
// stop scanning it is the user's call (via /universe remove), not this
// method's. 'manual' rows are never candidates for delisted, regardless of
// whether they happen to also appear in the embedded list.
//
// Tombstoned (removed=1) rows are skipped by this diff entirely, whatever
// their source or embedded-list membership — that's what makes a manual
// /universe remove of a seeded ticker stick against a future SyncSP500 call.
//
// A no-op run (added and delisted both empty) is the overwhelmingly common
// case, since deploy restarts happen far more often than the embedded list
// actually changes — callers must render that as complete silence (see
// bot.SyncUniverse), not a Telegram message, or a daily-restart deploy
// cadence would mean a near-daily notification about nothing.
func (d *DB) SyncSP500() (added, delisted []string, err error) {
	embedded := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(sp500Tickers), "\n") {
		if ticker := strings.TrimSpace(line); ticker != "" {
			embedded[ticker] = true
		}
	}

	rows, err := d.conn.Query(`SELECT ticker, source, removed FROM universe`)
	if err != nil {
		return nil, nil, err
	}
	type existingRow struct {
		source  string
		removed bool
	}
	existing := make(map[string]existingRow)
	for rows.Next() {
		var ticker, source string
		var removed bool
		if err := rows.Scan(&ticker, &source, &removed); err != nil {
			rows.Close()
			return nil, nil, err
		}
		existing[ticker] = existingRow{source, removed}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, err
	}
	rows.Close()

	for t := range embedded {
		if _, ok := existing[t]; !ok {
			added = append(added, t)
		}
	}
	sort.Strings(added)

	for t, e := range existing {
		if e.source == "sp500" && !e.removed && !embedded[t] {
			delisted = append(delisted, t)
		}
	}
	sort.Strings(delisted)

	if len(added) == 0 {
		return added, delisted, nil
	}

	tx, err := d.conn.Begin()
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	for _, t := range added {
		if _, err := tx.Exec(`INSERT INTO universe (ticker, source) VALUES (?, 'sp500')`, t); err != nil {
			return nil, nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	return added, delisted, nil
}

// SaveScanHit logs that ticker's daily universe scan found reason (a signal
// message) on date. Multiple hits for the same ticker/date are allowed — read
// back grouped by GetScanHits.
func (d *DB) SaveScanHit(ticker, date, reason string) error {
	_, err := d.conn.Exec(`INSERT INTO scan_hits (ticker, date, reason) VALUES (?, ?, ?)`, ticker, date, reason)
	return err
}

// GetScanHits returns every scan hit logged for date, keyed by ticker with
// multiple reasons for the same ticker joined by "; ".
func (d *DB) GetScanHits(date string) (map[string]string, error) {
	rows, err := d.conn.Query(`SELECT ticker, reason FROM scan_hits WHERE date = ? ORDER BY ticker, created_at`, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]string)
	for rows.Next() {
		var ticker, reason string
		if err := rows.Scan(&ticker, &reason); err != nil {
			return nil, err
		}
		if existing, ok := out[ticker]; ok {
			out[ticker] = existing + "; " + reason
		} else {
			out[ticker] = reason
		}
	}
	return out, rows.Err()
}
