package db

import "testing"

func TestSaveLessonAndGetLessonsForTickers(t *testing.T) {
	d := newTestDB(t)

	if err := d.SaveLesson("AAPL", "2026-06-01", "exited too early, missed the rest of the move"); err != nil {
		t.Fatalf("SaveLesson() error = %v", err)
	}
	if err := d.SaveLesson("AAPL", "2026-07-01", "should have trimmed into strength"); err != nil {
		t.Fatalf("SaveLesson() error = %v", err)
	}
	if err := d.SaveLesson("MSFT", "2026-06-15", "held through an earnings miss against thesis"); err != nil {
		t.Fatalf("SaveLesson() error = %v", err)
	}

	got, err := d.GetLessonsForTickers([]string{"AAPL", "MSFT", "NVDA"})
	if err != nil {
		t.Fatalf("GetLessonsForTickers() error = %v", err)
	}
	if len(got["AAPL"]) != 2 {
		t.Fatalf("GetLessonsForTickers()[AAPL] = %+v, want 2 lessons", got["AAPL"])
	}
	// Oldest-first within a ticker.
	if got["AAPL"][0].Date != "2026-06-01" || got["AAPL"][1].Date != "2026-07-01" {
		t.Errorf("GetLessonsForTickers()[AAPL] order = %+v, want oldest-first", got["AAPL"])
	}
	if len(got["MSFT"]) != 1 || got["MSFT"][0].Lesson != "held through an earnings miss against thesis" {
		t.Errorf("GetLessonsForTickers()[MSFT] = %+v, want the one MSFT lesson", got["MSFT"])
	}
	if _, ok := got["NVDA"]; ok {
		t.Errorf("GetLessonsForTickers()[NVDA] present, want absent (no lessons recorded)")
	}
}

func TestGetLessonsForTickersEmptyInput(t *testing.T) {
	d := newTestDB(t)
	got, err := d.GetLessonsForTickers(nil)
	if err != nil || got != nil {
		t.Errorf("GetLessonsForTickers(nil) = %v, %v; want nil, nil", got, err)
	}
}

func TestGetRecentLessons(t *testing.T) {
	d := newTestDB(t)

	if err := d.SaveLesson("AAPL", "2026-06-01", "lesson one"); err != nil {
		t.Fatalf("SaveLesson() error = %v", err)
	}
	if err := d.SaveLesson("MSFT", "2026-06-15", "lesson two"); err != nil {
		t.Fatalf("SaveLesson() error = %v", err)
	}
	if err := d.SaveLesson("NVDA", "2026-07-01", "lesson three"); err != nil {
		t.Fatalf("SaveLesson() error = %v", err)
	}

	got, err := d.GetRecentLessons(2)
	if err != nil {
		t.Fatalf("GetRecentLessons() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("GetRecentLessons(2) = %+v, want exactly 2", got)
	}
	// Newest first.
	if got[0].Ticker != "NVDA" || got[1].Ticker != "MSFT" {
		t.Errorf("GetRecentLessons(2) = %+v, want [NVDA, MSFT] newest-first", got)
	}
}

func TestGetRecentLessonsEmptyTable(t *testing.T) {
	d := newTestDB(t)
	got, err := d.GetRecentLessons(5)
	if err != nil {
		t.Fatalf("GetRecentLessons() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("GetRecentLessons() on empty table = %+v, want empty", got)
	}
}
