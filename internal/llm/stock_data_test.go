package llm

import (
	"strings"
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/i18n"
)

func TestWriteStockSectionOmitsThesisAndVsSPYWhenNil(t *testing.T) {
	var sb strings.Builder
	writeStockSection(&sb, i18n.EN, StockData{Quote: &data.Quote{Ticker: "AAPL", Price: 200}})

	got := sb.String()
	if strings.Contains(got, "Holding thesis") || strings.Contains(got, "vs. market") {
		t.Errorf("writeStockSection() should omit thesis/vs-SPY lines when both are nil, got:\n%s", got)
	}
}

func TestWriteStockSectionEarningsEstimatedWording(t *testing.T) {
	t.Run("real earnings date uses the confirmed wording", func(t *testing.T) {
		var sb strings.Builder
		writeStockSection(&sb, i18n.EN, StockData{
			Quote:    &data.Quote{Ticker: "AAPL", Price: 200},
			Earnings: &Earnings{Date: "2026-08-01", DaysUntil: 3},
		})
		got := sb.String()
		if !strings.Contains(got, "Earnings date:") {
			t.Errorf("writeStockSection() missing confirmed-earnings wording, got:\n%s", got)
		}
		if strings.Contains(got, "statutory filing deadline") {
			t.Errorf("writeStockSection() should not use the estimated wording for a real earnings date, got:\n%s", got)
		}
	})

	t.Run("TW statutory-deadline proxy uses the estimated wording", func(t *testing.T) {
		var sb strings.Builder
		writeStockSection(&sb, i18n.EN, StockData{
			Quote:    &data.Quote{Ticker: "2330", Price: 900},
			Earnings: &Earnings{Date: "2026-08-14", DaysUntil: 3, Estimated: true},
		})
		got := sb.String()
		if !strings.Contains(got, "statutory filing deadline") {
			t.Errorf("writeStockSection() missing estimated-deadline wording, got:\n%s", got)
		}
		if strings.Contains(got, "Earnings date:") {
			t.Errorf("writeStockSection() should not use the confirmed-earnings wording for an estimated deadline, got:\n%s", got)
		}
	})
}

func TestWriteStockSectionRendersCandles(t *testing.T) {
	var sb strings.Builder
	writeStockSection(&sb, i18n.EN, StockData{
		Quote: &data.Quote{Ticker: "AAPL", Price: 200},
		Candles: []data.Candle{
			{Date: time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC), Open: 198.5, High: 201, Low: 197.25, Close: 200, Volume: 54321},
		},
	})

	got := sb.String()
	for _, want := range []string{"2026-07-17", "198.50", "201.00", "197.25", "200.00", "54321"} {
		if !strings.Contains(got, want) {
			t.Errorf("writeStockSection() candle block missing %q, got:\n%s", want, got)
		}
	}
}

func TestWriteStockSectionOmitsBollingerWhenNil(t *testing.T) {
	var sb strings.Builder
	writeStockSection(&sb, i18n.EN, StockData{
		Quote:      &data.Quote{Ticker: "AAPL", Price: 200},
		Technicals: &Technicals{RSI14: 55}, // BollingerPctB left nil
	})

	if got := sb.String(); strings.Contains(got, "Bollinger") {
		t.Errorf("writeStockSection() should omit the Bollinger line when BollingerPctB is nil, got:\n%s", got)
	}
}

func TestWriteStockSectionRendersBollingerWhenPresent(t *testing.T) {
	pctB := 0.0 // a legitimate reading (price at the lower band), must still render
	var sb strings.Builder
	writeStockSection(&sb, i18n.EN, StockData{
		Quote:      &data.Quote{Ticker: "AAPL", Price: 200},
		Technicals: &Technicals{RSI14: 55, BollingerPctB: &pctB},
	})

	got := sb.String()
	if !strings.Contains(got, "Bollinger") {
		t.Errorf("writeStockSection() should render the Bollinger line when BollingerPctB is set (even to 0), got:\n%s", got)
	}
	if !strings.Contains(got, "0%") {
		t.Errorf("writeStockSection() Bollinger line should show 0%%, got:\n%s", got)
	}
}

// TestWriteStockSectionOmitsMonthRevenueYoYWhenZero covers Phase 6 PR3: this
// field is TW-only (FinMind) and 0 for every US ticker's Fundamentals (from
// Finnhub, which has no monthly-revenue concept), so it must not render a
// misleading "0.0%" line for a US recommendation.
func TestWriteStockSectionOmitsMonthRevenueYoYWhenZero(t *testing.T) {
	var sb strings.Builder
	writeStockSection(&sb, i18n.EN, StockData{
		Quote:        &data.Quote{Ticker: "AAPL", Price: 200},
		Fundamentals: &data.Fundamentals{PE: 28.5}, // MonthRevenueYoYPct left 0
	})

	if got := sb.String(); strings.Contains(got, "Month Revenue YoY") {
		t.Errorf("writeStockSection() should omit the Month Revenue YoY line when it's 0, got:\n%s", got)
	}
}

func TestWriteStockSectionRendersMonthRevenueYoYWhenPresent(t *testing.T) {
	var sb strings.Builder
	writeStockSection(&sb, i18n.EN, StockData{
		Quote:        &data.Quote{Ticker: "2330", Price: 1200},
		Fundamentals: &data.Fundamentals{PE: 32.3, MonthRevenueYoYPct: 67.9},
	})

	got := sb.String()
	if !strings.Contains(got, "Month Revenue YoY") || !strings.Contains(got, "67.9") {
		t.Errorf("writeStockSection() should render the Month Revenue YoY line when non-zero, got:\n%s", got)
	}
}

