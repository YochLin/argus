package service

import (
	"sort"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/market"
)

var recommendationCST = time.FixedZone("CST", 8*3600)

// RecommendationStore is the persistence boundary needed to track stored
// recommendations against today's quotes.
type RecommendationStore interface {
	GetRecommendationsSince(fromDate string) ([]db.Recommendation, error)
	GetSnapshotClose(ticker, date string) (float64, bool, error)
}

// RecommendationTrackRow is one BUY/SELL recommendation reduced to the
// fields needed for hit-rate and return aggregation.
type RecommendationTrackRow struct {
	Action    string
	Source    string
	Market    market.MarketID
	ChangePct float64
	Hit       bool
}

// RecommendationTrackStats accumulates hit-rate and average-magnitude
// statistics for one group of tracked recommendations.
type RecommendationTrackStats struct {
	Hits, Evaluated int
	BuySum          float64
	BuyCount        int
	SellSum         float64
	SellCount       int
}

func (s RecommendationTrackStats) HitRate() float64 {
	if s.Evaluated == 0 {
		return 0
	}
	return float64(s.Hits) / float64(s.Evaluated) * 100
}

func (s RecommendationTrackStats) AvgBuyPct() float64 {
	if s.BuyCount == 0 {
		return 0
	}
	return s.BuySum / float64(s.BuyCount)
}

func (s RecommendationTrackStats) AvgSellPct() float64 {
	if s.SellCount == 0 {
		return 0
	}
	return s.SellSum / float64(s.SellCount)
}

// RecommendationTrackSummary contains overall, source, and market
// aggregations. The maps are always initialized, including for an empty
// report, so adapters can render them without nil checks.
type RecommendationTrackSummary struct {
	Overall  RecommendationTrackStats
	BySource map[string]RecommendationTrackStats
	ByMarket map[market.MarketID]RecommendationTrackStats
}

// RecommendationTrackDetail carries the transport-neutral data needed to
// render one stored recommendation. A detail can be present without a row
// when its entry price or current quote is unavailable; those cases are
// intentionally rendered as a degraded line by each adapter.
type RecommendationTrackDetail struct {
	Date               string
	Ticker             string
	Action             string
	Source             string
	Market             market.MarketID
	BasePrice          float64
	CurrentPrice       float64
	ChangePct          float64
	BenchmarkTicker    string
	BenchmarkChangePct float64
	HasBasePrice       bool
	HasQuote           bool
	HasBenchmark       bool
	Hit                bool
	QuoteErr           error
	BenchmarkErr       error
}

// RecommendationTrackReport is the result of tracking every recommendation
// in the requested window. HasRecommendations distinguishes an empty DB
// window from a non-empty window whose rows all lack enough price data.
type RecommendationTrackReport struct {
	HasRecommendations bool
	Details            []RecommendationTrackDetail
	Rows               []RecommendationTrackRow
	Summary            RecommendationTrackSummary
}

// RecommendationTrackingService owns the shared /track use case. It reads
// recommendations, resolves their entry prices, fetches current prices and
// market-specific benchmarks, then computes the relative hit rule. Adapters
// remain responsible for localization, formatting, caching, and error text.
type RecommendationTrackingService struct {
	store  RecommendationStore
	quotes QuoteReader
	now    func() time.Time
}

func NewRecommendationTrackingService(store RecommendationStore, quotes QuoteReader) *RecommendationTrackingService {
	return &RecommendationTrackingService{store: store, quotes: quotes, now: time.Now}
}

