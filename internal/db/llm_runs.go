package db

import (
	"database/sql"

	"argus/internal/market"
)

// LLMRun is one row of Phase 19's audit trail (migration 20) — see
// docs/phase-19-llm-transparency.md. Kind distinguishes a manual /recommend
// call from a scheduled daily-report run ("recommend" / "daily_report");
// Model/LatencyMs come straight from llm.Client.GenerateRecommendations'
// return values (no tokens column — ACP has no per-token usage to report).
// WatchlistCount/CandidateCount/NewsCount are computed once at write time
// (see recordLLMRun in internal/bot/pipeline.go) so the /llm run list can
// show a summary without pulling every row's multi-hundred-KB input_json
// over the wire.
type LLMRun struct {
	ID             int64
	Kind           string
	Market         string
	Model          string
	LatencyMs      int64
	CreatedAt      string
	WatchlistCount int
	CandidateCount int
	NewsCount      int
	CandleGapCount int
}

// LLMRunDetail is LLMRun plus the full recorded input/output — InputJSON is
// the unprojected json.Marshal of that run's recommendationInputs (see
// pipeline.go's recordLLMRun: no field ever gets trimmed, since a missing
// field would defeat the point of a transparency feature), and OutputRaw is
// the model's reply exactly as received, even on a parse failure.
type LLMRunDetail struct {
	LLMRun
	InputJSON string
	OutputRaw string
}

// InsertLLMRun records one GenerateRecommendations call. Callers should only
// log a failure here, never abort the /recommend flow it's auditing (see
// pipeline.go's recordLLMRun).
func (d *DB) InsertLLMRun(kind string, m market.MarketID, model string, latencyMs int64, inputJSON, outputRaw string, watchlistCount, candidateCount, newsCount, candleGapCount int) error {
	_, err := d.conn.Exec(
		`INSERT INTO llm_runs (kind, market, model, latency_ms, input_json, output_raw, watchlist_count, candidate_count, news_count, candle_gap_count)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		kind, string(m), model, latencyMs, inputJSON, outputRaw, watchlistCount, candidateCount, newsCount, candleGapCount,
	)
	return err
}

// ListLLMRuns returns the most recent limit runs, newest first, without
// input_json/output_raw — the /llm list view's data source (§4.4's
// GET /api/llm-runs).
func (d *DB) ListLLMRuns(limit int) ([]LLMRun, error) {
	rows, err := d.conn.Query(
		`SELECT id, kind, market, model, latency_ms, created_at, watchlist_count, candidate_count, news_count, candle_gap_count
		 FROM llm_runs ORDER BY id DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LLMRun
	for rows.Next() {
		var r LLMRun
		if err := rows.Scan(&r.ID, &r.Kind, &r.Market, &r.Model, &r.LatencyMs, &r.CreatedAt, &r.WatchlistCount, &r.CandidateCount, &r.NewsCount, &r.CandleGapCount); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetLLMRun returns one run's full detail (id, ok=false if not found) — the
// /llm detail view's data source (§4.4's GET /api/llm-runs/{id}).
func (d *DB) GetLLMRun(id int64) (LLMRunDetail, bool, error) {
	var r LLMRunDetail
	err := d.conn.QueryRow(
		`SELECT id, kind, market, model, latency_ms, created_at, watchlist_count, candidate_count, news_count, candle_gap_count, input_json, output_raw
		 FROM llm_runs WHERE id = ?`,
		id,
	).Scan(&r.ID, &r.Kind, &r.Market, &r.Model, &r.LatencyMs, &r.CreatedAt, &r.WatchlistCount, &r.CandidateCount, &r.NewsCount, &r.CandleGapCount, &r.InputJSON, &r.OutputRaw)
	if err != nil {
		if err == sql.ErrNoRows {
			return LLMRunDetail{}, false, nil
		}
		return LLMRunDetail{}, false, err
	}
	return r, true, nil
}
