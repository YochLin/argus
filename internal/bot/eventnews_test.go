package bot

import (
	"testing"
	"time"

	"argus/internal/data"
)

func TestFilterStaleNews(t *testing.T) {
	at := func(s string) time.Time {
		ts, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return ts
	}
	asOf := at("2026-08-27T09:00:00+08:00")
	news := []data.NewsItem{
		{Headline: "today", PublishedAt: at("2026-08-27T08:00:00+08:00")},
		{Headline: "just under 72h", PublishedAt: at("2026-08-24T10:00:00+08:00")},
		{Headline: "just over 72h", PublishedAt: at("2026-08-24T08:00:00+08:00")},
		{Headline: "a week old", PublishedAt: at("2026-08-18T10:00:00+08:00")},
		{Headline: "undated"},
	}

	got := filterStaleNews(news, asOf)
	want := []string{"today", "just under 72h"}
	if len(got) != len(want) {
		t.Fatalf("kept %d items %v, want %v", len(got), headlines(got), want)
	}
	for i, h := range want {
		if got[i].Headline != h {
			t.Errorf("item %d = %q, want %q", i, got[i].Headline, h)
		}
	}
}

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

	p := &newsPicker{}
	got := p.pick(news, tickerNewsSlots)
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
	if got := p.pick(next, tickerNewsSlots); len(got) != 1 || got[0].Source != "中央社" {
		t.Errorf("second pick = %v, want only the story the first ticker had not shown", headlines(got))
	}

	// slots caps the answer even when everything is distinct.
	if got := (&newsPicker{}).pick(news, 2); len(got) != 2 {
		t.Errorf("pick(news, 2) kept %d items, want 2", len(got))
	}

	if got := (&newsPicker{}).pick(nil, tickerNewsSlots); got != nil {
		t.Errorf("pick(nil) = %v, want nil", got)
	}
}

func headlines(news []data.NewsItem) []string {
	out := make([]string, len(news))
	for i, n := range news {
		out[i] = n.Headline
	}
	return out
}
