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
