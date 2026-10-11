package service

import (
	"fmt"
	"sort"

	"argus/internal/assets"
	"argus/internal/db"
)

// insuranceKinds fixes the six coverage kinds' order and their unit (§8.6's
// "月給付/日額型保障不能跟一次金加總" — per_period is derived from kind
// server-side, never trusted from the client, so a coverage can never be
// filed under the wrong unit and silently corrupt the have-side aggregate).
// Order matches the design template's cov[] array (Argus Trading WebUI.dc.html
// insureModel(), lines 4288-4295).
var insuranceKinds = []struct{ kind, perPeriod string }{
	{"life", ""}, {"accident", ""}, {"ci", ""}, {"cancer", ""},
	{"disability", "month"}, {"hospital", "day"},
}

// InsuranceKindPerPeriod returns kind's benefit unit ("" lump sum, "month" or
// "day"); ok is false for a kind that isn't one of the six.
func InsuranceKindPerPeriod(kind string) (perPeriod string, ok bool) {
	for _, k := range insuranceKinds {
		if k.kind == kind {
			return k.perPeriod, true
		}
	}
	return "", false
}

// InsuranceCoverageRow is one coverage kind's have / need / gap line. The
// JSON tags (here and below) are the /api/wealth/insure wire shape.
type InsuranceCoverageRow struct {
	Kind      string   `json:"kind"`
	PerPeriod string   `json:"perPeriod,omitempty"` // "" | "month" | "day"
	Have      float64  `json:"have"`
	Need      *float64 `json:"need"`      // nil when the inputs behind it aren't available yet
	Gap       *float64 `json:"gap"`       // max(0, need-have); nil follows Need
	PctOfNeed *float64 `json:"pctOfNeed"` // have/need*100 (100 when need<=0 — trivially covered); nil follows Need
}

