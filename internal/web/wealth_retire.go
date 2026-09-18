package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"argus/internal/db"
	"argus/internal/logger"
	"argus/internal/service"
)

// defaultRetirementAge/defaultRetirementMonthlySpend/retirementAgeOptions/
// retirementSpendOptions mirror the design template's retireModel() defaults
// and quick-switch buttons exactly (Argus Trading WebUI.dc.html lines
// 4345/4375-4386) — used both as the GET response's fallback before any
// quick-switch selection has ever been saved, and as the values the
// frontend's buttons offer.
const (
	defaultRetirementAge          = 60
	defaultRetirementMonthlySpend = 90000.0
)

var (
	retirementAgeOptions   = []int{55, 60, 65}
	retirementSpendOptions = []float64{70000, 90000, 120000}
)

type retirementScenarioSummary struct {
	ProjectedAtRetirement float64 `json:"projectedAtRetirement"`
	AchievementPct        float64 `json:"achievementPct"`
	GapAmount             float64 `json:"gapAmount"`
	Funded                bool    `json:"funded"`
	DepletionAge          *int    `json:"depletionAge,omitempty"`
}

func toScenarioSummary(p service.RetirementProjection, retirementAge int) retirementScenarioSummary {
	s := retirementScenarioSummary{
		ProjectedAtRetirement: p.ProjectedAtRetirement,
		AchievementPct:        p.AchievementPct,
		GapAmount:             p.GapAmount,
		Funded:                p.Funded,
	}
	if p.DepletionYearsAfterRetirement != nil {
		age := retirementAge + *p.DepletionYearsAfterRetirement
		s.DepletionAge = &age
	}
	return s
}

type retirementPathPoint struct {
	Year    int     `json:"year"`
	Balance float64 `json:"balance"`
}

// retirementResponse backs both GET and POST /api/wealth/retire — a save
// just recomputes and returns the same shape so the page can render off one
// response without a second round trip. Baseline/Crash/LowReturn/Need/Path
// are nil/empty together with Pool: the retirement-earmarked pool's own FX
// resolution failing degrades the whole projection, same whole-metric rule
// as everywhere else in wealth (§8.17.1).
type retirementResponse struct {
	AsOf                 string    `json:"asOf"`
	HasBirthYear         bool      `json:"hasBirthYear"`
	RetirementAge        int       `json:"retirementAge"`
	RetirementAgeOptions []int     `json:"retirementAgeOptions"`
	MonthlySpend         float64   `json:"monthlySpend"`
	MonthlySpendOptions  []float64 `json:"monthlySpendOptions"`
	MonthlyContribution  float64   `json:"monthlyContribution"`
	Pool                 *float64  `json:"pool"`
	// RetirementYear is the calendar year the chart's vertical marker sits
	// at — 0 when it can't be computed (no birth year yet).
	RetirementYear int                        `json:"retirementYear,omitempty"`
	Need           *float64                   `json:"need"`
	Baseline       *retirementScenarioSummary `json:"baseline,omitempty"`
	Crash          *retirementScenarioSummary `json:"crash,omitempty"`
	LowReturn      *retirementScenarioSummary `json:"lowReturn,omitempty"`
	Path           []retirementPathPoint      `json:"path"`
	Goal           *goalItem                  `json:"goal,omitempty"`
}

