package service

import (
	"fmt"
	"math"
	"time"

	"argus/internal/db"
)

// GoalAssetItem is one earmarked asset on a goal's card, resolved with the
// asset's own name/venue so the page doesn't need a second lookup. The JSON
// tags (here and below) are the /api/wealth/goals wire shape.
type GoalAssetItem struct {
	AssetID int64    `json:"assetId"`
	Name    string   `json:"name"`
	Venue   string   `json:"venue,omitempty"`
	Ratio   float64  `json:"ratio"`
	Value   *float64 `json:"value"` // TWD, nil if the asset itself has no value yet or FX couldn't be priced
}

// GoalItem backs one row of /w/goals — Saved/ProgressPct are nil together
// when any earmarked asset's currency couldn't be priced to TWD (§8.17.1's
// whole-metric-degrades rule, same as /w/alloc and /w/cash). Status/MarkPct
// are unset when TargetDate is unset — there's nothing to be ahead of or
// behind. MarkPct is where the design template's progress bar draws its
// "expected progress" tick (§8.9 point 5's 應有進度標記) — the straight-line
// expected position GoalStatus classified Status against, not just the
// classification itself, so the bar can render the same reference point the
// status label describes.
type GoalItem struct {
	ID           int64    `json:"id"`
	Name         string   `json:"name"`
	Kind         string   `json:"kind"`
	TargetAmount float64  `json:"targetAmount"`
	Currency     string   `json:"currency"`
	TargetDate   string   `json:"targetDate,omitempty"`
	Note         string   `json:"note,omitempty"`
	Saved        *float64 `json:"saved"`
	ProgressPct  *float64 `json:"progressPct"`
	Status       string   `json:"status,omitempty"` // "ahead" | "onTrack" | "behind"
	MarkPct      *float64 `json:"markPct,omitempty"`
	// MonthlyContribution is read from the same profile.retirement_monthly_
	// contribution setting /w/retire uses for the kind="retirement" row, and
	// from the goals.monthly_contribution column (typed in the drawer) for
	// general goals — nil renders as "—".
	MonthlyContribution *float64 `json:"monthlyContribution,omitempty"`
	// StartYear is the drawer's 起始年, echoed back so an edit can prefill it.
	StartYear int             `json:"startYear,omitempty"`
	Assets    []GoalAssetItem `json:"assets"`
	// ProjectedAtRetirement/RetirementAge are set on the kind="retirement"
	// row only (once a birth year is on file): the template's card subtitle
	// reads "… · 60 歲屆退推估 NT$25.35M".
	ProjectedAtRetirement *float64 `json:"projectedAtRetirement,omitempty"`
	RetirementAge         int      `json:"retirementAge,omitempty"`
}

// GoalList is GET /api/wealth/goals' whole response.
type GoalList struct {
	AsOf  string     `json:"asOf"`
	Goals []GoalItem `json:"goals"`
}

// retirementStartAge is the age the template measures retirement progress
// from: the card's marker sits at (age-25)/(retirementAge-25), not at a
// calendar fraction of the goal row's created_at → target_date.
const retirementStartAge = 25

// ApplyRetirementPace gives the retirement goal card the template's pace
// semantics (goalRow with eta_late/expect overrides): the marker is age
// progress, "behind" means the baseline projection falls short of the need,
// otherwise ahead when saved progress has reached the marker.
func ApplyRetirementPace(g *GoalItem, currentAge, retirementAge int, projected, need float64) {
	if g.ProgressPct == nil || retirementAge <= retirementStartAge {
		return
	}
	expected := math.Round(float64(currentAge-retirementStartAge) / float64(retirementAge-retirementStartAge) * 100)
	expected = math.Max(0, math.Min(100, expected))
	g.MarkPct = &expected
	switch {
	case projected < need:
		g.Status = "behind"
	case *g.ProgressPct >= expected:
		g.Status = "ahead"
	default:
		g.Status = "onTrack"
	}
	g.ProjectedAtRetirement, g.RetirementAge = &projected, retirementAge
}

// GoalStatus compares actual progress against a straight-line expectation
// from the goal's creation date to its target date — a pure function, no
// LLM involvement, same "health metrics never go through the LLM" rule as
// /w/alloc's drift math. A 5pp band around the expected line counts as
// on-track rather than flagging every goal as barely ahead/behind on
// floating-point noise. ok=false (both other return values unset) when
// there's no target date to compare against, or CreatedAt fails to parse.
func GoalStatus(createdAt, targetDate string, progressPct float64, today time.Time) (status string, expectedPct float64, ok bool) {
	created, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		created, err = time.Parse("2006-01-02 15:04:05", createdAt)
		if err != nil {
			return "", 0, false
		}
	}
	target, err := time.Parse("2006-01-02", targetDate)
	if err != nil {
		return "", 0, false
	}
	total := target.Sub(created)
	if total <= 0 {
		return "", 0, false
	}
	elapsed := today.Sub(created)
	if elapsed < 0 {
		elapsed = 0
	}
	expectedPct = elapsed.Seconds() / total.Seconds() * 100
	switch {
	case progressPct >= expectedPct+5:
		status = "ahead"
	case progressPct <= expectedPct-5:
		status = "behind"
	default:
		status = "onTrack"
	}
	return status, expectedPct, true
}

