package web

import (
	"net/http"
	"sort"
	"time"

	"argus/internal/db"
	"argus/internal/logger"
	"argus/internal/service"
)

// fundRow backs one row of /w/funds' 扣款標的與績效 table. Cost/Value/Pnl/
// ReturnPct are nil when this fund's currency couldn't be priced to TWD for
// AsOf (same per-item degrade InsuranceCoverageRow.Need uses) — a single
// unpriceable holding shouldn't blank the whole table, unlike the KPI
// aggregate below which requires every fund to price. OneYearReturnPct is
// separately nil whenever there's no snapshot ~1 year back yet (a fund added
// recently), independent of whether today's price resolved.
type fundRow struct {
	AssetID              int64    `json:"assetId"`
	Name                 string   `json:"name"`
	Code                 string   `json:"code"`
	Platform             string   `json:"platform"`
	MonthlyAmount        *float64 `json:"monthlyAmount"` // nil/0 = stopped (lump-sum-only holding)
	Cost                 *float64 `json:"cost"`
	Value                *float64 `json:"value"`
	PnL                  *float64 `json:"pnl"`
	ReturnPct            *float64 `json:"returnPct"`
	OneYearReturnPct     *float64 `json:"oneYearReturnPct"`
	NextContributionDate string   `json:"nextContributionDate,omitempty"`
}

// fundScheduleItem is one 本月扣款排程 row — every active fund sharing the
// same NextContributionDate collapses into one row, names joined, amounts
// summed (matches the design template's schedule mock, which does the same
// grouping for its two same-date rows).
type fundScheduleItem struct {
	Date   string  `json:"date"`
	Names  string  `json:"names"`
	Amount float64 `json:"amount"`
}

// fundChartPoint is one point of the 24-month 累積投入 vs 市值 chart. Cost/
// Value are nil when no fund had a priceable snapshot on or before Date yet
// (e.g. every fund was added after that month) — the frontend skips a nil
// point rather than drawing it as zero.
type fundChartPoint struct {
	Date  string   `json:"date"`
	Cost  *float64 `json:"cost"`
	Value *float64 `json:"value"`
}

// wealthFundsResponse backs GET /api/wealth/funds. MarketValue/Cost/PnL/
// ReturnPct are the KPI row's four glow cards, all degrading together to nil
// the moment any fund's currency can't be priced to TWD for today (§8.17.1's
// whole-metric-degrades rule) — same as service.WealthTotals, and MarketValue
// isn't split out as an exception here either: a partial MV sum that quietly
// dropped one unpriced holding would be exactly the fabricated-partial-number
// failure mode §8.17.1 exists to prevent. MonthlyTotal doesn't need this —
// it's summed straight from fund_details.monthly_amount, no FX involved.
type wealthFundsResponse struct {
	AsOf         string             `json:"asOf"`
	MarketValue  *float64           `json:"marketValue"`
	Cost         *float64           `json:"cost"`
	PnL          *float64           `json:"pnl"`
	ReturnPct    *float64           `json:"returnPct"`
	MonthlyTotal float64            `json:"monthlyTotal"`
	Chart        []fundChartPoint   `json:"chart"`
	Schedule     []fundScheduleItem `json:"schedule"`
	Rows         []fundRow          `json:"rows"`
}

// fundsChartDates returns the 24 month-end dates (oldest first) the 累積投入
// vs 市值 chart plots — the last entry is "today" rather than the current
// month's actual last day, since that day hasn't happened yet.
func fundsChartDates(today time.Time) []string {
	dates := make([]string, 24)
	for i := 0; i < 24; i++ {
		monthsAgo := 23 - i
		y, mo, _ := today.Date()
		firstOfTargetMonth := time.Date(y, mo, 1, 0, 0, 0, 0, today.Location()).AddDate(0, -monthsAgo, 0)
		lastDay := firstOfTargetMonth.AddDate(0, 1, 0).AddDate(0, 0, -1)
		if monthsAgo == 0 || lastDay.After(today) {
			lastDay = today
		}
		dates[i] = lastDay.Format("2006-01-02")
	}
	return dates
}

// fundTotalsTWD sums a set of fund-type AssetWithValue rows to TWD as of
// date. ok=false means at least one fund's currency couldn't be priced —
// the caller must degrade cost/pnl/return to nil rather than sum only what
// resolved (same rule service.sumEntriesTWD enforces for the net-worth
// totals).
func fundTotalsTWD(funds []db.AssetWithValue, date string, live bool, quotes service.QuoteReader, fx service.WealthFXStore) (value, cost float64, ok bool) {
	for _, a := range funds {
		if a.Value == nil {
			continue
		}
		rate, rok := service.RateToTWD(a.Currency, date, live, quotes, fx)
		if !rok {
			return 0, 0, false
		}
		value += *a.Value * rate
		if a.Cost != nil {
			cost += *a.Cost * rate
		}
	}
	return value, cost, true
}

