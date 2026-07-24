package data

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"argus/internal/market"
)

// FinMind wraps api.finmindtrade.com's REST API — Taiwan-only fundamentals,
// the FundamentalsProvider counterpart to Yahoo's TW quote/history role for
// Provider (see yahoo.go): Finnhub's free tier doesn't cover Taiwan listings
// at all (see errTWNotSupported in fundamentals.go), so this is the only
// fundamentals source for a TW ticker. FINMIND_TOKEN is presence-gated the
// same way FINNHUB_API_KEY is — live-tested 2026-07-24, every dataset used
// here (TaiwanStockPER, TaiwanStockFinancialStatements,
// TaiwanStockMonthRevenue) returns real data completely unauthenticated, but
// this project still requires a token before constructing a client at all,
// matching the existing "commit to a real account before depending on a
// data source" convention rather than quietly running anonymous.
type FinMind struct {
	token   string
	client  *http.Client
	baseURL string // overridable in tests, defaults to the real API
}

func NewFinMind(token string) *FinMind {
	return &FinMind{
		token:   token,
		client:  &http.Client{Timeout: 10 * time.Second},
		baseURL: "https://api.finmindtrade.com/api/v4/data",
	}
}

func (f *FinMind) Name() string { return "finmind" }

// errNotTWTicker guards every FinMind method against a US ticker — FinMind
// only carries Taiwan listings, so without this a US ticker would waste a
// doomed request before the caller's router falls through to Finnhub (same
// "doomed request" shape as Finnhub's errTWNotSupported, market swapped).
var errNotTWTicker = errors.New("finmind: not a taiwan ticker")

// finmindEnvelope is every /api/v4/data response's outer shape — status 200
// means data is the real payload; anything else means msg carries a
// human-readable rejection (e.g. "Your level is free. Please update your
// user level." for a dataset/query shape the free tier doesn't cover — see
// the whole-market TaiwanStockNews finding in docs/phase-6-tw-market.md).
type finmindEnvelope struct {
	Msg    string          `json:"msg"`
	Status int             `json:"status"`
	Data   json.RawMessage `json:"data"`
}

func (f *FinMind) get(dataset, dataID, startDate string, out any) error {
	url := fmt.Sprintf("%s?dataset=%s&data_id=%s&start_date=%s", f.baseURL, dataset, dataID, startDate)
	if f.token != "" {
		url += "&token=" + f.token
	}
	resp, err := f.client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var env finmindEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return err
	}
	if env.Status != 200 {
		return fmt.Errorf("finmind: %s", env.Msg)
	}
	return json.Unmarshal(env.Data, out)
}

type finmindPERRow struct {
	Date          string  `json:"date"`
	DividendYield float64 `json:"dividend_yield"`
	PER           float64 `json:"PER"`
	PBR           float64 `json:"PBR"`
}

type finmindRevenueRow struct {
	Date         string  `json:"date"`
	Revenue      float64 `json:"revenue"`
	RevenueMonth int     `json:"revenue_month"`
	RevenueYear  int     `json:"revenue_year"`
}

type finmindStatementRow struct {
	Date  string  `json:"date"`
	Type  string  `json:"type"`
	Value float64 `json:"value"`
}

