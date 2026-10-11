package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"argus/internal/db"
	"argus/internal/logger"
	"argus/internal/service"
)

// parseRetirementLiveOverrides reads GET /api/wealth/retire's optional
// "settings drawer" query params — a page-local, never-persisted what-if
// preview (see service.RetirementOverrides). A POST's request has none, so it
// always renders the real, just-persisted state.
func parseRetirementLiveOverrides(r *http.Request) service.RetirementOverrides {
	q := r.URL.Query()
	return service.RetirementOverrides{
		Age: queryIntPtr(q, "age"), RetAge: queryIntPtr(q, "retAge"), LifeAge: queryIntPtr(q, "lifeAge"),
		Spend: queryFloatPtr(q, "spend"), OtherInc: queryFloatPtr(q, "otherInc"),
		Pool: queryFloatPtr(q, "pool"), ContribMo: queryFloatPtr(q, "contribMo"),
		PreR: queryFloatPtr(q, "preR"), PostR: queryFloatPtr(q, "postR"), SWR: queryFloatPtr(q, "swr"),
	}
}

func queryIntPtr(q url.Values, key string) *int {
	raw := q.Get(key)
	if raw == "" {
		return nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return nil
	}
	return &v
}

func queryFloatPtr(q url.Values, key string) *float64 {
	raw := q.Get(key)
	if raw == "" {
		return nil
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil
	}
	return &v
}

// retirementPlan is shared by the GET and POST handlers — a save just writes
// the goal row and then falls through to the same read path so the two never
// compute the projection differently.
func (s *Server) retirementPlan(r *http.Request) (service.RetirementPlan, error) {
	return service.ComputeRetirementPlan(s.db, s.fxDB, s.quotes, time.Now(), parseRetirementLiveOverrides(r))
}

