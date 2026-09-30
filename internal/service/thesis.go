package service

import (
	"errors"
	"time"

	"argus/internal/db"
)

// Phase 27 P1b: a thesis is one overwritable text per (ticker, round), stored
// in db.round_theses keyed by the round's start date. PendingRound ("") is
// the draft for a ticker with no open round (a watchlist name); db.RecordBuy
// re-keys it to the new round's start date when the first buy lands, so a
// draft is never shown for one round and then lost to the next.
//
// The Phase 21 journal (thesis_entries) is no longer written to; it stays as
// a read-only fallback so a round nobody has re-saved yet still shows what
// was last written during it.
const PendingRound = ""

// ErrThesisRoundNotFound means a write named a round start the ticker never had.
var ErrThesisRoundNotFound = errors.New("no such round for this ticker")

// ThesisReader is the slice of *db.DB the read side needs.
type ThesisReader interface {
	GetRoundThesis(ticker, roundStart string) (string, bool, error)
	GetThesisEntriesInRange(ticker, from, to string) ([]db.ThesisEntry, error)
}

// ThesisStore adds what CurrentThesis/SetCurrentThesis need to find the open
// round themselves. *db.DB satisfies it.
type ThesisStore interface {
	ThesisReader
	GetTransactions(ticker string) ([]db.Transaction, error)
	SetRoundThesis(ticker, roundStart, text string) error
}

// Thesis is a round's resolved text. Edited is false when it came from the
// legacy journal fallback rather than a saved per-round row.
type Thesis struct {
	Text   string
	Edited bool
}

// CurrentRoundKey is the open round's start date, or PendingRound when the
// ticker isn't held. rounds must come from SegmentRounds.
func CurrentRoundKey(rounds []Round) string {
	if n := len(rounds); n > 0 && rounds[n-1].EndDate == "" {
		return rounds[n-1].StartDate
	}
	return PendingRound
}

// ThesisRoundKey resolves which round a write targets: wanted == "" means the
// ticker's current round; otherwise wanted must be one of rounds' start dates.
func ThesisRoundKey(rounds []Round, wanted string) (string, error) {
	if wanted == "" {
		return CurrentRoundKey(rounds), nil
	}
	for _, r := range rounds {
		if r.StartDate == wanted {
			return wanted, nil
		}
	}
	return "", ErrThesisRoundNotFound
}

// RoundThesis returns the thesis of the round starting at start (PendingRound
// for the not-yet-open one), or ok=false when there is none. rounds must be
// ticker's SegmentRounds.
//
// Fallback when no per-round row exists: the latest journal entry written
// between the day after the previous round closed and this round's end — the
// start bound is what lets a thesis jotted before the buy still count.
func RoundThesis(st ThesisReader, ticker string, rounds []Round, start string) (Thesis, bool, error) {
	text, ok, err := st.GetRoundThesis(ticker, start)
	if err != nil {
		return Thesis{}, false, err
	}
	if ok {
		return Thesis{Text: text, Edited: true}, true, nil
	}

	idx := len(rounds) // PendingRound sits after every round
	if start != PendingRound {
		idx = -1
		for i, r := range rounds {
			if r.StartDate == start {
				idx = i
			}
		}
		if idx < 0 {
			return Thesis{}, false, nil
		}
	} else if CurrentRoundKey(rounds) != PendingRound {
		return Thesis{}, false, nil // a draft can't coexist with an open round
	}

	var from, to string
	if idx > 0 {
		prevEnd, err := time.Parse("2006-01-02", rounds[idx-1].EndDate)
		if err != nil {
			return Thesis{}, false, err
		}
		from = prevEnd.AddDate(0, 0, 1).Format("2006-01-02")
	}
	if idx < len(rounds) {
		to = rounds[idx].EndDate
	}
	entries, err := st.GetThesisEntriesInRange(ticker, from, to)
	if err != nil || len(entries) == 0 {
		return Thesis{}, false, err
	}
	return Thesis{Text: entries[len(entries)-1].Text}, true, nil
}

// CurrentThesis is the thesis of ticker's current round — what Telegram
// /thesis, the buy nudge, the recommendation prompt and MCP get_thesis show.
func CurrentThesis(st ThesisStore, ticker string) (string, bool, error) {
	txs, err := st.GetTransactions(ticker)
	if err != nil {
		return "", false, err
	}
	rounds := SegmentRounds(txs)
	th, ok, err := RoundThesis(st, ticker, rounds, CurrentRoundKey(rounds))
	return th.Text, ok, err
}

// SetCurrentThesis overwrites the current round's thesis.
func SetCurrentThesis(st ThesisStore, ticker, text string) error {
	txs, err := st.GetTransactions(ticker)
	if err != nil {
		return err
	}
	return st.SetRoundThesis(ticker, CurrentRoundKey(SegmentRounds(txs)), text)
}
