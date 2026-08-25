package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/llm"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/signals"
)

// cst is the scan's clock, matching internal/scheduler's fixed zone — the
// universe scan's trading-day gate and its scan_hits date both have to agree
// with the cron times that trigger it.
var cst = time.FixedZone("CST", 8*3600)

// ScanStore is the persistence ScanService needs: the signal_states dedup
// boundary CheckStatefulSignals uses (the same shape RiskStore's
// GetSignalState/SetSignalState pair covers, kept separate here since
// ScanService has no need for RiskStore's other methods), plus the
// universe/watchlist reads and scan_hits write RunUniverseScan does.
type ScanStore interface {
	GetSignalState(ticker, family string) (string, error)
	SetSignalState(ticker, family, state string) error
	GetUniverse() ([]db.UniverseEntry, error)
	GetWatchlistByMarket(m market.MarketID) ([]string, error)
	SaveScanHit(ticker, date, reason string) error
}

// RestrictedProvider is the TWSE/TPEx disposition (處置) and attention (注意)
// lists — the narrow slice of *sinopac.Client RestrictedTickers needs. nil
// when SHIOAJI_ADDR isn't configured, same nil-degrade convention as every
// other optional provider.
type RestrictedProvider interface {
	RegulatoryPunish(ctx context.Context) (map[string]string, error)
	RegulatoryNotice(ctx context.Context) (map[string]string, error)
}

// ScanService is jobs.go's stateful RSI/MACD/strategy signal detection,
// shared by the daily watchlist report and the universe scan (both call
// CheckStatefulSignals per ticker). detector holds the already-localized
// signals.Detector (its Signal.Message text is built with i18n internally,
// see internal/signals/strategies.go) — bear-regime warning decoration is
// left to the caller (formatting stays in the adapter, see
// docs/architecture/backend.md); every strategy hit's Type carries a
// "strategy_" prefix so callers can find them without re-deriving which
// checks are strategies.
type ScanService struct {
	store    ScanStore
	detector *signals.Detector
	// trustNet is Phase 15 網 5【主力跟單】's optional TW-only data source;
	// nil when FINMIND_TOKEN isn't set, same nil-degrade convention as every
	// other optional provider.
	trustNet data.TrustNetProvider
	// fundamentals backs the revenue-growth short-circuit gate (see
	// revenueGrowthOK) — nil when the caller has no fundamentals provider
	// configured, in which case the gate always fails closed.
	fundamentals func(ticker string) (*data.Fundamentals, error)

	// The rest are RunUniverseScan's own dependencies (Phase 24 Stage 3 Step
	// 3.2). restricted/lang/now are optional in the sense that a zero value
	// degrades rather than panics; history/quotes are not — RunUniverseScan
	// nil-checks both up front and errors out instead — but all five can
	// stay unset for a CheckStatefulSignals-only caller (and its tests),
	// which never reaches that check.
	history    RiskHistoryReader
	quotes     QuoteReader
	restricted RestrictedProvider
	lang       i18n.Lang
	now        func() time.Time
}

// ScanConfig is NewScanService's argument. A struct rather than positional
// parameters because Step 3.2 took this from four dependencies to nine, and
// most call sites (tests, the CheckStatefulSignals-only path) legitimately
// leave most of them nil.
type ScanConfig struct {
	Store        ScanStore
	Detector     *signals.Detector
	TrustNet     data.TrustNetProvider
	Fundamentals func(ticker string) (*data.Fundamentals, error)
	History      RiskHistoryReader
	Quotes       QuoteReader
	Restricted   RestrictedProvider
	Lang         i18n.Lang
	// Now defaults to time.Now — a field only so a test can pin the
	// trading-day gate and the scan_hits date, same seam bot.Bot's own now
	// field is.
	Now func() time.Time
}

func NewScanService(cfg ScanConfig) *ScanService {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &ScanService{
		store:        cfg.Store,
		detector:     cfg.Detector,
		trustNet:     cfg.TrustNet,
		fundamentals: cfg.Fundamentals,
		history:      cfg.History,
		quotes:       cfg.Quotes,
		restricted:   cfg.Restricted,
		lang:         cfg.Lang,
		now:          now,
	}
}

