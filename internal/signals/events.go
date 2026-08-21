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

// PriceEvent is one ticker's triggered gap and/or big-move and/or cumulative-
// decline event for a single closing snapshot. GapPct/ChangePct/CumulativePct
// are only non-zero for the threshold(s) actually crossed, so a caller can
// tell a pure gap apart from a pure big move (or a cumulative decline) or
// several firing together — the same-day dedup/merge that folds them into
// one price_events row happens one layer up (internal/bot).
type PriceEvent struct {
	Ticker        string
	GapPct        float64
	ChangePct     float64
	CumulativePct float64
}

// CheckPriceEvent screens q against t and returns the triggered event, or
// nil if neither the gap nor the big-move threshold was crossed.
// q.PrevClose == 0 (no prior-close data, e.g. a ticker's first session on
// record) skips both checks. q.Open == 0 (data.Quote's documented "has a
// close but no open" edge case) skips only the gap check — the big-move
// check doesn't depend on Open and still runs.
func CheckPriceEvent(q *data.Quote, t EventThresholds) *PriceEvent {
	if q.PrevClose == 0 {
		return nil
	}

	var ev PriceEvent
	if q.Open != 0 {
		gapPct := (q.Open - q.PrevClose) / q.PrevClose * 100
		if math.Abs(gapPct) >= t.GapPct {
			ev.GapPct = gapPct
		}
	}
	if math.Abs(q.ChangePercent) >= t.BigMovePct {
		ev.ChangePct = q.ChangePercent
	}
	if ev.GapPct == 0 && ev.ChangePct == 0 {
		return nil
	}
	ev.Ticker = q.Ticker
	return &ev
}

// CheckCumulativeDecline screens todayClose against windowAgoClose (the
// close t.CumulativeWindowDays sessions ago) for a decline of at least
// t.CumulativeDeclinePct — the multi-day case CheckPriceEvent's single-day
// checks can't see. windowAgoClose <= 0 (not enough daily_snapshots history
// yet for a new ticker) skips the check. Only fires on a decline, not a
// rally — "累積跌幅" per its name, unlike GapPct/ChangePct which are
// bidirectional.
func CheckCumulativeDecline(ticker string, windowAgoClose, todayClose float64, t EventThresholds) *PriceEvent {
	if windowAgoClose <= 0 {
		return nil
	}
	pct := (todayClose - windowAgoClose) / windowAgoClose * 100
	if pct > -t.CumulativeDeclinePct {
		return nil
	}
	return &PriceEvent{Ticker: ticker, CumulativePct: pct}
}
