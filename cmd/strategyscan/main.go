package main

import (
	"context"
	_ "embed"
	"encoding/csv"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"argus/internal/data"
	"argus/internal/market"
	"argus/internal/paper"
	"argus/internal/signals"
	"argus/internal/sinopac"
)

// defaultShioajiSocket is where `shioaji server start` puts its unix socket
// when SHIOAJI_ADDR isn't set — the daemon is the operator's, started in
// their own shell (token decryption needs a key only they hold).
func defaultShioajiSocket() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home + "/.shioaji/server-8080.sock"
}

//go:embed sp500_tickers.txt
var sp500TickersRaw string

//go:embed tw150_tickers.txt
var tw150TickersRaw string

//go:embed sp400_tickers.txt
var sp400TickersRaw string

// baseStrategies are the four technical screens common to both markets;
// twStrategies adds Phase 15's 網 5 (TW only — no US equivalent, see
// docs/phase-15-trust-follow.md §6). Order is shared by the hit map, the
// record loop and the summary printout.
var baseStrategies = []string{"squeeze_breakout", "box_bottom", "trend_breakout", "trend_pullback"}

const trustStrategy = "trust_follow"

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

	// §11.9 full-trade replay (strategy hits only, not baseline — see
	// simulateTrade doc comment).
	HasTrade        bool
	TradeExitRet    float64
	TradeExitReason string // "stop" | "target" | "timeout"
	TradeHoldDays   int
}

// TradeOutcome is one trigger's full-trade replay result.
type TradeOutcome struct {
	ExitRet    float64
	ExitReason string
	HoldDays   int
}

// simulatedAccountCash is a huge starting balance for simulateTrade's
// throwaway one-ticker paper.Account — big enough that SuggestShares' cash
// cap never binds ahead of its risk-based sizing, so the resulting position
// (and therefore its commission as a % of notional) converges to the real
// statutory rate instead of being distorted by TW's flat NT$20 commission
// floor at a tiny/arbitrary notional. simulateTrade only ever reads a
// Trade's Price/Fee/Reason, never Cash/Shares in isolation, so the absolute
// scale doesn't otherwise matter.
const simulatedAccountCash = 1e15

// atrPeriod matches every other ATR(14) read in this codebase (see
// stopCandidateATRMult's doc comment in internal/bot/pipeline.go) — the live
// paper account's stop/target/trailing formulas are all calibrated to 14,
// not parameterized, so this can't be a flag without silently drifting from
// what simulateTrade is trying to replicate.
const atrPeriod = 14

// simulateTrade replays entryIdx forward through the exact rule engine the
// live paper account uses (internal/paper.Account.ApplySignal/MarkClose) —
// PR3, docs/phase-23-strategy-data-uplift.md §5 — instead of this tool's own
// bespoke stop/target logic, which never matched what's actually running in
// production (the doc's whole point: those CSVs never described the real
// system). cfg carries the live exit rules (StopATRMult/StopLossPct/
// TrailingPct/TrailingATRMult/TakeProfitATRMult/Market/FeeDiscount) —
// InitialCash/RiskPct/MaxPositionPct are overridden internally to guarantee
// the simulated BUY always fills (see simulatedAccountCash), since this
// study only measures % return, never position size. slippagePct (PR2, a
// flag not a const — a fixed number here is a false-precision guess about
// market impact) is added on top of the two fills' ACTUAL commission+tax
// (Trade.Fee, computed by the real paper.FeeFor at cfg.Market's statutory
// rate net of cfg.FeeDiscount) as the round-trip friction subtracted from
// the raw price return. A trade that's still open at maxHoldDays is scored
// as a forced close at that day's close (a real sell fee is estimated for
// it via paper.FeeFor directly, since the account itself was never told to
// sell) — same "somebody has to eventually exit" convention the old model
// used for its timeout branch.
func simulateTrade(candles []data.Candle, entryIdx int, cfg paper.Config, slippagePct float64, maxHoldDays int) (TradeOutcome, bool) {
	entry := candles[entryIdx].Close
	if entry <= 0 {
		return TradeOutcome{}, false
	}
	highs, lows, closes := data.Highs(candles), data.Lows(candles), data.Closes(candles)
	atrAt := func(idx int) float64 {
		return signals.ATR(highs[:idx+1], lows[:idx+1], closes[:idx+1], atrPeriod)
	}

	sizingCfg := cfg
	sizingCfg.InitialCash = simulatedAccountCash
	sizingCfg.RiskPct = 100
	sizingCfg.MaxPositionPct = 0

	const ticker = "T"
	acct := paper.NewAccount(sizingCfg.InitialCash)
	entryDate := candles[entryIdx].Date.Format("2006-01-02")
	buyTrade, ok := acct.ApplySignal(paper.Signal{Date: entryDate, Ticker: ticker, Action: "BUY", Price: entry}, entry, atrAt(entryIdx), sizingCfg)
	if !ok {
		return TradeOutcome{}, false
	}
	notional := buyTrade.Price * buyTrade.Shares
	slippageRoundTripPct := 2 * slippagePct

	last := entryIdx
	for i := 1; i <= maxHoldDays; i++ {
		idx := entryIdx + i
		if idx >= len(candles) {
			break
		}
		last = idx
		date := candles[idx].Date.Format("2006-01-02")
		trades := acct.MarkClose(date, map[string]float64{ticker: candles[idx].Close}, map[string]float64{ticker: atrAt(idx)}, sizingCfg)
		if len(trades) > 0 {
			sell := trades[0]
			feePct := (buyTrade.Fee + sell.Fee) / notional * 100.0
			exitRet := (sell.Price-entry)/entry*100.0 - feePct - slippageRoundTripPct
			return TradeOutcome{ExitRet: exitRet, ExitReason: sell.Reason, HoldDays: i}, true
		}
	}
	if last == entryIdx {
		return TradeOutcome{}, false
	}
	sellFeeAtTimeout := paper.FeeFor(cfg.Market, "SELL", notional, cfg.FeeDiscount)
	feePct := (buyTrade.Fee + sellFeeAtTimeout) / notional * 100.0
	exitRet := (candles[last].Close-entry)/entry*100.0 - feePct - slippageRoundTripPct
	return TradeOutcome{ExitRet: exitRet, ExitReason: "timeout", HoldDays: last - entryIdx}, true
}

