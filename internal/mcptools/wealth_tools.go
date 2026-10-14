package mcptools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"argus/internal/assets"
	"argus/internal/db"
	"argus/internal/market"
	"argus/internal/service"
)

// The wealth tool set is what `argus mcp wealth` serves to the web's wealth
// assistant (Phase 28, docs/phase-28-wealth-ai-chat.md). It is a separate
// server, not a filter on NewServer's: the assistant must never see
// record_buy/add_to_watchlist, and it needs no market data provider at all —
// every tool here is a plain read of the user's own wealth records.
//
// Results are JSON, not the i18n text every other tool returns. These tools
// exist so the model can ask the user about what is missing and later
// propose changes to specific rows, which needs exact ids, nulls for "not
// recorded" and raw numbers rather than prose; a JSON reply is also the same
// in both languages, so there is nothing to localize. No withCache either:
// the rows change under the conversation (the user saves drafts mid-chat),
// and a stale read would have the model ask about something already
// recorded.

// NewWealthServer builds the `argus mcp wealth` server over database (the
// read-only connection) and drafts, the writable one the propose tools use.
// The only table they write is wealth_drafts: nothing the model does here
// reaches the user's records until the user confirms the drafts on the web.
func NewWealthServer(database, drafts *db.DB) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "argus-wealth", Version: Version}, nil)
	registerWealthTools(server, &wealthTools{db: database, drafts: drafts, now: time.Now})
	return server
}

// RunWealth serves NewWealthServer on stdio until ctx is cancelled or the
// connection closes — the `argus mcp wealth` counterpart of Run.
func RunWealth(ctx context.Context, database, drafts *db.DB) error {
	return NewWealthServer(database, drafts).Run(ctx, &mcp.StdioTransport{})
}

type wealthTools struct {
	db     *db.DB
	drafts *db.DB
	now    func() time.Time
}

func registerWealthTools(s *mcp.Server, wt *wealthTools) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_assets",
		Description: "List every recorded asset and liability (deposits, loans, credit cards, funds, property, insurance cash value, ...) with its id, latest value and the date of that value, plus its bank/loan/fund details. Also reports the trading account's latest net worth per market: stocks are recorded through trade records and must never be added here as assets.",
	}, wt.listAssets)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_cashflows",
		Description: "List the recurring monthly cash flows (salary, mortgage payment, DCA, premiums, ...): direction in/out, monthly amount, day of month, and whether each one is active or paused.",
	}, wt.listCashflows)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_insurance",
		Description: "List recorded insurance policies with insurer, insured person, annual premium, premium years, maturity, surrender value and their coverages (kind, amount, and whether the amount is a lump sum or per month/day).",
	}, wt.listInsurance)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_profile_and_goals",
		Description: "Get the household profile (annual salary, birth year, monthly retirement contribution, dependents, youngest child's age, whether the spouse has income; null = not recorded yet) and every goal, including the retirement goal (kind \"retirement\", whose target_date is the planned retirement date).",
	}, wt.getProfileAndGoals)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_drafts",
		Description: "List the drafts proposed so far that the user hasn't saved yet, each with its status: ok (ready to save), pending (the missing fields still need an answer) or error (the reason it can't be saved). Check this before proposing, so the same thing isn't proposed twice, and after the conversation restarted.",
	}, wt.listDrafts)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "propose_asset",
		Description: "Propose a new asset or liability as a draft. Nothing is saved: the user reviews the drafts and confirms them on the page. Fill in what the user said and leave out what they didn't — the draft is then pending and you should ask for the missing fields. Never for stocks or ETFs (they are trade records) or for something list_assets already has (propose_balance_update instead). Pass draft_id to replace an earlier draft once the user answers a follow-up question.",
	}, wt.proposeAsset)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "propose_balance_update",
		Description: "Propose a new current value for an asset list_assets already has, as a draft the user confirms on the page. Pass draft_id to replace an earlier balance draft.",
	}, wt.proposeBalance)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "propose_cashflow",
		Description: "Propose a new recurring monthly cash flow (salary, rent received, a loan or mortgage payment, a fund DCA, a premium, living costs, ...) as a draft the user confirms on the page. Check list_cashflows first: a flow already there is changed on the cash flow page, not proposed again. Leave out what the user didn't say and ask for it. Pass draft_id to replace an earlier cash flow draft.",
	}, wt.proposeCashflow)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "propose_insurance",
		Description: "Propose a new insurance policy with its main coverage as a draft the user confirms on the page. One draft is one policy and one coverage kind; if the policy has more coverages, say they can be added on the insurance page after saving. Check list_insurance first: a policy already there isn't proposed again. Leave out what the user didn't say and ask for it. The premium is also a cash flow: propose it with propose_cashflow (category insurance) only if the user pays it monthly. Pass draft_id to replace an earlier insurance draft.",
	}, wt.proposeInsurance)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "propose_profile",
		Description: "Propose changes to the household profile and the retirement plan as a draft the user confirms on the page. Give only the fields the user stated; the rest stay as they are. retirement_age and retirement_monthly_spend go together, and the age needs a birth year (stored, or in this draft). There is only one profile draft: if list_drafts has one, pass its draft_id and every field it should still change.",
	}, wt.proposeProfile)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "propose_goal",
		Description: "Propose a new savings goal (a child's education, a house down payment, a trip, ...) in TWD as a draft the user confirms on the page. Not for retirement: that is propose_profile. Check get_profile_and_goals first: a goal already there is changed on the goals page, not proposed again. Leave out what the user didn't say and ask for it. Pass draft_id to replace an earlier goal draft.",
	}, wt.proposeGoal)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "discard_draft_item",
		Description: "Delete one draft, e.g. when the user says it was wrong or a duplicate.",
	}, wt.discardDraft)
}

