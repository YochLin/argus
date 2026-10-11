package service

import (
	"fmt"
	"time"

	"argus/internal/db"
)

// DefaultRetirementAge/DefaultRetirementMonthlySpend/RetirementAgeOptions/
// RetirementSpendOptions mirror the design template's retireModel() defaults
// and quick-switch buttons exactly (Argus Trading WebUI.dc.html lines
// 4345/4375-4386) — used both as the plan's fallback before any quick-switch
// selection has ever been saved, and as the values the frontend's buttons
// offer.
const (
	DefaultRetirementAge          = 60
	DefaultRetirementMonthlySpend = 90000.0
	// DefaultSampleCurrentAge is the age the projection uses when no birth
	// year is on file yet — mirrors the design's own retDefaults() age:42
	// exactly. The plan is clearly tagged IsSample so this never reads as the
	// user's real age; it exists so the page has something to show (and the
	// settings drawer's what-if preview has something to preview) before the
	// one-time birth-year setup is filled in.
	DefaultSampleCurrentAge = 42
)

var (
	RetirementAgeOptions   = []int{55, 60, 65}
	RetirementSpendOptions = []float64{70000, 90000, 120000}
)

// RetirementScenarioSummary is one scenario's headline numbers. The JSON tags
// (here and below) are the /api/wealth/retire wire shape.
type RetirementScenarioSummary struct {
	ProjectedAtRetirement float64 `json:"projectedAtRetirement"`
	AchievementPct        float64 `json:"achievementPct"`
	GapAmount             float64 `json:"gapAmount"`
	Funded                bool    `json:"funded"`
	DepletionAge          *int    `json:"depletionAge,omitempty"`
}

