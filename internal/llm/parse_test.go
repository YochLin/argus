package llm

import (
	"reflect"
	"testing"

	"argus/internal/i18n"
)

func TestParseRecommendations(t *testing.T) {
	t.Run("english single ticker single-line reason", func(t *testing.T) {
		raw := "[TICKER: AAPL]\nReason: strong earnings and margin expansion.\n"
		got := parseRecommendations(i18n.EN, raw)
		want := []Recommendation{{Ticker: "AAPL", Reason: "strong earnings and margin expansion."}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("parseRecommendations() = %+v, want %+v", got, want)
		}
	})

	t.Run("chinese single ticker uses the zh marker", func(t *testing.T) {
		raw := "[TICKER: 2330.TW]\n原因: 營收成長強勁。\n"
		got := parseRecommendations(i18n.ZH, raw)
		want := []Recommendation{{Ticker: "2330.TW", Reason: "營收成長強勁。"}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("parseRecommendations() = %+v, want %+v", got, want)
		}
	})

	t.Run("multiple tickers", func(t *testing.T) {
		raw := "[TICKER: AAPL]\nReason: strong earnings.\n[TICKER: MSFT]\nReason: cloud growth.\n"
		got := parseRecommendations(i18n.EN, raw)
		want := []Recommendation{
			{Ticker: "AAPL", Reason: "strong earnings."},
			{Ticker: "MSFT", Reason: "cloud growth."},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("parseRecommendations() = %+v, want %+v", got, want)
		}
	})

	t.Run("reason wraps across multiple lines", func(t *testing.T) {
		raw := "[TICKER: AAPL]\nReason: strong earnings\nand continued margin expansion.\n"
		got := parseRecommendations(i18n.EN, raw)
		want := []Recommendation{{Ticker: "AAPL", Reason: "strong earnings and continued margin expansion."}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("parseRecommendations() = %+v, want %+v", got, want)
		}
	})

	t.Run("wrong-language marker is not recognized", func(t *testing.T) {
		// Regression guard for the failure CLAUDE.md calls out: if the
		// prompt's language and the parser's expected marker ever drift
		// apart, recommendations silently parse as ticker-with-no-reason
		// instead of erroring loudly.
		raw := "[TICKER: AAPL]\n原因: 這是中文原因。\n"
		got := parseRecommendations(i18n.EN, raw)
		want := []Recommendation{{Ticker: "AAPL", Reason: ""}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("parseRecommendations() = %+v, want %+v", got, want)
		}
	})

	t.Run("action line is parsed and normalized", func(t *testing.T) {
		raw := "[TICKER: AAPL]\nAction: buy\nReason: strong earnings.\n[TICKER: MSFT]\nAction: HOLD\nReason: fairly valued.\n"
		got := parseRecommendations(i18n.EN, raw)
		want := []Recommendation{
			{Ticker: "AAPL", Action: "BUY", Reason: "strong earnings."},
			{Ticker: "MSFT", Action: "HOLD", Reason: "fairly valued."},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("parseRecommendations() = %+v, want %+v", got, want)
		}
	})

	t.Run("chinese action marker", func(t *testing.T) {
		raw := "[TICKER: AAPL]\n動作: SELL\n原因: 估值過高。\n"
		got := parseRecommendations(i18n.ZH, raw)
		want := []Recommendation{{Ticker: "AAPL", Action: "SELL", Reason: "估值過高。"}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("parseRecommendations() = %+v, want %+v", got, want)
		}
	})

	t.Run("missing or invalid action leaves Action empty", func(t *testing.T) {
		raw := "[TICKER: AAPL]\nReason: no action line.\n[TICKER: MSFT]\nAction: MAYBE\nReason: made-up action word.\n"
		got := parseRecommendations(i18n.EN, raw)
		want := []Recommendation{
			{Ticker: "AAPL", Action: "", Reason: "no action line."},
			{Ticker: "MSFT", Action: "", Reason: "made-up action word."},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("parseRecommendations() = %+v, want %+v", got, want)
		}
	})

	t.Run("no ticker blocks yields no recommendations", func(t *testing.T) {
		got := parseRecommendations(i18n.EN, "just some prose with no structure")
		if len(got) != 0 {
			t.Errorf("parseRecommendations() = %+v, want empty", got)
		}
	})

	t.Run("empty input yields no recommendations", func(t *testing.T) {
		got := parseRecommendations(i18n.EN, "")
		if len(got) != 0 {
			t.Errorf("parseRecommendations() = %+v, want empty", got)
		}
	})
}

