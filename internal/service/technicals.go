package service

import (
	"argus/internal/data"
	"argus/internal/llm"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/signals"
)

// Bollinger/candle-window constants for ComputeTechnicals — moved verbatim
// from bot.bollingerPeriod/bollingerNumStdDev/promptCandleCount (Phase 24
// Stage 1 Recommendation Pipeline Service extraction).
const (
	BollingerPeriod    = 20
	BollingerNumStdDev = 2.0
	PromptCandleCount  = 60
)

// ComputeTechnicals fetches ticker's daily-candle history and reduces it to
// the RSI/MACD/moving-average values an LLM prompt needs (see
// llm.Technicals), plus the most recent PromptCandleCount raw candles for
// llm.StockData.Candles — both from the one GetHistory call, so the K-line
// context costs no extra fetch. Returns nils (not an error) on a
// history-fetch failure, so callers degrade the same way the fundamentals
// fetch does. Moved verbatim from bot.computeTechnicals.
func ComputeTechnicals(history RiskHistoryReader, ticker string, spyCloses []float64) (*llm.Technicals, []data.Candle, []llm.StrategyHitInfo) {
	candles, err := history.GetHistory(ticker, "1y")
	if err != nil {
		logger.Errorf("history %s: %v", ticker, err)
		return nil, nil, nil
	}
	closes := data.Closes(candles)
	highs := data.Highs(candles)
	lows := data.Lows(candles)
	volumes := data.Volumes(candles)

	t := &llm.Technicals{
		RSI14:       signals.RSI(closes, 14),
		MACDTrend:   signals.MACDTrend(closes),
		MA5:         signals.MA(closes, 5),
		MA20:        signals.MA(closes, 20),
		MA50:        signals.MA(closes, 50),
		MA60:        signals.MA(closes, 60),
		MA200:       signals.MA(closes, 200),
		VolumeRatio: signals.VolumeRatio(volumes, 20),
		ATR14:       signals.ATR(highs, lows, closes, 14),
		MAAlign:     signals.MAAlignment(closes),
		VolumePrice: signals.VolumePriceSignal(closes, volumes),
		NewHigh20:   signals.IsNewHigh(closes, 20),
		NewHigh52w:  signals.IsNewHigh(closes, len(closes)),
	}

	if len(closes) >= 26+9 {
		macdLine, _, _ := signals.MACD(closes)
		t.MACDAboveZero = &macdLine
	}

	if k, d := signals.StochasticSeries(highs, lows, closes, 9, 3); k != nil && len(k) > 0 {
		t.StochK = &k[len(k)-1]
		t.StochD = &d[len(d)-1]
	}

	if bw := signals.BollingerBandwidthSeries(closes, BollingerPeriod, BollingerNumStdDev); bw != nil && len(bw) > 0 {
		t.Bandwidth = &bw[len(bw)-1]
	}

	if rs, ok := signals.RelativeStrength(closes, spyCloses, 63); ok {
		t.RS63 = &rs
	}

	if len(volumes) > 0 {
		t.Volume = volumes[len(volumes)-1]
	}
	if pctB, ok := signals.BollingerPctB(closes, BollingerPeriod, BollingerNumStdDev); ok {
		t.BollingerPctB = &pctB
	}

	screenParams := signals.DefaultScreenParams(market.Of(ticker))
	var stratHits []llm.StrategyHitInfo
	if hit := signals.SqueezeBreakout(candles, screenParams); hit != nil {
		stratHits = append(stratHits, llm.StrategyHitInfo{Name: hit.Name, DaysAgo: hit.DaysAgo})
	}
	if hit := signals.BoxBottomRebound(candles, screenParams); hit != nil {
		stratHits = append(stratHits, llm.StrategyHitInfo{Name: hit.Name, DaysAgo: hit.DaysAgo})
	}
	if hit := signals.TrendBreakout(candles, screenParams); hit != nil {
		stratHits = append(stratHits, llm.StrategyHitInfo{Name: hit.Name, DaysAgo: hit.DaysAgo})
	}
	if hit := signals.TrendPullback(candles, screenParams); hit != nil {
		stratHits = append(stratHits, llm.StrategyHitInfo{Name: hit.Name, DaysAgo: hit.DaysAgo})
	}
	// TW only, deliberately unlike the alert path. Detector.CheckMTFCross
	// fires on US too (the user asked to observe it there), but US is
	// measured NEGATIVE past 1 SE in both splits — a Telegram notice can
	// carry that warning, an LLM prompt block cannot, and feeding a
	// measured-harmful screen into the recommendation prompt would degrade
	// the recommendation. See signals.CheckMTFCrossExact.
	if market.Of(ticker) == market.TW {
		if hit := signals.MTFCross(candles); hit != nil {
			stratHits = append(stratHits, llm.StrategyHitInfo{Name: hit.Name, DaysAgo: hit.DaysAgo})
		}
	}

	recent := candles
	if len(recent) > PromptCandleCount {
		recent = recent[len(recent)-PromptCandleCount:]
	}
	return t, recent, stratHits
}

// ComputeMarketRegime builds Phase 3.7 追加項's broad-market context block
// (see docs/phase-3.7-market-regime.md and llm.MarketContext): benchTicker's
// own trend (SPY for US, 0050 for TW — resolved by the caller, see
// bot.benchmarkFor, same policy-stays-in-the-adapter reasoning as
// RankAndTruncateCandidates' benchTicker param) and a volatility reading —
// US gets vixTicker's latest quote, TW gets VolProxyPct instead (ATR14/close
// off the same benchTicker candles already fetched, zero extra network
// call — no TW volatility-index dataset exists, free or paid). Either half
// failing just logs and leaves that half's fields at 0; both failing
// returns nil so the caller sees "no regime data" rather than an all-zero
// struct. Moved verbatim from bot.computeMarketRegime.
func ComputeMarketRegime(history RiskHistoryReader, quotes QuoteReader, m market.MarketID, benchTicker, vixTicker string) *llm.MarketContext {
	var mc llm.MarketContext

	mc.Bench = benchTicker
	candles, err := history.GetHistory(benchTicker, "1y")
	if err != nil {
		logger.Errorf("market regime: %s history: %v", benchTicker, err)
	} else if len(candles) > 0 {
		closes := data.Closes(candles)
		mc.SPYPrice = closes[len(closes)-1]
		mc.SPYMA50 = signals.MA(closes, 50)
		mc.SPYMA200 = signals.MA(closes, 200)

		if m == market.TW {
			if atr := signals.ATR(data.Highs(candles), data.Lows(candles), closes, 14); atr > 0 && mc.SPYPrice > 0 {
				mc.VolProxyPct = atr / mc.SPYPrice * 100
			}
		}
	}

	if m == market.US {
		if q, err := quotes.GetQuote(vixTicker); err != nil {
			logger.Errorf("market regime: %s quote: %v", vixTicker, err)
		} else {
			mc.VIX = q.Price
		}
	}

	if mc.SPYPrice == 0 && mc.VIX == 0 {
		return nil
	}
	return &mc
}
