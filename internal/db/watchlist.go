package db

import (
	"time"

	"argus/internal/market"
)

type Watchlist struct {
	Ticker  string
	AddedAt time.Time
}

func (d *DB) AddTicker(ticker string) error {
	_, err := d.conn.Exec(`INSERT OR IGNORE INTO watchlist (ticker, market) VALUES (?, ?)`, ticker, string(market.Of(ticker)))
	return err
}

func (d *DB) RemoveTicker(ticker string) error {
	_, err := d.conn.Exec(`DELETE FROM watchlist WHERE ticker = ?`, ticker)
	return err
}

// GetWatchlist returns every watchlist ticker regardless of market, for
// callers that want "the whole list" (chat context injection, /list,
// /status with no argument) — see GetWatchlistByMarket for a single-market
// query.
func (d *DB) GetWatchlist() ([]string, error) {
	rows, err := d.conn.Query(`SELECT ticker FROM watchlist ORDER BY ticker`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tickers []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		tickers = append(tickers, t)
	}
	return tickers, rows.Err()
}

// GetWatchlistByMarket returns watchlist tickers for one market only (Phase
// 6, market.US or market.TW) — used by the per-market jobs (closing
// snapshot, and PR1's US-only daily-report/recommend input gathering) that
// must not mix the two.
func (d *DB) GetWatchlistByMarket(m market.MarketID) ([]string, error) {
	rows, err := d.conn.Query(`SELECT ticker FROM watchlist WHERE market = ? ORDER BY ticker`, string(m))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tickers []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		tickers = append(tickers, t)
	}
	return tickers, rows.Err()
}
