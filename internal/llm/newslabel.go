package llm

import (
	"context"
	"fmt"
	"strings"

	"argus/internal/data"
	"argus/internal/i18n"
)

// NewsLabel is the model's reading of one headline. Sentiment is "" when the
// model's line for that item was missing or unusable, so a caller can tell
// "not classified" from a real "neutral".
type NewsLabel struct {
	Sentiment string // "" | "bull" | "bear" | "neutral"
	Tag       string // "earn" | "guide" | "analyst" | "sector" | "macro" | "flow" | "other"
	Major     bool
}

// newsLabelFormat is the one output line the prompt asks for and
// parseNewsLabels reads. It is injected into both languages' prompts via %s
// (KeyNewsClassifyTask) so the two cannot drift apart.
const newsLabelFormat = "N. sentiment=<bull|bear|neutral> tag=<earn|guide|analyst|sector|macro|flow|other> major=<0|1>"

var newsLabelTags = map[string]bool{"earn": true, "guide": true, "analyst": true, "sector": true, "macro": true, "flow": true, "other": true}

// ClassifyNews labels headlines with sentiment / kind / major-event, for the
// news stored alongside a fill. One one-shot call on checkModel (a per-headline
// judgement, not full analysis — same reasoning as ExplainPriceEvent). The
// result is index-aligned with news; an item the model skipped keeps the zero
// NewsLabel. Headline-only, so the labels are coarse and descriptive: nothing
// here feeds alerts, ranking, or a trade decision.
func (c *Client) ClassifyNews(ctx context.Context, ticker string, news []data.NewsItem) ([]NewsLabel, error) {
	if len(news) == 0 {
		return nil, nil
	}
	raw, _, _, err := c.prompt(ctx, buildNewsClassifyPrompt(c.lang, ticker, news), func(b backend) string { return b.checkModel })
	if err != nil {
		return nil, err
	}
	return parseNewsLabels(raw, len(news)), nil
}

func buildNewsClassifyPrompt(lang i18n.Lang, ticker string, news []data.NewsItem) string {
	var sb strings.Builder
	sb.WriteString(i18n.T(lang, i18n.KeyNewsClassifyIntro, ticker))
	for i, n := range news {
		writeNewsItem(&sb, lang, i+1, n)
	}
	sb.WriteString(i18n.T(lang, i18n.KeyNewsClassifyTask, newsLabelFormat))
	return sb.String()
}

// parseNewsLabels reads "N. sentiment=… tag=… major=…" lines into an n-long
// slice. Lenient about everything but the values: extra text, a missing
// trailing dot or a repeated number are tolerated (the first line for a number
// wins); an unknown sentiment leaves that item unclassified, an unknown tag
// becomes "other".
func parseNewsLabels(raw string, n int) []NewsLabel {
	out := make([]NewsLabel, n)
	done := make([]bool, n)
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		var idx int
		if _, err := fmt.Sscanf(strings.TrimRight(fields[0], ".):"), "%d", &idx); err != nil || idx < 1 || idx > n || done[idx-1] {
			continue
		}
		var l NewsLabel
		for _, f := range fields[1:] {
			k, v, ok := strings.Cut(f, "=")
			if !ok {
				continue
			}
			v = strings.ToLower(strings.Trim(v, "<>,;"))
			switch strings.ToLower(k) {
			case "sentiment":
				if v == "bull" || v == "bear" || v == "neutral" {
					l.Sentiment = v
				}
			case "tag":
				l.Tag = "other"
				if newsLabelTags[v] {
					l.Tag = v
				}
			case "major":
				l.Major = v == "1"
			}
		}
		if l.Sentiment == "" {
			continue
		}
		if l.Tag == "" {
			l.Tag = "other"
		}
		out[idx-1], done[idx-1] = l, true
	}
	return out
}
