package mcptools

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"argus/internal/data"
	"argus/internal/i18n"
	"argus/internal/signals"
)

// technicalsBollingerPeriod/NumStdDev mirror bot.computeTechnicals' own
// bollingerPeriod/bollingerNumStdDev constants, duplicated here rather than
// imported since this package must not depend on internal/bot (see
// registerTools' doc comment and CLAUDE.md's internal/mcptools entry).
// technicalsBenchmarkTicker mirrors bot.go's unexported benchmarkTicker for
// the same reason, backing the RS63-vs-SPY line below.
const (
	technicalsBollingerPeriod    = 20
	technicalsBollingerNumStdDev = 2.0
	technicalsBenchmarkTicker    = "SPY"
)

// getTechnicals reduces ~1y of daily candles (the same GetHistory call
// get_history exposes raw) to the RSI/MACD/moving-average/volume/volatility
// indicator set bot.computeTechnicals already feeds the recommendation
// prompt with — added because a chat model given only get_history has no
// tool for "what's the 50-day moving average" short of eyeballing ~250
// closes itself. There is no VWAP here: this project has no intraday bar
// data (see CLAUDE.md's internal/data entry on Finnhub's free-tier
// /stock/candle block and Yahoo's daily-only GetHistory), so a real
// session VWAP isn't something this tool can compute.
func (ts *toolset) getTechnicals(ctx context.Context, _ *mcp.CallToolRequest, in tickerInput) (*mcp.CallToolResult, any, error) {
	ticker := normalizeTicker(in.Ticker)
	result, err := ts.withCache(ctx, "get_technicals:"+ticker, longCacheTTL, func() (*mcp.CallToolResult, error) {
		candles, err := ts.history.GetHistory(ticker, "1y")
		if err != nil || len(candles) == 0 {
			return nil, ts.mcpErr(i18n.KeyMCPNoHistory, ticker)
		}

		// RS63 needs a benchmark series; degrade to skipping that one line
		// (same as bot.fetchStockData when SPY history can't be fetched)
		// rather than failing the whole tool call over it.
		var spyCloses []float64
		if ticker != technicalsBenchmarkTicker {
			if spyCandles, err := ts.history.GetHistory(technicalsBenchmarkTicker, "1y"); err == nil {
				spyCloses = data.Closes(spyCandles)
			}
		}

		text := i18n.T(ts.lang, i18n.KeyMCPTickerHeader, ticker) + formatTechnicals(ts.lang, candles, spyCloses)
		return textResult(text), nil
	})
	return result, nil, err
}

