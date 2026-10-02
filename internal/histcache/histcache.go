// Package histcache is a persistent read-through cache in front of a
// data.HistoryProvider: daily candles live in their own SQLite file, so a
// chart/peers/watchlist view is a local read instead of a fresh Yahoo call,
// and a Yahoo outage degrades to slightly stale bars instead of an empty
// chart.
//
// It is deliberately NOT part of argus.db. Prices are rebuildable and the
// ledger is not, argus.db's nightly VACUUM INTO backup would otherwise copy
// (and hold a read on) hundreds of MB of re-fetchable data, and a bulk write
// here must not contend with the single-writer ledger. Losing this file
// costs one re-fetch per ticker, nothing else.
//
// It is also NOT a backtest data source. Only tickers somebody has looked at
// ever get in, i.e. today's survivors — exactly the survivorship bias
// data/research's Sinopac point-in-time CSVs were built to avoid.
//
// Two correctness traps shape the refresh logic:
//
//   - Corporate actions. Bars are stored as the provider returned them, so a
//     later split leaves old rows on a different scale from new ones (a
//     fake 4:1 cliff on the chart). Every tail refresh therefore re-checks
//     its overlap with what is stored and re-pulls the whole ticker on any
//     disagreement, or when there is no overlap to check.
//   - Partial bars. The bar for a session still in progress is stored like
//     any other, then overwritten by the next tail refresh; the overlap
//     check skips bars from the previous fetch's own day for that reason.
package histcache

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"argus/internal/data"
	"argus/internal/logger"
)

const (
	// fullRange is what a cold or invalidated ticker is filled with — the
	// same window data.Yahoo already rewrites "max" to, and the oldest
	// window this cache serves (see rangeStart).
	fullRange = "10y"
	// tailRange is the cheap incremental refresh: about a month of bars,
	// enough overlap to notice a split without re-downloading a decade.
	tailRange = "1mo"
	// refreshTTL bounds how stale the newest bar can get while the market is
	// open. The previous behaviour was "fresh on every view"; 5 minutes keeps
	// the chart's last candle close to that at a fraction of the requests.
	refreshTTL = 5 * time.Minute
	// splitTolerance is the relative close difference above which stored and
	// fresh bars are considered to disagree. Same source, same series, so
	// honest values match exactly; any real corporate action moves the
	// scale by tens of percent.
	splitTolerance = 0.001
	maxYears       = 10
	dateLayout     = "2006-01-02"
)

// errStore marks a failure of the local file, as opposed to the provider, so
// GetHistory can fall back to the provider instead of failing the caller over
// what is only an optimisation.
var errStore = errors.New("histcache store")

// schema is CREATE IF NOT EXISTS with no migration ladder on purpose: the
// file is rebuildable, so a schema change means deleting it.
const schema = `
CREATE TABLE IF NOT EXISTS candles (
	ticker TEXT NOT NULL,
	date   TEXT NOT NULL,
	ts     INTEGER NOT NULL,
	open   REAL NOT NULL,
	high   REAL NOT NULL,
	low    REAL NOT NULL,
	close  REAL NOT NULL,
	volume INTEGER NOT NULL,
	PRIMARY KEY (ticker, date)
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS candle_fetches (
	ticker     TEXT PRIMARY KEY,
	fetched_at INTEGER NOT NULL
);`

// Cache implements data.HistoryProvider.
type Cache struct {
	db    *sql.DB
	inner data.HistoryProvider
	now   func() time.Time
	locks sync.Map // ticker -> *sync.Mutex, so a chart and its peers card don't both cold-fetch the same ticker
}

var _ data.HistoryProvider = (*Cache)(nil)

// Open creates (or reuses) the cache file at path in front of inner.
func Open(path string, inner data.HistoryProvider) (*Cache, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	// WAL so chart reads never wait on a refresh's write; synchronous=NORMAL
	// because a lost last write here is just a re-fetch.
	conn, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)", path))
	if err != nil {
		return nil, err
	}
	if _, err := conn.Exec(schema); err != nil {
		conn.Close()
		return nil, err
	}
	return &Cache{db: conn, inner: inner, now: time.Now}, nil
}

