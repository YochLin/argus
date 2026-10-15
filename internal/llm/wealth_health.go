package llm

import (
	"strings"

	"argus/internal/i18n"
)

// WealthHealthReportInput bundles Phase 9 波次3 PR9's monthly health-report
// numbers — service.ComputeHealthMetrics' four ratios plus
// service.ComputeRetirementGoalProgress' one-line goal progress. This call
// only narrates already-computed numbers, it never computes them itself
// (AGENTS.md's "health/financial metrics never go through the LLM" rule) —
// same division of labor as buildTradeReviewPrompt narrating a realized P&L
// bot.RunSellFollowup already calculated. Every ratio field is a pointer;
// nil means "not computable yet" (no salary set, no history) and the line
// is omitted rather than asking the model to interpret a dash.
type WealthHealthReportInput struct {
	Month                 string
	DebtRatioPct          *float64
	SavingsRatePct        *float64
	ExpenseRatioPct       *float64
	LiquidityMonths       *float64
	RetirementGoalName    string
	RetirementProgressPct *float64
	RetirementSaved       float64
	RetirementTarget      float64
}

// buildWealthHealthReportPrompt is RunWealthHealthReport's prompt — each
// metric line is only rendered when that pointer is non-nil, same
// optional-section convention as buildTradeReviewPrompt's VsSPY/Thesis
// blocks.
func buildWealthHealthReportPrompt(lang i18n.Lang, in WealthHealthReportInput) string {
	var sb strings.Builder
	sb.WriteString(i18n.T(lang, i18n.KeyWealthHealthPromptIntro, in.Month))

	if in.DebtRatioPct != nil {
		sb.WriteString(i18n.T(lang, i18n.KeyWealthHealthPromptDebtRatioLine, *in.DebtRatioPct))
	}
	if in.LiquidityMonths != nil {
		sb.WriteString(i18n.T(lang, i18n.KeyWealthHealthPromptLiquidityLine, *in.LiquidityMonths))
	}
	if in.SavingsRatePct != nil {
		sb.WriteString(i18n.T(lang, i18n.KeyWealthHealthPromptSavingsRateLine, *in.SavingsRatePct))
	}
	if in.ExpenseRatioPct != nil {
		sb.WriteString(i18n.T(lang, i18n.KeyWealthHealthPromptExpenseRatioLine, *in.ExpenseRatioPct))
	}
	if in.RetirementProgressPct != nil {
		sb.WriteString(i18n.T(lang, i18n.KeyWealthHealthPromptRetirementLine, in.RetirementGoalName, *in.RetirementProgressPct, in.RetirementSaved, in.RetirementTarget))
	}

	sb.WriteString(i18n.T(lang, i18n.KeyWealthHealthPromptTask))
	return sb.String()
}
