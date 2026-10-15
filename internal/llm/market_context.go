package llm

import (
	"fmt"
	"strings"

	"argus/internal/data"
	"argus/internal/i18n"
	"argus/internal/render"
)

// MarketContext is Phase 3.7 追加項's broad-market regime block (see
// docs/phase-3.7-market-regime.md): SPY's own trend (vs its MA50/MA200) and
// ^VIX's level, so a BUY call in a bull market isn't held to the same bar as
// one in a bear market. Package-local, not imported from internal/data, same
// attach-and-render convention as Position/Earnings/Technicals. 0 on any
// field means "couldn't be fetched" — writeMarketContext skips that field's
// line rather than rendering a misleading 0, same sentinel convention as
// Technicals' MA fields. VolProxyPct is TW's VIX substitute (docs/... TW
// data-gap investigation, 2026-07-28: no TW volatility-index ticker/dataset
// exists, free or paid) — an ATR14/close percentage on the TW benchmark's
// own candles rather than an options-implied figure, so it's rendered with
// its own line and wording, never conflated with VIX.
type MarketContext struct {
	// Bench is the benchmark ticker SPYPrice/SPYMA50/SPYMA200 were computed
	// from ("SPY" for US, "0050" for TW — see service.BenchmarkFor) — the field
	// names predate TW support and stay SPY-named, but the rendered label
	// must say which benchmark this actually is.
	Bench                       string
	SPYPrice, SPYMA50, SPYMA200 float64
	VIX                         float64
	VolProxyPct                 float64
}

// vixCalmThreshold/vixPanicThreshold are fixed, not env-configurable — 15/25
// are the market's own conventional coarse VIX bands, not a user preference
// (see the design doc's rejected-alternatives section).
const (
	vixCalmThreshold  = 15
	vixPanicThreshold = 25
)

// IndexQuote is a plain label/price/change tuple for the morning briefing's
// broad-index summary (S&P 500/Nasdaq/Dow/Russell 2000 via their SPY/QQQ/
// DIA/IWM ETF proxies) — package-local like MarketContext, but deliberately
// not folded into it since MarketContext is SPY-only and used by the
// existing recommendation/check prompts. Label is a proper noun ("S&P 500"),
// not localized.
type IndexQuote struct {
	Label         string
	Price         float64
	ChangePercent float64
}

// regimeLabel returns the risk-on/risk-off i18n key for SPY's current price
// against its MA200 — the coarsest-grained trend fact the prompt can give the
// model. Pure so it's unit-testable without a full StockData/MarketContext.
func regimeLabel(price, ma200 float64) i18n.Key {
	if price > ma200 {
		return i18n.KeyRiskOn
	}
	return i18n.KeyRiskOff
}

// vixLabel returns the coarse VIX-band i18n key for a given VIX level (see
// vixCalmThreshold/vixPanicThreshold above).
func vixLabel(vix float64) i18n.Key {
	switch {
	case vix < vixCalmThreshold:
		return i18n.KeyVIXCalm
	case vix > vixPanicThreshold:
		return i18n.KeyVIXPanic
	default:
		return i18n.KeyVIXNormal
	}
}

