package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/market"
	"argus/internal/paper"
	"argus/internal/receval"
	"argus/internal/signals"
	"argus/internal/web"
	"github.com/joho/godotenv"
)

// runBacktest runs the "backtest" subcommand: replays the recommendations
// table's full history for one market through internal/paper — the exact
// same rule engine the live paper account (internal/bot/paper.go, PR3) uses
// forward — so this answers "what would the paper account's equity curve
// have looked like so far" today, off data that already exists (see
// docs/phase-11-paper-account.md §5). Never invoked by the bot itself; same
// minimal-env, read-only-DB startup shape as runEval.
func runBacktest() {
	fs := flag.NewFlagSet("backtest", flag.ExitOnError)
	marketFlag := fs.String("market", "us", "us|tw — one market per run, the two books never mix (see docs/phase-11-paper-account.md §5.1)")
	cashFlag := fs.Float64("cash", 0, "starting cash; 0 = market default ($100,000 US / NT$1,000,000 TW)")
	riskFlag := fs.Float64("risk", 1, "risk %% of equity per trade")
	maxPosFlag := fs.Float64("maxpos", 25, "single-position notional cap, %% of equity")
	stopATRFlag := fs.Float64("stop-atr", 2, "ATR(14) multiple below entry for the initial stop")
	stopPctFlag := fs.Float64("stop-pct", 10, "fixed %% stop fallback when ATR is unavailable")
	trailingFlag := fs.Float64("trailing", 15, "fixed trailing-stop distance, %%; 0 disables")
	trailingATRFlag := fs.Float64("trailing-atr", 0, "ATR-based trailing distance multiple; <=0 = fixed %% only")
	takeProfitATRFlag := fs.Float64("take-profit-atr", 0, "ATR(14) multiple above entry for the take-profit target; <=0 = disabled")
	rangeFlag := fs.String("range", "2y", "Yahoo history range to fetch per ticker")
	csvFlag := fs.String("csv", "", "optional path to write a per-trade CSV")
	fs.Parse(os.Args[2:])

	var m market.MarketID
	switch *marketFlag {
	case "us":
		m = market.US
	case "tw":
		m = market.TW
	default:
		log.Fatalf("backtest: -market must be us or tw")
	}

	cash := *cashFlag
	if cash <= 0 {
		cash = 100000
		if m == market.TW {
			cash = 1000000
		}
	}
	cfg := paper.Config{
		InitialCash:       cash,
		RiskPct:           *riskFlag,
		MaxPositionPct:    *maxPosFlag,
		StopATRMult:       *stopATRFlag,
		StopLossPct:       *stopPctFlag,
		TrailingPct:       *trailingFlag,
		TrailingATRMult:   *trailingATRFlag,
		TakeProfitATRMult: *takeProfitATRFlag,
		Market:            m,
		FeeDiscount:       1.0, // no CLI flag — backtest defaults to the undiscounted statutory rate
	}

	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, reading env from environment")
	}
	dbPath := envOr("DB_PATH", "data/argus.db")
	lang := i18n.Parse(envOr("BOT_LANGUAGE", "zh"))

	database, err := db.OpenReadOnly(dbPath)
	if err != nil {
		log.Fatalf("backtest: open db: %v", err)
	}
	defer database.Close()

	all, err := database.GetRecommendationsSince("2000-01-01")
	if err != nil {
		log.Fatalf("backtest: read recommendations: %v", err)
	}

	var holdCount int
	var scorable []db.Recommendation
	sourceCounts := map[string]int{}
	startDate := ""
	for _, r := range all {
		if r.Market != string(m) {
			continue
		}
		if r.Action != "BUY" && r.Action != "SELL" {
			holdCount++
			continue
		}
		scorable = append(scorable, r)
		sourceCounts[receval.DisplaySource(r.Source)]++
		if startDate == "" || r.Date < startDate {
			startDate = r.Date
		}
	}
	if len(scorable) == 0 {
		fmt.Print(i18n.T(lang, i18n.KeyEvalNoData))
		return
	}

	benchTicker := "SPY"
	if m == market.TW {
		benchTicker = "0050"
	}
	fetchSet := map[string]bool{benchTicker: true}
	for _, r := range scorable {
		fetchSet[r.Ticker] = true
	}
	tickers := make([]string, 0, len(fetchSet))
	for t := range fetchSet {
		tickers = append(tickers, t)
	}

	fmt.Print(i18n.T(lang, i18n.KeyBacktestTitle, *marketFlag))
	yahoo := data.NewYahoo()
	seriesByTicker := map[string]tickerSeries{}
	var noHistoryCount int
	for i, t := range tickers {
		if i > 0 {
			time.Sleep(200 * time.Millisecond) // polite throttle, same as runEval
		}
		fmt.Print(i18n.T(lang, i18n.KeyEvalFetching, i+1, len(tickers), t))
		candles, err := yahoo.GetHistory(t, *rangeFlag)
		if err != nil || len(candles) == 0 {
			if err != nil {
				log.Printf("backtest: fetch %s: %v", t, err)
			}
			if t != benchTicker {
				noHistoryCount++
			}
			continue
		}
		seriesByTicker[t] = newTickerSeries(candles)
	}
	benchSeries, ok := seriesByTicker[benchTicker]
	if !ok || len(benchSeries.dates) == 0 {
		log.Fatalf("backtest: no benchmark (%s) history — cannot replay", benchTicker)
	}
	// Replay only from the first recommendation date onward — the benchmark
	// axis drives the whole loop and the report, so without this the CAGR/
	// benchmark/drawdown numbers compare the account against a multi-year
	// window it was never invested in (docs/phase-11-paper-strategy-tuning.md
	// §2). Per-ticker ATR warmup is unaffected: seriesByTicker keeps full
	// history, only this replay axis is windowed.
	benchSeries = benchSeries.windowFrom(startDate)

	recsByDate := map[string][]db.Recommendation{}
	for _, r := range scorable {
		if _, ok := seriesByTicker[r.Ticker]; !ok {
			continue // already counted in noHistoryCount above
		}
		recsByDate[r.Date] = append(recsByDate[r.Date], r)
	}

	acct := paper.NewAccount(cfg.InitialCash)
	cursor := map[string]int{}
	var allTrades []paper.Trade
	var curve []float64
	var dates []string
	var skippedBuyCount int

	for _, bd := range benchSeries.dates {
		closes := map[string]float64{}
		atrs := map[string]float64{}
		for t, s := range seriesByTicker {
			idx, ok := s.onOrBefore(bd, cursor[t])
			if !ok {
				continue
			}
			cursor[t] = idx
			closes[t] = s.closes[idx]
			atrs[t] = signals.ATR(s.highs[:idx+1], s.lows[:idx+1], s.closes[:idx+1], 14)
		}

		// yesterday's positions face today's stop before today's signals apply
		allTrades = append(allTrades, acct.MarkClose(bd, closes, atrs, cfg)...)

		for _, r := range recsByDate[bd] {
			price := r.Price
			if price <= 0 {
				price = closes[r.Ticker] // pre-migration-5 rows recorded no price
			}
			if price <= 0 {
				continue
			}
			_, alreadyHeld := acct.Holdings[r.Ticker]
			trade, ok := acct.ApplySignal(paper.Signal{Date: bd, Ticker: r.Ticker, Action: r.Action, Price: price}, price, atrs[r.Ticker], cfg)
			if ok {
				allTrades = append(allTrades, trade)
			} else if r.Action == "BUY" && !alreadyHeld {
				skippedBuyCount++
			}
		}

		curve = append(curve, acct.Equity(closes))
		dates = append(dates, bd)
	}

	printBacktestReport(lang, cfg, allTrades, curve, dates, benchTicker, benchSeries, holdCount, noHistoryCount, skippedBuyCount, sourceCounts)

	if *csvFlag != "" {
		if err := writeBacktestCSV(*csvFlag, allTrades); err != nil {
			log.Printf("backtest: write csv: %v", err)
		} else {
			fmt.Print(i18n.T(lang, i18n.KeyEvalCSVSaved, *csvFlag))
		}
	}
}

