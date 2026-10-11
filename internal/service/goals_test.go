package service

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"argus/internal/db"
)

type failingGoalsStore struct{ fakeWealthStore }

func (failingGoalsStore) ListGoals() ([]db.Goal, error) { return nil, errors.New("goals boom") }

// retiree is a 40-year-old with a NT$20M fund fully earmarked for a
// retirement goal that targets age 60 — the funded, birth-year-known path.
func retiree(now time.Time) *fakeWealthStore {
	v := 20000000.0
	return &fakeWealthStore{
		settings: map[string]string{BirthYearSettingKey: strconv.Itoa(now.Year() - 40)},
		assets: []db.AssetWithValue{
			{Asset: db.Asset{ID: 1, Side: "asset", Type: "fund", Name: "退休基金", Currency: "TWD"}, Value: &v},
		},
		goals: []db.Goal{{
			ID: 1, Name: "退休金", Kind: "retirement", Currency: "TWD", CreatedAt: "2020-01-01 00:00:00",
			TargetAmount: 90000 * 12 * RetirementWithdrawalMultiple, TargetDate: strconv.Itoa(now.Year()+20) + "-01-01",
		}},
		goalAssets: []db.GoalAsset{{GoalID: 1, AssetID: 1, Ratio: 1}},
	}
}

// TestComputeRetirementPlanOverridesLeaveGoalCardReal pins the drawer's
// what-if contract: an override moves the projection but never the goal
// card's saved figure, and any override (not just a pool one) switches the
// age-based pace call off, since it would be a verdict on a number the user
// only tried out.
func TestComputeRetirementPlanOverridesLeaveGoalCardReal(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	store := retiree(now)

	real, err := ComputeRetirementPlan(store, &fakeWealthFX{}, &fakeWealthQuotes{}, now, RetirementOverrides{})
	if err != nil {
		t.Fatalf("ComputeRetirementPlan() error = %v", err)
	}
	if real.Pool == nil || *real.Pool != 20000000 || real.Goal == nil || real.Goal.ProjectedAtRetirement == nil {
		t.Fatalf("real plan: pool = %v, goal = %+v; want the funded pool with the pace applied", real.Pool, real.Goal)
	}

	pool := 5000000.0
	what, err := ComputeRetirementPlan(store, &fakeWealthFX{}, &fakeWealthQuotes{}, now, RetirementOverrides{Pool: &pool})
	if err != nil {
		t.Fatalf("ComputeRetirementPlan(override) error = %v", err)
	}
	if what.Pool == nil || *what.Pool != pool {
		t.Errorf("override Pool = %v, want %v", what.Pool, pool)
	}
	if what.Goal.Saved == nil || *what.Goal.Saved != 20000000 {
		t.Errorf("override Goal.Saved = %v, want the real 20,000,000", what.Goal.Saved)
	}
	if what.Goal.ProjectedAtRetirement != nil {
		t.Errorf("override Goal.ProjectedAtRetirement = %v, want unset (no pace call off a what-if)", *what.Goal.ProjectedAtRetirement)
	}
}

func TestComputeRetirementPlanReadFailure(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	if _, err := ComputeRetirementPlan(&failingGoalsStore{}, &fakeWealthFX{}, &fakeWealthQuotes{}, now, RetirementOverrides{}); err == nil {
		t.Error("ComputeRetirementPlan() error = nil, want the goals read failure")
	}
	if _, err := ComputeGoals(&failingGoalsStore{}, &fakeWealthFX{}, &fakeWealthQuotes{}, now); err == nil {
		t.Error("ComputeGoals() error = nil, want the goals read failure")
	}
}