// FindRetirementGoal returns the kind="retirement" goal, nil if there is
// none yet. db.UpsertRetirementGoal keeps it to one row.
func FindRetirementGoal(goals []db.Goal) *db.Goal {
	for i := range goals {
		if goals[i].Kind == "retirement" {
			return &goals[i]
		}
	}
	return nil
}

// EarmarkedSavedTWD sums one goal's earmarked assets converted to TWD as of
// asOf — nil (not a partial sum) if any earmarked asset's currency can't be
// priced, the same whole-metric-degrades rule as everywhere else in wealth.
// The one place this sum is computed: /w/goals, /w/retire (the retirement
// pool is exactly this number for the kind="retirement" goal) and the
// monthly health report all read it.
func EarmarkedSavedTWD(earmarks []db.GoalAsset, assetByID map[int64]db.AssetWithValue, asOf string, quotes QuoteReader, fx WealthFXStore) (items []GoalAssetItem, saved *float64) {
	items = []GoalAssetItem{}
	var sum float64
	ok := true
	for _, ga := range earmarks {
		a, found := assetByID[ga.AssetID]
		row := GoalAssetItem{AssetID: ga.AssetID, Ratio: ga.Ratio}
		if found {
			row.Name, row.Venue = a.Name, a.Venue
		}
		if found && a.Value != nil {
			rate, rok := RateToTWD(a.Currency, asOf, true, quotes, fx)
			if !rok {
				ok = false
			} else {
				v := *a.Value * ga.Ratio * rate
				row.Value = &v
				sum += v
			}
		}
		items = append(items, row)
	}
	if !ok {
		return items, nil
	}
	return items, &sum
}

// ComputeGoals builds /w/goals: every goal with its earmarked assets, saved
// amount, progress and pace status as of now. A non-nil error means only that
// a store read failed — an unpriceable currency leaves that goal's Saved nil.
func ComputeGoals(store WealthStore, fx WealthFXStore, quotes QuoteReader, now time.Time) (GoalList, error) {
	today := now.Format("2006-01-02")
	resp := GoalList{AsOf: today, Goals: []GoalItem{}}

	goals, err := store.ListGoals()
	if err != nil {
		return resp, fmt.Errorf("list goals: %w", err)
	}
	earmarks, err := store.ListAllGoalAssets()
	if err != nil {
		return resp, fmt.Errorf("list earmarks: %w", err)
	}
	assetList, err := store.ListAssetsWithValue(true)
	if err != nil {
		return resp, fmt.Errorf("list assets: %w", err)
	}
	assetByID := make(map[int64]db.AssetWithValue, len(assetList))
	for _, a := range assetList {
		assetByID[a.ID] = a
	}
	earmarksByGoal := make(map[int64][]db.GoalAsset, len(goals))
	for _, ga := range earmarks {
		earmarksByGoal[ga.GoalID] = append(earmarksByGoal[ga.GoalID], ga)
	}

	for _, g := range goals {
		item := GoalItem{
			ID: g.ID, Name: g.Name, Kind: g.Kind, TargetAmount: g.TargetAmount,
			Currency: g.Currency, TargetDate: g.TargetDate, Note: g.Note, Assets: []GoalAssetItem{},
		}

		item.Assets, item.Saved = EarmarkedSavedTWD(earmarksByGoal[g.ID], assetByID, today, quotes, fx)
		if g.Kind != "retirement" {
			// A hand-typed 已累積 wins over the earmark sum (drawer, migration
			// 34); the retirement row's saved stays the earmarked pool.
			if g.SavedAmount != nil {
				item.Saved = g.SavedAmount
			}
			item.MonthlyContribution = g.MonthlyContribution
			item.StartYear = g.StartYear
		}

		if item.Saved != nil {
			saved := *item.Saved
			if g.TargetAmount > 0 {
				// Capped at 100 (matching the design template's goalRow, which
				// does the same) — an overfunded goal reads as "done", not as a
				// number that invites a second-guess about the math.
				pct := saved / g.TargetAmount * 100
				if pct > 100 {
					pct = 100
				}
				item.ProgressPct = &pct
				if g.TargetDate != "" {
					start := g.CreatedAt
					if g.StartYear > 0 {
						start = fmt.Sprintf("%04d-01-01 00:00:00", g.StartYear)
					}
					if status, expectedPct, ok := GoalStatus(start, g.TargetDate, pct, now); ok {
						item.Status = status
						item.MarkPct = &expectedPct
					}
				}
			}
		}
		if g.Kind == "retirement" {
			item.MonthlyContribution = LoadWealthProfile(store).RetirementMonthlyContribution
			// Same pace numbers /w/retire's own goal card shows, so the two
			// pages can't disagree; without a birth year the created_at-based
			// status above stands.
			if plan, err := ComputeRetirementPlan(store, fx, quotes, now, RetirementOverrides{}); err == nil && plan.Goal != nil && plan.Goal.ProjectedAtRetirement != nil {
				item.Status, item.MarkPct = plan.Goal.Status, plan.Goal.MarkPct
				item.ProjectedAtRetirement, item.RetirementAge = plan.Goal.ProjectedAtRetirement, plan.Goal.RetirementAge
			}
		}
		resp.Goals = append(resp.Goals, item)
	}
	return resp, nil
}
