package web

import (
	"net/http"
	"strconv"
	"time"

	"argus/internal/assets"
	"argus/internal/db"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/service"
)

// annualSalarySettingKey is profile.annual_salary (§9.3) — re-exported from
// service.AnnualSalarySettingKey for local brevity.
const annualSalarySettingKey = service.AnnualSalarySettingKey

// birthYearSettingKey/retirementContribSettingKey back the retirement page's
// two extra "個人參數" (Phase 9 波次3 PR7) — see service.BirthYearSettingKey's
// doc comment.
const (
	birthYearSettingKey         = service.BirthYearSettingKey
	retirementContribSettingKey = service.RetirementMonthlyContributionSettingKey
)

type balanceSheetItem struct {
	AssetID  int64   `json:"assetId,omitempty"` // 0 for the equity virtual row
	Name     string  `json:"name"`
	Venue    string  `json:"venue,omitempty"`
	Currency string  `json:"currency"`
	ValueTWD float64 `json:"valueTwd"`
	Type     string  `json:"type"`   // the asset's own Type, or "equity_us"/"equity_tw" for the virtual row
	Source   string  `json:"source"` // "manual"/"import"/"sync" (§9.1); "sync" for the equity virtual row
}

type assetGroupRow struct {
	Group       string             `json:"group"`
	MarketValue float64            `json:"marketValue"`
	PctOfAssets *float64           `json:"pctOfAssets"`
	Assets      []balanceSheetItem `json:"assets"`
}

type liabilityDetail struct {
	AssetID         int64    `json:"assetId"`
	Name            string   `json:"name"`
	Venue           string   `json:"venue,omitempty"`
	Currency        string   `json:"currency"`
	ValueTWD        float64  `json:"valueTwd"`
	RatePct         *float64 `json:"ratePct,omitempty"`
	RemainingMonths *int64   `json:"remainingMonths,omitempty"`
	MinPayment      *float64 `json:"minPayment,omitempty"`
	Source          string   `json:"source"` // "manual"/"import"/"sync" (§9.1)
}

type quarterPoint struct {
	Quarter  string   `json:"quarter"`
	NetWorth *float64 `json:"netWorth"`
}

// balanceSheetResponse backs GET /api/wealth/balance (`/w/balance`, §8.3-3).
// Editing stays on `/w`'s asset list (already built in the home-page
// checkpoint) — this page is a read-only summary, so it doesn't duplicate
// the quick-add/in-place-edit/archive UI.
type balanceSheetResponse struct {
	AsOf             string            `json:"asOf"`
	TotalAssets      *float64          `json:"totalAssets"`
	TotalLiabilities *float64          `json:"totalLiabilities"`
	NetWorth         *float64          `json:"netWorth"`
	DebtRatioPct     *float64          `json:"debtRatioPct"`
	LiquidityMonths  *float64          `json:"liquidityMonths"`
	SavingsRatePct   *float64          `json:"savingsRatePct"`
	ExpenseRatioPct  *float64          `json:"expenseRatioPct"`
	MonthlySalary    *float64          `json:"monthlySalary"`
	AssetGroups      []assetGroupRow   `json:"assetGroups"`
	Liabilities      []liabilityDetail `json:"liabilities"`
	QuarterlyTrend   []quarterPoint    `json:"quarterlyTrend"`
}

