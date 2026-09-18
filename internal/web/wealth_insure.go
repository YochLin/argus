package web

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"argus/internal/assets"
	"argus/internal/db"
	"argus/internal/logger"
	"argus/internal/service"
)

// dependentsSettingKey/youngestChildAgeSettingKey/spouseIncomeSettingKey are
// the insurance page's three "個人參數" (§8.16.1/§8.16.2, Phase 9 波次3 PR8) —
// re-exported from service for local brevity, same convention as
// annualSalarySettingKey/birthYearSettingKey above.
const (
	dependentsSettingKey       = service.DependentsSettingKey
	youngestChildAgeSettingKey = service.YoungestChildAgeSettingKey
	spouseIncomeSettingKey     = service.SpouseHasIncomeSettingKey
)

// insuranceKindOrder/insuranceKindPerPeriod fix the six coverage kinds' rows
// and their unit (§8.6's "月給付/日額型保障不能跟一次金加總" — per_period is
// derived from kind server-side, never trusted from the client, so a
// coverage can never be filed under the wrong unit and silently corrupt the
// have-side aggregate). Order matches the design template's cov[] array
// (Argus Trading WebUI.dc.html insureModel(), lines 4288-4295).
var insuranceKindOrder = []string{"life", "accident", "ci", "cancer", "disability", "hospital"}

var insuranceKindPerPeriod = map[string]string{
	"life": "", "accident": "", "ci": "", "cancer": "",
	"disability": "month", "hospital": "day",
}

type insuranceCoverageRow struct {
	Kind      string   `json:"kind"`
	PerPeriod string   `json:"perPeriod,omitempty"` // "" | "month" | "day"
	Have      float64  `json:"have"`
	Need      *float64 `json:"need"`      // nil when the inputs behind it aren't available yet
	Gap       *float64 `json:"gap"`       // max(0, need-have); nil follows Need
	PctOfNeed *float64 `json:"pctOfNeed"` // have/need*100 (100 when need<=0 — trivially covered); nil follows Need
}

type insurancePolicyItem struct {
	AssetID       int64    `json:"assetId"`
	Name          string   `json:"name"`
	Insurer       string   `json:"insurer"`
	Kind          string   `json:"kind"`
	PerPeriod     string   `json:"perPeriod,omitempty"`
	Amount        float64  `json:"amount"`
	AnnualPremium *float64 `json:"annualPremium"`
	PremiumYears  *int64   `json:"premiumYears"`
	Insured       string   `json:"insured,omitempty"`
	Source        string   `json:"source"`
}

// wealthInsureResponse backs GET /api/wealth/insure. HasProfile mirrors the
// retirement page's HasBirthYear convention — false shows a mini setup card
// instead of the life/accident/disability need figures (§8.16.1's two
// "asked once" inputs); the constant-only kinds (ci/cancer/hospital) stay
// populated regardless, since they don't depend on the profile at all.
type wealthInsureResponse struct {
	AsOf            string                 `json:"asOf"`
	HasProfile      bool                   `json:"hasProfile"`
	Rows            []insuranceCoverageRow `json:"rows"`
	Policies        []insurancePolicyItem  `json:"policies"`
	Count           int                    `json:"count"`
	Premium         float64                `json:"premium"`
	PremSharePct    *float64               `json:"premSharePct"`
	WorstKind       string                 `json:"worstKind,omitempty"`
	WorstGap        float64                `json:"worstGap"`
	WorstPct        float64                `json:"worstPct"`
	TotalGapLumpSum float64                `json:"totalGapLumpSum"`
}