// formatTechnicals renders the same lines internal/llm's writeStockSection
// does for StockData.Technicals, through the same i18n keys — this package
// can't import internal/llm (registerTools' doc comment), so the rendering
// is necessarily duplicated rather than shared, same
// can't-share-an-import shape as formatFundamentals before the
// internal/render extraction (see CLAUDE.md's internal/mcptools entry).
func formatTechnicals(lang i18n.Lang, candles []data.Candle, spyCloses []float64) string {
	closes := data.Closes(candles)
	highs := data.Highs(candles)
	lows := data.Lows(candles)
	volumes := data.Volumes(candles)
	price := closes[len(closes)-1]

	var sb strings.Builder

	macdLabel := i18n.T(lang, i18n.KeyTrendUnknown)
	switch signals.MACDTrend(closes) {
	case "bullish":
		macdLabel = i18n.T(lang, i18n.KeyTrendBullish)
	case "bearish":
		macdLabel = i18n.T(lang, i18n.KeyTrendBearish)
	}
	sb.WriteString(i18n.T(lang, i18n.KeyTechnicalsSummaryLine, signals.RSI(closes, 14), macdLabel))

	if k, d := signals.StochasticSeries(highs, lows, closes, 9, 3); len(k) > 0 {
		sb.WriteString(i18n.T(lang, i18n.KeyTechnicalsKDLine, k[len(k)-1], d[len(d)-1]))
	}

	// Each MA is only rendered when there's enough history to compute it —
	// signals.MA returns 0 as a sentinel otherwise (e.g. MA200 on a recent
	// IPO), and 0 would misleadingly look like a real price level.
	for _, ma := range []struct {
		period int
		value  float64
	}{
		{5, signals.MA(closes, 5)},
		{20, signals.MA(closes, 20)},
		{50, signals.MA(closes, 50)},
		{60, signals.MA(closes, 60)},
		{200, signals.MA(closes, 200)},
	} {
		if ma.value > 0 {
			sb.WriteString(i18n.T(lang, i18n.KeyTechnicalsMALine, maLabel(lang, price, ma.value), ma.period, ma.value))
		}
	}

	if align := signals.MAAlignment(closes); align != "" {
		alignLabel := i18n.T(lang, i18n.KeyTrendUnknown)
		if align == "bullish" {
			alignLabel = i18n.T(lang, i18n.KeyTrendBullish)
		} else if align == "bearish" {
			alignLabel = i18n.T(lang, i18n.KeyTrendBearish)
		}
		sb.WriteString(i18n.T(lang, i18n.KeyTechnicalsMAAlignLine, alignLabel))
	}

	if vr := signals.VolumeRatio(volumes, 20); vr > 0 {
		sb.WriteString(i18n.T(lang, i18n.KeyVolumeRatioLine, vr))
	}

	if vp := signals.VolumePriceSignal(closes, volumes); vp != "" {
		vpKey := i18n.KeyVolUpPriceUp
		switch vp {
		case "vol_down_price_down":
			vpKey = i18n.KeyVolDownPriceDown
		case "vol_down_price_up":
			vpKey = i18n.KeyVolDownPriceUp
		case "vol_up_price_down":
			vpKey = i18n.KeyVolUpPriceDown
		}
		sb.WriteString(i18n.T(lang, i18n.KeyTechnicalsVolumePriceLine, i18n.T(lang, vpKey)))
	}

	newHigh20 := signals.IsNewHigh(closes, 20)
	newHigh52w := signals.IsNewHigh(closes, len(closes))
	if newHigh20 || newHigh52w {
		nhKey := i18n.KeyNewHigh20
		if newHigh20 && newHigh52w {
			nhKey = i18n.KeyNewHigh20And52
		} else if newHigh52w {
			nhKey = i18n.KeyNewHigh52
		}
		sb.WriteString(i18n.T(lang, i18n.KeyTechnicalsNewHighLine, i18n.T(lang, nhKey)))
	}

	if len(closes) >= 26+9 {
		macdLine, _, _ := signals.MACD(closes)
		mzKey := i18n.KeyMACDAboveZero
		if macdLine < 0 {
			mzKey = i18n.KeyMACDBelowZero
		}
		sb.WriteString(i18n.T(lang, i18n.KeyTechnicalsMACDZeroLine, i18n.T(lang, mzKey), macdLine))
	}

	if rs, ok := signals.RelativeStrength(closes, spyCloses, 63); ok {
		rsKey := i18n.KeyRSStronger
		if rs < 0 {
			rsKey = i18n.KeyRSWeaker
		}
		sb.WriteString(i18n.T(lang, i18n.KeyTechnicalsRSLine, i18n.T(lang, rsKey), rs))
	}

	if atr := signals.ATR(highs, lows, closes, 14); atr > 0 && price > 0 {
		sb.WriteString(i18n.T(lang, i18n.KeyATRLine, atr, atr/price*100))
	}

	if bw := signals.BollingerBandwidthSeries(closes, technicalsBollingerPeriod, technicalsBollingerNumStdDev); len(bw) > 0 {
		sb.WriteString(i18n.T(lang, i18n.KeyTechnicalsBandwidthLine, bw[len(bw)-1]*100))
	}

	if pctB, ok := signals.BollingerPctB(closes, technicalsBollingerPeriod, technicalsBollingerNumStdDev); ok {
		sb.WriteString(i18n.T(lang, i18n.KeyBollingerLine, pctB*100))
	}

	return sb.String()
}

// maLabel mirrors internal/llm's unexported helper of the same name —
// small enough that duplicating it beats importing internal/llm for one
// two-line function (same reasoning as formatTechnicals' doc comment).
func maLabel(lang i18n.Lang, price, ma float64) string {
	if price > ma {
		return i18n.T(lang, i18n.KeyAboveMA)
	}
	return i18n.T(lang, i18n.KeyBelowMA)
}
