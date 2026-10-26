package main

import (
	"encoding/csv"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"sort"
	"strconv"
	"time"

	"argus/internal/data"
	"argus/internal/signals"
)

// breakout.go is the momentum-breakout study (-mb-backtest): a
// Qullamaggie-style "momentum leader, tight base, breakout, tight stop"
// strategy, specified with the user before any of it was run (2026-10-26).
// Like momentum.go it needs its own engine — its stop is the entry day's
// low, it sells half on day 3 and trails the rest on a moving average, and
// its candidate list is a cross-sectional rank — none of which
// paper.Account models.
//
// Pre-registered rules (§4.4: the bar and every number below are fixed;
// a different value is a new run, not a retry):
//
//   - Regime: S&P 500 (SPY) MA10 and MA20 both above their value 5 days
//     earlier, as of the previous close. Gates new entries only.
//   - List: top 5% of the pool by 1-, 3- and 6-month return (union), then
//     ADR20 >= 5% and 20-day average dollar volume >= $5M, then at most 50
//     by highest ADR. All as of the previous close.
//   - Base: signals.DetectMomentumBase on bars up to the previous close (the
//     live watch shares it), its high being the pivot.
//   - Entry: today's high > pivot. Fill at max(open, pivot) plus 0.2%
//     slippage. Stop = today's low; skipped if wider than one ADR. Skipped
//     if an SEC filing date falls within the next 10 calendar days.
//   - Exit: stop first. From day 3 on, the first close above entry sells
//     half and moves the stop to entry; a day-3 loser is simply held until
//     it either closes above entry or hits the stop. Only after that half is
//     sold does a close below MA10 sell the rest.
//   - Control: a random 10% of every (ticker, regime-on day) in the pool,
//     bought at that close, stop one ADR below, same exits.
//   - Bar: mean exit return minus the control's, > 1 SE in both slices
//     (split 2021-11-01), same sign. SE from a bootstrap that resamples
//     entry MONTHS, since trades held for weeks co-move within a month.
//
// Known biases of the daily-bar approximation, reported rather than fixed:
// the stop is the whole day's low, so a breakout that failed the same day
// below the low-at-entry-time is never stopped out on day 1 (the "pessimistic"
// reference row bounds this); the pool is today's S&P 400+600 members
// (survivorship) and misses non-index growth names.
// mbMinADRPct is the list's ADR floor, the one knob -mb-min-adr exposes:
// 5% is the rule as specified for the whole market; 3% is a separate,
// pre-stated run for S&P 500, whose large caps rarely move 5% a day.
var mbMinADRPct = 5.0

const (
	mbRankPct        = 0.05
	mbMinDollarVol   = 5e6
	mbMaxCandidates  = 50
	mbEarningsDays   = 10
	mbSlippagePct    = 0.2
	mbPartialDay     = 3
	mbRegimeSlope    = 5
	mbSplit          = "2021-11-01"
	mbControlRate    = 0.1
	mbBootstraps     = 400
	mbRiskPct        = 1.0
	mbMaxPositionPct = 25.0
	mbMaxPositions   = 4
	mbMinHistory     = 126 // 6-month return, and enough room for base + prior run
)

// mbSeries is one ticker's candles plus the indicators the study reads,
// each at index i computed through bar i inclusive.
type mbSeries struct {
	c                           []data.Candle
	idx                         map[string]int
	adr, dvol, ma10, ma20, ma50 []float64
}

func newMBSeries(c []data.Candle) *mbSeries {
	n := len(c)
	s := &mbSeries{c: c, idx: make(map[string]int, n)}
	rng := make([]float64, n)
	dv := make([]float64, n)
	cl := make([]float64, n)
	for i, k := range c {
		s.idx[k.Date.Format("2006-01-02")] = i
		if k.Low > 0 {
			rng[i] = (k.High/k.Low - 1) * 100
		}
		dv[i] = k.Close * float64(k.Volume)
		cl[i] = k.Close
	}
	s.adr, s.dvol = rollingMean(rng, 20), rollingMean(dv, 20)
	s.ma10, s.ma20, s.ma50 = rollingMean(cl, 10), rollingMean(cl, 20), rollingMean(cl, 50)
	return s
}

