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
}
