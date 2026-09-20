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
	CurrentAge           int       `json:"currentAge"`
	RetirementAge        int       `json:"retirementAge"`
	RetirementAgeOptions []int     `json:"retirementAgeOptions"`
	MonthlySpend         float64   `json:"monthlySpend"`
	MonthlySpendOptions  []float64 `json:"monthlySpendOptions"`
	MonthlyContribution  float64   `json:"monthlyContribution"`
	// OtherIncome is the settings drawer's "其他月收入" what-if input
	// (labour pension, rent, ...) — always 0 unless a live override sets
	// it; there's no persisted source for this figure yet.
	OtherIncome float64  `json:"otherIncome"`
	Pool        *float64 `json:"pool"`
	// HorizonAge/PreReturnPct/PostReturnPct/WithdrawalRatePct are the
	// resolved (default-or-overridden) assumptions the projection below was
	// actually computed with — echoed back so the settings drawer can
	// prefill its fields from the real defaults on first open (Argus
	// Trading WebUI.dc.html's retDefaults(), lines 4486-4499) without the
	// frontend duplicating service.RetirementHorizonAge/PreReturnReal/
	// PostReturnReal/WithdrawalRateReal as separate constants.
	HorizonAge        int     `json:"horizonAge"`
	PreReturnPct      float64 `json:"preReturnPct"`
	PostReturnPct     float64 `json:"postReturnPct"`
	WithdrawalRatePct float64 `json:"withdrawalRatePct"`
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

// retirementLiveOverrides holds GET /api/wealth/retire's optional
// "settings drawer" query params — a page-local, never-persisted what-if
// preview (Argus Trading WebUI.dc.html's wRetCfg/retCfg(), lines 4490-4510:
// "這些數字只影響退休頁的試算，不會改動你的實際資產紀錄"). Absent params fall
// back to the real computed value, same merge semantics as retCfg().
type retirementLiveOverrides struct {
	age, retAge, lifeAge             *int
	spend, otherInc, pool, contribMo *float64
	preR, postR, swr                 *float64
}

func parseRetirementLiveOverrides(r *http.Request) retirementLiveOverrides {
	q := r.URL.Query()
	return retirementLiveOverrides{
		age: queryIntPtr(q, "age"), retAge: queryIntPtr(q, "retAge"), lifeAge: queryIntPtr(q, "lifeAge"),
		spend: queryFloatPtr(q, "spend"), otherInc: queryFloatPtr(q, "otherInc"),
		pool: queryFloatPtr(q, "pool"), contribMo: queryFloatPtr(q, "contribMo"),
		preR: queryFloatPtr(q, "preR"), postR: queryFloatPtr(q, "postR"), swr: queryFloatPtr(q, "swr"),
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

// buildRetirementResponse is shared by the GET and POST handlers — a save
// just writes the goal row and then falls through to the same read path so
// the two never compute the projection differently. r's query string carries
// GET's optional live overrides (see retirementLiveOverrides); a POST's
// request has none, so it always renders the real, just-persisted state.
func (s *Server) buildRetirementResponse(r *http.Request) (retirementResponse, error) {
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

	// ov holds the settings drawer's optional live-assumption overrides
	// (GET only, never persisted). resp.Goal above is deliberately built
	// from the real pool, never an override, so a what-if pool figure can
	// never leak into the trustworthy "saved vs target" goal card.
	ov := parseRetirementLiveOverrides(r)

	currentAge := now.Year() - birthYear
	if ov.age != nil {
		currentAge = *ov.age
	}
	resp.CurrentAge = currentAge

	if ov.retAge != nil {
		resp.RetirementAge = *ov.retAge
	}
	if ov.spend != nil {
		resp.MonthlySpend = *ov.spend
	}
	if ov.contribMo != nil {
		resp.MonthlyContribution = *ov.contribMo
	}

	projPool := *pool
	if ov.pool != nil {
		projPool = *ov.pool
		resp.Pool = ov.pool
	}

	resp.HorizonAge = service.RetirementHorizonAge
	if ov.lifeAge != nil {
		resp.HorizonAge = *ov.lifeAge
	}

	if ov.otherInc != nil {
		resp.OtherIncome = *ov.otherInc
	}
	netSpend := resp.MonthlySpend - resp.OtherIncome
	if netSpend < 0 {
		netSpend = 0
	}

	preReturn := service.RetirementPreReturnReal
	if ov.preR != nil {
		preReturn = *ov.preR / 100
	}
	postReturn := service.RetirementPostReturnReal
	if ov.postR != nil {
		postReturn = *ov.postR / 100
	}
	withdrawalRate := service.RetirementWithdrawalRateReal
	if ov.swr != nil {
		withdrawalRate = *ov.swr / 100
	}
	resp.PreReturnPct, resp.PostReturnPct, resp.WithdrawalRatePct = preReturn*100, postReturn*100, withdrawalRate*100

	yearsToRetirement := resp.RetirementAge - currentAge
	if yearsToRetirement < 0 {
		yearsToRetirement = 0
	}
	yearsPostRetirement := resp.HorizonAge - resp.RetirementAge
	if yearsPostRetirement < 0 {
		yearsPostRetirement = 0
	}
	resp.RetirementYear = now.Year() + yearsToRetirement

	in := service.RetirementInputs{
		YearsToRetirement: yearsToRetirement, YearsPostRetirement: yearsPostRetirement,
		Pool: projPool, MonthlyContribution: resp.MonthlyContribution, MonthlySpend: netSpend,
		PreReturn: preReturn, PostReturn: postReturn, WithdrawalRate: withdrawalRate,
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
	resp, err := s.buildRetirementResponse(r)
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

	resp, err := s.buildRetirementResponse(r)
	if err != nil {
		logger.Errorf("web: wealth retire: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load retirement plan")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
