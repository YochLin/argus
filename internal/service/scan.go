package service

import (
	"argus/internal/data"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/signals"
)

// ScanStore is the signal_states dedup boundary CheckStatefulSignals needs —
// the same shape RiskStore's GetSignalState/SetSignalState pair covers, kept
// separate here since ScanService has no need for RiskStore's other methods.
type ScanStore interface {
	GetSignalState(ticker, family string) (string, error)
	SetSignalState(ticker, family, state string) error
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
}

func NewScanService(store ScanStore, detector *signals.Detector, trustNet data.TrustNetProvider, fundamentals func(ticker string) (*data.Fundamentals, error)) *ScanService {
	return &ScanService{store: store, detector: detector, trustNet: trustNet, fundamentals: fundamentals}
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