func TestParseExploreNominations(t *testing.T) {
	t.Run("english single nomination", func(t *testing.T) {
		raw := "[EXPLORE: NVDA]\nReason: named in a supply-chain story about AI chip demand.\n"
		got := parseExploreNominations(i18n.EN, raw)
		want := []ExploreNomination{{Ticker: "NVDA", Reason: "named in a supply-chain story about AI chip demand."}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("parseExploreNominations() = %+v, want %+v", got, want)
		}
	})

	t.Run("chinese marker and reason", func(t *testing.T) {
		raw := "[EXPLORE: 2454.TW]\n原因: 供應鏈新聞點名的二線受惠股。\n"
		got := parseExploreNominations(i18n.ZH, raw)
		want := []ExploreNomination{{Ticker: "2454.TW", Reason: "供應鏈新聞點名的二線受惠股。"}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("parseExploreNominations() = %+v, want %+v", got, want)
		}
	})

	t.Run("no marker yields no nominations", func(t *testing.T) {
		got := parseExploreNominations(i18n.EN, "just some prose with no structure")
		if len(got) != 0 {
			t.Errorf("parseExploreNominations() = %+v, want empty", got)
		}
	})

	t.Run("ticker normalized: trimmed, upper-cased, leading $ stripped", func(t *testing.T) {
		raw := "[EXPLORE:  $nvda ]\nReason: lowercase and dollar-prefixed by the model.\n"
		got := parseExploreNominations(i18n.EN, raw)
		want := []ExploreNomination{{Ticker: "NVDA", Reason: "lowercase and dollar-prefixed by the model."}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("parseExploreNominations() = %+v, want %+v", got, want)
		}
	})

	t.Run("more than maxExploreNominations is truncated", func(t *testing.T) {
		raw := "[EXPLORE: AAA]\nReason: one.\n[EXPLORE: BBB]\nReason: two.\n[EXPLORE: CCC]\nReason: three.\n[EXPLORE: DDD]\nReason: four.\n"
		got := parseExploreNominations(i18n.EN, raw)
		if len(got) != maxExploreNominations {
			t.Fatalf("parseExploreNominations() returned %d nominations, want %d", len(got), maxExploreNominations)
		}
		want := []ExploreNomination{
			{Ticker: "AAA", Reason: "one."},
			{Ticker: "BBB", Reason: "two."},
			{Ticker: "CCC", Reason: "three."},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("parseExploreNominations() = %+v, want %+v", got, want)
		}
	})

	t.Run("empty input yields no nominations", func(t *testing.T) {
		got := parseExploreNominations(i18n.EN, "")
		if len(got) != 0 {
			t.Errorf("parseExploreNominations() = %+v, want empty", got)
		}
	})
}