// Track evaluates recommendations from the previous days days. A BUY is a
// hit when it beats its market benchmark; a SELL is a hit when it
// underperforms it. If the same-period benchmark close is unavailable, the
// rule falls back to absolute direction, preserving the original /track
// behavior.
func (s *RecommendationTrackingService) Track(days int) (RecommendationTrackReport, error) {
	fromDate := s.now().In(recommendationCST).AddDate(0, 0, -days).Format("2006-01-02")
	recs, err := s.store.GetRecommendationsSince(fromDate)
	if err != nil {
		return RecommendationTrackReport{}, err
	}

	report := RecommendationTrackReport{
		HasRecommendations: len(recs) > 0,
		Details:            make([]RecommendationTrackDetail, 0, len(recs)),
		Rows:               make([]RecommendationTrackRow, 0, len(recs)),
		Summary: RecommendationTrackSummary{
			BySource: make(map[string]RecommendationTrackStats),
			ByMarket: make(map[market.MarketID]RecommendationTrackStats),
		},
	}
	if len(recs) == 0 {
		return report, nil
	}

	quotes := make(map[string]*data.Quote)
	quoteErrs := make(map[string]error)
	quoteLoaded := make(map[string]bool)
	benchmarkQuotes := make(map[market.MarketID]*data.Quote)
	benchmarkErrs := make(map[market.MarketID]error)
	benchmarkLoaded := make(map[market.MarketID]bool)

	for _, rec := range recs {
		recMarket := market.Of(rec.Ticker)
		detail := RecommendationTrackDetail{
			Date:            rec.Date,
			Ticker:          rec.Ticker,
			Action:          displayAction(rec.Action),
			Source:          DisplaySource(rec.Source),
			Market:          recMarket,
			BenchmarkTicker: BenchmarkFor(recMarket),
		}

		base := rec.Price
		if base == 0 {
			if close, ok, closeErr := s.store.GetSnapshotClose(rec.Ticker, rec.Date); closeErr == nil && ok {
				base = close
			}
		}
		if base == 0 {
			report.Details = append(report.Details, detail)
			continue
		}
		detail.BasePrice = base
		detail.HasBasePrice = true

		if !quoteLoaded[rec.Ticker] {
			quoteLoaded[rec.Ticker] = true
			if s.quotes == nil {
				quoteErrs[rec.Ticker] = errQuoteProviderUnavailable
			} else {
				q, quoteErr := s.quotes.GetQuote(rec.Ticker)
				quoteErrs[rec.Ticker] = quoteErr
				if quoteErr == nil {
					quotes[rec.Ticker] = q
				}
			}
		}
		q := quotes[rec.Ticker]
		if q == nil {
			detail.QuoteErr = quoteErrs[rec.Ticker]
			report.Details = append(report.Details, detail)
			continue
		}
		detail.HasQuote = true
		detail.CurrentPrice = q.Price
		detail.ChangePct = (q.Price - base) / base * 100

		if !benchmarkLoaded[recMarket] {
			benchmarkLoaded[recMarket] = true
			benchmark := BenchmarkFor(recMarket)
			if s.quotes != nil {
				q, quoteErr := s.quotes.GetQuote(benchmark)
				benchmarkErrs[recMarket] = quoteErr
				if quoteErr == nil {
					benchmarkQuotes[recMarket] = q
				}
			} else {
				benchmarkErrs[recMarket] = errQuoteProviderUnavailable
			}
		}
		benchmarkQuote := benchmarkQuotes[recMarket]
		detail.BenchmarkErr = benchmarkErrs[recMarket]
		if benchmarkQuote != nil {
			if benchmarkBase, ok, closeErr := s.store.GetSnapshotClose(detail.BenchmarkTicker, rec.Date); closeErr == nil && ok && benchmarkBase != 0 {
				detail.BenchmarkChangePct = (benchmarkQuote.Price - benchmarkBase) / benchmarkBase * 100
				detail.HasBenchmark = true
			}
		}
		if rec.Action == "BUY" || rec.Action == "SELL" {
			detail.Hit = TrackHit(rec.Action, detail.ChangePct, detail.BenchmarkChangePct, detail.HasBenchmark)
			report.Rows = append(report.Rows, RecommendationTrackRow{
				Action:    rec.Action,
				Source:    detail.Source,
				Market:    recMarket,
				ChangePct: detail.ChangePct,
				Hit:       detail.Hit,
			})
		}
		report.Details = append(report.Details, detail)
	}

	report.Summary = SummarizeTrack(report.Rows)
	return report, nil
}

var errQuoteProviderUnavailable = &quoteProviderError{}

type quoteProviderError struct{}

func (*quoteProviderError) Error() string { return "quote provider unavailable" }

func displayAction(action string) string {
	if action == "" {
		return "—"
	}
	return action
}

// TrackHit applies the relative-to-benchmark rule used by /track.
func TrackHit(action string, tickerChangePct, benchmarkChangePct float64, haveBenchmark bool) bool {
	switch action {
	case "BUY":
		if haveBenchmark {
			return tickerChangePct > benchmarkChangePct
		}
		return tickerChangePct > 0
	case "SELL":
		if haveBenchmark {
			return tickerChangePct < benchmarkChangePct
		}
		return tickerChangePct < 0
	default:
		return false
	}
}

// DisplaySource normalizes recommendations saved before the source column
// existed. Those rows represent the original watchlist path.
func DisplaySource(source string) string {
	if source == "" {
		return "watchlist"
	}
	return source
}

// SummarizeTrack aggregates tracking rows into overall, source, and market
// groups. It is exported so adapters can summarize a market-filtered subset
// for the weekly review without reimplementing the business rule.
func SummarizeTrack(rows []RecommendationTrackRow) RecommendationTrackSummary {
	summary := RecommendationTrackSummary{
		BySource: make(map[string]RecommendationTrackStats),
		ByMarket: make(map[market.MarketID]RecommendationTrackStats),
	}
	for _, row := range rows {
		accumulate(&summary.Overall, row)
		sourceStats := summary.BySource[row.Source]
		accumulate(&sourceStats, row)
		summary.BySource[row.Source] = sourceStats
		marketStats := summary.ByMarket[row.Market]
		accumulate(&marketStats, row)
		summary.ByMarket[row.Market] = marketStats
	}
	return summary
}

func accumulate(stats *RecommendationTrackStats, row RecommendationTrackRow) {
	stats.Evaluated++
	if row.Hit {
		stats.Hits++
	}
	switch row.Action {
	case "BUY":
		stats.BuySum += row.ChangePct
		stats.BuyCount++
	case "SELL":
		stats.SellSum += row.ChangePct
		stats.SellCount++
	}
}

// SortedSourceKeys returns stable source ordering for text adapters.
func SortedSourceKeys(bySource map[string]RecommendationTrackStats) []string {
	keys := make([]string, 0, len(bySource))
	for key := range bySource {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
