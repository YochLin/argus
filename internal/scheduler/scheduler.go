package scheduler

import (
	"context"
	"time"

	"github.com/robfig/cron/v3"

	"argus/internal/logger"
)

type JobFunc func(ctx context.Context)

// Job is one cron entry: a name for the logs and a robfig/cron spec WITH a
// leading seconds field, read in CST. The jobs below are the whole schedule;
// where each one sits relative to the others is the point of the comments,
// and scheduler_test.go pins the orderings those comments depend on.
type Job struct {
	Name string
	Spec string
}

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

// Add registers fn to run on job's schedule, passing it ctx. A spec that
// doesn't parse is a bug in this file, not an operating condition, so it is
// fatal at boot.
func (s *Scheduler) Add(ctx context.Context, job Job, fn JobFunc) {
	_, err := s.c.AddFunc(job.Spec, func() {
		logger.Infof("scheduler: running %s", job.Name)
		fn(ctx)
	})
	if err != nil {
		logger.Fatalf("scheduler: add %s: %v", job.Name, err)
	}
	logger.Infof("scheduler: %s registered (%s, CST)", job.Name, job.Spec)
}

// DailyReport runs the daily report job at 23:30 CST every day, so it runs
// after the US market has actually opened rather than off yesterday's close —
// the point being a same-day, price-action-informed buy/sell call instead of
// a pre-market guess (the user's own framing: this is a low-frequency bot,
// one BUY/SELL/HOLD decision a day, and that decision should see today's
// trading before it's made).
//
// US market open isn't fixed relative to Taiwan time: it's 21:30 CST during
// US daylight saving (~Mar–Nov) and 22:30 CST during standard time
// (~Nov–Mar) — Taiwan has no DST of its own, so the gap to US ET shifts by
// an hour twice a year. A single fixed cron can't land exactly "+1h after
// open" in both regimes, so 23:30 was chosen to guarantee *at least* an
// hour of post-open trading either way (2h into the session during
// daylight saving, 1h during standard time) rather than risk firing before
// or right at the open in the worse-case season.
var DailyReport = Job{"daily report", "0 30 23 * * *"}

// ClosingSnapshot runs the post-close snapshot job at 05:30 CST,
// Tuesday–Saturday (a US trading day Mon–Fri ends at 04:00 CST the next
// morning during daylight saving, 05:00 on standard time — 05:30 is past the
// close in both). Sunday/Monday mornings follow no US session, so they're
// excluded outright; US market holidays still fire but the job skips stale
// quotes itself.
var ClosingSnapshot = Job{"closing snapshot", "0 30 5 * * 2-6"}

// TWClosingSnapshot runs the TW-market post-close snapshot job at 14:30 CST,
// Monday–Friday (Phase 6, see docs/phase-6-tw-market.md §3.3): the TWSE/TPEx
// session closes at 13:30 Taipei time (== CST, Taiwan and Taipei share the
// same UTC+8 offset with no DST on either side, so unlike DailyReport's
// US-session arithmetic this needs no cross-zone adjustment), and Yahoo's
// chart endpoint has the closing bar available well before 14:00 in practice
// — 14:30 leaves a 30-minute buffer past that.
var TWClosingSnapshot = Job{"TW closing snapshot", "0 30 14 * * 1-5"}

// UniverseScan runs Phase 2.6's candidate-pool scan at 05:45 CST,
// Tuesday–Saturday — after the closing snapshot (05:30) has updated
// daily_snapshots/positions data, and before the backup (06:00) so a fresh
// scan_hits row from today is included in that day's backup. That 15-minute
// window is why bot.universeScanRequestDelay is a knob and not a bigger
// number: the full-universe scan it now runs takes ~10 min, so a delay much
// past 1s starts spilling today's later hits into tomorrow's backup (a
// cosmetic loss, not data loss — Backup's VACUUM INTO is a plain read and
// won't contend with the scan's writes).
var UniverseScan = Job{"universe scan", "0 45 5 * * 2-6"}

