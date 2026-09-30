package service

import (
	"errors"
	"testing"

	"argus/internal/db"
)

type fakeThesisStore struct {
	txs     []db.Transaction
	rows    map[string]string // round_start -> text (single ticker)
	journal []db.ThesisEntry  // CreatedAt = "YYYY-MM-DD"
}

func (f *fakeThesisStore) GetTransactions(string) ([]db.Transaction, error) { return f.txs, nil }
func (f *fakeThesisStore) GetRoundThesis(_, start string) (string, bool, error) {
	text, ok := f.rows[start]
	return text, ok, nil
}
func (f *fakeThesisStore) SetRoundThesis(_, start, text string) error {
	if f.rows == nil {
		f.rows = map[string]string{}
	}
	f.rows[start] = text
	return nil
}
func (f *fakeThesisStore) GetThesisEntriesInRange(_, from, to string) ([]db.ThesisEntry, error) {
	var out []db.ThesisEntry
	for _, e := range f.journal {
		if e.CreatedAt >= from && (to == "" || e.CreatedAt <= to) {
			out = append(out, e)
		}
	}
	return out, nil
}

// Two rounds: 2026-01-05..2026-02-02 (closed) and 2026-03-02.. (open).
func twoRoundStore() *fakeThesisStore {
	return &fakeThesisStore{txs: []db.Transaction{
		{Date: "2026-01-05", Side: "BUY", Shares: 10},
		{Date: "2026-02-02", Side: "SELL", Shares: 10},
		{Date: "2026-03-02", Side: "BUY", Shares: 10},
	}}
}

func TestRoundThesisSavedRowWins(t *testing.T) {
	st := twoRoundStore()
	st.rows = map[string]string{"2026-01-05": "saved"}
	st.journal = []db.ThesisEntry{{CreatedAt: "2026-01-20", Text: "journal"}}

	th, ok, err := RoundThesis(st, "AAPL", SegmentRounds(st.txs), "2026-01-05")
	if err != nil || !ok || th.Text != "saved" || !th.Edited {
		t.Errorf("RoundThesis() = %+v, %v, %v; want the saved row, Edited", th, ok, err)
	}
}

func TestRoundThesisJournalFallbackStaysInItsRound(t *testing.T) {
	st := twoRoundStore()
	st.journal = []db.ThesisEntry{
		{CreatedAt: "2026-01-06", Text: "early round-one thought"},
		{CreatedAt: "2026-01-20", Text: "late round-one thought"},
		{CreatedAt: "2026-02-20", Text: "written before the second buy"},
		{CreatedAt: "2026-03-10", Text: "during round two"},
	}
	rounds := SegmentRounds(st.txs)

	for start, want := range map[string]string{
		"2026-01-05": "late round-one thought", // not the later rounds' entries
		"2026-03-02": "during round two",
	} {
		th, ok, err := RoundThesis(st, "AAPL", rounds, start)
		if err != nil || !ok || th.Text != want || th.Edited {
			t.Errorf("RoundThesis(%s) = %+v, %v, %v; want %q from the journal, not Edited", start, th, ok, err, want)
		}
	}

	// With round two's own entry gone, the pre-buy note (dated after round one
	// closed) still counts for it.
	st.journal = st.journal[:3]
	if th, _, _ := RoundThesis(st, "AAPL", rounds, "2026-03-02"); th.Text != "written before the second buy" {
		t.Errorf("RoundThesis(round two) = %q; want the pre-buy journal note", th.Text)
	}
}

func TestRoundThesisPendingOnlyWhenNoOpenRound(t *testing.T) {
	st := twoRoundStore()
	st.journal = []db.ThesisEntry{{CreatedAt: "2026-03-10", Text: "x"}}
	if _, ok, _ := RoundThesis(st, "AAPL", SegmentRounds(st.txs), PendingRound); ok {
		t.Error("a pending draft must not resolve while a round is open")
	}

	st.txs = append(st.txs, db.Transaction{Date: "2026-04-01", Side: "SELL", Shares: 10}) // round two closes
	st.journal = append(st.journal, db.ThesisEntry{CreatedAt: "2026-04-10", Text: "watching again"})
	if th, ok, _ := RoundThesis(st, "AAPL", SegmentRounds(st.txs), PendingRound); !ok || th.Text != "watching again" {
		t.Errorf("pending fallback = %+v, %v; want only entries after the last round closed", th, ok)
	}
}

func TestCurrentThesisAndSetCurrentThesis(t *testing.T) {
	st := twoRoundStore()
	if _, ok, _ := CurrentThesis(st, "AAPL"); ok {
		t.Fatal("no thesis written yet, want ok=false")
	}
	if err := SetCurrentThesis(st, "AAPL", "hold"); err != nil {
		t.Fatal(err)
	}
	if st.rows["2026-03-02"] != "hold" {
		t.Errorf("rows = %v; want the write keyed by the open round's start", st.rows)
	}
	if got, ok, _ := CurrentThesis(st, "AAPL"); !ok || got != "hold" {
		t.Errorf("CurrentThesis() = %q, %v; want hold", got, ok)
	}

	// Watchlist-only ticker: the write becomes the pre-buy draft.
	flat := &fakeThesisStore{}
	if err := SetCurrentThesis(flat, "TSLA", "draft"); err != nil {
		t.Fatal(err)
	}
	if flat.rows[PendingRound] != "draft" {
		t.Errorf("rows = %v; want the draft slot", flat.rows)
	}
}

func TestThesisRoundKey(t *testing.T) {
	rounds := SegmentRounds(twoRoundStore().txs)

	if k, err := ThesisRoundKey(rounds, ""); err != nil || k != "2026-03-02" {
		t.Errorf("ThesisRoundKey(\"\") = %q, %v; want the open round", k, err)
	}
	if k, err := ThesisRoundKey(rounds, "2026-01-05"); err != nil || k != "2026-01-05" {
		t.Errorf("ThesisRoundKey(closed round) = %q, %v; want it accepted", k, err)
	}
	if _, err := ThesisRoundKey(rounds, "2025-12-31"); !errors.Is(err, ErrThesisRoundNotFound) {
		t.Errorf("ThesisRoundKey(unknown) err = %v; want ErrThesisRoundNotFound", err)
	}
	if k, _ := ThesisRoundKey(nil, ""); k != PendingRound {
		t.Errorf("ThesisRoundKey(no rounds) = %q; want the pending draft", k)
	}
}