// buildAssetGroups assembles the four asset_group buckets' market values
// (in TWD) and per-asset breakdown, plus the running grand total and a
// per-currency total — the shared FX-conversion/grouping core of both
// /w/balance and /w/alloc (Phase 9 PR4), so it exists in exactly one place.
// Liabilities are the caller's own loop (only /w/balance needs them).
// fxOK=false means at least one asset's currency couldn't be priced for
// today — same whole-metric-degrades rule as service.WealthTotals.
func (s *Server) buildAssetGroups(list []db.AssetWithValue, today string, usTotal float64, usOK bool, twTotal float64, twOK bool) (groups map[string]*assetGroupRow, byCurrency map[string]float64, totalAssets float64, fxOK bool) {
	groups = make(map[string]*assetGroupRow, len(assets.AssetGroups))
	for _, g := range assets.AssetGroups {
		groups[g] = &assetGroupRow{Group: g, Assets: []balanceSheetItem{}}
	}
	byCurrency = make(map[string]float64)
	fxOK = true

	for _, a := range list {
		if a.Value == nil || a.Side == "liability" {
			continue
		}
		rate, rok := service.RateToTWD(a.Currency, today, true, s.quotes, s.fxDB)
		if !rok {
			fxOK = false
			continue
		}
		valueTWD := *a.Value * rate
		totalAssets += valueTWD
		byCurrency[a.Currency] += valueTWD
		g := groups[a.AssetGroup]
		if g == nil {
			continue
		}
		g.MarketValue += valueTWD
		g.Assets = append(g.Assets, balanceSheetItem{AssetID: a.ID, Name: a.Name, Venue: a.Venue, Currency: a.Currency, ValueTWD: valueTWD, Type: a.Type, Source: a.Source})
	}
	for _, e := range service.EquityEntries(usTotal, usOK, twTotal, twOK) {
		rate, rok := service.RateToTWD(e.Currency, today, true, s.quotes, s.fxDB)
		if !rok {
			fxOK = false
			continue
		}
		valueTWD := e.Value * rate
		totalAssets += valueTWD
		byCurrency[e.Currency] += valueTWD
		g := groups["growth"]
		typ := "equity_tw"
		if e.Currency != "TWD" {
			typ = "equity_us"
		}
		g.MarketValue += valueTWD
		g.Assets = append(g.Assets, balanceSheetItem{Name: typ, Currency: e.Currency, ValueTWD: valueTWD, Type: typ, Source: "sync"})
	}
	return groups, byCurrency, totalAssets, fxOK
}

const quarterlyTrendQuarters = 8

// quarterlyDates returns n (year, quarter) points in chronological order,
// ending at today's quarter. The most recent point's date is today itself
// (that quarter isn't over yet); every earlier point is its quarter's last
// calendar day.
func quarterlyDates(now time.Time, n int) []struct {
	Label string
	Date  string
} {
	y, m, _ := now.Date()
	curQ := (int(m)-1)/3 + 1
	type yq struct{ y, q int }
	points := make([]yq, n)
	yy, qq := y, curQ
	for i := n - 1; i >= 0; i-- {
		points[i] = yq{yy, qq}
		qq--
		if qq == 0 {
			qq = 4
			yy--
		}
	}
	out := make([]struct {
		Label string
		Date  string
	}, n)
	for i, p := range points {
		label := strconv.Itoa(p.y) + "-Q" + strconv.Itoa(p.q)
		var date string
		if p.y == y && p.q == curQ {
			date = now.Format("2006-01-02")
		} else {
			lastMonth := time.Month(p.q*3 + 1)
			date = time.Date(p.y, lastMonth, 1, 0, 0, 0, 0, now.Location()).AddDate(0, 0, -1).Format("2006-01-02")
		}
		out[i] = struct {
			Label string
			Date  string
		}{label, date}
	}
	return out
}

