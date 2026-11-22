package bot

import (
	"context"
	"fmt"
	"strings"

	"argus/internal/i18n"
	"argus/internal/llm"
	"argus/internal/logger"
	"argus/internal/service"
)

// RunWealthHealthReport is Phase 9 波次3 PR9's monthly financial health
// report (see PLAN.md's "月度財務健康報告（scheduler＋LLM 解讀層），月報帶一行
// 退休目標進度") — a Telegram report with no page of its own (the design
// template's wealth pages have no report page, confirmed 2026-09-18). Reuses service.ComputeHealthMetrics/
// service.ComputeRetirementGoalProgress, the same functions /networth
// already renders, so the numbers can never disagree between the two
// surfaces; the only new work here is an LLM interpretation of
// already-computed numbers (never a second computation path — AGENTS.md's
// "health/financial metrics never go through the LLM"). Unlike
// RunMonthlyReport (deliberately non-LLM, see its own doc comment), this
// report's whole point is the narrative layer — but the deterministic
// numbers still send even if the LLM call fails, since they don't depend
// on it. It also lists the home page's 本月該做的事 (top three) and writes
// the month's AI view (llm.WealthView), which the home page shows too.
func (b *Bot) RunWealthHealthReport(ctx context.Context) {
	defer b.recoverJobPanic("wealth health report")

	now := b.now()
	today := now.Format("2006-01-02")
	month := now.Format("2006-01")

	hm := service.ComputeHealthMetrics(b.db, b.db, b.provider, now)
	progress, haveGoal := service.ComputeRetirementGoalProgress(b.db, b.db, b.provider, today)

	var checks []service.CheckItem
	if b.wealthChecks != nil {
		if c, err := b.wealthChecks(now); err != nil {
			logger.Errorf("wealth health report: checks: %v", err)
		} else {
			checks = c.Items
		}
	}

	if hm.DebtRatioPct == nil && hm.SavingsRatePct == nil && hm.ExpenseRatioPct == nil && hm.LiquidityMonths == nil && !haveGoal && len(checks) == 0 {
		logger.Infof("wealth health report: no computable data for %s, skipping", month)
		return
	}

	in := llm.WealthViewInput{
		Month: month, DebtRatioPct: hm.DebtRatioPct, SavingsRatePct: hm.SavingsRatePct,
		ExpenseRatioPct: hm.ExpenseRatioPct, LiquidityMonths: hm.LiquidityMonths,
	}
	if haveGoal {
		pct := progress.ProgressPct
		in.RetirementGoalName = progress.Name
		in.RetirementProgressPct = &pct
		in.RetirementSaved = progress.Saved
		in.RetirementTarget = progress.TargetAmount
	}
	for _, c := range checks {
		in.Checks = append(in.Checks, b.wealthCheckLine(c))
	}

	// The AI view is stored for the home page's card as well as sent; a
	// failed call keeps last month's view there and sends the numbers anyway.
	summary, points, err := b.llm.WealthView(ctx, in)
	if err != nil {
		logger.Errorf("wealth health report: ai view: %v", err)
		points = nil
	} else if err := service.SaveWealthAIView(b.db, service.WealthAIView{GeneratedAt: today, Summary: summary, Points: points}); err != nil {
		logger.Errorf("wealth health report: save ai view: %v", err)
	}

	var sb strings.Builder
	sb.WriteString(i18n.T(b.lang, i18n.KeyWealthHealthReportTitle, month))
	sb.WriteString(i18n.T(b.lang, i18n.KeyNetworthHealthHeader))
	sb.WriteString(i18n.T(b.lang, i18n.KeyNetworthHealthLine,
		pctPtrOrDash(hm.DebtRatioPct), monthsPtrOrDash(hm.LiquidityMonths),
		pctPtrOrDash(hm.SavingsRatePct), pctPtrOrDash(hm.ExpenseRatioPct)))
	if haveGoal {
		sb.WriteString(i18n.T(b.lang, i18n.KeyWealthHealthRetirementLine,
			progress.Name, fmt.Sprintf("%.1f%%", progress.ProgressPct), twd(progress.Saved), twd(progress.TargetAmount)))
	}
	if len(checks) > 0 {
		sb.WriteString(i18n.T(b.lang, i18n.KeyWealthHealthChecksHeader))
		for i, c := range checks {
			if i == 3 {
				sb.WriteString(i18n.T(b.lang, i18n.KeyWealthHealthChecksMore, len(checks)-3))
				break
			}
			sb.WriteString(i18n.T(b.lang, i18n.KeyWealthHealthCheckLine, severityMark[c.Severity], b.wealthCheckLine(c)))
		}
	}
	if len(points) > 0 {
		sb.WriteString(i18n.T(b.lang, i18n.KeyWealthHealthAIViewHeader))
		for _, p := range points {
			sb.WriteString("• " + p + "\n")
		}
	}
	sb.WriteString(i18n.T(b.lang, i18n.KeyWealthHealthDisclaimer))

	b.Send(sb.String())
}

var severityMark = map[string]string{"do": "🔴", "tip": "🟡", "data": "⚪"}

var wealthCheckTitles = map[string]i18n.Key{
	"emergency":  i18n.KeyWealthCheckTitleEmergency,
	"debtRatio":  i18n.KeyWealthCheckTitleDebtRatio,
	"card":       i18n.KeyWealthCheckTitleCard,
	"coverage":   i18n.KeyWealthCheckTitleCoverage,
	"retirement": i18n.KeyWealthCheckTitleRetirement,
	"goals":      i18n.KeyWealthCheckTitleGoals,
	"alloc":      i18n.KeyWealthCheckTitleAlloc,
	"savings":    i18n.KeyWealthCheckTitleSavings,
	"dependents": i18n.KeyWealthCheckTitleDependents,
	"age":        i18n.KeyWealthCheckTitleAge,
	"salary":     i18n.KeyWealthCheckTitleSalary,
	"stale":      i18n.KeyWealthCheckTitleStale,
}

// wealthCheckLine is one check's title, with its amount when it has one. A
// code with no title here (a rule added without one) shows the code itself.
func (b *Bot) wealthCheckLine(c service.CheckItem) string {
	title := c.Code
	if k, ok := wealthCheckTitles[c.Code]; ok {
		title = i18n.T(b.lang, k)
	}
	if c.Gap != nil && *c.Gap > 0 {
		return i18n.T(b.lang, i18n.KeyWealthHealthCheckGap, title, twd(*c.Gap))
	}
	return title
}
