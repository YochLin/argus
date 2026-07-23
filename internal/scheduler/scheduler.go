package scheduler

import (
	"context"
	"log"
	"time"

	"github.com/robfig/cron/v3"
)

type JobFunc func(ctx context.Context)

type Scheduler struct {
	c *cron.Cron
}

// New creates a scheduler that runs in Taiwan timezone (CST, UTC+8).
// Uses a fixed zone so it works in Docker without tzdata installed.
func New() *Scheduler {
	cst := time.FixedZone("CST", 8*3600)
	return &Scheduler{
		c: cron.New(cron.WithLocation(cst), cron.WithSeconds()),
	}
}

// AddDailyReport schedules the daily report job at 23:30 CST every day, so
// it runs after the US market has actually opened rather than off
// yesterday's close — the point being a same-day, price-action-informed
// buy/sell call instead of a pre-market guess (the user's own framing: this
// is a low-frequency bot, one BUY/SELL/HOLD decision a day, and that
// decision should see today's trading before it's made).
//
// US market open isn't fixed relative to Taiwan time: it's 21:30 CST during
// US daylight saving (~Mar–Nov) and 22:30 CST during standard time
// (~Nov–Mar) — Taiwan has no DST of its own, so the gap to US ET shifts by
// an hour twice a year. A single fixed cron can't land exactly "+1h after
// open" in both regimes, so 23:30 was chosen to guarantee *at least* an
// hour of post-open trading either way (2h into the session during
// daylight saving, 1h during standard time) rather than risk firing before
// or right at the open in the worse-case season.
// Cron with seconds: "0 30 23 * * *"
func (s *Scheduler) AddDailyReport(ctx context.Context, fn JobFunc) {
	_, err := s.c.AddFunc("0 30 23 * * *", func() {
		log.Println("scheduler: running daily report")
		fn(ctx)
	})
	if err != nil {
		log.Fatalf("scheduler: add daily report: %v", err)
	}
	log.Println("scheduler: daily report registered at 23:30 CST")
}

// AddClosingSnapshot schedules the post-close snapshot job at 05:30 CST,
// Tuesday–Saturday (a US trading day Mon–Fri ends at 04:00 CST the next
// morning during daylight saving, 05:00 on standard time — 05:30 is past the
// close in both). Sunday/Monday mornings follow no US session, so they're
// excluded outright; US market holidays still fire but the job skips stale
// quotes itself.
func (s *Scheduler) AddClosingSnapshot(ctx context.Context, fn JobFunc) {
	_, err := s.c.AddFunc("0 30 5 * * 2-6", func() {
		log.Println("scheduler: running closing snapshot")
		fn(ctx)
	})
	if err != nil {
		log.Fatalf("scheduler: add closing snapshot: %v", err)
	}
	log.Println("scheduler: closing snapshot registered at 05:30 CST (Tue–Sat)")
}

// AddTWClosingSnapshot schedules the TW-market post-close snapshot job at
// 14:30 CST, Monday–Friday (Phase 6, see docs/phase-6-tw-market.md §3.3): the
// TWSE/TPEx session closes at 13:30 Taipei time (== CST, Taiwan and Taipei
// share the same UTC+8 offset with no DST on either side, so unlike
// AddDailyReport's US-session arithmetic this needs no cross-zone
// adjustment), and Yahoo's chart endpoint has the closing bar available well
// before 14:00 in practice — 14:30 leaves a 30-minute buffer past that.
func (s *Scheduler) AddTWClosingSnapshot(ctx context.Context, fn JobFunc) {
	_, err := s.c.AddFunc("0 30 14 * * 1-5", func() {
		log.Println("scheduler: running TW closing snapshot")
		fn(ctx)
	})
	if err != nil {
		log.Fatalf("scheduler: add TW closing snapshot: %v", err)
	}
	log.Println("scheduler: TW closing snapshot registered at 14:30 CST (Mon–Fri)")
}

