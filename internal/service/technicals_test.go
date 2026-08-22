package service

import (
	"errors"
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/market"
)

// flatCandlesForTechnicals builds n days of flat OHLCV at the given
// close/volume, oldest first — local to this file rather than shared with
// candidate_rank_test.go's identically-shaped helper, since that file lives
// on a separate not-yet-merged PR branch and this one must build standalone
// against main.
func flatCandlesForTechnicals(n int, close float64, volume int64) []data.Candle {
	out := make([]data.Candle, n)
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		out[i] = data.Candle{
			Date: start.AddDate(0, 0, i),
			Open: close, High: close, Low: close, Close: close,
			Volume: volume,
		}
	}
	return out
}

// historyByTicker is a RiskHistoryReader stub keyed by ticker.
type historyByTicker map[string][]data.Candle

func (h historyByTicker) GetHistory(ticker, rangeParam string) ([]data.Candle, error) {
	return h[ticker], nil
}

func TestComputeTechnicalsHistoryFetchFailure(t *testing.T) {
	tech, candles, hits := ComputeTechnicals(errHistoryStub{}, "AAPL", nil)
	if tech != nil || candles != nil || hits != nil {
		t.Errorf("ComputeTechnicals() on fetch error = %v, %v, %v, want all nil", tech, candles, hits)
	}
}

func TestComputeTechnicalsHappyPath(t *testing.T) {
	candles := flatCandlesForTechnicals(250, 100, 1_000_000)
	for i := 200; i < len(candles); i++ {
		candles[i].Close = 100 + float64(i-199)
		candles[i].High, candles[i].Low, candles[i].Open = candles[i].Close, candles[i].Close, candles[i].Close
	}
	history := historyByTicker{"AAPL": candles}

	tech, recent, _ := ComputeTechnicals(history, "AAPL", nil)
	if tech == nil {
		t.Fatal("ComputeTechnicals() technicals = nil, want a populated struct")
	}
	if tech.MA5 == 0 || tech.MA20 == 0 {
		t.Errorf("ComputeTechnicals() MA5/MA20 = %v/%v, want non-zero with 250 candles", tech.MA5, tech.MA20)
	}
	if len(recent) != PromptCandleCount {
		t.Errorf("len(recent candles) = %d, want %d (PromptCandleCount cap)", len(recent), PromptCandleCount)
	}
}

// errHistoryStub always fails GetHistory, for ComputeTechnicals'/
// ComputeMarketRegime's fetch-error branches.
type errHistoryStub struct{}

func (errHistoryStub) GetHistory(ticker, rangeParam string) ([]data.Candle, error) {
	return nil, errors.New("history unavailable")
}

// errQuoteStub always fails GetQuote, for ComputeMarketRegime's VIX branch.
type errQuoteStub struct{}

func (errQuoteStub) GetQuote(ticker string) (*data.Quote, error) {
	return nil, errors.New("quote unavailable")
}

type fixedQuoteStub struct{ price float64 }

func (f fixedQuoteStub) GetQuote(ticker string) (*data.Quote, error) {
	return &data.Quote{Ticker: ticker, Price: f.price}, nil
}

func TestComputeMarketRegimeNilWhenBothHalvesFail(t *testing.T) {
	got := ComputeMarketRegime(errHistoryStub{}, errQuoteStub{}, market.US, "SPY", "^VIX")
	if got != nil {
		t.Errorf("ComputeMarketRegime() = %+v, want nil when both history and VIX quote fail", got)
	}
}

func TestComputeMarketRegimeUS(t *testing.T) {
	history := historyByTicker{"SPY": flatCandlesForTechnicals(250, 100, 1_000_000)}
	got := ComputeMarketRegime(history, fixedQuoteStub{price: 18.5}, market.US, "SPY", "^VIX")
	if got == nil {
		t.Fatal("ComputeMarketRegime() = nil, want a populated context")
	}
	if got.Bench != "SPY" || got.SPYPrice != 100 || got.VIX != 18.5 {
		t.Errorf("ComputeMarketRegime() = %+v, want Bench=SPY SPYPrice=100 VIX=18.5", got)
	}
	if got.VolProxyPct != 0 {
		t.Errorf("ComputeMarketRegime() US VolProxyPct = %v, want 0 (VIX-only, US doesn't use the ATR proxy)", got.VolProxyPct)
	}
}

func TestComputeMarketRegimeTWUsesVolProxy(t *testing.T) {
	candles := flatCandlesForTechnicals(60, 100, 1_000_000)
	for i := range candles {
		candles[i].High = 102
		candles[i].Low = 98
	}
	history := historyByTicker{"0050": candles}
	got := ComputeMarketRegime(history, errQuoteStub{}, market.TW, "0050", "^VIX")
	if got == nil {
		t.Fatal("ComputeMarketRegime() = nil, want a populated context (SPYPrice alone is enough)")
	}
	if got.VIX != 0 {
		t.Errorf("ComputeMarketRegime() TW VIX = %v, want 0 (TW never queries VIX)", got.VIX)
	}
	if got.VolProxyPct <= 0 {
		t.Errorf("ComputeMarketRegime() TW VolProxyPct = %v, want > 0 (ATR14/close proxy)", got.VolProxyPct)
	}
}
