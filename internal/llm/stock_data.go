package llm

import (
	"fmt"
	"strings"
	"time"

	"argus/internal/data"
	"argus/internal/i18n"
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
	// EarningsSurprises is Phase 23 PR8's actual-vs-estimate EPS history,
	// US-only, oldest first (nil when Finnhub isn't configured, same gate as
	// Fundamentals/AnalystRating/InsiderTx — see data.EarningsSurpriseProvider).
	// Briefing material only, never a ranking factor
	// (docs/phase-23-strategy-data-uplift.md §4.2) — writeStockSection
	// summarizes it into a beat/miss streak rather than listing every quarter,
	// same "raw data in, opinionated summary out" split as InsiderTx.
	EarningsSurprises []data.EarningsSurprise
	// InstitutionalFlow is optional, TW-only (see
	// data.InstitutionalFlowProvider): the ticker's most recent day of
	// three-major-institutional-investor (三大法人) net buy/sell, from TWSE's
	// own T86 report. Nil for a US ticker or when the lookup found no session
	// with data in range.
	InstitutionalFlow *data.InstitutionalFlow
	// SECSnapshot is Phase 23 PR6/PR7's valuation percentile + (US-only)
	// cash-flow quality — US via SEC EDGAR, TW via FinMind's TaiwanStockPER
	// (both share this one field/db.fundamental_snapshots row; see
	// bot.cachedValuationSnapshot for which provider gets picked). Nil when
	// the ticker's market has no configured provider or the lookup found
	// nothing usable. Briefing material only, never a ranking factor
	// (docs/phase-23-strategy-data-uplift.md §4.2) — reuses data.FundamentalSnapshot
	// directly rather than a mirrored llm-package type, same as
	// InstitutionalFlow above, since both already live in a package llm imports.
	SECSnapshot *data.FundamentalSnapshot
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
	// Thesis is the user's own holding rationale (see service.CurrentThesis,
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
// against (SPY for a US holding, 0050 for a TW one — see service.BenchmarkFor):
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

// Position is the subset of a db.Position an LLM prompt needs: shares held,
// the average cost basis, and the opened date (oldest open BUY lot, i.e.
// "held since") — without it the model has no way to tell a fresh
// yesterday's buy from a stale months-old holding when it calls SELL/HOLD.
// Kept separate from db.Position so this package doesn't need to import
// internal/db just for a prompt field.
type Position struct {
	Shares     float64
	AvgCost    float64
	OpenedDate string
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
				fmt.Fprint(sb, i18n.T(lang, i18n.KeyTechnicalsMALine, render.MALabel(lang, q.Price, ma.value), ma.period, ma.value))
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
		} else if hit.Name == "mtf_cross" {
			nameStr = i18n.T(lang, i18n.KeyStrategyMTFName)
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
			writeNewsItem(sb, lang, i+1, n)
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

	// Phase 23 PR8 (§4.2): a beat/miss streak + average surprise summary
	// rather than listing every quarter, same "opinionated summary" split as
	// InsiderTx above. SurprisePct itself is Finnhub's own figure, not
	// recomputed here — see data.EarningsSurprise's doc comment.
	if es := s.EarningsSurprises; len(es) > 0 {
		var beats, misses int
		var sum float64
		for _, e := range es {
			switch {
			case e.SurprisePct > 0:
				beats++
			case e.SurprisePct < 0:
				misses++
			}
			sum += e.SurprisePct
		}
		latest := es[len(es)-1]
		fmt.Fprint(sb, i18n.T(lang, i18n.KeyEarningsSurpriseLine, len(es), beats, misses, sum/float64(len(es)), latest.Period, latest.SurprisePct))
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

	if snap := s.SECSnapshot; snap != nil {
		// EPSAnnual is 0 for a Phase 23 PR7 (TW/FinMind) snapshot — that
		// path reports PER directly, no separate EPS figure — so this line
		// is conditional, unlike PR6's US/SEC path where it's always set
		// whenever snap is non-nil at all.
		if snap.EPSAnnual != 0 {
			fmt.Fprint(sb, i18n.T(lang, i18n.KeyValuationEPSLine, snap.AsOfFiscalYearEnd, snap.EPSAnnual))
		}
		if snap.PERatio > 0 {
			fmt.Fprint(sb, i18n.T(lang, i18n.KeyValuationPELine, snap.PERatio))
		}
		if snap.PEPercentile != nil {
			fmt.Fprint(sb, i18n.T(lang, i18n.KeyValuationPercentileLine, *snap.PEPercentile))
		}
		if snap.CashFlowQuality != nil {
			fmt.Fprint(sb, i18n.T(lang, i18n.KeyCashFlowQualityLine, *snap.CashFlowQuality))
		}
	}

	if p := s.Position; p != nil {
		unrealizedPct := (q.Price - p.AvgCost) / p.AvgCost * 100
		fmt.Fprint(sb, i18n.T(lang, i18n.KeyPositionLine, p.Shares, p.AvgCost, unrealizedPct, p.OpenedDate))
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
