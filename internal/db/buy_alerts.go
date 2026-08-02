package db

import (
	"time"

	"argus/internal/market"
)

// Buy-alert directions: which side of the target price triggers the alert.
// Below is a dip-buy watch ("tell me if it drops to/below X"); Above is a
// breakout watch ("tell me if it rises to/above X"). The caller (bot-layer
// addBuyAlert) infers which one applies by comparing the target price to the
// live quote at creation time, and stores the resolved direction here rather
// than re-deriving it on every check.
const (
	BuyAlertBelow = "below"
	BuyAlertAbove = "above"
)

// BuyAlert is one user-set price point to watch for a ticker the user may or
// may not currently hold. Unlike Position.StopPrice (a single column on a
// one-row-per-ticker table), a ticker can have any number of BuyAlert rows —
// see migration 14's doc comment in db.go.
type BuyAlert struct {
	ID        int64
	Ticker    string
	Market    string
	Price     float64
	Direction string
	CreatedAt time.Time
}

// AddBuyAlert inserts a new price-point watch for ticker, deriving Market via
// market.Of the same way AddTicker does. direction should be BuyAlertBelow or
// BuyAlertAbove.
func (d *DB) AddBuyAlert(ticker string, price float64, direction string) (int64, error) {
	res, err := d.conn.Exec(
		`INSERT INTO buy_alerts (ticker, market, price, direction) VALUES (?, ?, ?, ?)`,
		ticker, string(market.Of(ticker)), price, direction,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetBuyAlertsByTicker returns every buy alert set on ticker, oldest first —
// backs /buyalert TICKER's list branch and removal-by-price lookup.
func (d *DB) GetBuyAlertsByTicker(ticker string) ([]BuyAlert, error) {
	rows, err := d.conn.Query(
		`SELECT id, ticker, market, price, direction, created_at FROM buy_alerts WHERE ticker = ? ORDER BY id`,
		ticker,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []BuyAlert
	for rows.Next() {
		var a BuyAlert
		if err := rows.Scan(&a.ID, &a.Ticker, &a.Market, &a.Price, &a.Direction, &a.CreatedAt); err != nil {
			return nil, err
		}
		alerts = append(alerts, a)
	}
	return alerts, rows.Err()
}

// GetBuyAlertsByMarket returns every buy alert in market m, ticker then id
// order — feeds the daily-report/closing-snapshot alert check (both are
// already per-market) and the web watchlist-summary endpoint.
func (d *DB) GetBuyAlertsByMarket(m market.MarketID) ([]BuyAlert, error) {
	rows, err := d.conn.Query(
		`SELECT id, ticker, market, price, direction, created_at FROM buy_alerts WHERE market = ? ORDER BY ticker, id`,
		string(m),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []BuyAlert
	for rows.Next() {
		var a BuyAlert
		if err := rows.Scan(&a.ID, &a.Ticker, &a.Market, &a.Price, &a.Direction, &a.CreatedAt); err != nil {
			return nil, err
		}
		alerts = append(alerts, a)
	}
	return alerts, rows.Err()
}

// RemoveBuyAlert deletes a single buy alert by id.
func (d *DB) RemoveBuyAlert(id int64) error {
	_, err := d.conn.Exec(`DELETE FROM buy_alerts WHERE id = ?`, id)
	return err
}
