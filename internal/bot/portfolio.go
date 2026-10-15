package bot

import (
	"context"
	"errors"
	"math"
	"strings"

	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/service"
)

// twLotSize is one 台股 board lot (張) — 1,000 shares, the unit Taiwanese
// investors actually think in even though this project records TW positions
// in raw shares like US ones (§3 of docs/phase-6-tw-market.md: "單位一律
// 股").
const twLotSize = 1000

// handlePortfolio shows every open position's current market value and
// unrealized P&L against a live quote, plus cumulative realized P&L across
// all past sells. Each position goes out as its own message (same reasoning
// as sendAndSaveRecommendations — see that doc comment) so its
// [Check]/[Buy]/[Sell] quick-action row (quick_actions.go) attaches to that
// ticker specifically.
func (b *Bot) handlePortfolio() {
	portfolio := b.portfolios()
	if portfolio == nil {
		b.Send(i18n.T(b.lang, i18n.KeyQueryFailed, errors.New("portfolio service unavailable")))
		return
	}
	usSnapshot, err := portfolio.Snapshot(market.US)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyQueryFailed, err))
		return
	}
	twSnapshot, err := portfolio.Snapshot(market.TW)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyQueryFailed, err))
		return
	}
	optionPositions, err := b.db.GetOptionPositions()
	if err != nil {
		logger.Errorf("portfolio: option positions: %v", err)
	}
	if len(usSnapshot.Positions) == 0 && len(twSnapshot.Positions) == 0 && len(optionPositions) == 0 {
		b.Send(i18n.T(b.lang, i18n.KeyPortfolioEmpty))
		return
	}

	b.Send(i18n.T(b.lang, i18n.KeyPortfolioTitle))
	b.sendPortfolioSection(usSnapshot)
	b.sendPortfolioSection(twSnapshot)
	b.sendPortfolioOptionsSection(optionPositions)
}

// sendPortfolioSection renders /portfolio's per-market block (Phase 6, see
// docs/phase-6-tw-market.md §4.3): a section title tagged with the market's
// currency, each of that market's positions, and a market-scoped subtotal
// (GetRealizedPnL(m) — realized P&L never sums across markets). Sends
// nothing at all — not even the section title — when m has no open
// positions, so a single-market portfolio doesn't show a dangling empty
// block for the other market.
func (b *Bot) sendPortfolioSection(snapshot service.PortfolioSnapshot) {
	if len(snapshot.Positions) == 0 {
		return
	}

	m := snapshot.Market
	if snapshot.RealizedPnLErr != nil {
		logger.Errorf("portfolio: realized pnl (%s): %v", m, snapshot.RealizedPnLErr)
	}

	b.Send(i18n.T(b.lang, portfolioSectionTitleKey(m)))
	for _, valuation := range snapshot.Positions {
		p := valuation.Position
		if valuation.QuoteErr != nil || valuation.Quote == nil {
			if valuation.QuoteErr != nil {
				logger.Errorf("portfolio: quote %s: %v", p.Ticker, valuation.QuoteErr)
			}
			b.sendWithTickerActions(p.Ticker, i18n.T(b.lang, i18n.KeyQuoteUnavailable, b.tickerLabel(p.Ticker)))
			continue
		}
		line := i18n.T(b.lang, i18n.KeyPortfolioLine, b.tickerLabel(p.Ticker), p.Shares, b.money(p.Ticker, p.AvgCost), b.money(p.Ticker, valuation.Quote.Price), b.money(p.Ticker, valuation.MarketValue), valuation.UnrealizedPnL, valuation.UnrealizedPnLPct)
		if note := lotSuffix(b.lang, m, p.Shares); note != "" {
			// KeyPortfolioLine ends "...%%)\n\n" — splice the note onto that
			// last content line rather than appending after the trailing
			// blank line, so it reads as part of the same position summary.
			line = strings.TrimSuffix(line, "\n\n") + note + "\n\n"
		}
		b.sendWithTickerActions(p.Ticker, line)
	}
	if snapshot.CashErr != nil {
		logger.Errorf("portfolio: load cash (%s): %v", m, snapshot.CashErr)
	}
	cash, haveCash := snapshot.Cash, snapshot.HasCash
	realizedTotal := snapshot.RealizedPnL
	totalValue := snapshot.TotalMarketValue

	if m == market.TW {
		if haveCash {
			b.Send(i18n.T(b.lang, i18n.KeyPortfolioSummaryWithCashTWD, totalValue, realizedTotal, totalValue+cash, cash))
		} else {
			b.Send(i18n.T(b.lang, i18n.KeyPortfolioSummaryTWD, totalValue, realizedTotal))
		}
		return
	}
	if haveCash {
		b.Send(i18n.T(b.lang, i18n.KeyPortfolioSummaryWithCash, totalValue, realizedTotal, totalValue+cash, cash))
	} else {
		b.Send(i18n.T(b.lang, i18n.KeyPortfolioSummary, totalValue, realizedTotal))
	}
}

