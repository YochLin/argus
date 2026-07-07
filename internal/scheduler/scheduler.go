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

// AddDailyReport schedules the daily report job at 21:00 CST every day
// (US market opens at 21:30 CST on standard time, 20:30 during daylight saving).
// Cron with seconds: "0 0 21 * * *"
func (s *Scheduler) AddDailyReport(ctx context.Context, fn JobFunc) {
	_, err := s.c.AddFunc("0 0 21 * * *", func() {
		log.Println("scheduler: running daily report")
		fn(ctx)
	})
	if err != nil {
		log.Fatalf("scheduler: add daily report: %v", err)
	}
	log.Println("scheduler: daily report registered at 21:00 CST")
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

func (s *Scheduler) Start() {
	s.c.Start()
	log.Println("scheduler: started")
}

func (s *Scheduler) Stop() {
	s.c.Stop()
}
