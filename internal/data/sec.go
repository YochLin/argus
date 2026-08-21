package data

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// SEC wraps the SEC EDGAR XBRL frames API — Phase 23 PR6's US-only source
// for valuation history and cash-flow quality (docs/phase-23-strategy-data-uplift.md
// §3.2/§3.3), replacing an earlier Yahoo fundamentals-timeseries attempt that
// turned out to be capped at 4 years/5 quarters on the free tier (§3.1).
// userAgent is mandatory and must contain a real, working email address —
// SEC's edge filter technically requires the UA to look like
// "name contact@example.com" (live-verified: a repo-URL or bare "Mozilla/5.0"
// UA gets a 403; only an email-shaped one gets 200) and a bad address just
// gets the whole IP silently blocked with no notice, not a bounced email.
// SEC has no API key/registration — see NewSEC's caller for the nil-degrade
// gate (constructed only when SEC_USER_AGENT is set).
type SEC struct {
	userAgent string
	client    *http.Client
	baseURL   string // overridable in tests, defaults to the real API host
	tickerURL string // overridable in tests, defaults to the real ticker index

	cikOnce     sync.Once
	cikByTicker map[string]int
	cikErr      error

	reqMu       sync.Mutex
	lastRequest time.Time
}

// secMinRequestInterval caps SEC calls at 5 req/s (docs/phase-23-strategy-data-uplift.md
// §3.5: half of SEC's stated 10 req/s limit) — this project's on-demand,
// 90-day-cached access pattern never comes close to needing the other half,
// so the margin just protects against a burst (e.g. a fresh install's first
// report touching watchlist+positions+candidates all at once).
const secMinRequestInterval = 200 * time.Millisecond

func NewSEC(userAgent string) *SEC {
	return &SEC{
		userAgent: userAgent,
		client:    &http.Client{Timeout: 15 * time.Second},
		baseURL:   "https://data.sec.gov",
		tickerURL: "https://www.sec.gov/files/company_tickers.json",
	}
}

// FundamentalSnapshot is one ticker's Phase 23 PR6 valuation/cash-flow
// summary, derived from SEC EDGAR's companyfacts — briefing material only
// (docs/phase-23-strategy-data-uplift.md §4.2: never a ranking factor, never
// a hard filter). EPSAnnual/PERatio/OCF/NetIncome are the latest 10-K's
// figures (a fiscal-YEAR reading, not a trailing-twelve-month reconstruction
// — XBRL's own quarterly points don't cleanly sum to TTM without a
// subtraction trick this project deliberately skips as false precision for
// a briefing line). PEPercentile/CashFlowQuality are nil, not zero, when
// they can't be honestly computed (see computeFundamentalSnapshot) — a
// fresh IPO with too little annual history, or a loss-making fiscal year.
type FundamentalSnapshot struct {
	Ticker            string
	AsOfFiscalYearEnd string // the latest 10-K's fiscal-year-end date this snapshot is based on
	EPSAnnual         float64
	PERatio           float64  // current price / EPSAnnual; 0 when EPSAnnual <= 0 (P/E undefined for a loss year)
	PEPercentile      *float64 // 0-100, current P/E's percentile within its own annual P/E history; nil when there's no usable history
	OCF               float64
	NetIncome         float64
	CashFlowQuality   *float64 // OCF / NetIncome for the same fiscal year; nil when NetIncome == 0 or the two tags don't share a matching period
}

// FundamentalHistoryProvider is nil-checked by callers the same way every
// other optional data source in this package is (see FundamentalsProvider
// etc. in provider.go) — nil whenever SEC_USER_AGENT isn't set.
type FundamentalHistoryProvider interface {
	// GetFundamentalSnapshot fetches ticker's SEC EDGAR history and pairs it
	// against priceCandles (oldest-first, ideally several years deep — a 1y
	// window is too short for a meaningful multi-fiscal-year percentile) to
	// compute valuation/cash-flow figures. Returns nil, nil (not an error)
	// when EDGAR has no usable data for ticker at all — same "omit the line,
	// don't fabricate a number" convention as everywhere else here.
	GetFundamentalSnapshot(ticker string, priceCandles []Candle) (*FundamentalSnapshot, error)
}

