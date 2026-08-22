package bot

import (
	"strings"

	"argus/internal/i18n"
	"argus/internal/llm"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/paper"
	"argus/internal/service"
)

// paperInitialCashFor returns m's first-run cash seed (PAPER_INITIAL_CASH_USD/
// PAPER_INITIAL_CASH_TWD) — only ever consulted the first time loadPaperAccount
// runs for m (see loadOrSeedPaperCash); every later load reads the persisted
// settings value instead.
func (b *Bot) paperInitialCashFor(m market.MarketID) float64 {
	return b.papers().InitialCashFor(m)
}

// paperConfig builds m's paper.Config from the bot's own exit-discipline
// thresholds — reused verbatim rather than duplicated, so the paper account
// trades by the same stop/trailing-stop rules the real dashboard alerts on
// (see docs/phase-11-paper-account.md §6.6). RiskPct never disables itself
// the way b.riskPctPerTrade does at <=0 (RISK_PCT_PER_TRADE's own default):
// the paper account must always be able to size a trade, or the feature
// never fires.
func (b *Bot) paperConfig(m market.MarketID) paper.Config {
	risk := b.riskPctPerTrade
	if risk <= 0 {
		risk = 1.0
	}
	stopLossPct := b.stopLossPct
	trailingPct := b.trailingStopPct
	if m == market.TW {
		stopLossPct = b.stopLossPctTW
		trailingPct = b.trailingStopPctTW
	}
	return paper.Config{
		RiskPct:           risk,
		MaxPositionPct:    b.paperMaxPositionPct,
		StopATRMult:       stopCandidateATRMult,
		StopLossPct:       stopLossPct,
		TrailingPct:       trailingPct,
		TrailingATRMult:   b.trailingStopATRMult,
		TakeProfitATRMult: b.paperTakeProfitATRMult,
		Market:            m,
		FeeDiscount:       b.twFeeDiscount,
	}
}

// loadPaperAccount rebuilds m's in-memory paper.Account from paper.db: cash
// from settings (seeded from paperInitialCashFor on the very first call ever
// made for m) and holdings from positions, with each Holding.Peak restored
// via GetPeakClose(ticker, entryDate) — the same "recompute the running high
// from snapshot history on demand" trick checkTrailingStopAlerts already
// uses for the real portfolio — so a trailing stop's peak survives a bot
// restart instead of resetting to the entry price.
func (b *Bot) loadPaperAccount(m market.MarketID) (*paper.Account, error) {
	return b.papers().LoadAccount(m)
}

// applyPaperTrades is Phase 11 PR3's entry hook, called from
// sendAndSaveRecommendations right after SaveRecommendations — the single
// funnel point all three recommendation-producing paths (US daily report,
// TW daily report, /recommend) already go through (see
// docs/phase-11-paper-account.md §6.3). No-op when the feature is off
// (b.paperDB nil).
func (b *Bot) applyPaperTrades(recs []llm.Recommendation, prices, atrs map[string]float64, m market.MarketID) {
	if b.paperDB == nil {
		return
	}
	b.paperMu.Lock()
	defer b.paperMu.Unlock()

	results, err := b.papers().ApplyTrades(recs, prices, atrs, m, b.paperConfig(m), todayDate())
	if err != nil {
		logger.Errorf("paper: load account (%s): %v", m, err)
		return
	}
	for _, res := range results {
		b.sendPaperTradeAlert(res.Trade, res.CashAfter)
	}
}

// runPaperClose is Phase 11 PR3's exit hook, called from RunClosingSnapshot
// right after recordNetWorthSnapshot — so it only ever runs on a trading
// day, and only off a fresh (<=12h old) quote, the same guard
// RunClosingSnapshot already applies before building prices. date matches
// RunClosingSnapshot's own (Taiwan-relative) date semantics. Writes a
// daily_snapshots row per holding into paper.db before checking stops — the
// same "yesterday's positions face today's stop first" ordering PR1's
// MarkClose expects — which doubles as the data GetPeakClose (trailing
// stop) and the web dashboard's DailyPnL replay (PR4) both need, per
// docs/phase-11-paper-account.md §6.4.
func (b *Bot) runPaperClose(m market.MarketID, date string, prices map[string]float64) {
	if b.paperDB == nil {
		return
	}
	b.paperMu.Lock()
	defer b.paperMu.Unlock()

	atrFor := func(ticker string) (float64, bool) {
		if t, _, _ := b.computeTechnicals(ticker, nil); t != nil {
			return t.ATR14, true
		}
		return 0, false
	}
	results, ran, err := b.papers().RunClose(m, date, prices, atrFor, b.paperConfig(m))
	if err != nil {
		logger.Errorf("paper close: load account (%s): %v", m, err)
		return
	}
	if !ran {
		return
	}
	// snapshotBenchmarkTo writes an independent benchmark-ticker snapshot
	// row — no dependency on the holdings/trade data RunClose just computed
	// — so its ordering relative to that call doesn't matter.
	b.snapshotBenchmarkTo(b.paperDB, date, m)
	for _, res := range results {
		b.sendPaperTradeAlert(res.Trade, res.CashAfter)
	}
}

