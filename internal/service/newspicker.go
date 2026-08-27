package service

import (
	"strings"
	"unicode"

	"argus/internal/data"
)

// headlineDupThreshold is how similar two normalized headlines must be (Dice
// coefficient over rune bigrams) before the later one is treated as a
// syndicated copy of the earlier — see newsPicker.
const headlineDupThreshold = 0.8

// newsPicker fills prompt news slots for one run, skipping any item whose
// headline is a near-copy of one already picked, both within one ticker's
// results (TW's Google-News-RSS feed repeats the same story across outlets)
// and across the tickers of one prompt (Finnhub tags a generic market piece
// onto every symbol it name-drops). Zero value is ready to use; one picker
// per prompt, never one per process — yesterday's headlines must not block
// today's.
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
// appends, then punctuation, spacing and case.
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
// sets — 1 for identical, 0 for nothing in common. Strings under two runes
// fall back to equality, having no bigrams.
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
