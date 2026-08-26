package bot

import (
	"context"
	"time"

	"argus/internal/i18n"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/service"
)

// usIndices are the ETF proxies used for the morning briefing's broad-index
// summary — there's no direct index quote in data.Provider, but SPY/QQQ/DIA/
// IWM track the S&P 500/Nasdaq/Dow/Russell 2000 closely enough for a
// narrative recap (not a precise index-point figure).
var usIndices = []service.IndexProxy{
	{Ticker: "SPY", Label: "S&P 500"},
	{Ticker: "QQQ", Label: "Nasdaq"},
	{Ticker: "DIA", Label: "Dow Jones"},
	{Ticker: "IWM", Label: "Russell 2000"},
}

// twIndices mirrors usIndices for RunTWMorningBriefing's "prior TW close"
// section — 0050/0051 ETF proxies rather than raw ^TWII/^TWOII Yahoo index
// symbols: ^TWII returns live data but ^TWOII's quote timestamp was
// live-verified stale (over a year old), so both proxies use the same
// live-verified-fresh ETF-ticker approach usIndices already relies on.
var twIndices = []service.IndexProxy{
	{Ticker: "0050", Label: "台灣50"},
	{Ticker: "0051", Label: "中型100"},
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

	indices := service.FetchIndexQuotes(b.provider, usIndices)
	var vix float64
	if q, err := b.provider.GetQuote(vixTicker); err != nil {
		logger.Errorf("morning briefing: %s quote: %v", vixTicker, err)
	} else {
		vix = q.Price
	}
	marketNews := b.loadMarketNews(market.US)

	positions := b.loadPositions()

	watchlistTickers, err := b.db.GetWatchlistByMarket(market.US)
	if err != nil {
		logger.Errorf("morning briefing: watchlist: %v", err)
	}
	watchlist := service.LoadQuoteHighlights(b.provider, b.companyNames, watchlistTickers, positions)

	moverTickers, err := b.provider.GetMarketMovers()
	if err != nil {
		logger.Errorf("morning briefing: market movers: %v", err)
	}
	movers := service.LoadQuoteHighlights(b.provider, b.companyNames, moverTickers, positions)

	result, err := b.llm.MorningBriefing(ctx, reportDate.Format("2006-01-02"), indices, vix, marketNews, watchlist, movers, false)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyLLMFailed, err))
		return
	}
	b.Send(result)
}

// twPreOpenStaleness gates RunTWMorningBriefing's market-closed check — much
// looser than twMarketClosedStaleness (12h, used by isTWMarketClosed for the
// 11:30 daily report) because at 08:30 TW pre-open, the latest 0050 close is
// already ~19h old on an ordinary trading day; isTWMarketClosed would report
// "closed" every single morning if reused here. Must clear a bare weekend
// gap (Friday 13:30 close to Monday 08:30 pre-open is 67h — 48h was live-
// verified to misfire on every single Monday) with slack to spare; 72h
// covers weekend + a single adjacent holiday. Known gap: a multi-day holiday
// (e.g. Lunar New Year) still exceeds this and correctly reports closed for
// its first post-holiday morning — that day's prior-session recap content
// would still be valid, just not same-day-fresh, so this is an accepted
// trade-off rather than an attempt to cover every holiday length.
const twPreOpenStaleness = 72 * time.Hour

// RunTWMorningBriefing is the 08:30 CST scheduler entry point (see
// scheduler.AddTWMorningBriefing): a read-only narrative recap 30 minutes
// before TW's 09:00 open — the prior TW session's close, overnight US
// performance, VIX, and TW market news — distinct from RunTWDailyReport's
// BUY/SELL/HOLD decision later at 11:30. Deliberately touches no
// signal_states/recommendations persistence and triggers no stop-loss
// alert checks, same division of labor as RunUSMorningBriefing.
func (b *Bot) RunTWMorningBriefing(ctx context.Context) {
	defer b.recoverJobPanic("tw morning briefing")

	q, err := b.provider.GetQuote(benchmarkFor(market.TW))
	if err != nil {
		logger.Errorf("tw morning briefing: %s quote: %v", benchmarkFor(market.TW), err)
		b.Send(i18n.T(b.lang, i18n.KeyTWMorningBriefingMarketClosed))
		return
	}
	if time.Since(q.Timestamp) > twPreOpenStaleness {
		b.Send(i18n.T(b.lang, i18n.KeyTWMorningBriefingMarketClosed))
		return
	}
	// reportDate is the last TW trading session, derived from the 0050
	// quote's own timestamp rather than b.now().AddDate(0,0,-1) — that
	// AddDate approach only works for RunUSMorningBriefing because its cron
	// is Tue-Sat (so "yesterday" is always the prior US session); TW's cron
	// is Mon-Fri, so on Monday "yesterday" would wrongly land on Sunday.
	reportDate := q.Timestamp.In(cst)

	b.Send(i18n.T(b.lang, i18n.KeyTWMorningBriefingStart))

	indices := append(service.FetchIndexQuotes(b.provider, twIndices), service.FetchIndexQuotes(b.provider, usIndices)...)
	var vix float64
	if q, err := b.provider.GetQuote(vixTicker); err != nil {
		logger.Errorf("tw morning briefing: %s quote: %v", vixTicker, err)
	} else {
		vix = q.Price
	}
	marketNews := b.loadMarketNews(market.TW)

	positions := b.loadPositions()

	watchlistTickers, err := b.db.GetWatchlistByMarket(market.TW)
	if err != nil {
		logger.Errorf("tw morning briefing: watchlist: %v", err)
	}
	watchlist := service.LoadQuoteHighlights(b.provider, b.companyNames, watchlistTickers, positions)

	var moverTickers []string
	if b.twMovers != nil {
		moverTickers, err = b.twMovers.GetMarketMovers()
		if err != nil {
			logger.Errorf("tw morning briefing: market movers: %v", err)
		}
	}
	movers := service.LoadQuoteHighlights(b.provider, b.companyNames, moverTickers, positions)

	result, err := b.llm.MorningBriefing(ctx, reportDate.Format("2006-01-02"), indices, vix, marketNews, watchlist, movers, true)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyLLMFailed, err))
		return
	}
	b.Send(result)
}