func jsonResult(v any) (*mcp.CallToolResult, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return textResult(string(b)), nil
}

type wealthAsset struct {
	ID       int64    `json:"id"`
	Side     string   `json:"side"`
	Type     string   `json:"type"`
	Name     string   `json:"name"`
	Group    string   `json:"group"`
	Venue    string   `json:"venue,omitempty"`
	Currency string   `json:"currency"`
	Value    *float64 `json:"value"`
	AsOf     string   `json:"as_of,omitempty"`

	Deposit *wealthDeposit `json:"deposit,omitempty"`
	Loan    *wealthLoan    `json:"loan,omitempty"`
	Fund    *wealthFund    `json:"fund,omitempty"`
}

type wealthDeposit struct {
	Bank        string `json:"bank,omitempty"`
	AccountNote string `json:"account_note,omitempty"`
}

type wealthLoan struct {
	Lender            string   `json:"lender,omitempty"`
	RatePct           *float64 `json:"rate_pct"`
	OriginalPrincipal *float64 `json:"original_principal"`
	RemainingMonths   *int64   `json:"remaining_months"`
	SecuredAssetID    *int64   `json:"secured_asset_id,omitempty"`
}

type wealthFund struct {
	Code                 string   `json:"code,omitempty"`
	Platform             string   `json:"platform,omitempty"`
	MonthlyAmount        *float64 `json:"monthly_amount"`
	NextContributionDate string   `json:"next_contribution_date,omitempty"`
}

// tradingAccount is the latest net_worth_snapshots row for one market — the
// stock side of net worth, derived from trades, shown so the model knows
// stocks are already counted and doesn't offer to add them as assets.
type tradingAccount struct {
	Market   string  `json:"market"`
	Currency string  `json:"currency"`
	NetWorth float64 `json:"net_worth"`
}

