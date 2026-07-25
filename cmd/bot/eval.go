package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/market"
	"argus/internal/receval"
	"github.com/joho/godotenv"
)

// runEval runs the "eval" subcommand — a manual, offline batch scorer over
// the recommendations table's full history against actual subsequent price
// action (see docs/offline-rec-eval.md for the full design). Never invoked
// by the bot itself; same minimal-env startup shape as runMCPServer (own
// godotenv.Load, only the env vars it actually needs).
func runEval() {
	fs := flag.NewFlagSet("eval", flag.ExitOnError)
	horizonsFlag := fs.String("horizons", "5,20,60", "comma-separated trading-day horizons to evaluate")
	rangeFlag := fs.String("range", "2y", "Yahoo history range to fetch per ticker")
	marketFlag := fs.String("market", "all", "all|us|tw")
	csvFlag := fs.String("csv", "", "optional path to write a per-recommendation CSV")
	fs.Parse(os.Args[2:])

	horizons, err := parseHorizons(*horizonsFlag)
	if err != nil {
		log.Fatalf("eval: %v", err)
	}
	if *marketFlag != "all" && *marketFlag != string(market.US) && *marketFlag != string(market.TW) {
		log.Fatalf("eval: -market must be all, us, or tw")
	}

	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, reading env from environment")
	}
	dbPath := envOr("DB_PATH", "data/argus.db")
	lang := i18n.Parse(envOr("BOT_LANGUAGE", "zh"))

	database, err := db.OpenReadOnly(dbPath)
	if err != nil {
		log.Fatalf("eval: open db: %v", err)
	}
	defer database.Close()

	all, err := database.GetRecommendationsSince("2000-01-01")
	if err != nil {
		log.Fatalf("eval: read recommendations: %v", err)
	}

	// HOLD/"" recs aren't scorable (same as /track), same reasoning as
	// bot.handleTrack only evaluating BUY/SELL rows.
	var holdCount int
	var scorable []receval.Recommendation
	haveTW := false
	for _, r := range all {
		if *marketFlag != "all" && r.Market != *marketFlag {
			continue
		}
		if r.Action != "BUY" && r.Action != "SELL" {
			holdCount++
			continue
		}
		if r.Market == string(market.TW) {
			haveTW = true
		}
		scorable = append(scorable, receval.Recommendation{
			Date: r.Date, Ticker: r.Ticker, Action: r.Action,
			Price: r.Price, Source: r.Source, Market: r.Market,
		})
	}
	if len(scorable) == 0 {
		fmt.Print(i18n.T(lang, i18n.KeyEvalNoData))
		return
	}

	benchByMarket := map[string]string{string(market.US): "SPY"}
	if haveTW {
		benchByMarket[string(market.TW)] = "0050"
	}
	fetchSet := map[string]bool{}
	for _, r := range scorable {
		fetchSet[r.Ticker] = true
	}
	for _, b := range benchByMarket {
		fetchSet[b] = true
	}
	tickers := make([]string, 0, len(fetchSet))
	for t := range fetchSet {
		tickers = append(tickers, t)
	}
	sort.Strings(tickers)

	fmt.Print(i18n.T(lang, i18n.KeyEvalTitle))
	yahoo := data.NewYahoo()
	candles := map[string][]data.Candle{}
	for i, t := range tickers {
		if i > 0 {
			time.Sleep(200 * time.Millisecond) // polite throttle, Yahoo has no key/hard quota to respect here
		}
		fmt.Print(i18n.T(lang, i18n.KeyEvalFetching, i+1, len(tickers), t))
		c, err := yahoo.GetHistory(t, *rangeFlag)
		if err != nil {
			log.Printf("eval: fetch %s: %v", t, err)
			continue // left out of candles -> Score reports "no history data"
		}
		candles[t] = c
	}

	scored := make([]receval.ScoredRec, 0, len(scorable))
	for _, r := range scorable {
		scored = append(scored, receval.Score(r, candles[r.Ticker], candles[benchByMarket[r.Market]], horizons))
	}

	printEvalReport(lang, scored, horizons, holdCount, *marketFlag)

	if *csvFlag != "" {
		if err := writeEvalCSV(*csvFlag, scored, horizons); err != nil {
			log.Printf("eval: write csv: %v", err)
		} else {
			fmt.Print(i18n.T(lang, i18n.KeyEvalCSVSaved, *csvFlag))
		}
	}
}

func parseHorizons(s string) ([]int, error) {
	parts := strings.Split(s, ",")
	horizons := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("invalid -horizons value %q: must be positive integers", p)
		}
		horizons = append(horizons, n)
	}
	sort.Ints(horizons)
	return horizons, nil
}