// rollingMean is the trailing n-bar mean at each index, 0 before n bars exist.
func rollingMean(v []float64, n int) []float64 {
	out := make([]float64, len(v))
	var sum float64
	for i, x := range v {
		sum += x
		if i >= n {
			sum -= v[i-n]
		}
		if i >= n-1 {
			out[i] = sum / float64(n)
		}
	}
	return out
}

func (s *mbSeries) ret(e, n int) float64 { return s.c[e].Close/s.c[e-n].Close - 1 }

// mbFill is part of a position leaving: frac of the original shares at price.
type mbFill struct {
	date        string
	frac, price float64
	why         string // stop, partial, ma, end
}

type mbTrade struct {
	ticker, date string
	t            int // entry bar in the ticker's series
	entry, stop  float64
	mom3m        float64
	ret          float64 // % on the whole position, all fills
	fills        []mbFill
}

// mbSimulate runs the exit rules from entry bar t. maExit is the trailing MA
// (s.ma10 or s.ma20). pessimistic stops the trade out on the entry day
// whenever that day closed below entry — the bound on the whole-day-low
// stop's optimism.
func mbSimulate(s *mbSeries, tr *mbTrade, maExit []float64, pessimistic bool) {
	date := func(i int) string { return s.c[i].Date.Format("2006-01-02") }
	stop, rem := tr.stop, 1.0
	sell := func(i int, frac, price float64, why string) {
		tr.fills = append(tr.fills, mbFill{date(i), frac, price, why})
		rem -= frac
	}
	if pessimistic && s.c[tr.t].Close < tr.entry {
		sell(tr.t, 1, tr.stop, "stop")
	}
	for i := tr.t + 1; i < len(s.c) && rem > 0; i++ {
		k := s.c[i]
		if k.Low <= stop {
			sell(i, rem, math.Min(k.Open, stop), "stop")
			break
		}
		partial := rem < 1
		if !partial && i >= tr.t+mbPartialDay && k.Close > tr.entry {
			sell(i, 0.5, k.Close, "partial")
			stop = math.Max(stop, tr.entry)
			partial = true
		}
		if partial && k.Close < maExit[i] {
			sell(i, rem, k.Close, "ma")
		}
	}
	if rem > 1e-9 { // still open at the end of the data: marked at the last close
		sell(len(s.c)-1, rem, s.c[len(s.c)-1].Close, "end")
	}
	var got float64
	for _, f := range tr.fills {
		got += f.frac * f.price
	}
	tr.ret = (got/tr.entry - 1) * 100
}

type mbStudy struct {
	pool    map[string]*mbSeries
	tickers []string
	dates   []string // benchmark calendar
	regime  []bool   // regime on, decided at dates[i-1]'s close
	earn    map[string]map[string]bool
}

func newMBStudy(cache map[string][]data.Candle, bench []data.Candle, benchTicker string, earn map[string]map[string]bool) *mbStudy {
	st := &mbStudy{pool: make(map[string]*mbSeries), earn: earn}
	for t, c := range cache {
		if t == benchTicker {
			continue
		}
		st.pool[t] = newMBSeries(c)
		st.tickers = append(st.tickers, t)
	}
	sort.Strings(st.tickers)
	b := newMBSeries(bench)
	st.dates = make([]string, len(bench))
	st.regime = make([]bool, len(bench))
	for i, k := range bench {
		st.dates[i] = k.Date.Format("2006-01-02")
		if p := i - 1; p-mbRegimeSlope >= 19 {
			st.regime[i] = b.ma10[p] > b.ma10[p-mbRegimeSlope] && b.ma20[p] > b.ma20[p-mbRegimeSlope]
		}
	}
	return st
}

func (st *mbStudy) earningsSoon(ticker string, d time.Time) bool {
	for k := 0; k <= mbEarningsDays; k++ {
		if st.earn[ticker][d.AddDate(0, 0, k).Format("2006-01-02")] {
			return true
		}
	}
	return false
}

