package db

import "testing"

func TestSavePodcastInsight(t *testing.T) {
	d := newTestDB(t)

	if err := d.SavePodcastInsight(PodcastInsight{
		SourceURL:   "https://vocus.cc/article/example",
		SourceTitle: "EP692 逐字稿",
		Ticker:      "NVDA",
		Market:      "US",
		Stance:      "BULLISH",
		Thesis:      "AI 資本支出還在加速",
	}); err != nil {
		t.Fatalf("SavePodcastInsight() error = %v", err)
	}

	// Macro-only row: no ticker/market attached.
	if err := d.SavePodcastInsight(PodcastInsight{
		SourceURL:   "https://vocus.cc/article/example",
		SourceTitle: "EP692 逐字稿",
		Stance:      "NEUTRAL",
		Thesis:      "整體盤勢忽高忽低",
	}); err != nil {
		t.Fatalf("SavePodcastInsight() macro-only error = %v", err)
	}

	var count int
	if err := d.conn.QueryRow(`SELECT COUNT(*) FROM podcast_insights WHERE source_url = ?`, "https://vocus.cc/article/example").Scan(&count); err != nil {
		t.Fatalf("count query error = %v", err)
	}
	if count != 2 {
		t.Fatalf("row count = %d, want 2", count)
	}

	var ticker, stance, thesis string
	if err := d.conn.QueryRow(`SELECT ticker, stance, thesis FROM podcast_insights WHERE ticker = ?`, "NVDA").Scan(&ticker, &stance, &thesis); err != nil {
		t.Fatalf("select NVDA row error = %v", err)
	}
	if ticker != "NVDA" || stance != "BULLISH" || thesis != "AI 資本支出還在加速" {
		t.Errorf("NVDA row = (%q, %q, %q), want (NVDA, BULLISH, AI 資本支出還在加速)", ticker, stance, thesis)
	}

	var macroCount int
	if err := d.conn.QueryRow(`SELECT COUNT(*) FROM podcast_insights WHERE ticker = ''`).Scan(&macroCount); err != nil {
		t.Fatalf("macro count query error = %v", err)
	}
	if macroCount != 1 {
		t.Fatalf("macro row count = %d, want 1", macroCount)
	}

	var derivedFrom string
	if err := d.conn.QueryRow(`SELECT derived_from FROM podcast_insights WHERE ticker = ?`, "NVDA").Scan(&derivedFrom); err != nil {
		t.Fatalf("select NVDA derived_from error = %v", err)
	}
	if derivedFrom != "" {
		t.Errorf("NVDA derived_from = %q, want empty (directly mentioned, not inferred)", derivedFrom)
	}

	// A downstream-beneficiary row inferred from the model's own knowledge,
	// not something the transcript named directly.
	if err := d.SavePodcastInsight(PodcastInsight{
		SourceURL:   "https://vocus.cc/article/example",
		SourceTitle: "EP692 逐字稿",
		Ticker:      "2330",
		Market:      "TW",
		Stance:      "BULLISH",
		Thesis:      "晶圓代工受惠 AI 需求成長",
		DerivedFrom: "NVDA: GPU 需求成長",
	}); err != nil {
		t.Fatalf("SavePodcastInsight() derived row error = %v", err)
	}
	if err := d.conn.QueryRow(`SELECT derived_from FROM podcast_insights WHERE ticker = ?`, "2330").Scan(&derivedFrom); err != nil {
		t.Fatalf("select 2330 derived_from error = %v", err)
	}
	if derivedFrom != "NVDA: GPU 需求成長" {
		t.Errorf("2330 derived_from = %q, want %q", derivedFrom, "NVDA: GPU 需求成長")
	}
}

func TestCountPodcastInsightsByURL(t *testing.T) {
	d := newTestDB(t)

	if count, err := d.CountPodcastInsightsByURL("https://vocus.cc/article/example"); err != nil {
		t.Fatalf("CountPodcastInsightsByURL() before any save error = %v", err)
	} else if count != 0 {
		t.Fatalf("CountPodcastInsightsByURL() before any save = %d, want 0", count)
	}

	for range 2 {
		if err := d.SavePodcastInsight(PodcastInsight{
			SourceURL: "https://vocus.cc/article/example",
			Stance:    "NEUTRAL",
			Thesis:    "filler",
		}); err != nil {
			t.Fatalf("SavePodcastInsight() error = %v", err)
		}
	}
	if err := d.SavePodcastInsight(PodcastInsight{
		SourceURL: "https://vocus.cc/article/other",
		Stance:    "NEUTRAL",
		Thesis:    "unrelated episode",
	}); err != nil {
		t.Fatalf("SavePodcastInsight() unrelated episode error = %v", err)
	}

	if count, err := d.CountPodcastInsightsByURL("https://vocus.cc/article/example"); err != nil {
		t.Fatalf("CountPodcastInsightsByURL() error = %v", err)
	} else if count != 2 {
		t.Fatalf("CountPodcastInsightsByURL() = %d, want 2", count)
	}
	if count, err := d.CountPodcastInsightsByURL("https://vocus.cc/article/other"); err != nil {
		t.Fatalf("CountPodcastInsightsByURL() (other) error = %v", err)
	} else if count != 1 {
		t.Fatalf("CountPodcastInsightsByURL() (other) = %d, want 1", count)
	}
}
