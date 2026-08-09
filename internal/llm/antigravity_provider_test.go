package llm

import (
	"strings"
	"testing"
)

// TestSplitPrompt guards the two things a chunked agy feed depends on: every
// chunk fits inside one argv entry (the whole point — see agyMaxPromptArg),
// and the chunks reassemble to the original prompt byte for byte, so no
// ticker's data is silently dropped on the way to the model.
func TestSplitPrompt(t *testing.T) {
	cases := map[string]string{
		"under limit":        "hello\nworld\n",
		"multi line":         strings.Repeat("ticker row with some data\n", 500),
		"multibyte":          strings.Repeat("台積電 2330 收盤 1075 元，量增價漲\n", 500),
		"one oversized line": strings.Repeat("台", 5000), // no newline to cut at
	}
	const limit = 1024
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			chunks := splitPrompt(in, limit)
			if got := strings.Join(chunks, ""); got != in {
				t.Errorf("chunks don't reassemble to the original (%d bytes vs %d)", len(got), len(in))
			}
			for i, c := range chunks {
				if len(c) > limit {
					t.Errorf("chunk %d is %d bytes, over the %d limit", i, len(c), limit)
				}
				if !utf8ValidPrefixSafe(c) {
					t.Errorf("chunk %d was cut through a multi-byte rune", i)
				}
			}
		})
	}
}

func utf8ValidPrefixSafe(s string) bool {
	// A chunk cut mid-rune shows up as an invalid encoding at its own edge;
	// interior bytes are whatever the caller passed in.
	return strings.ToValidUTF8(s, "�") == s
}