// mbFunnel counts how many (ticker, day) pairs survive each stage, summed
// over days — to tell which rule is the one starving the trade count.
type mbFunnel struct{ days, listed, based, broke, narrow, trades int }

// signals returns every breakout entry, gated on the regime or not.
func (st *mbStudy) signals(gated bool) ([]mbTrade, mbFunnel) {
	var out []mbTrade
	var f mbFunnel
	type ranked struct {
		t    string
		i    int
		r    [3]float64
		adr  float64
		keep bool
	}
	for di, d := range st.dates {
		if di == 0 || (gated && !st.regime[di]) {
			continue
		}
		f.days++
		var pool []*ranked
		for _, t := range st.tickers {
			s := st.pool[t]
			i, ok := s.idx[d]
			if !ok || i-1 < mbMinHistory {
				continue
			}
			e := i - 1
			pool = append(pool, &ranked{t: t, i: i, r: [3]float64{s.ret(e, 21), s.ret(e, 63), s.ret(e, 126)}, adr: st.pool[t].adr[e]})
		}
		k := int(math.Ceil(mbRankPct * float64(len(pool))))
		for w := range 3 {
			sort.Slice(pool, func(a, b int) bool { return pool[a].r[w] > pool[b].r[w] })
			for _, p := range pool[:min(k, len(pool))] {
				p.keep = true
			}
		}
		var list []*ranked
		for _, p := range pool {
			if p.keep && p.adr >= mbMinADRPct && st.pool[p.t].dvol[p.i-1] >= mbMinDollarVol {
				list = append(list, p)
			}
		}
		sort.Slice(list, func(a, b int) bool { return list[a].adr > list[b].adr })
		if len(list) > mbMaxCandidates {
			list = list[:mbMaxCandidates]
		}
		f.listed += len(list)
		for _, p := range list {
			s := st.pool[p.t]
			base, ok := signals.DetectMomentumBase(s.c[:p.i])
			pivot := base.Pivot
			if !ok {
				continue
			}
			f.based++
			k := s.c[p.i]
			if k.High <= pivot {
				continue
			}
			f.broke++
			entry := math.Max(k.Open, pivot) * (1 + mbSlippagePct/100)
			if (entry-k.Low)/entry*100 > p.adr {
				continue
			}
			f.narrow++
			if st.earningsSoon(p.t, k.Date) {
				continue
			}
			f.trades++
			out = append(out, mbTrade{ticker: p.t, date: d, t: p.i, entry: entry, stop: k.Low, mom3m: p.r[1]})
		}
	}
	return out, f
}

// control samples mbControlRate of every eligible (ticker, day).
func (st *mbStudy) control(gated bool) []mbTrade {
	rng := rand.New(rand.NewPCG(20261026, 1))
	regimeOn := make(map[string]bool, len(st.dates))
	for i, d := range st.dates {
		regimeOn[d] = !gated || st.regime[i]
	}
	var out []mbTrade
	for _, t := range st.tickers {
		s := st.pool[t]
		for i := mbMinHistory + 1; i < len(s.c)-1; i++ {
			d := s.c[i].Date.Format("2006-01-02")
			if !regimeOn[d] || s.adr[i] <= 0 || rng.Float64() >= mbControlRate {
				continue
			}
			c := s.c[i].Close
			out = append(out, mbTrade{ticker: t, date: d, t: i, entry: c, stop: c * (1 - s.adr[i]/100), mom3m: s.ret(i, 63)})
		}
	}
	return out
}

func (st *mbStudy) simulate(trades []mbTrade, ma20, pessimistic bool) []mbTrade {
	out := make([]mbTrade, len(trades))
	for i, tr := range trades {
		s := st.pool[tr.ticker]
		ma := s.ma10
		if ma20 {
			ma = s.ma20
		}
		mbSimulate(s, &tr, ma, pessimistic)
		out[i] = tr
	}
	return out
}

