package service

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"argus/internal/assets"
	"argus/internal/db"
)

// The drafts pane translates rule problems by code; a failed write has no
// code and is shown as the server's text.
func TestCheckWealthDraftErrorCodes(t *testing.T) {
	d := db.WealthDraft{Kind: assets.DraftAsset, Payload: `{"type":"stock","name":"台積電","group":"x","currency":"TWD","value":1}`}
	c := CheckWealthDraft(d, nil, nil, "2026-10-15")
	var codes []string
	for _, e := range c.Errors {
		codes = append(codes, e.Code)
	}
	if c.Status != "error" || !slices.Equal(codes, []string{"GroupInvalid", "StockAsAsset"}) || !slices.Equal(c.Errors[0].Args, []string{"x"}) {
		t.Fatalf("check = %+v", c)
	}

	d.ApplyError = "boom"
	if c := CheckWealthDraft(d, nil, nil, "2026-10-15"); c.Error != "boom" || c.Errors != nil {
		t.Fatalf("failed write = %+v", c)
	}
}

// Every code a rule can send has a zh and an en string in the web's
// dictionary (asstErr<Code>), and every rule problem has a code.
func TestDraftErrorCodesTranslated(t *testing.T) {
	files, _ := filepath.Glob("../assets/*.go")
	files = append(files, "wealth_drafts.go")
	codeRe := regexp.MustCompile(`bad\("\w+", "(\w+)"|Code: "(\w+)"`)
	var src strings.Builder
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		src.Write(b)
	}
	for _, lit := range regexp.MustCompile(`Problem\{Field: "[^}]*\}`).FindAllString(src.String(), -1) {
		if !strings.Contains(lit, "Code:") {
			t.Errorf("Problem without a Code (use bad()): %s", lit)
		}
	}
	dicts := map[string]string{}
	for _, lang := range []string{"en", "zh"} {
		b, err := os.ReadFile("../../web/src/i18n/" + lang + ".ts")
		if err != nil {
			t.Fatal(err)
		}
		dicts[lang] = string(b)
	}
	ms := codeRe.FindAllStringSubmatch(src.String(), -1)
	if len(ms) < 20 {
		t.Fatalf("found only %d codes", len(ms))
	}
	for _, m := range ms {
		code := m[1] + m[2]
		for lang, dict := range dicts {
			if n := strings.Count(dict, "asstErr"+code+`: "`); n != 1 {
				t.Errorf("asstErr%s: %d strings in i18n/%s.ts, want 1", code, n, lang)
			}
		}
	}
}
