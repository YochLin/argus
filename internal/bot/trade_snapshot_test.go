package bot

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/service"
)

type errHistory struct{}

func (errHistory) GetHistory(string, string) ([]data.Candle, error) { return nil, errors.New("down") }

// snapCandles is n daily bars ending yesterday, close 100, 101, ... — so every
// bar is "in the past" and a fill on any of them is not today's.
func snapCandles(n int) []data.Candle {
	end := time.Now().AddDate(0, 0, -1)
	out := make([]data.Candle, n)
	for i := range out {
		c := 100 + float64(i)
		out[i] = data.Candle{Date: end.AddDate(0, 0, i-n+1), Open: c, High: c + 1, Low: c - 1, Close: c, Volume: 1000}
	}
	return out
}

func TestReviewSnapshots(t *testing.T) {
	d, err := db.New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })

	cs := snapCandles(200)
	day := func(i int) string { return cs[i].Date.Format("2006-01-02") }
	entry, exit := day(120), day(160)
	if err := d.SaveFillNews("AAPL", entry, []db.FillNews{{Headline: "AAPL beats", Source: "Reuters", URL: "https://x/1", Sentiment: "bull", Tag: "earn", Major: true}}); err != nil {
		t.Fatal(err)
	}

	b := &Bot{db: d, lang: i18n.EN, history: rankHistoryStub{byTicker: map[string][]data.Candle{"AAPL": cs}}}
	round := tradeRound{
		StartDate: entry, EndDate: exit,
		Legs: []db.Transaction{
			{Ticker: "AAPL", Side: "BUY", Shares: 10, Price: 220, Date: entry},
			{Ticker: "AAPL", Side: "SELL", Shares: 10, Price: 260, Date: exit},
		},
	}

	got := b.reviewSnapshots("AAPL", round)
	if len(got) != 2 || got[0].Side != "BUY" || got[1].Side != "SELL" {
		t.Fatalf("snapshots = %+v, want entry then exit", got)
	}
	if got[0].Date != entry || got[0].Price != 220 {
		t.Errorf("entry = %+v", got[0])
	}

	// The numbers are the chart tab's own (service.SnapshotAt at that bar).
	want := service.SnapshotAt(cs, 120)
	if got[0].RSI != want.RSI14 || got[0].Trend != want.Trend || got[0].VolRatio20 != want.VolRatio20 || got[0].MACDDays != want.MACDCrossDays {
		t.Errorf("entry readings %+v differ from SnapshotAt %+v", got[0], want)
	}

	// Headlines come along for the day that has them, not for the one that doesn't.
	if len(got[0].News) != 1 || got[0].News[0].Headline != "AAPL beats" || got[0].News[0].Source != "Reuters" {
		t.Errorf("entry news = %+v", got[0].News)
	}
	if len(got[1].News) != 0 {
		t.Errorf("exit news = %+v, want none stored", got[1].News)
	}

	// Nothing after the exit day leaks in: the later bars changing must not move
	// the exit snapshot.
	wild := append([]data.Candle(nil), cs...)
	for j := 161; j < len(wild); j++ {
		wild[j].Close, wild[j].High, wild[j].Low, wild[j].Volume = 1, 1e6, 0.01, 1e9
	}
	b.history = rankHistoryStub{byTicker: map[string][]data.Candle{"AAPL": wild}}
	if again := b.reviewSnapshots("AAPL", round); !reflect.DeepEqual(again[1], got[1]) {
		t.Errorf("the exit snapshot moved when only later bars changed:\n%+v\n%+v", again[1], got[1])
	}
}

func TestReviewSnapshotsDegrade(t *testing.T) {
	d, err := db.New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	cs := snapCandles(200)
	entry := cs[120].Date.Format("2006-01-02")
	round := tradeRound{StartDate: entry, EndDate: entry, Legs: []db.Transaction{{Ticker: "AAPL", Side: "BUY", Price: 220, Date: entry}}}

	if got := (&Bot{db: d}).reviewSnapshots("AAPL", round); got != nil {
		t.Errorf("no history provider: %+v", got)
	}
	if got := (&Bot{db: d, history: errHistory{}}).reviewSnapshots("AAPL", round); got != nil {
		t.Errorf("history error: %+v", got)
	}
	// A fill too early for 60 bars of history has no snapshot, but the others do.
	short := rankHistoryStub{byTicker: map[string][]data.Candle{"AAPL": cs[:30]}}
	if got := (&Bot{db: d, history: short}).reviewSnapshots("AAPL", round); len(got) != 0 {
		t.Errorf("too little history: %+v", got)
	}
	// A single-leg round yields one snapshot, not the same leg twice.
	ok := rankHistoryStub{byTicker: map[string][]data.Candle{"AAPL": cs}}
	if got := (&Bot{db: d, history: ok}).reviewSnapshots("AAPL", round); len(got) != 1 {
		t.Errorf("single leg: %+v", got)
	}
}

// buildClosedTradeReview is the one place every review path (at close, the
// 5-day follow-up, /review) gets its input, so the snapshots must arrive here.
func TestBuildClosedTradeReviewCarriesSnapshots(t *testing.T) {
	b, d := newPendingActionsTestBot(t)
	cs := snapCandles(200)
	entry, exit := cs[120].Date.Format("2006-01-02"), cs[160].Date.Format("2006-01-02")
	if _, err := d.RecordBuy("AAPL", 10, 220, 1, entry); err != nil {
		t.Fatal(err)
	}
	if _, _, err := d.RecordSell("AAPL", 10, 260, 1, exit); err != nil {
		t.Fatal(err)
	}
	b.history = rankHistoryStub{byTicker: map[string][]data.Candle{"AAPL": cs}}

	trade, ok, err := b.buildClosedTradeReview("AAPL", 0)
	if err != nil || !ok {
		t.Fatalf("buildClosedTradeReview() = %v, %v", ok, err)
	}
	if len(trade.Snapshots) != 2 || trade.Snapshots[0].Date != entry || trade.Snapshots[1].Date != exit {
		t.Fatalf("snapshots = %+v, want the entry and exit days", trade.Snapshots)
	}
	if trade.Followup != nil {
		t.Error("the at-close review must not carry post-exit data")
	}
}
