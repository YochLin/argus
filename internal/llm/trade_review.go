package llm

import (
	"fmt"
	"strings"

	"argus/internal/i18n"
)

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
	// Followup is Phase 26's post-sell follow-up review input (see
	// docs/phase-26-sell-followup.md) — nil for the existing at-close
	// review (Phase 3.8), set for the 5-trading-day-later look-back. When
	// non-nil, buildTradeReviewPrompt renders an extra post-exit price
	// block and swaps in the follow-up task section instead of the
	// standard one.
	Followup *TradeFollowup
	// Snapshots is Phase 27 P7b's picture of the stock on the day of the
	// round's entry and exit fills (see TradeSnapshot) — empty when the
	// candles or the stored news for those days aren't available. It never
	// carries anything from after the fill day: what happened since the exit
	// is Followup's job, and only the follow-up review gets it.
	Snapshots []TradeSnapshot
}

// TradeSnapshot is one fill's day, read at that day's close: the four
// indicator readings the chart page's 成交快照 tab shows (service.SnapshotAt),
// and the headlines bot.captureFillNews stored. Package-local, same
// convention as the rest of ClosedTrade; Trend is one of "bull" / "bear" /
// "range" (service.Trend*). Deliberately not carried: the candle patterns
// (descriptive only, no backtest — signals.DetectPatterns) and the LLM's own
// labels on the headlines, because a review's lesson is fed back into the
// recommendation prompt (PastLessons) and neither may reach it.
type TradeSnapshot struct {
	Side           string // "BUY" / "SELL"
	Date           string
	Price          float64
	RSI, RSIPrev5  float64
	MACDHist       float64
	MACDDays       int // bars the histogram has kept its sign
	Trend          string
	CloseVsMA20Pct float64
	MA20Slope5dPct float64
	VolRatio20     float64
	// Intraday marks a fill whose session was still running (service.BarForming):
	// the readings are of the latest price, and VolRatio20 is a part-day volume
	// that must not be shown — writeTradeSnapshot leaves it out.
	Intraday bool
	News     []TradeSnapshotNews
}

// TradeSnapshotNews is one stored headline.
type TradeSnapshotNews struct {
	Source   string
	Headline string
}

// TradeFollowup is Phase 26's post-sell follow-up input: how ticker traded
// in the 5 trading days after the exit that closed ClosedTrade's round
// trip. TradingDaysAfterExit is precomputed by the caller
// (bot.followupClosedTrade), same "no date math in this package" convention
// as ClosedTrade.HoldingDays. Verdict is bot.followupVerdict's pure
// three-way classification ("sold_early"/"good_exit"/"neutral") computed
// before the LLM call, so the prompt (and the message it renders even if
// the LLM call fails) always says something meaningful.
type TradeFollowup struct {
	TradingDaysAfterExit int
	ExitPrice            float64
	PriceAfter           float64
	HighAfter            float64
	LowAfter             float64
	PctSinceExit         float64
	Verdict              string
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

	for _, s := range trade.Snapshots {
		writeTradeSnapshot(&sb, lang, s)
	}

	task := i18n.KeyTradeReviewPromptTask
	if f := trade.Followup; f != nil {
		fmt.Fprint(&sb, i18n.T(lang, i18n.KeyTradeFollowupBlock, f.TradingDaysAfterExit, f.ExitPrice, f.PriceAfter, f.PctSinceExit, f.HighAfter, f.LowAfter))
		task = i18n.KeyTradeFollowupPromptTask
	}
	sb.WriteString(i18n.T(lang, task, i18n.T(lang, i18n.KeyLessonMarker)))
	return sb.String()
}

// writeTradeSnapshot renders one fill's snapshot block of the review prompt.
func writeTradeSnapshot(sb *strings.Builder, lang i18n.Lang, s TradeSnapshot) {
	trend := map[string]i18n.Key{
		"bull": i18n.KeyTradeSnapshotTrendBull,
		"bear": i18n.KeyTradeSnapshotTrendBear,
	}[s.Trend]
	if trend == "" {
		trend = i18n.KeyTradeSnapshotTrendRange
	}
	header := i18n.KeyTradeSnapshotHeader
	if s.Intraday {
		header = i18n.KeyTradeSnapshotHeaderIntraday
	}
	fmt.Fprint(sb, i18n.T(lang, header, s.Side, s.Date, s.Price))
	fmt.Fprint(sb, i18n.T(lang, i18n.KeyTradeSnapshotIndicators,
		s.RSI, s.RSIPrev5, s.MACDHist, s.MACDDays, i18n.T(lang, trend), s.CloseVsMA20Pct, s.MA20Slope5dPct))
	if !s.Intraday {
		fmt.Fprint(sb, i18n.T(lang, i18n.KeyTradeSnapshotVolume, s.VolRatio20))
	}
	if len(s.News) > 0 {
		sb.WriteString(i18n.T(lang, i18n.KeyTradeSnapshotNewsHeader))
		for _, n := range s.News {
			fmt.Fprint(sb, i18n.T(lang, i18n.KeyTradeSnapshotNewsLine, n.Headline, n.Source))
		}
	}
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
