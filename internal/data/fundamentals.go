package data

import (
	"fmt"

	"argus/internal/market"
)

// Fundamentals is a curated set of valuation/profitability/growth ratios.
// Sourced from Finnhub's /stock/metric endpoint — Yahoo's equivalent
// (quoteSummary) now requires a crumb/cookie handshake we don't do, so this
// is Finnhub-only (see FundamentalsProvider).
type Fundamentals struct {
	Ticker             string
	PE                 float64
	PB                 float64
	PS                 float64
	ROE                float64
	ROA                float64
	GrossMarginPct     float64
	OperatingMarginPct float64
	NetMarginPct       float64
	DebtToEquity       float64
	CurrentRatio       float64
	QuickRatio         float64
	RevenueGrowthYoY   float64
	EPSGrowthYoY       float64
	EPS                float64
	DividendYieldPct   float64
	MarketCapMillion   float64
	Beta               float64
	Week52High         float64
	Week52Low          float64
	BookValuePerShare  float64
}

// FinancialStatement holds the key line items from a single annual (10-K) or
// quarterly (10-Q) SEC filing, as reported — not restated/normalized.
type FinancialStatement struct {
	Ticker            string
	FiscalYear        int
	Form              string // "10-K" or "10-Q"
	PeriodEnd         string // "2025-09-27"
	Revenue           float64
	GrossProfit       float64
	OperatingIncome   float64
	NetIncome         float64
	DilutedEPS        float64
	TotalAssets       float64
	TotalLiabilities  float64
	TotalEquity       float64
	OperatingCashFlow float64
	CapEx             float64
	FreeCashFlow      float64
}

// FundamentalsProvider is implemented only by Finnhub. Unlike Provider,
// there's no Yahoo fallback to wrap this in a Multi for — Yahoo's
// fundamentals endpoint is blocked without a crumb/cookie handshake we
// deliberately don't implement (fragile, unofficial API, easy to break).
type FundamentalsProvider interface {
	GetFundamentals(ticker string) (*Fundamentals, error)
	GetFinancialStatements(ticker, freq string) (*FinancialStatement, error)
}

func (f *Finnhub) GetFundamentals(ticker string) (*Fundamentals, error) {
	if market.Of(ticker) == market.TW {
		return nil, errTWNotSupported
	}
	var result struct {
		Metric map[string]any `json:"metric"`
	}
	if err := f.get(fmt.Sprintf("/stock/metric?symbol=%s&metric=all", ticker), &result); err != nil {
		return nil, err
	}
	if len(result.Metric) == 0 {
		return nil, fmt.Errorf("finnhub: no fundamentals for %s", ticker)
	}

	get := func(keys ...string) float64 {
		for _, k := range keys {
			if v, ok := result.Metric[k].(float64); ok {
				return v
			}
		}
		return 0
	}

	return &Fundamentals{
		Ticker:             ticker,
		PE:                 get("peBasicExclExtraTTM", "peNormalizedAnnual"),
		PB:                 get("pbAnnual", "pbQuarterly"),
		PS:                 get("psTTM", "psAnnual"),
		ROE:                get("roeTTM", "roeRfy"),
		ROA:                get("roaTTM", "roaRfy"),
		GrossMarginPct:     get("grossMarginTTM"),
		OperatingMarginPct: get("operatingMarginTTM"),
		NetMarginPct:       get("netProfitMarginTTM"),
		DebtToEquity:       get("totalDebt/totalEquityAnnual", "totalDebt/totalEquityQuarterly"),
		CurrentRatio:       get("currentRatioAnnual"),
		QuickRatio:         get("quickRatioAnnual"),
		RevenueGrowthYoY:   get("revenueGrowthTTMYoy"),
		EPSGrowthYoY:       get("epsGrowthTTMYoy"),
		EPS:                get("epsTTM"),
		DividendYieldPct:   get("dividendYieldIndicatedAnnual"),
		MarketCapMillion:   get("marketCapitalization"),
		Beta:               get("beta"),
		Week52High:         get("52WeekHigh"),
		Week52Low:          get("52WeekLow"),
		BookValuePerShare:  get("bookValuePerShareAnnual"),
	}, nil
}