// tickerSeries is one ticker's daily history flattened into parallel slices
// (oldest first, same order data.Candle already comes in) for cheap ATR
// slicing and forward-fill lookups keyed by date string.
type tickerSeries struct {
	dates               []string
	highs, lows, closes []float64
}

func newTickerSeries(candles []data.Candle) tickerSeries {
	s := tickerSeries{
		dates:  make([]string, len(candles)),
		highs:  make([]float64, len(candles)),
		lows:   make([]float64, len(candles)),
		closes: make([]float64, len(candles)),
	}
	for i, c := range candles {
		s.dates[i] = c.Date.Format("2006-01-02")
		s.highs[i] = c.High
		s.lows[i] = c.Low
		s.closes[i] = c.Close
	}
	return s
}

// onOrBefore returns the index of the latest bar on or before date, forward-
// filling across gaps (holidays, cross-market mismatches) the same way
// docs/phase-11-paper-account.md §5.1 describes: "缺日沿用前一日". from is
// the previous call's result (0 on first call) — since both the benchmark
// axis and every ticker's own series are date-ascending, the cursor only
// ever needs to move forward, making one full replay O(bars) per ticker
// instead of O(days × bars).
func (s tickerSeries) onOrBefore(date string, from int) (int, bool) {
	cursor := from
	for cursor+1 < len(s.dates) && s.dates[cursor+1] <= date {
		cursor++
	}
	if s.dates[cursor] > date {
		return 0, false
	}
	return cursor, true
}

