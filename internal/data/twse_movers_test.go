package data

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsTWLeveragedOrInverse(t *testing.T) {
	cases := []struct {
		ticker string
		want   bool
	}{
		{"2330", false},
		{"0050", false},
		{"00685L", true},
		{"00632R", true},
		{"00679B", false}, // bond ETF, out of scope for this filter
	}
	for _, c := range cases {
		if got := isTWLeveragedOrInverse(c.ticker); got != c.want {
			t.Errorf("isTWLeveragedOrInverse(%q) = %v, want %v", c.ticker, got, c.want)
		}
	}
}

func TestTWSEGetMarketMovers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[
			{"Code":"3481","Name":"群創"},
			{"Code":"00685L","Name":"群益臺灣加權正2"},
			{"Code":"00632R","Name":"元大台灣50反1"},
			{"Code":"2330","Name":"台積電"}
		]`)
	}))
	defer srv.Close()

	twse := NewTWSE()
	twse.baseURL = srv.URL

	got, err := twse.GetMarketMovers()
	if err != nil {
		t.Fatalf("GetMarketMovers: %v", err)
	}
	want := []string{"3481", "2330"}
	if len(got) != len(want) {
		t.Fatalf("GetMarketMovers() = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("GetMarketMovers()[%d] = %q, want %q", i, got[i], w)
		}
	}
}

func TestTWSEGetMarketMovers_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	twse := NewTWSE()
	twse.baseURL = srv.URL

	if _, err := twse.GetMarketMovers(); err == nil {
		t.Error("GetMarketMovers() with 500 status = nil error, want an error")
	}
}
