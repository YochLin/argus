package llm

import (
	"strings"
	"testing"

	"argus/internal/i18n"
)

func healthFloatPtr(f float64) *float64 { return &f }

func TestBuildWealthHealthReportPromptOmitsNilMetrics(t *testing.T) {
	in := WealthHealthReportInput{
		Month:          "2026-09",
		DebtRatioPct:   healthFloatPtr(35.2),
		SavingsRatePct: healthFloatPtr(28.4),
		// ExpenseRatioPct and LiquidityMonths left nil — not computable yet.
	}

	prompt := buildWealthHealthReportPrompt(i18n.EN, in)

	if !strings.Contains(prompt, "2026-09") {
		t.Errorf("buildWealthHealthReportPrompt() missing month, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "35.2") {
		t.Errorf("buildWealthHealthReportPrompt() missing debt ratio, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "28.4") {
		t.Errorf("buildWealthHealthReportPrompt() missing savings rate, got:\n%s", prompt)
	}
	if strings.Contains(prompt, "Expense ratio") {
		t.Errorf("buildWealthHealthReportPrompt() should omit expense ratio line when nil, got:\n%s", prompt)
	}
	if strings.Contains(prompt, "Liquidity") {
		t.Errorf("buildWealthHealthReportPrompt() should omit liquidity line when nil, got:\n%s", prompt)
	}
	if strings.Contains(prompt, "Retirement goal") {
		t.Errorf("buildWealthHealthReportPrompt() should omit retirement line when nil, got:\n%s", prompt)
	}
}

func TestBuildWealthHealthReportPromptIncludesRetirementLine(t *testing.T) {
	in := WealthHealthReportInput{
		Month:                 "2026-09",
		RetirementGoalName:    "退休",
		RetirementProgressPct: healthFloatPtr(65.5),
		RetirementSaved:       5200000,
		RetirementTarget:      8000000,
	}

	prompt := buildWealthHealthReportPrompt(i18n.ZH, in)

	for _, want := range []string{"退休", "65.5", "5200000", "8000000"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("buildWealthHealthReportPrompt() missing %q, got:\n%s", want, prompt)
		}
	}
}
