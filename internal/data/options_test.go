package data

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const optionChainBody = `{"optionChain":{"result":[{"expirationDates":[1785888000,1786492800],"options":[{"calls":[{"contractSymbol":"AAPL260805C00310000","strike":310.0,"lastPrice":1.81,"volume":145858,"openInterest":9533,"bid":1.83,"ask":1.9,"expiration":1785888000,"lastTradeDate":1785873598,"impliedVolatility":0.3396,"inTheMoney":false}],"puts":[{"contractSymbol":"AAPL260805P00310000","strike":310.0,"lastPrice":2.1,"volume":500,"openInterest":100,"bid":2.05,"ask":2.15,"expiration":1785888000,"lastTradeDate":1785873598,"impliedVolatility":0.31,"inTheMoney":true}]}]}],"error":null}}`

// crumbAwareOptionServer serves the crumb handshake (a cookie-setting
// fc.yahoo.com stand-in plus /v1/test/getcrumb) and the v7 option chain
// endpoint, requiring a matching ?crumb= on the chain request — same
// contract as live Yahoo (§2.2 of docs/phase-12-options.md).
func crumbAwareOptionServer(t *testing.T, hits map[string]int) *httptest.Server {
	t.Helper()
	const validCrumb = "test-crumb-123"
	var mux http.ServeMux
	mux.HandleFunc("/v1/test/getcrumb", func(w http.ResponseWriter, r *http.Request) {
		hits["getcrumb"]++
		fmt.Fprint(w, validCrumb)
	})
	mux.HandleFunc("/v7/finance/options/AAPL", func(w http.ResponseWriter, r *http.Request) {
		hits["chain"]++
		if r.URL.Query().Get("crumb") != validCrumb {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"error":{"code":"Unauthorized","description":"Invalid Crumb"}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, optionChainBody)
	})
	return httptest.NewServer(&mux)
}

func TestGetOptionChain(t *testing.T) {
	hits := map[string]int{}
	srv := crumbAwareOptionServer(t, hits)
	defer srv.Close()

	y := NewYahoo()
	y.chartBaseURL = srv.URL
	y.fcBaseURL = srv.URL

	quotes, err := y.GetOptionChain("AAPL", time.Unix(1785888000, 0))
	if err != nil {
		t.Fatalf("GetOptionChain: %v", err)
	}
	if len(quotes) != 2 {
		t.Fatalf("got %d quotes, want 2", len(quotes))
	}
	if quotes[0].ContractSymbol != "AAPL260805C00310000" || quotes[0].Right != "C" {
		t.Errorf("quotes[0] = %+v, want the call", quotes[0])
	}
	if quotes[1].ContractSymbol != "AAPL260805P00310000" || quotes[1].Right != "P" {
		t.Errorf("quotes[1] = %+v, want the put", quotes[1])
	}
	if hits["getcrumb"] != 1 {
		t.Errorf("getcrumb hits = %d, want 1 (crumb should be cached)", hits["getcrumb"])
	}

	// Second call reuses the cached crumb — no extra getcrumb hit.
	if _, err := y.GetOptionChain("AAPL", time.Unix(1785888000, 0)); err != nil {
		t.Fatalf("GetOptionChain (cached crumb): %v", err)
	}
	if hits["getcrumb"] != 1 {
		t.Errorf("getcrumb hits after second call = %d, want still 1", hits["getcrumb"])
	}
}

func TestGetOptionChain_StaleCrumbRetriesOnce(t *testing.T) {
	hits := map[string]int{}
	srv := crumbAwareOptionServer(t, hits)
	defer srv.Close()

	y := NewYahoo()
	y.chartBaseURL = srv.URL
	y.fcBaseURL = srv.URL
	y.crumb = "stale-crumb" // simulate an already-cached, now-invalid crumb

	quotes, err := y.GetOptionChain("AAPL", time.Unix(1785888000, 0))
	if err != nil {
		t.Fatalf("GetOptionChain: %v", err)
	}
	if len(quotes) != 2 {
		t.Fatalf("got %d quotes, want 2", len(quotes))
	}
	if hits["getcrumb"] != 1 {
		t.Errorf("getcrumb hits = %d, want exactly 1 refetch after the 401", hits["getcrumb"])
	}
	if hits["chain"] != 2 {
		t.Errorf("chain hits = %d, want 2 (stale attempt + retry)", hits["chain"])
	}
}

func TestGetOptionExpirations(t *testing.T) {
	hits := map[string]int{}
	srv := crumbAwareOptionServer(t, hits)
	defer srv.Close()

	y := NewYahoo()
	y.chartBaseURL = srv.URL
	y.fcBaseURL = srv.URL

	dates, err := y.GetOptionExpirations("AAPL")
	if err != nil {
		t.Fatalf("GetOptionExpirations: %v", err)
	}
	if len(dates) != 2 {
		t.Fatalf("got %d dates, want 2", len(dates))
	}
}

func TestGetOptionChain_TWTickerReturnsNil(t *testing.T) {
	y := NewYahoo()
	quotes, err := y.GetOptionChain("2330", time.Unix(1785888000, 0))
	if err != nil || quotes != nil {
		t.Errorf("GetOptionChain(2330) = (%v, %v), want (nil, nil)", quotes, err)
	}
	dates, err := y.GetOptionExpirations("2330")
	if err != nil || dates != nil {
		t.Errorf("GetOptionExpirations(2330) = (%v, %v), want (nil, nil)", dates, err)
	}
}
