package data

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// fakeCompanyNames is a CompanyNameProvider stub — name is returned for
// every ticker, or err when set, so a test can pick the resolved/failed/
// absent-provider branch of GoogleNews.query.
type fakeCompanyNames struct {
	name string
	err  error
}

func (f fakeCompanyNames) GetCompanyName(string) (string, error) { return f.name, f.err }

const googleNewsFeedXML = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
<channel>
	<title>"台積電 when:7d" - Google 新聞</title>
	<item>
		<title>護國神山不行了？華爾街大咖「狂賣台積電」外媒曝真相 - Yahoo股市</title>
		<link>https://news.google.com/rss/articles/CBMi5wJBVV95cUxO</link>
		<guid isPermaLink="false">CBMi5wJBVV95cUxO</guid>
		<pubDate>Thu, 30 Jul 2026 09:54:20 GMT</pubDate>
		<description>&lt;a href="https://news.google.com/rss/articles/CBMi5wJBVV95cUxO"&gt;護國神山不行了？&lt;/a&gt;</description>
		<source url="https://tw.stock.yahoo.com">Yahoo股市</source>
	</item>
	<item>
		<title>台積電：熊本廠需調整校正已調派支援全面檢視地震影響 - 中央社 CNA</title>
		<link>https://news.google.com/rss/articles/CBMiabcdefg</link>
		<pubDate>Wed, 29 Jul 2026 12:58:00 GMT</pubDate>
		<source url="https://www.cna.com.tw">中央社 CNA</source>
	</item>
	<item>
		<title>焦點股 - 台積電 - 跳水重挫</title>
		<link>https://news.google.com/rss/articles/CBMihijklmn</link>
		<pubDate>not a date</pubDate>
		<source url="https://ec.ltn.com.tw">自由財經</source>
	</item>
	<item>
		<title>台積電股價攻頂！史上3次收漲停紀錄一次看 - ETtoday財經雲</title>
		<link>https://news.google.com/rss/articles/CBMiUkFVX3lx</link>
		<pubDate>Fri, 31 Jul 2026 07:03:00 GMT</pubDate>
		<source url="https://finance.ettoday.net">ETtoday財經雲</source>
	</item>