// GetFundamentals covers PER/PBR/dividend yield (TaiwanStockPER, a daily
// dataset — take the most recent row), EPS and gross margin (derived from
// the latest TaiwanStockFinancialStatements quarter's Revenue/GrossProfit),
// and month-revenue YoY (TaiwanStockMonthRevenue). Every other Fundamentals
// field (ROE, ROA, Beta, 52-week range, ...) stays 0 — FinMind's free
// datasets don't carry them, and Finnhub's aren't available for a TW ticker
// (see fundamentals.go's errTWNotSupported) — a documented partial-coverage
// gap, not a bug: the LLM already reasons about TW tickers primarily off
// technicals/K-lines (see docs/phase-6-tw-market.md §8).
func (f *FinMind) GetFundamentals(ticker string) (*Fundamentals, error) {
	if market.Of(ticker) != market.TW {
		return nil, errNotTWTicker
	}

	var perRows []finmindPERRow
	perStart := time.Now().AddDate(0, 0, -10).Format("2006-01-02")
	if err := f.get("TaiwanStockPER", ticker, perStart, &perRows); err != nil {
		return nil, err
	}
	if len(perRows) == 0 {
		return nil, fmt.Errorf("finmind: no PER data for %s", ticker)
	}
	sort.Slice(perRows, func(i, j int) bool { return perRows[i].Date < perRows[j].Date })
	latestPER := perRows[len(perRows)-1]

	fd := &Fundamentals{
		Ticker:           ticker,
		PE:               latestPER.PER,
		PB:               latestPER.PBR,
		DividendYieldPct: latestPER.DividendYield,
	}

	// Statement and month-revenue fetches degrade independently — PER/PBR/
	// dividend yield are already in hand above, so a failure here shouldn't
	// fail the whole call, same attach-what's-available convention as
	// internal/bot's fetchStockData.
	statementStart := time.Now().AddDate(0, -6, 0).Format("2006-01-02")
	var stRows []finmindStatementRow
	if err := f.get("TaiwanStockFinancialStatements", ticker, statementStart, &stRows); err == nil {
		var latestDate string
		for _, r := range stRows {
			if r.Date > latestDate {
				latestDate = r.Date
			}
		}
		var revenue, grossProfit float64
		for _, r := range stRows {
			if r.Date != latestDate {
				continue
			}
			switch r.Type {
			case "EPS":
				fd.EPS = r.Value
			case "Revenue":
				revenue = r.Value
			case "GrossProfit":
				grossProfit = r.Value
			}
		}
		if revenue != 0 {
			fd.GrossMarginPct = grossProfit / revenue * 100
		}
	}

	revenueStart := time.Now().AddDate(0, -14, 0).Format("2006-01-02")
	var revRows []finmindRevenueRow
	if err := f.get("TaiwanStockMonthRevenue", ticker, revenueStart, &revRows); err == nil && len(revRows) > 0 {
		sort.Slice(revRows, func(i, j int) bool { return revRows[i].Date < revRows[j].Date })
		latest := revRows[len(revRows)-1]
		for _, r := range revRows {
			if r.RevenueMonth == latest.RevenueMonth && r.RevenueYear == latest.RevenueYear-1 && r.Revenue != 0 {
				fd.MonthRevenueYoYPct = (latest.Revenue - r.Revenue) / r.Revenue * 100
				break
			}
		}
	}

	return fd, nil
}

// GetFinancialStatements returns the latest quarter on record from
// TaiwanStockFinancialStatements. freq is accepted for FundamentalsProvider
// interface parity but not applied — unlike Finnhub's 10-K/10-Q split, this
// dataset only ever returns single-quarter figures (no separate
// cumulative/annual endpoint has been verified), so Form is a bare "Q1"..
// "Q4" label rather than a real filing type. TotalAssets/TotalLiabilities/
// TotalEquity/OperatingCashFlow/CapEx/FreeCashFlow stay 0 — this dataset is
// income-statement-only, no balance sheet or cash flow figures at all (see
// render.FinancialStatement's skip-zero-section handling).
func (f *FinMind) GetFinancialStatements(ticker, freq string) (*FinancialStatement, error) {
	if market.Of(ticker) != market.TW {
		return nil, errNotTWTicker
	}
	var rows []finmindStatementRow
	start := time.Now().AddDate(-2, 0, 0).Format("2006-01-02")
	if err := f.get("TaiwanStockFinancialStatements", ticker, start, &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("finmind: no financial statements for %s", ticker)
	}

	var latest string
	for _, r := range rows {
		if r.Date > latest {
			latest = r.Date
		}
	}

	st := &FinancialStatement{
		Ticker:    ticker,
		Form:      quarterLabel(latest),
		PeriodEnd: latest,
	}
	if t, err := time.Parse("2006-01-02", latest); err == nil {
		st.FiscalYear = t.Year()
	}
	for _, r := range rows {
		if r.Date != latest {
			continue
		}
		switch r.Type {
		case "Revenue":
			st.Revenue = r.Value
		case "GrossProfit":
			st.GrossProfit = r.Value
		case "OperatingIncome":
			st.OperatingIncome = r.Value
		case "IncomeAfterTaxes":
			st.NetIncome = r.Value
		case "EPS":
			st.DilutedEPS = r.Value
		}
	}
	return st, nil
}

// quarterLabel derives "Q1".."Q4" from a quarter-end date; "" (unparseable)
// falls back to "TW" so FinancialStatement's Form field is never empty.
func quarterLabel(dateStr string) string {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return "TW"
	}
	return fmt.Sprintf("Q%d", (int(t.Month())-1)/3+1)
}