// handleWealthRetireGet backs GET /api/wealth/retire — ungated read, same
// convention as every other wealth GET route.
func (s *Server) handleWealthRetireGet(w http.ResponseWriter, r *http.Request) {
	resp, err := s.retirementPlan(r)
	if err != nil {
		logger.Errorf("web: wealth retire: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load retirement plan")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

type wealthRetireSaveRequest struct {
	Name          string  `json:"name"`
	RetirementAge int     `json:"retirementAge"`
	MonthlySpend  float64 `json:"monthlySpend"`
}

// handleWealthRetireSave backs POST /api/wealth/retire — the quick-switch
// buttons' write path (§8.7's "快切按鈕是展示手法...實作成表單存目標＋就地重算").
// Requires the birth year to already be set (via /api/wealth/profile) since
// there's no calendar date to save otherwise.
func (s *Server) handleWealthRetireSave(w http.ResponseWriter, r *http.Request) {
	var req wealthRetireSaveRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.MonthlySpend <= 0 {
		writeError(w, http.StatusBadRequest, "monthlySpend must be positive")
		return
	}
	by := service.LoadWealthProfile(s.db).BirthYear
	if by == nil {
		writeError(w, http.StatusBadRequest, "set your birth year first")
		return
	}
	birthYear := *by
	currentAge := time.Now().Year() - birthYear
	if req.RetirementAge <= currentAge || req.RetirementAge > service.RetirementHorizonAge {
		writeError(w, http.StatusBadRequest, "retirementAge must be between your current age and the simulation horizon")
		return
	}
	targetDate := fmt.Sprintf("%04d-01-01", birthYear+req.RetirementAge)
	targetAmount := req.MonthlySpend * 12 * service.RetirementWithdrawalMultiple
	if _, err := s.wealthDB.UpsertRetirementGoal(req.Name, targetAmount, targetDate); err != nil {
		logger.Errorf("web: upsert retirement goal: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to save")
		return
	}

	resp, err := s.retirementPlan(r)
	if err != nil {
		logger.Errorf("web: wealth retire: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load retirement plan")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

type retireAssignItem struct {
	AssetID int64   `json:"assetId"`
	Ratio   float64 `json:"ratio"`
}

// wealthRetireAssignRequest is the 指定退休資產 drawer's whole state: the
// complete list of assets earmarked for retirement (anything not listed is
// un-earmarked), plus the page's current settings — only used to create the
// retirement goal when there isn't one yet, never to change an existing one.
type wealthRetireAssignRequest struct {
	Name          string             `json:"name"`
	RetirementAge int                `json:"retirementAge"`
	MonthlySpend  float64            `json:"monthlySpend"`
	Assets        []retireAssignItem `json:"assets"`
}

// handleWealthRetireAssign backs POST /api/wealth/retire/assign. It replaces
// the retirement goal's earmarks with exactly req.Assets in one transaction,
// creating the goal first when none exists (so a new account can assign
// before it has ever touched a quick-switch button). Everything is validated
// before anything is written: a rejected request leaves no goal behind.
func (s *Server) handleWealthRetireAssign(w http.ResponseWriter, r *http.Request) {
	var req wealthRetireAssignRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	set := make([]db.GoalAsset, 0, len(req.Assets))
	seen := make(map[int64]bool, len(req.Assets))
	for _, it := range req.Assets {
		if it.AssetID <= 0 || seen[it.AssetID] {
			writeError(w, http.StatusBadRequest, "assetId must be set once per asset")
			return
		}
		if it.Ratio <= 0 || it.Ratio > 1 {
			writeError(w, http.StatusBadRequest, "ratio must be above 0 and at most 1")
			return
		}
		seen[it.AssetID] = true
		set = append(set, db.GoalAsset{AssetID: it.AssetID, Ratio: it.Ratio})
	}

	goals, err := s.db.ListGoals()
	if err != nil {
		logger.Errorf("web: retire assign: list goals: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to save")
		return
	}
	var goalID int64 // 0 until a retirement goal exists
	if g := service.FindRetirementGoal(goals); g != nil {
		goalID = g.ID
	}
	earmarks, err := s.db.ListAllGoalAssets()
	if err != nil {
		logger.Errorf("web: retire assign: list earmarks: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to save")
		return
	}
	assetList, err := s.db.ListAssetsWithValue(false)
	if err != nil {
		logger.Errorf("web: retire assign: list assets: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to save")
		return
	}
	isAsset := make(map[int64]bool, len(assetList))
	for _, a := range assetList {
		isAsset[a.ID] = a.Side == "asset"
	}
	for _, ga := range set {
		if !isAsset[ga.AssetID] {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("asset %d not found", ga.AssetID))
			return
		}
		if room := earmarkRoom(earmarks, goalID, ga.AssetID); ga.Ratio > room+earmarkEpsilon {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("asset %d: only %.0f%% is left to assign", ga.AssetID, room*100))
			return
		}
	}

	if goalID == 0 {
		name := strings.TrimSpace(req.Name)
		if name == "" || req.MonthlySpend <= 0 {
			writeError(w, http.StatusBadRequest, "name and a positive monthlySpend are needed to create the retirement goal")
			return
		}
		// Unlike the quick-switch save, a missing birth year is fine here: the
		// goal is created without a date (the page keeps showing its sample
		// age) and the date arrives with the first quick-switch save.
		targetDate := ""
		if by := service.LoadWealthProfile(s.db).BirthYear; by != nil {
			if req.RetirementAge <= time.Now().Year()-*by || req.RetirementAge > service.RetirementHorizonAge {
				writeError(w, http.StatusBadRequest, "retirementAge must be between your current age and the simulation horizon")
				return
			}
			targetDate = fmt.Sprintf("%04d-01-01", *by+req.RetirementAge)
		}
		goalID, err = s.wealthDB.UpsertRetirementGoal(name, req.MonthlySpend*12*service.RetirementWithdrawalMultiple, targetDate)
		if err != nil {
			logger.Errorf("web: retire assign: create goal: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to save")
			return
		}
	}
	if err := s.wealthDB.SetGoalAssets(goalID, set); err != nil {
		logger.Errorf("web: retire assign: set earmarks: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to save")
		return
	}

	resp, err := s.retirementPlan(r)
	if err != nil {
		logger.Errorf("web: wealth retire: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load retirement plan")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
