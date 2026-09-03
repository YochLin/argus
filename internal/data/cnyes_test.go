package data

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCnyesGetMarketNews(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"items": {
				"data": [
					{"newsId": 6548969, "title": "台積電熊本廠震後逐步恢復營運", "summary": null, "source": "優分析", "publishAt": 1785250812},
					{"newsId": 6548888, "title": "欣興Q2純益131億元創高", "summary": "PCB 及 IC 載板欣興公布最新獲利資訊", "source": "鉅亨網", "publishAt": 1785240000}
				]
			}
		}`))
	}))
	defer srv.Close()

	c := NewCnyes()
	c.baseURL = srv.URL

	items, err := c.GetMarketNews(10)
	if err != nil {
		t.Fatalf("GetMarketNews: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("GetMarketNews() = %d items, want 2", len(items))
	}
	if items[0].Headline != "台積電熊本廠震後逐步恢復營運" {
		t.Errorf("items[0].Headline = %q", items[0].Headline)
	}
	if items[0].Summary != "" {
		t.Errorf("items[0].Summary = %q, want \"\" for a null summary", items[0].Summary)
	}
	if items[0].URL != "https://news.cnyes.com/news/id/6548969" {
		t.Errorf("items[0].URL = %q, want the canonical cnyes article URL", items[0].URL)
	}
	if items[1].Summary == "" {
		t.Error("items[1].Summary is empty, want the real teaser text preserved")
	}
}

func TestCnyesGetMarketNews_LimitTruncates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":{"data":[{"newsId":1,"title":"a"},{"newsId":2,"title":"b"},{"newsId":3,"title":"c"}]}}`))
	}))
	defer srv.Close()

	c := NewCnyes()
	c.baseURL = srv.URL

	items, err := c.GetMarketNews(2)
	if err != nil {
		t.Fatalf("GetMarketNews: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("GetMarketNews(2) = %d items, want 2", len(items))
	}
}

// TestCnyesGetMarketNews_ContentFallback covers Phase 19 後續 PR5-1: an item
// with no summary but a real (HTML-encoded) content body should get a
// stripped, truncated stand-in summary instead of staying blank.
func TestCnyesGetMarketNews_ContentFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":{"data":[{"newsId":1,"title":"a","summary":"","content":"&lt;p&gt;鴻海劉揚偉表示&lt;/p&gt;","source":"鉅亨網"}]}}`))
	}))
	defer srv.Close()

	c := NewCnyes()
	c.baseURL = srv.URL

	items, err := c.GetMarketNews(10)
	if err != nil {
		t.Fatalf("GetMarketNews: %v", err)
	}
	if want := "鴻海劉揚偉表示"; items[0].Summary != want {
		t.Errorf("items[0].Summary = %q, want %q (HTML stripped, entities unescaped)", items[0].Summary, want)
	}
}

