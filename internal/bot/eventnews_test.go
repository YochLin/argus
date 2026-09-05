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

func headlines(news []data.NewsItem) []string {
	out := make([]string, len(news))
	for i, n := range news {
		out[i] = n.Headline
	}
	return out
}