func (wt *wealthTools) listAssets(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, any, error) {
	list, err := wt.db.ListAssetsWithValue(false)
	if err != nil {
		return nil, nil, fmt.Errorf("list assets: %w", err)
	}
	assets := make([]wealthAsset, 0, len(list))
	for _, a := range list {
		out := wealthAsset{
			ID: a.ID, Side: a.Side, Type: a.Type, Name: a.Name, Group: a.AssetGroup,
			Venue: a.Venue, Currency: a.Currency, Value: a.Value, AsOf: a.AsOf,
		}
		// Each Get*Details is nil, nil when the asset has no row of that
		// kind, so only the matching one comes back non-nil.
		if d, err := wt.db.GetDepositDetails(a.ID); err != nil {
			return nil, nil, fmt.Errorf("deposit details %d: %w", a.ID, err)
		} else if d != nil {
			out.Deposit = &wealthDeposit{Bank: d.Bank, AccountNote: d.AccountNote}
		}
		if d, err := wt.db.GetLoanDetails(a.ID); err != nil {
			return nil, nil, fmt.Errorf("loan details %d: %w", a.ID, err)
		} else if d != nil {
			out.Loan = &wealthLoan{Lender: d.Lender, RatePct: d.RatePct, OriginalPrincipal: d.OriginalPrincipal, RemainingMonths: d.RemainingMonths, SecuredAssetID: d.SecuredAssetID}
		}
		if d, err := wt.db.GetFundDetails(a.ID); err != nil {
			return nil, nil, fmt.Errorf("fund details %d: %w", a.ID, err)
		} else if d != nil {
			out.Fund = &wealthFund{Code: d.Code, Platform: d.Platform, MonthlyAmount: d.MonthlyAmount, NextContributionDate: d.NextContributionDate}
		}
		assets = append(assets, out)
	}

	today := wt.now().In(cst).Format("2006-01-02")
	accounts := []tradingAccount{}
	for _, m := range []struct {
		id       market.MarketID
		currency string
	}{{market.US, "USD"}, {market.TW, "TWD"}} {
		v, ok, err := wt.db.GetNetWorthOnOrBefore(today, m.id)
		if err != nil {
			return nil, nil, fmt.Errorf("trading account %s: %w", m.id, err)
		}
		if ok {
			accounts = append(accounts, tradingAccount{Market: string(m.id), Currency: m.currency, NetWorth: v})
		}
	}

	res, err := jsonResult(map[string]any{"assets": assets, "trading_account": accounts})
	return res, nil, err
}

type wealthCashflow struct {
	ID         int64   `json:"id"`
	Direction  string  `json:"direction"`
	Name       string  `json:"name"`
	Amount     float64 `json:"monthly_amount"`
	Currency   string  `json:"currency"`
	DayOfMonth *int64  `json:"day_of_month"`
	AssetID    *int64  `json:"asset_id,omitempty"`
	Category   string  `json:"category,omitempty"`
	Active     bool    `json:"active"`
}

func (wt *wealthTools) listCashflows(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, any, error) {
	list, err := wt.db.ListRecurringCashflows(false)
	if err != nil {
		return nil, nil, fmt.Errorf("list cashflows: %w", err)
	}
	out := make([]wealthCashflow, 0, len(list))
	for _, c := range list {
		out = append(out, wealthCashflow{ID: c.ID, Direction: c.Direction, Name: c.Name, Amount: c.Amount, Currency: c.Currency, DayOfMonth: c.DayOfMonth, AssetID: c.AssetID, Category: c.Category, Active: c.Active})
	}
	res, err := jsonResult(map[string]any{"cashflows": out})
	return res, nil, err
}

type wealthPolicy struct {
	AssetID        int64            `json:"asset_id"`
	Name           string           `json:"name"`
	Insurer        string           `json:"insurer,omitempty"`
	Insured        string           `json:"insured,omitempty"`
	PolicyNo       string           `json:"policy_no,omitempty"`
	AnnualPremium  *float64         `json:"annual_premium"`
	PremiumYears   *int64           `json:"premium_years"`
	MaturityDate   string           `json:"maturity_date,omitempty"`
	SurrenderValue *float64         `json:"surrender_value"`
	Coverages      []wealthCoverage `json:"coverages"`
}