// TWDailyReport runs the TW-market daily report job at 11:30 CST,
// Monday-Friday — moved from the original 15:00 post-close slot (Phase 6
// PR2, see docs/phase-6-tw-market.md §3.3/§5.1) to mirror DailyReport's
// US-session philosophy: an intraday snapshot of signals/LLM recommendations
// taken while the market is open, not a post-close recap. 11:30 is ~2.5h
// into the 9:00 TWSE/TPEx open, before the 13:30 close. Since this now runs
// before that day's TWClosingSnapshot (14:30) and TWUniverseScan (14:40), it
// picks up the previous trading day's scan_hits instead of a same-day one —
// the same pipeline shape DailyReport already uses against the prior
// evening's US universe scan, so no job reordering is needed.
var TWDailyReport = Job{"TW daily report", "0 30 11 * * 1-5"}

// TWUniverseScan runs Phase 6 PR2's TW-market universe scan at 14:40 CST,
// Monday-Friday — after TWClosingSnapshot (14:30) has updated
// daily_snapshots for the day. Its scan_hits row is picked up by the next
// trading day's 11:30 TWDailyReport rather than the same day's (that report
// now runs before this scan, not after — see TWDailyReport), mirroring how
// UniverseScan's overnight US scan feeds the next US session's DailyReport.
var TWUniverseScan = Job{"TW universe scan", "0 40 14 * * 1-5"}

// WeeklyReview runs Phase 3.6 PR2's Sunday portfolio review at 09:00 CST — a
// weekend read, not a reactive alert, so unlike DailyReport/ClosingSnapshot
// there's no market-open/close time to align with. By Sunday morning the
// most recent net_worth_snapshots/daily_snapshots row is already Friday's
// close (written by Saturday's 05:30 ClosingSnapshot run), so this job needs
// no fresher data than what's already on disk.
var WeeklyReview = Job{"weekly review", "0 0 9 * * 0"}

// MonthlyReport runs Phase 3.6 追加項's net-worth monthly report (see
// docs/phase-3.6-monthly-report.md) at 09:30 CST on the 1st of every month.
// The day-of-month field takes "1" for the 1st; day-of-week is left "*"
// rather than restricted, since cron's "dom AND dow both restricted means OR,
// not AND" semantics would otherwise cause an unintended extra firing. 09:30
// mirrors WeeklyReview's 09:00 "weekend/morning read" framing (no market
// open/close to align with) with a half-hour offset so the two never land in
// the same minute; by then the prior month's last trading day is always
// already snapshotted (worst case, a 1st that falls on a Saturday:
// ClosingSnapshot's own 05:30 run is still hours earlier than 09:30). On the
// roughly-one-in-seven months the 1st is also a Sunday, both this and the
// weekly review fire — that's intentional (see the design doc): a purely
// rule-based monthly archive shouldn't skip a month just to avoid two
// Telegram messages same morning.
var MonthlyReport = Job{"monthly report", "0 30 9 1 * *"}

// WealthHealthReport runs Phase 9 波次3 PR9's monthly financial health report
// (bot.RunWealthHealthReport) at 10:00 CST on the 1st of every month — 30
// minutes after MonthlyReport's 09:30 run so the two unrelated monthly
// reports (stock net worth vs. wealth-platform health metrics) never land in
// the same minute, same spacing reasoning as MonthlyReport's own doc comment.
// Same "*" day-of-week (not "1") as MonthlyReport, for the same cron
// OR-semantics reason.
var WealthHealthReport = Job{"wealth health report", "0 0 10 1 * *"}

// MorningBriefing runs the US-market morning briefing at 07:00 CST,
// Tuesday–Saturday — a read-only narrative recap of the US session that just
// closed (indices, macro backdrop, watchlist/mover highlights), distinct
// from DailyReport's own 23:30 CST run later the same evening, which makes an
// actual BUY/SELL/HOLD call informed by the *next*, still-open session. Same
// Tue–Sat weekday coverage as ClosingSnapshot/UniverseScan (Tue recaps Mon's
// session, ..., Sat recaps Fri's; no Sun/Mon firing since there's no fresh
// weekend session to recap), scheduled after both of those (05:30/05:45) so
// the day's post-close data is already on disk by 07:00.
var MorningBriefing = Job{"morning briefing", "0 0 7 * * 2-6"}

