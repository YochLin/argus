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