// CheckStatefulSignals runs the RSI/MACD checks and all five strategy
// screens against candles, diffing each against its own persisted
// signal_states row so a signal only fires once per occurrence. Moved
// verbatim from bot.checkStatefulSignals (Phase 24 Stage 1 Scan & Strategy
// Service extraction) except for the bear-regime message decoration, which
// stayed in the adapter.
func (s *ScanService) CheckStatefulSignals(ticker string, candles []data.Candle) []signals.Signal {
	var out []signals.Signal
	closes := data.Closes(candles)

	prevRSI, err := s.store.GetSignalState(ticker, signals.FamilyRSI)
	if err != nil {
		logger.Errorf("signal state %s/%s: %v", ticker, signals.FamilyRSI, err)
	}
	sig, newRSI := s.detector.CheckRSIState(ticker, closes, prevRSI)
	if sig != nil {
		out = append(out, *sig)
	}
	if newRSI != prevRSI {
		if err := s.store.SetSignalState(ticker, signals.FamilyRSI, newRSI); err != nil {
			logger.Errorf("signal state %s/%s: %v", ticker, signals.FamilyRSI, err)
		}
	}

	prevMACD, err := s.store.GetSignalState(ticker, signals.FamilyMACD)
	if err != nil {
		logger.Errorf("signal state %s/%s: %v", ticker, signals.FamilyMACD, err)
	}
	sig, newMACD := s.detector.CheckMACDCross(ticker, closes, prevMACD)
	if sig != nil {
		out = append(out, *sig)
	}
	if newMACD != prevMACD {
		if err := s.store.SetSignalState(ticker, signals.FamilyMACD, newMACD); err != nil {
			logger.Errorf("signal state %s/%s: %v", ticker, signals.FamilyMACD, err)
		}
	}

	// Strategy 1: Squeeze Breakout
	prevSqueeze, err := s.store.GetSignalState(ticker, signals.FamilyStrategySqueeze)
	if err != nil {
		logger.Errorf("signal state %s/%s: %v", ticker, signals.FamilyStrategySqueeze, err)
	}
	sig, newSqueeze := s.detector.CheckSqueezeBreakout(ticker, candles, prevSqueeze)
	if sig != nil {
		out = append(out, *sig)
	}
	if newSqueeze != prevSqueeze {
		if err := s.store.SetSignalState(ticker, signals.FamilyStrategySqueeze, newSqueeze); err != nil {
			logger.Errorf("signal state %s/%s: %v", ticker, signals.FamilyStrategySqueeze, err)
		}
	}

	// Strategy 2: Box Bottom Rebound
	prevBox, err := s.store.GetSignalState(ticker, signals.FamilyStrategyBox)
	if err != nil {
		logger.Errorf("signal state %s/%s: %v", ticker, signals.FamilyStrategyBox, err)
	}
	sig, newBox := s.detector.CheckBoxBottom(ticker, candles, prevBox)
	if sig != nil {
		out = append(out, *sig)
	}
	if newBox != prevBox {
		if err := s.store.SetSignalState(ticker, signals.FamilyStrategyBox, newBox); err != nil {
			logger.Errorf("signal state %s/%s: %v", ticker, signals.FamilyStrategyBox, err)
		}
	}

	// Strategy 3: Trend Breakout (Phase 14 網 3) — revenue-growth gate is
	// short-circuited: only evaluated when the (zero-request) technical AND
	// already passed, so the FinMind/Finnhub hit stays ~0-5/day (see
	// docs/phase-14-strategy-screens-2.md §4.2c).
	prevBreakout, err := s.store.GetSignalState(ticker, signals.FamilyStrategyBreakout)
	if err != nil {
		logger.Errorf("signal state %s/%s: %v", ticker, signals.FamilyStrategyBreakout, err)
	}
	sig, newBreakout := s.detector.CheckTrendBreakout(ticker, candles, prevBreakout)
	if sig != nil {
		p := signals.DefaultScreenParams(market.Of(ticker))
		if p.RequireRevenueGrowth && !s.revenueGrowthOK(ticker, p.MinRevenueGrowthPct) {
			sig = nil
		}
	}
	if sig != nil {
		out = append(out, *sig)
	}
	if newBreakout != prevBreakout {
		if err := s.store.SetSignalState(ticker, signals.FamilyStrategyBreakout, newBreakout); err != nil {
			logger.Errorf("signal state %s/%s: %v", ticker, signals.FamilyStrategyBreakout, err)
		}
	}

	// Strategy 4: Trend Pullback (Phase 14 網 4)
	prevPullback, err := s.store.GetSignalState(ticker, signals.FamilyStrategyPullback)
	if err != nil {
		logger.Errorf("signal state %s/%s: %v", ticker, signals.FamilyStrategyPullback, err)
	}
	sig, newPullback := s.detector.CheckTrendPullback(ticker, candles, prevPullback)
	if sig != nil {
		out = append(out, *sig)
	}
	if newPullback != prevPullback {
		if err := s.store.SetSignalState(ticker, signals.FamilyStrategyPullback, newPullback); err != nil {
			logger.Errorf("signal state %s/%s: %v", ticker, signals.FamilyStrategyPullback, err)
		}
	}

	// Strategy 5: Trust Follow (Phase 15 網 5, TW only) — the FinMind call is
	// short-circuited behind TrustFollowTechnicalGate the same way Strategy
	// 3's revenue-growth gate is short-circuited (see revenueGrowthOK's doc
	// comment): only tickers that already clear the candle-only liquidity/
	// trend/deviation conditions are worth a network request, keeping this to
	// a handful of TW tickers per day rather than the whole universe.
	p := signals.DefaultScreenParams(market.Of(ticker))
	if p.RequireTrustData && s.trustNet != nil && signals.TrustFollowTechnicalGate(candles, p) {
		prevTrust, err := s.store.GetSignalState(ticker, signals.FamilyStrategyTrust)
		if err != nil {
			logger.Errorf("signal state %s/%s: %v", ticker, signals.FamilyStrategyTrust, err)
		}
		rows, err := s.trustNet.GetTrustNetSeries(ticker, len(candles))
		if err != nil {
			logger.Errorf("trust net %s: %v", ticker, err)
		} else {
			trustAligned := signals.AlignTrustNet(candles, rows)
			foreignAligned := signals.AlignForeignNet(candles, rows)
			sig, newTrust := s.detector.CheckTrustFollow(ticker, candles, trustAligned, foreignAligned, prevTrust)
			if sig != nil {
				out = append(out, *sig)
			}
			if newTrust != prevTrust {
				if err := s.store.SetSignalState(ticker, signals.FamilyStrategyTrust, newTrust); err != nil {
					logger.Errorf("signal state %s/%s: %v", ticker, signals.FamilyStrategyTrust, err)
				}
			}
		}
	}

	return out
}