// windowFrom slices s down to bars on or after startDate, for the benchmark
// axis only (docs/phase-11-paper-strategy-tuning.md §2) — ticker series used
// for ATR warmup stay full-length via seriesByTicker.
func (s tickerSeries) windowFrom(startDate string) tickerSeries {
	if startDate == "" {
		return s
	}
	i := 0
	for i < len(s.dates) && s.dates[i] < startDate {
		i++
	}
	return tickerSeries{dates: s.dates[i:], highs: s.highs[i:], lows: s.lows[i:], closes: s.closes[i:]}
}

// maxDrawdownPct is the largest peak-to-trough decline in curve as a
// percentage of the running peak (internal/web.MaxDrawdownAbs is the same
// walk but returns a dollar amount against a cumulative P&L curve, which
// isn't the right denominator for an equity level series starting above
// zero — see internal/bot/format.go's maxDrawdownPct for the same
// peak-relative-%% shape this mirrors).
func maxDrawdownPct(curve []float64) float64 {
	if len(curve) < 2 {
		return 0
	}
	peak := curve[0]
	var maxDD float64
	for _, v := range curve[1:] {
		if v > peak {
			peak = v
			continue
		}
		if peak <= 0 {
			continue
		}
		if dd := (peak - v) / peak * 100; dd > maxDD {
			maxDD = dd
		}
	}
	return maxDD
}