// buildRetirementResponse is shared by the GET and POST handlers — a save
// just writes the goal row and then falls through to the same read path so
// the two never compute the projection differently.
func (s *Server) buildRetirementResponse() (retirementResponse, error) {
	now := time.Now()
	today := now.Format("2006-01-02")
	resp := retirementResponse{
		AsOf: today, RetirementAge: defaultRetirementAge, MonthlySpend: defaultRetirementMonthlySpend,
		RetirementAgeOptions: retirementAgeOptions, MonthlySpendOptions: retirementSpendOptions,
		Path: []retirementPathPoint{},
	}

	var birthYear int
	if raw, ok, err := s.db.GetSetting(birthYearSettingKey); err == nil && ok {
		if v, perr := strconv.Atoi(raw); perr == nil {
			birthYear = v
			resp.HasBirthYear = true
		}
	}
	if raw, ok, err := s.db.GetSetting(retirementContribSettingKey); err == nil && ok {
		if v, perr := strconv.ParseFloat(raw, 64); perr == nil {
			resp.MonthlyContribution = v
		}
	}

	goals, err := s.db.ListGoals()
	if err != nil {
		return resp, err
	}
	var retGoal *db.Goal
	for i := range goals {
		if goals[i].Kind == "retirement" {
			retGoal = &goals[i]
			break
		}
	}
	if retGoal != nil {
		resp.MonthlySpend = retGoal.TargetAmount / 12 / service.RetirementWithdrawalMultiple
		if resp.HasBirthYear && retGoal.TargetDate != "" {
			if td, perr := time.Parse("2006-01-02", retGoal.TargetDate); perr == nil {
				resp.RetirementAge = td.Year() - birthYear
			}
		}
	}

	var earmarks []db.GoalAsset
	if retGoal != nil {
		all, err := s.db.ListAllGoalAssets()
		if err != nil {
			return resp, err
		}
		for _, ga := range all {
			if ga.GoalID == retGoal.ID {
				earmarks = append(earmarks, ga)
			}
		}
	}
	assetList, err := s.db.ListAssetsWithValue(true)
	if err != nil {
		return resp, err
	}
	assetByID := make(map[int64]db.AssetWithValue, len(assetList))
	for _, a := range assetList {
		assetByID[a.ID] = a
	}
	assetItems, pool := s.earmarkedSavedTWD(earmarks, assetByID, today)
	resp.Pool = pool

	if retGoal != nil {
		goal := goalItem{
			ID: retGoal.ID, Name: retGoal.Name, Kind: retGoal.Kind, TargetAmount: retGoal.TargetAmount,
			Currency: retGoal.Currency, TargetDate: retGoal.TargetDate, Note: retGoal.Note, Assets: assetItems,
		}
		if pool != nil && retGoal.TargetAmount > 0 {
			pct := *pool / retGoal.TargetAmount * 100
			if pct > 100 {
				pct = 100
			}
			goal.Saved, goal.ProgressPct = pool, &pct
			if retGoal.TargetDate != "" {
				if status, expectedPct, ok := goalStatus(retGoal.CreatedAt, retGoal.TargetDate, pct, now); ok {
					goal.Status, goal.MarkPct = status, &expectedPct
				}
			}
		}
		resp.Goal = &goal
	}

	if !resp.HasBirthYear || pool == nil {
		return resp, nil
	}

	currentAge := now.Year() - birthYear
	yearsToRetirement := resp.RetirementAge - currentAge
	if yearsToRetirement < 0 {
		yearsToRetirement = 0
	}
	yearsPostRetirement := service.RetirementHorizonAge - resp.RetirementAge
	if yearsPostRetirement < 0 {
		yearsPostRetirement = 0
	}
	resp.RetirementYear = now.Year() + yearsToRetirement

	in := service.RetirementInputs{
		YearsToRetirement: yearsToRetirement, YearsPostRetirement: yearsPostRetirement,
		Pool: *pool, MonthlyContribution: resp.MonthlyContribution, MonthlySpend: resp.MonthlySpend,
	}
	baseline, crash, lowReturn := service.ComputeRetirementScenarios(in)
	bs, cs, ls := toScenarioSummary(baseline, resp.RetirementAge), toScenarioSummary(crash, resp.RetirementAge), toScenarioSummary(lowReturn, resp.RetirementAge)
	resp.Baseline, resp.Crash, resp.LowReturn = &bs, &cs, &ls
	resp.Need = &baseline.Need

	for _, p := range baseline.Path {
		resp.Path = append(resp.Path, retirementPathPoint{Year: now.Year() + p.YearsFromNow, Balance: p.Balance})
	}
	return resp, nil
}

// handleWealthRetireGet backs GET /api/wealth/retire — ungated read, same
// convention as every other wealth GET route.
func (s *Server) handleWealthRetireGet(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if p := recover(); p != nil {
			logger.Errorf("web: panic in handleWealthRetireGet: %v", p)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
	}()
	resp, err := s.buildRetirementResponse()
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
	raw, ok, err := s.db.GetSetting(birthYearSettingKey)
	birthYear, perr := strconv.Atoi(raw)
	if err != nil || !ok || perr != nil {
		writeError(w, http.StatusBadRequest, "set your birth year first")
		return
	}
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

	resp, err := s.buildRetirementResponse()
	if err != nil {
		logger.Errorf("web: wealth retire: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load retirement plan")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
