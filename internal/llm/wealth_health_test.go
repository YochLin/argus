package llm

import (
	"strings"
	"testing"

	"argus/internal/i18n"
)

func healthFloatPtr(f float64) *float64 { return &f }

func TestBuildWealthHealthReportPromptOmitsNilMetrics(t *testing.T) {
	in := WealthViewInput{
		Month:          "2026-09",
		DebtRatioPct:   healthFloatPtr(35.2),
		SavingsRatePct: healthFloatPtr(28.4),
		// ExpenseRatioPct and LiquidityMonths left nil — not computable yet.
	}

	prompt := buildWealthViewPrompt(i18n.EN, in)

	if !strings.Contains(prompt, "2026-09") {
		t.Errorf("buildWealthViewPrompt() missing month, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "35.2") {
		t.Errorf("buildWealthViewPrompt() missing debt ratio, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "28.4") {
		t.Errorf("buildWealthViewPrompt() missing savings rate, got:\n%s", prompt)
	}
	if strings.Contains(prompt, "Expense ratio") {
		t.Errorf("buildWealthViewPrompt() should omit expense ratio line when nil, got:\n%s", prompt)
	}
	if strings.Contains(prompt, "Liquidity") {
		t.Errorf("buildWealthViewPrompt() should omit liquidity line when nil, got:\n%s", prompt)
	}
	if strings.Contains(prompt, "Retirement goal") {
		t.Errorf("buildWealthViewPrompt() should omit retirement line when nil, got:\n%s", prompt)
	}
}

func TestBuildWealthHealthReportPromptIncludesRetirementLine(t *testing.T) {
	in := WealthViewInput{
		Month:                 "2026-09",
		RetirementGoalName:    "退休",
		RetirementProgressPct: healthFloatPtr(65.5),
		RetirementSaved:       5200000,
		RetirementTarget:      8000000,
	}

	prompt := buildWealthViewPrompt(i18n.ZH, in)

	for _, want := range []string{"退休", "65.5", "5200000", "8000000"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("buildWealthViewPrompt() missing %q, got:\n%s", want, prompt)
		}
	}
}

func TestBuildWealthViewPromptListsChecksAndMarker(t *testing.T) {
	prompt := buildWealthViewPrompt(i18n.ZH, WealthViewInput{Month: "2026-11", Checks: []string{"補足緊急預備金（NT$84,000）"}})
	for _, want := range []string{"補足緊急預備金（NT$84,000）", WealthViewSummaryMarker, "propose_", "稅務頁"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q, got:\n%s", want, prompt)
		}
	}
}

func TestParseWealthView(t *testing.T) {
	summary, points, err := parseWealthView("好的，以下是觀察：\n[SUMMARY] 保單重複、不動產集中\n\n- 兩張醫療險都附日額\n• 不動產占 44%\n-\n")
	if err != nil || summary != "保單重複、不動產集中" || len(points) != 2 || points[1] != "不動產占 44%" {
		t.Fatalf("parseWealthView() = %q, %q, %v", summary, points, err)
	}
	if _, _, err := parseWealthView("[SUMMARY] 沒什麼好說的"); err == nil {
		t.Error("parseWealthView() with no points should fail")
	}
}
