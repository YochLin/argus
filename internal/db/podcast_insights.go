package db

// PodcastInsight is one LLM-extracted market observation from a podcast
// transcript pasted via /podcast (migration 25). Ticker/Market are "" for a
// macro-only observation with no single stock attached (see
// internal/llm.ExtractPodcastInsights).
type PodcastInsight struct {
	ID          int64
	SourceURL   string
	SourceTitle string
	Ticker      string
	Market      string
	Stance      string
	Thesis      string
	CreatedAt   string
}

// SavePodcastInsight inserts one insight row. Deliberately no dedup — same
// append-only convention as SaveLesson, so re-running /podcast on the same
// episode adds fresh rows rather than silently no-op-ing.
func (d *DB) SavePodcastInsight(p PodcastInsight) error {
	_, err := d.conn.Exec(
		`INSERT INTO podcast_insights (source_url, source_title, ticker, market, stance, thesis)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		p.SourceURL, p.SourceTitle, p.Ticker, p.Market, p.Stance, p.Thesis,
	)
	return err
}
