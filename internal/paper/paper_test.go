package paper

import (
	"math"
	"testing"

	"argus/internal/market"
)

func almostEqual(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestSuggestShares(t *testing.T) {
	// capped by cash isn't SuggestShares' job (that's the caller's min()),
	// so this only covers the risk-budget formula itself plus its guards.
	if got := SuggestShares(100000, 1, 100, 90); got != 100 {
		t.Errorf("risk budget: got %d, want 100", got)
	}
	if got := SuggestShares(100000, 0, 100, 90); got != 0 {
		t.Errorf("riskPct<=0: got %d, want 0", got)
	}
	if got := SuggestShares(100000, 1, 100, 100); got != 0 {
		t.Errorf("stop>=price: got %d, want 0", got)
	}
	if got := SuggestShares(100, 0.001, 100, 99); got != 0 {
		t.Errorf("sub-1-share risk budget: got %d, want 0", got)
	}
}

func baseConfig() Config {
	return Config{
		InitialCash:    100000,
		RiskPct:        1,
		MaxPositionPct: 25,
		StopATRMult:    2,
		StopLossPct:    10,
		Market:         market.US,
		FeeDiscount:    1.0,
	}
}

func TestApplySignal_BuySizing(t *testing.T) {
	cfg := baseConfig()
	acct := NewAccount(cfg.InitialCash)

	// notional cap: MaxPositionPct=25% of 100k=25000 at price 10 -> cap 2500
	// shares, well below the risk-budget shares (1%*100000/(10-8)=500), so
	// this exercises the risk-budget path, not the cap — flip to a tight ATR
	// to force the cap branch instead.
	trade, ok := acct.ApplySignal(Signal{Date: "2024-01-02", Ticker: "AAA", Action: "BUY", Price: 100}, 100, 1, cfg)
	if !ok {
		t.Fatalf("expected buy to execute")
	}
	if trade.Stop <= 0 {
		t.Errorf("expected a stop price, got %v", trade.Stop)
	}
	wantCash := cfg.InitialCash - trade.Shares*trade.Price - trade.Fee
	if !almostEqual(acct.Cash, wantCash) {
		t.Errorf("cash after buy: got %v, want %v", acct.Cash, wantCash)
	}
	if h, held := acct.Holdings["AAA"]; !held || h.Stop != trade.Stop {
		t.Errorf("holding not recorded correctly: %+v", h)
	}
}

func TestApplySignal_CashCap(t *testing.T) {
	cfg := baseConfig()
	cfg.MaxPositionPct = 0  // isolate the cash-cap branch
	acct := NewAccount(500) // barely enough for a couple shares at price 100

	trade, ok := acct.ApplySignal(Signal{Date: "2024-01-02", Ticker: "AAA", Action: "BUY", Price: 100}, 100, 1, cfg)
	if !ok {
		t.Fatalf("expected buy to execute")
	}
	if trade.Shares > 5 {
		t.Errorf("expected cash cap to bound shares near 5, got %v", trade.Shares)
	}
	if acct.Cash < 0 {
		t.Errorf("cash went negative: %v", acct.Cash)
	}
}

// TestApplySignal_TWCashCapBudgetsMinFee guards against the cash-cap sizing
// estimate ignoring FeeFor's twMinFee floor: a low-priced TW fill where the
// percentage-rate fee estimate rounds under twMinFee must still leave
// account cash non-negative once the actual (floored) fee is charged.
func TestApplySignal_TWCashCapBudgetsMinFee(t *testing.T) {
	cfg := baseConfig()
	cfg.Market = market.TW
	cfg.FeeDiscount = 0.6
	cfg.MaxPositionPct = 0 // isolate the cash-cap branch
	acct := NewAccount(999)

	trade, ok := acct.ApplySignal(Signal{Date: "2024-01-02", Ticker: "2330", Action: "BUY", Price: 20}, 20, 1, cfg)
	if !ok {
		t.Fatalf("expected buy to execute")
	}
	if trade.Fee < twMinFee {
		t.Fatalf("expected the min-fee floor to apply, got fee %v", trade.Fee)
	}
	if acct.Cash < 0 {
		t.Errorf("cash went negative: %v", acct.Cash)
	}
}

func TestApplySignal_AtrFallback(t *testing.T) {
	cfg := baseConfig()
	acct := NewAccount(cfg.InitialCash)

	trade, ok := acct.ApplySignal(Signal{Date: "2024-01-02", Ticker: "AAA", Action: "BUY", Price: 100}, 100, 0, cfg)
	if !ok {
		t.Fatalf("expected buy to execute via fixed-%% stop fallback")
	}
	wantStop := 100 * (1 - cfg.StopLossPct/100)
	if !almostEqual(trade.Stop, wantStop) {
		t.Errorf("fallback stop: got %v, want %v", trade.Stop, wantStop)
	}
}

func TestApplySignal_BuyAlreadyHeldSkips(t *testing.T) {
	cfg := baseConfig()
	acct := NewAccount(cfg.InitialCash)

	if _, ok := acct.ApplySignal(Signal{Date: "2024-01-02", Ticker: "AAA", Action: "BUY", Price: 100}, 100, 1, cfg); !ok {
		t.Fatalf("first buy should execute")
	}
	cashAfterFirst := acct.Cash

	if _, ok := acct.ApplySignal(Signal{Date: "2024-01-03", Ticker: "AAA", Action: "BUY", Price: 105}, 105, 1, cfg); ok {
		t.Errorf("second buy on an already-held ticker should be a no-op")
	}
	if acct.Cash != cashAfterFirst {
		t.Errorf("cash changed on a skipped buy: %v -> %v", cashAfterFirst, acct.Cash)
	}
}

func TestMarkClose_StopLoss(t *testing.T) {
	cfg := baseConfig()
	acct := NewAccount(cfg.InitialCash)
	acct.ApplySignal(Signal{Date: "2024-01-02", Ticker: "AAA", Action: "BUY", Price: 100}, 100, 1, cfg)
	h := acct.Holdings["AAA"]

	trades := acct.MarkClose("2024-01-03", map[string]float64{"AAA": h.Stop - 1}, nil, cfg)
	if len(trades) != 1 || trades[0].Reason != "stop" {
		t.Fatalf("expected one stop-loss exit, got %+v", trades)
	}
	wantPnL := (h.Stop - 1 - h.AvgCost) * h.Shares
	if !almostEqual(trades[0].RealizedPnL, wantPnL) {
		t.Errorf("realized pnl: got %v, want %v", trades[0].RealizedPnL, wantPnL)
	}
	if _, held := acct.Holdings["AAA"]; held {
		t.Errorf("position should be closed after stop-loss exit")
	}
}

func TestMarkClose_TrailingStop(t *testing.T) {
	cfg := baseConfig()
	cfg.TrailingPct = 5 // tight enough to trigger well above the fixed stop
	acct := NewAccount(cfg.InitialCash)
	acct.ApplySignal(Signal{Date: "2024-01-02", Ticker: "AAA", Action: "BUY", Price: 100}, 100, 1, cfg)

	// rally to a new peak, then pull back past the 5% trailing distance but
	// still well above the (much lower) fixed stop.
	acct.MarkClose("2024-01-03", map[string]float64{"AAA": 150}, nil, cfg)
	trades := acct.MarkClose("2024-01-04", map[string]float64{"AAA": 140}, nil, cfg)
	if len(trades) != 1 || trades[0].Reason != "trailing" {
		t.Fatalf("expected one trailing-stop exit, got %+v", trades)
	}
}

func TestMarkClose_TakeProfit(t *testing.T) {
	cfg := baseConfig()
	cfg.TakeProfitATRMult = 2 // target = 100 + 2*atr(=1) = 102
	acct := NewAccount(cfg.InitialCash)
	acct.ApplySignal(Signal{Date: "2024-01-02", Ticker: "AAA", Action: "BUY", Price: 100}, 100, 1, cfg)
	h := acct.Holdings["AAA"]
	if h.Target != 102 {
		t.Fatalf("target: got %v, want 102", h.Target)
	}

	trades := acct.MarkClose("2024-01-03", map[string]float64{"AAA": h.Target}, nil, cfg)
	if len(trades) != 1 || trades[0].Reason != "target" {
		t.Fatalf("expected one take-profit exit, got %+v", trades)
	}
	if trades[0].RealizedPnL <= 0 {
		t.Errorf("expected a positive realized pnl on a take-profit exit, got %v", trades[0].RealizedPnL)
	}
	if _, held := acct.Holdings["AAA"]; held {
		t.Errorf("position should be closed after take-profit exit")
	}
}

func TestApplySignal_TakeProfitDisabled(t *testing.T) {
	// TakeProfitATRMult=0 (the default): Target must stay 0 regardless of ATR.
	cfg := baseConfig()
	acct := NewAccount(cfg.InitialCash)
	acct.ApplySignal(Signal{Date: "2024-01-02", Ticker: "AAA", Action: "BUY", Price: 100}, 100, 1, cfg)
	if h := acct.Holdings["AAA"]; h.Target != 0 {
		t.Errorf("TakeProfitATRMult=0: expected Target 0, got %v", h.Target)
	}

	// TakeProfitATRMult>0 but atr<=0 (fixed-%% stop fallback): still no
	// target, since there's no ATR to derive an R-multiple from.
	cfg2 := baseConfig()
	cfg2.TakeProfitATRMult = 2
	acct2 := NewAccount(cfg2.InitialCash)
	acct2.ApplySignal(Signal{Date: "2024-01-02", Ticker: "BBB", Action: "BUY", Price: 100}, 100, 0, cfg2)
	if h := acct2.Holdings["BBB"]; h.Target != 0 {
		t.Errorf("atr<=0: expected Target 0, got %v", h.Target)
	}
}

func TestCashConservation(t *testing.T) {
	cfg := baseConfig()
	acct := NewAccount(cfg.InitialCash)

	acct.ApplySignal(Signal{Date: "2024-01-02", Ticker: "AAA", Action: "BUY", Price: 100}, 100, 1, cfg)
	trade, ok := acct.ApplySignal(Signal{Date: "2024-01-05", Ticker: "AAA", Action: "SELL", Price: 120}, 120, 1, cfg)
	if !ok {
		t.Fatalf("expected sell to execute")
	}

	if !almostEqual(acct.Cash-cfg.InitialCash, trade.RealizedPnL) {
		t.Errorf("cash conservation: cash delta %v != realized pnl %v", acct.Cash-cfg.InitialCash, trade.RealizedPnL)
	}
}

func TestFeeFor_TW(t *testing.T) {
	notional := 100000.0
	sellFee := FeeFor(market.TW, "SELL", notional, 1.0)
	wantFee := notional * (0.001425 + 0.003)
	if !almostEqual(sellFee, wantFee) {
		t.Errorf("TW sell fee: got %v, want %v", sellFee, wantFee)
	}

	cfg := baseConfig()
	cfg.Market = market.TW
	acct := NewAccount(1000000)
	acct.ApplySignal(Signal{Date: "2024-01-02", Ticker: "2330", Action: "BUY", Price: 500}, 500, 5, cfg)
	h := acct.Holdings["2330"]
	cashBeforeSell := acct.Cash

	trade, _ := acct.ApplySignal(Signal{Date: "2024-01-05", Ticker: "2330", Action: "SELL", Price: 550}, 550, 5, cfg)
	sellNotional := 550 * h.Shares
	wantProceeds := sellNotional * (1 - 0.001425 - 0.003)
	gotProceeds := acct.Cash - cashBeforeSell
	if !almostEqual(gotProceeds, wantProceeds) {
		t.Errorf("TW sell proceeds: got %v, want %v", gotProceeds, wantProceeds)
	}
	if trade.Fee <= 0 {
		t.Errorf("expected a nonzero TW sell fee")
	}
}

func TestFeeFor_US(t *testing.T) {
	if got := FeeFor(market.US, "BUY", 10000, 1.0); got != 0 {
		t.Errorf("US fee should be 0, got %v", got)
	}
}

// TestFeeFor_TWDiscountAndMinFee covers §11.2 of docs/tw-us-parity.md: a
// TW buy of 1 lot at NT$100 (notional 100,000) pays NT$142.50 (rounds to
// 142 via float truncation elsewhere) at discount=1.0, but hits the NT$20
// commission floor at discount=0.28 (100,000*0.001425*0.28 = 39.9, still
// above the floor — use a smaller notional to actually hit it).
func TestFeeFor_TWDiscountAndMinFee(t *testing.T) {
	if got, want := FeeFor(market.TW, "BUY", 100000, 1.0), 142.5; !almostEqual(got, want) {
		t.Errorf("TW buy fee at discount=1.0: got %v, want %v", got, want)
	}
	if got, want := FeeFor(market.TW, "BUY", 100000, 0.28), 39.9; !almostEqual(got, want) {
		t.Errorf("TW buy fee at discount=0.28: got %v, want %v", got, want)
	}
	if got, want := FeeFor(market.TW, "BUY", 5000, 0.28), twMinFee; !almostEqual(got, want) {
		t.Errorf("TW buy fee below min: got %v, want floor %v", got, want)
	}
}

// TestFeeFor_TWSellMinFeeAppliesBeforeTax guards against the twMinFee floor
// being (mis)applied to commission+tax combined: a small sell whose
// commission alone rounds below the floor must still pay floor+tax, not
// just the floor — see FeeFor's own doc comment for why order matters.
func TestFeeFor_TWSellMinFeeAppliesBeforeTax(t *testing.T) {
	notional := 5000.0
	discount := 0.6
	commission := notional * 0.001425 * discount // 4.275, below twMinFee
	want := twMinFee + notional*0.003            // 20 + 15 = 35
	if got := FeeFor(market.TW, "SELL", notional, discount); !almostEqual(got, want) {
		t.Errorf("TW sell fee below commission floor: got %v, want %v (commission alone would be %v)", got, want, commission)
	}
}