func printEvalReport(lang i18n.Lang, scored []receval.ScoredRec, horizons []int, holdCount int, marketFlag string) {
	fmt.Print(i18n.T(lang, i18n.KeyEvalOverviewTitle))
	printStatsGroups(lang, receval.Aggregate(scored, func(r receval.Recommendation) string { return r.Action }),
		[]string{"BUY", "SELL"}, horizons)

	bySource := receval.Aggregate(scored, func(r receval.Recommendation) string { return receval.DisplaySource(r.Source) })
	fmt.Print(i18n.T(lang, i18n.KeyEvalSourceTitle))
	printStatsGroups(lang, bySource, sortedKeys(bySource), horizons)

	if marketFlag == "all" {
		byMarket := receval.Aggregate(scored, func(r receval.Recommendation) string { return r.Market })
		for _, m := range sortedKeys(byMarket) {
			fmt.Print(i18n.T(lang, i18n.KeyEvalMarketTitle, m))
			printStatsGroups(lang, byMarket, []string{m}, horizons)
		}
	}

	maxHorizon := horizons[len(horizons)-1]
	best, worst := receval.Extremes(scored, maxHorizon, 5)
	fmt.Print(i18n.T(lang, i18n.KeyEvalExtremesTitle, maxHorizon))
	fmt.Print(i18n.T(lang, i18n.KeyEvalExtremesBest, len(best)))
	printExtremeLines(lang, best, maxHorizon)
	fmt.Print(i18n.T(lang, i18n.KeyEvalExtremesWorst, len(worst)))
	printExtremeLines(lang, worst, maxHorizon)

	counts := receval.CountOutcomes(scored)
	fmt.Print(i18n.T(lang, i18n.KeyEvalCountsTitle))
	fmt.Print(i18n.T(lang, i18n.KeyEvalCountHold, holdCount))
	fmt.Print(i18n.T(lang, i18n.KeyEvalCountUnscorable, counts.Unscorable))
	for _, reason := range sortedStringMapKeys(counts.UnscorableByReason) {
		fmt.Print(i18n.T(lang, i18n.KeyEvalCountUnscorableReason, reason, counts.UnscorableByReason[reason]))
	}
	for _, h := range horizons {
		fmt.Print(i18n.T(lang, i18n.KeyEvalCountImmature, h, counts.ImmatureByHorizon[h]))
	}
}

func printStatsGroups(lang i18n.Lang, groups map[string]map[int]receval.Stats, keys []string, horizons []int) {
	for _, k := range keys {
		for _, h := range horizons {
			s := groups[k][h]
			if s.N == 0 {
				continue
			}
			fmt.Print(i18n.T(lang, i18n.KeyEvalStatsLine, k, h, s.N, s.HitRate(), s.AvgReturn(), s.MedianReturn(), s.AvgExcess()))
		}
	}
}

func printExtremeLines(lang i18n.Lang, recs []receval.ScoredRec, horizon int) {
	for _, sr := range recs {
		w := windowAt(sr, horizon)
		fmt.Print(i18n.T(lang, i18n.KeyEvalExtremeLine, sr.Rec.Ticker, sr.Rec.Date, sr.Rec.Action, sr.EntryPrice,
			horizon, w.TickerReturnPct, w.ExcessReturnPct, sr.MFEPct, sr.MAEPct))
	}
}

func windowAt(sr receval.ScoredRec, horizon int) receval.WindowScore {
	for _, w := range sr.Windows {
		if w.Horizon == horizon {
			return w
		}
	}
	return receval.WindowScore{Horizon: horizon}
}

func sortedKeys(m map[string]map[int]receval.Stats) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedStringMapKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func writeEvalCSV(path string, scored []receval.ScoredRec, horizons []int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	cw := csv.NewWriter(f)
	defer cw.Flush()

	header := []string{"Date", "Ticker", "Action", "Source", "Market", "EntryDate", "EntryPrice", "Unscorable", "Reason"}
	for _, h := range horizons {
		header = append(header,
			fmt.Sprintf("Ret%dd", h), fmt.Sprintf("BenchRet%dd", h), fmt.Sprintf("Excess%dd", h),
			fmt.Sprintf("Matured%dd", h), fmt.Sprintf("Hit%dd", h))
	}
	header = append(header, "MFEPct", "MAEPct")
	if err := cw.Write(header); err != nil {
		return err
	}

	for _, sr := range scored {
		row := []string{
			sr.Rec.Date, sr.Rec.Ticker, sr.Rec.Action, receval.DisplaySource(sr.Rec.Source), sr.Rec.Market,
			sr.EntryDate, fmt.Sprintf("%.2f", sr.EntryPrice),
			strconv.FormatBool(sr.Unscorable), sr.Reason,
		}
		for _, h := range horizons {
			w := windowAt(sr, h)
			row = append(row,
				fmt.Sprintf("%.2f", w.TickerReturnPct),
				fmt.Sprintf("%.2f", w.BenchReturnPct),
				fmt.Sprintf("%.2f", w.ExcessReturnPct),
				strconv.FormatBool(w.Matured),
				strconv.FormatBool(w.Hit),
			)
		}
		row = append(row, fmt.Sprintf("%.2f", sr.MFEPct), fmt.Sprintf("%.2f", sr.MAEPct))
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	return cw.Error()
}
