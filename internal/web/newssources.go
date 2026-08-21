package web

import (
	"net/http"
	"strings"

	"argus/internal/logger"
)

// newsSourceWriter backs POST /api/news-sources/{block,unblock} (Phase 19
// PR2, §5.3) — like watchlistWriter, a direct DB write with no bot-layer
// behavior beyond the DB write itself.
type newsSourceWriter interface {
	BlockNewsSource(source string) error
	UnblockNewsSource(source string) error
}

type blockedSourceResponse struct {
	Source    string `json:"source"`
	CreatedAt string `json:"createdAt"`
}

type blockedSourcesResponse struct {
	Sources []blockedSourceResponse `json:"sources"`
}

type newsSourceRequest struct {
	Source string `json:"source"`
}

// handleBlockedNewsSources serves GET /api/news-sources/blocked — the /llm
// page's blocklist panel (§5.3) and the data-quality "low-quality source"
// check both read this list.
func (s *Server) handleBlockedNewsSources(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if p := recover(); p != nil {
			logger.Errorf("web: panic in handleBlockedNewsSources: %v", p)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
	}()

	sources, err := s.db.ListBlockedNewsSources()
	if err != nil {
		logger.Errorf("web: list blocked news sources: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list blocked news sources")
		return
	}
	resp := blockedSourcesResponse{Sources: []blockedSourceResponse{}}
	for _, src := range sources {
		resp.Sources = append(resp.Sources, blockedSourceResponse{Source: src.Source, CreatedAt: src.CreatedAt})
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleNewsSourceBlock(w http.ResponseWriter, r *http.Request) {
	var req newsSourceRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	source := strings.TrimSpace(req.Source)
	if source == "" {
		writeError(w, http.StatusBadRequest, "source is required")
		return
	}
	if err := s.newsSourceDB.BlockNewsSource(source); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to block news source")
		return
	}
	writeJSON(w, http.StatusOK, tradeResponse{Message: "blocked " + source})
}

func (s *Server) handleNewsSourceUnblock(w http.ResponseWriter, r *http.Request) {
	var req newsSourceRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	source := strings.TrimSpace(req.Source)
	if source == "" {
		writeError(w, http.StatusBadRequest, "source is required")
		return
	}
	if err := s.newsSourceDB.UnblockNewsSource(source); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to unblock news source")
		return
	}
	writeJSON(w, http.StatusOK, tradeResponse{Message: "unblocked " + source})
}
