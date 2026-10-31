package llm

import (
	"context"
	"regexp"
	"strings"

	"argus/internal/data"
	"argus/internal/i18n"
)

// DropFacts is one drop as AttributeDrop sees it.
type DropFacts struct {
	Ticker    string
	Bars      int
	Pct       float64
	GapPct    *float64
	IndexPct  *float64
	Benchmark string
	BrokeMA60 bool // closed below the 60-day average it was above before the drop
	BrokeLow  bool // closed below the lowest low of the 20 bars before the drop
}

// DropAttribution is the model's reading of why a stock fell. All empty when
// the headlines don't say, which the prompt asks for rather than a guess.
type DropAttribution struct {
	Cause      string // "" | "earn" | "macro" | "sector" | "flow" | "corp" | "tech"
	Secondary  []string
	Confidence string // "high" | "mid"
	Summary    string
}

// dropCauseFormat is the reply the prompt asks for and parseDropAttribution
// reads, injected into both languages' prompts via %s (KeyDropCauseTask) so the
// two cannot drift apart.
const dropCauseFormat = "CAUSE: <earn|macro|sector|flow|corp|tech|unknown>\nALSO: <earn|macro|sector|flow|corp|tech|none>\nCONFIDENCE: <high|mid>\nSUMMARY: <one sentence>"

var dropCauses = map[string]bool{"earn": true, "macro": true, "sector": true, "flow": true, "corp": true, "tech": true}

// AttributeDrop is the 下跌事件 tab's cause: one one-shot call on checkModel,
// over the ticker's own headlines around the drop. Descriptive only: nothing
// here feeds alerts, ranking, or a trade decision.
func (c *Client) AttributeDrop(ctx context.Context, f DropFacts, news []data.NewsItem) (a DropAttribution, raw, model string, latencyMs int64, err error) {
	raw, model, latencyMs, err = c.prompt(ctx, buildDropCausePrompt(c.lang, f, news), func(b backend) string { return b.checkModel })
	if err != nil {
		return DropAttribution{}, raw, model, latencyMs, err
	}
	return parseDropAttribution(raw), raw, model, latencyMs, nil
}

func buildDropCausePrompt(lang i18n.Lang, f DropFacts, news []data.NewsItem) string {
	var sb strings.Builder
	sb.WriteString(i18n.T(lang, i18n.KeyDropCauseIntro, f.Ticker, f.Bars, f.Pct))
	if f.GapPct != nil {
		sb.WriteString(i18n.T(lang, i18n.KeyDropCauseGapLine, *f.GapPct))
	}
	if f.IndexPct != nil {
		sb.WriteString(i18n.T(lang, i18n.KeyDropCauseIndexLine, f.Benchmark, *f.IndexPct))
	}
	if f.BrokeMA60 {
		sb.WriteString(i18n.T(lang, i18n.KeyDropCauseBrokeMA60))
	}
	if f.BrokeLow {
		sb.WriteString(i18n.T(lang, i18n.KeyDropCauseBrokeLow))
	}
	sb.WriteString(i18n.T(lang, i18n.KeyNewsHeader))
	for i, n := range news {
		writeNewsItem(&sb, lang, i+1, n)
	}
	sb.WriteString(i18n.T(lang, i18n.KeyDropCauseTask, dropCauseFormat))
	return sb.String()
}

// dropCauseLine finds a key anywhere in a line. Not only at its start: the model
// has opened with stray text on the same line ("的思考過程：CAUSE: earn",
// run 231, the deepest drop of that day), and has to be read past it.
var dropCauseLine = regexp.MustCompile(`(?i)\b(CAUSE|ALSO|CONFIDENCE|SUMMARY)\*{0,2}\s*[:：](.*)`)

// parseDropAttribution reads dropCauseFormat's lines. An unknown or missing
// cause, or a missing summary, is no attribution at all: half of one would show
// a label with nothing behind it. A confidence the model made up reads as mid.
func parseDropAttribution(raw string) DropAttribution {
	var a DropAttribution
	for _, line := range strings.Split(raw, "\n") {
		m := dropCauseLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		v := strings.TrimSpace(m[2])
		word := strings.ToLower(strings.Trim(v, "<>*` "))
		switch strings.ToUpper(m[1]) {
		case "CAUSE":
			if dropCauses[word] {
				a.Cause = word
			}
		case "ALSO":
			if dropCauses[word] {
				a.Secondary = []string{word}
			}
		case "CONFIDENCE":
			a.Confidence = word
		case "SUMMARY":
			a.Summary = strings.Trim(v, "*` ")
		}
	}
	if a.Cause == "" || a.Summary == "" {
		return DropAttribution{}
	}
	if a.Confidence != "high" {
		a.Confidence = "mid"
	}
	if len(a.Secondary) == 1 && a.Secondary[0] == a.Cause {
		a.Secondary = nil
	}
	return a
}