func (s *Server) handleWealthInsureGet(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if p := recover(); p != nil {
			logger.Errorf("web: panic in handleWealthInsureGet: %v", p)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
	}()

	today := time.Now().Format("2006-01-02")
	resp := wealthInsureResponse{AsOf: today, Rows: []insuranceCoverageRow{}, Policies: []insurancePolicyItem{}}

	// Need-side inputs (§8.16.1's "平台算得出來" bucket) — each degrades
	// independently, same whole-metric rule as the rest of wealth/.
	var totalDebt float64
	var debtOK bool
	if _, tl, ok := service.AssetLiabilityTotals(s.db, s.fxDB, s.quotes, today, true); ok {
		totalDebt, debtOK = tl, true
	}
	var liquidAssets float64
	var liquidOK bool
	if _, byGroup, ok := service.WealthTotals(s.db, s.fxDB, s.quotes, today, true); ok {
		liquidAssets, liquidOK = byGroup["liquid"], true
	}
	cashflows, err := s.db.ListRecurringCashflows(true)
	if err != nil {
		logger.Errorf("web: wealth insure: list cashflows: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load")
		return
	}
	var annualExpense float64
	var expenseOK bool
	if monthlyOut, ok := service.MonthlyOutflowTWD(cashflows, today, s.quotes, s.fxDB); ok {
		annualExpense, expenseOK = monthlyOut*12, true
	}

	// Need-side "asked once" inputs (§8.16.2 profile) — Dependents==0 is a
	// real, explicitly-saved "no dependents," distinct from "not set yet"
	// (hasDependents false); see assets.InsuranceNeedInputs' doc comment.
	var dependents, youngestChildAge int
	var hasDependents, hasYoungestChildAge, hasSpouseSetting, spouseHasIncome bool
	if raw, ok, err := s.db.GetSetting(dependentsSettingKey); err == nil && ok {
		if v, perr := strconv.Atoi(raw); perr == nil {
			dependents, hasDependents = v, true
		}
	}
	if raw, ok, err := s.db.GetSetting(youngestChildAgeSettingKey); err == nil && ok {
		if v, perr := strconv.Atoi(raw); perr == nil {
			youngestChildAge, hasYoungestChildAge = v, true
		}
	}
	if raw, ok, err := s.db.GetSetting(spouseIncomeSettingKey); err == nil && ok {
		spouseHasIncome, hasSpouseSetting = raw == "1", true
	}
	// hasYoungestChildAge is only required when there actually are
	// dependents — dependencyYears() already ignores the age when
	// Dependents<=0, so don't block "no dependents" households on filling
	// in an age field that would be meaningless for them.
	resp.HasProfile = hasDependents && hasSpouseSetting && (dependents <= 0 || hasYoungestChildAge)
	needInputsOK := debtOK && liquidOK && expenseOK && resp.HasProfile

	in := assets.InsuranceNeedInputs{
		TotalDebt: totalDebt, AnnualExpense: annualExpense, LiquidAssets: liquidAssets,
		Dependents: dependents, YoungestChildAge: youngestChildAge, HasYoungestChildAge: hasYoungestChildAge,
		SpouseHasIncome: spouseHasIncome,
	}

	// Have-side: every coverage on a non-archived insurance-type asset,
	// summed by kind (§8.6: never across per_period, but insuranceKindOrder
	// already keeps kind 1:1 with per_period, so a plain per-kind sum can't
	// mix units).
	assetList, err := s.db.ListAssetsWithValue(false)
	if err != nil {
		logger.Errorf("web: wealth insure: list assets: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load")
		return
	}
	policyAssets := make(map[int64]db.AssetWithValue)
	for _, a := range assetList {
		if a.Type == "insurance" {
			policyAssets[a.ID] = a
		}
	}
	detailsByAsset := make(map[int64]*db.InsuranceDetails, len(policyAssets))
	for id := range policyAssets {
		det, derr := s.db.GetInsuranceDetails(id)
		if derr != nil {
			logger.Errorf("web: wealth insure: get details %d: %v", id, derr)
			writeError(w, http.StatusInternalServerError, "failed to load")
			return
		}
		detailsByAsset[id] = det
	}
	coverages, err := s.db.ListInsuranceCoverages()
	if err != nil {
		logger.Errorf("web: wealth insure: list coverages: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load")
		return
	}
	haveByKind := make(map[string]float64, len(insuranceKindOrder))
	for _, c := range coverages {
		a, ok := policyAssets[c.AssetID]
		if !ok {
			continue // archived policy, or an orphan row — excluded from both have and the policy list
		}
		haveByKind[c.Kind] += c.Amount
		item := insurancePolicyItem{
			AssetID: a.ID, Name: a.Name, Insurer: a.Venue, Kind: c.Kind, PerPeriod: c.PerPeriod,
			Amount: c.Amount, Source: a.Source,
		}
		if det := detailsByAsset[a.ID]; det != nil {
			item.AnnualPremium, item.PremiumYears, item.Insured = det.AnnualPremium, det.PremiumYears, det.Insured
		}
		resp.Policies = append(resp.Policies, item)
	}
	resp.Count = len(policyAssets)
	for _, det := range detailsByAsset {
		if det != nil && det.AnnualPremium != nil {
			resp.Premium += *det.AnnualPremium
		}
	}
	if raw, ok, err := s.db.GetSetting(annualSalarySettingKey); err == nil && ok {
		if salary, perr := strconv.ParseFloat(raw, 64); perr == nil && salary > 0 {
			pct := resp.Premium / salary * 100
			resp.PremSharePct = &pct
		}
	}

	needFor := func(kind string) (float64, bool) {
		switch kind {
		case "life":
			return assets.LifeInsuranceNeed(in), needInputsOK
		case "accident":
			return assets.AccidentInsuranceNeed(in), needInputsOK
		case "disability":
			return assets.DisabilityInsuranceNeed(in), needInputsOK
		case "ci":
			return assets.CriticalIllnessLumpSumTWD, true
		case "cancer":
			return assets.CancerLumpSumTWD, true
		case "hospital":
			return assets.HospitalDailyBaselineTWD, true
		}
		return 0, false
	}

	for _, kind := range insuranceKindOrder {
		row := insuranceCoverageRow{Kind: kind, PerPeriod: insuranceKindPerPeriod[kind], Have: haveByKind[kind]}
		if need, ok := needFor(kind); ok {
			n := need
			row.Need = &n
			gap := need - row.Have
			if gap < 0 {
				gap = 0
			}
			row.Gap = &gap
			pct := 100.0
			if need > 0 {
				pct = row.Have / need * 100
			}
			row.PctOfNeed = &pct
		}
		resp.Rows = append(resp.Rows, row)
	}

	// Worst gap / total gap only ever look at lump-sum rows (§8.6: a
	// monthly/daily benefit is a different unit, can't be summed or
	// compared against a lump-sum shortfall) with a known Need.
	type lumpGap struct {
		kind string
		gap  float64
		pct  float64
	}
	var lumps []lumpGap
	for _, row := range resp.Rows {
		if row.PerPeriod != "" || row.Gap == nil {
			continue
		}
		lumps = append(lumps, lumpGap{row.Kind, *row.Gap, *row.PctOfNeed})
		resp.TotalGapLumpSum += *row.Gap
	}
	if len(lumps) > 0 {
		sort.Slice(lumps, func(i, j int) bool { return lumps[i].gap > lumps[j].gap })
		resp.WorstKind, resp.WorstGap, resp.WorstPct = lumps[0].kind, lumps[0].gap, lumps[0].pct
	}

	writeJSON(w, http.StatusOK, resp)
}

