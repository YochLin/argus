package bot

import (
	"argus/internal/i18n"
	"argus/internal/logger"
	"argus/internal/notification"
)

// recoverJobPanic recovers from a panic inside a scheduler-invoked job
// (RunDailyReport/RunClosingSnapshot), logging it and alerting the user over
// Telegram. Without this, a panic in either job would kill that goroutine
// silently — the bot keeps running and answering commands, but the VPS is
// unattended, so a failed daily report or closing snapshot would otherwise
// go completely unnoticed. job names the job for the log line and alert.
func (b *Bot) recoverJobPanic(job string) {
	if r := recover(); r != nil {
		logger.Errorf("%s: panic: %v", job, r)
		b.publishAlert("job_panic", notification.LevelCritical, i18n.T(b.lang, i18n.KeyJobPanic, job, r))
	}
}
