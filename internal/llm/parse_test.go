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