// wealthInsureCreateRequest backs the add-policy form. PerPeriod is
// deliberately not a request field — it's derived from Kind server-side
// (insuranceKindPerPeriod) so a coverage can never be filed under the wrong
// unit, unlike the design template's drawer which offers 保障類型/給付單位
// as two independent selects (§8.18.2) and trusts the combination.
type wealthInsureCreateRequest struct {
	Insurer       string   `json:"insurer"`
	Name          string   `json:"name"`
	Kind          string   `json:"kind"`
	Amount        float64  `json:"amount"`
	AnnualPremium *float64 `json:"annualPremium"`
	PremiumYears  *int64   `json:"premiumYears"`
	Insured       string   `json:"insured"`
}

func (s *Server) handleWealthInsureCreate(w http.ResponseWriter, r *http.Request) {
	var req wealthInsureCreateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Insurer, req.Name, req.Insured = strings.TrimSpace(req.Insurer), strings.TrimSpace(req.Name), strings.TrimSpace(req.Insured)
	perPeriod, validKind := insuranceKindPerPeriod[req.Kind]
	if !validKind {
		writeError(w, http.StatusBadRequest, "kind is invalid")
		return
	}
	if req.Insurer == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "insurer and name are required")
		return
	}
	if req.Amount <= 0 {
		writeError(w, http.StatusBadRequest, "amount must be positive")
		return
	}
	if req.AnnualPremium != nil && *req.AnnualPremium < 0 {
		writeError(w, http.StatusBadRequest, "annualPremium must be >= 0")
		return
	}
	if req.PremiumYears != nil && *req.PremiumYears < 0 {
		writeError(w, http.StatusBadRequest, "premiumYears must be >= 0")
		return
	}

	// Venue = insurer, per §8.18.2 note 4: the policy drawer's "保險公司"
	// field doubles as the asset's venue, it has no separate venue field.
	// AssetGroup is left "" — the policy drawer has no 歸類 select either
	// (§8.18.2's field table), and it's functionally inert here anyway: an
	// insurance-type asset never gets an asset_snapshots row (its "value"
	// is coverage amount, not market value), so no group-total ever reads it.
	a := db.NewAsset{Name: req.Name, Venue: req.Insurer, Currency: "TWD", Source: "manual"}
	det := db.InsuranceDetails{Insurer: req.Insurer, Insured: req.Insured, AnnualPremium: req.AnnualPremium, PremiumYears: req.PremiumYears}
	cov := db.InsuranceCoverage{Kind: req.Kind, Amount: req.Amount, PerPeriod: perPeriod}
	id, err := s.wealthDB.CreateInsuranceAsset(a, det, cov)
	if err != nil {
		logger.Errorf("web: create insurance asset: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to save")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"id": id})
}