// revenueGrowthOK is Phase 14 §4.2c's short-circuit fundamentals gate for
// 網 3【趨勢突破】's TW-only revenue-growth condition: called only after the
// technical AND already passed (0-5 tickers/day, not the full universe), so
// routing through the caller's cached fundamentals reader keeps this to one
// request per ticker per cache window rather than one per scan. minPct is
// ScreenParams.MinRevenueGrowthPct; the underlying field differs by market
// (data.Fundamentals.MonthRevenueYoYPct is TW-only) but s.fundamentals
// already routes US/TW via the caller's FundamentalsRouter, so this reads
// whichever field is non-zero for ticker's market.
func (s *ScanService) revenueGrowthOK(ticker string, minPct float64) bool {
	if s.fundamentals == nil {
		return false
	}
	fd, err := s.fundamentals(ticker)
	if err != nil {
		logger.Errorf("revenue growth gate %s: %v", ticker, err)
		return false
	}
	growth := fd.MonthRevenueYoYPct
	if market.Of(ticker) != market.TW {
		growth = fd.RevenueGrowthYoY
	}
	return growth > minPct
}

// universeScanRequestDelay and scanChunkCount govern Phase 2.6's daily
// candidate-pool scan. Originally the universe (~500 S&P 500 + manual
// tickers) was split into 5 rotating slices so each day only fetched ~100
// histories; that traded freshness for request volume, which cost more than
// it saved — a Squeeze Breakout seen 4 days late is a chase, not an entry,
// and the RSI/MACD state machines silently collapsed any round trip that
// started and finished inside one rotation. Since the scan is an unattended
// 05:45 cron with no ctx timeout, wall-clock is free where request *rate* is
// not, so the chunking is off (chunkCount 1 = whole universe daily) and the
// budget is spent on a longer per-request delay instead: ~500 tickers × 1s
// is ~10 min, still well under the 06:00 backup, at a third of the old
// requests-per-second. Both stay tunable knobs rather than inlined constants
// — if Yahoo ever starts 429ing, raise the delay first, and only go back to
// chunkCount 2+ if that isn't enough.
const (
	scanChunkCount           = 1
	universeScanRequestDelay = 1 * time.Second
)

