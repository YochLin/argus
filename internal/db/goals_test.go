package db

import (
	"database/sql"
	"testing"
)

func TestCreateListDeleteGoal(t *testing.T) {
	d := newTestDB(t)

	id, err := d.CreateGoal(NewGoal{Name: "緊急預備金", TargetAmount: 300000, TargetDate: "2027-01-01"})
	if err != nil {
		t.Fatalf("CreateGoal: %v", err)
	}
	id2, err := d.CreateGoal(NewGoal{Name: "退休", Kind: "retirement", TargetAmount: 20000000, Currency: "TWD"})
	if err != nil {
		t.Fatalf("CreateGoal retirement: %v", err)
	}

	goals, err := d.ListGoals()
	if err != nil {
		t.Fatalf("ListGoals: %v", err)
	}
	if len(goals) != 2 {
		t.Fatalf("len(goals) = %d, want 2", len(goals))
	}
	// Newest first.
	if goals[0].ID != id2 || goals[0].Kind != "retirement" {
		t.Errorf("goals[0] = %+v, want id=%d kind=retirement", goals[0], id2)
	}
	if goals[1].ID != id || goals[1].Kind != "general" {
		t.Errorf("goals[1] = %+v, want id=%d kind=general (defaulted)", goals[1], id)
	}
	if goals[1].TargetDate != "2027-01-01" {
		t.Errorf("goals[1].TargetDate = %q, want 2027-01-01", goals[1].TargetDate)
	}

	if err := d.DeleteGoal(id); err != nil {
		t.Fatalf("DeleteGoal: %v", err)
	}
	goals, err = d.ListGoals()
	if err != nil {
		t.Fatalf("ListGoals after delete: %v", err)
	}
	if len(goals) != 1 || goals[0].ID != id2 {
		t.Fatalf("goals after delete = %+v, want only id=%d", goals, id2)
	}
}

func TestSetGoalAsset(t *testing.T) {
	d := newTestDB(t)

	goalID, _ := d.CreateGoal(NewGoal{Name: "換屋頭期款", TargetAmount: 2000000})
	assetID, err := d.CreateAsset(NewAsset{Side: "asset", Type: "deposit", Name: "玉山活存"})
	if err != nil {
		t.Fatalf("CreateAsset: %v", err)
	}

	if err := d.SetGoalAsset(goalID, assetID, 0.5); err != nil {
		t.Fatalf("SetGoalAsset: %v", err)
	}
	earmarks, err := d.ListAllGoalAssets()
	if err != nil {
		t.Fatalf("ListAllGoalAssets: %v", err)
	}
	if len(earmarks) != 1 || earmarks[0].Ratio != 0.5 {
		t.Fatalf("earmarks = %+v, want one row ratio=0.5", earmarks)
	}

	// Re-setting the same pair updates the ratio in place, not a second row.
	if err := d.SetGoalAsset(goalID, assetID, 0.8); err != nil {
		t.Fatalf("SetGoalAsset update: %v", err)
	}
	earmarks, _ = d.ListAllGoalAssets()
	if len(earmarks) != 1 || earmarks[0].Ratio != 0.8 {
		t.Fatalf("earmarks after update = %+v, want one row ratio=0.8", earmarks)
	}

	// ratio <= 0 removes the earmark.
	if err := d.SetGoalAsset(goalID, assetID, 0); err != nil {
		t.Fatalf("SetGoalAsset remove: %v", err)
	}
	earmarks, _ = d.ListAllGoalAssets()
	if len(earmarks) != 0 {
		t.Fatalf("earmarks after remove = %+v, want none", earmarks)
	}
}

func TestUpdateGoalPersistsDrawerFields(t *testing.T) {
	d := newTestDB(t)
	id, err := d.CreateGoal(NewGoal{Name: "旅行基金", TargetAmount: 600000, TargetDate: "2028-12-31"})
	if err != nil {
		t.Fatalf("CreateGoal: %v", err)
	}
	retID, err := d.CreateGoal(NewGoal{Name: "退休", Kind: "retirement", TargetAmount: 1})
	if err != nil {
		t.Fatalf("CreateGoal retirement: %v", err)
	}

	saved, monthly := 470000.0, 6000.0
	err = d.UpdateGoal(id, NewGoal{Name: "長假旅行", Note: "三個月", TargetAmount: 900000, TargetDate: "2028-12-31",
		SavedAmount: &saved, MonthlyContribution: &monthly, StartYear: 2024})
	if err != nil {
		t.Fatalf("UpdateGoal: %v", err)
	}
	goals, _ := d.ListGoals()
	var got Goal
	for _, g := range goals {
		if g.ID == id {
			got = g
		}
	}
	if got.Name != "長假旅行" || got.TargetAmount != 900000 || got.Note != "三個月" || got.StartYear != 2024 {
		t.Errorf("updated goal = %+v", got)
	}
	if got.SavedAmount == nil || *got.SavedAmount != 470000 || got.MonthlyContribution == nil || *got.MonthlyContribution != 6000 {
		t.Errorf("saved/monthly = %v/%v, want 470000/6000", got.SavedAmount, got.MonthlyContribution)
	}

	// The retirement row is derived data — UpdateGoal must not touch it.
	if err := d.UpdateGoal(retID, NewGoal{Name: "x", TargetAmount: 2}); err != sql.ErrNoRows {
		t.Errorf("UpdateGoal(retirement) err = %v, want sql.ErrNoRows", err)
	}
}