// InsurancePolicyItem is one coverage on one policy, with its policy's
// premium facts attached.
type InsurancePolicyItem struct {
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

// InsuranceGap is the insurance page's whole analysis. HasProfile mirrors the
// retirement page's HasBirthYear convention — false shows a mini setup card
// instead of the life/accident/disability need figures (§8.16.1's two
// "asked once" inputs); the constant-only kinds (ci/cancer/hospital) stay
// populated regardless, since they don't depend on the profile at all.
type InsuranceGap struct {
	AsOf            string                 `json:"asOf"`
	HasProfile      bool                   `json:"hasProfile"`
	Rows            []InsuranceCoverageRow `json:"rows"`
	Policies        []InsurancePolicyItem  `json:"policies"`
	Count           int                    `json:"count"`
	Premium         float64                `json:"premium"`
	PremSharePct    *float64               `json:"premSharePct"`
	WorstKind       string                 `json:"worstKind,omitempty"`
	WorstGap        float64                `json:"worstGap"`
	WorstPct        float64                `json:"worstPct"`
	TotalGapLumpSum float64                `json:"totalGapLumpSum"`
}

// InsuranceStore is the read boundary ComputeInsuranceGap needs: the wealth
// totals behind the need-side inputs, plus the policy tables themselves.
type InsuranceStore interface {
	WealthStore
	ListRecurringCashflows(activeOnly bool) ([]db.RecurringCashflow, error)
	GetInsuranceDetails(assetID int64) (*db.InsuranceDetails, error)
	ListInsuranceCoverages() ([]db.InsuranceCoverage, error)
}

// ComputeInsuranceGap compares what the user's policies cover (the have side)
// with a pure-function need estimate (assets.LifeInsuranceNeed and friends,
// §8.16.1), as of today ("2006-01-02"). Every need-side input degrades
// independently — a missing FX rate or an unset profile leaves that row's
// Need nil rather than fabricating one — so a non-nil error means only that a
// store read failed.
func ComputeInsuranceGap(store InsuranceStore, fx WealthFXStore, quotes QuoteReader, today string) (InsuranceGap, error) {
	resp := InsuranceGap{AsOf: today, Rows: []InsuranceCoverageRow{}, Policies: []InsurancePolicyItem{}}

	// Need-side inputs (§8.16.1's "平台算得出來" bucket) — each degrades
	// independently, same whole-metric rule as the rest of wealth/.
	var totalDebt float64
	var debtOK bool
	if _, tl, ok := AssetLiabilityTotals(store, fx, quotes, today, true); ok {
		totalDebt, debtOK = tl, true
	}
	var liquidAssets float64
	var liquidOK bool
	if _, byGroup, ok := WealthTotals(store, fx, quotes, today, true); ok {
		liquidAssets, liquidOK = byGroup["liquid"], true
	}
	cashflows, err := store.ListRecurringCashflows(true)
	if err != nil {
		return resp, fmt.Errorf("list cashflows: %w", err)
	}
	var annualExpense float64
	var expenseOK bool
	if monthlyOut, ok := MonthlyOutflowTWD(cashflows, today, quotes, fx); ok {
		annualExpense, expenseOK = monthlyOut*12, true
	}

	// Need-side "asked once" inputs (§8.16.2 profile) — Dependents==0 is a
	// real, explicitly-saved "no dependents," distinct from "not set yet"
	// (nil); see assets.InsuranceNeedInputs' doc comment. A youngest-child age
	// is only required when there actually are dependents —
	// assets.dependencyYears ignores the age when Dependents<=0, so don't
	// block "no dependents" households on filling in an age field that would
	// be meaningless for them.
	profile := LoadWealthProfile(store)
	in := assets.InsuranceNeedInputs{
		TotalDebt: totalDebt, AnnualExpense: annualExpense, LiquidAssets: liquidAssets,
	}
	if profile.Dependents != nil {
		in.Dependents = *profile.Dependents
	}
	if profile.YoungestChildAge != nil {
		in.YoungestChildAge, in.HasYoungestChildAge = *profile.YoungestChildAge, true
	}
	if profile.SpouseHasIncome != nil {
		in.SpouseHasIncome = *profile.SpouseHasIncome
	}
	resp.HasProfile = profile.Dependents != nil && profile.SpouseHasIncome != nil &&
		(in.Dependents <= 0 || in.HasYoungestChildAge)
	needInputsOK := debtOK && liquidOK && expenseOK && resp.HasProfile

	// Have-side: every coverage on a non-archived insurance-type asset,
	// summed by kind (§8.6: never across per_period, but insuranceKinds
	// already keeps kind 1:1 with per_period, so a plain per-kind sum can't
	// mix units).
	assetList, err := store.ListAssetsWithValue(false)
	if err != nil {
		return resp, fmt.Errorf("list assets: %w", err)
	}
	policyAssets := make(map[int64]db.AssetWithValue)
	for _, a := range assetList {
		if a.Type == "insurance" {
			policyAssets[a.ID] = a
		}
	}
	detailsByAsset := make(map[int64]*db.InsuranceDetails, len(policyAssets))
	for id := range policyAssets {
		det, derr := store.GetInsuranceDetails(id)
		if derr != nil {
			return resp, fmt.Errorf("get details %d: %w", id, derr)
		}
		detailsByAsset[id] = det
	}
	coverages, err := store.ListInsuranceCoverages()
	if err != nil {
		return resp, fmt.Errorf("list coverages: %w", err)
	}
	haveByKind := make(map[string]float64, len(insuranceKinds))
	for _, c := range coverages {
		a, ok := policyAssets[c.AssetID]
		if !ok {
			continue // archived policy, or an orphan row — excluded from both have and the policy list
		}
		haveByKind[c.Kind] += c.Amount
		item := InsurancePolicyItem{
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
	if profile.AnnualSalary != nil && *profile.AnnualSalary > 0 {
		pct := resp.Premium / *profile.AnnualSalary * 100
		resp.PremSharePct = &pct
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

	for _, k := range insuranceKinds {
		row := InsuranceCoverageRow{Kind: k.kind, PerPeriod: k.perPeriod, Have: haveByKind[k.kind]}
		if need, ok := needFor(k.kind); ok {
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

	return resp, nil
}
