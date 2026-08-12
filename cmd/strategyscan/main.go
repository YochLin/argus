package main

import (
	_ "embed"
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"argus/internal/data"
	"argus/internal/market"
	"argus/internal/signals"
)

//go:embed sp500_tickers.txt
var sp500TickersRaw string

//go:embed tw150_tickers.txt
var tw150TickersRaw string

// strategies lists the four rule-based screens in a fixed order shared by the
// hit map, the record loop and the summary printout.
var strategies = []string{"squeeze_breakout", "box_bottom", "trend_breakout", "trend_pullback"}

const baselineStrategy = "baseline"

type TriggerRecord struct {
	Ticker       string
	Date         string
	Strategy     string
	EntryPrice   float64
	MarketRegime string // "bull" (benchmark >= MA50) or "bear" (benchmark < MA50)

	Ret5d  float64
	Ret10d float64
	Ret20d float64
	Has5d  bool
	Has10d bool
	Has20d bool

	BenchRet5d  float64
	BenchRet10d float64
	BenchRet20d float64

	BeatBench5d  bool
	BeatBench10d bool
	BeatBench20d bool
}

func main() {
	marketFlag := flag.String("market", "us", "market to scan: us|tw")
	rangeFlag := flag.String("range", "5y", "history range: 1y|2y|5y")
	flag.Parse()

	m := market.US
	if *marketFlag == "tw" {
		m = market.TW
	} else if *marketFlag != "us" {
		fmt.Printf("Error: -market must be us or tw, got %q\n", *marketFlag)
		os.Exit(1)
	}

	fmt.Printf("=== Argus Strategy Historical Study Tool (cmd/strategyscan, market=%s, range=%s) ===\n", m, *rangeFlag)

	var tickers []string
	benchTicker := "SPY"
	if m == market.TW {
		tickers = parseTickers(tw150TickersRaw)
		benchTicker = "0050"
		fmt.Printf("Loaded %d tw150 tickers.\n", len(tickers))
	} else {
		tickers = parseTickers(sp500TickersRaw)
		fmt.Printf("Loaded %d S&P 500 tickers.\n", len(tickers))
	}
	screenParams := signals.DefaultScreenParams(m)

	yahoo := data.NewYahoo()

	fmt.Printf("Fetching %s history for market regime and benchmark...\n", benchTicker)
	benchCandles, err := yahoo.GetHistory(benchTicker, *rangeFlag)
	if err != nil || len(benchCandles) < 60 {
		fmt.Printf("Error fetching %s history: %v\n", benchTicker, err)
		os.Exit(1)
	}
	fmt.Printf("%s loaded with %d daily bars.\n", benchTicker, len(benchCandles))

	// Map benchmark date string -> index
	benchDateIdx := make(map[string]int)
	for i, c := range benchCandles {
		dateStr := c.Date.Format("2006-01-02")
		benchDateIdx[dateStr] = i
	}

	var records []TriggerRecord

	// §10.2: fetch accounting — "503 listed" was never the same as "503 fetched".
	fetched, fetchFailed, tooShort := 0, 0, 0
	var failedTickers []string

	// §10.4: alert-once dedup, aligned with signals.strategyLookbackDays (unexported;
	// value confirmed at internal/signals/strategies.go). lastHit is reset per ticker.
	const dedupWindowDays = 5
	rawHitCounts := make(map[string]int)
	dedupedHitCounts := make(map[string]int)

	count := 0
	total := len(tickers)
	for _, ticker := range tickers {
		count++
		if count%50 == 0 || count == total {
			fmt.Printf("Processing %d/%d (%s)...\n", count, total, ticker)
		}

		time.Sleep(200 * time.Millisecond) // rate limit
		candles, err := yahoo.GetHistory(ticker, *rangeFlag)
		if err != nil {
			fetchFailed++
			failedTickers = append(failedTickers, ticker)
			continue
		}
		if len(candles) < 60 {
			tooShort++
			continue
		}
		fetched++

		lastHit := make(map[string]int) // strategy -> last recorded index t

		// Evaluate historical triggers for t from index 59 to len(candles)-1
		for t := 59; t < len(candles); t++ {
			sub := candles[:t+1]
			evalDateStr := candles[t].Date.Format("2006-01-02")
			entryPrice := candles[t].Close
			if entryPrice <= 0 {
				continue
			}

			// Broad market regime at evalDate
			marketRegime := "bull"
			if sIdx, ok := benchDateIdx[evalDateStr]; ok && sIdx >= 49 {
				benchSub := benchCandles[:sIdx+1]
				benchMA50 := signals.MA(data.Closes(benchSub), 50)
				if benchMA50 > 0 && benchCandles[sIdx].Close < benchMA50 {
					marketRegime = "bear"
				}
			}

			// §10.3: forward returns computed for every day, not just hit days,
			// so baseline can walk the same (ticker, day) population.
			r5, bR5, ok5 := calcForwardReturn(t, 5, candles, benchCandles, benchDateIdx)
			r10, bR10, ok10 := calcForwardReturn(t, 10, candles, benchCandles, benchDateIdx)
			r20, bR20, ok20 := calcForwardReturn(t, 20, candles, benchCandles, benchDateIdx)

			baseRec := TriggerRecord{
				Ticker:       ticker,
				Date:         evalDateStr,
				EntryPrice:   entryPrice,
				MarketRegime: marketRegime,
			}
			if ok5 {
				baseRec.Ret5d, baseRec.BenchRet5d, baseRec.Has5d = r5, bR5, true
				baseRec.BeatBench5d = r5 > bR5
			}
			if ok10 {
				baseRec.Ret10d, baseRec.BenchRet10d, baseRec.Has10d = r10, bR10, true
				baseRec.BeatBench10d = r10 > bR10
			}
			if ok20 {
				baseRec.Ret20d, baseRec.BenchRet20d, baseRec.Has20d = r20, bR20, true
				baseRec.BeatBench20d = r20 > bR20
			}

			baseline := baseRec
			baseline.Strategy = baselineStrategy
			records = append(records, baseline)

			hits := map[string]bool{
				"squeeze_breakout": signals.CheckSqueezeBreakoutExact(sub, screenParams),
				"box_bottom":       signals.CheckBoxBottomReboundExact(sub, screenParams),
				"trend_breakout":   signals.CheckTrendBreakoutExact(sub, screenParams),
				"trend_pullback":   signals.CheckTrendPullbackExact(sub, screenParams),
			}

			for _, strat := range strategies {
				if !hits[strat] {
					continue
				}
				rawHitCounts[strat]++
				if prev, ok := lastHit[strat]; ok && t-prev < dedupWindowDays {
					continue
				}
				lastHit[strat] = t
				dedupedHitCounts[strat]++

				rec := baseRec
				rec.Strategy = strat
				records = append(records, rec)
			}
		}
	}

	fmt.Printf("\nFinished scanning.\n")
	fmt.Printf("Universe: %d listed / %d fetched / %d fetch errors / %d too short\n",
		total, fetched, fetchFailed, tooShort)
	if total > 0 && float64(fetchFailed)/float64(total) > 0.05 {
		fmt.Printf("WARNING: fetch error rate %.1f%% exceeds 5%% — this run's numbers are NOT comparable to other runs. Slow down the rate limit and re-run.\n",
			float64(fetchFailed)/float64(total)*100.0)
	}
	if len(failedTickers) > 0 {
		shown := failedTickers
		if len(shown) > 10 {
			shown = shown[:10]
		}
		fmt.Printf("Fetch failures (first %d of %d): %s\n", len(shown), len(failedTickers), strings.Join(shown, ", "))
	}

	// ponytail: baseline keeps every (ticker, day) record in memory (~5y x 503
	// tickers ~= 600k rows, ~90MB) rather than streaming stats — fine for an
	// offline tool; switch to running sums if that ever becomes a problem.
	writeCSV(fmt.Sprintf("strategyscan_results_%s.csv", m), records)

	fmt.Printf("\n總觸發次數（去重前 / 去重後，5 個交易日內同檔同策略只記一筆）:\n")
	for _, strat := range strategies {
		fmt.Printf("  • %s: %d 次 / %d 個獨立事件\n", strat, rawHitCounts[strat], dedupedHitCounts[strat])
	}

	// Output summary statistics — baseline first as the reading reference.
	printSummary(benchTicker, "Baseline（全樣本，未篩選）", filterByStrategy(records, baselineStrategy), nil)
	baseline10, baseline20 := summaryStats(filterByStrategy(records, baselineStrategy))
	printSummary(benchTicker, "Squeeze Breakout (網 1)", filterByStrategy(records, "squeeze_breakout"), &baseline10x20{baseline10, baseline20})
	printSummary(benchTicker, "Box Bottom Rebound (網 2)", filterByStrategy(records, "box_bottom"), &baseline10x20{baseline10, baseline20})
	printSummary(benchTicker, "Trend Breakout (網 3)", filterByStrategy(records, "trend_breakout"), &baseline10x20{baseline10, baseline20})
	printSummary(benchTicker, "Trend Pullback (網 4)", filterByStrategy(records, "trend_pullback"), &baseline10x20{baseline10, baseline20})
}