// mbExcess is mean(strat) - mean(ctrl) over trades entered in [from, to),
// with a bootstrap SE that resamples entry months independently per side.
func mbExcess(strat, ctrl []mbTrade, from, to string, seed uint64) (n int, sMean, cMean, se float64) {
	byMonth := func(ts []mbTrade) ([]string, map[string][]float64) {
		m := make(map[string][]float64)
		for _, t := range ts {
			if t.date >= from && (to == "" || t.date < to) {
				m[t.date[:7]] = append(m[t.date[:7]], t.ret)
			}
		}
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return keys, m
	}
	flat := func(keys []string, m map[string][]float64) []float64 {
		var v []float64
		for _, k := range keys {
			v = append(v, m[k]...)
		}
		return v
	}
	sk, sm := byMonth(strat)
	ck, cm := byMonth(ctrl)
	sv, cv := flat(sk, sm), flat(ck, cm)
	if len(sv) == 0 || len(cv) == 0 {
		return len(sv), 0, 0, math.NaN()
	}
	rng := rand.New(rand.NewPCG(seed, 2))
	draw := func(keys []string, m map[string][]float64) []float64 {
		var v []float64
		for range keys {
			v = append(v, m[keys[rng.IntN(len(keys))]]...)
		}
		return v
	}
	boots := make([]float64, mbBootstraps)
	for b := range boots {
		boots[b] = mean(draw(sk, sm)) - mean(draw(ck, cm))
	}
	bm := mean(boots)
	var ss float64
	for _, x := range boots {
		ss += (x - bm) * (x - bm)
	}
	return len(sv), mean(sv), mean(cv), math.Sqrt(ss / float64(len(boots)-1))
}

// portfolio replays trades through one account over [from, to): 1% risk,
// 25% cap, at most 4 names, highest 3-month return first on a crowded day.
func (st *mbStudy) portfolio(trades []mbTrade, from, to string) []equityPoint {
	byDate := make(map[string][]mbTrade)
	for _, t := range trades {
		byDate[t.date] = append(byDate[t.date], t)
	}
	type pos struct {
		tr     mbTrade
		shares float64
		next   int // next unprocessed fill
		rem    float64
	}
	cash, equity := 100000.0, 100000.0
	var open []*pos
	var curve []equityPoint
	for _, d := range st.dates {
		if d < from || (to != "" && d >= to) {
			continue
		}
		kept := open[:0]
		for _, p := range open {
			for ; p.next < len(p.tr.fills) && p.tr.fills[p.next].date <= d; p.next++ {
				f := p.tr.fills[p.next]
				cash += p.shares * f.frac * f.price
				p.rem -= f.frac
			}
			if p.rem > 1e-9 {
				kept = append(kept, p)
			}
		}
		open = kept
		todays := byDate[d]
		sort.SliceStable(todays, func(a, b int) bool { return todays[a].mom3m > todays[b].mom3m })
		for _, tr := range todays {
			if len(open) >= mbMaxPositions {
				break
			}
			held := false
			for _, p := range open {
				held = held || p.tr.ticker == tr.ticker
			}
			if held {
				continue
			}
			shares := equity * mbMaxPositionPct / 100 / tr.entry
			if risk := tr.entry - tr.stop; risk > 0 {
				shares = math.Min(shares, equity*mbRiskPct/100/risk)
			}
			shares = math.Floor(math.Min(shares, cash/tr.entry))
			if shares < 1 {
				continue
			}
			cash -= shares * tr.entry
			open = append(open, &pos{tr: tr, shares: shares, rem: 1})
		}
		equity = cash
		for _, p := range open {
			s := st.pool[p.tr.ticker]
			i, ok := s.idx[d]
			if !ok { // no bar today: last close before d
				i = sort.Search(len(s.c), func(j int) bool { return s.c[j].Date.Format("2006-01-02") > d }) - 1
			}
			equity += p.shares * p.rem * s.c[i].Close
		}
		curve = append(curve, equityPoint{Date: d, Equity: equity, Cash: cash, PositionsHeld: len(open)})
	}
	return curve
}

