package i18n

import (
	"regexp"
	"testing"
)

var verbRe = regexp.MustCompile(`%%|%[-+# 0]*\d*(\.\d+)?[a-zA-Z]`)

func countVerbs(s string) int {
	n := 0
	for _, m := range verbRe.FindAllString(s, -1) {
		if m != "%%" {
			n++
		}
	}
	return n
}

// TestTablesMatch guards against translation drift: every Key must be
// present in both zhMessages and enMessages, with the same number of
// fmt.Sprintf verbs in each — call sites pass one set of positional args
// and reuse it for whichever table T picks, so a mismatch here would be a
// runtime Sprintf error (or worse, silently wrong output) the first time
// someone actually runs the bot with BOT_LANGUAGE=en.
func TestTablesMatch(t *testing.T) {
	for k, zh := range zhMessages {
		en, ok := enMessages[k]
		if !ok {
			t.Errorf("key %q present in zh.go but missing from en.go", k)
			continue
		}
		zv, ev := countVerbs(zh), countVerbs(en)
		if zv != ev {
			t.Errorf("key %q: verb count mismatch zh=%d en=%d\n  zh: %q\n  en: %q", k, zv, ev, zh, en)
		}
	}
	for k := range enMessages {
		if _, ok := zhMessages[k]; !ok {
			t.Errorf("key %q present in en.go but missing from zh.go", k)
		}
	}
	t.Logf("checked %d keys", len(zhMessages))
}
