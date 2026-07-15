package webfetch

import "testing"

func TestExtractURL(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
		ok   bool
	}{
		{"bare url", "https://example.com/article", "https://example.com/article", true},
		{"url with leading text", "幫我看看這篇 https://example.com/a/b?x=1 對 NVDA 有沒有影響", "https://example.com/a/b?x=1", true},
		{"trailing chinese period", "看看這篇 https://example.com/a。", "https://example.com/a", true},
		{"wrapped in parens", "(https://example.com/a)", "https://example.com/a", true},
		{"no url", "NVDA 今天怎麼樣", "", false},
		{"http not https", "http://example.com", "http://example.com", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := ExtractURL(c.text)
			if ok != c.ok {
				t.Fatalf("ExtractURL(%q) ok = %v, want %v", c.text, ok, c.ok)
			}
			if ok && got != c.want {
				t.Fatalf("ExtractURL(%q) = %q, want %q", c.text, got, c.want)
			}
		})
	}
}