type wealthCoverage struct {
	Kind   string  `json:"kind"`
	Amount float64 `json:"amount"`
	// PerPeriod is "lump_sum", "month" or "day" — spelled out rather than
	// the table's "" for a lump sum, so the model can't read a blank as
	// "unknown".
	PerPeriod string `json:"per_period"`
}

func (wt *wealthTools) listInsurance(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, any, error) {
	list, err := wt.db.ListAssetsWithValue(false)
	if err != nil {
		return nil, nil, fmt.Errorf("list assets: %w", err)
	}
	covs, err := wt.db.ListInsuranceCoverages()
	if err != nil {
		return nil, nil, fmt.Errorf("list coverages: %w", err)
	}
	byAsset := map[int64][]wealthCoverage{}
	for _, c := range covs {
		per := c.PerPeriod
		if per == "" {
			per = "lump_sum"
		}
		byAsset[c.AssetID] = append(byAsset[c.AssetID], wealthCoverage{Kind: c.Kind, Amount: c.Amount, PerPeriod: per})
	}

	policies := []wealthPolicy{}
	for _, a := range list {
		if a.Type != "insurance" {
			continue
		}
		p := wealthPolicy{AssetID: a.ID, Name: a.Name, Coverages: byAsset[a.ID]}
		if p.Coverages == nil {
			p.Coverages = []wealthCoverage{}
		}
		det, err := wt.db.GetInsuranceDetails(a.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("insurance details %d: %w", a.ID, err)
		}
		if det != nil {
			p.Insurer, p.Insured, p.PolicyNo = det.Insurer, det.Insured, det.PolicyNo
			p.AnnualPremium, p.PremiumYears = det.AnnualPremium, det.PremiumYears
			p.MaturityDate, p.SurrenderValue = det.MaturityDate, det.SurrenderValue
		}
		policies = append(policies, p)
	}
	res, err := jsonResult(map[string]any{"policies": policies})
	return res, nil, err
}

type wealthGoal struct {
	ID                  int64    `json:"id"`
	Name                string   `json:"name"`
	Kind                string   `json:"kind"`
	TargetAmount        float64  `json:"target_amount"`
	Currency            string   `json:"currency"`
	TargetDate          string   `json:"target_date,omitempty"`
	Note                string   `json:"note,omitempty"`
	SavedAmount         *float64 `json:"saved_amount"`
	MonthlyContribution *float64 `json:"monthly_contribution"`
	StartYear           int      `json:"start_year,omitempty"`
}

func (wt *wealthTools) getProfileAndGoals(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, any, error) {
	list, err := wt.db.ListGoals()
	if err != nil {
		return nil, nil, fmt.Errorf("list goals: %w", err)
	}
	goals := make([]wealthGoal, 0, len(list))
	for _, g := range list {
		goals = append(goals, wealthGoal{ID: g.ID, Name: g.Name, Kind: g.Kind, TargetAmount: g.TargetAmount, Currency: g.Currency, TargetDate: g.TargetDate, Note: g.Note, SavedAmount: g.SavedAmount, MonthlyContribution: g.MonthlyContribution, StartYear: g.StartYear})
	}
	res, err := jsonResult(map[string]any{"profile": service.LoadWealthProfile(wt.db), "goals": goals})
	return res, nil, err
}

type wealthDraft struct {
	ID       int64           `json:"id"`
	Kind     string          `json:"kind"`
	TargetID *int64          `json:"asset_id,omitempty"`
	Payload  json.RawMessage `json:"draft"`
	service.DraftCheck
}

func (wt *wealthTools) today() string { return wt.now().In(cst).Format("2006-01-02") }

