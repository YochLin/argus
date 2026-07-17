package llm

import (
	"strings"

	"argus/internal/i18n"
)

// parseMarketSummary extracts the free-text market-news summary the model
// was instructed to emit under marker (see KeyRecMarketSummaryTask) before
// its per-ticker [TICKER: ...] blocks. Returns "" if marker never appears
// (e.g. the model omitted it) — same permissive-degrade shape as
// parseRecommendations returning an empty slice.
func parseMarketSummary(raw, marker string) string {
	lines := strings.Split(raw, "\n")
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == marker {
			start = i + 1
			break
		}
	}
	if start == -1 {
		return ""
	}

	end := len(lines)
	for i := start; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "[TICKER:") {
			end = i
			break
		}
	}
	return strings.TrimSpace(strings.Join(lines[start:end], "\n"))
}

// parseRecommendations parses the structured LLM output into Recommendation
// slices. Expected format:
//
//	[TICKER: AAPL]
//	<action marker> BUY|SELL|HOLD
//	<reason marker>: ...
//
// The action/reason line markers are language-dependent (i18n.KeyActionMarker
// / KeyReasonMarker — "動作:"/"原因:" for zh, "Action:"/"Reason:" for en)
// because buildRecommendationPrompt asks the model to use those same markers
// in its reply; they must stay in lockstep, which is why both sides read
// from the same i18n keys instead of each hardcoding a prefix. A missing or
// unrecognized action line leaves Action empty rather than dropping the
// block, so replies from before the action format (or a model that ignores
// it) still parse.
func parseRecommendations(lang i18n.Lang, raw string) []Recommendation {
	actionPrefix := i18n.T(lang, i18n.KeyActionMarker)
	reasonPrefix := i18n.T(lang, i18n.KeyReasonMarker)
	var recs []Recommendation
	lines := strings.Split(raw, "\n")
	var currentTicker string
	var currentAction string
	var reasonParts []string

	flush := func() {
		if currentTicker != "" {
			recs = append(recs, Recommendation{
				Ticker: currentTicker,
				Action: currentAction,
				Reason: strings.TrimSpace(strings.Join(reasonParts, " ")),
			})
		}
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[TICKER:") && strings.HasSuffix(line, "]") {
			flush()
			ticker := strings.TrimSuffix(strings.TrimPrefix(line, "[TICKER:"), "]")
			currentTicker = strings.TrimSpace(ticker)
			currentAction = ""
			reasonParts = nil
			continue
		}
		if strings.HasPrefix(line, actionPrefix) {
			currentAction = parseAction(strings.TrimPrefix(line, actionPrefix))
			continue
		}
		if strings.HasPrefix(line, reasonPrefix) {
			reason := strings.TrimPrefix(line, reasonPrefix)
			reasonParts = append(reasonParts, strings.TrimSpace(reason))
			continue
		}
		if currentTicker != "" && len(reasonParts) > 0 && line != "" {
			reasonParts = append(reasonParts, line)
		}
	}
	flush()
	return recs
}

// parseExploreNominations parses Phase 2.6 解凍's two-stage LLM exploration
// reply (see docs/phase-2.6-two-stage-llm-exploration.md). Expected format:
//
//	[EXPLORE: NVDA]
//	<reason marker> ...
//
// same block-then-reason-lines shape as parseRecommendations, but with only
// a ticker and a reason (no action line) — reusing KeyReasonMarker for the
// reason prefix, same as the prompt side reuses it rather than minting a
// second marker. Ticker values are trimmed, upper-cased, and stripped of a
// leading "$" (models sometimes format tickers as "$NVDA") before being
// kept. Zero matches returns an empty slice rather than an error — same
// permissive-degrade shape as parseMarketSummary/parseRecommendations, since
// "the model nominated nothing usable" is an ordinary outcome, not a
// failure. Results beyond maxExploreNominations are dropped defensively, in
// case the model ignores the prompt's stated cap.
func parseExploreNominations(lang i18n.Lang, raw string) []ExploreNomination {
	marker := i18n.T(lang, i18n.KeyExploreMarker)
	reasonPrefix := i18n.T(lang, i18n.KeyReasonMarker)

	var noms []ExploreNomination
	lines := strings.Split(raw, "\n")
	var currentTicker string
	var reasonParts []string

	flush := func() {
		if currentTicker != "" {
			noms = append(noms, ExploreNomination{
				Ticker: currentTicker,
				Reason: strings.TrimSpace(strings.Join(reasonParts, " ")),
			})
		}
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, marker) && strings.HasSuffix(line, "]") {
			flush()
			ticker := strings.TrimSuffix(strings.TrimPrefix(line, marker), "]")
			currentTicker = normalizeExploreTicker(ticker)
			reasonParts = nil
			continue
		}
		if strings.HasPrefix(line, reasonPrefix) {
			reason := strings.TrimPrefix(line, reasonPrefix)
			reasonParts = append(reasonParts, strings.TrimSpace(reason))
			continue
		}
		if currentTicker != "" && len(reasonParts) > 0 && line != "" {
			reasonParts = append(reasonParts, line)
		}
	}
	flush()

	if len(noms) > maxExploreNominations {
		noms = noms[:maxExploreNominations]
	}
	return noms
}

// normalizeExploreTicker trims whitespace, strips a leading "$" (a common
// model formatting habit), and upper-cases the result — permissive parsing
// on a value that IsUSEquitySymbol/GetQuote will verify for real downstream.
func normalizeExploreTicker(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "$")
	return strings.ToUpper(strings.TrimSpace(s))
}

// parseLesson extracts Phase 3.9's short closed-trade takeaway from a
// ReviewTrade reply (see docs/research-tradingagents.md's "反思回饋迴路"
// section): buildTradeReviewPrompt instructs the model to end its review
// with a line starting with the lesson marker (KeyLessonMarker), same
// prompt-and-parser-share-a-marker convention as KeyReasonMarker/
// KeyActionMarker. Unlike parseMarketSummary (marker alone on its own line,
// content on subsequent lines), the marker here is expected inline with the
// lesson text on the same line — so this also picks up any further
// non-empty lines after the marker line, in case the model wraps the
// lesson across more than one line. Returns "" if the marker never appears
// (model omitted it, or an older/malformed reply) — same permissive-degrade
// shape as parseMarketSummary/parseRecommendations, so a missing lesson
// doesn't stop reviewClosedTrade from at least sending the full review text.
func parseLesson(lang i18n.Lang, raw string) string {
	marker := i18n.T(lang, i18n.KeyLessonMarker)
	lines := strings.Split(raw, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, marker) {
			continue
		}
		parts := []string{strings.TrimSpace(strings.TrimPrefix(trimmed, marker))}
		for _, rest := range lines[i+1:] {
			if rest = strings.TrimSpace(rest); rest != "" {
				parts = append(parts, rest)
			}
		}
		return strings.TrimSpace(strings.Join(parts, " "))
	}
	return ""
}

// parseAction normalizes an action-line value to BUY/SELL/HOLD, returning ""
// for anything else so downstream consumers (display, /track hit-rate) never
// see a made-up action word.
func parseAction(s string) string {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "BUY":
		return "BUY"
	case "SELL":
		return "SELL"
	case "HOLD":
		return "HOLD"
	}
	return ""
}