// UniverseScanResult is what RunUniverseScan reports back to whoever
// scheduled it. Skipped means the market was closed and nothing was fetched
// — distinct from Scanned == 0, which would mean the universe itself was
// empty.
type UniverseScanResult struct {
	Market  market.MarketID
	Scanned int
	Hits    int
	Skipped bool
}

// RunUniverseScan is Phase 2.6's candidate-pool scan, generalized by
// Phase 6 PR2 to run per-market: it checks market m's universe entries
// (all of them daily as of scanChunkCount 1, see that const's comment;
// still routed through UniverseScanChunk so the rotation can come back as a
// one-line change) (filtered via market.Of(ticker) — not by source, since a
// manually /universe add'ed TW ticker is source='manual' and must still be
// scanned as TW, see docs/phase-6-tw-market.md §5.2) excluding anything
// already on m's own watchlist (which gets a full RSI/MACD check daily
// anyway) for a fresh RSI/MACD signal via the same CheckStatefulSignals used
// for the watchlist — safe to share signal_states with it since the universe
// and watchlist ticker sets never overlap, and safe to share across US/TW
// runs of this same function since a ticker only ever belongs to one market.
// Any hit is logged to scan_hits for the daily report/handleRecommend to
// pick up the same day and upgrade into an LLM candidate.
//
// Moved here from bot.runUniverseScan (Phase 24 Stage 3 Step 3.2): this is
// the one scheduled job that never sent anything to Telegram in the first
// place — results have always gone to the DB and the log — so inverting it
// to "scheduler calls a service, gets a DTO" needed no notification path at
// all. One consequence, deliberate: the scan now also runs on a process with
// no Telegram configured, where it previously sat inside main.go's
// Telegram-only job block and silently never ran.
func (s *ScanService) RunUniverseScan(ctx context.Context, m market.MarketID) (UniverseScanResult, error) {
	// Trading-day gate (Phase 13 §8) — silent, same closed-market signals
	// the closing snapshot/daily report already use per market (US: NYSE
	// calendar; TW: a live quote-freshness heuristic, see marketClosed for
	// why TW has no fixed holiday calendar). Without this, a holiday rerun
	// would scan stale/unchanged data and risk a duplicate scan_hits row for
	// the same signal.
	if s.marketClosed(m) {
		logger.Infof("universe scan: market=%s closed, skipping", m)
		return UniverseScanResult{Market: m, Skipped: true}, nil
	}

	// history/quotes are the two dependencies ScanConfig legitimately leaves
	// nil for a CheckStatefulSignals-only caller (see ScanService's doc
	// comment) — but ComputeMarketRegime below dereferences both
	// unconditionally, so a caller that builds one of those degraded
	// instances and calls RunUniverseScan anyway (on an open market — a
	// closed one never reaches here) needs a real error, not a nil-pointer
	// panic a few lines down.
	if s.history == nil || s.quotes == nil {
		return UniverseScanResult{Market: m}, errors.New("universe scan: no history/quotes provider configured")
	}

	entries, err := s.store.GetUniverse()
	if err != nil {
		return UniverseScanResult{Market: m}, err
	}
	watchlist, err := s.store.GetWatchlistByMarket(m)
	if err != nil {
		return UniverseScanResult{Market: m}, err
	}
	watchSet := make(map[string]bool, len(watchlist))
	for _, t := range watchlist {
		watchSet[t] = true
	}

	var tickers []string
	for _, e := range entries {
		if market.Of(e.Ticker) == m && !watchSet[e.Ticker] {
			tickers = append(tickers, e.Ticker)
		}
	}

	isBear := IsBearRegime(ComputeMarketRegime(s.history, s.quotes, m, BenchmarkFor(m), VIXTicker))

	// Phase 16: skip TWSE/TPEx disposition (處置) or attention (注意)
	// tickers — the bot would otherwise happily recommend a stock currently
	// in split-auction trading with no idea anything's wrong. Fetched once
	// per scan (not per ticker), nil when SHIOAJI_ADDR isn't set.
	restricted := s.RestrictedTickers(ctx, m)

	chunk := UniverseScanChunk(tickers, scanChunkCount, s.now().In(cst).YearDay())
	date := s.now().In(cst).Format("2006-01-02")
	out := UniverseScanResult{Market: m, Scanned: len(chunk)}
	for i, t := range chunk {
		select {
		case <-ctx.Done():
			logger.Warnf("universe scan: cancelled after %d/%d tickers", i, len(chunk))
			out.Scanned = i
			return out, ctx.Err()
		default:
		}

		if reason, ok := restricted[t]; ok {
			logger.Infof("universe scan: skipping %s: %s", t, reason)
			continue
		}

		candles, err := s.history.GetHistory(t, "1y")
		if err != nil {
			logger.Errorf("universe scan: history %s: %v", t, err)
			continue
		}
		for _, sig := range DecorateBearRegime(s.CheckStatefulSignals(t, candles), isBear, s.lang) {
			if err := s.store.SaveScanHit(t, date, sig.Message); err != nil {
				logger.Errorf("universe scan: save hit %s: %v", t, err)
				continue
			}
			out.Hits++
		}

		if i < len(chunk)-1 {
			time.Sleep(universeScanRequestDelay)
		}
	}
	return out, nil
}

