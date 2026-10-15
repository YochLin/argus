package llm

import (
	"strings"
	"testing"

	"argus/internal/i18n"
)

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
	for _, absent := range []string{"Price range", "vs. market", "Holding thesis", "Recommendations during", "R multiple"} {
		if strings.Contains(prompt, absent) {
			t.Errorf("buildTradeReviewPrompt() should omit %q when no data is set, got:\n%s", absent, prompt)
		}
	}
}

func TestBuildTradeReviewPromptRMultiple(t *testing.T) {
	trade := ClosedTrade{
		Ticker: "AAPL",
		Legs: []TradeLeg{
			{Side: "BUY", Shares: 10, Price: 150, Date: "2026-06-01"},
			{Side: "SELL", Shares: 10, Price: 180, Date: "2026-06-20"},
		},
		RealizedPnL: 300, // (180-150)*10
		HoldingDays: 19,
		StopPrice:   140, // per-share risk = 150-140 = 10; R = 300/(10*10) = 3
	}

	prompt := buildTradeReviewPrompt(i18n.EN, trade)
	if !strings.Contains(prompt, "3.0") {
		t.Errorf("buildTradeReviewPrompt() missing R-multiple of 3.0, got:\n%s", prompt)
	}

	t.Run("stop price at or above entry is skipped, not divided by zero or negative", func(t *testing.T) {
		trade := trade
		trade.StopPrice = 150
		prompt := buildTradeReviewPrompt(i18n.EN, trade)
		if strings.Contains(prompt, "R multiple") {
			t.Errorf("buildTradeReviewPrompt() should omit the R-multiple line when stop >= entry price, got:\n%s", prompt)
		}
	})
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

func TestBuildTradeReviewPromptFollowup(t *testing.T) {
	base := ClosedTrade{
		Ticker: "AAPL",
		Legs: []TradeLeg{
			{Side: "BUY", Shares: 10, Price: 150, Date: "2026-06-01"},
			{Side: "SELL", Shares: 10, Price: 180, Date: "2026-06-20"},
		},
		RealizedPnL: 300,
		HoldingDays: 19,
	}

	t.Run("nil Followup renders byte-for-byte the same as before Phase 26", func(t *testing.T) {
		got := buildTradeReviewPrompt(i18n.EN, base)
		want := buildTradeReviewPrompt(i18n.EN, base)
		if got != want {
			t.Errorf("buildTradeReviewPrompt() with nil Followup changed output:\ngot:  %q\nwant: %q", got, want)
		}
		if strings.Contains(got, "Price action since the exit") {
			t.Errorf("buildTradeReviewPrompt() with nil Followup should omit the follow-up block, got:\n%s", got)
		}
	})

	t.Run("non-nil Followup adds the block and swaps the task section", func(t *testing.T) {
		trade := base
		trade.Followup = &TradeFollowup{
			TradingDaysAfterExit: 5,
			ExitPrice:            180,
			PriceAfter:           198,
			HighAfter:            205,
			LowAfter:             178,
			PctSinceExit:         10,
			Verdict:              "sold_early",
		}
		prompt := buildTradeReviewPrompt(i18n.EN, trade)
		for _, want := range []string{"Price action since the exit", "198.00", "205.00", "178.00", "+10.0%", "Was the exit right?"} {
			if !strings.Contains(prompt, want) {
				t.Errorf("buildTradeReviewPrompt() with Followup missing %q, got:\n%s", want, prompt)
			}
		}
		if strings.Contains(prompt, "Give an honest, unvarnished review in English") {
			t.Errorf("buildTradeReviewPrompt() with Followup should not use the at-close task section, got:\n%s", prompt)
		}
	})
}

func TestBuildTradeReviewPromptSnapshots(t *testing.T) {
	base := ClosedTrade{
		Ticker: "AAPL",
		Legs: []TradeLeg{
			{Side: "BUY", Shares: 10, Price: 150, Date: "2026-06-01"},
			{Side: "SELL", Shares: 10, Price: 180, Date: "2026-06-20"},
		},
		RealizedPnL: 300,
		HoldingDays: 19,
	}
	snaps := []TradeSnapshot{
		{
			Side: "BUY", Date: "2026-06-01", Price: 150,
			RSI: 71.2, RSIPrev5: 55.4, MACDHist: 0.5, MACDDays: 3, Trend: "bull",
			CloseVsMA20Pct: 3.1, MA20Slope5dPct: 0.6, VolRatio20: 1.8,
			News: []TradeSnapshotNews{{Source: "Reuters", Headline: "AAPL beats"}},
		},
		{Side: "SELL", Date: "2026-06-20", Price: 180, RSI: 45, RSIPrev5: 50, Trend: "range", VolRatio20: 0.7},
	}

	t.Run("no snapshots renders exactly what it did before P7b", func(t *testing.T) {
		got := buildTradeReviewPrompt(i18n.EN, base)
		if strings.Contains(got, "Snapshot on the fill day") {
			t.Errorf("a trade without snapshots must not mention them:\n%s", got)
		}
	})

	for _, lang := range []i18n.Lang{i18n.EN, i18n.ZH} {
		trade := base
		trade.Snapshots = snaps
		got := buildTradeReviewPrompt(lang, trade)
		for _, want := range []string{"2026-06-01", "71.2", "55.4", "AAPL beats", "Reuters", "2026-06-20"} {
			if !strings.Contains(got, want) {
				t.Errorf("lang %v: prompt missing %q:\n%s", lang, want, got)
			}
		}
		// One header per snapshot; the headline block only where there is news.
		if n := strings.Count(got, i18n.T(lang, i18n.KeyTradeSnapshotNewsHeader)); n != 1 {
			t.Errorf("lang %v: %d news headers, want 1 (only the entry has news)", lang, n)
		}
		// The snapshot comes before the task, so the model reads the data first.
		if strings.Index(got, "2026-06-20") > strings.Index(got, i18n.T(lang, i18n.KeyLessonMarker)) {
			t.Errorf("lang %v: snapshot after the task section", lang)
		}
	}

	t.Run("a snapshot never carries what happened afterwards", func(t *testing.T) {
		trade := base
		trade.Snapshots = snaps
		got := buildTradeReviewPrompt(i18n.EN, trade)
		if strings.Contains(got, "Price action since the exit") {
			t.Errorf("without a Followup there is no post-exit block:\n%s", got)
		}
	})

	t.Run("the trend label follows the reading", func(t *testing.T) {
		trade := base
		trade.Snapshots = snaps
		got := buildTradeReviewPrompt(i18n.EN, trade)
		if !strings.Contains(got, i18n.T(i18n.EN, i18n.KeyTradeSnapshotTrendBull)) || !strings.Contains(got, i18n.T(i18n.EN, i18n.KeyTradeSnapshotTrendRange)) {
			t.Errorf("trend labels missing:\n%s", got)
		}
		if strings.Contains(got, i18n.T(i18n.EN, i18n.KeyTradeSnapshotTrendBear)) {
			t.Errorf("no snapshot was bearish:\n%s", got)
		}
	})
}

// A fill made while its session was still running has a part-day volume: the
// prompt must say the snapshot is of that moment and leave the ratio out.
func TestWriteTradeSnapshotIntraday(t *testing.T) {
	snap := TradeSnapshot{Side: "SELL", Date: "2026-06-20", Price: 180, RSI: 45, RSIPrev5: 50, Trend: "range", VolRatio20: 0.24}
	for _, lang := range []i18n.Lang{i18n.EN, i18n.ZH} {
		var final, live strings.Builder
		writeTradeSnapshot(&final, lang, snap)
		snap.Intraday = true
		writeTradeSnapshot(&live, lang, snap)
		snap.Intraday = false

		if !strings.Contains(final.String(), "0.24") {
			t.Errorf("lang %v: a finished bar keeps its volume ratio:\n%s", lang, final.String())
		}
		if strings.Contains(live.String(), "0.24") {
			t.Errorf("lang %v: an intraday bar must not show a part-day volume ratio:\n%s", lang, live.String())
		}
		if !strings.Contains(live.String(), "45.0") || !strings.Contains(live.String(), "2026-06-20") {
			t.Errorf("lang %v: the price-based readings stay:\n%s", lang, live.String())
		}
		if strings.Contains(live.String(), i18n.T(lang, i18n.KeyTradeSnapshotHeader, "SELL", "2026-06-20", 180.0)) {
			t.Errorf("lang %v: the intraday block must not claim to be the close", lang)
		}
	}
}

// The lesson instruction tells the model not to turn one trade's indicator
// readings into numeric thresholds (a review's lesson is fed back into every
// recommendation prompt), in both reviews that carry snapshots, and the '%'
// in its example must survive formatting.
func TestReviewTaskForbidsThresholdLessons(t *testing.T) {
	trade := ClosedTrade{Ticker: "AAPL", Legs: []TradeLeg{{Side: "BUY", Shares: 1, Price: 1, Date: "2026-06-01"}, {Side: "SELL", Shares: 1, Price: 2, Date: "2026-06-20"}}}
	for _, lang := range []i18n.Lang{i18n.EN, i18n.ZH} {
		atClose := buildTradeReviewPrompt(lang, trade)
		followup := trade
		followup.Followup = &TradeFollowup{TradingDaysAfterExit: 5}
		later := buildTradeReviewPrompt(lang, followup)
		for name, got := range map[string]string{"at close": atClose, "follow-up": later} {
			if strings.Contains(got, "%%") || strings.Contains(got, "%!") {
				t.Errorf("lang %v %s: unformatted percent in the prompt:\n%s", lang, name, got)
			}
		}
		want := map[i18n.Lang]string{i18n.EN: "N% above the MA20", i18n.ZH: "乖離幾 %"}[lang]
		if !strings.Contains(atClose, want) {
			t.Errorf("lang %v: at-close task lost the threshold warning (want %q)", lang, want)
		}
	}
}