// handleWealthBalance backs GET /api/wealth/balance — ungated like every
// other read route.
func (s *Server) handleWealthBalance(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if p := recover(); p != nil {
			logger.Errorf("web: panic in handleWealthBalance: %v", p)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
	}()

	now := time.Now()
	today := now.Format("2006-01-02")

	list, err := s.db.ListAssetsWithValue(false)
	if err != nil {
		logger.Errorf("web: wealth balance: list assets: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load assets")
		return
	}
	usTotal, usOK, err := s.db.GetNetWorthOnOrBefore(today, market.US)
	if err != nil {
		logger.Errorf("web: wealth balance: US net worth: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load net worth")
		return
	}
	twTotal, twOK, err := s.db.GetNetWorthOnOrBefore(today, market.TW)
	if err != nil {
		logger.Errorf("web: wealth balance: TW net worth: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load net worth")
		return
	}

	var totalLiabilities float64
	var liabilities []liabilityDetail
	liabFxOK := true
	for _, a := range list {
		if a.Value == nil || a.Side != "liability" {
			continue
		}
		rate, rok := service.RateToTWD(a.Currency, today, true, s.quotes, s.fxDB)
		if !rok {
			liabFxOK = false
			continue
		}
		valueTWD := *a.Value * rate
		totalLiabilities += valueTWD
		ld := liabilityDetail{AssetID: a.ID, Name: a.Name, Venue: a.Venue, Currency: a.Currency, ValueTWD: valueTWD, Source: a.Source}
		if assets.LoanTypes[a.Type] {
			if det, err := s.db.GetLoanDetails(a.ID); err == nil && det != nil {
				ld.RatePct = det.RatePct
				ld.RemainingMonths = det.RemainingMonths
				if det.RatePct != nil && det.RemainingMonths != nil {
					if mp, ok := assets.AmortizedMinPayment(*a.Value, *det.RatePct, int(*det.RemainingMonths)); ok {
						ld.MinPayment = &mp
					}
				}
			} else if err != nil {
				logger.Errorf("web: wealth balance: loan details for asset %d: %v", a.ID, err)
			}
		}
		liabilities = append(liabilities, ld)
	}

	groups, _, totalAssets, groupsFxOK := s.buildAssetGroups(list, today, usTotal, usOK, twTotal, twOK)
	fxOK := liabFxOK && groupsFxOK

	resp := balanceSheetResponse{AsOf: today, Liabilities: liabilities}
	if fxOK {
		ta, tl := totalAssets, totalLiabilities
		resp.TotalAssets = &ta
		resp.TotalLiabilities = &tl
		nw := ta - tl
		resp.NetWorth = &nw
		if dr, ok := assets.DebtRatio(ta, tl); ok {
			resp.DebtRatioPct = &dr
		}
	}
	for _, g := range assets.AssetGroups {
		row := groups[g]
		if fxOK && totalAssets > 0 {
			pct := row.MarketValue / totalAssets * 100
			row.PctOfAssets = &pct
		}
		resp.AssetGroups = append(resp.AssetGroups, *row)
	}

	if fxOK {
		hm := service.ComputeHealthMetrics(s.db, s.fxDB, s.quotes, now)
		resp.MonthlySalary = hm.MonthlySalary
		resp.SavingsRatePct = hm.SavingsRatePct
		resp.ExpenseRatioPct = hm.ExpenseRatioPct
		resp.LiquidityMonths = hm.LiquidityMonths
	}

	for _, qd := range quarterlyDates(now, quarterlyTrendQuarters) {
		point := quarterPoint{Quarter: qd.Label}
		if total, _, ok := service.WealthTotals(s.db, s.fxDB, s.quotes, qd.Date, false); ok {
			nw := total
			point.NetWorth = &nw
		}
		resp.QuarterlyTrend = append(resp.QuarterlyTrend, point)
	}

	writeJSON(w, http.StatusOK, resp)
}

// wealthProfileResponse/handleWealthProfileGet+Update back GET/POST
// /api/wealth/profile — the single settings field (profile.annual_salary)
// the health metrics need, per §9.3. There's no dedicated "個人參數"
// settings page yet (that's PR8/PR13 scope) — this is the minimal write
// path so the balance-sheet page isn't permanently stuck at "—".
type wealthProfileResponse struct {
	AnnualSalary                  *float64 `json:"annualSalary"`
	BirthYear                     *int     `json:"birthYear"`
	RetirementMonthlyContribution *float64 `json:"retirementMonthlyContribution"`
}