func TestParsePodcastInsights(t *testing.T) {
	t.Run("english single ticker", func(t *testing.T) {
		raw := "[TICKER: NVDA]\nMarket: US\nStance: BULLISH\nReason: AI capex still accelerating.\n"
		got := parsePodcastInsights(i18n.EN, raw)
		want := []PodcastInsight{{Ticker: "NVDA", Market: "US", Stance: "BULLISH", Thesis: "AI capex still accelerating."}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("parsePodcastInsights() = %+v, want %+v", got, want)
		}
	})

	t.Run("chinese markers", func(t *testing.T) {
		raw := "[TICKER: 2330]\n市場: TW\n觀點: BEARISH\n原因: 法說保守，訂單能見度下修。\n"
		got := parsePodcastInsights(i18n.ZH, raw)
		want := []PodcastInsight{{Ticker: "2330", Market: "TW", Stance: "BEARISH", Thesis: "法說保守，訂單能見度下修。"}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("parsePodcastInsights() = %+v, want %+v", got, want)
		}
	})

	t.Run("macro-only block has empty ticker and market but is still kept", func(t *testing.T) {
		raw := "[TICKER: ]\nMarket:\nStance: NEUTRAL\nReason: Overall market chopping sideways today.\n"
		got := parsePodcastInsights(i18n.EN, raw)
		want := []PodcastInsight{{Ticker: "", Market: "", Stance: "NEUTRAL", Thesis: "Overall market chopping sideways today."}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("parsePodcastInsights() = %+v, want %+v", got, want)
		}
	})

	t.Run("multiple blocks including a macro one", func(t *testing.T) {
		raw := "[TICKER: NVDA]\nMarket: US\nStance: BULLISH\nReason: capex cycle.\n[TICKER: ]\nMarket:\nStance: WATCH\nReason: Fed decision next week.\n"
		got := parsePodcastInsights(i18n.EN, raw)
		want := []PodcastInsight{
			{Ticker: "NVDA", Market: "US", Stance: "BULLISH", Thesis: "capex cycle."},
			{Ticker: "", Market: "", Stance: "WATCH", Thesis: "Fed decision next week."},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("parsePodcastInsights() = %+v, want %+v", got, want)
		}
	})

	t.Run("invalid stance leaves Stance empty rather than a made-up word", func(t *testing.T) {
		raw := "[TICKER: NVDA]\nMarket: US\nStance: SUPER BULLISH\nReason: enthusiastic host.\n"
		got := parsePodcastInsights(i18n.EN, raw)
		want := []PodcastInsight{{Ticker: "NVDA", Market: "US", Stance: "", Thesis: "enthusiastic host."}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("parsePodcastInsights() = %+v, want %+v", got, want)
		}
	})

	t.Run("no ticker blocks yields no insights", func(t *testing.T) {
		got := parsePodcastInsights(i18n.EN, "just ad-read chatter, nothing market-relevant")
		if len(got) != 0 {
			t.Errorf("parsePodcastInsights() = %+v, want empty", got)
		}
	})

	t.Run("empty input yields no insights", func(t *testing.T) {
		got := parsePodcastInsights(i18n.EN, "")
		if len(got) != 0 {
			t.Errorf("parsePodcastInsights() = %+v, want empty", got)
		}
	})

	t.Run("downstream-beneficiary block carries DerivedFrom", func(t *testing.T) {
		raw := "[TICKER: NVDA]\nMarket: US\nStance: BULLISH\nReason: GPU demand accelerating.\n" +
			"[TICKER: 2330]\nMarket: TW\nStance: BULLISH\nReason: foundry capacity fully booked.\nDerived from: NVDA: GPU demand\n"
		got := parsePodcastInsights(i18n.EN, raw)
		want := []PodcastInsight{
			{Ticker: "NVDA", Market: "US", Stance: "BULLISH", Thesis: "GPU demand accelerating."},
			{Ticker: "2330", Market: "TW", Stance: "BULLISH", Thesis: "foundry capacity fully booked.", DerivedFrom: "NVDA: GPU demand"},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("parsePodcastInsights() = %+v, want %+v", got, want)
		}
	})

	t.Run("chinese derived-from marker", func(t *testing.T) {
		raw := "[TICKER: 2330]\n市場: TW\n觀點: BULLISH\n原因: 晶圓代工受惠 AI 需求成長。\n推論自: NVDA：GPU 需求成長\n"
		got := parsePodcastInsights(i18n.ZH, raw)
		want := []PodcastInsight{{Ticker: "2330", Market: "TW", Stance: "BULLISH", Thesis: "晶圓代工受惠 AI 需求成長。", DerivedFrom: "NVDA：GPU 需求成長"}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("parsePodcastInsights() = %+v, want %+v", got, want)
		}
	})

	t.Run("more than maxPodcastInsights is truncated", func(t *testing.T) {
		var raw string
		for i := 0; i < maxPodcastInsights+5; i++ {
			raw += "[TICKER: AAA]\nMarket: US\nStance: WATCH\nReason: filler.\n"
		}
		got := parsePodcastInsights(i18n.EN, raw)
		if len(got) != maxPodcastInsights {
			t.Fatalf("parsePodcastInsights() returned %d insights, want %d", len(got), maxPodcastInsights)
		}
	})
}