// sendPaperTradeAlert notifies a paper.db fill over Telegram — a 🧪 prefix
// so it reads as clearly distinct from a real /buy or /sell confirmation.
func (b *Bot) sendPaperTradeAlert(t paper.Trade, cashAfter float64) {
	if t.Side == "BUY" {
		b.Send(i18n.T(b.lang, i18n.KeyPaperBuyAlert, b.tickerLabel(t.Ticker), t.Shares, t.Price, t.Stop, cashAfter))
		return
	}
	b.Send(i18n.T(b.lang, i18n.KeyPaperSellAlert, b.tickerLabel(t.Ticker), t.Shares, t.Price, t.Reason, t.RealizedPnL, cashAfter))
}

// handlePaper is the /paper command: no argument reports both markets'
// current state (see sendPaperSection); "reset confirm" wipes paper.db and
// lets the next load reseed from PAPER_INITIAL_CASH_USD/TWD (see
// docs/phase-11-paper-account.md §6.5) — the "confirm" literal is required
// so a bare "/paper reset" typo can't wipe trading history by accident.
func (b *Bot) handlePaper(args string) {
	if b.paperDB == nil {
		b.Send(i18n.T(b.lang, i18n.KeyPaperDisabled))
		return
	}
	args = strings.TrimSpace(args)
	if args == "" {
		b.Send(i18n.T(b.lang, i18n.KeyPaperTitle))
		b.sendPaperSection(market.US)
		b.sendPaperSection(market.TW)
		return
	}
	if strings.EqualFold(args, "reset confirm") {
		b.handlePaperReset()
		return
	}
	b.Send(i18n.T(b.lang, i18n.KeyPaperResetUsage))
}

func (b *Bot) handlePaperReset() {
	b.paperMu.Lock()
	defer b.paperMu.Unlock()
	if err := b.papers().Reset(); err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyQueryFailed, err))
		return
	}
	b.Send(i18n.T(b.lang, i18n.KeyPaperResetDone))
}

// sendPaperSection renders /paper's per-market block, mirroring
// sendPortfolioSection's shape: sends nothing at all — not even the section
// title — for a market with no holdings and no trade history, so an
// unused book doesn't show a dangling empty block.
func (b *Bot) sendPaperSection(m market.MarketID) {
	acct, err := b.loadPaperAccount(m)
	if err != nil {
		logger.Errorf("paper: load account (%s): %v", m, err)
		return
	}
	realizedTotal, err := b.papers().RealizedPnL(m)
	if err != nil {
		logger.Errorf("paper: realized pnl (%s): %v", m, err)
	}
	if len(acct.Holdings) == 0 && realizedTotal == 0 {
		return
	}

	b.Send(i18n.T(b.lang, paperSectionTitleKey(m)))
	prices := make(map[string]float64, len(acct.Holdings))
	for _, ticker := range sortedHoldingTickers(acct.Holdings) {
		h := acct.Holdings[ticker]
		price, ok := b.priceFor(ticker, prices)
		if !ok {
			b.Send(i18n.T(b.lang, i18n.KeyQuoteUnavailable, b.tickerLabel(ticker)))
			continue
		}
		prices[ticker] = price

		unrealized := (price - h.AvgCost) * h.Shares
		unrealizedPct := (price - h.AvgCost) / h.AvgCost * 100
		var distToStopPct float64
		if price > 0 {
			distToStopPct = (price - h.Stop) / price * 100
		}
		b.Send(i18n.T(b.lang, i18n.KeyPaperLine, b.tickerLabel(ticker), h.Shares, h.AvgCost, price, h.Stop, distToStopPct, unrealized, unrealizedPct))
	}

	equity := acct.Equity(prices)
	initialCash := b.paperInitialCashFor(m)
	var totalReturnPct float64
	if initialCash > 0 {
		totalReturnPct = (equity - initialCash) / initialCash * 100
	}
	if m == market.TW {
		b.Send(i18n.T(b.lang, i18n.KeyPaperSummaryTWD, acct.Cash, equity, initialCash, totalReturnPct, realizedTotal))
	} else {
		b.Send(i18n.T(b.lang, i18n.KeyPaperSummary, acct.Cash, equity, initialCash, totalReturnPct, realizedTotal))
	}
}

func paperSectionTitleKey(m market.MarketID) i18n.Key {
	if m == market.TW {
		return i18n.KeyPaperSectionTW
	}
	return i18n.KeyPaperSectionUS
}

func sortedHoldingTickers(holdings map[string]paper.Holding) []string {
	return service.SortedHoldingTickers(holdings)
}
