package data

import (
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