func TestParseMarketSummary(t *testing.T) {
	marker := "[MARKET SUMMARY]"

	t.Run("summary present, ticker blocks follow", func(t *testing.T) {
		raw := "[MARKET SUMMARY]\n- Fed signals a pause.\n- Oil prices climb.\n\n[TICKER: AAPL]\nReason: strong earnings.\n"
		got := parseMarketSummary(raw, marker)
		want := "- Fed signals a pause.\n- Oil prices climb."
		if got != want {
			t.Errorf("parseMarketSummary() = %q, want %q", got, want)
		}
	})

	t.Run("marker absent (Finnhub not configured / model omitted it) yields empty", func(t *testing.T) {
		raw := "[TICKER: AAPL]\nReason: strong earnings.\n"
		if got := parseMarketSummary(raw, marker); got != "" {
			t.Errorf("parseMarketSummary() = %q, want empty", got)
		}
	})

	t.Run("marker present with no ticker blocks extracts to end of string", func(t *testing.T) {
		raw := "[MARKET SUMMARY]\n- Only macro news today, no picks.\n"
		got := parseMarketSummary(raw, marker)
		want := "- Only macro news today, no picks."
		if got != want {
			t.Errorf("parseMarketSummary() = %q, want %q", got, want)
		}
	})

	t.Run("empty input yields empty summary", func(t *testing.T) {
		if got := parseMarketSummary("", marker); got != "" {
			t.Errorf("parseMarketSummary() = %q, want empty", got)
		}
	})
}

func TestParseLesson(t *testing.T) {
	t.Run("english marker inline with the lesson text", func(t *testing.T) {
		raw := "1. Entry/exit timing: fine.\n2. Thesis check: held up.\n\nLesson: Sold too early into strength; should have trimmed instead of exiting fully.\n"
		got := parseLesson(i18n.EN, raw)
		want := "Sold too early into strength; should have trimmed instead of exiting fully."
		if got != want {
			t.Errorf("parseLesson() = %q, want %q", got, want)
		}
	})

	t.Run("chinese marker", func(t *testing.T) {
		raw := "1. 進出場時點：尚可。\n\n教訓: 賣得太早，應該分批減碼而不是清倉。\n"
		got := parseLesson(i18n.ZH, raw)
		want := "賣得太早，應該分批減碼而不是清倉。"
		if got != want {
			t.Errorf("parseLesson() = %q, want %q", got, want)
		}
	})

	t.Run("lesson wraps across multiple lines after the marker", func(t *testing.T) {
		raw := "Lesson: Exited too early\nand missed the rest of the move.\n"
		got := parseLesson(i18n.EN, raw)
		want := "Exited too early and missed the rest of the move."
		if got != want {
			t.Errorf("parseLesson() = %q, want %q", got, want)
		}
	})

	t.Run("marker absent (model omitted it) yields empty", func(t *testing.T) {
		raw := "1. Entry/exit timing: fine.\n2. Thesis check: held up.\n"
		if got := parseLesson(i18n.EN, raw); got != "" {
			t.Errorf("parseLesson() = %q, want empty", got)
		}
	})

	t.Run("empty input yields empty lesson", func(t *testing.T) {
		if got := parseLesson(i18n.EN, ""); got != "" {
			t.Errorf("parseLesson() = %q, want empty", got)
		}
	})
}
