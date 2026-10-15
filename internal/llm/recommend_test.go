package llm

import (
	"strings"
	"testing"

	"argus/internal/i18n"
)

func TestBuildRecommendationPromptRendersRecentLessons(t *testing.T) {
	prompt := buildRecommendationPrompt(i18n.EN, nil, nil, nil, nil, []PastLesson{
		{Ticker: "NVDA", Date: "2026-06-20", Lesson: "chased a breakout too late"},
	}, false)
	if !strings.Contains(prompt, "NVDA") || !strings.Contains(prompt, "chased a breakout too late") {
		t.Errorf("buildRecommendationPrompt() missing recent-lessons content, got:\n%s", prompt)
	}
}

func TestBuildRecommendationPromptOmitsRecentLessonsWhenEmpty(t *testing.T) {
	prompt := buildRecommendationPrompt(i18n.EN, nil, nil, nil, nil, nil, false)
	if strings.Contains(prompt, "Recent Trade Lessons") {
		t.Errorf("buildRecommendationPrompt() should omit recent-lessons block when empty, got:\n%s", prompt)
	}
}

func TestBuildRecommendationPromptUsesFocusedOutputRules(t *testing.T) {
	prompt := buildRecommendationPrompt(i18n.EN, nil, nil, nil, nil, nil, false)
	for _, want := range []string{"Position:", "worth buying now", "omit the entire block"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("buildRecommendationPrompt() missing focused-output rule %q, got:\n%s", want, prompt)
		}
	}
}
