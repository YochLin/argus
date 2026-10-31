package llm

import (
	"strings"
	"testing"

	"argus/internal/data"
	"argus/internal/i18n"
)

func TestParseDropAttribution(t *testing.T) {
	got := parseDropAttribution("CAUSE: earn\nALSO: flow\nCONFIDENCE: high\nSUMMARY: Guidance was cut.")
	if got.Cause != "earn" || len(got.Secondary) != 1 || got.Secondary[0] != "flow" || got.Confidence != "high" || got.Summary != "Guidance was cut." {
		t.Fatalf("got %+v", got)
	}

	got = parseDropAttribution("**CAUSE:** macro\nALSO: none\nCONFIDENCE: certain\nSUMMARY: Rates rose.")
	if got.Cause != "macro" || got.Secondary != nil || got.Confidence != "mid" {
		t.Errorf("got %+v", got)
	}

	// Production run 231: stray text before the first key, on the same line.
	got = parseDropAttribution("的思考過程：CAUSE: earn\nALSO: tech\nCONFIDENCE: high\nSUMMARY: 財測疲弱：跳空重挫。")
	if got.Cause != "earn" || got.Summary != "財測疲弱：跳空重挫。" {
		t.Errorf("got %+v", got)
	}

	got = parseDropAttribution("CAUSE：sector\nSUMMARY：同業下修。")
	if got.Cause != "sector" || got.Summary != "同業下修。" {
		t.Errorf("full-width colons: got %+v", got)
	}

	if got := parseDropAttribution("Fell because: guidance\nSUMMARY: x"); got.Cause != "" {
		t.Errorf("'because' read as a key: got %+v", got)
	}

	for name, raw := range map[string]string{
		"unknown":    "CAUSE: unknown\nCONFIDENCE: mid\nSUMMARY: Nothing in the headlines.",
		"no summary": "CAUSE: earn\nCONFIDENCE: mid",
		"made-up":    "CAUSE: vibes\nSUMMARY: x",
		"prose":      "The stock fell on weak guidance.",
	} {
		if got := parseDropAttribution(raw); got.Cause != "" || got.Summary != "" {
			t.Errorf("%s: got %+v", name, got)
		}
	}
}

func TestParseDropAttribution_SecondarySameAsCause(t *testing.T) {
	if got := parseDropAttribution("CAUSE: earn\nALSO: earn\nSUMMARY: x"); got.Secondary != nil {
		t.Errorf("Secondary = %v", got.Secondary)
	}
}

func TestBuildDropCausePrompt(t *testing.T) {
	gap, idx := -3.1, -1.2
	for _, lang := range []i18n.Lang{i18n.ZH, i18n.EN} {
		p := buildDropCausePrompt(lang, DropFacts{Ticker: "AAPL", Bars: 3, Pct: -8.4, GapPct: &gap, IndexPct: &idx, Benchmark: "^GSPC", BrokeMA60: true},
			[]data.NewsItem{{Headline: "Apple cuts guidance", Source: "Reuters"}})
		for _, want := range []string{"AAPL", "-8.4", "-3.1", "^GSPC", "-1.2", "60", "Apple cuts guidance", "CAUSE:", "tech", "unknown"} {
			if !strings.Contains(p, want) {
				t.Errorf("lang %v: prompt missing %q:\n%s", lang, want, p)
			}
		}
		if strings.Contains(p, "%!") {
			t.Errorf("lang %v: bad format verb:\n%s", lang, p)
		}
	}
}
