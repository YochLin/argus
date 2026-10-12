package assets

import (
	"reflect"
	"testing"
)

// fields renders problems as "field" (missing) or "field!" (invalid).
func fields(ps []Problem) []string {
	out := []string{}
	for _, p := range ps {
		if p.Missing {
			out = append(out, p.Field)
		} else {
			out = append(out, p.Field+"!")
		}
	}
	return out
}

func TestValidateAsset(t *testing.T) {
	v := 100.0
	ok := NewAsset{Side: "asset", Type: "deposit", Name: "玉山活存", Group: "liquid", Value: &v}
	cases := []struct {
		name string
		edit func(*NewAsset)
		want []string
	}{
		{"complete", func(*NewAsset) {}, []string{}},
		{"past date", func(a *NewAsset) { a.Date = "2026-01-31" }, []string{}},
		{"nothing filled in", func(a *NewAsset) { *a = NewAsset{} }, []string{"side", "type", "name", "group", "value"}},
		{"bad side and group", func(a *NewAsset) { a.Side, a.Group = "both", "cash" }, []string{"side!", "group!"}},
		{"missing value, bad date", func(a *NewAsset) { a.Value, a.Date = nil, "2026/01/31" }, []string{"value", "date!"}},
	}
	for _, c := range cases {
		a := ok
		c.edit(&a)
		if got := fields(ValidateAsset(a)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestValidateBalance(t *testing.T) {
	v := 1.0
	today := "2026-10-12"
	cases := []struct {
		name  string
		id    int64
		value *float64
		date  string
		want  []string
	}{
		{"today by default", 1, &v, "", []string{}},
		{"today", 1, &v, today, []string{}},
		{"missing both", 0, nil, "", []string{"assetId", "value"}},
		{"future", 1, &v, "2026-10-13", []string{"date!"}},
		{"bad date", 1, &v, "12/10", []string{"date!"}},
	}
	for _, c := range cases {
		if got := fields(ValidateBalance(c.id, c.value, c.date, today)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
