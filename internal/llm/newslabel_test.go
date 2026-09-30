package llm

import (
	"strings"
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/i18n"
)

func TestParseNewsLabels(t *testing.T) {
	raw := `Here you go:
1. sentiment=bull tag=earn major=1
2) sentiment=BEAR tag=weird major=0
3. sentiment=maybe tag=macro major=1
4. sentiment=neutral tag=analyst major=0
4. sentiment=bull tag=earn major=1
9. sentiment=bull tag=earn major=1
garbage line`
	got := parseNewsLabels(raw, 5)
	want := []NewsLabel{
		{"bull", "earn", true},
		{"bear", "other", false},      // unknown tag falls back to other
		{},                            // unknown sentiment: not classified, not "neutral"
		{"neutral", "analyst", false}, // first line for a number wins
		{},                            // never mentioned; index 9 is out of range
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("item %d: got %+v, want %+v", i+1, got[i], want[i])
		}
	}
}

func TestBuildNewsClassifyPrompt(t *testing.T) {
	news := []data.NewsItem{{Headline: "NVDA beats", Source: "Reuters", PublishedAt: time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)}}
	for _, lang := range []i18n.Lang{i18n.ZH, i18n.EN} {
		p := buildNewsClassifyPrompt(lang, "NVDA", news)
		if !strings.Contains(p, "NVDA beats") || !strings.Contains(p, newsLabelFormat) {
			t.Errorf("%v prompt missing headline or format line:\n%s", lang, p)
		}
	}
}
