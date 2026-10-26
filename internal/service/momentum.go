package service

import (
	"context"
	"math"
	"sort"
	"time"

	"argus/internal/data"
	"argus/internal/logger"
	"argus/internal/signals"
)

// The momentum-breakout watch's list rules, as the user set them for S&P 500
// (see signals.TypeMomentumSetup for the backtest behind them). Fixed, not
// knobs: they are the rules that were measured.
const (
	momentumRankPct        = 0.05
	momentumMinADRPct      = 3.0
	momentumMinDollarVol   = 5e6
	momentumMaxCandidates  = 50
	momentumEarningsDays   = 10
	momentumRegimeSlope    = 5
	momentumMinHistoryBars = 127 // a 6-month return, plus the base's own lookback
)

// MomentumRequestDelay paces the watch's ~500 history fetches like the
// universe scan's.
const MomentumRequestDelay = universeScanRequestDelay

// MomentumRegime is SPY's MA10/MA20 now (today's price as the last bar)
// against five sessions earlier. On is both rising.
type MomentumRegime struct {
	On                             bool
	Price                          float64
	MA10, MA10Prev, MA20, MA20Prev float64
}

// MomentumPick is one stock whose base is complete. Broke is whether today's
// high so far went over the pivot; Entry/StopPct describe that breakout —
// Entry is max(open, pivot), StopPct how far today's low so far sits below
// it — and are zero otherwise.
type MomentumPick struct {
	Ticker   string
	Pivot    float64
	BaseDays int
	ADRPct   float64
	Mom3mPct float64
	Quote    *data.Quote
	Broke    bool
	Entry    float64
	StopPct  float64
}

// WideStop is the user's "stop wider than one ADR" warning.
func (p MomentumPick) WideStop() bool { return p.Broke && p.StopPct > p.ADRPct }

// OpenBreak is a gap over the pivot at the open.
func (p MomentumPick) OpenBreak() bool { return p.Broke && p.Quote.Open > p.Pivot }

// MomentumWatch is one run's result. Picks is empty when Regime is off: the
// rule is not to trade then, so nothing else is fetched. EarningsUnknown is
// set when no earnings calendar was available to filter with.
type MomentumWatch struct {
	Regime          MomentumRegime
	Picks           []MomentumPick
	EarningsUnknown bool
}

// UpcomingEarningsReader is the one data.EarningsProvider method the watch
// needs.
type UpcomingEarningsReader interface {
	GetUpcomingEarnings(tickers []string, days int) (map[string]data.EarningsEvent, error)
}

