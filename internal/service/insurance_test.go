package service

import (
	"errors"
	"math"
	"strings"
	"testing"

	"argus/internal/db"
)

type fakeInsuranceStore struct {
	fakeWealthStore
	cashflows    []db.RecurringCashflow
	details      map[int64]*db.InsuranceDetails
	coverages    []db.InsuranceCoverage
	cashflowErr  error
	coverageErr  error
	detailsErrID int64
}

func (f *fakeInsuranceStore) ListRecurringCashflows(bool) ([]db.RecurringCashflow, error) {
	return f.cashflows, f.cashflowErr
}

func (f *fakeInsuranceStore) GetInsuranceDetails(id int64) (*db.InsuranceDetails, error) {
	if id == f.detailsErrID {
		return nil, errors.New("details boom")
	}
	return f.details[id], nil
}

func (f *fakeInsuranceStore) ListInsuranceCoverages() ([]db.InsuranceCoverage, error) {
	return f.coverages, f.coverageErr
}

// household: NT$1,000,000 liquid, a NT$400,000 loan, NT$50,000/month of
// spending — the three platform-computed need inputs.
func household() *fakeInsuranceStore {
	return &fakeInsuranceStore{
		fakeWealthStore: fakeWealthStore{
			assets: []db.AssetWithValue{
				{Asset: db.Asset{ID: 1, Side: "asset", Type: "deposit", AssetGroup: "liquid"}, Value: floatPtr(1000000)},
				{Asset: db.Asset{ID: 2, Side: "liability", Type: "loan", AssetGroup: "hard"}, Value: floatPtr(400000)},
			},
			settings: map[string]string{},
		},
		cashflows: []db.RecurringCashflow{{ID: 1, Direction: "out", Amount: 50000, Currency: "TWD", Active: true}},
	}
}

func rowOf(t *testing.T, g InsuranceGap, kind string) InsuranceCoverageRow {
	t.Helper()
	for _, r := range g.Rows {
		if r.Kind == kind {
			return r
		}
	}
	t.Fatalf("no %q row in %+v", kind, g.Rows)
	return InsuranceCoverageRow{}
}

func TestComputeInsuranceGapWithoutProfileKeepsConstantKinds(t *testing.T) {
	g, err := ComputeInsuranceGap(household(), &fakeWealthFX{}, &fakeWealthQuotes{}, "2026-10-11")
	if err != nil {
		t.Fatalf("ComputeInsuranceGap() error = %v", err)
	}
	if g.HasProfile || len(g.Rows) != 6 {
		t.Fatalf("HasProfile = %v, len(Rows) = %d; want false, 6", g.HasProfile, len(g.Rows))
	}
	for _, kind := range []string{"life", "accident", "disability"} {
		if r := rowOf(t, g, kind); r.Need != nil || r.Gap != nil || r.PctOfNeed != nil {
			t.Errorf("%s = %+v, want no need/gap/pct without a profile", kind, r)
		}
	}
	if r := rowOf(t, g, "ci"); r.Need == nil || *r.Need != 2000000 {
		t.Errorf("ci = %+v, want the constant need 2,000,000", r)
	}
	if r := rowOf(t, g, "hospital"); r.PerPeriod != "day" || r.Need == nil || *r.Need != 3000 {
		t.Errorf("hospital = %+v, want per-day need 3,000", r)
	}
}

func TestComputeInsuranceGapGapsAndWorstKind(t *testing.T) {
	s := household()
	s.settings = map[string]string{
		DependentsSettingKey: "1", YoungestChildAgeSettingKey: "10", SpouseHasIncomeSettingKey: "0",
		AnnualSalarySettingKey: "1000000",
	}
	s.assets = append(s.assets, db.AssetWithValue{Asset: db.Asset{ID: 3, Side: "asset", Type: "insurance", Name: "壽險", Venue: "國泰", Source: "manual"}})
	premium := 68000.0
	s.details = map[int64]*db.InsuranceDetails{3: {Insured: "本人", AnnualPremium: &premium}}
	s.coverages = []db.InsuranceCoverage{
		{AssetID: 3, Kind: "life", Amount: 5000000},
		{AssetID: 99, Kind: "life", Amount: 777}, // no such live policy: excluded
	}

	g, err := ComputeInsuranceGap(s, &fakeWealthFX{}, &fakeWealthQuotes{}, "2026-10-11")
	if err != nil {
		t.Fatalf("ComputeInsuranceGap() error = %v", err)
	}
	// life need = debt 400,000 + (22-10) years * 600,000 spend - liquid 1,000,000.
	life := rowOf(t, g, "life")
	if life.Need == nil || *life.Need != 6600000 || life.Have != 5000000 || *life.Gap != 1600000 {
		t.Errorf("life = %+v, want need 6.6M have 5M gap 1.6M", life)
	}
	// Lump-sum kinds only: life 1.6M + accident 6.6M + ci 2M + cancer 1.5M.
	if g.TotalGapLumpSum != 11700000 {
		t.Errorf("TotalGapLumpSum = %v, want 11,700,000 (disability/hospital are different units)", g.TotalGapLumpSum)
	}
	if g.WorstKind != "accident" || g.WorstGap != 6600000 || g.WorstPct != 0 {
		t.Errorf("worst = %q gap %v pct %v; want accident 6.6M 0%%", g.WorstKind, g.WorstGap, g.WorstPct)
	}
	if g.Count != 1 || g.Premium != 68000 || len(g.Policies) != 1 || g.Policies[0].Insured != "本人" {
		t.Errorf("Count %d Premium %v Policies %+v; want one 本人 policy at 68,000", g.Count, g.Premium, g.Policies)
	}
	if g.PremSharePct == nil || math.Abs(*g.PremSharePct-6.8) > 1e-9 {
		t.Errorf("PremSharePct = %v, want 6.8", g.PremSharePct)
	}
}