// marketClosed answers the trading-day gate for m. US checks *yesterday's*
// CST date, not today's — this job runs at 05:45 CST (like the closing
// snapshot's 05:30), which is already the next calendar day in Taiwan
// relative to the US session just closed, so checking today's date misjudges
// Saturday (a genuine trading day, Friday's session) as a weekend skip and
// can misjudge the day after a US holiday too. TW has no fixed holiday
// calendar to check against, so it falls back to a live 0050 quote-freshness
// heuristic (docs/phase-6-tw-market.md §3.3); a quote-fetch failure is
// treated as closed, since scanning off an unreachable provider would fail
// per-ticker anyway.
func (s *ScanService) marketClosed(m market.MarketID) bool {
	if m == market.US {
		return !market.IsTradingDay(s.now().In(cst).AddDate(0, 0, -1))
	}
	if s.quotes == nil {
		return true
	}
	q, err := s.quotes.GetQuote(BenchmarkFor(market.TW))
	if err != nil {
		logger.Errorf("tw market closed check: quote: %v", err)
		return true
	}
	return time.Since(q.Timestamp) > TWMarketClosedStaleness
}

// TWMarketClosedStaleness is how old a 0050 quote's timestamp must be before
// the TW market counts as closed. 12h rather than something tighter because
// the providers' TW quote timestamps are only as fresh as their own feed —
// the question being answered is "was there a session today at all", not
// "is it open right now".
const TWMarketClosedStaleness = 12 * time.Hour

