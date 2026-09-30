package db

import "time"

// FillNews is one headline captured when a trade was recorded (migration 36).
// Sentiment is "" until the LLM has classified the row; Tag and Major are only
// meaningful once it has.
type FillNews struct {
	ID          int64
	Ticker      string
	FillDate    string // the fill's own YYYY-MM-DD
	Headline    string
	Source      string
	URL         string
	PublishedAt time.Time // zero when the provider gave no timestamp
	Sentiment   string    // "" | "bull" | "bear" | "neutral"
	Tag         string    // "" | "earn" | "guide" | "analyst" | "sector" | "macro" | "flow" | "other"
	Major       bool
}

// SaveFillNews stores items under (ticker, fillDate). A headline already stored
// for that ticker/day is skipped, so recording a second fill the same day, or
// retrying a capture, never duplicates rows or resets their labels.
func (d *DB) SaveFillNews(ticker, fillDate string, items []FillNews) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, n := range items {
		pub := ""
		if !n.PublishedAt.IsZero() {
			pub = n.PublishedAt.UTC().Format(time.RFC3339)
		}
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO fill_news (ticker, fill_date, headline, source, url, published_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			ticker, fillDate, n.Headline, n.Source, n.URL, pub,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// FillNewsFor returns the news stored for (ticker, fillDate), oldest first.
func (d *DB) FillNewsFor(ticker, fillDate string) ([]FillNews, error) {
	rows, err := d.conn.Query(
		`SELECT id, ticker, fill_date, headline, source, url, published_at, sentiment, tag, major
		 FROM fill_news WHERE ticker = ? AND fill_date = ?
		 ORDER BY CASE WHEN published_at = '' THEN 1 ELSE 0 END, published_at, id`,
		ticker, fillDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FillNews
	for rows.Next() {
		var n FillNews
		var pub string
		var major int
		if err := rows.Scan(&n.ID, &n.Ticker, &n.FillDate, &n.Headline, &n.Source, &n.URL, &pub, &n.Sentiment, &n.Tag, &major); err != nil {
			return nil, err
		}
		n.PublishedAt, _ = time.Parse(time.RFC3339, pub)
		n.Major = major != 0
		out = append(out, n)
	}
	return out, rows.Err()
}

// SetFillNewsLabel records the LLM's reading of one stored headline.
func (d *DB) SetFillNewsLabel(id int64, sentiment, tag string, major bool) error {
	m := 0
	if major {
		m = 1
	}
	_, err := d.conn.Exec(`UPDATE fill_news SET sentiment = ?, tag = ?, major = ? WHERE id = ?`, sentiment, tag, m, id)
	return err
}
