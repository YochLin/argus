package llm

import (
	"strings"
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/i18n"
)

func TestBuildInsightPromptTotalValueAndCashLine(t *testing.T) {
	positions := []StockData{
		{Quote: &data.Quote{Ticker: "AAPL", Price: 200}, Position: &Position{Shares: 10, AvgCost: 150}},
		{Quote: &data.Quote{Ticker: "MSFT", Price: 300}, Position: &Position{Shares: 5, AvgCost: 250}},
	}

	prompt := buildInsightPrompt(i18n.EN, positions, 500, true)

	// Total position value: 10*200 + 5*300 = 3500; grand total with cash: 4000.
	if !strings.Contains(prompt, "3500.00") {
		t.Errorf("buildInsightPrompt() missing total position value 3500.00, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "500.00") {
		t.Errorf("buildInsightPrompt() missing cash balance 500.00, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "4000.00") {
		t.Errorf("buildInsightPrompt() missing grand total 4000.00, got:\n%s", prompt)
	}
	for _, want := range []string{"AAPL", "MSFT"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("buildInsightPrompt() missing ticker %q, got:\n%s", want, prompt)
		}
	}
}

func TestBuildInsightPromptOmitsCashLineWhenUnset(t *testing.T) {
	positions := []StockData{
		{Quote: &data.Quote{Ticker: "AAPL", Price: 200}, Position: &Position{Shares: 10, AvgCost: 150}},
	}

	prompt := buildInsightPrompt(i18n.EN, positions, 0, false)

	if strings.Contains(prompt, "Cash balance") {
		t.Errorf("buildInsightPrompt() with haveCash=false should omit the cash line entirely, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "2000.00") {
		t.Errorf("buildInsightPrompt() missing total position value 2000.00, got:\n%s", prompt)
	}
}

func TestBuildInsightPromptSkipsValueForPositionWithoutQuote(t *testing.T) {
	// A position with a nil Quote shouldn't happen in practice (writeStockSection
	// itself early-returns without one), but the total-value accumulation
	// guards against it explicitly, so make sure it doesn't panic or miscount.
	positions := []StockData{
		{Quote: &data.Quote{Ticker: "AAPL", Price: 200}, Position: &Position{Shares: 10, AvgCost: 150}},
		{Quote: nil, Position: &Position{Shares: 5, AvgCost: 100}},
	}

	prompt := buildInsightPrompt(i18n.EN, positions, 0, false)

	if !strings.Contains(prompt, "2000.00") {
		t.Errorf("buildInsightPrompt() missing total position value 2000.00 (should ignore the quote-less position), got:\n%s", prompt)
	}
}

func TestBuildInsightPromptRendersThesisAndVsSPY(t *testing.T) {
	thesis := "long-term compounder, services growth"
	positions := []StockData{
		{
			Quote:    &data.Quote{Ticker: "AAPL", Price: 200},
			Position: &Position{Shares: 10, AvgCost: 150},
			Thesis:   &thesis,
			VsSPY:    &VsSPYReturn{TickerPct: 33.3, SPYPct: 10.0},
		},
	}

	prompt := buildInsightPrompt(i18n.EN, positions, 0, false)

	if !strings.Contains(prompt, thesis) {
		t.Errorf("buildInsightPrompt() missing thesis text, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "+33.3%") || !strings.Contains(prompt, "+10.0%") {
		t.Errorf("buildInsightPrompt() missing vs-SPY percentages, got:\n%s", prompt)
	}
}

func TestBuildWeeklyReviewPromptIncludesTrackSummaryWhenPresent(t *testing.T) {
	positions := []StockData{
		{Quote: &data.Quote{Ticker: "AAPL", Price: 200}, Position: &Position{Shares: 10, AvgCost: 150}},
	}

	prompt := buildWeeklyReviewPrompt(i18n.EN, positions, 0, false, "Hit rate: 3/5 (60%)")

	if !strings.Contains(prompt, "Hit rate: 3/5 (60%)") {
		t.Errorf("buildWeeklyReviewPrompt() missing track summary text, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "This week's recommendation tracking") {
		t.Errorf("buildWeeklyReviewPrompt() missing track summary header, got:\n%s", prompt)
	}
}

func TestBuildWeeklyReviewPromptOmitsTrackSectionWhenEmpty(t *testing.T) {
	positions := []StockData{
		{Quote: &data.Quote{Ticker: "AAPL", Price: 200}, Position: &Position{Shares: 10, AvgCost: 150}},
	}

	prompt := buildWeeklyReviewPrompt(i18n.EN, positions, 0, false, "")

	if strings.Contains(prompt, "This week's recommendation tracking") {
		t.Errorf("buildWeeklyReviewPrompt() with empty trackSummary should omit the track section entirely, got:\n%s", prompt)
	}
}

func TestBuildWeeklyReviewPromptTotalValueAndCashLine(t *testing.T) {
	positions := []StockData{
		{Quote: &data.Quote{Ticker: "AAPL", Price: 200}, Position: &Position{Shares: 10, AvgCost: 150}},
		{Quote: &data.Quote{Ticker: "MSFT", Price: 300}, Position: &Position{Shares: 5, AvgCost: 250}},
	}

	prompt := buildWeeklyReviewPrompt(i18n.EN, positions, 500, true, "")

	// Total position value: 10*200 + 5*300 = 3500; grand total with cash: 4000.
	if !strings.Contains(prompt, "3500.00") || !strings.Contains(prompt, "500.00") || !strings.Contains(prompt, "4000.00") {
		t.Errorf("buildWeeklyReviewPrompt() missing total/cash/grand-total values, got:\n%s", prompt)
	}
}

func TestWriteStockSectionOmitsThesisAndVsSPYWhenNil(t *testing.T) {
	var sb strings.Builder
	writeStockSection(&sb, i18n.EN, StockData{Quote: &data.Quote{Ticker: "AAPL", Price: 200}})

	got := sb.String()
	if strings.Contains(got, "Holding thesis") || strings.Contains(got, "vs. market") {
		t.Errorf("writeStockSection() should omit thesis/vs-SPY lines when both are nil, got:\n%s", got)
	}
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

func TestBuildTradeReviewPromptMinimal(t *testing.T) {
	trade := ClosedTrade{
		Ticker: "AAPL",
		Legs: []TradeLeg{
			{Side: "BUY", Shares: 10, Price: 150, Date: "2026-06-01"},
			{Side: "SELL", Shares: 10, Price: 180, Date: "2026-06-20"},
		},
		RealizedPnL: 300,
		HoldingDays: 19,
	}

	prompt := buildTradeReviewPrompt(i18n.EN, trade)

	if !strings.Contains(prompt, "AAPL") {
		t.Errorf("buildTradeReviewPrompt() missing ticker, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "BUY 10") || !strings.Contains(prompt, "SELL 10") {
		t.Errorf("buildTradeReviewPrompt() missing leg lines, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "+300.00") || !strings.Contains(prompt, "19 days") {
		t.Errorf("buildTradeReviewPrompt() missing P&L/holding-days line, got:\n%s", prompt)
	}
	for _, absent := range []string{"Price range", "vs. market", "Holding thesis", "Recommendations during"} {
		if strings.Contains(prompt, absent) {
			t.Errorf("buildTradeReviewPrompt() should omit %q when no data is set, got:\n%s", absent, prompt)
		}
	}
}

func TestBuildTradeReviewPromptFull(t *testing.T) {
	thesis := "long-term compounder"
	trade := ClosedTrade{
		Ticker: "AAPL",
		Legs: []TradeLeg{
			{Side: "BUY", Shares: 10, Price: 150, Date: "2026-06-01"},
			{Side: "SELL", Shares: 10, Price: 180, Date: "2026-06-20"},
		},
		RealizedPnL: 300,
		HoldingDays: 19,
		VsSPY:       &VsSPYReturn{TickerPct: 20, SPYPct: 5},
		PeriodHigh:  195,
		PeriodLow:   145,
		Thesis:      &thesis,
		Recommendations: []TradeRecommendation{
			{Date: "2026-06-15", Action: "HOLD", Reason: "still above support"},
		},
	}

	prompt := buildTradeReviewPrompt(i18n.EN, trade)

	for _, want := range []string{"195.00", "145.00", "+20.0%", "+5.0%", thesis, "HOLD", "still above support"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("buildTradeReviewPrompt() missing %q, got:\n%s", want, prompt)
		}
	}
}

func TestRegimeLabel(t *testing.T) {
	tests := []struct {
		name         string
		price, ma200 float64
		want         i18n.Key
	}{
		{"above MA200 is risk-on", 500, 480, i18n.KeyRiskOn},
		{"below MA200 is risk-off", 460, 480, i18n.KeyRiskOff},
		{"exactly at MA200 is risk-off", 480, 480, i18n.KeyRiskOff},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := regimeLabel(tt.price, tt.ma200); got != tt.want {
				t.Errorf("regimeLabel(%v, %v) = %v, want %v", tt.price, tt.ma200, got, tt.want)
			}
		})
	}
}

func TestVIXLabel(t *testing.T) {
	tests := []struct {
		name string
		vix  float64
		want i18n.Key
	}{
		{"calm below threshold", 12, i18n.KeyVIXCalm},
		{"just under calm threshold", 14.9, i18n.KeyVIXCalm},
		{"normal at calm boundary", 15, i18n.KeyVIXNormal},
		{"normal mid-range", 20, i18n.KeyVIXNormal},
		{"normal at panic boundary", 25, i18n.KeyVIXNormal},
		{"panic above threshold", 30, i18n.KeyVIXPanic},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := vixLabel(tt.vix); got != tt.want {
				t.Errorf("vixLabel(%v) = %v, want %v", tt.vix, got, tt.want)
			}
		})
	}
}

func TestWriteMarketContext(t *testing.T) {
	t.Run("nil market renders nothing", func(t *testing.T) {
		var sb strings.Builder
		writeMarketContext(&sb, i18n.EN, nil)
		if sb.String() != "" {
			t.Errorf("writeMarketContext(nil) = %q, want empty", sb.String())
		}
	})

	t.Run("both zero renders nothing", func(t *testing.T) {
		var sb strings.Builder
		writeMarketContext(&sb, i18n.EN, &MarketContext{})
		if sb.String() != "" {
			t.Errorf("writeMarketContext(all-zero) = %q, want empty", sb.String())
		}
	})

	t.Run("SPY only omits VIX line", func(t *testing.T) {
		var sb strings.Builder
		writeMarketContext(&sb, i18n.EN, &MarketContext{SPYPrice: 500, SPYMA50: 490, SPYMA200: 480})
		got := sb.String()
		if !strings.Contains(got, "SPY price") {
			t.Errorf("writeMarketContext() missing SPY line, got:\n%s", got)
		}
		if strings.Contains(got, "VIX") {
			t.Errorf("writeMarketContext() should omit VIX line when VIX is 0, got:\n%s", got)
		}
	})

	t.Run("VIX only omits SPY line", func(t *testing.T) {
		var sb strings.Builder
		writeMarketContext(&sb, i18n.EN, &MarketContext{VIX: 18})
		got := sb.String()
		if strings.Contains(got, "SPY price") {
			t.Errorf("writeMarketContext() should omit SPY line when SPY data is 0, got:\n%s", got)
		}
		if !strings.Contains(got, "VIX") {
			t.Errorf("writeMarketContext() missing VIX line, got:\n%s", got)
		}
	})

	t.Run("full context renders both lines with labels", func(t *testing.T) {
		var sb strings.Builder
		writeMarketContext(&sb, i18n.EN, &MarketContext{SPYPrice: 500, SPYMA50: 490, SPYMA200: 480, VIX: 30})
		got := sb.String()
		if !strings.Contains(got, "risk-on") {
			t.Errorf("writeMarketContext() missing risk-on label, got:\n%s", got)
		}
		if !strings.Contains(got, "high panic") {
			t.Errorf("writeMarketContext() missing high-panic label, got:\n%s", got)
		}
	})
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

func TestBuildRecommendationPromptRendersRecentLessons(t *testing.T) {
	prompt := buildRecommendationPrompt(i18n.EN, nil, nil, nil, nil, []PastLesson{
		{Ticker: "NVDA", Date: "2026-06-20", Lesson: "chased a breakout too late"},
	})
	if !strings.Contains(prompt, "NVDA") || !strings.Contains(prompt, "chased a breakout too late") {
		t.Errorf("buildRecommendationPrompt() missing recent-lessons content, got:\n%s", prompt)
	}
}

func TestBuildRecommendationPromptOmitsRecentLessonsWhenEmpty(t *testing.T) {
	prompt := buildRecommendationPrompt(i18n.EN, nil, nil, nil, nil, nil)
	if strings.Contains(prompt, "Recent Trade Lessons") {
		t.Errorf("buildRecommendationPrompt() should omit recent-lessons block when empty, got:\n%s", prompt)
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
