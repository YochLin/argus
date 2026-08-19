package db

import (
	"testing"

	"argus/internal/market"
)

func TestInsertLLMRunAndListLLMRuns(t *testing.T) {
	d := newTestDB(t)

	if err := d.InsertLLMRun("recommend", market.US, "opus", 4200, `{"watchlist":[]}`, "raw reply", 2, 5, 8); err != nil {
		t.Fatalf("InsertLLMRun() error = %v", err)
	}
	if err := d.InsertLLMRun("daily_report", market.TW, "sonnet", 3100, `{"watchlist":[]}`, "raw reply 2", 1, 3, 4); err != nil {
		t.Fatalf("InsertLLMRun() error = %v", err)
	}

	got, err := d.ListLLMRuns(10)
	if err != nil {
		t.Fatalf("ListLLMRuns() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListLLMRuns() = %+v, want 2 rows", got)
	}
	// Newest first.
	if got[0].Kind != "daily_report" || got[0].Market != "tw" || got[0].Model != "sonnet" {
		t.Errorf("ListLLMRuns()[0] = %+v, want the daily_report/tw/sonnet row newest-first", got[0])
	}
	if got[1].WatchlistCount != 2 || got[1].CandidateCount != 5 || got[1].NewsCount != 8 {
		t.Errorf("ListLLMRuns()[1] counts = %+v, want {2,5,8}", got[1])
	}
}

func TestGetLLMRun(t *testing.T) {
	d := newTestDB(t)

	if err := d.InsertLLMRun("recommend", market.US, "opus", 4200, `{"watchlist":[]}`, "raw reply", 2, 5, 8); err != nil {
		t.Fatalf("InsertLLMRun() error = %v", err)
	}

	runs, err := d.ListLLMRuns(1)
	if err != nil || len(runs) != 1 {
		t.Fatalf("ListLLMRuns() = %+v, %v", runs, err)
	}

	got, ok, err := d.GetLLMRun(runs[0].ID)
	if err != nil {
		t.Fatalf("GetLLMRun() error = %v", err)
	}
	if !ok {
		t.Fatal("GetLLMRun() ok = false, want true")
	}
	if got.InputJSON != `{"watchlist":[]}` || got.OutputRaw != "raw reply" {
		t.Errorf("GetLLMRun() = %+v, want input/output round-tripped", got)
	}

	_, ok, err = d.GetLLMRun(runs[0].ID + 999)
	if err != nil {
		t.Fatalf("GetLLMRun(missing) error = %v", err)
	}
	if ok {
		t.Error("GetLLMRun(missing) ok = true, want false")
	}
}
