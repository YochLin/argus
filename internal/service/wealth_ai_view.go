package service

import "encoding/json"

// AIViewSettingKey holds the latest monthly AI view: the home page's "AI 觀點"
// card, written by the monthly health report. Only the newest is kept; under
// profile.* so the wealth reset clears it with the rest.
const AIViewSettingKey = "profile.ai_view"

// WealthAIView is the model's own observations beyond the rule-based checks.
// It isn't checked against any rule, which is why the page labels it.
type WealthAIView struct {
	GeneratedAt string   `json:"generatedAt"` // YYYY-MM-DD
	Summary     string   `json:"summary"`
	Points      []string `json:"points"`
}

// LoadWealthAIView returns the stored view, or nil when there is none yet (or
// it can't be read; the card is a convenience).
func LoadWealthAIView(r SettingReader) *WealthAIView {
	raw, ok, err := r.GetSetting(AIViewSettingKey)
	if err != nil || !ok {
		return nil
	}
	var v WealthAIView
	if json.Unmarshal([]byte(raw), &v) != nil || len(v.Points) == 0 {
		return nil
	}
	return &v
}

// SaveWealthAIView replaces the stored view.
func SaveWealthAIView(w interface{ SetSetting(key, value string) error }, v WealthAIView) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return w.SetSetting(AIViewSettingKey, string(raw))
}
