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

// tickerNewsSlots is how many news items a per-ticker prompt section is
// given, and tickerNewsFetch how many are pulled to fill them. The gap
// between the two is the room the date filter and the dedupe need: asking
// for exactly five and then throwing three away leaves the model with two,
// so the fetch is widened rather than the filters loosened. Both filters run
// before the truncation, so the five that survive are the five freshest
// distinct stories. Shared by the event summary and the daily report /
// morning briefing (see newsPicker) — a wasted slot costs the same wherever
// it happens.
const (
	tickerNewsSlots = 5
	tickerNewsFetch = 10
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

// tickerNewsMaxAge is how old a fetchStockData news item may be before it's
// dropped as stale — Phase 19 後續 PR3. Unlike a price event,
// fetchStockData's daily-report/recommend path has no event date to anchor
// filterNewsNearDate's ±eventNewsWindow to, so filterStaleNews below just
// checks age against "now" instead, with a window widened to 3 days
// (matching the /llm audit page's own isStale threshold, LlmRunsView.tsx) —
// tighter than that and today's genuinely slow-news tickers would come back
// empty rather than merely older. Live-verified against run id=8
// (2026-08-26 US daily report): 50/239 items were older than this.
const tickerNewsMaxAge = 72 * time.Hour

// filterStaleNews drops items older than tickerNewsMaxAge as of asOf.
// Unlike filterNewsNearDate's symmetric window, only a lower bound is
// needed here — news is never dated in the future. Same "undated item is
// dropped, not kept" rule as filterNewsNearDate.
func filterStaleNews(news []data.NewsItem, asOf time.Time) []data.NewsItem {
	var out []data.NewsItem
	for _, n := range news {
		if n.PublishedAt.IsZero() {
			continue
		}
		if asOf.Sub(n.PublishedAt) <= tickerNewsMaxAge {
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

// newsPicker fills prompt news slots for one run, skipping any item whose
// headline is a near-copy of one already picked. Two things it catches, both
// measured on live /llm audit runs:
//
// Within a ticker — TW news comes from Google News RSS, which returns the
// same story from several outlets, and TW items carry no Summary at all
// (KeyNewsSummaryLine never fires there), so the headline is the only thing
// the model gets. With five slots and nothing but titles in them, spending
// three on one story is most of the ticker's evidence gone.
//
// Across the tickers of the same run — Finnhub tags a generic market piece
// onto every symbol it name-drops ("The Deep Unknowns Of AI" filled a slot
// under five different tickers in one US report; 45 of 229 news slots in
// that run went to a story an earlier ticker had already shown). The
// duplicate never looks like one from inside a single ticker's five, which
// is why the seen set outlives the call.
//
// Zero value is ready to use. One picker per prompt, never one per process:
// yesterday's headlines must not block today's. Not TW-gated — a duplicate
// is noise in any market — and the linear scan over the few hundred
// headlines one run keeps is nothing next to the network calls it sits
// between.
type newsPicker struct{ seen []string }

// pick returns up to slots items from news (newest first, as every
// data.Provider hands it over), skipping near-duplicates of anything this
// picker has already returned. Fewer than slots is a valid answer: no news
// beats the same news twice.
func (p *newsPicker) pick(news []data.NewsItem, slots int) []data.NewsItem {
	var out []data.NewsItem
	for _, n := range news {
		if len(out) >= slots {
			break
		}
		norm := normalizeHeadline(n.Headline)
		dup := false
		for _, k := range p.seen {
			if diceSimilarity(norm, k) >= headlineDupThreshold {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		out = append(out, n)
		p.seen = append(p.seen, norm)
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
