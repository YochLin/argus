package llm

import (
	"fmt"
	"strings"
	"time"

	"argus/internal/data"
	"argus/internal/i18n"
	"argus/internal/market"
	"argus/internal/render"
)

type StockData struct {
	Quote *data.Quote
	News  []data.NewsItem
	// CompanyName is the ticker's resolved display name (e.g. "台積電" for
	// 2330), set only for a TW ticker Argus could resolve one for (see
	// bot.companyName/data.CompanyNameProvider) — "" for a US ticker (its
	// symbol is already human-readable) or an unresolvable TW one.
	// writeStockSection renders it via data.TickerLabel instead of the bare
	// ticker in the section header, same attach-what's-available convention
	// as every other optional field here.
	CompanyName string
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
	// InsiderTx is optional (nil when Finnhub isn't configured, same gate as
	// Fundamentals/AnalystRating): the ticker's most recent SEC Form 4
	// filings, US-only (see data.InsiderTransactionProvider). writeStockSection
	// summarizes it into net open-market buy/sell counts rather than listing
	// every row, same "raw data in, opinionated summary out" split as
	// Fundamentals.
	InsiderTx []data.InsiderTransaction
	// InstitutionalFlow is optional, TW-only (see
	// data.InstitutionalFlowProvider): the ticker's most recent day of
	// three-major-institutional-investor (三大法人) net buy/sell, from TWSE's
	// own T86 report. Nil for a US ticker or when the lookup found no session
	// with data in range.
	InstitutionalFlow *data.InstitutionalFlow
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
	// Candles is the tail of the ticker's daily OHLCV history (the most
	// recent ~20 bars, oldest first — see bot.computeTechnicals'
	// promptCandleCount), so the model can read candlestick-level structure
	// (gaps, long wicks, a volume spike on a reversal day) that the
	// pre-digested indicator values in Technicals average away. Comes from
	// the same GetHistory call that computes Technicals, so it's nil exactly
	// when Technicals is.
	Candles []data.Candle
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
	// PastLessons is Phase 3.9's reflect-then-inject feedback loop (see
	// docs/research-tradingagents.md's "反思回饋迴路" section and
	// bot.loadPastLessons): every past closed-trade review lesson on record
	// for this specific ticker, oldest first. Nil/empty for a ticker with no
	// closed-trade review history yet. This is the "same ticker: bring all
	// of them" half of the loop; the "cross ticker: recent N, general" half
	// is a separate, prompt-wide input (see GenerateRecommendations'
	// recentLessons parameter) rather than a per-ticker field, since it
	// isn't about any one ticker.
	PastLessons []PastLesson
	// StrategyHits (Phase 3.10, extended Phase 14) contains any strategy
	// screen hit (squeeze_breakout / box_bottom / trend_breakout /
	// trend_pullback) detected within the last 5 days.
	StrategyHits []StrategyHitInfo
}

type StrategyHitInfo struct {
	Name    string
	DaysAgo int
}

// PastLesson is one row from Phase 3.9's trade-review feedback loop: a
// short, distilled takeaway parsed out of a past ReviewTrade reply (see
// llm.Client.ReviewTrade's lesson return value and db.Lesson). Ticker is
// included even though StockData.PastLessons is already scoped to one
// ticker, because the same struct is reused for GenerateRecommendations'
// cross-ticker recentLessons parameter, where showing which ticker a
// general lesson came from is the point.
type PastLesson struct {
	Ticker string
	Date   string
	Lesson string
}