// TWMorningBriefing runs the TW-market pre-open briefing at 08:30 CST,
// Monday–Friday (TW's own trading calendar, not the US Tue–Sat shifted week
// MorningBriefing uses) — a read-only narrative recap 30 minutes before TW's
// 09:00 open: prior TW close, overnight US performance, VIX, TW market news.
// Distinct from TWDailyReport's 11:30 CST run, which makes an actual
// BUY/SELL/HOLD call once the TW session is already underway. 08:30 sits
// before both TWDailyReport (11:30) and the afternoon cluster of
// TWClosingSnapshot/TWUniverseScan (14:30/14:40) — "how did overnight US
// affect TW today" is only useful pre-open, so this can't be folded into
// either of those later jobs.
var TWMorningBriefing = Job{"TW morning briefing", "0 30 8 * * 1-5"}

// LogRotation runs the rotating log writer's Rotate at midnight CST daily.
// lumberjack.Logger only rotates on size by itself, so this is what turns
// that into an actual daily rotation; MaxAge/MaxBackups on the logger handle
// pruning old files. No other job may fire at 00:00 — a rotation boundary is
// no place for two jobs' log lines to interleave.
var LogRotation = Job{"log rotation", "0 0 0 * * *"}

// Backup runs the SQLite backup routine at 06:00 CST daily, after the
// closing snapshot (05:30) so each day's backup captures that day's
// post-close data.
var Backup = Job{"backup", "0 0 6 * * *"}

// SinopacSync runs Phase 16's Shioaji auto-bookkeeping sync at 14:50 CST,
// Monday-Friday — after the after-hours odd-lot session's 14:30 match, since
// the sync only proposes the day's own trades (see sinopac.Trades) and a
// trade it misses is never proposed later. On its own minute, apart from
// TWClosingSnapshot (14:30) and TWUniverseScan (14:40). A no-op
// (SHIOAJI_ADDR unset) when Sinopac isn't configured — fn handles that
// internally, same as every other job here.
var SinopacSync = Job{"sinopac sync", "0 50 14 * * 1-5"}

// TWUniverseRefresh runs the TW scan pool's liquidity rotation at 08:30 CST
// on the 1st of every month. Monthly, not daily: the top-300 by turnover
// barely moves week to week, and a pool that churned daily would make the
// universe scan's signal_states dedup meaningless for anything near the cut
// line (a ticker rotating in and out re-fires its first signal every time it
// returns). 08:30 is before the 09:00 TW open, so the ranking it reads is
// whole trading sessions only, and well clear of that day's 14:40
// TWUniverseScan, which then scans the freshly rotated pool. Day-of-month
// "1" with day-of-week left "*" for the same cron OR-semantics reason
// MonthlyReport documents. A no-op when SHIOAJI_ADDR is unset — fn handles
// that internally, same as SinopacSync.
var TWUniverseRefresh = Job{"TW universe refresh", "0 30 8 1 * *"}

// SectorFlowScan runs Phase 18's US sector money-flow scan at 06:10 CST,
// Tuesday–Saturday — after the 06:00 backup (this job makes no DB writes, so
// it can't corrupt that VACUUM INTO read, but starting after it keeps the two
// off each other's back regardless) and well clear of the 05:45 universe
// scan's own ~10-minute run. A no-op (fn checks its own Sector/IndustryMap
// providers) when FINNHUB_API_KEY isn't configured.
var SectorFlowScan = Job{"sector flow scan (us)", "0 10 6 * * 2-6"}

// TWSectorFlowScan is SectorFlowScan's TW counterpart, at 14:55 CST
// Monday–Friday — 15 minutes after TWUniverseScan (14:40), comfortable
// clearance for the tw150 pool's much shorter scan (~150 tickers vs. the US
// scan's ~500, and a single FinMind call for classification instead of one
// per ticker).
var TWSectorFlowScan = Job{"sector flow scan (tw)", "0 55 14 * * 1-5"}

func (s *Scheduler) Start() {
	s.c.Start()
	logger.Info("scheduler: started")
}

func (s *Scheduler) Stop() {
	s.c.Stop()
}

// JobCount reports how many jobs are registered. Exists for internal/app's
// boot test, which has no other way to tell "this deployment schedules its
// closing snapshot" from "this deployment silently schedules nothing" — the
// exact regression Phase 24 Stage 3's Telegram gate used to cause.
func (s *Scheduler) JobCount() int { return len(s.c.Entries()) }