func toScenarioSummary(p RetirementProjection, retirementAge int) RetirementScenarioSummary {
	s := RetirementScenarioSummary{
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

// RetirementPlanPoint is one year of the baseline path, keyed by calendar year.
type RetirementPlanPoint struct {
	Year    int     `json:"year"`
	Balance float64 `json:"balance"`
}

// RetirementPlan is the retirement page's whole response. A save just
// recomputes and returns the same shape so the page can render off one
// response without a second round trip. Baseline/Crash/LowReturn/Need/Path
// are nil/empty together with Pool: the retirement-earmarked pool's own FX
// resolution failing degrades the whole projection, same whole-metric rule
// as everywhere else in wealth (§8.17.1).
type RetirementPlan struct {
	AsOf         string `json:"asOf"`
	HasBirthYear bool   `json:"hasBirthYear"`
	// IsSample is true exactly when HasBirthYear is false and the
	// projection below still rendered — using DefaultSampleCurrentAge
	// instead of a real one. Tied purely to HasBirthYear, never to whether
	// a live what-if override is in play (same as the design's own
	// `isSample = !hasAge`), so previewing "what if I were 35" in the
	// settings drawer doesn't make the sample tag disappear early.
	IsSample             bool      `json:"isSample"`
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
	// frontend duplicating RetirementHorizonAge/RetirementPreReturnReal/
	// RetirementPostReturnReal/RetirementWithdrawalRateReal as separate
	// constants.
	HorizonAge        int     `json:"horizonAge"`
	PreReturnPct      float64 `json:"preReturnPct"`
	PostReturnPct     float64 `json:"postReturnPct"`
	WithdrawalRatePct float64 `json:"withdrawalRatePct"`
	// RetirementYear is the calendar year the chart's vertical marker sits
	// at — 0 when it can't be computed (no birth year yet).
	RetirementYear int                        `json:"retirementYear,omitempty"`
	Need           *float64                   `json:"need"`
	Baseline       *RetirementScenarioSummary `json:"baseline,omitempty"`
	Crash          *RetirementScenarioSummary `json:"crash,omitempty"`
	LowReturn      *RetirementScenarioSummary `json:"lowReturn,omitempty"`
	Path           []RetirementPlanPoint      `json:"path"`
	Goal           *GoalItem                  `json:"goal,omitempty"`
}

// RetirementOverrides holds the settings drawer's optional "what-if" inputs —
// a page-local, never-persisted preview (Argus Trading WebUI.dc.html's
// wRetCfg/retCfg(), lines 4490-4510: "這些數字只影響退休頁的試算，不會改動你的
// 實際資產紀錄"). A nil field falls back to the real computed value, same
// merge semantics as retCfg().
type RetirementOverrides struct {
	Age, RetAge, LifeAge             *int
	Spend, OtherInc, Pool, ContribMo *float64
	PreR, PostR, SWR                 *float64
}

// ComputeRetirementPlan builds the retirement page as of now. The GET and
// POST handlers share it so the two never compute the projection differently;
// a POST passes no overrides, so it always renders the real, just-persisted
// state. A non-nil error means only that a store read failed.
func ComputeRetirementPlan(store WealthStore, fx WealthFXStore, quotes QuoteReader, now time.Time, ov RetirementOverrides) (RetirementPlan, error) {
	today := now.Format("2006-01-02")
	resp := RetirementPlan{
		AsOf: today, RetirementAge: DefaultRetirementAge, MonthlySpend: DefaultRetirementMonthlySpend,
		RetirementAgeOptions: RetirementAgeOptions, MonthlySpendOptions: RetirementSpendOptions,
		Path: []RetirementPlanPoint{},
	}

	profile := LoadWealthProfile(store)
	var birthYear int
	if profile.BirthYear != nil {
		birthYear = *profile.BirthYear
		resp.HasBirthYear = true
	}
	if profile.RetirementMonthlyContribution != nil {
		resp.MonthlyContribution = *profile.RetirementMonthlyContribution
	}

	goals, err := store.ListGoals()
	if err != nil {
		return resp, fmt.Errorf("list goals: %w", err)
	}
	retGoal := FindRetirementGoal(goals)
	if retGoal != nil {
		resp.MonthlySpend = retGoal.TargetAmount / 12 / RetirementWithdrawalMultiple
		if resp.HasBirthYear && retGoal.TargetDate != "" {
			if td, perr := time.Parse("2006-01-02", retGoal.TargetDate); perr == nil {
				resp.RetirementAge = td.Year() - birthYear
			}
		}
	}

	var earmarks []db.GoalAsset
	if retGoal != nil {
		all, err := store.ListAllGoalAssets()
		if err != nil {
			return resp, fmt.Errorf("list earmarks: %w", err)
		}
		for _, ga := range all {
			if ga.GoalID == retGoal.ID {
				earmarks = append(earmarks, ga)
			}
		}
	}
	assetList, err := store.ListAssetsWithValue(true)
	if err != nil {
		return resp, fmt.Errorf("list assets: %w", err)
	}
	assetByID := make(map[int64]db.AssetWithValue, len(assetList))
	for _, a := range assetList {
		assetByID[a.ID] = a
	}
	assetItems, pool := EarmarkedSavedTWD(earmarks, assetByID, today, quotes, fx)
	resp.Pool = pool

	if retGoal != nil {
		goal := GoalItem{
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
				if status, expectedPct, ok := GoalStatus(retGoal.CreatedAt, retGoal.TargetDate, pct, now); ok {
					goal.Status, goal.MarkPct = status, &expectedPct
				}
			}
		}
		resp.Goal = &goal
	}

	if pool == nil {
		return resp, nil
	}

	// resp.Goal above is deliberately built from the real pool, never an
	// override, so a what-if pool figure can never leak into the trustworthy
	// "saved vs target" goal card.
	resp.IsSample = !resp.HasBirthYear
	currentAge := DefaultSampleCurrentAge
	if resp.HasBirthYear {
		currentAge = now.Year() - birthYear
	}
	if ov.Age != nil {
		currentAge = *ov.Age
	}
	resp.CurrentAge = currentAge

	if ov.RetAge != nil {
		resp.RetirementAge = *ov.RetAge
	}
	if ov.Spend != nil {
		resp.MonthlySpend = *ov.Spend
	}
	if ov.ContribMo != nil {
		resp.MonthlyContribution = *ov.ContribMo
	}

	projPool := *pool
	if ov.Pool != nil {
		projPool = *ov.Pool
		resp.Pool = ov.Pool
	}

	resp.HorizonAge = RetirementHorizonAge
	if ov.LifeAge != nil {
		resp.HorizonAge = *ov.LifeAge
	}

	if ov.OtherInc != nil {
		resp.OtherIncome = *ov.OtherInc
	}
	netSpend := resp.MonthlySpend - resp.OtherIncome
	if netSpend < 0 {
		netSpend = 0
	}

	preReturn := RetirementPreReturnReal
	if ov.PreR != nil {
		preReturn = *ov.PreR / 100
	}
	postReturn := RetirementPostReturnReal
	if ov.PostR != nil {
		postReturn = *ov.PostR / 100
	}
	withdrawalRate := RetirementWithdrawalRateReal
	if ov.SWR != nil {
		withdrawalRate = *ov.SWR / 100
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

	in := RetirementInputs{
		YearsToRetirement: yearsToRetirement, YearsPostRetirement: yearsPostRetirement,
		Pool: projPool, MonthlyContribution: resp.MonthlyContribution, MonthlySpend: netSpend,
		PreReturn: preReturn, PostReturn: postReturn, WithdrawalRate: withdrawalRate,
	}
	baseline, crash, lowReturn := ComputeRetirementScenarios(in)
	bs, cs, ls := toScenarioSummary(baseline, resp.RetirementAge), toScenarioSummary(crash, resp.RetirementAge), toScenarioSummary(lowReturn, resp.RetirementAge)
	resp.Baseline, resp.Crash, resp.LowReturn = &bs, &cs, &ls
	resp.Need = &baseline.Need
	// The goal card stays on the real (non-what-if, non-sample) numbers, so
	// the template's pace semantics apply only when no drawer override is
	// in play AND a real birth year is on file — a sample age is exactly as
	// untrustworthy as a what-if override for a "saved vs target" pace call.
	if resp.Goal != nil && ov == (RetirementOverrides{}) && resp.HasBirthYear {
		ApplyRetirementPace(resp.Goal, currentAge, resp.RetirementAge, baseline.ProjectedAtRetirement, baseline.Need)
	}

	for _, p := range baseline.Path {
		resp.Path = append(resp.Path, RetirementPlanPoint{Year: now.Year() + p.YearsFromNow, Balance: p.Balance})
	}
	return resp, nil
}