func TestCnyesGetMarketNews_ContentFallbackTruncates(t *testing.T) {
	long := ""
	for i := 0; i < 300; i++ {
		long += "字"
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"items":{"data":[{"newsId":1,"title":"a","summary":"","content":%q}]}}`, "<p>"+long+"</p>")
	}))
	defer srv.Close()

	c := NewCnyes()
	c.baseURL = srv.URL

	items, err := c.GetMarketNews(10)
	if err != nil {
		t.Fatalf("GetMarketNews: %v", err)
	}
	// cnyesSummaryFallbackMaxRunes runes plus the "…" truncation marker.
	if got := len([]rune(items[0].Summary)); got != cnyesSummaryFallbackMaxRunes+1 {
		t.Errorf("Summary rune length = %d, want %d (%d + ellipsis)", got, cnyesSummaryFallbackMaxRunes+1, cnyesSummaryFallbackMaxRunes)
	}
}

// cnyesPagedTestServer serves cnyesStockNewsPages pages of the category feed,
// one item per page tagged to a distinct ticker's market[], for GetNews's
// reverse-index tests below. It also counts requests so the cache test can
// assert a second call within the TTL doesn't refetch.
func cnyesPagedTestServer(t *testing.T, requests *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests != nil {
			*requests++
		}
		page := r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "application/json")
		switch page {
		case "1":
			fmt.Fprint(w, `{"items":{"data":[{"newsId":1,"title":"台積電法說會重點","summary":"重點摘要","source":"鉅亨網","market":[{"code":"2330"}]}]}}`)
		case "2":
			fmt.Fprint(w, `{"items":{"data":[{"newsId":2,"title":"鴻海與輝達合作深化","summary":"合作內容","source":"鉅亨網","market":[{"code":"2317"},{"code":"NVDA"}]}]}}`)
		default:
			fmt.Fprint(w, `{"items":{"data":[]}}`)
		}
	}))
}

func TestCnyesGetNews_TagReverseIndex(t *testing.T) {
	srv := cnyesPagedTestServer(t, nil)
	defer srv.Close()

	c := NewCnyes()
	c.baseURL = srv.URL

	items, err := c.GetNews("2330", 10)
	if err != nil {
		t.Fatalf("GetNews(2330): %v", err)
	}
	if len(items) != 1 || items[0].Headline != "台積電法說會重點" {
		t.Fatalf("GetNews(2330) = %+v, want the tagged 2330 item", items)
	}

	// A TW item tagged with a US ticker too (Phase 19 後續 PR5-1: US codes
	// are kept in RelatedTickers, not stripped) surfaces under its TW
	// ticker with NVDA still listed — GetNews itself stays TW-gated, so
	// "NVDA" is never a directly queryable key here, just a tag carried on
	// the 2317 item for the prompt renderer.
	items, err = c.GetNews("2317", 10)
	if err != nil {
		t.Fatalf("GetNews(2317): %v", err)
	}
	if len(items) != 1 || items[0].Headline != "鴻海與輝達合作深化" {
		t.Fatalf("GetNews(2317) = %+v, want the cross-tagged item", items)
	}
	if want := []string{"2317", "NVDA"}; !equalStrSlices(items[0].RelatedTickers, want) {
		t.Errorf("RelatedTickers = %v, want %v", items[0].RelatedTickers, want)
	}
}

func equalStrSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestCnyesGetNews_UntaggedTickerReturnsNilNoError(t *testing.T) {
	srv := cnyesPagedTestServer(t, nil)
	defer srv.Close()

	c := NewCnyes()
	c.baseURL = srv.URL

	items, err := c.GetNews("9999", 10)
	if err != nil {
		t.Fatalf("GetNews(9999): want nil error (no fallback per §5.1), got %v", err)
	}
	if items != nil {
		t.Errorf("GetNews(9999) = %v, want nil", items)
	}
}

func TestCnyesGetNews_NonTWReturnsError(t *testing.T) {
	c := NewCnyes()
	if _, err := c.GetNews("AAPL", 10); err != errCnyesNotTW {
		t.Errorf("GetNews(AAPL) error = %v, want errCnyesNotTW", err)
	}
}

func TestCnyesGetNews_CachesWithinTTL(t *testing.T) {
	var requests int
	srv := cnyesPagedTestServer(t, &requests)
	defer srv.Close()

	c := NewCnyes()
	c.baseURL = srv.URL

	if _, err := c.GetNews("2330", 10); err != nil {
		t.Fatalf("GetNews: %v", err)
	}
	firstRequests := requests
	if firstRequests != cnyesStockNewsPages {
		t.Fatalf("requests after first call = %d, want %d (one per page)", firstRequests, cnyesStockNewsPages)
	}

	if _, err := c.GetNews("2317", 10); err != nil {
		t.Fatalf("GetNews: %v", err)
	}
	if requests != firstRequests {
		t.Errorf("requests after second call (within TTL) = %d, want unchanged %d", requests, firstRequests)
	}
}