</channel>
</rss>`

func newGoogleNewsTestServer(t *testing.T, gotQuery *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gotQuery != nil {
			*gotQuery = r.URL.Query().Get("q")
		}
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(googleNewsFeedXML))
	}))
}

func TestGoogleNewsGetNews(t *testing.T) {
	var gotQuery string
	srv := newGoogleNewsTestServer(t, &gotQuery)
	defer srv.Close()

	g := NewGoogleNews(fakeCompanyNames{name: "台積電"})
	g.baseURL = srv.URL

	items, err := g.GetNews("2330", 10)
	if err != nil {
		t.Fatalf("GetNews: %v", err)
	}
	if len(items) != 4 {
		t.Fatalf("GetNews() = %d items, want 4", len(items))
	}

	// The feed arrives relevance-ordered, so the newest item is last in the
	// XML and must come back first.
	if want := "台積電股價攻頂！史上3次收漲停紀錄一次看"; items[0].Headline != want {
		t.Errorf("items[0].Headline = %q, want the newest item (%q) sorted to the front", items[0].Headline, want)
	}

	if want := "護國神山不行了？華爾街大咖「狂賣台積電」外媒曝真相"; items[1].Headline != want {
		t.Errorf("items[1].Headline = %q, want the \" - Publisher\" suffix stripped (%q)", items[1].Headline, want)
	}
	if items[1].Source != "Yahoo股市" {
		t.Errorf("items[1].Source = %q, want the <source> element's text", items[1].Source)
	}
	if items[1].URL != "https://news.google.com/rss/articles/CBMi5wJBVV95cUxO" {
		t.Errorf("items[1].URL = %q", items[1].URL)
	}
	if items[1].Summary != "" {
		t.Errorf("items[1].Summary = %q, want \"\" — the <description> is a link wrapper, not a teaser", items[1].Summary)
	}
	if want := time.Date(2026, 7, 30, 9, 54, 20, 0, time.UTC); !items[1].PublishedAt.Equal(want) {
		t.Errorf("items[1].PublishedAt = %v, want %v", items[1].PublishedAt, want)
	}

	// " - " inside a headline is only stripped when the suffix is the
	// publisher's own name, so this one keeps its embedded dashes. Its
	// unparseable pubDate also sorts it last rather than first.
	if want := "焦點股 - 台積電 - 跳水重挫"; items[3].Headline != want {
		t.Errorf("items[3].Headline = %q, want %q left intact and sorted last", items[3].Headline, want)
	}
	if !items[3].PublishedAt.IsZero() {
		t.Errorf("items[3].PublishedAt = %v, want the zero time for an unparseable pubDate", items[3].PublishedAt)
	}
}

// Truncation happens after the recency sort, so limit keeps the newest
// items rather than the most "relevant" ones the feed happened to lead with.
func TestGoogleNewsGetNews_LimitKeepsNewest(t *testing.T) {
	srv := newGoogleNewsTestServer(t, nil)
	defer srv.Close()

	g := NewGoogleNews(nil)
	g.baseURL = srv.URL

	items, err := g.GetNews("2330", 2)
	if err != nil {
		t.Fatalf("GetNews: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("GetNews(2) = %d items, want 2", len(items))
	}
	want := []string{
		"台積電股價攻頂！史上3次收漲停紀錄一次看",
		"護國神山不行了？華爾街大咖「狂賣台積電」外媒曝真相",
	}
	for i, w := range want {
		if items[i].Headline != w {
			t.Errorf("items[%d].Headline = %q, want %q", i, items[i].Headline, w)
		}
	}
}

// A US ticker must fail before any request goes out: GoogleNews sits ahead
// of Yahoo in the Multi chain, so answering one would change the existing
// Finnhub-then-Yahoo US news path.
func TestGoogleNewsGetNews_USTickerFailsWithoutRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("GetNews made a request for a US ticker")
	}))
	defer srv.Close()

	g := NewGoogleNews(fakeCompanyNames{name: "台積電"})
	g.baseURL = srv.URL

	if _, err := g.GetNews("AAPL", 5); !errors.Is(err, errGoogleNewsNotTW) {
		t.Fatalf("GetNews(AAPL) error = %v, want errGoogleNewsNotTW", err)
	}
}

func TestGoogleNewsQuery(t *testing.T) {
	tests := []struct {
		name  string
		names CompanyNameProvider
		want  string
	}{
		{"resolved name", fakeCompanyNames{name: "台積電"}, "台積電 when:7d"},
		{"no provider", nil, "2330 when:7d"},
		{"lookup failed", fakeCompanyNames{err: errors.New("boom")}, "2330 when:7d"},
		{"empty name", fakeCompanyNames{name: ""}, "2330 when:7d"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotQuery string
			srv := newGoogleNewsTestServer(t, &gotQuery)
			defer srv.Close()

			g := NewGoogleNews(tt.names)
			g.baseURL = srv.URL

			if _, err := g.GetNews("2330", 5); err != nil {
				t.Fatalf("GetNews: %v", err)
			}
			if gotQuery != tt.want {
				t.Errorf("q = %q, want %q", gotQuery, tt.want)
			}
		})
	}
}

func TestGoogleNewsLocalizationParams(t *testing.T) {
	var got map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = map[string]string{
			"hl":   r.URL.Query().Get("hl"),
			"gl":   r.URL.Query().Get("gl"),
			"ceid": r.URL.Query().Get("ceid"),
		}
		w.Write([]byte(googleNewsFeedXML))
	}))
	defer srv.Close()

	g := NewGoogleNews(nil)
	g.baseURL = srv.URL

	if _, err := g.GetNews("2330", 5); err != nil {
		t.Fatalf("GetNews: %v", err)
	}
	// Without these the endpoint answers with the US English edition, which
	// is the coverage Yahoo already provides — the whole point of this
	// provider is the zh-TW edition.
	want := map[string]string{"hl": "zh-TW", "gl": "TW", "ceid": "TW:zh-Hant"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestGoogleNewsGetNews_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	g := NewGoogleNews(nil)
	g.baseURL = srv.URL

	if _, err := g.GetNews("2330", 5); err == nil {
		t.Fatal("GetNews() error = nil, want an error so Multi falls through to Yahoo")
	}
}
