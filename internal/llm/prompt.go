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
	// AnalystRating is optional (nil when Finnhub isn't configured, same as
	// Fundamentals), giving valuation numbers an external analyst-consensus
	// anchor. Fetched only where Fundamentals is (see bot.fetchStockData's
	// includeFundamentals gate) since it's the same Finnhub per-ticker-call
	// rate-limit trade-off.
	AnalystRating *data.AnalystRating
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
	// Technicals is set whenever bot.HistoryProvider.GetHistory succeeded for
	// this ticker (see bot.fetchStockData/handleCheck), so a BUY/SELL call has
	// trend context (RSI/MACD/moving averages) instead of just a single day's
	// OHLCV. Nil if history couldn't be fetched.
	Technicals *Technicals
	// PrevRec is set when this ticker has a prior recommendation on record
	// (see bot.loadPrevRecs), so today's call comes with continuity: the
	// prompt can ask the model to explain a reversal instead of contradicting
	// an earlier call with no acknowledgment. Nil for a ticker recommended
	// for the first time.
	PrevRec *PrevRecommendation
	// Thesis is the user's own holding rationale (see db.GetThesis/SetThesis,
	// bot's /thesis command), attached only by handleInsight — deliberately
	// never by fetchStockData's other callers (/recommend, RunDailyReport),
	// per the Phase 3.6 expansion design: feeding the user's own bull case
	// into the recommendation prompt invites the model to confirm it rather
	// than challenge it, which is exactly the insight's job. Nil when the
	// ticker has no thesis on record.
	Thesis *string
	// VsSPY is the position's holding-period return set against SPY's over
	// the same period (see bot's per-position-vs-SPY helper), attached only
	// by handleInsight for the same reason Position/Earnings are — nil when
	// there's no BUY date or no same-date SPY snapshot to anchor the
	// comparison to.
	VsSPY *VsSPYReturn
}

// VsSPYReturn is a position's own holding-period return next to SPY's over
// the same period, so the insight can say whether a holding is actually
// beating the market rather than judging its price action in isolation. Both
// percentages ignore dividends (SPY distributes too, so this is a
// price-return-vs-price-return comparison on both sides — consistent, if not
// total-return-precise; see docs/phase-3.6-portfolio-insight.md's dividend
// icebox note).
type VsSPYReturn struct {
	TickerPct float64
	SPYPct    float64
}

