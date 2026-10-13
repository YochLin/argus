package mcptools

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"argus/internal/db"
	"argus/internal/market"
	"argus/internal/service"
)

func connectWealth(t *testing.T, d *db.DB) *mcp.ClientSession {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "argus-wealth-test", Version: "0.0.0"}, nil)
	registerWealthTools(server, &wealthTools{db: d, drafts: d, now: func() time.Time { return time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC) }})
	ctx := context.Background()
	t1, t2 := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, t1, nil); err != nil {
		t.Fatal(err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "0.0.0"}, nil).Connect(ctx, t2, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func callJSON(t *testing.T, session *mcp.ClientSession, name string, out any) {
	t.Helper()
	text, isError := callText(t, session, name, map[string]any{})
	if isError {
		t.Fatalf("%s returned an error: %s", name, text)
	}
	if err := json.Unmarshal([]byte(text), out); err != nil {
		t.Fatalf("%s result is not JSON: %v\n%s", name, err, text)
	}
}

func mustID(t *testing.T) func(int64, error) int64 {
	return func(id int64, err error) int64 {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
}

func TestWealthServer_OnlyWealthTools(t *testing.T) {
	session := connectWealth(t, newTestDB(t))
	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	slices.Sort(names)
	want := []string{"discard_draft_item", "get_profile_and_goals", "list_assets", "list_cashflows", "list_drafts", "list_insurance", "propose_asset", "propose_balance_update", "propose_cashflow"}
	if !slices.Equal(names, want) {
		t.Errorf("wealth server tools = %v, want exactly %v (no trade or watchlist tools)", names, want)
	}
}

func TestWealthTools_EmptyDB(t *testing.T) {
	session := connectWealth(t, newTestDB(t))

	var assets struct {
		Assets         []wealthAsset    `json:"assets"`
		TradingAccount []tradingAccount `json:"trading_account"`
	}
	callJSON(t, session, "list_assets", &assets)
	if assets.Assets == nil || assets.TradingAccount == nil || len(assets.Assets)+len(assets.TradingAccount) != 0 {
		t.Errorf("empty DB list_assets = %+v, want two empty (not null) lists", assets)
	}

	var profile struct {
		Profile service.WealthProfile `json:"profile"`
		Goals   []wealthGoal          `json:"goals"`
	}
	callJSON(t, session, "get_profile_and_goals", &profile)
	if profile.Profile.BirthYear != nil || len(profile.Goals) != 0 {
		t.Errorf("empty DB get_profile_and_goals = %+v, want nothing recorded", profile)
	}
}

func TestWealthTools_ListAssets(t *testing.T) {
	d := newTestDB(t)
	dep := mustID(t)(d.CreateDepositAsset(db.NewAsset{Side: "asset", Type: "deposit", Name: "國泰活存", AssetGroup: "liquid", Currency: "TWD"}, db.DepositDetails{Bank: "國泰"}))
	rate, months := 2.1, int64(240)
	loan := mustID(t)(d.CreateLoanAsset(db.NewAsset{Side: "liability", Type: "loan", Name: "房貸", AssetGroup: "hard", Currency: "TWD"}, db.LoanDetails{Lender: "台銀", RatePct: &rate, RemainingMonths: &months}))
	gone := mustID(t)(d.CreateAsset(db.NewAsset{Side: "asset", Type: "other", Name: "已結清", AssetGroup: "liquid", Currency: "TWD"}))
	if err := d.ArchiveAsset(gone); err != nil {
		t.Fatal(err)
	}
	for id, v := range map[int64]float64{dep: 800000, loan: 6000000} {
		if err := d.UpsertAssetSnapshot(db.AssetSnapshot{AssetID: id, Date: "2026-10-01", Value: v}); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.SaveNetWorthSnapshot("2026-10-09", market.TW, 1500000); err != nil {
		t.Fatal(err)
	}

	var got struct {
		Assets         []wealthAsset    `json:"assets"`
		TradingAccount []tradingAccount `json:"trading_account"`
	}
	callJSON(t, connectWealth(t, d), "list_assets", &got)

	if len(got.Assets) != 2 {
		t.Fatalf("list_assets returned %d assets, want 2 (archived one left out): %+v", len(got.Assets), got.Assets)
	}
	byID := map[int64]wealthAsset{}
	for _, a := range got.Assets {
		byID[a.ID] = a
	}
	if a := byID[dep]; a.Deposit == nil || a.Deposit.Bank != "國泰" || a.Loan != nil || a.Value == nil || *a.Value != 800000 || a.AsOf != "2026-10-01" {
		t.Errorf("deposit = %+v, want bank 國泰, value 800000 as of 2026-10-01, no loan details", a)
	}
	if a := byID[loan]; a.Loan == nil || a.Loan.RatePct == nil || *a.Loan.RatePct != 2.1 || a.Loan.OriginalPrincipal != nil || a.Side != "liability" {
		t.Errorf("loan = %+v, want a liability with rate 2.1 and no recorded principal", a)
	}
	if len(got.TradingAccount) != 1 || got.TradingAccount[0].Market != "tw" || got.TradingAccount[0].NetWorth != 1500000 {
		t.Errorf("trading_account = %+v, want the TW net worth only", got.TradingAccount)
	}
}

func TestWealthTools_ListCashflowsAndInsurance(t *testing.T) {
	d := newTestDB(t)
	day := int64(5)
	mustID(t)(d.CreateRecurringCashflow(db.NewRecurringCashflow{Direction: "out", Name: "房貸月付", Amount: 32000, Currency: "TWD", DayOfMonth: &day}))
	premium := 36000.0
	pol := mustID(t)(d.CreateInsuranceAsset(db.NewAsset{Name: "富邦終身壽險", AssetGroup: "hard"}, db.InsuranceDetails{Insurer: "富邦人壽", AnnualPremium: &premium}, db.InsuranceCoverage{Kind: "life", Amount: 2000000}))
	session := connectWealth(t, d)

	var flows struct {
		Cashflows []wealthCashflow `json:"cashflows"`
	}
	callJSON(t, session, "list_cashflows", &flows)
	if len(flows.Cashflows) != 1 || flows.Cashflows[0].Amount != 32000 || *flows.Cashflows[0].DayOfMonth != 5 || !flows.Cashflows[0].Active {
		t.Errorf("list_cashflows = %+v, want the one active mortgage payment on the 5th", flows.Cashflows)
	}

	var ins struct {
		Policies []wealthPolicy `json:"policies"`
	}
	callJSON(t, session, "list_insurance", &ins)
	if len(ins.Policies) != 1 {
		t.Fatalf("list_insurance = %+v, want 1 policy", ins.Policies)
	}
	p := ins.Policies[0]
	if p.AssetID != pol || p.Insurer != "富邦人壽" || p.AnnualPremium == nil || *p.AnnualPremium != 36000 {
		t.Errorf("policy = %+v, want 富邦人壽 at 36000 a year", p)
	}
	if len(p.Coverages) != 1 || p.Coverages[0].PerPeriod != "lump_sum" || p.Coverages[0].Amount != 2000000 {
		t.Errorf("coverages = %+v, want one lump-sum 2,000,000 life cover", p.Coverages)
	}
}

func TestWealthTools_ProfileAndGoals(t *testing.T) {
	d := newTestDB(t)
	if err := d.SetSetting(service.BirthYearSettingKey, "1995"); err != nil {
		t.Fatal(err)
	}
	mustID(t)(d.UpsertRetirementGoal("退休", 30000000, "2045-01-01"))

	var got struct {
		Profile service.WealthProfile `json:"profile"`
		Goals   []wealthGoal          `json:"goals"`
	}
	callJSON(t, connectWealth(t, d), "get_profile_and_goals", &got)
	if got.Profile.BirthYear == nil || *got.Profile.BirthYear != 1995 || got.Profile.AnnualSalary != nil {
		t.Errorf("profile = %+v, want birth year 1995 and salary still null", got.Profile)
	}
	if len(got.Goals) != 1 || got.Goals[0].Kind != "retirement" || got.Goals[0].TargetDate != "2045-01-01" {
		t.Errorf("goals = %+v, want the retirement goal dated 2045-01-01", got.Goals)
	}
}

func TestWealthTools_Drafts(t *testing.T) {
	d := newTestDB(t)
	esun := mustID(t)(d.CreateAsset(db.NewAsset{Side: "asset", Type: "deposit", Name: "玉山活存", AssetGroup: "liquid", Currency: "TWD"}))
	session := connectWealth(t, d)

	propose := func(tool string, args map[string]any) (struct {
		DraftID int64    `json:"draft_id"`
		Status  string   `json:"status"`
		Missing []string `json:"missing"`
	}, string) {
		t.Helper()
		var out struct {
			DraftID int64    `json:"draft_id"`
			Status  string   `json:"status"`
			Missing []string `json:"missing"`
		}
		text, isError := callText(t, session, tool, args)
		if isError {
			return out, text
		}
		if err := json.Unmarshal([]byte(text), &out); err != nil {
			t.Fatalf("%s: %v\n%s", tool, err, text)
		}
		return out, ""
	}

	// Incomplete is stored as pending; the missing fields come back.
	loan, errText := propose("propose_asset", map[string]any{"type": "Loan", "name": " 房貸 ", "group": "hard", "currency": "twd"})
	if errText != "" || loan.Status != "pending" || !slices.Equal(loan.Missing, []string{"value"}) {
		t.Fatalf("pending loan = %+v (%s), want pending, missing [value]", loan, errText)
	}
	// Answering replaces the same draft.
	if got, errText := propose("propose_asset", map[string]any{"draft_id": loan.DraftID, "type": "loan", "name": "房貸", "group": "hard", "currency": "TWD", "value": 6000000, "rate_pct": 2.1}); errText != "" || got.DraftID != loan.DraftID || got.Status != "ok" {
		t.Fatalf("replaced loan = %+v (%s), want draft %d ok", got, errText, loan.DraftID)
	}

	// Invalid ones are refused and not stored.
	for name, args := range map[string]map[string]any{
		"stock":     {"type": "stock", "name": "台積電", "group": "growth", "currency": "TWD", "value": 1},
		"duplicate": {"type": "deposit", "name": "玉山活存", "group": "liquid", "currency": "TWD", "value": 1},
	} {
		if _, errText := propose("propose_asset", args); errText == "" {
			t.Errorf("%s: propose_asset accepted it, want a tool error", name)
		}
	}
	if _, errText := propose("propose_balance_update", map[string]any{"asset_id": 999, "value": 1}); errText == "" {
		t.Error("balance update for a missing asset was accepted")
	}

	bal, errText := propose("propose_balance_update", map[string]any{"asset_id": esun, "value": 150000})
	if errText != "" || bal.Status != "ok" {
		t.Fatalf("balance draft = %+v (%s), want ok", bal, errText)
	}

	var list struct {
		Drafts []wealthDraft `json:"drafts"`
	}
	callJSON(t, session, "list_drafts", &list)
	if len(list.Drafts) != 2 || list.Drafts[0].Kind != "asset" || list.Drafts[1].TargetID == nil || *list.Drafts[1].TargetID != esun {
		t.Fatalf("list_drafts = %+v, want the loan and the 玉山 balance update", list.Drafts)
	}
	var loanDraft struct {
		Name     string `json:"name"`
		Currency string `json:"currency"`
	}
	if err := json.Unmarshal(list.Drafts[0].Payload, &loanDraft); err != nil || loanDraft.Name != "房貸" || loanDraft.Currency != "TWD" {
		t.Errorf("stored loan payload = %s, want trimmed name and upper-case currency", list.Drafts[0].Payload)
	}

	if _, isError := callText(t, session, "discard_draft_item", map[string]any{"draft_id": bal.DraftID}); isError {
		t.Fatal("discard_draft_item failed")
	}
	if rows, _ := d.ListWealthDrafts(); len(rows) != 1 {
		t.Errorf("after discard %d drafts left, want 1", len(rows))
	}

	// A cash flow: pending without its amount, refused with an income
	// category on an expense.
	pay, errText := propose("propose_cashflow", map[string]any{"direction": "OUT", "name": "房貸扣款", "currency": "TWD", "day_of_month": 5, "category": "mortgage"})
	if errText != "" || pay.Status != "pending" || !slices.Equal(pay.Missing, []string{"amount"}) {
		t.Fatalf("cash flow draft = %+v (%s), want pending, missing [amount]", pay, errText)
	}
	if _, errText := propose("propose_cashflow", map[string]any{"direction": "out", "name": "薪水", "amount": 1, "currency": "TWD", "category": "salary"}); errText == "" {
		t.Error("an expense with category salary was accepted")
	}
}
