package assets

import "testing"

func TestLifeInsuranceNeedNoDependents(t *testing.T) {
	// No dependents: need is just debt minus the liquid-asset buffer, no
	// income-replacement years — matches PLAN.md's PR8 note on the
	// formula's deliberate scope.
	in := InsuranceNeedInputs{TotalDebt: 1000000, AnnualExpense: 600000, LiquidAssets: 300000}
	got := LifeInsuranceNeed(in)
	want := 700000.0 // 1,000,000 - 300,000
	if !approxEqual(got, want) {
		t.Errorf("LifeInsuranceNeed() = %v, want %v", got, want)
	}
}

func TestLifeInsuranceNeedWithDependents(t *testing.T) {
	in := InsuranceNeedInputs{
		TotalDebt: 1000000, AnnualExpense: 600000, LiquidAssets: 300000,
		Dependents: 1, YoungestChildAge: 10, HasYoungestChildAge: true, SpouseHasIncome: false,
	}
	// dependencyYears = 22-10 = 12; 1,000,000 + 12*600,000 - 300,000
	got := LifeInsuranceNeed(in)
	want := 1000000.0 + 12*600000 - 300000
	if !approxEqual(got, want) {
		t.Errorf("LifeInsuranceNeed() = %v, want %v", got, want)
	}
}

func TestLifeInsuranceNeedSpouseIncomeHalves(t *testing.T) {
	base := InsuranceNeedInputs{
		AnnualExpense: 600000, Dependents: 1, YoungestChildAge: 12, HasYoungestChildAge: true,
	}
	noSpouseIncome := base
	withSpouseIncome := base
	withSpouseIncome.SpouseHasIncome = true

	got, want := LifeInsuranceNeed(withSpouseIncome), LifeInsuranceNeed(noSpouseIncome)/2
	if !approxEqual(got, want) {
		t.Errorf("spouse-income need = %v, want half of no-spouse-income need (%v)", got, want)
	}
}

func TestLifeInsuranceNeedFloorsAtZero(t *testing.T) {
	// No debt, no dependents, ample liquid assets — need must not go
	// negative.
	in := InsuranceNeedInputs{TotalDebt: 0, LiquidAssets: 5000000}
	if got := LifeInsuranceNeed(in); got != 0 {
		t.Errorf("LifeInsuranceNeed() = %v, want 0", got)
	}
}

func TestLifeInsuranceNeedDependentsWithoutAgeIsZeroYears(t *testing.T) {
	// Dependents set but youngest child age never provided — must not
	// guess an age; dependencyYears degrades to 0, same as no dependents.
	withAge := InsuranceNeedInputs{Dependents: 2, YoungestChildAge: 5, HasYoungestChildAge: true, AnnualExpense: 500000}
	withoutAge := InsuranceNeedInputs{Dependents: 2, AnnualExpense: 500000}
	if got := LifeInsuranceNeed(withoutAge); got != 0 {
		t.Errorf("LifeInsuranceNeed() without age = %v, want 0", got)
	}
	if got := LifeInsuranceNeed(withAge); got == 0 {
		t.Errorf("LifeInsuranceNeed() with age = 0, want > 0")
	}
}

func TestAccidentInsuranceNeedMirrorsLife(t *testing.T) {
	in := InsuranceNeedInputs{TotalDebt: 800000, AnnualExpense: 400000, Dependents: 1, YoungestChildAge: 15, HasYoungestChildAge: true}
	if got, want := AccidentInsuranceNeed(in), LifeInsuranceNeed(in); got != want {
		t.Errorf("AccidentInsuranceNeed() = %v, want == LifeInsuranceNeed() %v", got, want)
	}
}

func TestDisabilityInsuranceNeed(t *testing.T) {
	in := InsuranceNeedInputs{AnnualExpense: 600000}
	got := DisabilityInsuranceNeed(in)
	want := 50000.0 // 600,000/12
	if !approxEqual(got, want) {
		t.Errorf("DisabilityInsuranceNeed() = %v, want %v", got, want)
	}

	withSpouseIncome := in
	withSpouseIncome.SpouseHasIncome = true
	got2 := DisabilityInsuranceNeed(withSpouseIncome)
	if !approxEqual(got2, want/2) {
		t.Errorf("DisabilityInsuranceNeed() with spouse income = %v, want %v", got2, want/2)
	}
}