// writeMarketContext renders the market-regime block (nil market, or a
// market with SPYPrice/SPYMA200, VIX, and VolProxyPct all unavailable,
// renders nothing — see MarketContext's 0-sentinel convention). Each line
// is independently skipped when its own inputs aren't available, same
// per-field degradation as writeStockSection's MA lines. VIX and
// VolProxyPct are mutually exclusive in practice (computeMarketRegime only
// ever sets one, depending on market) but aren't guarded against both being
// set here — rendering both would just be redundant, not wrong.
func writeMarketContext(sb *strings.Builder, lang i18n.Lang, m *MarketContext) {
	if m == nil {
		return
	}
	haveSPY := m.SPYPrice > 0 && m.SPYMA200 > 0
	haveVIX := m.VIX > 0
	haveVolProxy := m.VolProxyPct > 0
	if !haveSPY && !haveVIX && !haveVolProxy {
		return
	}

	sb.WriteString(i18n.T(lang, i18n.KeyMarketRegimeHeader))
	if haveSPY {
		fmt.Fprint(sb, i18n.T(lang, i18n.KeyMarketRegimeSPYLine, m.Bench, render.Money(m.Bench, m.SPYPrice), render.Money(m.Bench, m.SPYMA200), render.Money(m.Bench, m.SPYMA50), i18n.T(lang, regimeLabel(m.SPYPrice, m.SPYMA200))))
	}
	if haveVIX {
		fmt.Fprint(sb, i18n.T(lang, i18n.KeyMarketRegimeVIXLine, m.VIX, i18n.T(lang, vixLabel(m.VIX))))
	}
	if haveVolProxy {
		fmt.Fprint(sb, i18n.T(lang, i18n.KeyMarketRegimeVolProxyLine, m.VolProxyPct))
	}
	sb.WriteString("\n")
}

// buildMorningBriefingPrompt is the 07:00 CST US morning briefing's prompt
// (see bot.RunUSMorningBriefing), or, when isTW is true, the 08:30 CST TW
// pre-open briefing's prompt (see bot.RunTWMorningBriefing) — either way a
// narrative recap, distinct from buildRecommendationPrompt's BUY/SELL/HOLD
// framing. Reuses writeStockSection for the watchlist/movers sections the
// same way buildRecommendationPrompt does — StockData's per-field
// degradation means passing quote+news-only values (no
// Technicals/Fundamentals, see bot.loadQuoteHighlights) renders safely with
// no changes needed there.
func buildMorningBriefingPrompt(lang i18n.Lang, date string, indices []IndexQuote, vix float64, marketNews []data.NewsItem, watchlist []StockData, movers []StockData, isTW bool) string {
	var sb strings.Builder

	if isTW {
		sb.WriteString(i18n.T(lang, i18n.KeyTWMorningBriefingPromptIntro))
	} else {
		sb.WriteString(i18n.T(lang, i18n.KeyMorningBriefingPromptIntro))
	}

	sb.WriteString(i18n.T(lang, i18n.KeyMorningBriefingIndicesHeader))
	for _, idx := range indices {
		fmt.Fprint(&sb, i18n.T(lang, i18n.KeyMorningBriefingIndexLine, idx.Label, idx.Price, idx.ChangePercent))
	}
	if vix > 0 {
		fmt.Fprint(&sb, i18n.T(lang, i18n.KeyMorningBriefingVIXLine, vix))
	}
	sb.WriteString("\n")

	if len(marketNews) > 0 {
		sb.WriteString(i18n.T(lang, i18n.KeyMorningBriefingNewsHeader))
		for i, n := range marketNews {
			writeNewsItem(&sb, lang, i+1, n)
		}
		sb.WriteString("\n")
	}

	sb.WriteString(i18n.T(lang, i18n.KeyMorningBriefingWatchlistHeader))
	if len(watchlist) == 0 {
		sb.WriteString(i18n.T(lang, i18n.KeyRecNoWatchlist))
	} else {
		for _, s := range watchlist {
			writeStockSection(&sb, lang, s)
		}
	}

	sb.WriteString(i18n.T(lang, i18n.KeyMorningBriefingMoversHeader))
	if len(movers) == 0 {
		sb.WriteString(i18n.T(lang, i18n.KeyRecNoCandidates))
	} else {
		for _, s := range movers {
			writeStockSection(&sb, lang, s)
		}
	}

	if isTW {
		sb.WriteString(i18n.T(lang, i18n.KeyTWMorningBriefingTaskBlock, date))
	} else {
		sb.WriteString(i18n.T(lang, i18n.KeyMorningBriefingTaskBlock, date))
	}
	return sb.String()
}