// AddUniverseScan schedules Phase 2.6's chunked candidate-pool scan at 05:45
// CST, Tuesday–Saturday — after the closing snapshot (05:30) has updated
// daily_snapshots/positions data, and before the backup (06:00) so a fresh
// scan_hits row from today is included in that day's backup.
func (s *Scheduler) AddUniverseScan(ctx context.Context, fn JobFunc) {
	_, err := s.c.AddFunc("0 45 5 * * 2-6", func() {
		log.Println("scheduler: running universe scan")
		fn(ctx)
	})
	if err != nil {
		log.Fatalf("scheduler: add universe scan: %v", err)
	}
	log.Println("scheduler: universe scan registered at 05:45 CST (Tue–Sat)")
}

// AddWeeklyReview schedules Phase 3.6 PR2's Sunday portfolio review at 09:00
// CST — a weekend read, not a reactive alert, so unlike AddDailyReport/
// AddClosingSnapshot there's no market-open/close time to align with. By
// Sunday morning the most recent net_worth_snapshots/daily_snapshots row is
// already Friday's close (written by Saturday's 05:30 AddClosingSnapshot
// run), so this job needs no fresher data than what's already on disk.
func (s *Scheduler) AddWeeklyReview(ctx context.Context, fn JobFunc) {
	_, err := s.c.AddFunc("0 0 9 * * 0", func() {
		log.Println("scheduler: running weekly review")
		fn(ctx)
	})
	if err != nil {
		log.Fatalf("scheduler: add weekly review: %v", err)
	}
	log.Println("scheduler: weekly review registered at 09:00 CST (Sun)")
}

// AddMonthlyReport schedules Phase 3.6 追加項's net-worth monthly report
// (see docs/phase-3.6-monthly-report.md) at 09:30 CST on the 1st of every
// month. The day-of-month field takes "1" for the 1st; day-of-week is left
// "*" rather than restricted, since cron's "dom AND dow both restricted
// means OR, not AND" semantics would otherwise cause an unintended extra
// firing. 09:30 mirrors AddWeeklyReview's 09:00 "weekend/morning read"
// framing (no market open/close to align with) with a half-hour offset so
// the two never land in the same minute; by then the prior month's last
// trading day is always already snapshotted (worst case, a 1st that falls
// on a Saturday: AddClosingSnapshot's own 05:30 run is still hours earlier
// than 09:30). On the roughly-one-in-seven months the 1st is also a Sunday,
// both this and the weekly review fire — that's intentional (see the design
// doc): a purely rule-based monthly archive shouldn't skip a month just to
// avoid two Telegram messages same morning.
// Cron with seconds: "0 30 9 1 * *"
func (s *Scheduler) AddMonthlyReport(ctx context.Context, fn JobFunc) {
	_, err := s.c.AddFunc("0 30 9 1 * *", func() {
		log.Println("scheduler: running monthly report")
		fn(ctx)
	})
	if err != nil {
		log.Fatalf("scheduler: add monthly report: %v", err)
	}
	log.Println("scheduler: monthly report registered at 09:30 CST (1st of month)")
}

// AddLogRotation schedules fn (typically a rotating log writer's Rotate
// method) at midnight CST daily. lumberjack.Logger only rotates on size by
// itself, so this is what turns that into an actual daily rotation; MaxAge/
// MaxBackups on the logger handle pruning old files.
func (s *Scheduler) AddLogRotation(fn func()) {
	_, err := s.c.AddFunc("0 0 0 * * *", func() {
		log.Println("scheduler: rotating log")
		fn()
	})
	if err != nil {
		log.Fatalf("scheduler: add log rotation: %v", err)
	}
	log.Println("scheduler: log rotation registered at 00:00 CST")
}

// AddBackup schedules fn (the SQLite backup routine) at 06:00 CST daily,
// after the closing snapshot (05:30) so each day's backup captures that
// day's post-close data.
func (s *Scheduler) AddBackup(fn func()) {
	_, err := s.c.AddFunc("0 0 6 * * *", func() {
		log.Println("scheduler: running backup")
		fn()
	})
	if err != nil {
		log.Fatalf("scheduler: add backup: %v", err)
	}
	log.Println("scheduler: backup registered at 06:00 CST")
}

func (s *Scheduler) Start() {
	s.c.Start()
	log.Println("scheduler: started")
}

func (s *Scheduler) Stop() {
	s.c.Stop()
}
