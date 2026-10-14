package web

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"argus/internal/assets"
	"argus/internal/db"
	"argus/internal/logger"
	"argus/internal/service"
)

// handleWealthGoalsList backs GET /api/wealth/goals — ungated read, same
// convention as every other wealth GET route.
func (s *Server) handleWealthGoalsList(w http.ResponseWriter, r *http.Request) {
	resp, err := service.ComputeGoals(s.db, s.fxDB, s.quotes, time.Now())
	if err != nil {
		logger.Errorf("web: wealth goals: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load goals")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

type wealthGoalCreateRequest struct {
	Name         string  `json:"name"`
	Kind         string  `json:"kind"`
	TargetAmount float64 `json:"targetAmount"`
	Currency     string  `json:"currency"`
	TargetDate   string  `json:"targetDate"`
	Note         string  `json:"note"`

	SavedAmount         *float64 `json:"savedAmount"`
	MonthlyContribution *float64 `json:"monthlyContribution"`
	StartYear           int      `json:"startYear"`
}

// newGoal validates a create/update body into a db.NewGoal, writing the 400
// itself so both handlers share one rule set.
func (req wealthGoalCreateRequest) newGoal(w http.ResponseWriter) (db.NewGoal, bool) {
	req.Name = strings.TrimSpace(req.Name)
	if ps := assets.ValidateGoal(assets.NewGoal{
		Name: req.Name, TargetAmount: &req.TargetAmount, SavedAmount: req.SavedAmount, MonthlyContribution: req.MonthlyContribution, StartYear: req.StartYear,
	}); len(ps) > 0 {
		writeError(w, http.StatusBadRequest, ps[0].Msg)
		return db.NewGoal{}, false
	}
	return db.NewGoal{
		Name: req.Name, Kind: strings.TrimSpace(req.Kind), TargetAmount: req.TargetAmount,
		Currency: strings.TrimSpace(req.Currency), TargetDate: strings.TrimSpace(req.TargetDate), Note: strings.TrimSpace(req.Note),
		SavedAmount: req.SavedAmount, MonthlyContribution: req.MonthlyContribution, StartYear: req.StartYear,
	}, true
}

// handleWealthGoalCreate backs POST /api/wealth/goals — the add-goal form's
// write path, gated like every other wealth write route.
func (s *Server) handleWealthGoalCreate(w http.ResponseWriter, r *http.Request) {
	var req wealthGoalCreateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	ng, ok := req.newGoal(w)
	if !ok {
		return
	}
	id, err := s.wealthDB.CreateGoal(ng)
	if err != nil {
		logger.Errorf("web: create goal: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create goal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

type wealthGoalUpdateRequest struct {
	ID int64 `json:"id"`
	wealthGoalCreateRequest
}

// handleWealthGoalUpdate backs POST /api/wealth/goals/update — the edit
// drawer's save path. Only general goals are editable; the retirement row is
// derived (see db.UpdateGoal), so it 404s here.
func (s *Server) handleWealthGoalUpdate(w http.ResponseWriter, r *http.Request) {
	var req wealthGoalUpdateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.ID <= 0 {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}
	ng, ok := req.newGoal(w)
	if !ok {
		return
	}
	switch err := s.wealthDB.UpdateGoal(req.ID, ng); {
	case errors.Is(err, sql.ErrNoRows):
		writeError(w, http.StatusNotFound, "goal not found or not editable")
	case err != nil:
		logger.Errorf("web: update goal: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to update goal")
	default:
		writeJSON(w, http.StatusOK, tradeResponse{Message: "saved"})
	}
}

type wealthGoalDeleteRequest struct {
	ID int64 `json:"id"`
}

// handleWealthGoalDelete backs POST /api/wealth/goals/delete — a hard
// delete (goals have no soft-delete column, see db.DeleteGoal's doc
// comment), cascading to the goal's earmarks.
func (s *Server) handleWealthGoalDelete(w http.ResponseWriter, r *http.Request) {
	var req wealthGoalDeleteRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.ID <= 0 {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}
	if err := s.wealthDB.DeleteGoal(req.ID); err != nil {
		logger.Errorf("web: delete goal: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to delete goal")
		return
	}
	writeJSON(w, http.StatusOK, tradeResponse{Message: "deleted"})
}

// earmarkEpsilon absorbs float noise in ratios that were typed as percentages
// (70% → 0.7) and summed with others.
const earmarkEpsilon = 1e-6

// earmarkRoom is how much of an asset is still free to assign to goalID: 1
// minus what every OTHER goal already takes. Without this cap an asset could
// be counted at 100% by two goals at once, double-counting it in both
// progress figures.
func earmarkRoom(all []db.GoalAsset, goalID, assetID int64) float64 {
	room := 1.0
	for _, ga := range all {
		if ga.AssetID == assetID && ga.GoalID != goalID {
			room -= ga.Ratio
		}
	}
	return math.Max(room, 0)
}

type wealthGoalEarmarkRequest struct {
	GoalID  int64   `json:"goalId"`
	AssetID int64   `json:"assetId"`
	Ratio   float64 `json:"ratio"`
}

// handleWealthGoalEarmark backs POST /api/wealth/goals/earmark — sets (or,
// with ratio<=0, removes) one asset's earmark against one goal.
func (s *Server) handleWealthGoalEarmark(w http.ResponseWriter, r *http.Request) {
	var req wealthGoalEarmarkRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.GoalID <= 0 || req.AssetID <= 0 {
		writeError(w, http.StatusBadRequest, "goalId and assetId are required")
		return
	}
	if req.Ratio < 0 || req.Ratio > 1 {
		writeError(w, http.StatusBadRequest, "ratio must be between 0 and 1")
		return
	}
	if req.Ratio > 0 {
		all, err := s.db.ListAllGoalAssets()
		if err != nil {
			logger.Errorf("web: list earmarks: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to update earmark")
			return
		}
		if room := earmarkRoom(all, req.GoalID, req.AssetID); req.Ratio > room+earmarkEpsilon {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("only %.0f%% of this asset is left to assign", room*100))
			return
		}
	}
	if err := s.wealthDB.SetGoalAsset(req.GoalID, req.AssetID, req.Ratio); err != nil {
		logger.Errorf("web: set goal earmark: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to update earmark")
		return
	}
	writeJSON(w, http.StatusOK, tradeResponse{Message: "ok"})
}
