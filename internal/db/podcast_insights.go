package db

// PodcastInsight is one LLM-extracted market observation from a podcast
// transcript pasted via /podcast (migration 25). Ticker/Market are "" for a
// macro-only observation with no single stock attached (see
// internal/llm.ExtractPodcastInsights). DerivedFrom is "" for a row grounded
// in something the transcript actually said, and non-empty for a
// downstream-beneficiary row the model inferred from its own supply-chain
// knowledge (e.g. Ticker=2330, DerivedFrom="NVDA: GPU demand") — see
// migration 25's comment for why that distinction matters.
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
// episode adds fresh rows rather than silently no-op-ing.
func (d *DB) SavePodcastInsight(p PodcastInsight) error {
	_, err := d.conn.Exec(
		`INSERT INTO podcast_insights (source_url, source_title, ticker, market, stance, thesis, derived_from)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.SourceURL, p.SourceTitle, p.Ticker, p.Market, p.Stance, p.Thesis, p.DerivedFrom,
	)
	return err
}
