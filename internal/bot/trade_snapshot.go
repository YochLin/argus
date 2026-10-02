package bot

import (
	"time"

	"argus/internal/db"
	"argus/internal/llm"
	"argus/internal/logger"
	"argus/internal/service"
)

// Phase 27 P7b — the fill-day picture a trade review reads. For a closed round
// it is the entry fill's day and the exit fill's day, each as
// service.SnapshotAt sees it: the indicator readings at that day's close, never
// anything after (what happened since the exit is the follow-up review's own
// llm.TradeFollowup block), plus the headlines captureFillNews stored for that
// day. Every failure — no history provider, the fetch failing, a fill with no
// usable bar, no stored news — just leaves that part out: a review without a
// snapshot is the review the bot sent before P7b.

// reviewSnapshots returns the entry and exit snapshots of round, oldest first.
func (b *Bot) reviewSnapshots(ticker string, round tradeRound) []llm.TradeSnapshot {
	if b.history == nil || len(round.Legs) == 0 {
		return nil
	}
	candles, err := b.history.GetHistory(ticker, service.FillHistoryRange(round.StartDate, time.Now()))
	if err != nil {
		logger.Errorf("review %s: snapshot history: %v", ticker, err)
		return nil
	}

	// The round opens with a BUY and a closed one ends with a SELL.
	legs := []db.Transaction{round.Legs[0]}
	if n := len(round.Legs); n > 1 {
		legs = append(legs, round.Legs[n-1])
	}
	var out []llm.TradeSnapshot
	for _, leg := range legs {
		i, ok := service.FillBar(candles, leg.Date)
		if !ok {
			continue
		}
		s := service.SnapshotAt(candles, i)
		// A fill from today may still be racing captureFillNews (the review
		// starts right behind it); storing is idempotent, so make sure its
		// headlines are in before reading them.
		if leg.Date == todayDate() {
			b.storeFillNews(ticker, leg.Date)
		}
		snap := llm.TradeSnapshot{
			Side: leg.Side, Date: leg.Date, Price: leg.Price,
			RSI: s.RSI14, RSIPrev5: s.RSI14Prev5,
			MACDHist: s.MACDH, MACDDays: s.MACDCrossDays,
			Trend: s.Trend, CloseVsMA20Pct: s.CloseVsMA20Pct, MA20Slope5dPct: s.MA20Slope5dPct,
			VolRatio20: s.VolRatio20,
			// A fill from today's session still running: its volume is a part day.
			Intraday: service.BarForming(candles, i, ticker, time.Now()),
		}
		if news, err := b.db.FillNewsFor(ticker, leg.Date); err != nil {
			logger.Errorf("review %s: snapshot news %s: %v", ticker, leg.Date, err)
		} else {
			for _, n := range news {
				snap.News = append(snap.News, llm.TradeSnapshotNews{Source: n.Source, Headline: n.Headline})
			}
		}
		out = append(out, snap)
	}
	return out
}