func runMBBacktest(cache map[string][]data.Candle, bench []data.Candle, benchTicker string, earn map[string]map[string]bool, tradesOut string) {
	st := newMBStudy(cache, bench, benchTicker, earn)
	fmt.Printf("\n=== 動能突破回測（-mb-backtest）：%d 檔股票池，%s..%s，ADR 門檻 %.1f%% ===\n", len(st.tickers), st.dates[0], st.dates[len(st.dates)-1], mbMinADRPct)

	gatedSig, fn := st.signals(true)
	openSig, _ := st.signals(false)
	fmt.Printf("篩選漏斗（主規則，加總所有交易日）：大盤過濾開著 %d 天 → 動能名單 %d → 有整理型態 %d → 當天突破 %d → 停損寬度 ≤ 1 ADR %d → 避開財報 %d 筆\n",
		fn.days, fn.listed, fn.based, fn.broke, fn.narrow, fn.trades)
	primary := st.simulate(gatedSig, false, false)
	ctrl := st.simulate(st.control(true), false, false)

	type row struct {
		name        string
		strat, ctrl []mbTrade
	}
	rows := []row{
		{"主規則（大盤過濾 + MA10 出場）", primary, ctrl},
		{"參考：沒有大盤過濾", st.simulate(openSig, false, false), st.simulate(st.control(false), false, false)},
		{"參考：MA20 出場", st.simulate(gatedSig, true, false), st.simulate(st.control(true), true, false)},
		{"參考：進場日收在進場價下就算停損（悲觀）", st.simulate(gatedSig, false, true), ctrl},
	}
	slices := [][2]string{{"", mbSplit}, {mbSplit, ""}}
	fmt.Printf("基準 = 隨機抽 %.0f%% 的（股票, 日）於收盤進場、停損 1 ADR、同樣出場；SE 以進場月份重抽 %d 次\n", mbControlRate*100, mbBootstraps)
	for _, r := range rows {
		fmt.Printf("\n%s\n", r.name)
		fmt.Printf("  %-14s %6s %10s %10s %10s %7s %7s\n", "時段", "筆數", "策略平均", "基準平均", "超額", "SE", "σ")
		for si, sl := range slices {
			n, s, c, se := mbExcess(r.strat, r.ctrl, sl[0], sl[1], uint64(si+1))
			label := "~" + mbSplit
			if sl[0] != "" {
				label = mbSplit + "~"
			}
			fmt.Printf("  %-14s %6d %+9.2f%% %+9.2f%% %+9.2f%% %6.2f %6.1f\n", label, n, s, c, s-c, se, (s-c)/se)
		}
	}

	fmt.Printf("\n組合（十萬美元、每筆風險 %.0f%%、單檔上限 %.0f%%、最多 %d 檔；只作參考）\n", mbRiskPct, mbMaxPositionPct, mbMaxPositions)
	for _, sl := range slices {
		curve := st.portfolio(primary, sl[0], sl[1])
		if len(curve) < 2 {
			continue
		}
		fmt.Printf("  %s..%s  年化 %+.2f%%  最大回撤 %.2f%%  Sharpe %.2f  期末 $%.0f\n",
			curve[0].Date, curve[len(curve)-1].Date, annualizedReturnPct(curve), maxDrawdownPct(curve), sharpeAnnualized(curve), curve[len(curve)-1].Equity)
	}

	if tradesOut == "" {
		return
	}
	f, err := os.Create(tradesOut)
	if err != nil {
		fmt.Printf("Error creating %s: %v\n", tradesOut, err)
		return
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	w.Write([]string{"Date", "Ticker", "Entry", "Stop", "Mom3m", "Ret", "ExitDate", "Path"})
	ff := func(v float64) string { return strconv.FormatFloat(v, 'f', 4, 64) }
	for _, t := range primary {
		var path string
		for i, f := range t.fills {
			if i > 0 {
				path += "+"
			}
			path += f.why
		}
		w.Write([]string{t.date, t.ticker, ff(t.entry), ff(t.stop), ff(t.mom3m), ff(t.ret), t.fills[len(t.fills)-1].date, path})
	}
	fmt.Printf("\n主規則逐筆交易 -> %s\n", tradesOut)
}
