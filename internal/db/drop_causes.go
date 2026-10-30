package db

import (
	"encoding/json"
	"strings"
)

// DropNews is one headline kept with a drop. Stored as JSON on the row: it is
// only ever read back whole, with its drop.
type DropNews struct {
	Headline    string `json:"headline"`
	Source      string `json:"source"`
	URL         string `json:"url"`
	PublishedAt string `json:"publishedAt"` // RFC3339, "" when the source gave none
}

// DropCause is what was recorded when a watchlist ticker's drop was first seen
// (migration 41). News is always there; Cause, Secondary, Confidence and
// Summary stay empty when the LLM wasn't called (past the run's cap), failed,
// or couldn't tell from the headlines. Descriptive only, like the drop
// detection itself: nothing may act on it.
type DropCause struct {
	Ticker     string
	Market     string
	StartDate  string // the drop's first falling bar when it was recorded
	EndDate    string // the bar it was recorded on
	Cause      string // "" | "earn" | "macro" | "sector" | "flow" | "corp" | "tech"
	Secondary  []string
	Confidence string // "" | "high" | "mid"
	Summary    string
	News       []DropNews
}

// SaveDropCause stores c, keeping the row already there for (ticker, start).
func (d *DB) SaveDropCause(c DropCause) error {
	news, err := json.Marshal(c.News)
	if err != nil {
		return err
	}
	_, err = d.conn.Exec(
		`INSERT OR IGNORE INTO drop_causes (ticker, market, start_date, end_date, cause, secondary, confidence, summary, news)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Ticker, c.Market, c.StartDate, c.EndDate, c.Cause, strings.Join(c.Secondary, ","), c.Confidence, c.Summary, string(news),
	)
	return err
}

// DropCausesFor returns ticker's recorded drops, oldest first.
func (d *DB) DropCausesFor(ticker string) ([]DropCause, error) {
	rows, err := d.conn.Query(
		`SELECT ticker, market, start_date, end_date, cause, secondary, confidence, summary, news
		 FROM drop_causes WHERE ticker = ? ORDER BY start_date`, ticker,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DropCause
	for rows.Next() {
		var c DropCause
		var secondary, news string
		if err := rows.Scan(&c.Ticker, &c.Market, &c.StartDate, &c.EndDate, &c.Cause, &secondary, &c.Confidence, &c.Summary, &news); err != nil {
			return nil, err
		}
		if secondary != "" {
			c.Secondary = strings.Split(secondary, ",")
		}
		if err := json.Unmarshal([]byte(news), &c.News); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
