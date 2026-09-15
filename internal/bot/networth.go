package bot

import (
	"fmt"
	"time"

	"argus/internal/i18n"
	"argus/internal/render"
	"argus/internal/service"
)

// handleNetworth backs /networth (Phase 9 波次1 PR2') — a text summary of
// the same numbers /w/balance renders on the web dashboard, built off the
// shared service.WealthTotals/service.AssetLiabilityTotals/
// service.ComputeHealthMetrics so the two surfaces can never disagree about
// a number. b.db satisfies service.WealthStore/WealthFXStore directly
// (*db.DB has every method both interfaces need) and b.provider satisfies
// service.QuoteReader.
func (b *Bot) handleNetworth() {
	now := b.now()
	today := now.Format("2006-01-02")

	todayTotal, _, todayOK := service.WealthTotals(b.db, b.db, b.provider, today, true)
	if !todayOK {
		b.Send(i18n.T(b.lang, i18n.KeyNetworthNoData))
		return
	}

	yearStart := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
	ytdPct, ytdOK := service.PeriodReturnPct(b.db, b.db, b.provider, yearStart, todayTotal, todayOK)
	monthAgo := now.AddDate(0, -1, 0).Format("2006-01-02")
	momPct, momOK := service.PeriodReturnPct(b.db, b.db, b.provider, monthAgo, todayTotal, todayOK)
	totalAssets, totalLiabilities, totalsOK := service.AssetLiabilityTotals(b.db, b.db, b.provider, today, true)
	hm := service.ComputeHealthMetrics(b.db, b.db, b.provider, now)

	var sb string
	sb += i18n.T(b.lang, i18n.KeyNetworthTitle)
	sb += i18n.T(b.lang, i18n.KeyNetworthSummaryLine,
		twd(todayTotal), pctOrDash(ytdPct, ytdOK), pctOrDash(momPct, momOK),
		twdOrDash(totalAssets, totalsOK), twdOrDash(totalLiabilities, totalsOK))
	sb += i18n.T(b.lang, i18n.KeyNetworthHealthHeader)
	sb += i18n.T(b.lang, i18n.KeyNetworthHealthLine,
		pctPtrOrDash(hm.DebtRatioPct), monthsPtrOrDash(hm.LiquidityMonths),
		pctPtrOrDash(hm.SavingsRatePct), pctPtrOrDash(hm.ExpenseRatioPct))

	b.Send(sb)
}

func twd(v float64) string { return "NT$" + render.Commaf(v) }

func twdOrDash(v float64, ok bool) string {
	if !ok {
		return "—"
	}
	return twd(v)
}

func pctOrDash(v float64, ok bool) string {
	if !ok {
		return "—"
	}
	return fmt.Sprintf("%+.1f%%", v)
}

func pctPtrOrDash(v *float64) string {
	if v == nil {
		return "—"
	}
	return fmt.Sprintf("%.1f%%", *v)
}

func monthsPtrOrDash(v *float64) string {
	if v == nil {
		return "—"
	}
	return fmt.Sprintf("%.1f", *v)
}