// ScanMomentumBreakout runs the watch over tickers (the S&P 500) during the
// US session. Ranking, ADR and the base use bars through the previous close —
// a base is complete before the day it breaks — while the regime and the
// breakout use the live quote. earnings may be nil. delay paces the history
// fetches the same way the universe scan does.
func ScanMomentumBreakout(ctx context.Context, history RiskHistoryReader, quotes QuoteReader, earnings UpcomingEarningsReader, tickers []string, now time.Time, delay time.Duration) (*MomentumWatch, error) {
	spy, err := quotes.GetQuote("SPY")
	if err != nil {
		return nil, err
	}
	spyBars, err := history.GetHistory("SPY", "1y")
	if err != nil {
		return nil, err
	}
	closes := append(data.Closes(completedBars(spyBars, "SPY", now)), spy.Price)
	n := len(closes)
	if n < 20+momentumRegimeSlope {
		return nil, ErrHistoryUnavailable
	}
	r := MomentumRegime{
		Price:    spy.Price,
		MA10:     signals.MA(closes, 10),
		MA10Prev: signals.MA(closes[:n-momentumRegimeSlope], 10),
		MA20:     signals.MA(closes, 20),
		MA20Prev: signals.MA(closes[:n-momentumRegimeSlope], 20),
	}
	r.On = r.MA10 > r.MA10Prev && r.MA20 > r.MA20Prev
	out := &MomentumWatch{Regime: r}
	if !r.On {
		return out, nil
	}

	type ranked struct {
		ticker string
		bars   []data.Candle
		ret    [3]float64
		keep   bool
	}
	var pool []*ranked
	for i, t := range tickers {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if i > 0 {
			time.Sleep(delay)
		}
		bars, err := history.GetHistory(t, "1y")
		if err != nil {
			logger.Errorf("momentum watch: history %s: %v", t, err)
			continue
		}
		bars = completedBars(bars, t, now)
		if len(bars) < momentumMinHistoryBars {
			continue
		}
		c := data.Closes(bars)
		e := len(c) - 1
		pool = append(pool, &ranked{ticker: t, bars: bars, ret: [3]float64{c[e]/c[e-21] - 1, c[e]/c[e-63] - 1, c[e]/c[e-126] - 1}})
	}
	k := int(math.Ceil(momentumRankPct * float64(len(pool))))
	for w := range 3 {
		sort.Slice(pool, func(a, b int) bool { return pool[a].ret[w] > pool[b].ret[w] })
		for _, p := range pool[:min(k, len(pool))] {
			p.keep = true
		}
	}
	var picks []MomentumPick
	for _, p := range pool {
		adr := signals.ADRPct(p.bars, 20)
		if !p.keep || adr < momentumMinADRPct || dollarVolume20(p.bars) < momentumMinDollarVol {
			continue
		}
		picks = append(picks, MomentumPick{Ticker: p.ticker, ADRPct: adr, Mom3mPct: p.ret[1] * 100})
	}
	sort.Slice(picks, func(a, b int) bool { return picks[a].ADRPct > picks[b].ADRPct })
	if len(picks) > momentumMaxCandidates {
		picks = picks[:momentumMaxCandidates]
	}
	bars := make(map[string][]data.Candle, len(pool))
	for _, p := range pool {
		bars[p.ticker] = p.bars
	}
	based := picks[:0]
	for _, p := range picks {
		if b, ok := signals.DetectMomentumBase(bars[p.Ticker]); ok {
			p.Pivot, p.BaseDays = b.Pivot, b.Days
			based = append(based, p)
		}
	}
	picks = based

	if earnings == nil {
		out.EarningsUnknown = true
	} else if len(picks) > 0 {
		names := make([]string, len(picks))
		for i, p := range picks {
			names[i] = p.Ticker
		}
		events, err := earnings.GetUpcomingEarnings(names, momentumEarningsDays)
		if err != nil {
			logger.Errorf("momentum watch: earnings: %v", err)
			out.EarningsUnknown = true
		}
		kept := picks[:0]
		for _, p := range picks {
			if _, soon := events[p.Ticker]; !soon {
				kept = append(kept, p)
			}
		}
		picks = kept
	}

	for _, p := range picks {
		q, err := quotes.GetQuote(p.Ticker)
		if err != nil {
			logger.Errorf("momentum watch: quote %s: %v", p.Ticker, err)
			continue
		}
		p.Quote = q
		if q.High > p.Pivot && q.Low > 0 {
			p.Broke = true
			p.Entry = math.Max(q.Open, p.Pivot)
			p.StopPct = (p.Entry - q.Low) / p.Entry * 100
		}
		out.Picks = append(out.Picks, p)
	}
	sort.SliceStable(out.Picks, func(a, b int) bool {
		if out.Picks[a].Broke != out.Picks[b].Broke {
			return out.Picks[a].Broke
		}
		return out.Picks[a].Mom3mPct > out.Picks[b].Mom3mPct
	})
	return out, nil
}

// completedBars drops a still-forming session from the end of bars.
func completedBars(bars []data.Candle, ticker string, now time.Time) []data.Candle {
	if n := len(bars); n > 0 && BarForming(bars, n-1, ticker, now) {
		return bars[:n-1]
	}
	return bars
}

func dollarVolume20(bars []data.Candle) float64 {
	if len(bars) < 20 {
		return 0
	}
	var sum float64
	for _, c := range bars[len(bars)-20:] {
		sum += c.Close * float64(c.Volume)
	}
	return sum / 20
}