type finnhubReportLine struct {
	Concept string  `json:"concept"`
	Label   string  `json:"label"`
	Value   float64 `json:"value"`
}

// GetFinancialStatements returns the most recent filing's key line items.
// freq is "annual" (10-K) or "quarterly" (10-Q).
func (f *Finnhub) GetFinancialStatements(ticker, freq string) (*FinancialStatement, error) {
	if market.Of(ticker) == market.TW {
		return nil, errTWNotSupported
	}
	var result struct {
		Data []struct {
			Year    int    `json:"year"`
			Form    string `json:"form"`
			EndDate string `json:"endDate"`
			Report  struct {
				IC []finnhubReportLine `json:"ic"`
				BS []finnhubReportLine `json:"bs"`
				CF []finnhubReportLine `json:"cf"`
			} `json:"report"`
		} `json:"data"`
	}
	if err := f.get(fmt.Sprintf("/stock/financials-reported?symbol=%s&freq=%s", ticker, freq), &result); err != nil {
		return nil, err
	}
	if len(result.Data) == 0 {
		return nil, fmt.Errorf("finnhub: no financial statements for %s", ticker)
	}

	// Finnhub returns filings most-recent-first.
	d := result.Data[0]
	r := d.Report

	// XBRL concept names vary by filer/period even for the same conceptual
	// line item, so each lookup tries a short list of known aliases and
	// takes the first match.
	find := func(lines []finnhubReportLine, concepts ...string) float64 {
		for _, c := range concepts {
			for _, l := range lines {
				if l.Concept == c {
					return l.Value
				}
			}
		}
		return 0
	}

	operatingCashFlow := find(r.CF, "us-gaap_NetCashProvidedByUsedInOperatingActivities", "us-gaap_NetCashProvidedByUsedInOperatingActivitiesContinuingOperations")
	capex := find(r.CF, "us-gaap_PaymentsToAcquirePropertyPlantAndEquipment")

	return &FinancialStatement{
		Ticker:     ticker,
		FiscalYear: d.Year,
		Form:       d.Form,
		PeriodEnd:  periodEndDate(d.EndDate),
		Revenue: find(r.IC,
			"us-gaap_RevenueFromContractWithCustomerExcludingAssessedTax",
			"us-gaap_RevenueFromContractWithCustomerIncludingAssessedTax",
			"us-gaap_Revenues",
			"us-gaap_SalesRevenueNet",
		),
		GrossProfit:      find(r.IC, "us-gaap_GrossProfit"),
		OperatingIncome:  find(r.IC, "us-gaap_OperatingIncomeLoss"),
		NetIncome:        find(r.IC, "us-gaap_NetIncomeLoss"),
		DilutedEPS:       find(r.IC, "us-gaap_EarningsPerShareDiluted", "us-gaap_EarningsPerShareBasicAndDiluted", "us-gaap_EarningsPerShareBasic"),
		TotalAssets:      find(r.BS, "us-gaap_Assets"),
		TotalLiabilities: find(r.BS, "us-gaap_Liabilities"),
		TotalEquity: find(r.BS,
			"us-gaap_StockholdersEquity",
			"us-gaap_StockholdersEquityIncludingPortionAttributableToNoncontrollingInterest",
		),
		OperatingCashFlow: operatingCashFlow,
		CapEx:             capex,
		FreeCashFlow:      operatingCashFlow - capex,
	}, nil
}

// periodEndDate trims Finnhub's "2025-09-27 00:00:00" down to the date part.
func periodEndDate(s string) string {
	if i := len(s); i >= 10 {
		return s[:10]
	}
	return s
}
