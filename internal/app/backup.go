package app

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"argus/internal/db"
	"argus/internal/logger"
)

var cst = time.FixedZone("CST", 8*3600)

// runBackup writes a dated SQLite backup (via db.Backup's VACUUM INTO) into
// dir and prunes backup files older than retentionDays. transactions/
// positions are irreplaceable personal financial data with no other backup
// path on a single VPS, hence a daily on-disk copy — see PLAN.md. prefix
// distinguishes the real ("argus") and paper-account ("argus-paper", Phase
// 11 PR3) backup files sharing one dir — pruneOldBackups doesn't filter by
// prefix, so calling this for both databases just prunes the shared dir
// twice, which is harmless at once-a-day cron frequency.
func runBackup(database *db.DB, dir string, retentionDays int, prefix string) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logger.Errorf("backup: create dir: %v", err)
		return
	}

	dest := filepath.Join(dir, fmt.Sprintf("%s-%s.db", prefix, time.Now().In(cst).Format("2006-01-02")))
	if err := database.Backup(dest); err != nil {
		logger.Errorf("backup: %v", err)
		return
	}
	logger.Infof("backup: wrote %s", dest)

	pruneOldBackups(dir, retentionDays)
}

func pruneOldBackups(dir string, retentionDays int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		logger.Errorf("backup: prune: read dir: %v", err)
		return
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			logger.Errorf("backup: prune: stat %s: %v", e.Name(), err)
			continue
		}
		if info.ModTime().After(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
			logger.Errorf("backup: prune: remove %s: %v", e.Name(), err)
		}
	}
}
