package mcptools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

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
// read-only connection).
func NewWealthServer(database *db.DB) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "argus-wealth", Version: Version}, nil)
	registerWealthTools(server, &wealthTools{db: database, now: time.Now})
	return server
}

// RunWealth serves NewWealthServer on stdio until ctx is cancelled or the
// connection closes — the `argus mcp wealth` counterpart of Run.
func RunWealth(ctx context.Context, database *db.DB) error {
	return NewWealthServer(database).Run(ctx, &mcp.StdioTransport{})
}

type wealthTools struct {
	db  *db.DB
	now func() time.Time
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
