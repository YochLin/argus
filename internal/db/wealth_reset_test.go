package db

import "testing"

func TestResetWealthData(t *testing.T) {
	d := newTestDB(t)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	id := func(n int64, err error) int64 {
		t.Helper()
		must(err)
		return n
	}

	// Trading data, which must survive.
	_, err := d.RecordBuy("AAPL", 10, 200, 0, "2026-10-01")
	must(err)
	must(d.SetSetting("cash_balance", "5000"))

	// One of everything the wealth account holds.
	dep := id(d.CreateDepositAsset(NewAsset{Side: "asset", Type: "deposit", Name: "活存", AssetGroup: "liquid"}, DepositDetails{Bank: "永豐"}))
	must(d.UpsertAssetSnapshot(AssetSnapshot{AssetID: dep, Date: "2026-10-14", Value: 400000}))
	id(d.CreateLoanAsset(NewAsset{Side: "liability", Type: "loan", Name: "房貸", AssetGroup: "hard"}, LoanDetails{Lender: "銀行"}))
	id(d.CreateFundAsset(NewAsset{Side: "asset", Type: "fund", Name: "基金", AssetGroup: "growth"}, FundDetails{Code: "X1"}))
	id(d.CreateInsuranceAsset(NewAsset{Side: "asset", Type: "insurance", Name: "壽險", AssetGroup: "protect"}, InsuranceDetails{Insurer: "國泰"}, InsuranceCoverage{Kind: "life", Amount: 1000000}))
	g := id(d.CreateGoal(NewGoal{Name: "退休", Kind: "retirement", TargetAmount: 1e7}))
	must(d.SetGoalAsset(g, dep, 0.5))
	id(d.CreateRecurringCashflow(NewRecurringCashflow{Direction: "in", Name: "薪水", Amount: 90000}))
	must(d.SetSetting("profile.annual_salary", "1080000"))
	id(d.AddWealthDraft("asset", nil, "{}"))
	_, err = d.AddAssistantMessages([]AssistantMessage{{Role: "user", Text: "hi"}})
	must(err)

	must(d.ResetWealthData("2026-10-14 23:30"))

	for _, table := range []string{
		"assets", "asset_snapshots", "deposit_details", "loan_details", "fund_details",
		"insurance_details", "insurance_coverages", "goals", "goal_assets",
		"recurring_cashflows", "wealth_drafts", "wealth_assistant_messages",
	} {
		var n int
		must(d.conn.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n))
		if n != 0 {
			t.Errorf("%s: %d rows left, want 0", table, n)
		}
	}
	if _, ok, _ := d.GetSetting("profile.annual_salary"); ok {
		t.Error("profile.annual_salary survived the reset")
	}
	if v, _, _ := d.GetSetting(WealthResetAtKey); v != "2026-10-14 23:30" {
		t.Errorf("%s = %q, want the reset time", WealthResetAtKey, v)
	}

	for _, table := range []string{"positions", "transactions"} {
		var n int
		must(d.conn.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n))
		if n != 1 {
			t.Errorf("%s: %d rows, want the 1 trading row untouched", table, n)
		}
	}
	if v, _, _ := d.GetSetting("cash_balance"); v != "5000" {
		t.Errorf("cash_balance = %q, want 5000 untouched", v)
	}
}