func main() {
	marketFlag := flag.String("market", "us", "market to scan: us|tw")
	rangeFlag := flag.String("range", "5y", "history range: 1y|2y|5y")
	// PR3 (docs/phase-23-strategy-data-uplift.md §5): these five default to
	// exactly the live paper account's own defaults (stopCandidateATRMult in
	// internal/bot/pipeline.go, STOP_LOSS_PCT/TRAILING_STOP_PCT/
	// TRAILING_STOP_ATR_MULT/PAPER_TAKE_PROFIT_ATR_MULT in .env.example, and
	// cmd/bot/backtest.go's own flags) — same names as that tool's flags on
	// purpose, so a user familiar with one recognizes the other.
	stopATRFlag := flag.Float64("stop-atr", 2, "full-trade replay: ATR(14) multiple below entry for the initial stop")
	stopPctFlag := flag.Float64("stop-pct", 10, "full-trade replay: fixed %% stop fallback when ATR is unavailable")
	trailingPctFlag := flag.Float64("trailing-pct", 15, "full-trade replay: fixed trailing-stop distance, %%; 0 disables")
	trailingATRFlag := flag.Float64("trailing-atr", 0, "full-trade replay: ATR-based trailing distance multiple; <=0 = fixed %% only")
	takeProfitATRFlag := flag.Float64("take-profit-atr", 0, "full-trade replay: ATR(14) multiple above entry for the take-profit target; <=0 = disabled")
	maxHoldDaysFlag := flag.Int("max-hold-days", 60, "full-trade replay: max holding days before a timeout exit (§11.9/PR3: 20 -> 60, matching the 數週到數月 position style)")
	// PR2 friction cost: -1 sentinel means "use the market's default" (US
	// 0.1%, TW 0.15%, live-verified as a reasonable one-side slippage guess)
	// since flag.Float64 can't default on a value (-market) not known until
	// after Parse. Commission/tax are no longer a separate flag/model here —
	// PR3's simulateTrade computes them from the real paper.FeeFor at the
	// simulated trade's actual size (see simulatedAccountCash).
	slippagePctFlag := flag.Float64("slippage-pct", -1, "full-trade replay: one-side slippage %% assumption per fill; default is market-specific (US 0.1%%, TW 0.15%%)")
	feeDiscountFlag := flag.Float64("fee-discount", 1.0, "full-trade replay: TW broker commission discount (1.0 = no discount); unused for US")
	// PR4 out-of-sample time-slice (docs/phase-23-strategy-data-uplift.md §5):
	// -range still controls how much history GetHistory fetches (relative to
	// today, Yahoo has no absolute date-range param — see internal/data/
	// yahoo.go's GetHistory doc comment), so covering 2016-2021 needs both a
	// wide enough -range (e.g. -range=10y) AND these two bounds to actually
	// restrict which triggers get evaluated/recorded. Empty = no bound
	// (today's unbounded behavior, unchanged default).
	dateFromFlag := flag.String("date-from", "", "out-of-sample: only evaluate/record triggers on or after this date (YYYY-MM-DD)")
	dateToFlag := flag.String("date-to", "", "out-of-sample: only evaluate/record triggers on or before this date (YYYY-MM-DD)")
	// The control group the §11.10 profit-factor table never had: replay the
	// SAME exit rules from every Nth (ticker, day) regardless of any screen,
	// so "網 X 盈虧比 1.9" can be read against "隨便哪天進場也是 1.9". Every
	// day would be ~600k replays of noise-on-noise; every 10th is already a
	// far larger sample than any screen's hit count.
	baselineTradeSampleFlag := flag.Int("baseline-trade-sample", 10, "full-trade replay for baseline: sample every Nth evaluated day (0 = off)")
	// The 標的切 half of PR4's out-of-sample plan, which was never done — only
	// the time-slice was. Both committed lists are TODAY's index members, so a
	// clean time-slice still can't tell an edge from survivorship bias; the
	// S&P 400 mid-caps are a disjoint universe (an index is either 500 or 400,
	// never both) drawn from the same market, which is the cheap way to ask
	// "does this screen work on stocks it wasn't tuned on".
	universeFlag := flag.String("universe", "", "US only: alternate ticker universe — sp400 (mid-cap) instead of the default S&P 500")
	// 網 3 calibration. Evaluated as extra pseudo-strategies in the SAME run
	// rather than by re-running with a different -flag: the run cost is the
	// per-ticker history fetch, and CheckTrendBreakoutExact is pure and cheap,
	// so N thresholds cost ~nothing extra. Each variant keeps its own dedup
	// state, so these are exact re-screens, not a post-hoc filter of the
	// default screen's hits (which would be wrong — a tighter cap lets a hit
	// through that the 5-day dedup had swallowed).
	tbDevSweepFlag := flag.String("tb-dev-sweep", "", "網 3 calibration: comma-separated MaxMA20DevPct values to also evaluate (e.g. 6,8,10)")
	// FinMind's free tier has an hourly request quota, and a TW run spends one
	// request per ticker on trust-net data that ONLY 網 5 uses. A run studying
	// 網 1-4 (or calibrating 網 3 with -tb-dev-sweep) therefore burns the quota
	// for nothing — and when it runs out, PR1's guard correctly refuses to
	// report a crippled 網 5, which throws away the whole run including the
	// four screens that never needed FinMind at all. This flag skips the fetch
	// outright rather than weakening that guard.
	// TW history via the Shioaji daemon instead of Yahoo. See
	// history_cache.go for why this is about survivorship bias as much as
	// about speed. Two steps on purpose: -build-history pays the fetch once,
	// every later run reads the file and touches no network at all, which is
	// what makes an exit-layer parameter grid affordable.
	buildHistoryFlag := flag.String("build-history", "", "TW only: fetch whole-market daily quotes from the Shioaji daemon into this CSV cache, then exit")
	historyFileFlag := flag.String("history-file", "", "TW only: read candles from this cache (built by -build-history) instead of Yahoo; universe becomes every listed equity in it")
	shioajiAddrFlag := flag.String("shioaji-addr", os.Getenv("SHIOAJI_ADDR"), "Shioaji daemon unix socket path or host:port (default $SHIOAJI_ADDR)")
	// Every screen gates on 5-day average volume, so a screen can only ever
	// pick a liquid name — but the BASELINE has no such gate, and on the
	// tw150/S&P universes that never mattered because every listed member was
	// liquid anyway. The whole-market cache breaks that: ~2,000 point-in-time
	// codes are mostly small and thin, so an ungated control is drawn from a
	// different population than the screens can reach, and every excess number
	// silently becomes a size/liquidity comparison rather than a screen one.
	// -1 defers to the market's own ScreenParams.MinAvgVolume5d whenever a
	// cache is in play (same sentinel convention as -slippage-pct above).
	minAvgVolumeFlag := flag.Float64("min-avg-volume", -1, "liquidity floor (shares) applied to EVERY evaluated day, baseline included; -1 = ScreenParams.MinAvgVolume5d when -history-file is set, 0 (off) otherwise")
	skipTrustFlag := flag.Bool("skip-trust", false, "TW only: don't fetch FinMind trust-net data and drop 網 5 from the study (the other four screens don't use it)")
	flag.Parse()

	m := market.US
	if *marketFlag == "tw" {
		m = market.TW
	} else if *marketFlag != "us" {
		fmt.Printf("Error: -market must be us or tw, got %q\n", *marketFlag)
		os.Exit(1)
	}

	if *buildHistoryFlag != "" {
		if m != market.TW {
			fmt.Printf("Error: -build-history is TW-only (Shioaji serves no US market data)\n")
			os.Exit(1)
		}
		addr := *shioajiAddrFlag
		if addr == "" {
			addr = defaultShioajiSocket()
		}
		from, to := *dateFromFlag, *dateToFlag
		if from == "" {
			from = time.Now().AddDate(-10, 0, 0).Format("2006-01-02")
		}
		if to == "" {
			to = time.Now().Format("2006-01-02")
		}
		fromT, err1 := time.Parse("2006-01-02", from)
		toT, err2 := time.Parse("2006-01-02", to)
		if err1 != nil || err2 != nil {
			fmt.Printf("Error: -date-from/-date-to must be YYYY-MM-DD\n")
			os.Exit(1)
		}
		fmt.Printf("Building TW history cache from Shioaji at %s: %s .. %s -> %s\n", addr, from, to, *buildHistoryFlag)
		if err := buildHistoryCache(context.Background(), sinopac.New(addr), fromT, toT, *buildHistoryFlag); err != nil {
			fmt.Printf("Error building cache: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if *slippagePctFlag < 0 {
		if m == market.TW {
			*slippagePctFlag = 0.15
		} else {
			*slippagePctFlag = 0.1
		}
	}
	exitCfg := paper.Config{
		StopATRMult:       *stopATRFlag,
		StopLossPct:       *stopPctFlag,
		TrailingPct:       *trailingPctFlag,
		TrailingATRMult:   *trailingATRFlag,
		TakeProfitATRMult: *takeProfitATRFlag,
		Market:            m,
		FeeDiscount:       *feeDiscountFlag,
	}
	fmt.Printf("Exit model (PR3, live-aligned): stop-atr=%.1f stop-pct=%.1f%% trailing-pct=%.1f%% trailing-atr=%.1f take-profit-atr=%.1f max-hold-days=%d slippage=%.2f%%/side fee-discount=%.2f\n",
		*stopATRFlag, *stopPctFlag, *trailingPctFlag, *trailingATRFlag, *takeProfitATRFlag, *maxHoldDaysFlag, *slippagePctFlag, *feeDiscountFlag)
	if *dateFromFlag != "" || *dateToFlag != "" {
		fmt.Printf("Out-of-sample window (PR4): [%s .. %s] — make sure -range is wide enough to actually reach %s (e.g. -range=10y for a 2016 start)\n",
			orDash(*dateFromFlag), orDash(*dateToFlag), orDash(*dateFromFlag))
	}

	fmt.Printf("=== Argus Strategy Historical Study Tool (cmd/strategyscan, market=%s, range=%s) ===\n", m, *rangeFlag)

	var tickers []string
	benchTicker := "SPY"
	strategies := baseStrategies
	if m == market.TW {
		tickers = parseTickers(tw150TickersRaw)
		benchTicker = "0050"
		if !*skipTrustFlag {
			strategies = append(append([]string{}, baseStrategies...), trustStrategy)
		}
		fmt.Printf("Loaded %d tw150 tickers.\n", len(tickers))
	} else if *universeFlag == "sp400" {
		tickers = parseTickers(sp400TickersRaw)
		fmt.Printf("Loaded %d S&P 400 mid-cap tickers (out-of-sample universe).\n", len(tickers))
	} else if *universeFlag != "" {
		fmt.Printf("Error: -universe must be empty or sp400, got %q\n", *universeFlag)
		os.Exit(1)
	} else {
		tickers = parseTickers(sp500TickersRaw)
		fmt.Printf("Loaded %d S&P 500 tickers.\n", len(tickers))
	}
	if m == market.TW && *universeFlag != "" {
		fmt.Printf("Error: -universe is US-only (no committed TW mid-cap list)\n")
		os.Exit(1)
	}
	screenParams := signals.DefaultScreenParams(m)

	if *minAvgVolumeFlag < 0 {
		if *historyFileFlag != "" {
			*minAvgVolumeFlag = screenParams.MinAvgVolume5d
		} else {
			*minAvgVolumeFlag = 0
		}
	}
	if *minAvgVolumeFlag > 0 {
		fmt.Printf("Liquidity floor: %.0f shares (5-day average), applied to baseline as well as screens\n", *minAvgVolumeFlag)
	}

	devVariants, err := parseDevSweep(*tbDevSweepFlag, screenParams)
	if err != nil {
		fmt.Printf("Error: -tb-dev-sweep: %v\n", err)
		os.Exit(1)
	}
	for _, v := range devVariants {
		strategies = append(strategies, v.name)
		fmt.Printf("網 3 calibration variant: %s (MaxMA20DevPct=%g, default %g)\n", v.name, v.devPct, screenParams.MaxMA20DevPct)
	}

	// getHistory is the single read path for candles, so the Yahoo and cache
	// sources differ in exactly one place rather than at every call site.
	yahoo := data.NewYahoo()
	var cache map[string][]data.Candle
	getHistory := func(ticker string) ([]data.Candle, error) {
		time.Sleep(200 * time.Millisecond) // rate limit
		return yahoo.GetHistory(ticker, *rangeFlag)
	}
	if *historyFileFlag != "" {
		if m != market.TW {
			fmt.Printf("Error: -history-file is TW-only\n")
			os.Exit(1)
		}
		if !*skipTrustFlag {
			fmt.Printf("Error: -history-file needs -skip-trust — the cache universe is the whole market (~2,000 tickers) and 網 5 would need one FinMind request per ticker, far past the free tier's quota.\n")
			os.Exit(1)
		}
		var err error
		if cache, err = loadHistoryCache(*historyFileFlag, 60, benchTicker); err != nil {
			fmt.Printf("Error loading cache: %v\n", err)
			os.Exit(1)
		}
		getHistory = func(ticker string) ([]data.Candle, error) {
			c, ok := cache[ticker]
			if !ok {
				return nil, fmt.Errorf("%s not in cache", ticker)
			}
			return c, nil
		}
		tickers = tickers[:0]
		for code := range cache {
			if code != benchTicker {
				tickers = append(tickers, code)
			}
		}
		sort.Strings(tickers)
		fmt.Printf("Universe replaced by cache: %d point-in-time listed equities (was %d from tw150_tickers.txt)\n", len(tickers), len(parseTickers(tw150TickersRaw)))
	}
	// docs/phase-15-trust-follow.md §4.1: FinMind's free tier serves every
	// dataset used here unauthenticated (live-verified), so this backtest
	// tool doesn't gate construction on FINMIND_TOKEN the way the bot does.
	finmind := data.NewFinMind(os.Getenv("FINMIND_TOKEN"))

	fmt.Printf("Fetching %s history for market regime and benchmark...\n", benchTicker)
	benchCandles, err := getHistory(benchTicker)
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

	// Phase 15 §4.5: trust-net fetch accounting, counted and printed
	// separately rather than folded silently into fetchFailed — a failure
	// here just means that ticker's trust_follow gets skipped for the
	// day, not that the ticker itself is dropped from the study.
	trustFetchFailed := 0
	var trustFailedTickers []string

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

		candles, err := getHistory(ticker)
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

		// Phase 15 §4.5: one extra FinMind request per TW ticker, whole
		// history in one shot rather than day-by-day — trustAligned/
		// foreignAligned stay nil on failure, which just drops trust_follow
		// for this ticker's records below (baseline/other strategies are
		// unaffected). Both series ride the same request/rows (see
		// data.TrustNetDay).
		var trustAligned, foreignAligned []int64
		if m == market.TW && !*skipTrustFlag {
			time.Sleep(200 * time.Millisecond) // rate limit
			rows, err := finmind.GetTrustNetSeries(ticker, len(candles))
			if err != nil {
				trustFetchFailed++
				trustFailedTickers = append(trustFailedTickers, ticker)
			} else {
				trustAligned = signals.AlignTrustNet(candles, rows)
				foreignAligned = signals.AlignForeignNet(candles, rows)
			}
		}

		lastHit := make(map[string]int) // strategy -> last recorded index t

		// Evaluate historical triggers for t from index 59 to len(candles)-1
		for t := 59; t < len(candles); t++ {
			sub := candles[:t+1]
			evalDateStr := candles[t].Date.Format("2006-01-02")
			// PR4: out-of-sample time-slice — skip triggers outside
			// [-date-from, -date-to] entirely (not just at CSV-write time),
			// so baseline/summary stats are computed over the same restricted
			// window the strategy hits are.
			if *dateFromFlag != "" && evalDateStr < *dateFromFlag {
				continue
			}
			if *dateToFlag != "" && evalDateStr > *dateToFlag {
				continue
			}
			entryPrice := candles[t].Close
			if entryPrice <= 0 {
				continue
			}
			// Same window the screens use (the five bars BEFORE the trigger
			// bar, see CheckTrendBreakoutExact) so the control population is
			// exactly the one a screen could have drawn from.
			if *minAvgVolumeFlag > 0 && avgVolume5(candles, t) < *minAvgVolumeFlag {
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
			if *baselineTradeSampleFlag > 0 && t%*baselineTradeSampleFlag == 0 {
				if outcome, ok := simulateTrade(candles, t, exitCfg, *slippagePctFlag, *maxHoldDaysFlag); ok {
					baseline.HasTrade = true
					baseline.TradeExitRet = outcome.ExitRet
					baseline.TradeExitReason = outcome.ExitReason
					baseline.TradeHoldDays = outcome.HoldDays
				}
			}
			records = append(records, baseline)

			hits := map[string]bool{
				"squeeze_breakout": signals.CheckSqueezeBreakoutExact(sub, screenParams),
				"box_bottom":       signals.CheckBoxBottomReboundExact(sub, screenParams),
				"trend_breakout":   signals.CheckTrendBreakoutExact(sub, screenParams),
				"trend_pullback":   signals.CheckTrendPullbackExact(sub, screenParams),
			}
			if trustAligned != nil {
				hits[trustStrategy] = signals.CheckTrustFollowExact(sub, trustAligned[:t+1], foreignAligned[:t+1], screenParams)
			}
			for _, v := range devVariants {
				hits[v.name] = signals.CheckTrendBreakoutExact(sub, v.params)
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
				if outcome, ok := simulateTrade(candles, t, exitCfg, *slippagePctFlag, *maxHoldDaysFlag); ok {
					rec.HasTrade = true
					rec.TradeExitRet = outcome.ExitRet
					rec.TradeExitReason = outcome.ExitReason
					rec.TradeHoldDays = outcome.HoldDays
				}
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
	if m == market.TW && *skipTrustFlag {
		fmt.Printf("Trust-net (FinMind): skipped (-skip-trust); 網 5 is not part of this run.\n")
	}
	if m == market.TW && !*skipTrustFlag {
		fmt.Printf("Trust-net (FinMind): %d fetch errors out of %d fetched tickers\n", trustFetchFailed, fetched)
		if len(trustFailedTickers) > 0 {
			shown := trustFailedTickers
			if len(shown) > 10 {
				shown = shown[:10]
			}
			fmt.Printf("Trust-net fetch failures (first %d of %d): %s\n", len(shown), len(trustFailedTickers), strings.Join(shown, ", "))
		}
		// PR1 (docs/phase-23-strategy-data-uplift.md §5): a silent drop here is
		// how 網 5 went unbacktested for an entire phase without anyone
		// noticing (§2.3) — FINMIND_TOKEN missing/invalid must fail the run,
		// not just print a line nobody reads.
		if fetched > 0 && float64(trustFetchFailed)/float64(fetched) > 0.05 {
			fmt.Printf("FATAL: trust-net fetch error rate %.1f%% exceeds 5%% — 網 5 (trust_follow) would be studied on a crippled sample. Check FINMIND_TOKEN and re-run.\n",
				float64(trustFetchFailed)/float64(fetched)*100.0)
			os.Exit(1)
		}
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
	baselineRecs := filterByStrategy(records, baselineStrategy)
	printSummary(benchTicker, "Baseline（全樣本，未篩選）", baselineRecs, nil)
	ctrl := computeControl(baselineRecs)
	printSummary(benchTicker, "Squeeze Breakout (網 1)", filterByStrategy(records, "squeeze_breakout"), &ctrl)
	printSummary(benchTicker, "Box Bottom Rebound (網 2)", filterByStrategy(records, "box_bottom"), &ctrl)
	printSummary(benchTicker, "Trend Breakout (網 3)", filterByStrategy(records, "trend_breakout"), &ctrl)
	printSummary(benchTicker, "Trend Pullback (網 4)", filterByStrategy(records, "trend_pullback"), &ctrl)
	if m == market.TW && !*skipTrustFlag {
		printSummary(benchTicker, "Trust Follow (網 5，主力跟單 v2)", filterByStrategy(records, trustStrategy), &ctrl)
	}
	for _, v := range devVariants {
		printSummary(benchTicker, fmt.Sprintf("Trend Breakout 校準：MaxMA20DevPct=%g (網 3)", v.devPct), filterByStrategy(records, v.name), &ctrl)
	}

	// §11.2 point 1: baseline aggregate stats as their own CSV so anyone can
	// independently re-derive every excess number above.
	writeBaselineSummaryCSV(fmt.Sprintf("strategyscan_baseline_%s.csv", m), baselineRecs)
}

// devVariant is one 網 3 re-screen at a different MaxMA20DevPct, carried as
// its own pseudo-strategy name so it flows through the same dedup / record /
// summary path as a real screen.
type devVariant struct {
	name   string
	devPct float64
	params signals.ScreenParams
}

func parseDevSweep(raw string, base signals.ScreenParams) ([]devVariant, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var out []devVariant
	for _, field := range strings.Split(raw, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		v, err := strconv.ParseFloat(field, 64)
		if err != nil {
			return nil, fmt.Errorf("%q is not a number", field)
		}
		if v <= 0 {
			return nil, fmt.Errorf("%g must be > 0", v)
		}
		p := base
		p.MaxMA20DevPct = v
		out = append(out, devVariant{name: fmt.Sprintf("trend_breakout_dev%g", v), devPct: v, params: p})
	}
	return out, nil
}

// avgVolume5 is the mean volume of the five bars preceding index t.
func avgVolume5(candles []data.Candle, t int) float64 {
	if t < 5 {
		return 0
	}
	var sum int64
	for _, c := range candles[t-5 : t] {
		sum += c.Volume
	}
	return float64(sum) / 5.0
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
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

// control is the random-entry baseline every strategy line is read against
// (§10.3 point 4) — the only number that actually says whether a screen beats
// picking any (ticker, day) at random. It carries the bull/bear split too:
// without it "網 X 在空頭比較差" can't be told apart from "空頭本來就比較差",
// which is the whole question behind gating signals on market regime.
type control struct {
	d5, d10, d20         windowStats
	trade                tradeStats
	bull10, bear10       windowStats // 10d forward return, regime-split
	bullTrade, bearTrade tradeStats
}

func computeControl(recs []TriggerRecord) control {
	var c control
	c.d5, c.d10, c.d20 = summaryStats5d10d20d(recs)
	c.trade = computeTradeStats(recs)
	bull, bear := splitRegime(recs)
	_, c.bull10, _ = summaryStats5d10d20d(bull)
	_, c.bear10, _ = summaryStats5d10d20d(bear)
	c.bullTrade, c.bearTrade = computeTradeStats(bull), computeTradeStats(bear)
	return c
}

// splitRegime partitions by the benchmark's regime at entry (MarketRegime,
// set once per evaluated day in the scan loop), not by anything about the
// trade itself.
func splitRegime(recs []TriggerRecord) (bull, bear []TriggerRecord) {
	for _, r := range recs {
		if r.MarketRegime == "bull" {
			bull = append(bull, r)
		} else {
			bear = append(bear, r)
		}
	}
	return bull, bear
}

// summaryStats5d10d20d returns win rate / mean / median for the 5d/10d/20d
// forward-return windows (§11.2 point 3 — 5d is the window most relevant to
// short-horizon screens like 網 4 but wasn't being surfaced anywhere).
func summaryStats5d10d20d(recs []TriggerRecord) (d5, d10, d20 windowStats) {
	var ret5s, ret10s, ret20s []float64
	var beat5, beat10, beat20 int
	for _, r := range recs {
		if r.Has5d {
			ret5s = append(ret5s, r.Ret5d)
			if r.BeatBench5d {
				beat5++
			}
		}
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
	if n := len(ret5s); n > 0 {
		d5 = windowStats{n, float64(beat5) / float64(n) * 100.0, mean(ret5s), median(ret5s)}
	}
	if n := len(ret10s); n > 0 {
		d10 = windowStats{n, float64(beat10) / float64(n) * 100.0, mean(ret10s), median(ret10s)}
	}
	if n := len(ret20s); n > 0 {
		d20 = windowStats{n, float64(beat20) / float64(n) * 100.0, mean(ret20s), median(ret20s)}
	}
	return d5, d10, d20
}

func printSummary(benchTicker, title string, recs []TriggerRecord, ctrl *control) {
	isBaseline := ctrl == nil
	fmt.Printf("\n=======================================================\n")
	fmt.Printf(" 策略統計：%s\n", title)
	fmt.Printf("=======================================================\n")

	totalHits := len(recs)
	fmt.Printf("總觸發次數: %d 次\n", totalHits)
	if totalHits == 0 {
		return
	}

	d5, d10, d20 := summaryStats5d10d20d(recs)

	if d5.n > 0 {
		fmt.Printf("\n[5 日前瞻] (有效樣本: %d 筆)\n", d5.n)
		fmt.Printf("  • 跑贏 %s 勝率: %.1f%%\n", benchTicker, d5.winRate)
		fmt.Printf("  • 平均 5d 報酬: %+.2f%%\n", d5.meanRet)
		fmt.Printf("  • 中位數 5d 報酬: %+.2f%%\n", d5.medRet)
		if ctrl != nil && ctrl.d5.n > 0 {
			fmt.Printf("  • 超額 vs baseline: 勝率 %+.1f 個百分點, 平均報酬 %+.2f%%\n",
				d5.winRate-ctrl.d5.winRate, d5.meanRet-ctrl.d5.meanRet)
		}
	}

	if d10.n > 0 {
		fmt.Printf("\n[10 日前瞻] (有效樣本: %d 筆)\n", d10.n)
		fmt.Printf("  • 跑贏 %s 勝率: %.1f%%\n", benchTicker, d10.winRate)
		fmt.Printf("  • 平均 10d 報酬: %+.2f%%\n", d10.meanRet)
		fmt.Printf("  • 中位數 10d 報酬: %+.2f%%\n", d10.medRet)
		if ctrl != nil && ctrl.d10.n > 0 {
			fmt.Printf("  • 超額 vs baseline: 勝率 %+.1f 個百分點, 平均報酬 %+.2f%%\n",
				d10.winRate-ctrl.d10.winRate, d10.meanRet-ctrl.d10.meanRet)
		}
	}

	if d20.n > 0 {
		fmt.Printf("\n[20 日前瞻] (有效樣本: %d 筆)\n", d20.n)
		fmt.Printf("  • 跑贏 %s 勝率: %.1f%%\n", benchTicker, d20.winRate)
		fmt.Printf("  • 平均 20d 報酬: %+.2f%%\n", d20.meanRet)
		fmt.Printf("  • 中位數 20d 報酬: %+.2f%%\n", d20.medRet)
		if ctrl != nil && ctrl.d20.n > 0 {
			fmt.Printf("  • 超額 vs baseline: 勝率 %+.1f 個百分點, 平均報酬 %+.2f%%\n",
				d20.winRate-ctrl.d20.winRate, d20.meanRet-ctrl.d20.meanRet)
		}
	}

	// §11.9: full-trade replay stats. Baseline gets them too when
	// -baseline-trade-sample is on (printTradeStats no-ops at n=0), so every
	// strategy's numbers have a same-exit-rules control to be read against.
	var baseTrade *tradeStats
	if ctrl != nil {
		baseTrade = &ctrl.trade
	}
	printTradeStats(recs, baseTrade)

	// Market regime breakdown. Each side gets the SAME-regime slice of the
	// control, not the all-days one — a screen only deserves to be gated on
	// regime if it underperforms a random entry made on those same bad days.
	bull, bear := splitRegime(recs)
	fmt.Printf("\n[多空情境分組]\n")
	var bullCtrl10, bearCtrl10 *windowStats
	var bullCtrlTrade, bearCtrlTrade *tradeStats
	if ctrl != nil {
		bullCtrl10, bearCtrl10 = &ctrl.bull10, &ctrl.bear10
		bullCtrlTrade, bearCtrlTrade = &ctrl.bullTrade, &ctrl.bearTrade
	}
	printRegimeGroup(benchTicker, fmt.Sprintf("多頭情境 (%s >= MA50)", benchTicker), bull, bullCtrl10, bullCtrlTrade)
	printRegimeGroup(benchTicker, fmt.Sprintf("空頭情境 (%s < MA50)", benchTicker), bear, bearCtrl10, bearCtrlTrade)

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

// tradeStats is one strategy's §11.9 full-trade replay aggregated. Kept as a
// value (not printed straight from the loop) so the baseline's own replay can
// be passed back in as the control every strategy line is read against.
type tradeStats struct {
	n                               int
	stop, trailing, target, timeout int
	mean, median, winRate           float64
	profitFactor                    float64 // sum(wins) / |sum(losses)|, 0 = undefined
	avgHold                         float64
}

func computeTradeStats(recs []TriggerRecord) tradeStats {
	var ts tradeStats
	var rets []float64
	var sumWin, sumLoss, sumHold float64
	var wins int
	for _, r := range recs {
		if !r.HasTrade {
			continue
		}
		rets = append(rets, r.TradeExitRet)
		sumHold += float64(r.TradeHoldDays)
		if r.TradeExitRet > 0 {
			wins++
			sumWin += r.TradeExitRet
		} else {
			sumLoss += r.TradeExitRet
		}
		switch r.TradeExitReason {
		case "stop":
			ts.stop++
		case "trailing":
			ts.trailing++
		case "target":
			ts.target++
		case "timeout":
			ts.timeout++
		}
	}
	ts.n = len(rets)
	if ts.n == 0 {
		return ts
	}
	ts.mean, ts.median = mean(rets), median(rets)
	ts.winRate = float64(wins) / float64(ts.n) * 100.0
	ts.avgHold = sumHold / float64(ts.n)
	if sumLoss < 0 {
		// Profit factor over ALL trades, not 停利平均/|停損平均| — PR3's
		// live-aligned exit model has no take-profit by default, so the old
		// target-vs-stop ratio was structurally N/A (and, before that,
		// degenerated to target%/stop% by construction — §11.10).
		ts.profitFactor = sumWin / math.Abs(sumLoss)
	}
	return ts
}

func printTradeStats(recs []TriggerRecord, base *tradeStats) {
	ts := computeTradeStats(recs)
	if ts.n == 0 {
		return
	}
	pct := func(c int) float64 { return float64(c) / float64(ts.n) * 100.0 }
	fmt.Printf("\n[完整交易統計（live 出場規則，§11.9）] (有效樣本: %d 筆)\n", ts.n)
	fmt.Printf("  • 出場分布: 停損 %d (%.1f%%) / 移動停利 %d (%.1f%%) / 停利 %d (%.1f%%) / 超時 %d (%.1f%%)\n",
		ts.stop, pct(ts.stop), ts.trailing, pct(ts.trailing), ts.target, pct(ts.target), ts.timeout, pct(ts.timeout))
	fmt.Printf("  • 平均報酬 %+.2f%% / 中位數 %+.2f%% / 賺錢比例 %.1f%% / 平均持有 %.1f 天\n",
		ts.mean, ts.median, ts.winRate, ts.avgHold)
	if ts.profitFactor > 0 {
		fmt.Printf("  • 盈虧比 (總獲利 / |總虧損|): %.2f\n", ts.profitFactor)
	} else {
		fmt.Printf("  • 盈虧比: N/A（無虧損樣本）\n")
	}
	if base != nil && base.n > 0 {
		fmt.Printf("  • 超額 vs baseline 同出場規則: 平均報酬 %+.2f%% / 賺錢比例 %+.1f 個百分點 / 盈虧比 %+.2f\n",
			ts.mean-base.mean, ts.winRate-base.winRate, ts.profitFactor-base.profitFactor)
	}
}

// printRegimeGroup prints one regime's 10d forward return and full-trade
// stats. ctrl10/ctrlTrade are the control's SAME-regime numbers (nil for the
// control's own printout), so the excess columns answer "does this screen add
// anything on these days", not "are these days bad" — the latter is true for
// every screen in a bear market and says nothing about the screen.
func printRegimeGroup(benchTicker, name string, recs []TriggerRecord, ctrl10 *windowStats, ctrlTrade *tradeStats) {
	if len(recs) == 0 {
		fmt.Printf("  • %s: 無觸發筆數\n", name)
		return
	}
	_, d10, _ := summaryStats5d10d20d(recs)
	suffix := ""
	if d10.n < 20 {
		suffix = "（樣本數過小，不下結論）"
	}
	fmt.Printf("  • %s (%d 筆): 跑贏 %s 勝率 %.1f%%, 平均 10d 報酬 %+.2f%%%s\n",
		name, d10.n, benchTicker, d10.winRate, d10.meanRet, suffix)
	if ctrl10 != nil && ctrl10.n > 0 {
		fmt.Printf("      超額 vs 同情境 baseline: 勝率 %+.1f 個百分點, 平均 10d %+.2f%%\n",
			d10.winRate-ctrl10.winRate, d10.meanRet-ctrl10.meanRet)
	}
	ts := computeTradeStats(recs)
	if ts.n == 0 {
		return
	}
	fmt.Printf("      完整交易 (%d 筆): 平均 %+.2f%% / 賺錢比例 %.1f%% / 盈虧比 %.2f\n",
		ts.n, ts.mean, ts.winRate, ts.profitFactor)
	if ctrlTrade != nil && ctrlTrade.n > 0 {
		fmt.Printf("      超額 vs 同情境 baseline: 平均 %+.2f%% / 賺錢比例 %+.1f 個百分點 / 盈虧比 %+.2f\n",
			ts.mean-ctrlTrade.mean, ts.winRate-ctrlTrade.winRate, ts.profitFactor-ctrlTrade.profitFactor)
	}
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
		"Ret5d", "BenchRet5d", "BeatBench5d", "Has5d",
		"Ret10d", "BenchRet10d", "BeatBench10d", "Has10d",
		"Ret20d", "BenchRet20d", "BeatBench20d", "Has20d",
		"TradeExitRet", "TradeExitReason", "TradeHoldDays", "HasTrade",
	})

	for _, r := range recs {
		if r.Strategy == baselineStrategy {
			continue // §10.3: baseline is a few hundred thousand rows; CSV is for manual spot-checks — see writeBaselineSummaryCSV instead.
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
			fmt.Sprintf("%t", r.Has5d),
			fmt.Sprintf("%.2f", r.Ret10d),
			fmt.Sprintf("%.2f", r.BenchRet10d),
			fmt.Sprintf("%t", r.BeatBench10d),
			fmt.Sprintf("%t", r.Has10d),
			fmt.Sprintf("%.2f", r.Ret20d),
			fmt.Sprintf("%.2f", r.BenchRet20d),
			fmt.Sprintf("%t", r.BeatBench20d),
			fmt.Sprintf("%t", r.Has20d),
			fmt.Sprintf("%.2f", r.TradeExitRet),
			r.TradeExitReason,
			fmt.Sprintf("%d", r.TradeHoldDays),
			fmt.Sprintf("%t", r.HasTrade),
		})
	}
	fmt.Printf("Saved CSV report to %s\n", path)
}

// writeBaselineSummaryCSV writes the baseline's aggregate 5d/10d/20d stats
// (§11.2 point 1) — not the raw few-hundred-thousand-row population, which
// would defeat the "CSV is for manual spot-checks" purpose (§10.3 point 3).
// This lets anyone independently re-derive every excess number in §8 without
// re-fetching a baseline themselves.
func writeBaselineSummaryCSV(path string, baselineRecs []TriggerRecord) {
	f, err := os.Create(path)
	if err != nil {
		fmt.Printf("Error creating baseline summary CSV: %v\n", err)
		return
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	w.Write([]string{"Window", "N", "WinRatePct", "MeanRetPct", "MedianRetPct"})
	d5, d10, d20 := summaryStats5d10d20d(baselineRecs)
	for _, row := range []struct {
		window string
		s      windowStats
	}{
		{"5d", d5}, {"10d", d10}, {"20d", d20},
	} {
		w.Write([]string{
			row.window,
			fmt.Sprintf("%d", row.s.n),
			fmt.Sprintf("%.2f", row.s.winRate),
			fmt.Sprintf("%.2f", row.s.meanRet),
			fmt.Sprintf("%.2f", row.s.medRet),
		})
	}
	fmt.Printf("Saved baseline summary CSV to %s\n", path)
}