func (s *Server) handleWealthFundsGet(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if p := recover(); p != nil {
			logger.Errorf("web: panic in handleWealthFundsGet: %v", p)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
	}()

	today := time.Now().Format("2006-01-02")
	resp := wealthFundsResponse{AsOf: today, Chart: []fundChartPoint{}, Schedule: []fundScheduleItem{}, Rows: []fundRow{}}

	assetList, err := s.db.ListAssetsWithValue(false)
	if err != nil {
		logger.Errorf("web: wealth funds: list assets: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load")
		return
	}
	var funds []db.AssetWithValue
	for _, a := range assetList {
		if a.Type == "fund" {
			funds = append(funds, a)
		}
	}

	// KPI row: market value/cost/pnl/return all degrade together the moment
	// any fund's currency can't be priced (see wealthFundsResponse's doc
	// comment on why MarketValue isn't split out as a partial-sum exception).
	if mv, cost, fxOK := fundTotalsTWD(funds, today, true, s.quotes, s.fxDB); fxOK {
		v, c := mv, cost
		resp.MarketValue, resp.Cost = &v, &c
		pnl := mv - cost
		resp.PnL = &pnl
		if cost > 0 {
			ret := pnl / cost * 100
			resp.ReturnPct = &ret
		}
	}

	oneYearAgo := time.Now().AddDate(-1, 0, 0).Format("2006-01-02")
	oneYearAgoAssets, err := s.db.ListAssetsValueAsOf(oneYearAgo, false)
	if err != nil {
		logger.Errorf("web: wealth funds: list assets as of %s: %v", oneYearAgo, err)
		writeError(w, http.StatusInternalServerError, "failed to load")
		return
	}
	oneYearAgoByID := make(map[int64]db.AssetWithValue, len(oneYearAgoAssets))
	for _, a := range oneYearAgoAssets {
		oneYearAgoByID[a.ID] = a
	}

	for _, a := range funds {
		det, derr := s.db.GetFundDetails(a.ID)
		if derr != nil {
			logger.Errorf("web: wealth funds: get details %d: %v", a.ID, derr)
			writeError(w, http.StatusInternalServerError, "failed to load")
			return
		}
		row := fundRow{AssetID: a.ID, Name: a.Name}
		if det != nil {
			row.Code, row.Platform, row.MonthlyAmount, row.NextContributionDate =
				det.Code, det.Platform, det.MonthlyAmount, det.NextContributionDate
		}

		if a.Value != nil {
			if rate, rok := service.RateToTWD(a.Currency, today, true, s.quotes, s.fxDB); rok {
				v := *a.Value * rate
				row.Value = &v
				if a.Cost != nil {
					c := *a.Cost * rate
					row.Cost = &c
					pnl := v - c
					row.PnL = &pnl
					if c > 0 {
						ret := pnl / c * 100
						row.ReturnPct = &ret
					}
				}
			}
		}

		if prior, ok := oneYearAgoByID[a.ID]; ok && prior.Value != nil && *prior.Value > 0 && a.Value != nil {
			if rate, rok := service.RateToTWD(a.Currency, today, true, s.quotes, s.fxDB); rok {
				priorRate, prok := service.RateToTWD(prior.Currency, oneYearAgo, false, s.quotes, s.fxDB)
				if prok {
					nowTWD := *a.Value * rate
					priorTWD := *prior.Value * priorRate
					if priorTWD > 0 {
						pct := (nowTWD - priorTWD) / priorTWD * 100
						row.OneYearReturnPct = &pct
					}
				}
			}
		}

		resp.Rows = append(resp.Rows, row)

		if row.MonthlyAmount != nil && *row.MonthlyAmount > 0 {
			resp.MonthlyTotal += *row.MonthlyAmount
			if row.NextContributionDate != "" {
				resp.Schedule = append(resp.Schedule, fundScheduleItem{
					Date: row.NextContributionDate, Names: row.Name, Amount: *row.MonthlyAmount,
				})
			}
		}
	}
	resp.Schedule = mergeFundSchedule(resp.Schedule)

	// 24-month chart: reuse ListAssetsValueAsOf (already the codebase's one
	// point-in-time asset query) rather than a new bulk-history query —
	// 24 lightweight lookups on a single-user DB is not worth a second query
	// shape.
	for _, date := range fundsChartDates(time.Now()) {
		pointAssets, perr := s.db.ListAssetsValueAsOf(date, false)
		if perr != nil {
			logger.Errorf("web: wealth funds: chart list assets as of %s: %v", date, perr)
			writeError(w, http.StatusInternalServerError, "failed to load")
			return
		}
		var pointFunds []db.AssetWithValue
		for _, a := range pointAssets {
			if a.Type == "fund" {
				pointFunds = append(pointFunds, a)
			}
		}
		point := fundChartPoint{Date: date}
		if v, c, ok := fundTotalsTWD(pointFunds, date, false, s.quotes, s.fxDB); ok && len(pointFunds) > 0 {
			point.Value, point.Cost = &v, &c
		}
		resp.Chart = append(resp.Chart, point)
	}

	writeJSON(w, http.StatusOK, resp)
}

// mergeFundSchedule collapses same-date entries into one row (names joined,
// amounts summed) and sorts ascending by date — mirrors the design
// template's schedule mock, which shows one row per date, not per fund.
func mergeFundSchedule(items []fundScheduleItem) []fundScheduleItem {
	byDate := make(map[string]*fundScheduleItem)
	var order []string
	for _, it := range items {
		if existing, ok := byDate[it.Date]; ok {
			existing.Names += " · " + it.Names
			existing.Amount += it.Amount
			continue
		}
		cp := it
		byDate[it.Date] = &cp
		order = append(order, it.Date)
	}
	sort.Strings(order)
	out := make([]fundScheduleItem, 0, len(order))
	for _, d := range order {
		out = append(out, *byDate[d])
	}
	return out
}