func parseTickers(raw string) []string {
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		t := strings.TrimSpace(line)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

func calcForwardReturn(t, days int, stockCandles, benchCandles []data.Candle, benchDateIdx map[string]int) (stockRet, benchRet float64, ok bool) {
	if t+days >= len(stockCandles) {
		return 0, 0, false
	}
	entryDateStr := stockCandles[t].Date.Format("2006-01-02")
	exitDateStr := stockCandles[t+days].Date.Format("2006-01-02")

	bStart, ok1 := benchDateIdx[entryDateStr]
	bEnd, ok2 := benchDateIdx[exitDateStr]
	if !ok1 || !ok2 {
		return 0, 0, false
	}

	cEntry := stockCandles[t].Close
	cExit := stockCandles[t+days].Close
	if cEntry <= 0 {
		return 0, 0, false
	}
	stockRet = (cExit - cEntry) / cEntry * 100.0

	benchEntry := benchCandles[bStart].Close
	benchExit := benchCandles[bEnd].Close
	if benchEntry <= 0 {
		return 0, 0, false
	}
	benchRet = (benchExit - benchEntry) / benchEntry * 100.0

	return stockRet, benchRet, true
}

func filterByStrategy(records []TriggerRecord, strat string) []TriggerRecord {
	var out []TriggerRecord
	for _, r := range records {
		if r.Strategy == strat {
			out = append(out, r)
		}
	}
	return out
}

// windowStats holds win rate / mean / median for one forward-return window.
type windowStats struct {
	n       int
	winRate float64
	meanRet float64
	medRet  float64
}

// baseline10x20 carries the baseline's 10d/20d stats so a strategy's summary
// can print its excess (§10.3 point 4) — the only number that actually says
// whether a screen beats picking any (ticker, day) at random.
type baseline10x20 struct {
	d10, d20 windowStats
}

func summaryStats(recs []TriggerRecord) (d10, d20 windowStats) {
	var ret10s, ret20s []float64
	var beat10, beat20 int
	for _, r := range recs {
		if r.Has10d {
			ret10s = append(ret10s, r.Ret10d)
			if r.BeatBench10d {
				beat10++
			}
		}
		if r.Has20d {
			ret20s = append(ret20s, r.Ret20d)
			if r.BeatBench20d {
				beat20++
			}
		}
	}
	if n := len(ret10s); n > 0 {
		d10 = windowStats{n, float64(beat10) / float64(n) * 100.0, mean(ret10s), median(ret10s)}
	}
	if n := len(ret20s); n > 0 {
		d20 = windowStats{n, float64(beat20) / float64(n) * 100.0, mean(ret20s), median(ret20s)}
	}
	return d10, d20
}

func printSummary(benchTicker, title string, recs []TriggerRecord, base *baseline10x20) {
	isBaseline := base == nil
	fmt.Printf("\n=======================================================\n")
	fmt.Printf(" 策略統計：%s\n", title)
	fmt.Printf("=======================================================\n")

	totalHits := len(recs)
	fmt.Printf("總觸發次數: %d 次\n", totalHits)
	if totalHits == 0 {
		return
	}

	d10, d20 := summaryStats(recs)

	if d10.n > 0 {
		fmt.Printf("\n[10 日前瞻] (有效樣本: %d 筆)\n", d10.n)
		fmt.Printf("  • 跑贏 %s 勝率: %.1f%%\n", benchTicker, d10.winRate)
		fmt.Printf("  • 平均 10d 報酬: %+.2f%%\n", d10.meanRet)
		fmt.Printf("  • 中位數 10d 報酬: %+.2f%%\n", d10.medRet)
		if base != nil && base.d10.n > 0 {
			fmt.Printf("  • 超額 vs baseline: 勝率 %+.1f 個百分點, 平均報酬 %+.2f%%\n",
				d10.winRate-base.d10.winRate, d10.meanRet-base.d10.meanRet)
		}
	}

	if d20.n > 0 {
		fmt.Printf("\n[20 日前瞻] (有效樣本: %d 筆)\n", d20.n)
		fmt.Printf("  • 跑贏 %s 勝率: %.1f%%\n", benchTicker, d20.winRate)
		fmt.Printf("  • 平均 20d 報酬: %+.2f%%\n", d20.meanRet)
		fmt.Printf("  • 中位數 20d 報酬: %+.2f%%\n", d20.medRet)
		if base != nil && base.d20.n > 0 {
			fmt.Printf("  • 超額 vs baseline: 勝率 %+.1f 個百分點, 平均報酬 %+.2f%%\n",
				d20.winRate-base.d20.winRate, d20.meanRet-base.d20.meanRet)
		}
	}

	// Market Regime breakdown (10d)
	var bull10s, bear10s []TriggerRecord
	for _, r := range recs {
		if !r.Has10d {
			continue
		}
		if r.MarketRegime == "bull" {
			bull10s = append(bull10s, r)
		} else {
			bear10s = append(bear10s, r)
		}
	}
	fmt.Printf("\n[多空情境分組 10d 表現]\n")
	printRegimeGroup(benchTicker, fmt.Sprintf("多頭情境 (%s >= MA50)", benchTicker), bull10s)
	printRegimeGroup(benchTicker, fmt.Sprintf("空頭情境 (%s < MA50)", benchTicker), bear10s)

	// §10.6: baseline's worst-case list has no diagnostic value at hundreds of
	// thousands of rows and just floods the output — skip it.
	if isBaseline {
		return
	}

	var valid10d []TriggerRecord
	for _, r := range recs {
		if r.Has10d {
			valid10d = append(valid10d, r)
		}
	}
	sort.Slice(valid10d, func(i, j int) bool {
		return valid10d[i].Ret10d < valid10d[j].Ret10d
	})
	fmt.Printf("\n[最差 10d 案例抽查 Top 5]\n")
	for i := 0; i < len(valid10d) && i < 5; i++ {
		r := valid10d[i]
		fmt.Printf("  %d. %s (%s) @ $%.2f -> 10d: %+.2f%% (%s: %+.2f%%) [%s]\n",
			i+1, r.Ticker, r.Date, r.EntryPrice, r.Ret10d, benchTicker, r.BenchRet10d, r.MarketRegime)
	}
}

func printRegimeGroup(benchTicker, name string, recs []TriggerRecord) {
	if len(recs) == 0 {
		fmt.Printf("  • %s: 無觸發筆數\n", name)
		return
	}
	var beatCount int
	var rets []float64
	for _, r := range recs {
		if r.BeatBench10d {
			beatCount++
		}
		rets = append(rets, r.Ret10d)
	}
	winRate := float64(beatCount) / float64(len(recs)) * 100.0
	suffix := ""
	if len(recs) < 20 {
		suffix = "（樣本數過小，不下結論）"
	}
	fmt.Printf("  • %s (%d 筆): 跑贏 %s 勝率 %.1f%%, 平均 10d 報酬 %+.2f%%%s\n",
		name, len(recs), benchTicker, winRate, mean(rets), suffix)
}

func mean(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	var sum float64
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

func median(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	copied := append([]float64(nil), vals...)
	sort.Float64s(copied)
	n := len(copied)
	if n%2 == 1 {
		return copied[n/2]
	}
	return (copied[n/2-1] + copied[n/2]) / 2.0
}

func writeCSV(path string, recs []TriggerRecord) {
	f, err := os.Create(path)
	if err != nil {
		fmt.Printf("Error creating CSV: %v\n", err)
		return
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	w.Write([]string{
		"Ticker", "Date", "Strategy", "EntryPrice", "MarketRegime",
		"Ret5d", "BenchRet5d", "BeatBench5d",
		"Ret10d", "BenchRet10d", "BeatBench10d",
		"Ret20d", "BenchRet20d", "BeatBench20d",
	})

	for _, r := range recs {
		if r.Strategy == baselineStrategy {
			continue // §10.3: baseline is a few hundred thousand rows; CSV is for manual spot-checks.
		}
		w.Write([]string{
			r.Ticker,
			r.Date,
			r.Strategy,
			fmt.Sprintf("%.2f", r.EntryPrice),
			r.MarketRegime,
			fmt.Sprintf("%.2f", r.Ret5d),
			fmt.Sprintf("%.2f", r.BenchRet5d),
			fmt.Sprintf("%t", r.BeatBench5d),
			fmt.Sprintf("%.2f", r.Ret10d),
			fmt.Sprintf("%.2f", r.BenchRet10d),
			fmt.Sprintf("%t", r.BeatBench10d),
			fmt.Sprintf("%.2f", r.Ret20d),
			fmt.Sprintf("%.2f", r.BenchRet20d),
			fmt.Sprintf("%t", r.BeatBench20d),
		})
	}
	fmt.Printf("Saved CSV report to %s\n", path)
}
