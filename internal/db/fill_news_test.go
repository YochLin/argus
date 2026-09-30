package db

import (
	"testing"
	"time"
)

func TestFillNews_SaveDedupeAndLabel(t *testing.T) {
	d := newTestDB(t)
	t1 := time.Date(2026, 9, 30, 13, 0, 0, 0, time.UTC)
	items := []FillNews{
		{Headline: "B later", Source: "Reuters", URL: "https://x/b", PublishedAt: t1.Add(time.Hour)},
		{Headline: "A earlier", Source: "WSJ", URL: "https://x/a", PublishedAt: t1},
		{Headline: "undated", Source: "cnyes"},
	}
	if err := d.SaveFillNews("NVDA", "2026-09-30", items); err != nil {
		t.Fatal(err)
	}
	got, err := d.FillNewsFor("NVDA", "2026-09-30")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Headline != "A earlier" || got[1].Headline != "B later" || got[2].Headline != "undated" {
		t.Fatalf("want published order with undated last, got %+v", got)
	}
	if got[0].URL != "https://x/a" || !got[0].PublishedAt.Equal(t1) || !got[2].PublishedAt.IsZero() {
		t.Fatalf("url/time round trip wrong: %+v", got)
	}

	// Labelling survives a second capture of the same headlines.
	if err := d.SetFillNewsLabel(got[0].ID, "bull", "earn", true); err != nil {
		t.Fatal(err)
	}
	if err := d.SaveFillNews("NVDA", "2026-09-30", items); err != nil {
		t.Fatal(err)
	}
	again, _ := d.FillNewsFor("NVDA", "2026-09-30")
	if len(again) != 3 {
		t.Fatalf("re-save duplicated rows: %d", len(again))
	}
	if a := again[0]; a.Sentiment != "bull" || a.Tag != "earn" || !a.Major {
		t.Fatalf("label lost on re-save: %+v", a)
	}
	if again[1].Sentiment != "" {
		t.Fatalf("unclassified row should stay empty: %+v", again[1])
	}

	// Another day or ticker is a separate set.
	if other, _ := d.FillNewsFor("NVDA", "2026-10-01"); len(other) != 0 {
		t.Fatalf("other day leaked: %+v", other)
	}
}
