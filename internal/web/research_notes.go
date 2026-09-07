package web

import (
	"net/http"
	"strings"

	"argus/internal/logger"
)

// researchNoteTags is the fixed set of note categories the chart page's
// compose form offers as tag chips (see plan doc) — stored as plain English
// constants with no enum type of their own, matching podcast_insights'
// stance column convention. Adjusting the taxonomy is a frontend-only
// change; this set only exists server-side to reject garbage input at the
// write boundary.
var researchNoteTags = map[string]bool{
	"TECHNICAL": true,
	"CHIPS":     true,
	"NEWS":      true,
	"OTHER":     true,
}

// researchNoteResponse is one row of GET /api/research-notes' list —
// mirrors thesisEntryResponse's shape (see rounds.go) plus Tag/Pinned/ID,
// since pin/delete need an id to target and the notebook card groups by tag.
type researchNoteResponse struct {
	ID     int64  `json:"id"`
	Tag    string `json:"tag"`
	Text   string `json:"text"`
	Pinned bool   `json:"pinned"`
	Date   string `json:"date"`
}

type researchNotesListResponse struct {
	Notes []researchNoteResponse `json:"notes"`
}

// handleResearchNotesGet backs GET /api/research-notes?ticker=... — the
// chart page's notebook card history for one ticker. Ungated like every
// other GET route (see handleChart): this is a read against the local
// dashboard's own DB, not a write.
func (s *Server) handleResearchNotesGet(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if p := recover(); p != nil {
			logger.Errorf("web: panic in handleResearchNotesGet: %v", p)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
	}()

	ticker := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("ticker")))
	if ticker == "" {
		writeError(w, http.StatusBadRequest, "ticker is required")
		return
	}

	notes, err := s.db.GetResearchNotesByTicker(ticker)
	if err != nil {
		logger.Errorf("web: get research notes for %s: %v", ticker, err)
		writeError(w, http.StatusInternalServerError, "failed to load research notes")
		return
	}

	resp := researchNotesListResponse{Notes: []researchNoteResponse{}}
	for _, n := range notes {
		resp.Notes = append(resp.Notes, researchNoteResponse{
			ID:     n.ID,
			Tag:    n.Tag,
			Text:   n.Text,
			Pinned: n.Pinned,
			Date:   n.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

type researchNoteSaveRequest struct {
	Ticker string `json:"ticker"`
	Tag    string `json:"tag"`
	Text   string `json:"text"`
}

// handleResearchNoteSave backs POST /api/research-notes — upserts ticker's
// note for today (db.UpsertResearchNote overwrites any note already
// recorded today). Tag is validated against the fixed taxonomy here since
// it's a real user-input trust boundary; ticker/text are required the same
// way handleThesisSet requires them.
func (s *Server) handleResearchNoteSave(w http.ResponseWriter, r *http.Request) {
	var req researchNoteSaveRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	ticker := strings.ToUpper(strings.TrimSpace(req.Ticker))
	text := strings.TrimSpace(req.Text)
	tag := strings.ToUpper(strings.TrimSpace(req.Tag))
	if ticker == "" || text == "" {
		writeError(w, http.StatusBadRequest, "ticker and text are required")
		return
	}
	if !researchNoteTags[tag] {
		writeError(w, http.StatusBadRequest, "invalid tag")
		return
	}
	if err := s.notesDB.UpsertResearchNote(ticker, tag, text); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save research note")
		return
	}
	writeJSON(w, http.StatusOK, tradeResponse{Message: "saved"})
}

type researchNotePinRequest struct {
	ID     int64 `json:"id"`
	Pinned bool  `json:"pinned"`
}

func (s *Server) handleResearchNotePin(w http.ResponseWriter, r *http.Request) {
	var req researchNotePinRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.notesDB.SetResearchNotePinned(req.ID, req.Pinned); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update research note")
		return
	}
	writeJSON(w, http.StatusOK, tradeResponse{Message: "saved"})
}

type researchNoteDeleteRequest struct {
	ID int64 `json:"id"`
}

func (s *Server) handleResearchNoteDelete(w http.ResponseWriter, r *http.Request) {
	var req researchNoteDeleteRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.notesDB.DeleteResearchNote(req.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete research note")
		return
	}
	writeJSON(w, http.StatusOK, tradeResponse{Message: "deleted"})
}