func (wt *wealthTools) listDrafts(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, any, error) {
	drafts, err := wt.db.ListWealthDrafts()
	if err != nil {
		return nil, nil, fmt.Errorf("list drafts: %w", err)
	}
	existing, err := wt.db.ListAssetsWithValue(false)
	if err != nil {
		return nil, nil, fmt.Errorf("list assets: %w", err)
	}
	birthYear := service.LoadWealthProfile(wt.db).BirthYear
	out := make([]wealthDraft, 0, len(drafts))
	for _, d := range drafts {
		c := service.CheckWealthDraft(d, existing, birthYear, wt.today())
		out = append(out, wealthDraft{ID: d.ID, Kind: d.Kind, TargetID: d.TargetID, Payload: json.RawMessage(d.Payload), DraftCheck: c})
	}
	res, err := jsonResult(map[string]any{"drafts": out})
	return res, nil, err
}

type proposeAssetInput struct {
	DraftID int64 `json:"draft_id,omitempty" jsonschema:"an earlier asset draft to replace; omit for a new one"`
	assets.AssetDraft
}

func (wt *wealthTools) proposeAsset(ctx context.Context, _ *mcp.CallToolRequest, in proposeAssetInput) (*mcp.CallToolResult, any, error) {
	in.Normalize()
	payload, err := json.Marshal(in.AssetDraft)
	if err != nil {
		return nil, nil, err
	}
	return wt.propose(in.DraftID, assets.DraftAsset, nil, string(payload))
}

type proposeBalanceInput struct {
	DraftID int64   `json:"draft_id,omitempty" jsonschema:"an earlier balance draft to replace; omit for a new one"`
	AssetID int64   `json:"asset_id" jsonschema:"the id from list_assets"`
	Value   float64 `json:"value" jsonschema:"the new value; for a loan or credit card the amount still owed, as a positive number"`
	Date    string  `json:"date,omitempty" jsonschema:"YYYY-MM-DD the value is as of; omit for today, never in the future"`
}

func (wt *wealthTools) proposeBalance(ctx context.Context, _ *mcp.CallToolRequest, in proposeBalanceInput) (*mcp.CallToolResult, any, error) {
	payload, err := json.Marshal(assets.BalanceDraft{Value: &in.Value, Date: strings.TrimSpace(in.Date)})
	if err != nil {
		return nil, nil, err
	}
	return wt.propose(in.DraftID, assets.DraftBalance, &in.AssetID, string(payload))
}

type proposeCashflowInput struct {
	DraftID int64 `json:"draft_id,omitempty" jsonschema:"an earlier cash flow draft to replace; omit for a new one"`
	assets.CashflowDraft
}

func (wt *wealthTools) proposeCashflow(ctx context.Context, _ *mcp.CallToolRequest, in proposeCashflowInput) (*mcp.CallToolResult, any, error) {
	in.Normalize()
	payload, err := json.Marshal(in.CashflowDraft)
	if err != nil {
		return nil, nil, err
	}
	return wt.propose(in.DraftID, assets.DraftCashflow, nil, string(payload))
}

type proposeInsuranceInput struct {
	DraftID int64 `json:"draft_id,omitempty" jsonschema:"an earlier insurance draft to replace; omit for a new one"`
	assets.InsuranceDraft
}

func (wt *wealthTools) proposeInsurance(ctx context.Context, _ *mcp.CallToolRequest, in proposeInsuranceInput) (*mcp.CallToolResult, any, error) {
	in.Normalize()
	payload, err := json.Marshal(in.InsuranceDraft)
	if err != nil {
		return nil, nil, err
	}
	return wt.propose(in.DraftID, assets.DraftInsurance, nil, string(payload))
}

type proposeProfileInput struct {
	DraftID int64 `json:"draft_id,omitempty" jsonschema:"the existing profile draft; omit only when list_drafts has none"`
	assets.ProfileDraft
}

