package web

import (
	"net/http"
	"strconv"
	"time"

	"argus/internal/data"
	"argus/internal/logger"
	"argus/internal/service"
)

// Phase 27 P6 — /api/fill-snapshot: what the stock looked like on the day a
// trade was filled. Indicators, patterns and the after-the-fact move are
// computed here from candles on every request (service.SnapshotAt only sees bars
// up to the fill day); the one thing that can't be recomputed — the news around
// that day — was stored when the fill was recorded (fill_news, Phase 27 P7a).
// Numbers and codes only: names, labels and wording are the frontend's.

// fillResponse is one of a ticker's fills in /api/chart: the transaction plus
// the round it belongs to, so the snapshot tab can number it without guessing
// from dates (a sell and the next round's buy can share a day).
type fillResponse struct {
	transactionResponse
	RoundStart string `json:"roundStart"`
}

type fillPatternResponse struct {
	Type string `json:"type"`
	Dir  string `json:"dir"`
}

// fillSnapshotBody is service.FillSnapshot on the wire.
type fillSnapshotBody struct {
	Date           string                `json:"date"` // the candle the fill maps to
	Close          float64               `json:"close"`
	High           float64               `json:"high"`
	Low            float64               `json:"low"`
	DayChangePct   float64               `json:"dayChangePct"`
	RSI14          float64               `json:"rsi14"`
	RSI14Prev5     float64               `json:"rsi14Prev5"`
	RSIZone        string                `json:"rsiZone"` // hot | cold | mid
	MACDDif        float64               `json:"macdDif"`
	MACDDea        float64               `json:"macdDea"`
	MACDHist       float64               `json:"macdHist"`
	MACDCrossDays  int                   `json:"macdCrossDays"`
	MACDWidening   bool                  `json:"macdWidening"`
	Trend          string                `json:"trend"` // bull | bear | range
	CloseVsMA20Pct float64               `json:"closeVsMa20Pct"`
	MA20Slope5dPct float64               `json:"ma20Slope5dPct"`
	VolRatio20     float64               `json:"volRatio20"`
	VolRatio5v20   float64               `json:"volRatio5v20"`
	VolState       string                `json:"volState"` // up | down | flat
	Patterns       []fillPatternResponse `json:"patterns"`
	// Provisional is true when the bar is a session still in progress
	// (service.BarForming): the price-based readings are as of now, and the
	// volume ratios are of a day not yet traded out, so the page must not show
	// them as the close's.
	Provisional bool `json:"provisional"`
}

// fillHindsightBody holds price moves from the fill price; null where the
// candles don't reach that far yet.
type fillHindsightBody struct {
	Fwd5Pct    *float64 `json:"fwd5Pct"`
	Fwd20Pct   *float64 `json:"fwd20Pct"`
	MaxUpPct   *float64 `json:"maxUpPct"`
	MaxDownPct *float64 `json:"maxDownPct"`
}

type fillNewsResponse struct {
	Headline    string `json:"headline"`
	Source      string `json:"source"`
	URL         string `json:"url"`
	PublishedAt string `json:"publishedAt"` // RFC3339, "" when the source gave none
	Sentiment   string `json:"sentiment"`   // bull | bear | neutral | "" (not labelled)
	Tag         string `json:"tag"`         // earn | guide | analyst | sector | macro | flow | other | ""
	Major       bool   `json:"major"`
}

// fillSnapshotResponse: Snapshot is null when there is no usable bar for the
// fill (history unavailable, the fill is older than what the provider keeps, or
// too little history before it) — the page then says "no data" instead of
// failing. News is independent of that.
type fillSnapshotResponse struct {
	Ticker    string             `json:"ticker"`
	Date      string             `json:"date"`
	Snapshot  *fillSnapshotBody  `json:"snapshot"`
	Hindsight *fillHindsightBody `json:"hindsight"`
	News      []fillNewsResponse `json:"news"`
}

func buildFillSnapshot(database dbReader, history data.HistoryProvider, ticker, date string, price float64, now time.Time) fillSnapshotResponse {
	resp := fillSnapshotResponse{Ticker: ticker, Date: date, News: []fillNewsResponse{}}

	candles, err := history.GetHistory(ticker, service.FillHistoryRange(date, now))
	if err != nil {
		logger.Errorf("web: fill snapshot for %s %s: history unavailable: %v", ticker, date, err)
	} else if i, ok := service.FillBar(candles, date); ok {
		s := service.SnapshotAt(candles, i)
		resp.Snapshot = &fillSnapshotBody{
			Date: s.Date, Close: s.Close, High: s.High, Low: s.Low, DayChangePct: s.DayChangePct,
			RSI14: s.RSI14, RSI14Prev5: s.RSI14Prev5, RSIZone: s.RSIZone,
			MACDDif: s.MACDDif, MACDDea: s.MACDDea, MACDHist: s.MACDH,
			MACDCrossDays: s.MACDCrossDays, MACDWidening: s.MACDWidening,
			Trend: s.Trend, CloseVsMA20Pct: s.CloseVsMA20Pct, MA20Slope5dPct: s.MA20Slope5dPct,
			VolRatio20: s.VolRatio20, VolRatio5v20: s.VolRatio5v20, VolState: s.VolState,
			Patterns:    make([]fillPatternResponse, 0, len(s.Patterns)),
			Provisional: service.BarForming(candles, i, ticker, now),
		}
		for _, p := range s.Patterns {
			resp.Snapshot.Patterns = append(resp.Snapshot.Patterns, fillPatternResponse{Type: p.Type, Dir: p.Dir})
		}
		h := service.HindsightAfter(candles, i, price)
		resp.Hindsight = &fillHindsightBody{Fwd5Pct: h.Fwd5Pct, Fwd20Pct: h.Fwd20Pct, MaxUpPct: h.MaxUpPct, MaxDownPct: h.MaxDownPct}
	}

	if database != nil {
		rows, err := database.FillNewsFor(ticker, date)
		if err != nil {
			logger.Errorf("web: fill snapshot for %s %s: news: %v", ticker, date, err)
		}
		for _, n := range rows {
			item := fillNewsResponse{Headline: n.Headline, Source: n.Source, URL: n.URL, Sentiment: n.Sentiment, Tag: n.Tag, Major: n.Major}
			if !n.PublishedAt.IsZero() {
				item.PublishedAt = n.PublishedAt.UTC().Format(time.RFC3339)
			}
			resp.News = append(resp.News, item)
		}
	}
	return resp
}

func (s *Server) handleFillSnapshot(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if p := recover(); p != nil {
			logger.Errorf("web: panic in handleFillSnapshot: %v", p)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
	}()

	q := r.URL.Query()
	ticker, date := q.Get("ticker"), q.Get("date")
	if ticker == "" || date == "" {
		writeError(w, http.StatusBadRequest, "ticker and date are required")
		return
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		writeError(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	// price is only the reference for the after-the-fact moves; without a
	// usable one they simply come back null.
	price, _ := strconv.ParseFloat(q.Get("price"), 64)
	writeJSON(w, http.StatusOK, buildFillSnapshot(s.db, s.history, ticker, date, price, time.Now()))
}
