package data

import (
	"strings"
	"testing"
)

type fakeNewsProvider struct {
	Provider
	items []NewsItem
	err   error
}

func (f *fakeNewsProvider) GetNews(string, int) ([]NewsItem, error) { return f.items, f.err }

type fakeMarketNewsProvider struct {
	items []NewsItem
	err   error
}

func (f *fakeMarketNewsProvider) GetMarketNews(int) ([]NewsItem, error) { return f.items, f.err }

func blockLowerSet(names ...string) func(string) bool {
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return func(source string) bool { return set[strings.ToLower(strings.TrimSpace(source))] }
}

func TestFilteredNewsProvider_GetNews(t *testing.T) {
	fake := &fakeNewsProvider{items: []NewsItem{
		{Headline: "a", Source: "Good Source"},
		{Headline: "b", Source: "Spammy Blog"},
	}}
	p := NewNewsFilter(fake, blockLowerSet("spammy blog"))

	got, err := p.GetNews("AAPL", 10)
	if err != nil {
		t.Fatalf("GetNews() err = %v, want nil", err)
	}
	if len(got) != 1 || got[0].Headline != "a" {
		t.Errorf("GetNews() = %+v, want only the non-blocked item", got)
	}
}

func TestFilteredNewsProvider_FilteredToEmptyIsNotAnError(t *testing.T) {
	fake := &fakeNewsProvider{items: []NewsItem{{Headline: "a", Source: "Spammy Blog"}}}
	p := NewNewsFilter(fake, blockLowerSet("spammy blog"))

	got, err := p.GetNews("AAPL", 10)
	if err != nil {
		t.Fatalf("GetNews() err = %v, want nil even when every item is filtered", err)
	}
	if len(got) != 0 {
		t.Errorf("GetNews() = %+v, want empty", got)
	}
}

func TestFilteredMarketNewsProvider_GetMarketNews(t *testing.T) {
	fake := &fakeMarketNewsProvider{items: []NewsItem{
		{Headline: "a", Source: "Good Source"},
		{Headline: "b", Source: "Spammy Blog"},
	}}
	p := NewMarketNewsFilter(fake, blockLowerSet("spammy blog"))

	got, err := p.GetMarketNews(10)
	if err != nil {
		t.Fatalf("GetMarketNews() err = %v, want nil", err)
	}
	if len(got) != 1 || got[0].Headline != "a" {
		t.Errorf("GetMarketNews() = %+v, want only the non-blocked item", got)
	}
}
