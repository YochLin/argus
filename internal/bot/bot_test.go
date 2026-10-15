package bot

import (
	"testing"
)

func TestParseCommand(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		wantCmd  string
		wantArgs string
		wantOK   bool
	}{
		{"bare command", "/status", "status", "", true},
		{"command with args", "/add AAPL", "add", "AAPL", true},
		{"command with multi-word args", "/buy AAPL 10 200", "buy", "AAPL 10 200", true},
		{"botname suffix stripped", "/status@my_bot", "status", "", true},
		{"botname suffix with args", "/add@my_bot AAPL", "add", "AAPL", true},
		{"plain text is not a command", "hello there", "", "", false},
		{"empty text is not a command", "", "", "", false},
		{"bare slash is not a command", "/", "", "", false},
		{"args get trimmed", "/add   AAPL  ", "add", "AAPL", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, args, ok := parseCommand(tt.text)
			if cmd != tt.wantCmd || args != tt.wantArgs || ok != tt.wantOK {
				t.Errorf("parseCommand(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tt.text, cmd, args, ok, tt.wantCmd, tt.wantArgs, tt.wantOK)
			}
		})
	}
}

// TestUniverseScanChunkFullCoverage/TestUniverseScanChunkEmptyAndNegativeDay
// moved to internal/service/scan_test.go (Phase 24 Stage 1) along with
// universeScanChunk itself — see service.UniverseScanChunk.

// suggestShares/trailingStopThreshold moved to internal/paper (Phase 11
// PR1, see paper.SuggestShares/paper.TrailingStopThreshold) — their tests
// moved with them to internal/paper/paper_test.go.
//
// TestBreachAlertDecision/TestStopBreachDecision/TestTargetReachedDecision
// moved to internal/service/risk_test.go (Phase 24 tech debt 1) — the
// bot-package wrapper functions they tested were orphan copies of
// service.BreachAlertDecision/StopBreachDecision/TargetReachedDecision left
// behind by Stage 1.1's RiskService extraction, unused by anything except
// these tests; deleting the wrappers moved the tests to where the real
// implementation lives instead of dropping the edge-case coverage.
//
// TestRestrictedAlertDecision moved to internal/service/risk_test.go the
// same way (Phase 24 tech debt 2) — restrictedAlertDecision itself moved
// into RiskService.EvaluateRestrictedAlerts.
