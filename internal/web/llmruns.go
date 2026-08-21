package web

import (
	"encoding/json"
	"net/http"
	"strconv"

	"argus/internal/db"
	"argus/internal/logger"
)

// llmRunsListLimit is the fixed page size for GET /api/llm-runs (Phase 19
// §4.4/§6) — no pagination/filtering, per the design doc's "a year is
// 700–1000 rows, reading three months back is unlikely enough to defer a
// ?before= param until someone actually needs it."
const llmRunsListLimit = 60

// llmRunSummaryResponse is one row of GET /api/llm-runs — everything
// ListLLMRuns' underlying query selects except input_json/output_raw (see
// db.LLMRun's doc comment for why those are excluded from the list read).
type llmRunSummaryResponse struct {
	ID             int64  `json:"id"`
	Kind           string `json:"kind"`
	Market         string `json:"market"`
	Model          string `json:"model"`
	LatencyMs      int64  `json:"latencyMs"`
	CreatedAt      string `json:"createdAt"`
	WatchlistCount int    `json:"watchlistCount"`
	CandidateCount int    `json:"candidateCount"`
	NewsCount      int    `json:"newsCount"`
	CandleGapCount int    `json:"candleGapCount"`
}

type llmRunsResponse struct {
	Runs []llmRunSummaryResponse `json:"runs"`
}

// llmRunDetailResponse is GET /api/llm-runs/{id}'s body. Input is the exact
// bytes db.LLMRunDetail.InputJSON holds, passed through as json.RawMessage
// so it lands in the response as a native JSON object rather than a
// JSON-string-in-JSON — the doc's "原樣透傳，web 端不重新整形" (§4.4): every
// data-quality/section-by-section rendering decision belongs to the
// frontend, not this handler.
type llmRunDetailResponse struct {
	llmRunSummaryResponse
	Input     json.RawMessage `json:"input"`
	OutputRaw string          `json:"outputRaw"`
}

func toLLMRunSummary(r db.LLMRun) llmRunSummaryResponse {
	return llmRunSummaryResponse{
		ID:             r.ID,
		Kind:           r.Kind,
		Market:         r.Market,
		Model:          r.Model,
		LatencyMs:      r.LatencyMs,
		CreatedAt:      r.CreatedAt,
		WatchlistCount: r.WatchlistCount,
		CandidateCount: r.CandidateCount,
		NewsCount:      r.NewsCount,
		CandleGapCount: r.CandleGapCount,
	}
}

func (s *Server) handleLLMRuns(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if p := recover(); p != nil {
			logger.Errorf("web: panic in handleLLMRuns: %v", p)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
	}()

	runs, err := s.db.ListLLMRuns(llmRunsListLimit)
	if err != nil {
		logger.Errorf("web: list llm runs: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list llm runs")
		return
	}
	resp := llmRunsResponse{Runs: make([]llmRunSummaryResponse, len(runs))}
	for i, run := range runs {
		resp.Runs[i] = toLLMRunSummary(run)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleLLMRunDetail(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if p := recover(); p != nil {
			logger.Errorf("web: panic in handleLLMRunDetail: %v", p)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
	}()

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	run, ok, err := s.db.GetLLMRun(id)
	if err != nil {
		logger.Errorf("web: get llm run %d: %v", id, err)
		writeError(w, http.StatusInternalServerError, "failed to get llm run")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "llm run not found")
		return
	}
	writeJSON(w, http.StatusOK, llmRunDetailResponse{
		llmRunSummaryResponse: toLLMRunSummary(run.LLMRun),
		Input:                 json.RawMessage(run.InputJSON),
		OutputRaw:             run.OutputRaw,
	})
}
