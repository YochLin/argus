package assets

import (
	"slices"
	"time"
)

// Problem is one reason a proposed record can't be written yet. Missing
// separates "not filled in" from "filled in wrong": the wealth assistant's
// drafts (Phase 28) show the first kind as 待補 — a question the user can
// answer in the chat — and only the second as an error. The web handlers
// just report the first problem's Msg. Msg is English, for the model and the
// forms; Code and Args are what the drafts pane translates (the web's
// asstErr<Code> strings, %s filled from Args in order).
type Problem struct {
	Field   string
	Missing bool
	Msg     string
	Code    string
	Args    []string
}

func bad(field, code, msg string, args ...string) Problem {
	return Problem{Field: field, Msg: msg, Code: code, Args: args}
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
		ps = append(ps, bad("side", "SideInvalid", `side must be "asset" or "liability": `+a.Side, a.Side))
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
		ps = append(ps, bad("group", "GroupInvalid", "group must be one of liquid/growth/income/hard: "+a.Group, a.Group))
	}
	if a.Value == nil {
		ps = append(ps, missing("value"))
	}
	if a.Date != "" && !validDate(a.Date) {
		ps = append(ps, badDate("date", a.Date))
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
		ps = append(ps, badDate("date", date))
	case date > today:
		ps = append(ps, bad("date", "DateFuture", "date can't be in the future"))
	}
	return ps
}

func missing(field string) Problem {
	return Problem{Field: field, Missing: true, Msg: field + " is required"}
}

func badDate(field, s string) Problem {
	return bad(field, "DateInvalid", "invalid date, expected YYYY-MM-DD: "+s, s)
}

func validDate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}
