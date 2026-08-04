package bot

import (
	"log"
	"sort"
	"strconv"
	"strings"

	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/llm"
	"argus/internal/market"
	"argus/internal/paper"
)

// paperInitialCashFor returns m's first-run cash seed (PAPER_INITIAL_CASH_USD/
// PAPER_INITIAL_CASH_TWD) — only ever consulted the first time loadPaperAccount
// runs for m (see loadOrSeedPaperCash); every later load reads the persisted
// settings value instead.
func (b *Bot) paperInitialCashFor(m market.MarketID) float64 {
	if m == market.TW {
		return b.paperInitialCashTWD
	}
	return b.paperInitialCashUSD
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
	return paper.Config{
		RiskPct:         risk,
		MaxPositionPct:  b.paperMaxPositionPct,
		StopATRMult:     stopCandidateATRMult,
		StopLossPct:     b.stopLossPct,
		TrailingPct:     b.trailingStopPct,
		TrailingATRMult: b.trailingStopATRMult,
		Market:          m,
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
	cash, err := b.loadOrSeedPaperCash(m)
	if err != nil {
		return nil, err
	}
	acct := paper.NewAccount(cash)

	positions, err := b.paperDB.GetPositions()
	if err != nil {
		return nil, err
	}
	for _, p := range positions {
		if market.Of(p.Ticker) != m {
			continue
		}
		h := paper.Holding{Shares: p.Shares, AvgCost: p.AvgCost, Stop: p.StopPrice, Peak: p.AvgCost}
		if buyDate, ok, err := b.paperDB.GetEarliestBuyDate(p.Ticker); err == nil && ok {
			h.EntryDate = buyDate
			if peak, ok, err := b.paperDB.GetPeakClose(p.Ticker, buyDate); err == nil && ok && peak > 0 {
				h.Peak = peak
			}
		}
		acct.Holdings[p.Ticker] = h
	}
	return acct, nil
}

// loadOrSeedPaperCash returns m's current paper cash balance, seeding it
// from paperInitialCashFor(m) (and logging the seed, per
// docs/phase-11-paper-account.md §8.3's acceptance check) the first time
// it's ever read.
func (b *Bot) loadOrSeedPaperCash(m market.MarketID) (float64, error) {
	raw, ok, err := b.paperDB.GetSetting(cashSettingKeyFor(m))
	if err != nil {
		return 0, err
	}
	if !ok {
		cash := b.paperInitialCashFor(m)
		if err := b.paperDB.SetSetting(cashSettingKeyFor(m), strconv.FormatFloat(cash, 'f', 2, 64)); err != nil {
			return 0, err
		}
		log.Printf("paper account: seeded %s cash %.2f", m, cash)
		return cash, nil
	}
	return strconv.ParseFloat(raw, 64)
}

// persistPaperTrade writes t (an internal/paper.Account.ApplySignal/
// MarkClose result) to paper.db and syncs the cash setting to acct.Cash.
// acct is passed in rather than this function recomputing the new cash
// balance from t's own fields, since acct already holds the authoritative
// post-trade balance — recomputing it here would be a second implementation
// of arithmetic paper.Account's buy/sell already did, the exact kind of
// drift docs/phase-11-paper-account.md §1 says this whole design exists to
// avoid. RecordSell's realized-P&L/stop-price snapshot both come from
// paper.db's own positions row (set by the matching RecordBuy/SetStopPrice
// call), so they're guaranteed consistent with what RecordBuy wrote.
func (b *Bot) persistPaperTrade(acct *paper.Account, t paper.Trade) error {
	if t.Side == "BUY" {
		if _, err := b.paperDB.RecordBuy(t.Ticker, t.Shares, t.Price, t.Fee, t.Date); err != nil {
			return err
		}
		if err := b.paperDB.SetStopPrice(t.Ticker, t.Stop); err != nil {
			return err
		}
	} else {
		if _, _, err := b.paperDB.RecordSell(t.Ticker, t.Shares, t.Price, t.Fee, t.Date); err != nil {
			return err
		}
	}
	return b.paperDB.SetSetting(cashSettingKeyFor(market.Of(t.Ticker)), strconv.FormatFloat(acct.Cash, 'f', 2, 64))
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

	acct, err := b.loadPaperAccount(m)
	if err != nil {
		log.Printf("paper: load account (%s): %v", m, err)
		return
	}
	cfg := b.paperConfig(m)
	date := todayDate()

	for _, r := range recs {
		if r.Action != "BUY" && r.Action != "SELL" {
			continue
		}
		price, ok := prices[r.Ticker]
		if !ok || price <= 0 {
			continue
		}
		trade, ok := acct.ApplySignal(paper.Signal{Date: date, Ticker: r.Ticker, Action: r.Action, Price: price}, price, atrs[r.Ticker], cfg)
		if !ok {
			continue
		}
		if err := b.persistPaperTrade(acct, trade); err != nil {
			log.Printf("paper: persist %s %s: %v", trade.Side, trade.Ticker, err)
			continue
		}
		b.sendPaperTradeAlert(trade, acct.Cash)
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

	acct, err := b.loadPaperAccount(m)
	if err != nil {
		log.Printf("paper close: load account (%s): %v", m, err)
		return
	}
	if len(acct.Holdings) == 0 {
		return
	}

	closes := make(map[string]float64, len(acct.Holdings))
	atrs := make(map[string]float64, len(acct.Holdings))
	for ticker := range acct.Holdings {
		price, ok := b.priceFor(ticker, prices)
		if !ok {
			continue
		}
		closes[ticker] = price
		// ponytail: OHLC all set to the single close price we have — priceFor
		// discards a fresh GetQuote's real Open/High/Low, and this row is
		// only ever read back via its Close (GetPeakClose/DailyPnL), so the
		// other fields being flat doesn't affect anything today. Thread real
		// OHLC through if a future feature reads them.
		if err := b.paperDB.SaveSnapshot(db.DailySnapshot{Ticker: ticker, Date: date, Open: price, Close: price, High: price, Low: price}); err != nil {
			log.Printf("paper close: save snapshot %s: %v", ticker, err)
		}
		if t, _, _ := b.computeTechnicals(ticker, nil); t != nil {
			atrs[ticker] = t.ATR14
		}
	}
	b.snapshotBenchmarkTo(b.paperDB, date, m)

	cfg := b.paperConfig(m)
	for _, t := range acct.MarkClose(date, closes, atrs, cfg) {
		if err := b.persistPaperTrade(acct, t); err != nil {
			log.Printf("paper close: persist %s %s: %v", t.Side, t.Ticker, err)
			continue
		}
		b.sendPaperTradeAlert(t, acct.Cash)
	}

	var total float64
	for ticker, h := range acct.Holdings {
		if price, ok := closes[ticker]; ok {
			total += h.Shares * price
		}
	}
	if err := b.paperDB.SaveNetWorthSnapshot(date, m, total); err != nil {
		log.Printf("paper close: net worth snapshot: %v", err)
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
	if err := b.paperDB.ResetTradingData(); err != nil {
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
		log.Printf("paper: load account (%s): %v", m, err)
		return
	}
	realizedTotal, err := b.paperDB.GetRealizedPnL(m)
	if err != nil {
		log.Printf("paper: realized pnl (%s): %v", m, err)
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
	tickers := make([]string, 0, len(holdings))
	for t := range holdings {
		tickers = append(tickers, t)
	}
	sort.Strings(tickers)
	return tickers
}
