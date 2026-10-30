package bot

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/llm"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/service"
	"argus/internal/signals"
)

const (
	// dropCauseCap caps the LLM calls of one closing run, deepest drop first.
	// Measured on the 2026-10 watchlist over two years: about 1.5 drops a
	// session per market, more than 5 on 7% of sessions, up to 19. Those are
	// market-wide days, where the causes repeat, so the tail costs little.
	dropCauseCap = 5
	// dropCauseHistory reaches back past any run of lower closes, plus the
	// 60 bars service.DropTechBreaks looks at before it.
	dropCauseHistory = "6mo"
)

// recordDropCauses keeps why a watchlist ticker's drop happened while that can
// still be found: GetNews has no date parameter, so this is the only chance,
// and every drop keeps its headlines even past dropCauseCap. It rides the tail
// of RunClosingSnapshot because the LLM calls are slow, and it pushes nothing:
// this is for the chart's 下跌事件 tab, and the rule fires far more often than
// the price events that do go to Telegram. Best-effort, one row per drop: a
// failed LLM call leaves the headlines without a cause and isn't retried.
func (b *Bot) recordDropCauses(ctx context.Context, tickers []string, m market.MarketID, date string) {
	if b.history == nil {
		return
	}
	type hit struct {
		ticker  string
		candles []data.Candle
		drop    signals.DropEvent
	}
	var hits []hit
	for _, ticker := range tickers {
		candles, err := b.history.GetHistory(ticker, dropCauseHistory)
		if err != nil {
			logger.Errorf("drop causes %s: history: %v", ticker, err)
			continue
		}
		d := service.DropOn(candles, date)
		if d == nil {
			continue
		}
		seen, err := b.dropRecorded(ticker, candles, *d)
		if err != nil {
			logger.Errorf("drop causes %s: lookup: %v", ticker, err)
			continue
		}
		if !seen {
			hits = append(hits, hit{ticker, candles, *d})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].drop.Pct < hits[j].drop.Pct })

	attributed := 0
	for _, h := range hits {
		news, err := b.provider.GetNews(h.ticker, service.EventNewsFetch)
		if err != nil {
			logger.Errorf("drop causes %s: news: %v", h.ticker, err)
			continue
		}
		news = (&service.NewsPicker{}).Pick(service.FilterNewsNearDate(news, date), service.EventNewsSlots)
		if len(news) == 0 {
			continue // a streak's next bar can still find some
		}

		row := db.DropCause{
			Ticker: h.ticker, Market: string(m), EndDate: date,
			StartDate: h.candles[h.drop.Start].Date.Format("2006-01-02"),
			News:      dropNews(news),
		}
		if attributed < dropCauseCap && b.llm != nil {
			attributed++
			a := b.attributeDrop(ctx, h.ticker, h.candles, h.drop, m, news)
			row.Cause, row.Secondary, row.Confidence, row.Summary = a.Cause, a.Secondary, a.Confidence, a.Summary
		}
		if err := b.db.SaveDropCause(row); err != nil {
			logger.Errorf("drop causes %s: save: %v", h.ticker, err)
		}
	}
}

// dropRecorded reports a row already inside d. Its start can differ from d's:
// a gap recorded on one day can turn out to be part of a longer run.
func (b *Bot) dropRecorded(ticker string, candles []data.Candle, d signals.DropEvent) (bool, error) {
	rows, err := b.db.DropCausesFor(ticker)
	if err != nil {
		return false, err
	}
	from, to := candles[d.Start].Date.Format("2006-01-02"), candles[d.End].Date.Format("2006-01-02")
	for _, r := range rows {
		if r.StartDate >= from && r.StartDate <= to {
			return true, nil
		}
	}
	return false, nil
}

func (b *Bot) attributeDrop(ctx context.Context, ticker string, candles []data.Candle, d signals.DropEvent, m market.MarketID, news []data.NewsItem) llm.DropAttribution {
	index, closes := service.DropIndexCloses(b.history, ticker)
	facts := llm.DropFacts{
		Ticker: ticker, Bars: d.End - d.Start + 1, Pct: d.Pct, GapPct: d.GapPct,
		IndexPct: service.DropIndexPct(closes, candles, d), Benchmark: index,
	}
	facts.BrokeMA60, facts.BrokeLow = service.DropTechBreaks(candles, d)
	a, raw, model, latencyMs, err := b.llm.AttributeDrop(ctx, facts, news)
	b.recordDropCauseLLMRun(facts, m, news, raw, model, latencyMs)
	if err != nil {
		logger.Errorf("drop causes %s: LLM: %v", ticker, err)
	}
	return a
}

// recordDropCauseLLMRun files one AttributeDrop call into llm_runs, with the
// fields recordPriceEventLLMRun's payload uses so the audit page reads both.
// The whole drop goes in as cumulativePct: a drop has no single day's change.
func (b *Bot) recordDropCauseLLMRun(f llm.DropFacts, m market.MarketID, news []data.NewsItem, raw, model string, latencyMs int64) {
	if raw == "" && model == "" {
		return
	}
	payload := struct {
		Ticker        string          `json:"ticker"`
		GapPct        *float64        `json:"gapPct,omitempty"`
		CumulativePct float64         `json:"cumulativePct"`
		News          []data.NewsItem `json:"news"`
	}{f.Ticker, f.GapPct, f.Pct, news}
	inputJSON, err := json.Marshal(payload)
	if err != nil {
		logger.Errorf("llm run: marshal drop cause input: %v", err)
		return
	}
	if err := b.db.InsertLLMRun("drop_cause", m, model, latencyMs, string(inputJSON), raw, 0, 0, len(news), 0); err != nil {
		logger.Errorf("llm run: insert drop cause: %v", err)
	}
}

func dropNews(news []data.NewsItem) []db.DropNews {
	out := make([]db.DropNews, len(news))
	for i, n := range news {
		out[i] = db.DropNews{Headline: n.Headline, Source: n.Source, URL: n.URL}
		if !n.PublishedAt.IsZero() {
			out[i].PublishedAt = n.PublishedAt.UTC().Format(time.RFC3339)
		}
	}
	return out
}
