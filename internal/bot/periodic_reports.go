package bot

import (
	"context"
	"strings"
	"time"

	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/service"
)

// weeklyNetWorthLine renders RunWeeklyReview's opening line: total position
// value and its % change from about a week ago — net_worth_snapshots' first
// reader since RunClosingSnapshot's recordNetWorthSnapshot started writing
// it in Phase 2 (Phase 3.6 PR2). The business logic (which baseline to
// compare against, when to skip) lives in service.WeeklyNetWorthChange
// (Phase 24 Stage 3); this is just the i18n rendering of its result,
// picking the TWD-labeled key pair for market.TW, same precedent as
// sendPortfolioSection's US/TW key selection.
func (b *Bot) weeklyNetWorthLine(m market.MarketID, cash float64, haveCash bool) (string, error) {
	latest, pctChange, ok, err := service.WeeklyNetWorthChange(b.db, m)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", nil
	}

	if m == market.TW {
		if haveCash {
			return i18n.T(b.lang, i18n.KeyWeeklyNetWorthLineWithCashTWD, latest, pctChange, latest+cash), nil
		}
		return i18n.T(b.lang, i18n.KeyWeeklyNetWorthLineTWD, latest, pctChange), nil
	}
	if haveCash {
		return i18n.T(b.lang, i18n.KeyWeeklyNetWorthLineWithCash, latest, pctChange, latest+cash), nil
	}
	return i18n.T(b.lang, i18n.KeyWeeklyNetWorthLine, latest, pctChange), nil
}

// RunWeeklyReview is Phase 3.6 PR2's Sunday portfolio review: the same
// per-position data assembly handleInsight uses (positions, technicals,
// fundamentals, earnings, thesis, vs-SPY, cash), plus this week's /track
// summary folded into the same LLM call (so the model can comment on
// recommendation accuracy alongside its portfolio judgment — see
// llm.Client.WeeklyReview), wrapped with a net-worth opening line and a
// next-week earnings preview appended after. Scheduled for Sunday (US
// markets closed) — a review rhythm, not a reactive one, deliberately below
// the project's daily-cadence ceiling (see PLAN.md's Phase 3.6 note).
// Wired up only after several manual /insight runs had proven the
// underlying prompt, so an untuned prompt never lands in the push channel
// (see docs/phase-3.6-portfolio-insight.md).
//
// One message per market, each from its own LLM call over only that market's
// holdings, cash, /track summary and earnings preview: a single combined call
// produced a review that compared TWD position sizes against USD ones,
// summed both into one "total assets" figure, and interleaved US and TW
// judgments in one wall of prose. Markets with no positions are skipped
// entirely (no empty per-market message), same convention as
// RunMonthlyReport's per-market blocks.
func (b *Bot) RunWeeklyReview(ctx context.Context) {
	defer b.recoverJobPanic("weekly review")

	positions, err := b.db.GetPositions()
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyQueryFailed, err))
		return
	}
	if len(positions) == 0 {
		b.Send(i18n.T(b.lang, i18n.KeyPortfolioEmpty))
		return
	}

	b.Send(i18n.T(b.lang, i18n.KeyWeeklyReviewStart))

	// Computed once for both markets — computeTrackRows fetches a quote per
	// recommended ticker, and running it twice would double that traffic just
	// to throw half the rows away each time. Each market's call filters it.
	var trackRows []trackRow
	if rows, _, ok, err := b.computeTrackRows(7); err != nil {
		logger.Errorf("weekly review: track rows: %v", err)
	} else if ok {
		trackRows = rows
	}

	for _, m := range []market.MarketID{market.US, market.TW} {
		b.runWeeklyReviewMarket(ctx, m, positions, trackRows)
	}
}

// runWeeklyReviewMarket is RunWeeklyReview's per-market half: one LLM call
// and one message covering market m only. Returns silently when m has no
// positions.
func (b *Bot) runWeeklyReviewMarket(ctx context.Context, m market.MarketID, positions []db.Position, trackRows []trackRow) {
	stocks, earnings := b.portfolioStocks(m, positions)
	if len(stocks) == 0 {
		return
	}

	cash, haveCash, err := b.loadCash(m)
	if err != nil {
		logger.Errorf("weekly review: load cash (%s): %v", m, err)
	}

	var trackSummary string
	var marketRows []trackRow
	for _, r := range trackRows {
		if r.Market == m {
			marketRows = append(marketRows, r)
		}
	}
	if len(marketRows) > 0 {
		overall, bySource, byMarket := summarizeTrack(marketRows)
		trackSummary = renderTrackSummary(b.lang, overall, bySource, byMarket)
	}

	result, err := b.llm.WeeklyReview(ctx, stocks, cash, haveCash, trackSummary, m == market.TW)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyLLMFailed, err))
		return
	}

	titleKey := i18n.KeyWeeklyReviewResultTitleUS
	if m == market.TW {
		titleKey = i18n.KeyWeeklyReviewResultTitleTW
	}

	var sb strings.Builder
	// Skipped individually when this market has no snapshot history yet
	// (weeklyNetWorthLine's own "" = skip contract).
	if line, err := b.weeklyNetWorthLine(m, cash, haveCash); err != nil {
		logger.Errorf("weekly review: net worth line (%s): %v", m, err)
	} else if line != "" {
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	sb.WriteString(i18n.T(b.lang, titleKey, result))
	sb.WriteString(renderEarningsPreview(b.lang, earnings, 7))

	b.Send(sb.String())
}

