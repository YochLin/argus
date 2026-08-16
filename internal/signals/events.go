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
}

func DefaultEventThresholds(m market.MarketID) EventThresholds {
	if m == market.TW {
		return EventThresholds{GapPct: 7.0, BigMovePct: 10.0}
	}
	return EventThresholds{GapPct: 5.0, BigMovePct: 7.0}
}

// PriceEvent is one ticker's triggered gap and/or big-move event for a
// single closing snapshot. GapPct/ChangePct are only non-zero for the
// threshold(s) actually crossed, so a caller can tell a pure gap apart from
// a pure big move or both firing together — the same-day dedup that folds
// both into one price_events row happens one layer up (internal/bot).
type PriceEvent struct {
	Ticker    string
	GapPct    float64
	ChangePct float64
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
