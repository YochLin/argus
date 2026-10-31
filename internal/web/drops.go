package web

import (
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/service"
	"argus/internal/signals"
)

// dropResponse is one signals.DetectDrops hit, dates instead of candle
// indices. The numbers are computed per request from the candles, so any
// ticker has its whole chart window. Cause down to News come from drop_causes,
// recorded only for a watchlist ticker from the day this shipped: a past day's
// news can't be fetched (data.Provider.GetNews takes no date). Summary falls
// back to the Telegram price event's writeup for a day inside the drop, and
// is "" when there is neither.
type dropResponse struct {
	Kind        string   `json:"kind"` // "streak"/"gap"/"day"
	Start       string   `json:"start"`
	End         string   `json:"end"`
	Pct         float64  `json:"pct"`
	GapPct      *float64 `json:"gapPct"`
	PreClose    float64  `json:"preClose"`
	Low         float64  `json:"low"`
	RecoverBars *int     `json:"recoverBars"`
	Fwd10Pct    *float64 `json:"fwd10Pct"`
	IndexPct    *float64 `json:"indexPct"` // the market index over the same span; null if it has no bars for it
	Summary     string   `json:"summary"`

	Cause      string             `json:"cause"` // "" | "earn" | "macro" | "sector" | "flow" | "corp" | "tech"
	Secondary  []string           `json:"secondary"`
	Confidence string             `json:"confidence"` // "" | "high" | "mid"
	LoggedOn   string             `json:"loggedOn"`   // the bar the cause was recorded on
	News       []dropNewsResponse `json:"news"`
}

type dropNewsResponse struct {
	Headline    string `json:"headline"`
	Source      string `json:"source"`
	URL         string `json:"url"`
	PublishedAt string `json:"publishedAt"`
}

// buildDrops assembles /api/chart's drops and the index their IndexPct is
// measured on. A bar still in session is left out: the events say "after the
// close", and a part-day close could make one that's gone by the end of the day.
func buildDrops(database dbReader, history data.HistoryProvider, ticker string, candles []data.Candle) ([]dropResponse, string) {
	if n := len(candles); n > 0 && service.BarForming(candles, n-1, ticker, time.Now()) {
		candles = candles[:n-1]
	}
	drops := signals.DetectDrops(candles)
	out := make([]dropResponse, 0, len(drops))
	if len(drops) == 0 {
		return out, service.DropIndexFor(market.Of(ticker))
	}

	index, indexCloses := service.DropIndexCloses(history, ticker)
	summaries := priceEventSummaries(database, ticker)
	causes := dropCauses(database, ticker)
	for _, d := range drops {
		r := dropResponse{
			Kind:        d.Kind,
			Start:       candles[d.Start].Date.Format("2006-01-02"),
			End:         candles[d.End].Date.Format("2006-01-02"),
			Pct:         d.Pct,
			GapPct:      d.GapPct,
			PreClose:    d.PreClose,
			Low:         d.Low,
			RecoverBars: d.RecoverBars,
			Fwd10Pct:    d.Fwd10Pct,
			IndexPct:    service.DropIndexPct(indexCloses, candles, d),
			Secondary:   []string{},
			News:        []dropNewsResponse{},
		}
		// By range, not equal start: a one-day drop can turn out to be the
		// second bar of a longer run.
		for _, c := range causes {
			if c.StartDate >= r.Start && c.StartDate <= r.End {
				r.attach(c)
				break
			}
		}
		// Latest day first, so a streak reads its last writeup.
		for i := d.End; i >= d.Start && r.Summary == ""; i-- {
			r.Summary = summaries[candles[i].Date.Format("2006-01-02")]
		}
		out = append(out, r)
	}
	return out, index
}

func (r *dropResponse) attach(c db.DropCause) {
	r.Cause, r.Confidence, r.Summary, r.LoggedOn = c.Cause, c.Confidence, c.Summary, c.EndDate
	if c.Secondary != nil {
		r.Secondary = c.Secondary
	}
	for _, n := range c.News {
		r.News = append(r.News, dropNewsResponse(n))
	}
}

func dropCauses(database dbReader, ticker string) []db.DropCause {
	if database == nil {
		return nil
	}
	causes, err := database.DropCausesFor(ticker)
	if err != nil {
		logger.Errorf("web: drops for %s: causes: %v", ticker, err)
		return nil
	}
	return causes
}

func priceEventSummaries(database dbReader, ticker string) map[string]string {
	if database == nil {
		return nil
	}
	events, err := database.GetPriceEventsForTicker(ticker, eventsListLimit)
	if err != nil {
		logger.Errorf("web: drops for %s: price events: %v", ticker, err)
		return nil
	}
	out := make(map[string]string, len(events))
	for _, ev := range events {
		out[ev.Date] = ev.Summary
	}
	return out
}