// RunMonthlyReport is Phase 3.6 追加項's net-worth monthly report (see
// docs/phase-3.6-monthly-report.md): a deliberately non-LLM data archive for
// the prior full calendar month — deterministic (same DB contents always
// produce the same report) and unaffected by the LLM provider chain being
// down, unlike RunWeeklyReview's judgment-based prose. Phase 6 PR2 splits
// this into per-market blocks (§5.3) via buildMonthlyReportBlock — a market
// with no net_worth_snapshots row anywhere in the month has its whole block
// skipped (see that function), same "查無資料就跳過不發空報告" convention as
// every other optional block in this file; if *neither* market has any data
// at all, nothing is sent (log-only), same as the pre-Phase-6 single-block
// behavior for a fresh install's first month.
func (b *Bot) RunMonthlyReport(ctx context.Context) {
	defer b.recoverJobPanic("monthly report")

	from, to := service.MonthRange(time.Now().In(cst))

	usBlock, usOK := b.buildMonthlyReportBlock(market.US, from, to)
	twBlock, twOK := b.buildMonthlyReportBlock(market.TW, from, to)
	if !usOK && !twOK {
		logger.Infof("monthly report: no net worth snapshots for %s..%s in either market, skipping", from, to)
		return
	}

	var sb strings.Builder
	sb.WriteString(i18n.T(b.lang, i18n.KeyMonthlyReportTitle, from[:7]))
	if usOK {
		sb.WriteString(i18n.T(b.lang, i18n.KeyPortfolioSectionUS))
		sb.WriteString(usBlock)
	}
	if twOK {
		sb.WriteString(i18n.T(b.lang, i18n.KeyPortfolioSectionTW))
		sb.WriteString(twBlock)
	}

	b.Send(sb.String())
}

// buildMonthlyReportBlock renders one market's monthly-report body
// (sparkline, month-end change, max drawdown, realized P&L, transaction
// count, benchmark move, cash) — Phase 6 PR2's per-market split of what used
// to be RunMonthlyReport's single US-only body (§5.3). ok is false when m
// has no net_worth_snapshots row anywhere in [from, to], meaning the whole
// block should be skipped rather than shown empty; every other line is
// independently optional (guarded by the corresponding Have* field) and
// just omitted, same degrade-per-field convention as the rest of this file.
// The actual data gathering/derivation lives in
// service.BuildMonthlyReportBlock (Phase 24 Stage 3) — this function is
// just the i18n rendering of its result. Content lines deliberately keep
// their pre-Phase-6 "$"-formatted keys (KeyMonthlyReportChangeLine et al.,
// reused unchanged for the TW block too) rather than gaining a TWD variant
// each — the caller's KeyPortfolioSectionUS/TW header is what establishes
// which currency a block's numbers are in, matching this project's accepted
// "只有新增的聚合行 key 直接把幣別做進文案" simplification (see
// docs/phase-6-tw-market.md §3.2); only the genuinely new benchmark line
// (SPY vs. 0050 — a different benchmark name, not just a currency symbol)
// gets its own TW-specific key.
func (b *Bot) buildMonthlyReportBlock(m market.MarketID, from, to string) (string, bool) {
	cash, haveCash, err := b.loadCash(m)
	if err != nil {
		logger.Errorf("monthly report: load cash (%s): %v", m, err)
	}

	block, ok := service.BuildMonthlyReportBlock(b.db, m, from, to, cash, haveCash)
	if !ok {
		return "", false
	}

	var sb strings.Builder
	sb.WriteString(i18n.T(b.lang, i18n.KeyMonthlyReportSparklineLine, service.Sparkline(block.Values)))
	if block.HaveChange {
		sb.WriteString(i18n.T(b.lang, i18n.KeyMonthlyReportChangeLine, block.Latest, block.ChangePct))
	}
	sb.WriteString(i18n.T(b.lang, i18n.KeyMonthlyReportDrawdownLine, block.DrawdownPct))
	if block.SellCount > 0 {
		sb.WriteString(i18n.T(b.lang, i18n.KeyMonthlyReportRealizedLine, block.Realized))
	}
	sb.WriteString(i18n.T(b.lang, i18n.KeyMonthlyReportTxCountLine, block.TxCount))
	if block.HaveBenchmark {
		if m == market.TW {
			sb.WriteString(i18n.T(b.lang, i18n.KeyMonthlyReportTWBenchmarkLine, block.BenchmarkPct))
		} else {
			sb.WriteString(i18n.T(b.lang, i18n.KeyMonthlyReportSPYLine, block.BenchmarkPct))
		}
	}
	if block.HaveCash {
		sb.WriteString(i18n.T(b.lang, i18n.KeyMonthlyReportCashLine, block.Latest+block.Cash, block.Cash))
	}

	return sb.String(), true
}
