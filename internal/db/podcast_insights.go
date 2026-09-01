package db

// PodcastInsight is one LLM-extracted market observation from a podcast
// transcript pasted via /podcast (migration 25). Ticker/Market are "" for a
// macro-only observation with no single stock attached (see
// internal/llm.ExtractPodcastInsights). DerivedFrom (migration 26) is "" for
// a row grounded in something the transcript actually said, and non-empty
// for a downstream-beneficiary row the model inferred from its own
// supply-chain knowledge (e.g. Ticker=2330, DerivedFrom="NVDA: GPU demand")
// — see migration 26's comment for why that distinction matters.
type PodcastInsight struct {
	ID          int64
	SourceURL   string
	SourceTitle string
	Ticker      string
	Market      string
	Stance      string
	Thesis      string
	DerivedFrom string
	CreatedAt   string
}

// SavePodcastInsight inserts one insight row. Deliberately no dedup — same
// append-only convention as SaveLesson, so re-running /podcast on the same
// episode adds fresh rows rather than silently no-op-ing (see
// CountPodcastInsightsByURL for the warn-don't-block guard against doing
// this by accident).
func (d *DB) SavePodcastInsight(p PodcastInsight) error {
	_, err := d.conn.Exec(
		`INSERT INTO podcast_insights (source_url, source_title, ticker, market, stance, thesis, derived_from)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.SourceURL, p.SourceTitle, p.Ticker, p.Market, p.Stance, p.Thesis, p.DerivedFrom,
	)
	return err
}

// CountPodcastInsightsByURL returns how many insight rows already exist for
// sourceURL. handlePodcast calls this before (re-)processing a link so it
// can warn the user a duplicate submission is about to double-count that
// episode in any future cross-episode aggregation (the whole reason
// podcast_insights exists) — it's a warning, not a block, since re-running
// on purpose (e.g. after a prompt improvement) is legitimate too.
func (d *DB) CountPodcastInsightsByURL(sourceURL string) (int, error) {
	var count int
	err := d.conn.QueryRow(`SELECT COUNT(*) FROM podcast_insights WHERE source_url = ?`, sourceURL).Scan(&count)
	return count, err
}
