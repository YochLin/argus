package bot

import (
	"strings"
	"time"
	"unicode"

	"argus/internal/data"
	"argus/internal/llm"
	"argus/internal/signals"
)

// priceEventFacts converts a detected event into the fact sheet the LLM
// prompt takes. Two structs rather than one shared type is deliberate:
// internal/llm doesn't import internal/signals (see llm.StrategyHitInfo for
// the same split), so this three-line copy is what keeps that boundary.
func priceEventFacts(ev signals.PriceEvent) llm.PriceEventFacts {
	return llm.PriceEventFacts{
		Ticker:              ev.Ticker,
		GapPct:              ev.GapPct,
		ChangePct:           ev.ChangePct,
		CumulativePct:       ev.CumulativePct,
		GapTriggered:        ev.GapTriggered,
		ChangeTriggered:     ev.ChangeTriggered,
		CumulativeTriggered: ev.CumulativeTriggered,
	}
}

// eventNewsWindow is how far from the event's own trading date a news item
// may be published and still be offered to ExplainPriceEvent as a possible
// cause. One day either side, not a tighter window: a US event dated in
// Taiwan-yesterday terms has its session news spread across two CST
// calendar days (the US open is 21:30 CST, the close 04:00 CST the next
// morning), so anything narrower would throw away news from the very
// session being explained.
const eventNewsWindow = 24 * time.Hour

// eventNewsSlots is how many news items an event summary is given, and
// eventNewsFetch how many are pulled to fill them. The gap between the two
// is the room the date filter and the syndication dedupe need: asking for
// exactly five and then throwing three away leaves the model with two, so
// the fetch is widened rather than the filters loosened. Both filters run
// before the truncation, so the five that survive are the five freshest
// distinct stories.
const (
	eventNewsSlots = 5
	eventNewsFetch = 10
)

// filterNewsNearDate keeps only the news published within eventNewsWindow of
// date (a CST "2006-01-02" trading date), dropping anything older — Phase 20
// 後續 PR2's fix for the summaries' main quality problem. The feed is
// correctly ordered newest-first, but a ticker's top-5 routinely contained
// 4–7-day-old stories, and the prompt asks the model to infer a cause from
// whatever news it's given, so it dutifully blamed last week's headline for
// today's gap. Filtering to nothing is the intended outcome for a move with
// no news behind it: the prompt already has a "no related news, cause
// unknown" branch that until now was almost never reached.
//
// Items with no publish timestamp are dropped rather than kept — an undated
// item can't be shown to be recent, and this is the one place where being
// wrong about recency is the whole failure mode. An unparseable date leaves
// news untouched (a filter bug must not silently starve the prompt).
func filterNewsNearDate(news []data.NewsItem, date string) []data.NewsItem {
	day, err := time.ParseInLocation("2006-01-02", date, cst)
	if err != nil {
		return news
	}
	var out []data.NewsItem
	for _, n := range news {
		if n.PublishedAt.IsZero() {
			continue
		}
		p := n.PublishedAt.In(cst)
		published := time.Date(p.Year(), p.Month(), p.Day(), 0, 0, 0, 0, cst)
		if diff := published.Sub(day); diff <= eventNewsWindow && diff >= -eventNewsWindow {
			out = append(out, n)
		}
	}
	return out
}

// headlineDupThreshold is how similar two normalized headlines must be
// (Dice coefficient over rune bigrams) before the later one is treated as a
// syndicated copy of the earlier. 0.8 is calibrated for the case that
// motivated this — the same wire story carried by several outlets, differing
// by an outlet tag or a couple of characters — and deliberately sits well
// above where two genuinely different stories about the same company land.
const headlineDupThreshold = 0.8

// dedupeHeadlines drops news whose headline is a near-copy of an earlier
// one's, keeping the first (the feed is newest-first, so the first is the
// freshest). Phase 20 後續 PR4: TW news comes from Google News RSS, which
// returns the same story from several outlets — one of ten sampled TW
// tickers had the same headline three times inside its top five — and TW
// items carry no Summary at all (KeyNewsSummaryLine never fires there, 50/50
// sampled items were empty), so the headline is the only thing the model
// gets. With only five slots and nothing but titles in them, spending three
// on one story is most of the ticker's evidence gone.
//
// Not TW-gated: a duplicate headline is noise in any market, and the check
// is a few string comparisons over at most a handful of items.
func dedupeHeadlines(news []data.NewsItem) []data.NewsItem {
	var out []data.NewsItem
	var kept []string
	for _, n := range news {
		norm := normalizeHeadline(n.Headline)
		dup := false
		for _, k := range kept {
			if diceSimilarity(norm, k) >= headlineDupThreshold {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		out = append(out, n)
		kept = append(kept, norm)
	}
	return out
}

// normalizeHeadline strips a headline down to what two syndications of the
// same story share: the trailing " - 媒體名" outlet tag Google News RSS
// appends, then punctuation, spacing and case. What's left is letters and
// digits, which for a CJK headline is the characters themselves.
func normalizeHeadline(h string) string {
	if i := strings.LastIndex(h, " - "); i > 0 {
		h = h[:i]
	}
	var sb strings.Builder
	for _, r := range strings.ToLower(h) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// diceSimilarity is the Dice coefficient over the two strings' rune bigram
// sets — 1 for identical, 0 for nothing in common. Chosen over an edit
// distance because it needs no allocation-heavy matrix and behaves the same
// on CJK (where a bigram is two characters, already a meaningful unit) as on
// English; strings under two runes fall back to equality, having no bigrams.
func diceSimilarity(a, b string) float64 {
	ar, br := []rune(a), []rune(b)
	if len(ar) < 2 || len(br) < 2 {
		return boolToFloat(a == b)
	}
	seen := make(map[string]bool, len(ar))
	for i := 0; i+1 < len(ar); i++ {
		seen[string(ar[i:i+2])] = true
	}
	matches := 0
	other := make(map[string]bool, len(br))
	for i := 0; i+1 < len(br); i++ {
		bg := string(br[i : i+2])
		if other[bg] {
			continue
		}
		other[bg] = true
		if seen[bg] {
			matches++
		}
	}
	return 2 * float64(matches) / float64(len(seen)+len(other))
}

func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
