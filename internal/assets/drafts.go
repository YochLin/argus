package assets

import (
	"slices"
	"strings"
)

// Draft kinds — wealth_drafts.kind (Phase 28 B2, B3a, B3b). B3c adds
// profile and goal.
const (
	DraftAsset     = "asset"
	DraftBalance   = "balance"
	DraftCashflow  = "cashflow"
	DraftInsurance = "insurance"
)

// DraftAssetTypes is what the wealth assistant may propose as a new asset —
// the quick-add drawer's kinds plus credit_card, minus insurance (a policy
// needs its coverage too; B3b). Stocks are deliberately absent: they are
// recorded through trades and show up as the trading account.
var DraftAssetTypes = []string{"deposit", "loan", "credit_card", "fund", "bond", "estate", "gold", "crypto", "pension", "other"}

// stockTypes are the type names a model reaches for when it tries to record
// a stock as an asset; they get a pointed message instead of the generic one.
var stockTypes = []string{"stock", "stocks", "etf", "equity", "equities", "share", "shares", "股票", "股"}

// AssetDraft is a proposed new asset — wealth_drafts.payload for kind
// "asset", and the propose_asset tool's input. The detail fields only apply
// to their type (deposit; loan/credit_card; fund) and are ignored otherwise.
type AssetDraft struct {
	Type     string   `json:"type" jsonschema:"one of deposit, loan, credit_card, fund, bond, estate, gold, crypto, pension, other — never a stock"`
	Name     string   `json:"name" jsonschema:"what the user calls it, e.g. 國泰活存 or 房貸"`
	Group    string   `json:"group" jsonschema:"liquid, growth, income or hard (deposit→liquid, fund/crypto→growth, bond→income, loan/estate/gold/pension→hard)"`
	Currency string   `json:"currency" jsonschema:"ISO code, e.g. TWD or USD"`
	Value    *float64 `json:"value,omitempty" jsonschema:"current balance or value; for a loan or credit card the amount still owed, as a positive number. Omit if the user hasn't said"`
	Date     string   `json:"date,omitempty" jsonschema:"YYYY-MM-DD the value is as of; omit for today"`
	Venue    string   `json:"venue,omitempty" jsonschema:"bank, broker or platform holding it"`

	Bank        string `json:"bank,omitempty" jsonschema:"deposit only"`
	AccountNote string `json:"account_note,omitempty" jsonschema:"deposit only: e.g. the last digits of the account"`

	Lender            string   `json:"lender,omitempty" jsonschema:"loan/credit_card only"`
	RatePct           *float64 `json:"rate_pct,omitempty" jsonschema:"loan/credit_card only: annual interest rate in percent, e.g. 2.1"`
	OriginalPrincipal *float64 `json:"original_principal,omitempty" jsonschema:"loan only"`
	RemainingMonths   *int64   `json:"remaining_months,omitempty" jsonschema:"loan only"`

	FundCode                 string   `json:"fund_code,omitempty" jsonschema:"fund only"`
	FundPlatform             string   `json:"fund_platform,omitempty" jsonschema:"fund only"`
	FundMonthlyAmount        *float64 `json:"fund_monthly_amount,omitempty" jsonschema:"fund only: monthly DCA amount"`
	FundNextContributionDate string   `json:"fund_next_contribution_date,omitempty" jsonschema:"fund only: YYYY-MM-DD"`
}

// Normalize trims every text field and folds the enum-like ones' case.
func (d *AssetDraft) Normalize() {
	for _, p := range []*string{&d.Name, &d.Venue, &d.Bank, &d.AccountNote, &d.Lender, &d.FundCode, &d.FundPlatform, &d.Date, &d.FundNextContributionDate} {
		*p = strings.TrimSpace(*p)
	}
	d.Type = strings.ToLower(strings.TrimSpace(d.Type))
	d.Group = strings.ToLower(strings.TrimSpace(d.Group))
	d.Currency = strings.ToUpper(strings.TrimSpace(d.Currency))
}

// Side is asset or liability, decided by the type — the model never picks it.
func (d AssetDraft) Side() string {
	if LoanTypes[d.Type] {
		return "liability"
	}
	return "asset"
}

// Problems is ValidateAsset plus what only a draft needs: a type from
// DraftAssetTypes and a currency (the drawer and CSV have their own defaults
// for it; a model that leaves it out has to be asked).
func (d AssetDraft) Problems() []Problem {
	ps := ValidateAsset(NewAsset{Side: d.Side(), Type: d.Type, Name: d.Name, Group: d.Group, Value: d.Value, Date: d.Date})
	switch {
	case d.Type == "":
	case slices.Contains(stockTypes, d.Type):
		ps = append(ps, Problem{Field: "type", Msg: "stocks are recorded as trades, not as assets"})
	case !slices.Contains(DraftAssetTypes, d.Type):
		ps = append(ps, Problem{Field: "type", Msg: "type must be one of " + strings.Join(DraftAssetTypes, ", ") + ": " + d.Type})
	}
	if d.Currency == "" {
		ps = append(ps, missing("currency"))
	}
	if d.FundNextContributionDate != "" && !validDate(d.FundNextContributionDate) {
		ps = append(ps, Problem{Field: "fund_next_contribution_date", Msg: "invalid date, expected YYYY-MM-DD: " + d.FundNextContributionDate})
	}
	return ps
}

// BalanceDraft is a proposed new value for an existing asset —
// wealth_drafts.payload for kind "balance"; the asset is the row's target_id.
type BalanceDraft struct {
	Value *float64 `json:"value"`
	Date  string   `json:"date,omitempty"`
}

// AssetKey identifies "the same asset" for duplicate detection: assets have
// no natural unique key, so name+type, case-insensitive, is the closest.
func AssetKey(name, typ string) string {
	return strings.ToLower(strings.TrimSpace(name)) + "|" + strings.ToLower(strings.TrimSpace(typ))
}
