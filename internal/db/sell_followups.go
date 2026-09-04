package db

// SellFollowup is one row from Phase 26's post-sell follow-up review
// (migration 27, see docs/phase-26-sell-followup.md) — the outcome of
// looking back at a fully closed round trip 5 trading days after the exit.
type SellFollowup struct {
	ID            int64
	Ticker        string
	Market        string
	ExitDate      string
	ReviewDate    string
	ExitPrice     float64
	PriceAtReview float64
	PctSinceExit  float64
	Verdict       string
	CreatedAt     string
}

// HasSellFollowup reports whether ticker already has a sell_followups row
// for exitDate — bot.checkSellFollowups' dedup check before doing any work,
// same shape as HasPriceEvent.
func (d *DB) HasSellFollowup(ticker, exitDate string) (bool, error) {
	var exists bool
	err := d.conn.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM sell_followups WHERE ticker = ? AND exit_date = ?)`,
		ticker, exitDate,
	).Scan(&exists)
	return exists, err
}

// SaveSellFollowup inserts one sell_followups row. Returns an error if the
// (ticker, exit_date) unique index is violated; callers should log and skip
// rather than treat that as fatal, same convention as SavePriceEvent.
func (d *DB) SaveSellFollowup(f SellFollowup) error {
	_, err := d.conn.Exec(
		`INSERT INTO sell_followups (ticker, market, exit_date, review_date, exit_price, price_at_review, pct_since_exit, verdict) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		f.Ticker, f.Market, f.ExitDate, f.ReviewDate, f.ExitPrice, f.PriceAtReview, f.PctSinceExit, f.Verdict,
	)
	return err
}
