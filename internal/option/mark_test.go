package option

import "testing"

func TestMark(t *testing.T) {
	cases := []struct {
		name           string
		bid, ask, last float64
		want           float64
	}{
		{"normal bid/ask", 1.83, 1.90, 1.81, 1.865},
		{"crossed bid/ask falls back to last", 2.0, 1.5, 1.81, 1.81},
		{"no bid falls back to last", 0, 1.90, 1.81, 1.81},
		{"no ask falls back to last", 1.83, 0, 1.81, 1.81},
		{"both zero falls back to last", 0, 0, 1.81, 1.81},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Mark(c.bid, c.ask, c.last); got != c.want {
				t.Errorf("Mark(%v, %v, %v) = %v, want %v", c.bid, c.ask, c.last, got, c.want)
			}
		})
	}
}
