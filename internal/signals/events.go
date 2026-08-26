package signals

import (
	"math"

	"argus/internal/data"
	"argus/internal/market"
)

// EventThresholds are Phase 20's price-event gap/big-move thresholds — same
// "per-market struct, not env-configurable" shape as ScreenParams (see
// DefaultScreenParams's doc comment and docs/phase-20-price-event-log.md §2
// "門檻可調性": these are calibration starting points, deliberately not
// tunable via env var).
type EventThresholds struct {
	GapPct     float64 // gap % (open vs prev close), US 5 / TW 7
	BigMovePct float64 // big move % (close vs prev close), US 7 / TW 10 — TW's
	// value sits right at the ±10% daily limit, which is expected: hitting the
	// limit is exactly the kind of day worth recording.

	// CumulativeDeclinePct/CumulativeWindowDays catch the "death by a
	// thousand cuts" case GapPct/BigMovePct miss: several days each under
	// the single-day threshold that add up to a real decline. Starting-point
	// calibration, same "not backtested" caveat as GapPct/BigMovePct — US 8%
	// over 5 sessions, TW 10% (wider, same daily-limit-driven reasoning as
	// BigMovePct).
	CumulativeDeclinePct float64
	CumulativeWindowDays int
}

func DefaultEventThresholds(m market.MarketID) EventThresholds {
	if m == market.TW {
		return EventThresholds{GapPct: 7.0, BigMovePct: 10.0, CumulativeDeclinePct: 10.0, CumulativeWindowDays: 5}
	}
	return EventThresholds{GapPct: 5.0, BigMovePct: 7.0, CumulativeDeclinePct: 8.0, CumulativeWindowDays: 5}
}

// PriceEvent is one ticker's triggered price event for a single closing
// snapshot — distinct from db.PriceEvent (same name, different package,
// same convention as db.DailySnapshot vs this package staying independent
// of each other).
//
// All three percentages carry the day's real value whenever it could be
// computed at all, not only when its own threshold fired (Phase 20 後續 PR3);
// which threshold actually fired is what the *Triggered flags say. Before
// that split, "gapped -6% but closed down only 1%" (opened weak, bought all
// day — the most interesting thing that can happen) and "gapped -6%, closed
// -8%" reached the LLM as byte-identical prompts, because the non-triggering
// number was zeroed out. A summary whose entire job is stating facts was
// missing exactly the facts.
//
// 0 still means "unavailable" for GapPct (no open price) and CumulativePct
// (not enough daily_snapshots history) — a value that genuinely rounds to
// 0.0% carries nothing a reader or a model would miss.
type PriceEvent struct {
	Ticker              string
	GapPct              float64
	ChangePct           float64
	CumulativePct       float64
	GapTriggered        bool
	ChangeTriggered     bool
	CumulativeTriggered bool
}

// Triggered reports whether any threshold fired — the "is this an event at
// all" question, kept next to the flags so no caller has to re-OR them.
func (e PriceEvent) Triggered() bool {
	return e.GapTriggered || e.ChangeTriggered || e.CumulativeTriggered
}

// CheckPriceEvent screens q against t and returns the day's numbers, or nil
// if no threshold was crossed. windowAgoClose is the close
// t.CumulativeWindowDays sessions ago (0 when the caller has no such
// history yet), covering the multi-day decline the single-day checks can't
// see; only a decline triggers it, never a rally — "累積跌幅" per its name,
// unlike the bidirectional gap/big-move checks.
//
// Each of the three checks degrades on its own: q.PrevClose == 0 (no prior
// close on record, e.g. a ticker's first session) skips both single-day
// checks, q.Open == 0 (data.Quote's documented "has a close but no open"
// case) skips only the gap one, and windowAgoClose <= 0 skips the
// cumulative one. Gap and cumulative decline were two functions until Phase
// 20 後續 PR3 folded them together — with every number computed every time,
// a caller merging two partial results back into one row (price_events is
// one row per ticker per day) was pure ceremony.
func CheckPriceEvent(q *data.Quote, windowAgoClose float64, t EventThresholds) *PriceEvent {
	ev := PriceEvent{Ticker: q.Ticker}

	if q.PrevClose != 0 {
		if q.Open != 0 {
			ev.GapPct = (q.Open - q.PrevClose) / q.PrevClose * 100
			ev.GapTriggered = math.Abs(ev.GapPct) >= t.GapPct
		}
		ev.ChangePct = q.ChangePercent
		ev.ChangeTriggered = math.Abs(ev.ChangePct) >= t.BigMovePct
	}

	if windowAgoClose > 0 {
		ev.CumulativePct = (q.Price - windowAgoClose) / windowAgoClose * 100
		ev.CumulativeTriggered = ev.CumulativePct <= -t.CumulativeDeclinePct
	}

	if !ev.Triggered() {
		return nil
	}
	return &ev
}