func (c *Cache) Close() error { return c.db.Close() }

var rangeRe = regexp.MustCompile(`^(\d+)(mo|y)$`)

// rangeStart turns a Yahoo chart-API range value into the first calendar day
// the caller wants. ok is false for anything this cache can't answer from
// ten years of daily bars (intraday-ish "5d", unknown strings, windows past
// ten years) — those go straight to the provider.
func rangeStart(rangeParam string, now time.Time) (time.Time, bool) {
	var from time.Time
	switch rangeParam {
	case "":
		from = now.AddDate(-1, 0, 0)
	case "max":
		from = now.AddDate(-maxYears, 0, 0)
	case "ytd":
		from = time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location())
	default:
		m := rangeRe.FindStringSubmatch(rangeParam)
		if m == nil {
			return time.Time{}, false
		}
		n, _ := strconv.Atoi(m[1])
		if m[2] == "mo" {
			from = now.AddDate(0, -n, 0)
		} else {
			from = now.AddDate(-n, 0, 0)
		}
	}
	if from.Before(now.AddDate(-maxYears, 0, 0)) {
		return time.Time{}, false
	}
	return from, true
}

// GetHistory serves rangeParam from the local store, refreshing it first when
// stale. Signature and ordering (oldest first) match data.Yahoo.GetHistory.
func (c *Cache) GetHistory(ticker, rangeParam string) ([]data.Candle, error) {
	now := c.now()
	from, ok := rangeStart(rangeParam, now)
	if !ok {
		return c.inner.GetHistory(ticker, rangeParam)
	}

	mu := c.lock(ticker)
	mu.Lock()
	err := c.refresh(ticker, now)
	mu.Unlock()
	if err == nil {
		var candles []data.Candle
		if candles, err = c.read(ticker, from); err == nil {
			return candles, nil
		}
	}
	if errors.Is(err, errStore) {
		logger.Warnf("histcache: %s: %v (falling back to the provider)", ticker, err)
		return c.inner.GetHistory(ticker, rangeParam)
	}
	return nil, err
}

func (c *Cache) lock(ticker string) *sync.Mutex {
	m, _ := c.locks.LoadOrStore(ticker, &sync.Mutex{})
	return m.(*sync.Mutex)
}

// refresh brings ticker's stored bars up to date: a full pull when unknown or
// contradicted by fresh data, a one-month tail otherwise, nothing inside the
// TTL. A failed tail refresh is not an error — the stored bars are still the
// best answer available — but a failed full pull is, since there is nothing
// (trustworthy) to serve instead.
func (c *Cache) refresh(ticker string, now time.Time) error {
	prev, found, err := c.lastFetch(ticker)
	if err != nil {
		return err
	}
	if !found {
		return c.full(ticker, now)
	}
	if now.Sub(prev) < refreshTTL {
		return nil
	}
	tail, err := c.inner.GetHistory(ticker, tailRange)
	if err != nil {
		// ponytail: during a provider outage every view still pays the
		// provider's timeout before serving stale bars; add a per-ticker
		// backoff if that ever hurts.
		logger.Warnf("histcache: %s: tail refresh failed, serving stored bars: %v", ticker, err)
		return nil
	}
	agrees, err := c.overlapAgrees(ticker, tail, prev)
	if err != nil {
		return err
	}
	if !agrees {
		logger.Infof("histcache: %s: stored bars disagree with fresh ones (split/adjustment?), re-fetching %s", ticker, fullRange)
		return c.full(ticker, now)
	}
	return c.store(ticker, tail, now, false)
}

func (c *Cache) full(ticker string, now time.Time) error {
	candles, err := c.inner.GetHistory(ticker, fullRange)
	if err != nil {
		return err
	}
	return c.store(ticker, candles, now, true)
}

func (c *Cache) lastFetch(ticker string) (time.Time, bool, error) {
	var unix int64
	err := c.db.QueryRow(`SELECT fetched_at FROM candle_fetches WHERE ticker = ?`, ticker).Scan(&unix)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("%w: %v", errStore, err)
	}
	return time.Unix(unix, 0), true, nil
}

