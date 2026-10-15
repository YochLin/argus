package scheduler

import (
	"testing"
	"time"

	"github.com/robfig/cron/v3"
)

var testCST = time.FixedZone("CST", 8*3600)

var allJobs = []Job{
	DailyReport, ClosingSnapshot, TWClosingSnapshot, UniverseScan, TWDailyReport, TWUniverseScan,
	WeeklyReview, MonthlyReport, WealthHealthReport, MorningBriefing, TWMorningBriefing,
	LogRotation, Backup, SinopacSync, TWUniverseRefresh, SectorFlowScan, TWSectorFlowScan,
}

var specParser = cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

func mustParse(t *testing.T, j Job) cron.Schedule {
	t.Helper()
	sched, err := specParser.Parse(j.Spec)
	if err != nil {
		t.Fatalf("%s: spec %q does not parse: %v", j.Name, j.Spec, err)
	}
	return sched
}

func TestJobSpecsParse(t *testing.T) {
	seen := map[string]bool{}
	for _, j := range allJobs {
		mustParse(t, j)
		if seen[j.Name] {
			t.Errorf("duplicate job name %q", j.Name)
		}
		seen[j.Name] = true
	}
}

// The comments on each Job (and AGENTS.md) lean on these orderings: a job
// that reads another's output must fire after it, on the same day.
func TestJobOrdering(t *testing.T) {
	// 2026-10-06 is a Tuesday, 2026-10-05 a Monday: a day on which both the
	// US (Tue–Sat) and TW (Mon–Fri) jobs fire.
	usDay := time.Date(2026, 10, 6, 0, 0, 0, 0, testCST)
	twDay := time.Date(2026, 10, 5, 0, 0, 0, 0, testCST)

	for _, tc := range []struct {
		name  string
		day   time.Time
		order []Job
	}{
		{"US morning", usDay, []Job{ClosingSnapshot, UniverseScan, Backup, SectorFlowScan}},
		{"US briefing after the day's data", usDay, []Job{UniverseScan, MorningBriefing}},
		{"TW afternoon", twDay, []Job{TWClosingSnapshot, TWUniverseScan, SinopacSync, TWSectorFlowScan}},
		{"TW before the open and the report", twDay, []Job{TWMorningBriefing, TWDailyReport, SinopacSync}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var prev time.Time
			for i, j := range tc.order {
				// Next from midnight is that job's first fire of the day.
				at := mustParse(t, j).Next(tc.day)
				if at.Day() != tc.day.Day() {
					t.Fatalf("%s first fires on %s, not on %s", j.Name, at, tc.day.Format("2006-01-02"))
				}
				if i > 0 && !prev.Before(at) {
					t.Errorf("%s (%s) must fire after the previous job (%s)", j.Name, at.Format("15:04"), prev.Format("15:04"))
				}
				prev = at
			}
		})
	}

	// Monthly jobs: the report pair never shares a minute, and the TW pool
	// rotation precedes that day's TW universe scan.
	first := time.Date(2026, 10, 1, 0, 0, 0, 0, testCST)
	monthly, wealth := mustParse(t, MonthlyReport).Next(first), mustParse(t, WealthHealthReport).Next(first)
	if !monthly.Before(wealth) {
		t.Errorf("%s (%s) must precede %s (%s)", MonthlyReport.Name, monthly, WealthHealthReport.Name, wealth)
	}
	refresh, scan := mustParse(t, TWUniverseRefresh).Next(first), mustParse(t, TWUniverseScan).Next(first)
	if !refresh.Before(scan) || refresh.Day() != 1 {
		t.Errorf("%s (%s) must fire on the 1st, before %s (%s)", TWUniverseRefresh.Name, refresh, TWUniverseScan.Name, scan)
	}
}

// No other job may fire at the minute the log is rotated (AGENTS.md), or a
// rotation boundary splits two jobs' interleaved lines.
func TestNoJobFiresAtLogRotation(t *testing.T) {
	rot := mustParse(t, LogRotation).Next(time.Date(2026, 10, 5, 0, 0, 1, 0, testCST))
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, testCST)
	end := start.AddDate(0, 0, 35)
	for _, j := range allJobs {
		if j == LogRotation {
			continue
		}
		sched := mustParse(t, j)
		for at := sched.Next(start); at.Before(end); at = sched.Next(at) {
			if at.Hour() == rot.Hour() && at.Minute() == rot.Minute() {
				t.Errorf("%s fires at %s, the log-rotation minute", j.Name, at)
				break
			}
		}
	}
}
