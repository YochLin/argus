package data

// NewsFilter wraps a Provider and drops news items whose Source is blocked,
// without changing GetQuote/GetMarketMovers or triggering Multi's fallback
// semantics (a filtered-to-empty result is still a nil error, not a failure)
// — see docs/phase-19-llm-transparency.md §5. blocked is a closure rather
// than a snapshot set so a block/unblock in the DB takes effect on the very
// next call, and so this package never has to import internal/db.
type NewsFilter struct {
	Provider
	blocked func(source string) bool
}

func NewNewsFilter(p Provider, blocked func(source string) bool) *NewsFilter {
	return &NewsFilter{Provider: p, blocked: blocked}
}

func (f *NewsFilter) GetNews(ticker string, limit int) ([]NewsItem, error) {
	items, err := f.Provider.GetNews(ticker, limit)
	if err != nil {
		return nil, err
	}
	return filterBlockedSources(items, f.blocked), nil
}

// MarketNewsFilter is NewsFilter's MarketNewsProvider counterpart (Phase 19
// §5) — a separate type since GetMarketNews isn't part of Provider.
type MarketNewsFilter struct {
	MarketNewsProvider
	blocked func(source string) bool
}

func NewMarketNewsFilter(p MarketNewsProvider, blocked func(source string) bool) *MarketNewsFilter {
	return &MarketNewsFilter{MarketNewsProvider: p, blocked: blocked}
}

func (f *MarketNewsFilter) GetMarketNews(limit int) ([]NewsItem, error) {
	items, err := f.MarketNewsProvider.GetMarketNews(limit)
	if err != nil {
		return nil, err
	}
	return filterBlockedSources(items, f.blocked), nil
}

// filterBlockedSources drops items whose Source the caller's blocked closure
// flags. blocked is responsible for its own case-insensitive/trimmed
// comparison — this stays a dumb per-item filter.
func filterBlockedSources(items []NewsItem, blocked func(source string) bool) []NewsItem {
	if blocked == nil {
		return items
	}
	out := make([]NewsItem, 0, len(items))
	for _, item := range items {
		if blocked(item.Source) {
			continue
		}
		out = append(out, item)
	}
	return out
}