func TestWriteStockSectionOmitsEarningsSurprisesWhenEmpty(t *testing.T) {
	var sb strings.Builder
	writeStockSection(&sb, i18n.EN, StockData{
		Quote: &data.Quote{Ticker: "AAPL", Price: 200},
	})
	if got := sb.String(); strings.Contains(got, "beat") {
		t.Errorf("writeStockSection() should omit the earnings-surprise line when empty, got:\n%s", got)
	}
}

func TestWriteStockSectionRendersEarningsSurpriseBeatMissStreak(t *testing.T) {
	var sb strings.Builder
	writeStockSection(&sb, i18n.EN, StockData{
		Quote: &data.Quote{Ticker: "AAPL", Price: 200},
		EarningsSurprises: []data.EarningsSurprise{
			{Period: "2025-09-30", Actual: 1.85, Estimate: 1.8075, SurprisePct: 2.3513},
			{Period: "2025-12-31", Actual: 2.84, Estimate: 2.7257, SurprisePct: 4.1934},
			{Period: "2026-03-31", Actual: 2.01, Estimate: 1.9884, SurprisePct: 1.0863},
			{Period: "2026-06-30", Actual: 1.91, Estimate: 1.9271, SurprisePct: -0.8873}, // the one miss
		},
	})
	got := sb.String()
	if !strings.Contains(got, "3 beat / 1 miss") {
		t.Errorf("writeStockSection() = %q, want a \"3 beat / 1 miss\" line", got)
	}
	if !strings.Contains(got, "2026-06-30") {
		t.Errorf("writeStockSection() = %q, want the most recent quarter's period", got)
	}
}

func TestWriteRecentLessonsEmpty(t *testing.T) {
	var sb strings.Builder
	writeRecentLessons(&sb, i18n.EN, nil)
	if sb.String() != "" {
		t.Errorf("writeRecentLessons(nil) = %q, want empty", sb.String())
	}
}

func TestWriteRecentLessonsRendersEachLesson(t *testing.T) {
	var sb strings.Builder
	writeRecentLessons(&sb, i18n.EN, []PastLesson{
		{Ticker: "AAPL", Date: "2026-06-01", Lesson: "exited too early"},
		{Ticker: "MSFT", Date: "2026-06-15", Lesson: "held through an earnings miss"},
	})
	got := sb.String()
	for _, want := range []string{"AAPL", "exited too early", "MSFT", "held through an earnings miss"} {
		if !strings.Contains(got, want) {
			t.Errorf("writeRecentLessons() missing %q, got:\n%s", want, got)
		}
	}
}

func TestWriteStockSectionOmitsPastLessonsWhenEmpty(t *testing.T) {
	var sb strings.Builder
	writeStockSection(&sb, i18n.EN, StockData{Quote: &data.Quote{Ticker: "AAPL", Price: 200}})
	if strings.Contains(sb.String(), "Past lessons") {
		t.Errorf("writeStockSection() should omit past-lessons block when empty, got:\n%s", sb.String())
	}
}

func TestWriteStockSectionRendersPastLessons(t *testing.T) {
	var sb strings.Builder
	writeStockSection(&sb, i18n.EN, StockData{
		Quote: &data.Quote{Ticker: "AAPL", Price: 200},
		PastLessons: []PastLesson{
			{Ticker: "AAPL", Date: "2026-05-01", Lesson: "sold too early last time"},
		},
	})
	got := sb.String()
	if !strings.Contains(got, "Past lessons") || !strings.Contains(got, "sold too early last time") {
		t.Errorf("writeStockSection() missing past-lessons content, got:\n%s", got)
	}
}

func TestWriteStockSectionRendersPhase310TechnicalsAndStrategyHits(t *testing.T) {
	stochK, stochD := 75.5, 68.2
	bandwidth := 0.085
	macdAboveZero := 1.25
	rs63 := 5.4

	var sb strings.Builder
	writeStockSection(&sb, i18n.EN, StockData{
		Quote: &data.Quote{Ticker: "NVDA", Price: 120},
		Technicals: &Technicals{
			RSI14:         62,
			MACDTrend:     "bullish",
			MA5:           118,
			MA20:          115,
			MA50:          110,
			MA60:          108,
			MA200:         95,
			StochK:        &stochK,
			StochD:        &stochD,
			Bandwidth:     &bandwidth,
			MAAlign:       "bullish",
			VolumePrice:   "vol_up_price_up",
			NewHigh20:     true,
			NewHigh52w:    true,
			MACDAboveZero: &macdAboveZero,
			RS63:          &rs63,
		},
		StrategyHits: []StrategyHitInfo{
			{Name: "squeeze_breakout", DaysAgo: 0},
			{Name: "box_bottom", DaysAgo: 2},
		},
	})

	got := sb.String()
	for _, want := range []string{
		"KD(9,3,3): K=75.5, D=68.2",
		"Above MA5 ($118.00)", "Above MA60 ($108.00)",
		"MA Alignment (5/20/60): Bullish",
		"Volume-Price: Vol Up Price Up (Bullish)",
		"New High Record: 20-Day & 52-Week New High",
		"MACD Zero-Line: Above Zero Line (Bullish Zone) (MACD=1.25)",
		"Relative Strength RS63: Outperforming Market (vs SPY +5.4%)",
		"Bollinger Bandwidth: 8.5%",
		"⚠️ Strategy Hit: Squeeze Breakout (Today)",
		"⚠️ Strategy Hit: Box Bottom Rebound (2 days ago)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("writeStockSection() missing %q, got:\n%s", want, got)
		}
	}
}
