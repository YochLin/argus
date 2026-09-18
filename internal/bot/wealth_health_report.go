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
// 退休目標進度") — a Telegram-only report, deliberately with no matching web
// page (the design template's eleven wealth pages have no report page,
// confirmed 2026-09-18). Reuses service.ComputeHealthMetrics/
// service.ComputeRetirementGoalProgress, the same functions /networth
// already renders, so the numbers can never disagree between the two
// surfaces; the only new work here is an LLM interpretation of
// already-computed numbers (never a second computation path — AGENTS.md's
// "health/financial metrics never go through the LLM"). Unlike
// RunMonthlyReport (deliberately non-LLM, see its own doc comment), this
// report's whole point is the narrative layer — but the deterministic
// numbers still send even if the LLM call fails, since they don't depend
// on it.
func (b *Bot) RunWealthHealthReport(ctx context.Context) {
	defer b.recoverJobPanic("wealth health report")

	now := b.now()
	today := now.Format("2006-01-02")
	month := now.Format("2006-01")

	hm := service.ComputeHealthMetrics(b.db, b.db, b.provider, now)
	progress, haveGoal := service.ComputeRetirementGoalProgress(b.db, b.db, b.provider, today)

	if hm.DebtRatioPct == nil && hm.SavingsRatePct == nil && hm.ExpenseRatioPct == nil && hm.LiquidityMonths == nil && !haveGoal {
		logger.Infof("wealth health report: no computable data for %s, skipping", month)
		return
	}

	in := llm.WealthHealthReportInput{
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

	insight, err := b.llm.WealthHealthReport(ctx, in)
	if err != nil {
		logger.Errorf("wealth health report: llm: %v", err)
		insight = ""
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
	if insight != "" {
		sb.WriteString(i18n.T(b.lang, i18n.KeyWealthHealthInsightHeader))
		sb.WriteString(insight)
	}

	b.Send(sb.String())
}
