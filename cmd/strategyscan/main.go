package main

import (
	_ "embed"
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"argus/internal/data"
	"argus/internal/signals"
)

//go:embed sp500_tickers.txt
var sp500TickersRaw string

type TriggerRecord struct {
	Ticker       string
	Date         string
	Strategy     string
	EntryPrice   float64
	MarketRegime string // "bull" (SPY >= MA50) or "bear" (SPY < MA50)

	Ret5d  float64
	Ret10d float64
	Ret20d float64

	SpyRet5d  float64
	SpyRet10d float64
	SpyRet20d float64

	BeatSpy5d  bool
	BeatSpy10d bool
	BeatSpy20d bool
}

func main() {
	fmt.Println("=== Argus Strategy Historical Study Tool (cmd/strategyscan) ===")

	tickers := parseTickers(sp500TickersRaw)
	fmt.Printf("Loaded %d S&P 500 tickers.\n", len(tickers))

	yahoo := data.NewYahoo()

	fmt.Println("Fetching SPY history for market regime and benchmark...")
	spyCandles, err := yahoo.GetHistory("SPY", "1y")
	if err != nil || len(spyCandles) < 60 {
		fmt.Printf("Error fetching SPY history: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("SPY loaded with %d daily bars.\n", len(spyCandles))

	// Map SPY date string -> index
	spyDateIdx := make(map[string]int)
	for i, c := range spyCandles {
		dateStr := c.Date.Format("2006-01-02")
		spyDateIdx[dateStr] = i
	}

	var records []TriggerRecord

	count := 0
	total := len(tickers)
	for _, ticker := range tickers {
		count++
		if count%50 == 0 || count == total {
			fmt.Printf("Processing %d/%d (%s)...\n", count, total, ticker)
		}

		time.Sleep(200 * time.Millisecond) // rate limit
		candles, err := yahoo.GetHistory(ticker, "1y")
		if err != nil || len(candles) < 60 {
			continue
		}

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
			if sIdx, ok := spyDateIdx[evalDateStr]; ok && sIdx >= 49 {
				spySub := spyCandles[:sIdx+1]
				spyMA50 := signals.MA(data.Closes(spySub), 50)
				if spyMA50 > 0 && spyCandles[sIdx].Close < spyMA50 {
					marketRegime = "bear"
				}
			}

			isSqueeze := signals.CheckSqueezeBreakoutExact(sub)
			isBox := signals.CheckBoxBottomReboundExact(sub)

			if !isSqueeze && !isBox {
				continue
			}

			// Check forward returns
			r5, spyR5, ok5 := calcForwardReturn(t, 5, candles, spyCandles, spyDateIdx)
			r10, spyR10, ok10 := calcForwardReturn(t, 10, candles, spyCandles, spyDateIdx)
			r20, spyR20, ok20 := calcForwardReturn(t, 20, candles, spyCandles, spyDateIdx)

			if isSqueeze {
				rec := TriggerRecord{
					Ticker:       ticker,
					Date:         evalDateStr,
					Strategy:     "squeeze_breakout",
					EntryPrice:   entryPrice,
					MarketRegime: marketRegime,
				}
				if ok5 {
					rec.Ret5d = r5
					rec.SpyRet5d = spyR5
					rec.BeatSpy5d = r5 > spyR5
				}
				if ok10 {
					rec.Ret10d = r10
					rec.SpyRet10d = spyR10
					rec.BeatSpy10d = r10 > spyR10
				}
				if ok20 {
					rec.Ret20d = r20
					rec.SpyRet20d = spyR20
					rec.BeatSpy20d = r20 > spyR20
				}
				records = append(records, rec)
			}

			if isBox {
				rec := TriggerRecord{
					Ticker:       ticker,
					Date:         evalDateStr,
					Strategy:     "box_bottom",
					EntryPrice:   entryPrice,
					MarketRegime: marketRegime,
				}
				if ok5 {
					rec.Ret5d = r5
					rec.SpyRet5d = spyR5
					rec.BeatSpy5d = r5 > spyR5
				}
				if ok10 {
					rec.Ret10d = r10
					rec.SpyRet10d = spyR10
					rec.BeatSpy10d = r10 > spyR10
				}
				if ok20 {
					rec.Ret20d = r20
					rec.SpyRet20d = spyR20
					rec.BeatSpy20d = r20 > spyR20
				}
				records = append(records, rec)
			}
		}
	}

	fmt.Printf("\nFinished scanning. Total trigger events recorded: %d\n", len(records))

	// Write CSV
	writeCSV("strategyscan_results.csv", records)

	// Output summary statistics
	printSummary("Squeeze Breakout (網 1)", filterByStrategy(records, "squeeze_breakout"))
	printSummary("Box Bottom Rebound (網 2)", filterByStrategy(records, "box_bottom"))
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

func calcForwardReturn(t, days int, stockCandles, spyCandles []data.Candle, spyDateIdx map[string]int) (stockRet, spyRet float64, ok bool) {
	if t+days >= len(stockCandles) {
		return 0, 0, false
	}
	entryDateStr := stockCandles[t].Date.Format("2006-01-02")
	exitDateStr := stockCandles[t+days].Date.Format("2006-01-02")

	sStart, ok1 := spyDateIdx[entryDateStr]
	sEnd, ok2 := spyDateIdx[exitDateStr]
	if !ok1 || !ok2 {
		return 0, 0, false
	}

	cEntry := stockCandles[t].Close
	cExit := stockCandles[t+days].Close
	if cEntry <= 0 {
		return 0, 0, false
	}
	stockRet = (cExit - cEntry) / cEntry * 100.0

	spyEntry := spyCandles[sStart].Close
	spyExit := spyCandles[sEnd].Close
	if spyEntry <= 0 {
		return 0, 0, false
	}
	spyRet = (spyExit - spyEntry) / spyEntry * 100.0

	return stockRet, spyRet, true
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

func printSummary(title string, recs []TriggerRecord) {
	fmt.Printf("\n=======================================================\n")
	fmt.Printf(" 策略統計：%s\n", title)
	fmt.Printf("=======================================================\n")

	totalHits := len(recs)
	fmt.Printf("總觸發次數: %d 次\n", totalHits)
	if totalHits == 0 {
		return
	}

	// 10-day metrics
	var valid10d []TriggerRecord
	var beatSpy10Count int
	var ret10s []float64
	var bull10s, bear10s []TriggerRecord

	for _, r := range recs {
		if r.Ret10d != 0 || r.SpyRet10d != 0 {
			valid10d = append(valid10d, r)
			ret10s = append(ret10s, r.Ret10d)
			if r.BeatSpy10d {
				beatSpy10Count++
			}
			if r.MarketRegime == "bull" {
				bull10s = append(bull10s, r)
			} else {
				bear10s = append(bear10s, r)
			}
		}
	}

	if len(valid10d) > 0 {
		winRate10 := float64(beatSpy10Count) / float64(len(valid10d)) * 100.0
		mean10 := mean(ret10s)
		med10 := median(ret10s)
		fmt.Printf("\n[10 日前瞻] (有效樣本: %d 筆)\n", len(valid10d))
		fmt.Printf("  • 跑贏 SPY 勝率: %.1f%% (%d/%d)\n", winRate10, beatSpy10Count, len(valid10d))
		fmt.Printf("  • 平均 10d 報酬: %+.2f%%\n", mean10)
		fmt.Printf("  • 中位數 10d 報酬: %+.2f%%\n", med10)
	}

	// 20-day metrics
	var valid20d []TriggerRecord
	var beatSpy20Count int
	var ret20s []float64

	for _, r := range recs {
		if r.Ret20d != 0 || r.SpyRet20d != 0 {
			valid20d = append(valid20d, r)
			ret20s = append(ret20s, r.Ret20d)
			if r.BeatSpy20d {
				beatSpy20Count++
			}
		}
	}

	if len(valid20d) > 0 {
		winRate20 := float64(beatSpy20Count) / float64(len(valid20d)) * 100.0
		mean20 := mean(ret20s)
		med20 := median(ret20s)
		fmt.Printf("\n[20 日前瞻] (有效樣本: %d 筆)\n", len(valid20d))
		fmt.Printf("  • 跑贏 SPY 勝率: %.1f%% (%d/%d)\n", winRate20, beatSpy20Count, len(valid20d))
		fmt.Printf("  • 平均 20d 報酬: %+.2f%%\n", mean20)
		fmt.Printf("  • 中位數 20d 報酬: %+.2f%%\n", med20)
	}

	// Market Regime breakdown (10d)
	fmt.Printf("\n[多空情境分組 10d 表現]\n")
	printRegimeGroup("多頭情境 (SPY >= MA50)", bull10s)
	printRegimeGroup("空頭情境 (SPY < MA50)", bear10s)

	// Worst 5 cases
	sort.Slice(valid10d, func(i, j int) bool {
		return valid10d[i].Ret10d < valid10d[j].Ret10d
	})
	fmt.Printf("\n[最差 10d 案例抽查 Top 5]\n")
	for i := 0; i < len(valid10d) && i < 5; i++ {
		r := valid10d[i]
		fmt.Printf("  %d. %s (%s) @ $%.2f -> 10d: %+.2f%% (SPY: %+.2f%%) [%s]\n",
			i+1, r.Ticker, r.Date, r.EntryPrice, r.Ret10d, r.SpyRet10d, r.MarketRegime)
	}
}

func printRegimeGroup(name string, recs []TriggerRecord) {
	if len(recs) == 0 {
		fmt.Printf("  • %s: 無觸發筆數\n", name)
		return
	}
	var beatCount int
	var rets []float64
	for _, r := range recs {
		if r.BeatSpy10d {
			beatCount++
		}
		rets = append(rets, r.Ret10d)
	}
	winRate := float64(beatCount) / float64(len(recs)) * 100.0
	fmt.Printf("  • %s (%d 筆): 跑贏 SPY 勝率 %.1f%%, 平均 10d 報酬 %+.2f%%\n",
		name, len(recs), winRate, mean(rets))
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
		"Ret5d", "SpyRet5d", "BeatSpy5d",
		"Ret10d", "SpyRet10d", "BeatSpy10d",
		"Ret20d", "SpyRet20d", "BeatSpy20d",
	})

	for _, r := range recs {
		w.Write([]string{
			r.Ticker,
			r.Date,
			r.Strategy,
			fmt.Sprintf("%.2f", r.EntryPrice),
			r.MarketRegime,
			fmt.Sprintf("%.2f", r.Ret5d),
			fmt.Sprintf("%.2f", r.SpyRet5d),
			fmt.Sprintf("%t", r.BeatSpy5d),
			fmt.Sprintf("%.2f", r.Ret10d),
			fmt.Sprintf("%.2f", r.SpyRet10d),
			fmt.Sprintf("%t", r.BeatSpy10d),
			fmt.Sprintf("%.2f", r.Ret20d),
			fmt.Sprintf("%.2f", r.SpyRet20d),
			fmt.Sprintf("%t", r.BeatSpy20d),
		})
	}
	fmt.Printf("Saved CSV report to %s\n", path)
}
