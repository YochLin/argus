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
// as "no data", not "no volume". ATR14 (Average True Range, 14-day) is a
// volatility read from the daily high/low range rather than the closing
// price alone; like VolumeRatio, 0 means "not enough history yet", not
// "zero volatility".
// BollingerPctB is a pointer, unlike this struct's other fields, because 0
// is a legitimate reading for it (price sitting exactly at the lower band)
// — the 0-means-"not enough history" sentinel convention the other fields
// use would silently hide that real signal. nil means not enough history
// to compute it; see signals.BollingerPctB and bot.computeTechnicals.
type Technicals struct {
	RSI14             float64
	MACDTrend         string
	MA20, MA50, MA200 float64
	Volume            int64
	VolumeRatio       float64
	ATR14             float64
	BollingerPctB     *float64
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

// TradeLeg is one buy or sell within a ClosedTrade's round trip, oldest
// first. Side is the literal "BUY"/"SELL" (not localized — same convention
// as Recommendation.Action and the BUY/SELL/HOLD literals in
// KeyRecTaskBlock's expected output).
type TradeLeg struct {
	Side   string
	Shares float64
	Price  float64
	Date   string
}

// TradeRecommendation is one recommendation issued on a ticker during a
// ClosedTrade's holding period, so the review prompt can compare what the
// model said against what the user actually did.
type TradeRecommendation struct {
	Date   string
	Action string
	Reason string
}

// ClosedTrade is Phase 3.8 追加項's sell-review input (see
// docs/phase-3.8-sell-review.md): everything the review prompt needs about
// one fully closed round trip in a ticker. Package-local, not importing
// internal/db, same convention as Position/Earnings/PrevRecommendation.
// HoldingDays is precomputed by the caller (bot.reviewClosedTrade) so this
// package doesn't do date math against "now", same reasoning as
// Earnings.DaysUntil. VsSPY reuses the existing VsSPYReturn pair (ticker's
// own return next to SPY's over the same period) rather than a new type —
// nil when there's no same-period SPY close on either end to compare
// against (e.g. a buy that predates snapshotBenchmark, or a backdated date
// that wasn't a trading day). PeriodHigh/PeriodLow are 0 when there's no
// snapshot data in range at all.
type ClosedTrade struct {
	Ticker          string
	Legs            []TradeLeg
	RealizedPnL     float64
	HoldingDays     int
	VsSPY           *VsSPYReturn
	PeriodHigh      float64
	PeriodLow       float64
	Thesis          *string
	Recommendations []TradeRecommendation
}

// MarketContext is Phase 3.7 追加項's broad-market regime block (see
// docs/phase-3.7-market-regime.md): SPY's own trend (vs its MA50/MA200) and
// ^VIX's level, so a BUY call in a bull market isn't held to the same bar as
// one in a bear market. Package-local, not imported from internal/data, same
// attach-and-render convention as Position/Earnings/Technicals. 0 on any
// field means "couldn't be fetched" — writeMarketContext skips that field's
// line rather than rendering a misleading 0, same sentinel convention as
// Technicals' MA fields.
type MarketContext struct {
	SPYPrice, SPYMA50, SPYMA200 float64
	VIX                         float64
}

// vixCalmThreshold/vixPanicThreshold are fixed, not env-configurable — 15/25
// are the market's own conventional coarse VIX bands, not a user preference
// (see the design doc's rejected-alternatives section).
const (
	vixCalmThreshold  = 15
	vixPanicThreshold = 25
)

// regimeLabel returns the risk-on/risk-off i18n key for SPY's current price
// against its MA200 — the coarsest-grained trend fact the prompt can give the
// model. Pure so it's unit-testable without a full StockData/MarketContext.
func regimeLabel(price, ma200 float64) i18n.Key {
	if price > ma200 {
		return i18n.KeyRiskOn
	}
	return i18n.KeyRiskOff
}

// vixLabel returns the coarse VIX-band i18n key for a given VIX level (see
// vixCalmThreshold/vixPanicThreshold above).
func vixLabel(vix float64) i18n.Key {
	switch {
	case vix < vixCalmThreshold:
		return i18n.KeyVIXCalm
	case vix > vixPanicThreshold:
		return i18n.KeyVIXPanic
	default:
		return i18n.KeyVIXNormal
	}
}

// writeMarketContext renders the market-regime block (nil market, or a
// market with both SPYPrice/SPYMA200 and VIX unavailable, renders nothing —
// see MarketContext's 0-sentinel convention). Each of the SPY line and VIX
// line is independently skipped when its own inputs aren't available, same
// per-field degradation as writeStockSection's MA lines.
func writeMarketContext(sb *strings.Builder, lang i18n.Lang, m *MarketContext) {
	if m == nil {
		return
	}
	haveSPY := m.SPYPrice > 0 && m.SPYMA200 > 0
	haveVIX := m.VIX > 0
	if !haveSPY && !haveVIX {
		return
	}

	sb.WriteString(i18n.T(lang, i18n.KeyMarketRegimeHeader))
	if haveSPY {
		fmt.Fprint(sb, i18n.T(lang, i18n.KeyMarketRegimeSPYLine, m.SPYPrice, m.SPYMA200, m.SPYMA50, i18n.T(lang, regimeLabel(m.SPYPrice, m.SPYMA200))))
	}
	if haveVIX {
		fmt.Fprint(sb, i18n.T(lang, i18n.KeyMarketRegimeVIXLine, m.VIX, i18n.T(lang, vixLabel(m.VIX))))
	}
	sb.WriteString("\n")
}

func buildRecommendationPrompt(lang i18n.Lang, watchlist []StockData, candidates []StockData, marketNews []data.NewsItem, market *MarketContext) string {
	var sb strings.Builder

	sb.WriteString(i18n.T(lang, i18n.KeyRecPromptIntro))
	writeMarketContext(&sb, lang, market)

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
		if t.ATR14 > 0 && q.Price > 0 {
			fmt.Fprint(sb, i18n.T(lang, i18n.KeyATRLine, t.ATR14, t.ATR14/q.Price*100))
		}
		if t.BollingerPctB != nil {
			fmt.Fprint(sb, i18n.T(lang, i18n.KeyBollingerLine, *t.BollingerPctB*100))
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

// buildExplorePrompt is Phase 2.6 解凍's two-stage LLM exploration prompt
// (see docs/phase-2.6-two-stage-llm-exploration.md): market news only (no
// per-ticker StockData — the model is nominating tickers, not analyzing
// ones already in hand) plus the exclude list (watchlist ∪ candidates ∪
// positions) so the model doesn't waste a nomination on something already
// covered. Reuses KeyNewsItem/KeyReasonMarker verbatim rather than minting
// exploration-specific duplicates of the same content shape, same
// convention as buildTradeReviewPrompt reusing KeyVsSPYLine/KeyThesisLine.
func buildExplorePrompt(lang i18n.Lang, marketNews []data.NewsItem, exclude []string) string {
	var sb strings.Builder
	sb.WriteString(i18n.T(lang, i18n.KeyExplorePromptIntro))
	for i, n := range marketNews {
		fmt.Fprint(&sb, i18n.T(lang, i18n.KeyNewsItem, i+1, n.Source, n.Headline))
	}
	sb.WriteString("\n")
	sb.WriteString(i18n.T(lang, i18n.KeyExploreExcludeLine, strings.Join(exclude, ", ")))
	sb.WriteString(i18n.T(lang, i18n.KeyExploreTaskBlock, maxExploreNominations, i18n.T(lang, i18n.KeyExploreMarker), i18n.T(lang, i18n.KeyReasonMarker)))
	return sb.String()
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

// buildTradeReviewPrompt is Phase 3.8 追加項's sell-review prompt (see
// docs/phase-3.8-sell-review.md): a one-shot look back at a single fully
// closed round trip — every leg, the realized P&L, how the exit compares to
// the period's own high/low and to SPY over the same window, whether the
// user's own thesis panned out, and what the model itself said along the
// way. Each optional section (VsSPY/Thesis/period high-low/recommendations)
// is only rendered when data is actually available, same degrade-per-field
// convention as writeStockSection.
func buildTradeReviewPrompt(lang i18n.Lang, trade ClosedTrade) string {
	var sb strings.Builder
	sb.WriteString(i18n.T(lang, i18n.KeyTradeReviewPromptIntro, trade.Ticker))

	for _, leg := range trade.Legs {
		fmt.Fprint(&sb, i18n.T(lang, i18n.KeyTradeReviewLegLine, leg.Side, leg.Shares, leg.Price, leg.Date))
	}
	fmt.Fprint(&sb, i18n.T(lang, i18n.KeyTradeReviewPnLLine, trade.RealizedPnL, trade.HoldingDays))

	if trade.PeriodHigh > 0 && trade.PeriodLow > 0 {
		fmt.Fprint(&sb, i18n.T(lang, i18n.KeyTradeReviewRangeLine, trade.PeriodHigh, trade.PeriodLow))
	}
	if trade.VsSPY != nil {
		fmt.Fprint(&sb, i18n.T(lang, i18n.KeyVsSPYLine, trade.VsSPY.TickerPct, trade.VsSPY.SPYPct))
	}
	if trade.Thesis != nil {
		fmt.Fprint(&sb, i18n.T(lang, i18n.KeyThesisLine, *trade.Thesis))
	}

	if len(trade.Recommendations) > 0 {
		sb.WriteString(i18n.T(lang, i18n.KeyTradeReviewRecsHeader))
		for _, r := range trade.Recommendations {
			fmt.Fprint(&sb, i18n.T(lang, i18n.KeyTradeReviewRecLine, r.Date, r.Action, r.Reason))
		}
	}

	sb.WriteString(i18n.T(lang, i18n.KeyTradeReviewPromptTask))
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
