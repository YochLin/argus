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

// SavePodcastInsight inserts one insight row. It doesn't dedup by itself —
// handlePodcast is what keeps one episode from accumulating near-duplicate
// rows across repeat /podcast runs, by calling DeletePodcastInsightsByURL
// first when CountPodcastInsightsByURL says the URL already has some.
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
// can warn the user their existing insights for this episode are about to
// be replaced.
func (d *DB) CountPodcastInsightsByURL(sourceURL string) (int, error) {
	var count int
	err := d.conn.QueryRow(`SELECT COUNT(*) FROM podcast_insights WHERE source_url = ?`, sourceURL).Scan(&count)
	return count, err
}

// DeletePodcastInsightsByURL removes every existing row for sourceURL.
// handlePodcast calls this right before saving a fresh batch when
// re-processing a URL that already has insights on record, so a given
// episode always holds exactly one current set of insights — overwritten on
// re-run, not accumulated — which matters because podcast_insights exists
// to be aggregated across episodes later (e.g. "was 股癌 net bullish or
// bearish on X this month"), and a duplicated episode would double-count
// itself in that read.
func (d *DB) DeletePodcastInsightsByURL(sourceURL string) error {
	_, err := d.conn.Exec(`DELETE FROM podcast_insights WHERE source_url = ?`, sourceURL)
	return err
}