func (wt *wealthTools) proposeProfile(ctx context.Context, _ *mcp.CallToolRequest, in proposeProfileInput) (*mcp.CallToolResult, any, error) {
	if in.DraftID == 0 {
		drafts, err := wt.db.ListWealthDrafts()
		if err != nil {
			return nil, nil, fmt.Errorf("list drafts: %w", err)
		}
		for _, d := range drafts {
			if d.Kind == assets.DraftProfile {
				return nil, nil, fmt.Errorf("profile draft %d already exists: pass it as draft_id, with every field it should change", d.ID)
			}
		}
	}
	payload, err := json.Marshal(in.ProfileDraft)
	if err != nil {
		return nil, nil, err
	}
	return wt.propose(in.DraftID, assets.DraftProfile, nil, string(payload))
}

type proposeGoalInput struct {
	DraftID int64 `json:"draft_id,omitempty" jsonschema:"an earlier goal draft to replace; omit for a new one"`
	assets.GoalDraft
}

func (wt *wealthTools) proposeGoal(ctx context.Context, _ *mcp.CallToolRequest, in proposeGoalInput) (*mcp.CallToolResult, any, error) {
	in.Normalize()
	payload, err := json.Marshal(in.GoalDraft)
	if err != nil {
		return nil, nil, err
	}
	return wt.propose(in.DraftID, assets.DraftGoal, nil, string(payload))
}

// propose stores a draft unless it is invalid: an invalid one comes back as
// a tool error for the model to correct, while a merely incomplete one is
// stored as pending — asking the user is the model's job, and the pending
// card is how the user sees what is still open.
func (wt *wealthTools) propose(draftID int64, kind string, targetID *int64, payload string) (*mcp.CallToolResult, any, error) {
	existing, err := wt.db.ListAssetsWithValue(false)
	if err != nil {
		return nil, nil, fmt.Errorf("list assets: %w", err)
	}
	c := service.CheckWealthDraft(db.WealthDraft{Kind: kind, TargetID: targetID, Payload: payload}, existing, service.LoadWealthProfile(wt.db).BirthYear, wt.today())
	if c.Status == "error" {
		return nil, nil, fmt.Errorf("not saved as a draft: %s", c.Error)
	}
	if draftID > 0 {
		if err := wt.replaceDraft(draftID, kind, targetID, payload); err != nil {
			return nil, nil, err
		}
	} else if draftID, err = wt.drafts.AddWealthDraft(kind, targetID, payload); err != nil {
		return nil, nil, fmt.Errorf("save draft: %w", err)
	}
	res, err := jsonResult(map[string]any{"draft_id": draftID, "status": c.Status, "missing": c.Missing})
	return res, nil, err
}

// replaceDraft overwrites draft id, which must be the same kind (and, for a
// balance draft, the same asset) — changing either is a new draft.
func (wt *wealthTools) replaceDraft(id int64, kind string, targetID *int64, payload string) error {
	drafts, err := wt.db.ListWealthDrafts()
	if err != nil {
		return fmt.Errorf("list drafts: %w", err)
	}
	for _, d := range drafts {
		if d.ID != id {
			continue
		}
		if d.Kind != kind || (targetID != nil && (d.TargetID == nil || *d.TargetID != *targetID)) {
			return fmt.Errorf("draft %d is a different kind or asset: discard it and propose a new one", id)
		}
		if err := wt.drafts.UpdateWealthDraft(id, payload); err != nil {
			return fmt.Errorf("update draft: %w", err)
		}
		return nil
	}
	return fmt.Errorf("draft %d not found: the user may have saved or deleted it", id)
}

type discardDraftInput struct {
	DraftID int64 `json:"draft_id" jsonschema:"the id from list_drafts"`
}

func (wt *wealthTools) discardDraft(ctx context.Context, _ *mcp.CallToolRequest, in discardDraftInput) (*mcp.CallToolResult, any, error) {
	if err := wt.drafts.DeleteWealthDraft(in.DraftID); err != nil {
		return nil, nil, fmt.Errorf("discard draft %d: %w", in.DraftID, err)
	}
	return textResult("discarded"), nil, nil
}