func (s *SEC) throttle() {
	s.reqMu.Lock()
	defer s.reqMu.Unlock()
	if wait := secMinRequestInterval - time.Since(s.lastRequest); wait > 0 {
		time.Sleep(wait)
	}
	s.lastRequest = time.Now()
}

func (s *SEC) get(url string) (*http.Response, error) {
	s.throttle()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", s.userAgent)
	// Deliberately no explicit Accept-Encoding header: net/http's default
	// Transport already requests gzip and transparently decompresses when
	// the caller hasn't set the header itself — live-verified 14x smaller
	// on the wire (§3.2), so setting it manually here would only add work
	// (manual gzip.NewReader) for no benefit.
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("sec: %s: status %d", url, resp.StatusCode)
	}
	return resp, nil
}

// ensureCIKIndex lazily fetches company_tickers.json (SEC's ticker -> CIK
// index) once per process lifetime — it's a slow-moving list (new listings/
// delistings only), so there's no TTL, just a one-time load shared by every
// GetFundamentalSnapshot call.
func (s *SEC) ensureCIKIndex() error {
	s.cikOnce.Do(func() {
		s.cikByTicker, s.cikErr = s.fetchCIKIndex()
	})
	return s.cikErr
}

func (s *SEC) fetchCIKIndex() (map[string]int, error) {
	resp, err := s.get(s.tickerURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var raw map[string]struct {
		CIK    int    `json:"cik_str"`
		Ticker string `json:"ticker"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("sec: decode company_tickers.json: %w", err)
	}
	out := make(map[string]int, len(raw))
	for _, v := range raw {
		out[strings.ToUpper(v.Ticker)] = v.CIK
	}
	return out, nil
}

type secFactPoint struct {
	End   string  `json:"end"`
	Val   float64 `json:"val"`
	Form  string  `json:"form"`
	Filed string  `json:"filed"`
}

type secCompanyFacts struct {
	Facts struct {
		USGAAP map[string]struct {
			Units map[string][]secFactPoint `json:"units"`
		} `json:"us-gaap"`
	} `json:"facts"`
}

func (s *SEC) fetchCompanyFacts(cik int) (*secCompanyFacts, error) {
	url := fmt.Sprintf("%s/api/xbrl/companyfacts/CIK%010d.json", s.baseURL, cik)
	resp, err := s.get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var facts secCompanyFacts
	if err := json.NewDecoder(resp.Body).Decode(&facts); err != nil {
		return nil, fmt.Errorf("sec: decode companyfacts CIK%d: %w", cik, err)
	}
	return &facts, nil
}

func (s *SEC) GetFundamentalSnapshot(ticker string, priceCandles []Candle) (*FundamentalSnapshot, error) {
	if err := s.ensureCIKIndex(); err != nil {
		return nil, err
	}
	cik, ok := s.cikByTicker[strings.ToUpper(ticker)]
	if !ok {
		return nil, fmt.Errorf("sec: no CIK found for ticker %s", ticker)
	}
	facts, err := s.fetchCompanyFacts(cik)
	if err != nil {
		return nil, err
	}
	return computeFundamentalSnapshot(ticker, facts, priceCandles), nil
}

// annualPoint is one 10-K fiscal-year figure for a single us-gaap tag.
type annualPoint struct {
	end string // fiscal year end, "2025-09-27"
	val float64
}

// annual10KPoints extracts tag's 10-K-only data points (skipping 10-Q
// quarterly figures entirely — see FundamentalSnapshot's doc comment on why
// this project doesn't attempt a TTM reconstruction), deduped by fiscal
// year end (keeping the latest-filed value, in case of a later restatement)
// and sorted ascending by end date. Returns nil if the tag isn't present at
// all — a normal outcome (not every company reports every tag), not an
// error.
func annual10KPoints(facts *secCompanyFacts, tag string) []annualPoint {
	t, ok := facts.Facts.USGAAP[tag]
	if !ok {
		return nil
	}
	byEnd := make(map[string]secFactPoint)
	for _, pts := range t.Units {
		for _, p := range pts {
			if p.Form != "10-K" {
				continue
			}
			if existing, ok := byEnd[p.End]; !ok || p.Filed > existing.Filed {
				byEnd[p.End] = p
			}
		}
	}
	out := make([]annualPoint, 0, len(byEnd))
	for end, p := range byEnd {
		out = append(out, annualPoint{end: end, val: p.Val})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].end < out[j].end })
	return out
}

// priceNear returns the close of the latest candle on or before dateStr —
// candles must be sorted ascending by date (GetHistory's standard contract).
// ok is false when dateStr doesn't parse or every candle is after it (the
// price history doesn't reach back that far).
func priceNear(candles []Candle, dateStr string) (float64, bool) {
	target, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return 0, false
	}
	best := -1
	for i, c := range candles {
		if c.Date.After(target) {
			break
		}
		best = i
	}
	if best < 0 {
		return 0, false
	}
	return candles[best].Close, true
}

// percentileOf returns what percent of vals are <= x, in [0, 100].
func percentileOf(vals []float64, x float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	var countLE int
	for _, v := range vals {
		if v <= x {
			countLE++
		}
	}
	return float64(countLE) / float64(len(vals)) * 100.0
}

// minPEHistoryPoints is the fewest priced, profitable fiscal years needed
// before a percentile is meaningful enough to show — below this the number
// would carry false precision (e.g. "50th percentile" off a single
// comparison point).
const minPEHistoryPoints = 3

// peHistoryPercentile builds each profitable fiscal year's P/E (that year's
// close-to-fiscal-year-end price / that year's EPS) and returns where
// currentPE ranks within that self-relative distribution.
func peHistoryPercentile(epsPoints []annualPoint, candles []Candle, currentPE float64) (float64, bool) {
	if currentPE <= 0 {
		return 0, false
	}
	var pes []float64
	for _, p := range epsPoints {
		if p.val <= 0 {
			continue
		}
		price, ok := priceNear(candles, p.end)
		if !ok {
			continue
		}
		pes = append(pes, price/p.val)
	}
	if len(pes) < minPEHistoryPoints {
		return 0, false
	}
	return percentileOf(pes, currentPE), true
}

// computeFundamentalSnapshot is the pure (network-free, unit-testable) half
// of GetFundamentalSnapshot. Returns nil when EDGAR had nothing usable at
// all for this ticker (e.g. a tag-name mismatch this v0 doesn't know about
// yet — docs/phase-23-strategy-data-uplift.md §5: "tag 正規化不用一次做完").
func computeFundamentalSnapshot(ticker string, facts *secCompanyFacts, priceCandles []Candle) *FundamentalSnapshot {
	epsPoints := annual10KPoints(facts, "EarningsPerShareDiluted")
	if len(epsPoints) == 0 {
		epsPoints = annual10KPoints(facts, "EarningsPerShareBasic")
	}
	ocfPoints := annual10KPoints(facts, "NetCashProvidedByUsedInOperatingActivities")
	niPoints := annual10KPoints(facts, "NetIncomeLoss")

	snap := &FundamentalSnapshot{Ticker: ticker}
	haveAny := false

	if len(epsPoints) > 0 {
		latest := epsPoints[len(epsPoints)-1]
		snap.EPSAnnual = latest.val
		snap.AsOfFiscalYearEnd = latest.end
		haveAny = true
		if latest.val > 0 && len(priceCandles) > 0 {
			currentPrice := priceCandles[len(priceCandles)-1].Close
			snap.PERatio = currentPrice / latest.val
			if pct, ok := peHistoryPercentile(epsPoints, priceCandles, snap.PERatio); ok {
				snap.PEPercentile = &pct
			}
		}
	}

	if len(ocfPoints) > 0 && len(niPoints) > 0 {
		latestOCF := ocfPoints[len(ocfPoints)-1]
		for _, ni := range niPoints {
			if ni.end != latestOCF.end {
				continue
			}
			snap.OCF = latestOCF.val
			snap.NetIncome = ni.val
			haveAny = true
			if ni.val != 0 {
				q := latestOCF.val / ni.val
				snap.CashFlowQuality = &q
			}
			break
		}
	}

	if !haveAny {
		return nil
	}
	return snap
}
