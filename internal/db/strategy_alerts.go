package db

// Where a pushed strategy signal came from (strategy_alerts.channel).
const (
	AlertChannelWatchlist = "watchlist" // watchlist/position ticker, daily report
	AlertChannelScan      = "scan"      // universe-scan hit
)

// StrategyAlert is one strategy signal that was pushed to the user
// (migration 37). Strategy is the signals.Type* value, SignalDate the last
// candle it was computed on, Message the text that was sent.
type StrategyAlert struct {
	ID         int64
	Ticker     string
	Strategy   string
	SignalDate string
	Channel    string
	Message    string
}

// SaveStrategyAlert records one pushed strategy signal. The same
// (ticker, strategy, signal_date) is kept once, so a rerun that surfaces the
// same bar again can't double it.
func (d *DB) SaveStrategyAlert(ticker, strategy, signalDate, channel, message string) error {
	_, err := d.conn.Exec(
		`INSERT OR IGNORE INTO strategy_alerts (ticker, strategy, signal_date, channel, message)
		 VALUES (?, ?, ?, ?, ?)`,
		ticker, strategy, signalDate, channel, message)
	return err
}

// StrategyAlertsFor returns every recorded alert for ticker, oldest first.
func (d *DB) StrategyAlertsFor(ticker string) ([]StrategyAlert, error) {
	rows, err := d.conn.Query(
		`SELECT id, ticker, strategy, signal_date, channel, message
		 FROM strategy_alerts WHERE ticker = ? ORDER BY signal_date, id`, ticker)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StrategyAlert
	for rows.Next() {
		var a StrategyAlert
		if err := rows.Scan(&a.ID, &a.Ticker, &a.Strategy, &a.SignalDate, &a.Channel, &a.Message); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