// Technicals is the subset of computed technical-indicator values an LLM
// prompt needs. MACDTrend mirrors signals.MACDTrend's own vocabulary
// ("bullish"/"bearish"/"" for not-enough-history) as a plain string rather
// than importing internal/signals here, same reasoning as Position/Earnings
// staying package-local mini-structs. MA200 is 0 when there isn't ~200 days
// of history yet (e.g. a recent IPO). Volume/VolumeRatio come from
// HistoryProvider.GetHistory rather than StockData.Quote.Volume — the latter
// is always 0 for a Finnhub-quoted ticker (Finnhub's /quote has no volume
// field), so this is the only reliable volume source. VolumeRatio is 0 when
// there isn't ~21 days of history yet (see signals.VolumeRatio); 0 renders
// as "no data", not "no volume".
type Technicals struct {
	RSI14             float64
	MACDTrend         string
	MA20, MA50, MA200 float64
	Volume            int64
	VolumeRatio       float64
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

// PrevRecommendation is the subset of a db.Recommendation an LLM prompt
// needs for Phase 3.8's recommendation continuity: what the model said last
// time, so a reversal in today's call comes with an explanation instead of
// silent flip-flopping. DaysAgo is precomputed by the caller (bot.loadPrevRecs)
// so this package doesn't do date math against "now", same reasoning as
// Earnings.DaysUntil. Kept as a package-local mini-struct rather than reusing
// db.Recommendation directly, same reasoning as Position/Earnings.
type PrevRecommendation struct {
	Action  string
	Date    string
	Price   float64
	DaysAgo int
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
	// Prefer Technicals.Volume (from Yahoo's history endpoint) over
	// q.Volume — the latter is always 0 whenever the quote came from
	// Finnhub, which has no volume field on /quote at all.
	vol := q.Volume
	if t := s.Technicals; t != nil && t.Volume > 0 {
		vol = t.Volume
	}
	fmt.Fprint(sb, i18n.T(lang, i18n.KeyVolumeLine, vol, q.PrevClose))
	fmt.Fprint(sb, i18n.T(lang, i18n.KeyQuoteTimeLine, q.Timestamp.In(time.FixedZone("CST", 8*3600)).Format("2006-01-02 15:04")))

	if t := s.Technicals; t != nil {
		macdLabel := i18n.T(lang, i18n.KeyTrendUnknown)
		switch t.MACDTrend {
		case "bullish":
			macdLabel = i18n.T(lang, i18n.KeyTrendBullish)
		case "bearish":
			macdLabel = i18n.T(lang, i18n.KeyTrendBearish)
		}
		fmt.Fprint(sb, i18n.T(lang, i18n.KeyTechnicalsSummaryLine, t.RSI14, macdLabel))
		// Each MA is only rendered when there was enough history to compute
		// it — MA returns 0 as a sentinel otherwise (e.g. MA200 on a recent
		// IPO), and 0 would misleadingly look like a real price level.
		for _, ma := range []struct {
			period int
			value  float64
		}{{20, t.MA20}, {50, t.MA50}, {200, t.MA200}} {
			if ma.value > 0 {
				fmt.Fprint(sb, i18n.T(lang, i18n.KeyTechnicalsMALine, maLabel(lang, q.Price, ma.value), ma.period, ma.value))
			}
		}
		if t.VolumeRatio > 0 {
			fmt.Fprint(sb, i18n.T(lang, i18n.KeyVolumeRatioLine, t.VolumeRatio))
		}
	}

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
			fd.DebtToEquity, fd.RevenueGrowthYoY, fd.EPSGrowthYoY, fd.DividendYieldPct, fd.Beta,
			fd.EPS, fd.CurrentRatio, fd.MarketCapMillion,
			pctFrom(q.Price, fd.Week52High), pctFrom(q.Price, fd.Week52Low)))
	}

	if ar := s.AnalystRating; ar != nil {
		fmt.Fprint(sb, i18n.T(lang, i18n.KeyAnalystRatingLine, ar.Period, ar.StrongBuy, ar.Buy, ar.Hold, ar.Sell, ar.StrongSell))
		if ar.HasPrev {
			buyChange := (ar.StrongBuy + ar.Buy) - (ar.PrevStrongBuy + ar.PrevBuy)
			sellChange := (ar.StrongSell + ar.Sell) - (ar.PrevStrongSell + ar.PrevSell)
			fmt.Fprint(sb, i18n.T(lang, i18n.KeyAnalystRatingTrendLine, buyChange, sellChange))
		}
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

	if pr := s.PrevRec; pr != nil {
		fmt.Fprint(sb, i18n.T(lang, i18n.KeyPrevRecLine, pr.Action, pr.Price, pr.DaysAgo))
	}

	if th := s.Thesis; th != nil {
		fmt.Fprint(sb, i18n.T(lang, i18n.KeyThesisLine, *th))
	}

	if v := s.VsSPY; v != nil {
		fmt.Fprint(sb, i18n.T(lang, i18n.KeyVsSPYLine, v.TickerPct, v.SPYPct))
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

// buildInsightPrompt is Phase 3.6's portfolio-level analysis prompt: every
// held position (via writeStockSection, same per-ticker rendering
// /recommend and /check already use — quote, technicals, fundamentals,
// earnings, cost basis) followed by a portfolio-wide summary line and a task
// block that explicitly asks for concentration/thesis/rebalancing judgment
// rather than a repeat of the per-ticker analysis above it. cash is only
// rendered when haveCash is true (the user has run /cash at least once) —
// see PLAN.md's Phase 3.6 "現金水位" item: an unset cash balance should read
// as "no data," not silently as $0, which would misleadingly suggest 100%
// invested.
func buildInsightPrompt(lang i18n.Lang, positions []StockData, cash float64, haveCash bool) string {
	var sb strings.Builder
	sb.WriteString(i18n.T(lang, i18n.KeyInsightPromptIntro))

	var totalValue float64
	for _, s := range positions {
		if s.Quote != nil && s.Position != nil {
			totalValue += s.Quote.Price * s.Position.Shares
		}
		writeStockSection(&sb, lang, s)
	}

	sb.WriteString(i18n.T(lang, i18n.KeyInsightPositionValueLine, totalValue))
	if haveCash {
		sb.WriteString(i18n.T(lang, i18n.KeyInsightCashLine, cash, totalValue+cash))
	}

	sb.WriteString(i18n.T(lang, i18n.KeyInsightPromptTask))
	return sb.String()
}

// buildWeeklyReviewPrompt is Phase 3.6 PR2's Sunday weekly review prompt:
// the same per-position rendering and portfolio-value/cash lines as
// buildInsightPrompt, plus this week's /track summary (trackSummary, empty
// when there's no recommendation history yet — see bot.computeTrackData)
// folded into the same prompt so the model's portfolio judgment and its
// comment on recommendation accuracy come from a single coherent call
// rather than two.
func buildWeeklyReviewPrompt(lang i18n.Lang, positions []StockData, cash float64, haveCash bool, trackSummary string) string {
	var sb strings.Builder
	sb.WriteString(i18n.T(lang, i18n.KeyWeeklyReviewPromptIntro))

	var totalValue float64
	for _, s := range positions {
		if s.Quote != nil && s.Position != nil {
			totalValue += s.Quote.Price * s.Position.Shares
		}
		writeStockSection(&sb, lang, s)
	}

	sb.WriteString(i18n.T(lang, i18n.KeyInsightPositionValueLine, totalValue))
	if haveCash {
		sb.WriteString(i18n.T(lang, i18n.KeyInsightCashLine, cash, totalValue+cash))
	}

	if trackSummary != "" {
		sb.WriteString(i18n.T(lang, i18n.KeyWeeklyReviewTrackHeader))
		sb.WriteString(trackSummary)
	}

	sb.WriteString(i18n.T(lang, i18n.KeyWeeklyReviewPromptTask))
	return sb.String()
}

// maLabel renders whether price sits above or below a moving average as an
// already-localized string, so writeStockSection never builds display text
// outside of internal/i18n.
func maLabel(lang i18n.Lang, price, ma float64) string {
	if price > ma {
		return i18n.T(lang, i18n.KeyAboveMA)
	}
	return i18n.T(lang, i18n.KeyBelowMA)
}

// pctFrom returns the percentage difference of price from ref (e.g. a 52-week
// high/low), or 0 if ref is unavailable (0) to avoid a divide-by-zero.
func pctFrom(price, ref float64) float64 {
	if ref == 0 {
		return 0
	}
	return (price - ref) / ref * 100
}
