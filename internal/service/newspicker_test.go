package service

import (
	"testing"

	"argus/internal/data"
)

func TestNewsPicker(t *testing.T) {
	news := []data.NewsItem{
		// The TW case: one wire story, three outlets, Google News' " - 媒體"
		// tag being the only difference.
		{Source: "經濟日報", Headline: "台積電法說會釋出樂觀展望 外資調高目標價 - 經濟日報"},
		{Source: "工商時報", Headline: "台積電法說會釋出樂觀展望 外資調高目標價 - 工商時報"},
		{Source: "鉅亨網", Headline: "台積電法說會釋出樂觀展望，外資調高目標價"},
		// Same company, genuinely different story — must survive.
		{Source: "中央社", Headline: "台積電宣布高雄二廠動工時程"},
		{Source: "Reuters", Headline: "TSMC lifts capex guidance for 2026"},
		{Source: "Bloomberg", Headline: "TSMC lifts capex guidance for 2026 - Bloomberg"},
	}

	const slots = 5 // mirrors internal/bot's tickerNewsSlots

	p := &NewsPicker{}
	got := p.Pick(news, slots)
	want := []string{"經濟日報", "中央社", "Reuters"}
	if len(got) != len(want) {
		t.Fatalf("kept %d items %v, want %v", len(got), headlines(got), want)
	}
	for i, src := range want {
		if got[i].Source != src {
			t.Errorf("item %d from %q, want the first copy from %q", i, got[i].Source, src)
		}
	}

	// The cross-ticker case: the same picker, the next ticker's feed. The
	// market-wide story the first ticker already used is skipped, and the
	// slot goes to the next distinct one instead of being lost.
	next := []data.NewsItem{
		{Source: "Reuters", Headline: "TSMC lifts capex guidance for 2026"},
		{Source: "中央社", Headline: "聯發科天璣新平台發表"},
	}
	if got := p.Pick(next, slots); len(got) != 1 || got[0].Source != "中央社" {
		t.Errorf("second pick = %v, want only the story the first ticker had not shown", headlines(got))
	}

	// slots caps the answer even when everything is distinct.
	if got := (&NewsPicker{}).Pick(news, 2); len(got) != 2 {
		t.Errorf("pick(news, 2) kept %d items, want 2", len(got))
	}

	if got := (&NewsPicker{}).Pick(nil, slots); got != nil {
		t.Errorf("pick(nil) = %v, want nil", got)
	}
}
