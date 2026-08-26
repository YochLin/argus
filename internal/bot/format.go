package bot

import (
	"sort"
	"strings"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/market"
	"argus/internal/render"
)

// formatChatContext renders the read-only background block prefixed to
// chat messages: each ticker's most recent close plus, for tickers actually
// held, cost basis and unrealized P&L against that close. tickers is the
// order to render in; positions/snapshots are keyed by ticker. Returns ""
// for an empty tickers list so callers can skip prefixing entirely.
func formatChatContext(lang i18n.Lang, tickers []string, positions map[string]db.Position, snapshots map[string]db.DailySnapshot) string {
	if len(tickers) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(i18n.T(lang, i18n.KeyChatContextHeader))
	for _, t := range tickers {
		snap, ok := snapshots[t]
		if !ok {
			sb.WriteString(i18n.T(lang, i18n.KeyChatContextTickerNoData, t))
			continue
		}
		if p, held := positions[t]; held {
			unrealizedPct := (snap.Close - p.AvgCost) / p.AvgCost * 100
			sb.WriteString(i18n.T(lang, i18n.KeyChatContextPositionLine, t, snap.Date, render.Money(t, snap.Close), snap.ChangePercent, p.Shares, render.Money(t, p.AvgCost), unrealizedPct))
		} else {
			sb.WriteString(i18n.T(lang, i18n.KeyChatContextWatchLine, t, snap.Date, render.Money(t, snap.Close), snap.ChangePercent))
		}
	}
	sb.WriteString(i18n.T(lang, i18n.KeyChatContextFooter))
	return sb.String()
}

// daysUntil returns the whole number of days from today (Taiwan time) until
// dateStr (YYYY-MM-DD), which may be negative for a past date. Both sides
// are compared as date-only values (not instants) so it's not sensitive to
// what time of day it's called.
func daysUntil(dateStr string) int {
	target, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return 0
	}
	today, _ := time.Parse("2006-01-02", time.Now().In(cst).Format("2006-01-02"))
	return int(target.Sub(today).Hours() / 24)
}

func formatQuote(lang i18n.Lang, q *data.Quote, label string) string {
	arrow := "▲"
	if q.ChangePercent < 0 {
		arrow = "▼"
	}
	return i18n.T(lang, i18n.KeyQuoteLine, label, q.Price, arrow, q.ChangePercent, q.Open, q.High, q.Low)
}

// companyName resolves ticker's display name via b.companyNames — "" (not
// an error) for a US ticker (already human-readable, no lookup needed), a
// nil provider (FINMIND_TOKEN unset), or a failed lookup, so every caller
// can treat "" as "no name available" without a separate error check. See
// data.CompanyNameProvider.
func (b *Bot) companyName(ticker string) string {
	if b.companyNames == nil || market.Of(ticker) != market.TW {
		return ""
	}
	name, err := b.companyNames.GetCompanyName(ticker)
	if err != nil {
		return ""
	}
	return name
}

// tickerLabel formats ticker for display as "name(ticker)" (e.g.
// "台積電(2330)") when a company name can be resolved, and returns ticker
// unchanged otherwise (US tickers, or a TW ticker with no resolvable name) —
// the single call site every user-facing message should go through instead
// of printing a bare ticker directly.
func (b *Bot) tickerLabel(ticker string) string {
	return data.TickerLabel(ticker, b.companyName(ticker))
}

// money is render.Money, exposed as a *Bot method so call sites read
// b.money(ticker, v) like the rest of this file's helpers.
func (b *Bot) money(ticker string, v float64) string {
	return render.Money(ticker, v)
}

func todayDate() string {
	return time.Now().In(cst).Format("2006-01-02")
}

// renderTrackSummary formats the hit-rate/avg-return/by-source/by-market
// breakdown — shared by /track's own display (handleTrack) and
// RunWeeklyReview's strategy-feedback block, which additionally asks the
// model to comment on it. Returns "" when nothing's been evaluated yet (no
// BUY/SELL row with a resolvable price), so callers can skip the block
// entirely rather than show an empty summary. byMarket (Phase 6 PR2 §5.3)
// mirrors bySource's own "only show the breakdown when there's more than one
// group" gate — a single-market user (the common case pre-Phase-6, and any
// TW-only or US-only holder) never sees a one-row market breakdown.
func renderTrackSummary(lang i18n.Lang, overall trackSourceStats, bySource map[string]trackSourceStats, byMarket map[market.MarketID]trackSourceStats) string {
	if overall.Evaluated == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(i18n.T(lang, i18n.KeyTrackSummary, overall.Hits, overall.Evaluated, overall.HitRate()))
	sb.WriteString(i18n.T(lang, i18n.KeyTrackAvgReturnLine, overall.AvgBuyPct(), overall.BuyCount, overall.AvgSellPct(), overall.SellCount))
	if len(bySource) > 1 {
		sb.WriteString(i18n.T(lang, i18n.KeyTrackBySourceHeader))
		for _, source := range sortedSourceKeys(bySource) {
			s := bySource[source]
			sb.WriteString(i18n.T(lang, i18n.KeyTrackBySourceLine, source, s.Hits, s.Evaluated, s.HitRate()))
		}
	}
	if len(byMarket) > 1 {
		sb.WriteString(i18n.T(lang, i18n.KeyTrackByMarketHeader))
		for _, m := range sortedMarketKeys(byMarket) {
			s := byMarket[m]
			sb.WriteString(i18n.T(lang, i18n.KeyTrackByMarketLine, string(m), s.Hits, s.Evaluated, s.HitRate()))
		}
	}
	return sb.String()
}

// sortedMarketKeys returns byMarket's keys in stable order (US before TW),
// mirroring sortedSourceKeys' role for the by-source breakdown.
func sortedMarketKeys(byMarket map[market.MarketID]trackSourceStats) []market.MarketID {
	keys := make([]market.MarketID, 0, len(byMarket))
	for k := range byMarket {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

// dedup returns tickers in a that are not present in b.
func dedup(a, b []string) []string {
	set := make(map[string]bool, len(b))
	for _, t := range b {
		set[t] = true
	}
	var out []string
	for _, t := range a {
		if !set[t] {
			out = append(out, t)
		}
	}
	return out
}