// VsSPYReturn is a position's own holding-period return next to SPY's over
// the same period, so the insight can say whether a holding is actually
// beating the market rather than judging its price action in isolation. Both
// percentages ignore dividends (SPY distributes too, so this is a
// price-return-vs-price-return comparison on both sides — consistent, if not
// total-return-precise; see docs/phase-3.6-portfolio-insight.md's dividend
// icebox note). Bench is the benchmark ticker the SPYPct side was measured
// against (SPY for a US holding, 0050 for a TW one — see bot.benchmarkFor):
// with both markets in the same report, a line that hardcoded "SPY" would
// mislabel every TW comparison.
type VsSPYReturn struct {
	TickerPct float64
	SPYPct    float64
	Bench     string
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
	RSI14                        float64
	MACDTrend                    string
	MA5, MA20, MA50, MA60, MA200 float64
	Volume                       int64
	VolumeRatio                  float64
	ATR14                        float64
	BollingerPctB                *float64
	StochK, StochD               *float64
	Bandwidth                    *float64
	MAAlign                      string
	VolumePrice                  string
	NewHigh20, NewHigh52w        bool
	MACDAboveZero                *float64
	RS63                         *float64
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
// doesn't need to do date math against "now" itself. Estimated mirrors
// data.EarningsEvent.Estimated (true for the TW statutory-deadline proxy,
// see data.GetTWUpcomingEarnings) — writeStockSection renders a distinct,
// honest line for it rather than implying a confirmed per-company earnings
// date.
type Earnings struct {
	Date      string
	DaysUntil int
	Estimated bool
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
// StopPrice (Phase 3.11 PR1, §3.5) is the position's per-trade stop-loss
// price at the moment the round trip closed — 0 when none was ever set.
// This is deliberately the stop as it stood at close, not at entry: the
// caller (bot.recordSell) reads db.Position.StopPrice right before calling
// db.RecordSell, since a full close deletes the positions row and takes the
// stop price with it. If the stop was adjusted mid-hold, the R-multiple
// buildTradeReviewPrompt renders reflects that final risk definition rather
// than the original one — a deliberate approximation (see the design doc)
// rather than standing up a stop-price history table just for this.
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
	StopPrice       float64
}

// MarketContext is Phase 3.7 追加項's broad-market regime block (see
// docs/phase-3.7-market-regime.md): SPY's own trend (vs its MA50/MA200) and
// ^VIX's level, so a BUY call in a bull market isn't held to the same bar as
// one in a bear market. Package-local, not imported from internal/data, same
// attach-and-render convention as Position/Earnings/Technicals. 0 on any
// field means "couldn't be fetched" — writeMarketContext skips that field's
// line rather than rendering a misleading 0, same sentinel convention as
// Technicals' MA fields. VolProxyPct is TW's VIX substitute (docs/... TW
// data-gap investigation, 2026-07-28: no TW volatility-index ticker/dataset
// exists, free or paid) — an ATR14/close percentage on the TW benchmark's
// own candles rather than an options-implied figure, so it's rendered with
// its own line and wording, never conflated with VIX.
type MarketContext struct {
	// Bench is the benchmark ticker SPYPrice/SPYMA50/SPYMA200 were computed
	// from ("SPY" for US, "0050" for TW — see bot.benchmarkFor) — the field
	// names predate TW support and stay SPY-named, but the rendered label
	// must say which benchmark this actually is.
	Bench                       string
	SPYPrice, SPYMA50, SPYMA200 float64
	VIX                         float64
	VolProxyPct                 float64
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
// market with SPYPrice/SPYMA200, VIX, and VolProxyPct all unavailable,
// renders nothing — see MarketContext's 0-sentinel convention). Each line
// is independently skipped when its own inputs aren't available, same
// per-field degradation as writeStockSection's MA lines. VIX and
// VolProxyPct are mutually exclusive in practice (computeMarketRegime only
// ever sets one, depending on market) but aren't guarded against both being
// set here — rendering both would just be redundant, not wrong.
func writeMarketContext(sb *strings.Builder, lang i18n.Lang, m *MarketContext) {
	if m == nil {
		return
	}
	haveSPY := m.SPYPrice > 0 && m.SPYMA200 > 0
	haveVIX := m.VIX > 0
	haveVolProxy := m.VolProxyPct > 0
	if !haveSPY && !haveVIX && !haveVolProxy {
		return
	}

	sb.WriteString(i18n.T(lang, i18n.KeyMarketRegimeHeader))
	if haveSPY {
		fmt.Fprint(sb, i18n.T(lang, i18n.KeyMarketRegimeSPYLine, m.Bench, render.Money(m.Bench, m.SPYPrice), render.Money(m.Bench, m.SPYMA200), render.Money(m.Bench, m.SPYMA50), i18n.T(lang, regimeLabel(m.SPYPrice, m.SPYMA200))))
	}
	if haveVIX {
		fmt.Fprint(sb, i18n.T(lang, i18n.KeyMarketRegimeVIXLine, m.VIX, i18n.T(lang, vixLabel(m.VIX))))
	}
	if haveVolProxy {
		fmt.Fprint(sb, i18n.T(lang, i18n.KeyMarketRegimeVolProxyLine, m.VolProxyPct))
	}
	sb.WriteString("\n")
}

// IndexQuote is a plain label/price/change tuple for the morning briefing's
// broad-index summary (S&P 500/Nasdaq/Dow/Russell 2000 via their SPY/QQQ/
// DIA/IWM ETF proxies) — package-local like MarketContext, but deliberately
// not folded into it since MarketContext is SPY-only and used by the
// existing recommendation/check prompts. Label is a proper noun ("S&P 500"),
// not localized.
type IndexQuote struct {
	Label         string
	Price         float64
	ChangePercent float64
}

// buildMorningBriefingPrompt is the 07:00 CST US morning briefing's prompt
// (see bot.RunUSMorningBriefing), or, when isTW is true, the 08:30 CST TW
// pre-open briefing's prompt (see bot.RunTWMorningBriefing) — either way a
// narrative recap, distinct from buildRecommendationPrompt's BUY/SELL/HOLD
// framing. Reuses writeStockSection for the watchlist/movers sections the
// same way buildRecommendationPrompt does — StockData's per-field
// degradation means passing quote+news-only values (no
// Technicals/Fundamentals, see bot.loadQuoteHighlights) renders safely with
// no changes needed there.
func buildMorningBriefingPrompt(lang i18n.Lang, date string, indices []IndexQuote, vix float64, marketNews []data.NewsItem, watchlist []StockData, movers []StockData, isTW bool) string {
	var sb strings.Builder

	if isTW {
		sb.WriteString(i18n.T(lang, i18n.KeyTWMorningBriefingPromptIntro))
	} else {
		sb.WriteString(i18n.T(lang, i18n.KeyMorningBriefingPromptIntro))
	}

	sb.WriteString(i18n.T(lang, i18n.KeyMorningBriefingIndicesHeader))
	for _, idx := range indices {
		fmt.Fprint(&sb, i18n.T(lang, i18n.KeyMorningBriefingIndexLine, idx.Label, idx.Price, idx.ChangePercent))
	}
	if vix > 0 {
		fmt.Fprint(&sb, i18n.T(lang, i18n.KeyMorningBriefingVIXLine, vix))
	}
	sb.WriteString("\n")

	if len(marketNews) > 0 {
		sb.WriteString(i18n.T(lang, i18n.KeyMorningBriefingNewsHeader))
		for i, n := range marketNews {
			fmt.Fprint(&sb, i18n.T(lang, i18n.KeyNewsItem, i+1, n.Source, n.Headline))
			if n.Summary != "" {
				fmt.Fprint(&sb, i18n.T(lang, i18n.KeyNewsSummaryLine, n.Summary))
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString(i18n.T(lang, i18n.KeyMorningBriefingWatchlistHeader))
	if len(watchlist) == 0 {
		sb.WriteString(i18n.T(lang, i18n.KeyRecNoWatchlist))
	} else {
		for _, s := range watchlist {
			writeStockSection(&sb, lang, s)
		}
	}

	sb.WriteString(i18n.T(lang, i18n.KeyMorningBriefingMoversHeader))
	if len(movers) == 0 {
		sb.WriteString(i18n.T(lang, i18n.KeyRecNoCandidates))
	} else {
		for _, s := range movers {
			writeStockSection(&sb, lang, s)
		}
	}

	if isTW {
		sb.WriteString(i18n.T(lang, i18n.KeyTWMorningBriefingTaskBlock, date))
	} else {
		sb.WriteString(i18n.T(lang, i18n.KeyMorningBriefingTaskBlock, date))
	}
	return sb.String()
}

func buildRecommendationPrompt(lang i18n.Lang, watchlist []StockData, candidates []StockData, marketNews []data.NewsItem, market *MarketContext, recentLessons []PastLesson, isTW bool) string {
	var sb strings.Builder

	if isTW {
		sb.WriteString(i18n.T(lang, i18n.KeyRecPromptIntroTW))
		sb.WriteString(i18n.T(lang, i18n.KeyRecTWMarketNote))
	} else {
		sb.WriteString(i18n.T(lang, i18n.KeyRecPromptIntro))
	}
	writeMarketContext(&sb, lang, market)
	writeRecentLessons(&sb, lang, recentLessons)

	if len(marketNews) > 0 {
		sb.WriteString(i18n.T(lang, i18n.KeyRecMarketNewsHeader))
		for i, n := range marketNews {
			fmt.Fprint(&sb, i18n.T(lang, i18n.KeyNewsItem, i+1, n.Source, n.Headline))
			if n.Summary != "" {
				fmt.Fprint(&sb, i18n.T(lang, i18n.KeyNewsSummaryLine, n.Summary))
			}
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
	sb.WriteString(i18n.T(lang, i18n.KeyTechGuidanceBlock))
	return sb.String()
}

func writeStockSection(sb *strings.Builder, lang i18n.Lang, s StockData) {
	q := s.Quote
	if q == nil {
		return
	}
	fmt.Fprint(sb, i18n.T(lang, i18n.KeyStockHeader, data.TickerLabel(q.Ticker, s.CompanyName)))
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
		if t.StochK != nil && t.StochD != nil {
			fmt.Fprint(sb, i18n.T(lang, i18n.KeyTechnicalsKDLine, *t.StochK, *t.StochD))
		}
		// Each MA is only rendered when there was enough history to compute
		// it — MA returns 0 as a sentinel otherwise (e.g. MA200 on a recent
		// IPO), and 0 would misleadingly look like a real price level.
		for _, ma := range []struct {
			period int
			value  float64
		}{{5, t.MA5}, {20, t.MA20}, {50, t.MA50}, {60, t.MA60}, {200, t.MA200}} {
			if ma.value > 0 {
				fmt.Fprint(sb, i18n.T(lang, i18n.KeyTechnicalsMALine, maLabel(lang, q.Price, ma.value), ma.period, ma.value))
			}
		}
		if t.MAAlign != "" {
			alignLabel := i18n.T(lang, i18n.KeyTrendUnknown)
			if t.MAAlign == "bullish" {
				alignLabel = i18n.T(lang, i18n.KeyTrendBullish)
			} else if t.MAAlign == "bearish" {
				alignLabel = i18n.T(lang, i18n.KeyTrendBearish)
			}
			fmt.Fprint(sb, i18n.T(lang, i18n.KeyTechnicalsMAAlignLine, alignLabel))
		}
		if t.VolumeRatio > 0 {
			fmt.Fprint(sb, i18n.T(lang, i18n.KeyVolumeRatioLine, t.VolumeRatio))
		}
		if t.VolumePrice != "" {
			vpKey := i18n.KeyVolUpPriceUp
			switch t.VolumePrice {
			case "vol_up_price_up":
				vpKey = i18n.KeyVolUpPriceUp
			case "vol_down_price_down":
				vpKey = i18n.KeyVolDownPriceDown
			case "vol_down_price_up":
				vpKey = i18n.KeyVolDownPriceUp
			case "vol_up_price_down":
				vpKey = i18n.KeyVolUpPriceDown
			}
			fmt.Fprint(sb, i18n.T(lang, i18n.KeyTechnicalsVolumePriceLine, i18n.T(lang, vpKey)))
		}
		if t.NewHigh20 || t.NewHigh52w {
			nhKey := i18n.KeyNewHigh20
			if t.NewHigh20 && t.NewHigh52w {
				nhKey = i18n.KeyNewHigh20And52
			} else if t.NewHigh52w {
				nhKey = i18n.KeyNewHigh52
			}
			fmt.Fprint(sb, i18n.T(lang, i18n.KeyTechnicalsNewHighLine, i18n.T(lang, nhKey)))
		}
		if t.MACDAboveZero != nil {
			mzKey := i18n.KeyMACDAboveZero
			if *t.MACDAboveZero < 0 {
				mzKey = i18n.KeyMACDBelowZero
			}
			fmt.Fprint(sb, i18n.T(lang, i18n.KeyTechnicalsMACDZeroLine, i18n.T(lang, mzKey), *t.MACDAboveZero))
		}
		if t.RS63 != nil {
			rsKey := i18n.KeyRSStronger
			if *t.RS63 < 0 {
				rsKey = i18n.KeyRSWeaker
			}
			fmt.Fprint(sb, i18n.T(lang, i18n.KeyTechnicalsRSLine, i18n.T(lang, rsKey), *t.RS63))
		}
		if t.ATR14 > 0 && q.Price > 0 {
			fmt.Fprint(sb, i18n.T(lang, i18n.KeyATRLine, t.ATR14, t.ATR14/q.Price*100))
		}
		if t.Bandwidth != nil {
			fmt.Fprint(sb, i18n.T(lang, i18n.KeyTechnicalsBandwidthLine, *t.Bandwidth*100))
		}
		if t.BollingerPctB != nil {
			fmt.Fprint(sb, i18n.T(lang, i18n.KeyBollingerLine, *t.BollingerPctB*100))
		}
	}

	for _, hit := range s.StrategyHits {
		nameStr := hit.Name
		if hit.Name == "squeeze_breakout" {
			nameStr = i18n.T(lang, i18n.KeyStrategySqueezeName)
		} else if hit.Name == "box_bottom" {
			nameStr = i18n.T(lang, i18n.KeyStrategyBoxName)
		} else if hit.Name == "trend_breakout" {
			nameStr = i18n.T(lang, i18n.KeyStrategyBreakoutName)
		} else if hit.Name == "trend_pullback" {
			nameStr = i18n.T(lang, i18n.KeyStrategyPullbackName)
		}
		daysAgoStr := i18n.T(lang, i18n.KeyDaysAgoToday)
		if hit.DaysAgo > 0 {
			daysAgoStr = i18n.T(lang, i18n.KeyDaysAgoN, hit.DaysAgo)
		}
		fmt.Fprint(sb, i18n.T(lang, i18n.KeyStrategyHitLine, nameStr, daysAgoStr))
	}

	if len(s.Candles) > 0 {
		fmt.Fprint(sb, i18n.T(lang, i18n.KeyCandlesHeader, len(s.Candles)))
		for _, c := range s.Candles {
			fmt.Fprint(sb, i18n.T(lang, i18n.KeyCandleLine,
				c.Date.Format("2006-01-02"), c.Open, c.High, c.Low, c.Close, c.Volume))
		}
	}

	if len(s.News) > 0 {
		sb.WriteString(i18n.T(lang, i18n.KeyNewsHeader))
		for i, n := range s.News {
			if i >= 5 {
				break
			}
			fmt.Fprint(sb, i18n.T(lang, i18n.KeyNewsItem, i+1, n.Source, n.Headline))
			if n.Summary != "" {
				fmt.Fprint(sb, i18n.T(lang, i18n.KeyNewsSummaryLine, n.Summary))
			}
		}
	}

	if fd := s.Fundamentals; fd != nil {
		fmt.Fprint(sb, i18n.T(lang, i18n.KeyFundamentalsSummaryLine,
			fd.PE, fd.PB, fd.ROE, fd.GrossMarginPct, fd.OperatingMarginPct, fd.NetMarginPct,
			fd.DebtToEquity, fd.RevenueGrowthYoY, fd.EPSGrowthYoY, fd.DividendYieldPct, fd.Beta,
			fd.EPS, fd.CurrentRatio, fd.MarketCapMillion,
			pctFrom(q.Price, fd.Week52High), pctFrom(q.Price, fd.Week52Low)))
		// TW-only (FinMind, Phase 6 PR3) — 0 for every US ticker, so kept
		// out of the packed summary line above and skipped like the MA
		// lines below rather than rendering a misleading "0.0%".
		if fd.MonthRevenueYoYPct != 0 {
			fmt.Fprint(sb, i18n.T(lang, i18n.KeyMonthRevenueYoYLine, fd.MonthRevenueYoYPct))
		}
	}

	if ar := s.AnalystRating; ar != nil {
		fmt.Fprint(sb, i18n.T(lang, i18n.KeyAnalystRatingLine, ar.Period, ar.StrongBuy, ar.Buy, ar.Hold, ar.Sell, ar.StrongSell))
		if ar.HasPrev {
			buyChange := (ar.StrongBuy + ar.Buy) - (ar.PrevStrongBuy + ar.PrevBuy)
			sellChange := (ar.StrongSell + ar.Sell) - (ar.PrevStrongSell + ar.PrevSell)
			fmt.Fprint(sb, i18n.T(lang, i18n.KeyAnalystRatingTrendLine, buyChange, sellChange))
		}
	}

	// Only "P" (open-market buy) and "S" (open-market sale) reflect an
	// insider actually spending/collecting real money — codes like "M"
	// (option exercise), "A" (grant), "G" (gift), "F" (tax withholding) are
	// mechanical and would just dilute the signal if summed in.
	if txs := s.InsiderTx; len(txs) > 0 {
		var buyCount, sellCount int
		var buyShares, sellShares int64
		var latestDate string
		for _, tx := range txs {
			if tx.TransactionCode != "P" && tx.TransactionCode != "S" {
				continue
			}
			if tx.Change > 0 {
				buyCount++
				buyShares += tx.Change
			} else if tx.Change < 0 {
				sellCount++
				sellShares += -tx.Change
			}
			if tx.TransactionDate > latestDate {
				latestDate = tx.TransactionDate
			}
		}
		if buyCount > 0 || sellCount > 0 {
			fmt.Fprint(sb, i18n.T(lang, i18n.KeyInsiderTxLine, latestDate, buyCount, buyShares, sellCount, sellShares))
		}
	}

	if fl := s.InstitutionalFlow; fl != nil {
		fmt.Fprint(sb, i18n.T(lang, i18n.KeyInstitutionalFlowLine, fl.Date,
			fl.ForeignNet+fl.ForeignDealerNet, fl.TrustNet, fl.DealerNet, fl.TotalNet))
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
		key := i18n.KeyEarningsLine
		if e.Estimated {
			key = i18n.KeyEarningsLineEstimated
		}
		fmt.Fprint(sb, i18n.T(lang, key, e.Date, e.DaysUntil))
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
		fmt.Fprint(sb, i18n.T(lang, i18n.KeyVsSPYLine, v.TickerPct, v.Bench, v.SPYPct))
	}

	if len(s.PastLessons) > 0 {
		sb.WriteString(i18n.T(lang, i18n.KeyPastLessonsHeader))
		for _, l := range s.PastLessons {
			fmt.Fprint(sb, i18n.T(lang, i18n.KeyPastLessonLine, l.Date, l.Lesson))
		}
	}

	sb.WriteString("\n")
}

// writeRecentLessons renders Phase 3.9's cross-ticker "recent N lessons,
// general" block (see docs/research-tradingagents.md's "反思回饋迴路"
// section) — prompt-wide context, not per-ticker, so it's written once near
// the top of buildRecommendationPrompt rather than inside writeStockSection.
// Writes nothing at all when lessons is empty, so a fresh install (no
// closed-trade reviews yet) doesn't leave a dangling empty header.
func writeRecentLessons(sb *strings.Builder, lang i18n.Lang, lessons []PastLesson) {
	if len(lessons) == 0 {
		return
	}
	sb.WriteString(i18n.T(lang, i18n.KeyRecentLessonsHeader))
	for _, l := range lessons {
		fmt.Fprint(sb, i18n.T(lang, i18n.KeyRecentLessonLine, l.Ticker, l.Date, l.Lesson))
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
		if n.Summary != "" {
			fmt.Fprint(&sb, i18n.T(lang, i18n.KeyNewsSummaryLine, n.Summary))
		}
	}
	sb.WriteString("\n")
	sb.WriteString(i18n.T(lang, i18n.KeyExploreExcludeLine, strings.Join(exclude, ", ")))
	sb.WriteString(i18n.T(lang, i18n.KeyExploreTaskBlock, maxExploreNominations, i18n.T(lang, i18n.KeyExploreMarker), i18n.T(lang, i18n.KeyReasonMarker)))
	return sb.String()
}

func buildCheckPrompt(lang i18n.Lang, s StockData) string {
	var sb strings.Builder
	if s.Quote != nil && market.Of(s.Quote.Ticker) == market.TW {
		sb.WriteString(i18n.T(lang, i18n.KeyCheckPromptIntroTW))
	} else {
		sb.WriteString(i18n.T(lang, i18n.KeyCheckPromptIntro))
	}
	writeStockSection(&sb, lang, s)
	sb.WriteString(i18n.T(lang, i18n.KeyCheckPromptTask))
	sb.WriteString(i18n.T(lang, i18n.KeyTechGuidanceBlock))
	return sb.String()
}

// buildInsightPrompt is Phase 3.6's portfolio-level analysis prompt: every
// held position (via writeStockSection, the same per-ticker rendering
// /recommend and /check already use — quote, technicals, fundamentals,
// earnings, cost basis) followed by a portfolio-wide summary line and a task
// block that explicitly asks for concentration/thesis/rebalancing judgment
// rather than a repeat of the per-ticker analysis above it. cash is only
// rendered when haveCash is true (the user has run /cash at least once) —
// see PLAN.md's Phase 3.6 "現金水位" item: an unset cash balance should read
// as "no data," not silently as $0, which would misleadingly suggest 100%
// invested.
//
// One market per call (isTW says which, reusing KeyRecTWMarketNote to declare
// the whole prompt TWD-denominated), same split as buildWeeklyReviewPrompt:
// a mixed prompt summed TWD and USD position values into one meaningless
// total value and had the model comparing position sizes across currencies.
func buildInsightPrompt(lang i18n.Lang, positions []StockData, cash float64, haveCash bool, isTW bool) string {
	var sb strings.Builder
	sb.WriteString(i18n.T(lang, i18n.KeyInsightPromptIntro))
	if isTW {
		sb.WriteString(i18n.T(lang, i18n.KeyRecTWMarketNote))
	}

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
//
// One market per call (positions/cash/trackSummary are all that market's, and
// isTW says which — reusing KeyRecTWMarketNote to declare the whole prompt
// TWD-denominated, same as buildRecommendationPrompt): a mixed prompt summed
// TWD and USD position values into one number and left the model comparing
// concentration across two currencies, which is what made the combined review
// unreadable.
func buildWeeklyReviewPrompt(lang i18n.Lang, positions []StockData, cash float64, haveCash bool, trackSummary string, isTW bool) string {
	var sb strings.Builder
	sb.WriteString(i18n.T(lang, i18n.KeyWeeklyReviewPromptIntro))
	if isTW {
		sb.WriteString(i18n.T(lang, i18n.KeyRecTWMarketNote))
	}

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
		fmt.Fprint(&sb, i18n.T(lang, i18n.KeyVsSPYLine, trade.VsSPY.TickerPct, trade.VsSPY.Bench, trade.VsSPY.SPYPct))
	}
	if trade.Thesis != nil {
		fmt.Fprint(&sb, i18n.T(lang, i18n.KeyThesisLine, *trade.Thesis))
	}
	if trade.StopPrice > 0 {
		if entryPrice, shares := tradeEntryPrice(trade.Legs); entryPrice > trade.StopPrice && shares > 0 {
			rMultiple := trade.RealizedPnL / ((entryPrice - trade.StopPrice) * shares)
			fmt.Fprint(&sb, i18n.T(lang, i18n.KeyRMultipleLine, rMultiple))
		}
	}

	if len(trade.Recommendations) > 0 {
		sb.WriteString(i18n.T(lang, i18n.KeyTradeReviewRecsHeader))
		for _, r := range trade.Recommendations {
			fmt.Fprint(&sb, i18n.T(lang, i18n.KeyTradeReviewRecLine, r.Date, r.Action, r.Reason))
		}
	}

	sb.WriteString(i18n.T(lang, i18n.KeyTradeReviewPromptTask, i18n.T(lang, i18n.KeyLessonMarker)))
	return sb.String()
}

// tradeEntryPrice returns the share-weighted average BUY price and total
// BUY shares within legs — buildTradeReviewPrompt's R-multiple denominator
// input (§3.5). Package-local rather than a reuse of bot's own
// weightedAvgPrice: that helper operates on []db.Transaction, this on
// []TradeLeg, and this package doesn't import internal/db (same convention
// as every other ClosedTrade/StockData mini-struct here). Returns 0, 0 when
// legs has no BUY side at all (shouldn't happen for a real closed round,
// but the caller already guards on shares > 0 before dividing by it).
func tradeEntryPrice(legs []TradeLeg) (price, shares float64) {
	var totalShares, totalCost float64
	for _, l := range legs {
		if l.Side != "BUY" {
			continue
		}
		totalShares += l.Shares
		totalCost += l.Shares * l.Price
	}
	if totalShares == 0 {
		return 0, 0
	}
	return totalCost / totalShares, totalShares
}

// buildPriceEventPrompt is Phase 20's gap/big-move event-summary prompt (see
// docs/phase-20-price-event-log.md §4.3): the day's triggered gap%/change%
// numbers plus ticker-only news (reusing KeyNewsItem/KeyNewsSummaryLine, same
// convention as buildExplorePrompt) — no market-wide news, per §2's "LLM
// 素材" decision. gapPct/changePct are 0 when that particular threshold
// didn't fire (see signals.PriceEvent), so only the ones that are non-zero
// get rendered rather than always naming both.
func buildPriceEventPrompt(lang i18n.Lang, ticker string, gapPct, changePct float64, news []data.NewsItem) string {
	var sb strings.Builder
	sb.WriteString(i18n.T(lang, i18n.KeyPriceEventPromptIntro, ticker))

	if gapPct != 0 {
		fmt.Fprint(&sb, i18n.T(lang, i18n.KeyPriceEventGapLine, gapPct))
	}
	if changePct != 0 {
		fmt.Fprint(&sb, i18n.T(lang, i18n.KeyPriceEventChangeLine, changePct))
	}

	if len(news) > 0 {
		sb.WriteString(i18n.T(lang, i18n.KeyNewsHeader))
		for i, n := range news {
			fmt.Fprint(&sb, i18n.T(lang, i18n.KeyNewsItem, i+1, n.Source, n.Headline))
			if n.Summary != "" {
				fmt.Fprint(&sb, i18n.T(lang, i18n.KeyNewsSummaryLine, n.Summary))
			}
		}
	}

	sb.WriteString(i18n.T(lang, i18n.KeyPriceEventPromptTask))
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