// TestComputeGoalsRetirementRowBorrowsPlan pins the two row shapes: the
// retirement row takes its contribution from the profile setting and its pace
// from the retirement plan (so /w/goals and /w/retire can't disagree), while a
// general goal's hand-typed 已累積 wins over its earmark sum.
func TestComputeGoalsRetirementRowBorrowsPlan(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	store := retiree(now)
	store.settings[RetirementMonthlyContributionSettingKey] = "28000"
	typed := 123456.0
	store.goals = append(store.goals, db.Goal{
		ID: 2, Name: "買房", Kind: "general", Currency: "TWD", TargetAmount: 1000000, SavedAmount: &typed,
		CreatedAt: "2026-01-01 00:00:00",
	})
	store.goalAssets = append(store.goalAssets, db.GoalAsset{GoalID: 2, AssetID: 1, Ratio: 0.5})

	got, err := ComputeGoals(store, &fakeWealthFX{}, &fakeWealthQuotes{}, now)
	if err != nil {
		t.Fatalf("ComputeGoals() error = %v", err)
	}
	plan, _ := ComputeRetirementPlan(store, &fakeWealthFX{}, &fakeWealthQuotes{}, now, RetirementOverrides{})
	byID := map[int64]GoalItem{}
	for _, g := range got.Goals {
		byID[g.ID] = g
	}
	ret := byID[1]
	if ret.MonthlyContribution == nil || *ret.MonthlyContribution != 28000 {
		t.Errorf("retirement MonthlyContribution = %v, want 28000", ret.MonthlyContribution)
	}
	// The age-based marker (40-25)/(60-25) = 43 differs from the calendar one
	// (created 2020 → target 2046 is ~26), so this only holds if the row
	// really took the plan's pace.
	if plan.Goal.MarkPct == nil || *plan.Goal.MarkPct != 43 {
		t.Fatalf("plan MarkPct = %v, want the age-based 43", plan.Goal.MarkPct)
	}
	if ret.Status != plan.Goal.Status || ret.MarkPct == nil || *ret.MarkPct != 43 || ret.RetirementAge != 60 || ret.ProjectedAtRetirement == nil {
		t.Errorf("retirement row = %+v, want the plan's pace (status %q, mark 43, age 60)", ret, plan.Goal.Status)
	}
	if house := byID[2]; house.Saved == nil || *house.Saved != typed {
		t.Errorf("general goal Saved = %v, want the typed %v, not the earmark sum", house.Saved, typed)
	}
}

// TestGoalStatus pins the straight-line expected-progress classification a
// target_date goal gets: ahead/behind/onTrack around a 5pp band.
func TestGoalStatus(t *testing.T) {
	created := "2026-01-01 00:00:00"
	target := "2027-01-01"                             // one year out
	today, _ := time.Parse("2006-01-02", "2026-07-02") // ~half the year elapsed, expected ~50%

	if got, expectedPct, ok := GoalStatus(created, target, 80, today); got != "ahead" || !ok || expectedPct < 49 || expectedPct > 51 {
		t.Errorf("progress 80%% at ~50%% elapsed = (%q, %v, %v), want (ahead, ~50, true)", got, expectedPct, ok)
	}
	if got, _, ok := GoalStatus(created, target, 20, today); got != "behind" || !ok {
		t.Errorf("progress 20%% at ~50%% elapsed = (%q, ok=%v), want behind", got, ok)
	}
	if got, _, ok := GoalStatus(created, target, 50, today); got != "onTrack" || !ok {
		t.Errorf("progress 50%% at ~50%% elapsed = (%q, ok=%v), want onTrack", got, ok)
	}
	if _, _, ok := GoalStatus(created, "", 50, today); ok {
		t.Errorf("no targetDate: ok = %v, want false", ok)
	}
}

// TestApplyRetirementPace pins the template's retirement-card semantics:
// marker = (age-25)/(retAge-25), "behind" only when the projection falls
// short of the need, otherwise ahead once progress reaches the marker.
func TestApplyRetirementPace(t *testing.T) {
	pct := func(v float64) *float64 { return &v }
	tests := []struct {
		name            string
		progress        float64
		projected, need float64
		wantStatus      string
	}{
		{"projection short is behind even if progress is past the marker", 80, 9e6, 10e6, "behind"},
		{"funded and past the marker is ahead", 40, 12e6, 10e6, "ahead"},
		{"funded but under the marker is on track", 10, 12e6, 10e6, "onTrack"},
	}
	for _, tt := range tests {
		g := &GoalItem{ProgressPct: pct(tt.progress)}
		ApplyRetirementPace(g, 36, 60, tt.projected, tt.need) // (36-25)/(60-25) = 31.43 -> 31
		if g.Status != tt.wantStatus {
			t.Errorf("%s: status = %q, want %q", tt.name, g.Status, tt.wantStatus)
		}
		if g.MarkPct == nil || *g.MarkPct != 31 {
			t.Errorf("%s: markPct = %v, want 31", tt.name, g.MarkPct)
		}
		if g.RetirementAge != 60 || g.ProjectedAtRetirement == nil || *g.ProjectedAtRetirement != tt.projected {
			t.Errorf("%s: retirement fields not set: %+v", tt.name, g)
		}
	}
}
