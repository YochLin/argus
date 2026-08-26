package bot

import (
	"testing"
	"time"

	"argus/internal/data"
)

func TestFilterNewsNearDate(t *testing.T) {
	at := func(s string) time.Time {
		ts, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return ts
	}
	news := []data.NewsItem{
		{Headline: "same day", PublishedAt: at("2026-08-25T14:00:00+08:00")},
		{Headline: "us session, next cst day", PublishedAt: at("2026-08-26T03:30:00+08:00")},
		{Headline: "day before", PublishedAt: at("2026-08-24T09:00:00+08:00")},
		{Headline: "two days old", PublishedAt: at("2026-08-23T23:59:00+08:00")},
		{Headline: "a week old", PublishedAt: at("2026-08-18T10:00:00+08:00")},
		{Headline: "undated"},
	}

	got := filterNewsNearDate(news, "2026-08-25")
	want := []string{"same day", "us session, next cst day", "day before"}
	if len(got) != len(want) {
		t.Fatalf("kept %d items %v, want %v", len(got), headlines(got), want)
	}
	for i, h := range want {
		if got[i].Headline != h {
			t.Errorf("item %d = %q, want %q", i, got[i].Headline, h)
		}
	}

	if got := filterNewsNearDate(news, "not-a-date"); len(got) != len(news) {
		t.Errorf("unparseable date filtered %d items away, want the news left untouched", len(news)-len(got))
	}
	if got := filterNewsNearDate(news, "2026-01-05"); len(got) != 0 {
		t.Errorf("kept %v for an unrelated date, want none (the prompt's 'cause unknown' branch)", headlines(got))
	}
}

func headlines(news []data.NewsItem) []string {
	out := make([]string, len(news))
	for i, n := range news {
		out[i] = n.Headline
	}
	return out
}
