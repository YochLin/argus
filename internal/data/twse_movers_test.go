package data

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsTWETF(t *testing.T) {
	cases := []struct {
		ticker string
		want   bool
	}{
		{"2330", false},
		{"3481", false},
		{"0050", true},
		{"0056", true},
		{"00919", true},
		{"009816", true},
		{"00403A", true}, // plain (non-leveraged) ETF, live-verified 2026-07-29 to dominate movers too
		{"00685L", true}, // leveraged
		{"00632R", true}, // inverse
		{"00679B", true}, // bond ETF
	}
	for _, c := range cases {
		if got := isTWETF(c.ticker); got != c.want {
			t.Errorf("isTWETF(%q) = %v, want %v", c.ticker, got, c.want)
		}
	}
}

func TestTWSEGetMarketMovers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[
			{"Code":"3481","Name":"群創"},
			{"Code":"0050","Name":"元大台灣50"},
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