func (s *Server) handleWealthProfileGet(w http.ResponseWriter, r *http.Request) {
	resp := wealthProfileResponse{}
	if raw, ok, err := s.db.GetSetting(annualSalarySettingKey); err == nil && ok {
		if v, perr := strconv.ParseFloat(raw, 64); perr == nil {
			resp.AnnualSalary = &v
		}
	}
	if raw, ok, err := s.db.GetSetting(birthYearSettingKey); err == nil && ok {
		if v, perr := strconv.Atoi(raw); perr == nil {
			resp.BirthYear = &v
		}
	}
	if raw, ok, err := s.db.GetSetting(retirementContribSettingKey); err == nil && ok {
		if v, perr := strconv.ParseFloat(raw, 64); perr == nil {
			resp.RetirementMonthlyContribution = &v
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// wealthProfileUpdateRequest fields are pointers so a caller can update just
// one of the three settings without clobbering the others — the balance
// sheet's salary form and the retirement page's birth-year/contribution form
// both post to this same endpoint independently.
type wealthProfileUpdateRequest struct {
	AnnualSalary                  *float64 `json:"annualSalary"`
	BirthYear                     *int     `json:"birthYear"`
	RetirementMonthlyContribution *float64 `json:"retirementMonthlyContribution"`
}

func (s *Server) handleWealthProfileUpdate(w http.ResponseWriter, r *http.Request) {
	var req wealthProfileUpdateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.AnnualSalary != nil {
		if *req.AnnualSalary < 0 {
			writeError(w, http.StatusBadRequest, "annualSalary must be >= 0")
			return
		}
		if err := s.wealthDB.SetSetting(annualSalarySettingKey, strconv.FormatFloat(*req.AnnualSalary, 'f', 2, 64)); err != nil {
			logger.Errorf("web: set %s: %v", annualSalarySettingKey, err)
			writeError(w, http.StatusInternalServerError, "failed to save")
			return
		}
	}
	if req.BirthYear != nil {
		if *req.BirthYear < 1900 || *req.BirthYear > time.Now().Year() {
			writeError(w, http.StatusBadRequest, "birthYear is out of range")
			return
		}
		if err := s.wealthDB.SetSetting(birthYearSettingKey, strconv.Itoa(*req.BirthYear)); err != nil {
			logger.Errorf("web: set %s: %v", birthYearSettingKey, err)
			writeError(w, http.StatusInternalServerError, "failed to save")
			return
		}
	}
	if req.RetirementMonthlyContribution != nil {
		if *req.RetirementMonthlyContribution < 0 {
			writeError(w, http.StatusBadRequest, "retirementMonthlyContribution must be >= 0")
			return
		}
		if err := s.wealthDB.SetSetting(retirementContribSettingKey, strconv.FormatFloat(*req.RetirementMonthlyContribution, 'f', 2, 64)); err != nil {
			logger.Errorf("web: set %s: %v", retirementContribSettingKey, err)
			writeError(w, http.StatusInternalServerError, "failed to save")
			return
		}
	}
	writeJSON(w, http.StatusOK, tradeResponse{Message: "saved"})
}

// debtPayoffResponse backs GET /api/wealth/debt-payoff?extra=NNNN — the
// snowball/avalanche comparison (§10.2①). Loans with no usable rate/term
// (nil RatePct or RemainingMonths) are silently excluded rather than
// guessed at, same "don't fabricate" rule as everywhere else in this
// package.
type debtPayoffResponse struct {
	Loans              []string       `json:"loans"` // names actually included in the simulation
	Snowball           debtPayoffPlan `json:"snowball"`
	Avalanche          debtPayoffPlan `json:"avalanche"`
	InterestDifference float64        `json:"interestDifference"` // snowball - avalanche, always >= 0
}

type debtPayoffPlan struct {
	Order         []string `json:"order"`
	Months        int      `json:"months"`
	TotalInterest float64  `json:"totalInterest"`
	MonthsSaved   int      `json:"monthsSaved"`
}

func (s *Server) handleWealthDebtPayoff(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if p := recover(); p != nil {
			logger.Errorf("web: panic in handleWealthDebtPayoff: %v", p)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
	}()

	extra, _ := strconv.ParseFloat(r.URL.Query().Get("extra"), 64)
	if extra < 0 {
		extra = 0
	}

	list, err := s.db.ListAssetsWithValue(false)
	if err != nil {
		logger.Errorf("web: debt payoff: list assets: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load assets")
		return
	}

	var loans []assets.Loan
	var names []string
	for _, a := range list {
		if a.Side != "liability" || !assets.LoanTypes[a.Type] || a.Value == nil {
			continue
		}
		det, err := s.db.GetLoanDetails(a.ID)
		if err != nil {
			logger.Errorf("web: debt payoff: loan details for asset %d: %v", a.ID, err)
			continue
		}
		if det == nil || det.RatePct == nil || det.RemainingMonths == nil {
			continue
		}
		minPay, ok := assets.AmortizedMinPayment(*a.Value, *det.RatePct, int(*det.RemainingMonths))
		if !ok {
			continue
		}
		loans = append(loans, assets.Loan{Name: a.Name, Balance: *a.Value, RatePct: *det.RatePct, MinPayment: minPay})
		names = append(names, a.Name)
	}

	snowball, avalanche := assets.ComputeDebtPayoff(loans, extra)
	diff := snowball.TotalInterest - avalanche.TotalInterest
	if diff < 0 {
		diff = 0
	}
	writeJSON(w, http.StatusOK, debtPayoffResponse{
		Loans:              names,
		Snowball:           debtPayoffPlan(snowball),
		Avalanche:          debtPayoffPlan(avalanche),
		InterestDifference: diff,
	})
}
