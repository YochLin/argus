package llm

import (
	"fmt"
	"strings"
	"time"

	"argus/internal/data"
	"argus/internal/i18n"
)

type StockData struct {
	Quote *data.Quote
	News  []data.NewsItem
	// Fundamentals and Statement are optional (nil when Finnhub isn't
	// configured). Statement is deliberately left unset for broad
	// multi-ticker calls (e.g. /recommend's market-mover candidates) to
	// keep prompts compact — see writeStockSection.
	Fundamentals *data.Fundamentals
	Statement    *data.FinancialStatement
	// Position is set when the user holds shares of this ticker, so a
	// SELL/HOLD call has actual cost basis to reason against instead of just
	// price action. Nil for tickers with no open position.
	Position *Position
	// Earnings is set when this ticker has a scheduled earnings report
	// within the fetch window (see bot.loadEarnings), so a BUY call doesn't
	// walk straight into next-day earnings volatility. Nil if nothing's
	// scheduled soon.
	Earnings *Earnings
	// ScanReason is set when this candidate was surfaced by the Phase 2.6
	// universe scan (bot.RunUniverseScan) rather than the market-movers list,
	// so the prompt can say what technical signal actually triggered its
	// inclusion. Nil for watchlist tickers and movers-sourced candidates.
	ScanReason *string
}

// Position is the subset of a db.Position an LLM prompt needs: shares held
// and the average cost basis. Kept separate from db.Position so this
// package doesn't need to import internal/db just for a prompt field.
type Position struct {
	Shares  float64
	AvgCost float64
}

// Earnings is the subset of a data.EarningsEvent an LLM prompt needs, with
// DaysUntil precomputed by the caller (bot.loadEarnings) so this package
// doesn't need to do date math against "now" itself.
type Earnings struct {
	Date      string
	DaysUntil int
}

type Recommendation struct {
	Ticker string
	Action string // BUY / SELL / HOLD ("" if the model omitted the action line)
	Reason string
}

func buildRecommendationPrompt(lang i18n.Lang, watchlist []StockData, candidates []StockData, marketNews []data.NewsItem) string {
	var sb strings.Builder

	sb.WriteString(i18n.T(lang, i18n.KeyRecPromptIntro))

	if len(marketNews) > 0 {
		sb.WriteString(i18n.T(lang, i18n.KeyRecMarketNewsHeader))
		for i, n := range marketNews {
			fmt.Fprint(&sb, i18n.T(lang, i18n.KeyNewsItem, i+1, n.Source, n.Headline))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(i18n.T(lang, i18n.KeyRecWatchlistHeader))

	if len(watchlist) == 0 {
		sb.WriteString(i18n.T(lang, i18n.KeyRecNoWatchlist))
	} else {
		for _, s := range watchlist {
			writeStockSection(&sb, lang, s)
		}
	}

	sb.WriteString(i18n.T(lang, i18n.KeyRecMoversHeader))
	if len(candidates) == 0 {
		sb.WriteString(i18n.T(lang, i18n.KeyRecNoCandidates))
	} else {
		for _, s := range candidates {
			writeStockSection(&sb, lang, s)
		}
	}

	if len(marketNews) > 0 {
		sb.WriteString(i18n.T(lang, i18n.KeyRecMarketSummaryTask, i18n.T(lang, i18n.KeyMarketSummaryMarker)))
	}

	action := i18n.T(lang, i18n.KeyActionMarker)
	reason := i18n.T(lang, i18n.KeyReasonMarker)
	sb.WriteString(i18n.T(lang, i18n.KeyRecTaskBlock, action, reason, action, reason))
	return sb.String()
}

func writeStockSection(sb *strings.Builder, lang i18n.Lang, s StockData) {
	q := s.Quote
	if q == nil {
		return
	}
	fmt.Fprint(sb, i18n.T(lang, i18n.KeyStockHeader, q.Ticker))
	fmt.Fprint(sb, i18n.T(lang, i18n.KeyPriceLine, q.Price, q.ChangePercent))
	fmt.Fprint(sb, i18n.T(lang, i18n.KeyOHLLine, q.Open, q.High, q.Low))
	fmt.Fprint(sb, i18n.T(lang, i18n.KeyVolumeLine, q.Volume, q.PrevClose))
	fmt.Fprint(sb, i18n.T(lang, i18n.KeyQuoteTimeLine, q.Timestamp.In(time.FixedZone("CST", 8*3600)).Format("2006-01-02 15:04")))

	if len(s.News) > 0 {
		sb.WriteString(i18n.T(lang, i18n.KeyNewsHeader))
		for i, n := range s.News {
			if i >= 5 {
				break
			}
			fmt.Fprint(sb, i18n.T(lang, i18n.KeyNewsItem, i+1, n.Source, n.Headline))
		}
	}

	if fd := s.Fundamentals; fd != nil {
		fmt.Fprint(sb, i18n.T(lang, i18n.KeyFundamentalsSummaryLine,
			fd.PE, fd.PB, fd.ROE, fd.GrossMarginPct, fd.OperatingMarginPct, fd.NetMarginPct,
			fd.DebtToEquity, fd.RevenueGrowthYoY, fd.EPSGrowthYoY, fd.DividendYieldPct, fd.Beta))
	}

	if st := s.Statement; st != nil {
		fmt.Fprint(sb, i18n.T(lang, i18n.KeyStatementSummaryLine,
			st.Form, st.FiscalYear, st.PeriodEnd,
			st.Revenue/1e6, st.GrossProfit/1e6, st.OperatingIncome/1e6, st.NetIncome/1e6,
			st.TotalAssets/1e6, st.TotalLiabilities/1e6, st.TotalEquity/1e6, st.OperatingCashFlow/1e6, st.FreeCashFlow/1e6))
	}

	if p := s.Position; p != nil {
		unrealizedPct := (q.Price - p.AvgCost) / p.AvgCost * 100
		fmt.Fprint(sb, i18n.T(lang, i18n.KeyPositionLine, p.Shares, p.AvgCost, unrealizedPct))
	}

	if e := s.Earnings; e != nil {
		fmt.Fprint(sb, i18n.T(lang, i18n.KeyEarningsLine, e.Date, e.DaysUntil))
	}

	if r := s.ScanReason; r != nil {
		fmt.Fprint(sb, i18n.T(lang, i18n.KeyScanHitLine, *r))
	}

	sb.WriteString("\n")
}

func buildCheckPrompt(lang i18n.Lang, s StockData) string {
	var sb strings.Builder
	sb.WriteString(i18n.T(lang, i18n.KeyCheckPromptIntro))
	writeStockSection(&sb, lang, s)
	sb.WriteString(i18n.T(lang, i18n.KeyCheckPromptTask))
	return sb.String()
}
