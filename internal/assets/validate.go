package assets

import (
	"slices"
	"time"
)

// Problem is one reason a proposed record can't be written yet. Missing
// separates "not filled in" from "filled in wrong": the wealth assistant's
// drafts (Phase 28) show the first kind as 待補 — a question the user can
// answer in the chat — and only the second as an error. The web handlers
// just report the first problem's Msg.
type Problem struct {
	Field   string
	Missing bool
	Msg     string
}

// NewAsset is the part of a proposed asset that has rules: what the quick-add
// drawer, CSV import and the assistant's drafts all validate before creating
// one. Value is the day-one value (nil = not given); Date is its date in
// YYYY-MM-DD, "" meaning today. Callers trim/case-fold first.
type NewAsset struct {
	Side, Type, Name, Group string
	Value                   *float64
	Date                    string
}

// ValidateAsset returns every problem with a, in field order; none means it
// can be written.
func ValidateAsset(a NewAsset) []Problem {
	var ps []Problem
	switch a.Side {
	case "":
		ps = append(ps, missing("side"))
	case "asset", "liability":
	default:
		ps = append(ps, Problem{Field: "side", Msg: `side must be "asset" or "liability": ` + a.Side})
	}
	if a.Type == "" {
		ps = append(ps, missing("type"))
	}
	if a.Name == "" {
		ps = append(ps, missing("name"))
	}
	switch {
	case a.Group == "":
		ps = append(ps, missing("group"))
	case !slices.Contains(AssetGroups, a.Group):
		ps = append(ps, Problem{Field: "group", Msg: "group must be one of liquid/growth/income/hard: " + a.Group})
	}
	if a.Value == nil {
		ps = append(ps, missing("value"))
	}
	if a.Date != "" && !validDate(a.Date) {
		ps = append(ps, Problem{Field: "date", Msg: "invalid date, expected YYYY-MM-DD: " + a.Date})
	}
	return ps
}

// ValidateBalance checks a new value for an existing asset: date "" means
// today, and a future date is refused (history is logged, not forecast).
func ValidateBalance(assetID int64, value *float64, date, today string) []Problem {
	var ps []Problem
	if assetID <= 0 {
		ps = append(ps, missing("assetId"))
	}
	if value == nil {
		ps = append(ps, missing("value"))
	}
	switch {
	case date == "":
	case !validDate(date):
		ps = append(ps, Problem{Field: "date", Msg: "invalid date, expected YYYY-MM-DD: " + date})
	case date > today:
		ps = append(ps, Problem{Field: "date", Msg: "date can't be in the future"})
	}
	return ps
}

func missing(field string) Problem {
	return Problem{Field: field, Missing: true, Msg: field + " is required"}
}

func validDate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}
