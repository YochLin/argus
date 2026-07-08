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