// portfolioSectionTitleKey selects /portfolio's per-market section title.
func portfolioSectionTitleKey(m market.MarketID) i18n.Key {
	if m == market.TW {
		return i18n.KeyPortfolioSectionTW
	}
	return i18n.KeyPortfolioSectionUS
}

// lotSuffix appends a "(= N 張)" note to a TW portfolio line when shares is
// an exact multiple of a board lot (1,000) — a non-round share count (e.g.
// a partial fill) simply gets no note, same as any other "not enough data"
// omission in this codebase. Always "" for US, where board lots aren't a
// concept.
func lotSuffix(lang i18n.Lang, m market.MarketID, shares float64) string {
	if m != market.TW || shares < twLotSize {
		return ""
	}
	lots := shares / twLotSize
	if lots != math.Trunc(lots) {
		return ""
	}
	return i18n.T(lang, i18n.KeyPortfolioLotSuffix, int(lots))
}

// handleInsight is Phase 3.6's portfolio-level analysis command: unlike
// /recommend (a per-candidate scanning view) or /check (a single ticker),
// this steps back to look at the whole set of holdings together —
// concentration risk, whether each position's original thesis still holds,
// add/reduce/stop-loss suggestions. Reuses fetchStockData/loadEarnings
// exactly like /recommend and RunDailyReport do (same attach-and-render
// StockData fields: technicals, fundamentals, earnings, cost basis) rather
// than building a separate data-gathering path.
//
// One message per market, each from its own LLM call over only that market's
// holdings and cash — same reasoning as RunWeeklyReview's split: a combined
// call weighed TWD position sizes against USD ones and summed both into one
// "total assets" number.
func (b *Bot) handleInsight(ctx context.Context) {
	positions, err := b.db.GetPositions()
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyQueryFailed, err))
		return
	}
	if len(positions) == 0 {
		b.Send(i18n.T(b.lang, i18n.KeyPortfolioEmpty))
		return
	}

	b.Send(i18n.T(b.lang, i18n.KeyAnalyzing))

	for _, m := range []market.MarketID{market.US, market.TW} {
		b.insightMarket(ctx, m, positions)
	}
}

// insightMarket is handleInsight's per-market half: one LLM call and one
// message covering market m only, silently skipped when m holds nothing.
func (b *Bot) insightMarket(ctx context.Context, m market.MarketID, positions []db.Position) {
	stocks, _ := b.portfolioStocks(m, positions)
	if len(stocks) == 0 {
		return
	}

	cash, haveCash, err := b.loadCash(m)
	if err != nil {
		logger.Errorf("insight: load cash (%s): %v", m, err)
	}

	result, err := b.llm.InsightPortfolio(ctx, stocks, cash, haveCash, m == market.TW)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyLLMFailed, err))
		return
	}

	titleKey := i18n.KeyInsightResultTitleUS
	if m == market.TW {
		titleKey = i18n.KeyInsightResultTitleTW
	}
	b.Send(i18n.T(b.lang, titleKey, result))
}