// RestrictedTickers returns TW disposition (處置)/attention (注意) tickers
// mapped to a human-readable reason, for RunUniverseScan and the bot's
// restricted-stock alerts to skip/warn about — nil for US (no such
// classification exists there) or when no RestrictedProvider is configured
// (SHIOAJI_ADDR not set), same nil-degrade convention as every other
// optional provider. Not cached across calls: RegulatoryPunish/
// RegulatoryNotice are each one free GET request, and this is called at most
// twice a day (once per job).
func (s *ScanService) RestrictedTickers(ctx context.Context, m market.MarketID) map[string]string {
	if m != market.TW || s.restricted == nil {
		return nil
	}
	out := make(map[string]string)
	if punish, err := s.restricted.RegulatoryPunish(ctx); err != nil {
		logger.Errorf("restricted tickers: regulatory punish: %v", err)
	} else {
		for code, reason := range punish {
			out[code] = reason
		}
	}
	if notice, err := s.restricted.RegulatoryNotice(ctx); err != nil {
		logger.Errorf("restricted tickers: regulatory notice: %v", err)
	} else {
		for code, reason := range notice {
			if _, ok := out[code]; !ok {
				out[code] = reason
			}
		}
	}
	return out
}

// DecorateBearRegime appends the bear-regime caveat to every strategy hit's
// message when the market is weak. It stays a formatting concern, but not an
// adapter-only one: RunUniverseScan persists the decorated text into
// scan_hits, which is read back by the daily report and the LLM pipeline —
// so the decoration has to happen before the write, not at render time.
// Returns sigs itself (decorating in place); callers own the slice.
func DecorateBearRegime(sigs []signals.Signal, isBear bool, lang i18n.Lang) []signals.Signal {
	if !isBear {
		return sigs
	}
	for i := range sigs {
		if strings.HasPrefix(sigs[i].Type, "strategy_") {
			sigs[i].Message += "\n" + i18n.T(lang, i18n.KeyStrategyBearRegimeWarning)
		}
	}
	return sigs
}

// BenchmarkFor is the project-wide market benchmark: SPY for US, 0050 for
// TW. Previously spelled out separately in internal/bot, internal/web and
// internal/mcptools; the bot's copy now delegates here.
func BenchmarkFor(m market.MarketID) string {
	if m == market.TW {
		return "0050"
	}
	return "SPY"
}

// VIXTicker is ComputeMarketRegime's volatility input — a US symbol used for
// both markets, since TW has no free equivalent index quote.
const VIXTicker = "^VIX"

// IsBearRegime reports whether the market context indicates a weak/bear
// regime (the benchmark below its 50-day or 200-day moving average).
func IsBearRegime(mc *llm.MarketContext) bool {
	if mc == nil || mc.SPYPrice == 0 {
		return false
	}
	if mc.SPYMA50 > 0 && mc.SPYPrice < mc.SPYMA50 {
		return true
	}
	if mc.SPYMA200 > 0 && mc.SPYPrice < mc.SPYMA200 {
		return true
	}
	return false
}

// UniverseScanChunk returns the slice of tickers to scan for dayIndex (an
// ever-increasing day counter, e.g. time.Now().YearDay()), rotating through
// all of tickers over chunkCount calls. Pure and stateless — no persisted
// scan cursor needed — so coverage is deterministic given the same tickers
// and dayIndex, at the cost of chunk boundaries shifting slightly as the
// universe's membership changes day to day (harmless: PLAN.md tolerates
// staleness on the order of months for this data).
func UniverseScanChunk(tickers []string, chunkCount, dayIndex int) []string {
	if len(tickers) == 0 || chunkCount <= 0 {
		return nil
	}
	size := (len(tickers) + chunkCount - 1) / chunkCount
	idx := dayIndex % chunkCount
	if idx < 0 {
		idx += chunkCount
	}
	start := idx * size
	if start >= len(tickers) {
		return nil
	}
	end := start + size
	if end > len(tickers) {
		end = len(tickers)
	}
	return tickers[start:end]
}
