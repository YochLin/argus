package service

import (
	"sort"
	"strconv"

	"argus/internal/db"
	"argus/internal/llm"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/paper"
)

// PaperStore is the persistence boundary PaperService needs — paper.db's
// own settings/positions/snapshot writes, narrowed to exactly what
// PaperService calls (paper.db is a second, independent *db.DB, but this
// service has no reason to depend on the concrete type).
type PaperStore interface {
	GetPositions() ([]db.Position, error)
	GetEarliestBuyDate(ticker string) (string, bool, error)
	GetPeakClose(ticker, sinceDate string) (float64, bool, error)
	GetSetting(key string) (string, bool, error)
	SetSetting(key, value string) error
	RecordBuy(ticker string, shares, price, fee float64, date string) (db.Position, error)
	SetStopPrice(ticker string, price float64) error
	RecordSell(ticker string, shares, price, fee float64, date string) (db.Position, float64, error)
	SaveSnapshot(s db.DailySnapshot) error
	SaveNetWorthSnapshot(date string, m market.MarketID, total float64) error
	GetRealizedPnL(m market.MarketID) (float64, error)
	ResetTradingData() error
}

// PaperService owns the Phase 11 paper-trading account: loading it from
// paper.db, applying recommendation-driven entries, and marking exits at
// the close. Formatting/Telegram alerts stay in internal/bot — every method
// here returns raw paper.Trade values (paired with the account's post-trade
// cash balance) for the adapter to render.
type PaperService struct {
	store                          PaperStore
	quotes                         RiskQuoteReader
	initialCashUSD, initialCashTWD float64
	takeProfitATRMult              float64 // PAPER_TAKE_PROFIT_ATR_MULT; <=0 disables Target reconstruction
}

func NewPaperService(store PaperStore, quotes RiskQuoteReader, initialCashUSD, initialCashTWD, takeProfitATRMult float64) *PaperService {
	return &PaperService{store: store, quotes: quotes, initialCashUSD: initialCashUSD, initialCashTWD: initialCashTWD, takeProfitATRMult: takeProfitATRMult}
}

// InitialCashFor returns m's first-run cash seed — only ever consulted the
// first time LoadAccount runs for m (see loadOrSeedCash); every later load
// reads the persisted settings value instead.
func (s *PaperService) InitialCashFor(m market.MarketID) float64 {
	if m == market.TW {
		return s.initialCashTWD
	}
	return s.initialCashUSD
}

func (s *PaperService) priceFor(ticker string, prices map[string]float64) (float64, bool) {
	if p, ok := prices[ticker]; ok {
		return p, true
	}
	if s.quotes != nil {
		q, err := s.quotes.GetQuote(ticker)
		if err == nil && q != nil && q.Price > 0 {
			return q.Price, true
		}
	}
	return 0, false
}

// LoadAccount rebuilds m's in-memory paper.Account from paper.db: cash from
// settings (seeded from InitialCashFor(m) the very first time it's ever read
// for m) and holdings from positions, with each Holding.Peak restored via
// GetPeakClose(ticker, entryDate) — the same "recompute the running high
// from snapshot history on demand" trick the real portfolio's trailing-stop
// check uses — so a trailing stop's peak survives a bot restart instead of
// resetting to the entry price. Target is reverse-derived from the stored
// stop_price using StopCandidateATRMult (paper.db's positions row has no
// target/atr_at_entry column — see docs/phase-11-paper-strategy-tuning.md
// §3.2); a position that fell back to a fixed StopLossPct stop gets no
// target, matching Account.ApplySignal's own "no ATR, no target" rule.
func (s *PaperService) LoadAccount(m market.MarketID) (*paper.Account, error) {
	cash, err := s.loadOrSeedCash(m)
	if err != nil {
		return nil, err
	}
	acct := paper.NewAccount(cash)

	positions, err := s.store.GetPositions()
	if err != nil {
		return nil, err
	}
	for _, p := range positions {
		if market.Of(p.Ticker) != m {
			continue
		}
		h := paper.Holding{Shares: p.Shares, AvgCost: p.AvgCost, Stop: p.StopPrice, Peak: p.AvgCost}
		if s.takeProfitATRMult > 0 && p.StopPrice > 0 && p.StopPrice < p.AvgCost {
			atrAtEntry := (p.AvgCost - p.StopPrice) / StopCandidateATRMult
			h.Target = p.AvgCost + s.takeProfitATRMult*atrAtEntry
		}
		if buyDate, ok, err := s.store.GetEarliestBuyDate(p.Ticker); err == nil && ok {
			h.EntryDate = buyDate
			if peak, ok, err := s.store.GetPeakClose(p.Ticker, buyDate); err == nil && ok && peak > 0 {
				h.Peak = peak
			}
		}
		acct.Holdings[p.Ticker] = h
	}
	return acct, nil
}

func (s *PaperService) loadOrSeedCash(m market.MarketID) (float64, error) {
	raw, ok, err := s.store.GetSetting(CashSettingKey(m))
	if err != nil {
		return 0, err
	}
	if !ok {
		cash := s.InitialCashFor(m)
		if err := s.store.SetSetting(CashSettingKey(m), strconv.FormatFloat(cash, 'f', 2, 64)); err != nil {
			return 0, err
		}
		logger.Infof("paper account: seeded %s cash %.2f", m, cash)
		return cash, nil
	}
	return strconv.ParseFloat(raw, 64)
}