func printBacktestReport(lang i18n.Lang, cfg paper.Config, trades []paper.Trade, curve []float64, dates []string,
	benchTicker string, benchSeries tickerSeries, holdCount, noHistoryCount, skippedBuyCount int, sourceCounts map[string]int) {

	finalEquity := 0.0
	if len(curve) > 0 {
		finalEquity = curve[len(curve)-1]
	}
	totalReturnPct := 0.0
	if cfg.InitialCash > 0 {
		totalReturnPct = (finalEquity - cfg.InitialCash) / cfg.InitialCash * 100
	}
	cagrPct := computeCAGR(cfg.InitialCash, finalEquity, dates)
	maxDDPct := maxDrawdownPct(curve)

	var sells []db.Transaction
	for _, t := range trades {
		if t.Side == "SELL" {
			sells = append(sells, db.Transaction{
				Ticker: t.Ticker, Side: t.Side, Shares: t.Shares, Price: t.Price,
				Fee: t.Fee, RealizedPnL: t.RealizedPnL, StopPrice: t.Stop,
			})
		}
	}
	winRate := web.WinRate(sells) * 100
	profitFactor := web.ProfitFactor(sells)
	expectancy := web.Expectancy(sells)

	fmt.Print(i18n.T(lang, i18n.KeyBacktestOverviewTitle))
	fmt.Print(i18n.T(lang, i18n.KeyBacktestOverviewLine,
		cfg.InitialCash, finalEquity, totalReturnPct, cagrPct, maxDDPct,
		len(sells), winRate, profitFactor, expectancy))

	benchReturnPct := 0.0
	if first := benchSeries.closes[0]; first > 0 {
		benchReturnPct = (benchSeries.closes[len(benchSeries.closes)-1] - first) / first * 100
	}
	fmt.Print(i18n.T(lang, i18n.KeyBacktestBenchmarkLine, benchTicker, benchReturnPct, totalReturnPct-benchReturnPct))

	fmt.Print(i18n.T(lang, i18n.KeyBacktestExitReasonTitle))
	for _, reason := range []string{"llm_sell", "stop", "target", "trailing"} {
		var count, wins int
		var sumRetPct float64
		for _, t := range trades {
			if t.Side != "SELL" || t.Reason != reason {
				continue
			}
			count++
			if t.RealizedPnL > 0 {
				wins++
			}
			if cost := t.Price*t.Shares - t.Fee - t.RealizedPnL; cost > 0 {
				sumRetPct += t.RealizedPnL / cost * 100
			}
		}
		if count == 0 {
			continue
		}
		fmt.Print(i18n.T(lang, i18n.KeyBacktestExitReasonLine, reason, count, float64(wins)/float64(count)*100, sumRetPct/float64(count)))
	}

	fmt.Print(i18n.T(lang, i18n.KeyBacktestSourceTitle))
	for _, src := range []string{"watchlist", "movers", "scan"} {
		if n := sourceCounts[src]; n > 0 {
			fmt.Print(i18n.T(lang, i18n.KeyBacktestSourceLine, src, n))
		}
	}

	fmt.Print(i18n.T(lang, i18n.KeyBacktestCountsTitle))
	fmt.Print(i18n.T(lang, i18n.KeyBacktestCountHold, holdCount))
	fmt.Print(i18n.T(lang, i18n.KeyBacktestCountNoHistory, noHistoryCount))
	fmt.Print(i18n.T(lang, i18n.KeyBacktestCountSkippedBuy, skippedBuyCount))
}

// computeCAGR annualizes the total return over dates' span; returns 0 when
// there's under a day of history or either endpoint is non-positive
// (a wipeout final equity has no meaningful annualized rate).
func computeCAGR(initialCash, finalEquity float64, dates []string) float64 {
	if len(dates) < 2 || initialCash <= 0 || finalEquity <= 0 {
		return 0
	}
	d0, err0 := time.Parse("2006-01-02", dates[0])
	d1, err1 := time.Parse("2006-01-02", dates[len(dates)-1])
	if err0 != nil || err1 != nil {
		return 0
	}
	years := d1.Sub(d0).Hours() / 24 / 365.25
	if years <= 0 {
		return 0
	}
	return (math.Pow(finalEquity/initialCash, 1/years) - 1) * 100
}

func writeBacktestCSV(path string, trades []paper.Trade) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	cw := csv.NewWriter(f)
	defer cw.Flush()

	if err := cw.Write([]string{"Date", "Ticker", "Side", "Shares", "Price", "Fee", "RealizedPnL", "Stop", "Reason"}); err != nil {
		return err
	}
	for _, t := range trades {
		row := []string{
			t.Date, t.Ticker, t.Side,
			strconv.FormatFloat(t.Shares, 'f', 2, 64),
			strconv.FormatFloat(t.Price, 'f', 2, 64),
			strconv.FormatFloat(t.Fee, 'f', 2, 64),
			strconv.FormatFloat(t.RealizedPnL, 'f', 2, 64),
			strconv.FormatFloat(t.Stop, 'f', 2, 64),
			t.Reason,
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	return cw.Error()
}
