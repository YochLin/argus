package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"argus/internal/db"
	"argus/internal/i18n"
)

type resetResponse struct {
	LastResetAt *string `json:"lastResetAt"`
	BackupDir   string  `json:"backupDir"`
	Backup      string  `json:"backup"`
}

func getReset(t *testing.T, s *Server, cookie *http.Cookie) resetResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/wealth/reset", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET = %d: %s", rec.Code, rec.Body.String())
	}
	var r resetResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestWealthReset(t *testing.T) {
	chat := &fakeChatter{}
	s, cookie := newAssistantTestServer(t, chat)
	s.backupDir = filepath.Join(t.TempDir(), "backups")
	d := s.resetDB.(*db.DB)
	if _, err := d.CreateGoal(db.NewGoal{Name: "Trip", TargetAmount: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.RecordBuy("AAPL", 1, 100, 0, "2026-10-01"); err != nil {
		t.Fatal(err)
	}

	if r := getReset(t, s, cookie); r.LastResetAt != nil || r.BackupDir != s.backupDir {
		t.Fatalf("before = %+v, want never reset and the backup dir", r)
	}

	rec := postAssistant(t, s, cookie, "/api/wealth/reset", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST = %d: %s", rec.Code, rec.Body.String())
	}
	var r resetResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	// The backup is the database as it was: the goal is still in it.
	backup, err := db.New(r.Backup)
	if err != nil {
		t.Fatalf("open backup %q: %v", r.Backup, err)
	}
	defer backup.Close()
	if goals, _ := backup.ListGoals(); len(goals) != 1 {
		t.Errorf("backup goals = %d, want the 1 from before the reset", len(goals))
	}
	if goals, _ := d.ListGoals(); len(goals) != 0 {
		t.Errorf("goals after reset = %d, want 0", len(goals))
	}
	if pos, _ := d.GetPositions(); len(pos) != 1 {
		t.Errorf("positions after reset = %d, want the trading one untouched", len(pos))
	}
	if chat.resets != 1 {
		t.Errorf("assistant resets = %d, want 1", chat.resets)
	}
	if got := getReset(t, s, cookie); got.LastResetAt == nil || *got.LastResetAt != *r.LastResetAt {
		t.Errorf("last reset = %v, want %q", got.LastResetAt, *r.LastResetAt)
	}

	// Busy assistant: refused, nothing written.
	s.assistantMu.Lock()
	if rec := postAssistant(t, s, cookie, "/api/wealth/reset", nil); rec.Code != http.StatusConflict {
		t.Errorf("reset during a turn = %d, want 409", rec.Code)
	}
	s.assistantMu.Unlock()
}

func TestWealthReset_BackupFailureDeletesNothing(t *testing.T) {
	s, cookie := newAssistantTestServer(t, &fakeChatter{})
	// A file where the directory should be: MkdirAll fails.
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	s.backupDir = filepath.Join(blocker, "backups")
	d := s.resetDB.(*db.DB)
	if _, err := d.CreateGoal(db.NewGoal{Name: "Trip", TargetAmount: 1}); err != nil {
		t.Fatal(err)
	}
	if rec := postAssistant(t, s, cookie, "/api/wealth/reset", nil); rec.Code != http.StatusInternalServerError {
		t.Fatalf("POST = %d, want 500", rec.Code)
	}
	if goals, _ := d.ListGoals(); len(goals) != 1 {
		t.Errorf("goals = %d, want the goal kept when the backup fails", len(goals))
	}
}

func TestWealthReset_ReadOnly(t *testing.T) {
	d, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	s := New(Config{DB: d, Lang: i18n.EN})
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/wealth/reset", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("read-only POST = %d, want 404", rec.Code)
	}
}