// persistTrade writes t (an Account.ApplySignal/MarkClose result) to
// paper.db and syncs the cash setting to acct.Cash — acct already holds the
// authoritative post-trade balance, so this never recomputes it.
func (s *PaperService) persistTrade(acct *paper.Account, t paper.Trade) error {
	if t.Side == "BUY" {
		if _, err := s.store.RecordBuy(t.Ticker, t.Shares, t.Price, t.Fee, t.Date); err != nil {
			return err
		}
		if err := s.store.SetStopPrice(t.Ticker, t.Stop); err != nil {
			return err
		}
	} else {
		if _, _, err := s.store.RecordSell(t.Ticker, t.Shares, t.Price, t.Fee, t.Date); err != nil {
			return err
		}
	}
	return s.store.SetSetting(CashSettingKey(market.Of(t.Ticker)), strconv.FormatFloat(acct.Cash, 'f', 2, 64))
}

// PaperTradeResult pairs one executed trade with the account's cash balance
// immediately after it, since ApplyTrades/RunClose apply trades against a
// live *paper.Account whose Cash keeps moving as each fill is processed.
type PaperTradeResult struct {
	Trade     paper.Trade
	CashAfter float64
}

// ApplyTrades is Phase 11 PR3's entry hook: it walks recs in order,
// applying any BUY/SELL action against acct one at a time via
// Account.ApplySignal and persisting each fill immediately (so CashAfter on
// trade N already reflects trades 1..N of this same call).
func (s *PaperService) ApplyTrades(recs []llm.Recommendation, prices, atrs map[string]float64, m market.MarketID, cfg paper.Config, date string) ([]PaperTradeResult, error) {
	acct, err := s.LoadAccount(m)
	if err != nil {
		return nil, err
	}

	var results []PaperTradeResult
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
		if err := s.persistTrade(acct, trade); err != nil {
			logger.Errorf("paper: persist %s %s: %v", trade.Side, trade.Ticker, err)
			continue
		}
		results = append(results, PaperTradeResult{Trade: trade, CashAfter: acct.Cash})
	}
	return results, nil
}

// RunClose is Phase 11 PR3's exit hook: snapshot each holding's close price
// into paper.db (the same data GetPeakClose and the web dashboard's daily
// P&L replay both need), then mark stops/targets/trailing exits via
// Account.MarkClose, persisting and returning every resulting trade. ran is
// false when m has no open paper holdings — the caller should skip anything
// that only makes sense after a real close pass (e.g. a benchmark snapshot).
// atrFor looks up a ticker's ATR14 (the caller's technicals computation
// isn't this package's concern); a ticker atrFor can't price is simply
// omitted from the atrs map passed to MarkClose, same as the map lookup
// returning its zero value would have done.
func (s *PaperService) RunClose(m market.MarketID, date string, prices map[string]float64, atrFor func(ticker string) (float64, bool), cfg paper.Config) (results []PaperTradeResult, ran bool, err error) {
	acct, err := s.LoadAccount(m)
	if err != nil {
		return nil, false, err
	}
	if len(acct.Holdings) == 0 {
		return nil, false, nil
	}

	closes := make(map[string]float64, len(acct.Holdings))
	atrs := make(map[string]float64, len(acct.Holdings))
	for ticker := range acct.Holdings {
		price, ok := s.priceFor(ticker, prices)
		if !ok {
			continue
		}
		closes[ticker] = price
		// ponytail: OHLC all set to the single close price we have — priceFor
		// discards a fresh GetQuote's real Open/High/Low, and this row is only
		// ever read back via its Close (GetPeakClose/DailyPnL), so the other
		// fields being flat doesn't affect anything today.
		if err := s.store.SaveSnapshot(db.DailySnapshot{Ticker: ticker, Date: date, Open: price, Close: price, High: price, Low: price}); err != nil {
			logger.Errorf("paper close: save snapshot %s: %v", ticker, err)
		}
		if atr, ok := atrFor(ticker); ok {
			atrs[ticker] = atr
		}
	}

	for _, t := range acct.MarkClose(date, closes, atrs, cfg) {
		if err := s.persistTrade(acct, t); err != nil {
			logger.Errorf("paper close: persist %s %s: %v", t.Side, t.Ticker, err)
			continue
		}
		results = append(results, PaperTradeResult{Trade: t, CashAfter: acct.Cash})
	}

	var total float64
	for ticker, h := range acct.Holdings {
		if price, ok := closes[ticker]; ok {
			total += h.Shares * price
		}
	}
	if err := s.store.SaveNetWorthSnapshot(date, m, total); err != nil {
		logger.Errorf("paper close: net worth snapshot: %v", err)
	}

	return results, true, nil
}

// RealizedPnL passes through to paper.db's own realized-P&L total for m.
func (s *PaperService) RealizedPnL(m market.MarketID) (float64, error) {
	return s.store.GetRealizedPnL(m)
}

// Reset wipes paper.db's trading data so the next LoadAccount reseeds from
// the configured initial cash.
func (s *PaperService) Reset() error {
	return s.store.ResetTradingData()
}

// SortedHoldingTickers returns holdings' tickers sorted ascending, for
// deterministic /paper output ordering.
func SortedHoldingTickers(holdings map[string]paper.Holding) []string {
	tickers := make([]string, 0, len(holdings))
	for t := range holdings {
		tickers = append(tickers, t)
	}
	sort.Strings(tickers)
	return tickers
}