func TestComputeInsuranceGapOverCoveredIsZeroGapNotNegative(t *testing.T) {
	s := household()
	s.settings = map[string]string{DependentsSettingKey: "0", SpouseHasIncomeSettingKey: "1"}
	s.assets = append(s.assets, db.AssetWithValue{Asset: db.Asset{ID: 3, Side: "asset", Type: "insurance"}})
	s.coverages = []db.InsuranceCoverage{{AssetID: 3, Kind: "ci", Amount: 9000000}}

	g, _ := ComputeInsuranceGap(s, &fakeWealthFX{}, &fakeWealthQuotes{}, "2026-10-11")
	if r := rowOf(t, g, "ci"); *r.Gap != 0 || *r.PctOfNeed != 450 {
		t.Errorf("ci = gap %v pct %v, want 0 and 450", *r.Gap, *r.PctOfNeed)
	}
}

// A childless household isn't blocked on a youngest-child age that would be
// meaningless for it, but a household WITH dependents still is.
func TestComputeInsuranceGapProfileNeedsAgeOnlyWithDependents(t *testing.T) {
	s := household()
	s.settings = map[string]string{DependentsSettingKey: "0", SpouseHasIncomeSettingKey: "0"}
	if g, _ := ComputeInsuranceGap(s, &fakeWealthFX{}, &fakeWealthQuotes{}, "2026-10-11"); !g.HasProfile {
		t.Error("dependents=0 without an age: HasProfile = false, want true")
	}

	s.settings[DependentsSettingKey] = "2"
	if g, _ := ComputeInsuranceGap(s, &fakeWealthFX{}, &fakeWealthQuotes{}, "2026-10-11"); g.HasProfile {
		t.Error("dependents=2 without an age: HasProfile = true, want false")
	}
}

func TestComputeInsuranceGapMissingFXDegradesNeedNotTheWholeCall(t *testing.T) {
	s := household()
	s.settings = map[string]string{DependentsSettingKey: "1", YoungestChildAgeSettingKey: "10", SpouseHasIncomeSettingKey: "0"}
	s.assets = append(s.assets, db.AssetWithValue{
		Asset: db.Asset{ID: 9, Side: "liability", Type: "loan", AssetGroup: "hard", Currency: "USD"}, Value: floatPtr(1000),
	})

	g, err := ComputeInsuranceGap(s, &fakeWealthFX{}, &fakeWealthQuotes{}, "2026-10-11")
	if err != nil {
		t.Fatalf("ComputeInsuranceGap() error = %v, want nil: a missing rate is not a failed read", err)
	}
	if !g.HasProfile || rowOf(t, g, "life").Need != nil {
		t.Errorf("HasProfile %v, life.Need %v; want profile true but no life need without USD→TWD", g.HasProfile, rowOf(t, g, "life").Need)
	}
}

func TestComputeInsuranceGapStoreErrorsAreWrapped(t *testing.T) {
	boom := errors.New("boom")

	s := household()
	s.cashflowErr = boom
	if _, err := ComputeInsuranceGap(s, &fakeWealthFX{}, &fakeWealthQuotes{}, "2026-10-11"); !errors.Is(err, boom) || !strings.Contains(err.Error(), "list cashflows") {
		t.Errorf("cashflow failure: err = %v, want it to wrap boom and say list cashflows", err)
	}

	s = household()
	s.coverageErr = boom
	if _, err := ComputeInsuranceGap(s, &fakeWealthFX{}, &fakeWealthQuotes{}, "2026-10-11"); !errors.Is(err, boom) || !strings.Contains(err.Error(), "list coverages") {
		t.Errorf("coverage failure: err = %v, want it to wrap boom and say list coverages", err)
	}

	s = household()
	s.assets = append(s.assets, db.AssetWithValue{Asset: db.Asset{ID: 3, Side: "asset", Type: "insurance"}})
	s.detailsErrID = 3
	if _, err := ComputeInsuranceGap(s, &fakeWealthFX{}, &fakeWealthQuotes{}, "2026-10-11"); err == nil || !strings.Contains(err.Error(), "get details 3") {
		t.Errorf("details failure: err = %v, want it to say get details 3", err)
	}
}

func TestInsuranceKindPerPeriod(t *testing.T) {
	for kind, want := range map[string]string{"life": "", "accident": "", "ci": "", "cancer": "", "disability": "month", "hospital": "day"} {
		if got, ok := InsuranceKindPerPeriod(kind); !ok || got != want {
			t.Errorf("InsuranceKindPerPeriod(%q) = %q, %v; want %q, true", kind, got, ok, want)
		}
	}
	if _, ok := InsuranceKindPerPeriod("savings"); ok {
		t.Error("InsuranceKindPerPeriod(savings) ok = true, want false")
	}
}
