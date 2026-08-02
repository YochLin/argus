package bot

import (
	"context"
	"log"

	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/llm"
	"argus/internal/market"
)

// usIndices are the ETF proxies used for the morning briefing's broad-index
// summary — there's no direct index quote in data.Provider, but SPY/QQQ/DIA/
// IWM track the S&P 500/Nasdaq/Dow/Russell 2000 closely enough for a
// narrative recap (not a precise index-point figure).
var usIndices = []struct {
	Ticker string
	Label  string
}{
	{"SPY", "S&P 500"},
	{"QQQ", "Nasdaq"},
	{"DIA", "Dow Jones"},
	{"IWM", "Russell 2000"},
}

// fetchIndexQuotes fetches each of usIndices' current quotes, skipping
// (logging) any that fail rather than failing the whole briefing — same
// per-field-degrades convention as computeMarketRegime's own VIX fetch.
func (b *Bot) fetchIndexQuotes() []llm.IndexQuote {
	var out []llm.IndexQuote
	for _, idx := range usIndices {
		q, err := b.provider.GetQuote(idx.Ticker)
		if err != nil {
			log.Printf("morning briefing: %s quote: %v", idx.Ticker, err)
			continue
		}
		out = append(out, llm.IndexQuote{Label: idx.Label, Price: q.Price, ChangePercent: q.ChangePercent})
	}
	return out
}

// loadQuoteHighlights is a lighter sibling of fetchStockData for the morning
// briefing's watchlist/mover sections: quote + a few news items + company
// name + position (if held), deliberately skipping computeTechnicals'
// GetHistory("1y") call — a narrative recap doesn't need RSI/MACD/candles,
// and fetching full technicals for both the watchlist and ~20 mover tickers
// every morning would double the daily history-fetch volume for no benefit
// here. writeStockSection renders the result safely regardless (see
// StockData's per-field degradation convention).
func (b *Bot) loadQuoteHighlights(tickers []string, positions map[string]db.Position) []llm.StockData {
	var result []llm.StockData
	for _, t := range tickers {
		q, err := b.provider.GetQuote(t)
		if err != nil {
			log.Printf("morning briefing: quote %s: %v", t, err)
			continue
		}
		news, _ := b.provider.GetNews(t, 3)
		stock := llm.StockData{Quote: q, News: news, CompanyName: b.companyName(t)}
		if p, ok := positions[t]; ok {
			stock.Position = &llm.Position{Shares: p.Shares, AvgCost: p.AvgCost}
		}
		result = append(result, stock)
	}
	return result
}

// RunUSMorningBriefing is the 07:00 CST scheduler entry point (see
// scheduler.AddMorningBriefing): a read-only narrative recap of the US
// session that just closed — indices, macro news, watchlist/mover
// highlights — distinct from runDailyReport's BUY/SELL/HOLD decision later
// the same evening. Deliberately touches no signal_states/recommendations
// persistence and triggers no stop-loss/trailing-stop/target/MA5-break
// alert checks; those all stay owned by runDailyReport.
func (b *Bot) RunUSMorningBriefing(ctx context.Context) {
	defer b.recoverJobPanic("morning briefing")

	reportDate := b.now().In(cst).AddDate(0, 0, -1)
	if !market.IsTradingDay(reportDate) {
		b.Send(i18n.T(b.lang, i18n.KeyMorningBriefingMarketClosed))
		return
	}

	b.Send(i18n.T(b.lang, i18n.KeyMorningBriefingStart))

	indices := b.fetchIndexQuotes()
	var vix float64
	if q, err := b.provider.GetQuote(vixTicker); err != nil {
		log.Printf("morning briefing: %s quote: %v", vixTicker, err)
	} else {
		vix = q.Price
	}
	marketNews := b.loadMarketNews(market.US)

	positions := b.loadPositions()

	watchlistTickers, err := b.db.GetWatchlistByMarket(market.US)
	if err != nil {
		log.Printf("morning briefing: watchlist: %v", err)
	}
	watchlist := b.loadQuoteHighlights(watchlistTickers, positions)

	moverTickers, err := b.provider.GetMarketMovers()
	if err != nil {
		log.Printf("morning briefing: market movers: %v", err)
	}
	movers := b.loadQuoteHighlights(moverTickers, positions)

	result, err := b.llm.MorningBriefing(ctx, reportDate.Format("2006-01-02"), indices, vix, marketNews, watchlist, movers)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyLLMFailed, err))
		return
	}
	b.Send(result)
}
