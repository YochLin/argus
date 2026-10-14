package web

import (
	"net/http"
	"os"
	"path/filepath"
	"time"

	"argus/internal/db"
	"argus/internal/llm"
	"argus/internal/logger"
)

// /api/wealth/reset (Phase 28 D) — the settings page's 重設整個管理帳戶:
// wipe the wealth account back to a first-run state. The trading account is
// never touched (db.ResetWealthData). There is no undo in the app; the
// database is backed up into the nightly backups' directory first, and
// restoring is swapping that file back in and restarting. The assistant
// can't reach this — its only write is wealth_drafts.

// wealthResetter is *db.DB; set only when cfg.DB != nil (apiDB's typed-nil
// reason).
type wealthResetter interface {
	Backup(destPath string) error
	ResetWealthData(resetAt string) error
	GetSetting(key string) (string, bool, error)
}

// resetBackupDir is the backup directory as an absolute path, for the dialog
// to tell the user where the copy goes.
func (s *Server) resetBackupDir() string {
	if abs, err := filepath.Abs(s.backupDir); err == nil {
		return abs
	}
	return s.backupDir
}

// handleWealthResetGet reports the last reset (null when never) and where the
// backup will be written.
func (s *Server) handleWealthResetGet(w http.ResponseWriter, r *http.Request) {
	if s.resetDB == nil {
		writeError(w, http.StatusConflict, "the database is not available")
		return
	}
	at, ok, err := s.resetDB.GetSetting(db.WealthResetAtKey)
	if err != nil {
		logger.Errorf("wealth reset: read last reset: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to read the last reset")
		return
	}
	var last *string
	if ok {
		last = &at
	}
	writeJSON(w, http.StatusOK, map[string]any{"lastResetAt": last, "backupDir": s.resetBackupDir()})
}

// handleWealthReset backs the database up, then wipes the wealth account and
// starts the assistant over. It holds the assistant's lock so a turn in
// progress can't add drafts to the account it just emptied; a failed backup
// stops it before anything is deleted.
func (s *Server) handleWealthReset(w http.ResponseWriter, r *http.Request) {
	if s.resetDB == nil {
		writeError(w, http.StatusConflict, "the database is not available")
		return
	}
	if !s.assistantMu.TryLock() {
		writeError(w, http.StatusConflict, "the assistant is still answering")
		return
	}
	defer s.assistantMu.Unlock()

	now := time.Now()
	// The nightly backup's pruning (BACKUP_RETENTION_DAYS) applies to this
	// file too — it's a safety net for second thoughts, not an archive.
	backup := filepath.Join(s.resetBackupDir(), "argus-before-wealth-reset-"+now.Format("20060102-150405")+".db")
	if err := os.MkdirAll(filepath.Dir(backup), 0o755); err != nil {
		logger.Errorf("wealth reset: create backup dir: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to back up the database; nothing was deleted")
		return
	}
	if err := s.resetDB.Backup(backup); err != nil {
		logger.Errorf("wealth reset: backup: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to back up the database; nothing was deleted")
		return
	}
	resetAt := now.Format("2006-01-02 15:04")
	if err := s.resetDB.ResetWealthData(resetAt); err != nil {
		logger.Errorf("wealth reset: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to reset the wealth account; nothing was deleted")
		return
	}
	if s.assistant != nil {
		s.assistant.ResetChat(llm.ChatWealth)
	}
	logger.Infof("wealth reset: done, backup at %s", backup)
	writeJSON(w, http.StatusOK, map[string]any{"lastResetAt": resetAt, "backup": backup})
}
