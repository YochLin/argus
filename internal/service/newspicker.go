package service

import (
	"strings"
	"unicode"

	"argus/internal/data"
)

// headlineDupThreshold is how similar two normalized headlines must be
// (Dice coefficient over rune bigrams) before the later one is treated as a
// syndicated copy of the earlier. 0.8 is calibrated for the case that
// motivated this — the same wire story carried by several outlets, differing
// by an outlet tag or a couple of characters — and deliberately sits well
// above where two genuinely different stories about the same company land.
const headlineDupThreshold = 0.8

// NewsPicker fills prompt news slots for one run, skipping any item whose
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
// between. Shared by the daily report / recommend / event-summary paths
// (internal/bot) and the morning briefing (below).
type NewsPicker struct{ seen []string }

// Pick returns up to slots items from news (newest first, as every
// data.Provider hands it over), skipping near-duplicates of anything this
// picker has already returned. Fewer than slots is a valid answer: no news
// beats the same news twice.
func (p *NewsPicker) Pick(news []data.NewsItem, slots int) []data.NewsItem {
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