// overlapAgrees compares fresh tail bars with stored ones on the days both
// have. Days on or after the previous fetch's own date are skipped — that
// stored bar may have been a session still in progress. Zero comparable days
// counts as disagreement: a gap is exactly where an unseen split would hide.
func (c *Cache) overlapAgrees(ticker string, tail []data.Candle, prev time.Time) (bool, error) {
	rows, err := c.db.Query(`SELECT date, close FROM candles WHERE ticker = ? AND date < ?`, ticker, prev.Format(dateLayout))
	if err != nil {
		return false, fmt.Errorf("%w: %v", errStore, err)
	}
	defer rows.Close()
	stored := map[string]float64{}
	for rows.Next() {
		var d string
		var closePx float64
		if err := rows.Scan(&d, &closePx); err != nil {
			return false, fmt.Errorf("%w: %v", errStore, err)
		}
		stored[d] = closePx
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("%w: %v", errStore, err)
	}

	overlap := 0
	for _, b := range tail {
		old, ok := stored[b.Date.Format(dateLayout)]
		if !ok {
			continue
		}
		overlap++
		if math.Abs(old-b.Close) > splitTolerance*math.Abs(b.Close) {
			return false, nil
		}
	}
	return overlap > 0, nil
}

// store upserts candles and stamps the fetch time, replacing the ticker's
// whole history first when replace is set. One transaction, so a crash never
// leaves rows from two different price scales side by side.
func (c *Cache) store(ticker string, candles []data.Candle, now time.Time, replace bool) error {
	tx, err := c.db.Begin()
	if err != nil {
		return fmt.Errorf("%w: %v", errStore, err)
	}
	defer tx.Rollback()

	if replace {
		if _, err := tx.Exec(`DELETE FROM candles WHERE ticker = ?`, ticker); err != nil {
			return fmt.Errorf("%w: %v", errStore, err)
		}
	}
	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO candles (ticker, date, ts, open, high, low, close, volume) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("%w: %v", errStore, err)
	}
	defer stmt.Close()
	for _, b := range candles {
		if b.Date.IsZero() { // yahoo.go leaves Date unset when its timestamp array is short
			continue
		}
		if _, err := stmt.Exec(ticker, b.Date.Format(dateLayout), b.Date.Unix(), b.Open, b.High, b.Low, b.Close, b.Volume); err != nil {
			return fmt.Errorf("%w: %v", errStore, err)
		}
	}
	if _, err := tx.Exec(`INSERT OR REPLACE INTO candle_fetches (ticker, fetched_at) VALUES (?, ?)`, ticker, now.Unix()); err != nil {
		return fmt.Errorf("%w: %v", errStore, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%w: %v", errStore, err)
	}
	return nil
}

// read returns stored bars on or after from, oldest first. Dates come back as
// the original Unix instant (time.Unix, Local) rather than a re-parsed
// midnight, so consumers that compare against the provider's own timestamps
// see exactly what they saw before this cache existed.
func (c *Cache) read(ticker string, from time.Time) ([]data.Candle, error) {
	rows, err := c.db.Query(`SELECT ts, open, high, low, close, volume FROM candles WHERE ticker = ? AND date >= ? ORDER BY date`, ticker, from.Format(dateLayout))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errStore, err)
	}
	defer rows.Close()
	var out []data.Candle
	for rows.Next() {
		var ts int64
		var b data.Candle
		if err := rows.Scan(&ts, &b.Open, &b.High, &b.Low, &b.Close, &b.Volume); err != nil {
			return nil, fmt.Errorf("%w: %v", errStore, err)
		}
		b.Date = time.Unix(ts, 0)
		// Rows stored before data.Yahoo stopped returning filler bars are
		// still in the file, and a tail refresh never deletes a day it no
		// longer returns.
		if b.IsFiller() {
			continue
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: %v", errStore, err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("histcache: no candles for %s since %s", ticker, from.Format(dateLayout))
	}
	return out, nil
}
