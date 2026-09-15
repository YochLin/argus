package service

import (
	"strconv"
	"time"

	"argus/internal/assets"
	"argus/internal/db"
	"argus/internal/logger"
	"argus/internal/market"
)

// AnnualSalarySettingKey is profile.annual_salary (§9.3) — the health
// metrics' one denominator (收支比/儲蓄率), stored in the existing settings
// key/value table rather than a new one, same convention as CashSettingKey.
const AnnualSalarySettingKey = "profile.annual_salary"

// WealthStore is the read boundary the wealth-summary logic (net worth,
// health metrics, debt payoff) needs — shared by internal/web's wealth
// pages and internal/bot's /networth command so neither duplicates the
// FX-conversion/asset-grouping assembly (moved here from internal/web's
// wealth_home.go/wealth_balance.go, Phase 9 PR2').
type WealthStore interface {
	ListAssetsWithValue(includeArchived bool) ([]db.AssetWithValue, error)
	ListAssetsValueAsOf(asOfDate string, includeArchived bool) ([]db.AssetWithValue, error)
	GetNetWorthOnOrBefore(date string, m market.MarketID) (float64, bool, error)
	GetSetting(key string) (string, bool, error)
}

// WealthFXStore is the currency-conversion cache (§8.2) — read-and-write
// together, unlike WealthStore's pure reads.
type WealthFXStore interface {
	GetFXRate(date, pair string) (float64, bool, error)
	SaveFXRate(date, pair string, rate float64) error
}

// wealthEntry is one signed, native-currency amount going into a net-worth
// total — an asset's latest value, a liability's latest value negated, or
// the equity virtual row's market value (§9.1 rule 3: never a real assets
// row, always derived at read time from positions/net_worth_snapshots).
type wealthEntry struct {
	Currency string
	Group    string
	Value    float64
}

func assetEntries(list []db.AssetWithValue) []wealthEntry {
	out := make([]wealthEntry, 0, len(list))
	for _, a := range list {
		if a.Value == nil {
			continue
		}
		v := *a.Value
		if a.Side == "liability" {
			v = -v
		}
		out = append(out, wealthEntry{Currency: a.Currency, Group: a.AssetGroup, Value: v})
	}
	return out
}

// equityEntries wraps the trading account's own net worth (already
// cash+positions, from net_worth_snapshots) as the "equity" virtual row,
// one per market that has any history — always asset_group "growth". A
// market with no net_worth_snapshots row on/before the date (e.g. an
// account that has never held anything in that market) simply contributes
// nothing, distinct from a currency-conversion failure below.
func EquityEntries(usTotal float64, usOK bool, twTotal float64, twOK bool) []wealthEntry {
	var out []wealthEntry
	if usOK && usTotal != 0 {
		out = append(out, wealthEntry{Currency: "USD", Group: "growth", Value: usTotal})
	}
	if twOK && twTotal != 0 {
		out = append(out, wealthEntry{Currency: "TWD", Group: "growth", Value: twTotal})
	}
	return out
}

// rateToTWD resolves currency's rate to TWD on date. TWD itself is always
// 1. live=true additionally fetches+caches today's rate from the quote
// provider when fx_rates doesn't have it yet — only ever passed true for
// today's date, since a historical rate can't be fetched live. ok=false is
// the signal the caller must degrade the *whole* metric to "—" rather than
// silently sum only the currencies it could price (§8.17.1's rule against
// fabricating a number from an incomplete period applies equally to an
// incomplete currency set).
func RateToTWD(currency, date string, live bool, quotes QuoteReader, fx WealthFXStore) (float64, bool) {
	if currency == "" || currency == "TWD" {
		return 1, true
	}
	pair := currency + "TWD"
	if rate, ok, err := fx.GetFXRate(date, pair); err == nil && ok {
		return rate, true
	}
	if !live {
		return 0, false
	}
	q, err := quotes.GetQuote(pair + "=X")
	if err != nil || q.Price <= 0 {
		return 0, false
	}
	// Best-effort cache: a save failure doesn't invalidate the rate we just
	// fetched, it only means tomorrow's historical lookup for today will
	// have to re-fetch (or degrade) — not worth failing the whole request
	// over.
	if err := fx.SaveFXRate(date, pair, q.Price); err != nil {
		logger.Errorf("service: cache fx rate %s@%s: %v", pair, date, err)
	}
	return q.Price, true
}

// sumEntriesTWD converts every entry to TWD and totals it, plus per-group
// subtotals. ok=false means at least one entry's currency couldn't be
// priced for date — the caller must not render a partial sum.
func sumEntriesTWD(entries []wealthEntry, date string, live bool, quotes QuoteReader, fx WealthFXStore) (total float64, byGroup map[string]float64, ok bool) {
	byGroup = make(map[string]float64, len(assets.AssetGroups))
	for _, e := range entries {
		rate, rok := RateToTWD(e.Currency, date, live, quotes, fx)
		if !rok {
			return 0, nil, false
		}
		converted := e.Value * rate
		total += converted
		byGroup[e.Group] += converted
	}
	return total, byGroup, true
}

// WealthTotals is the one place that assembles "everything the user owns,
// in TWD, as of date." live must only be true for today (see rateToTWD).
func WealthTotals(store WealthStore, fx WealthFXStore, quotes QuoteReader, date string, live bool) (float64, map[string]float64, bool) {
	var list []db.AssetWithValue
	var err error
	if live {
		list, err = store.ListAssetsWithValue(false)
	} else {
		list, err = store.ListAssetsValueAsOf(date, false)
	}
	if err != nil {
		logger.Errorf("service: wealth totals: list assets as of %s: %v", date, err)
		return 0, nil, false
	}
	entries := assetEntries(list)

	usTotal, usOK, err := store.GetNetWorthOnOrBefore(date, market.US)
	if err != nil {
		logger.Errorf("service: wealth totals: US net worth as of %s: %v", date, err)
		return 0, nil, false
	}
	twTotal, twOK, err := store.GetNetWorthOnOrBefore(date, market.TW)
	if err != nil {
		logger.Errorf("service: wealth totals: TW net worth as of %s: %v", date, err)
		return 0, nil, false
	}
	entries = append(entries, EquityEntries(usTotal, usOK, twTotal, twOK)...)

	return sumEntriesTWD(entries, date, live, quotes, fx)
}

// PeriodReturnPct is (todayTotal - baseline)/baseline, or ok=false when
// either side is unavailable — todayTotal/todayOK are passed in rather than
// recomputed so YTD and MoM don't each re-run WealthTotals(today) (and its
// live FX fetch) a second and third time.
func PeriodReturnPct(store WealthStore, fx WealthFXStore, quotes QuoteReader, fromDate string, todayTotal float64, todayOK bool) (float64, bool) {
	if !todayOK {
		return 0, false
	}
	baseline, _, ok := WealthTotals(store, fx, quotes, fromDate, false)
	if !ok || baseline == 0 {
		return 0, false
	}
	return (todayTotal - baseline) / baseline * 100, true
}

// AssetLiabilityTotals splits WealthTotals' net figure into its two sides
// (design-mock hero shows both, not just the net) — same whole-metric-
// degrades-on-any-FX-miss rule as WealthTotals/sumEntriesTWD.
func AssetLiabilityTotals(store WealthStore, fx WealthFXStore, quotes QuoteReader, date string, live bool) (totalAssets, totalLiabilities float64, ok bool) {
	var list []db.AssetWithValue
	var err error
	if live {
		list, err = store.ListAssetsWithValue(false)
	} else {
		list, err = store.ListAssetsValueAsOf(date, false)
	}
	if err != nil {
		logger.Errorf("service: asset/liability totals as of %s: %v", date, err)
		return 0, 0, false
	}
	for _, a := range list {
		if a.Value == nil {
			continue
		}
		rate, rok := RateToTWD(a.Currency, date, live, quotes, fx)
		if !rok {
			return 0, 0, false
		}
		v := *a.Value * rate
		if a.Side == "liability" {
			totalLiabilities += v
		} else {
			totalAssets += v
		}
	}

	usTotal, usOK, err := store.GetNetWorthOnOrBefore(date, market.US)
	if err != nil {
		logger.Errorf("service: asset/liability totals: US net worth as of %s: %v", date, err)
		return 0, 0, false
	}
	twTotal, twOK, err := store.GetNetWorthOnOrBefore(date, market.TW)
	if err != nil {
		logger.Errorf("service: asset/liability totals: TW net worth as of %s: %v", date, err)
		return 0, 0, false
	}
	for _, e := range EquityEntries(usTotal, usOK, twTotal, twOK) {
		rate, rok := RateToTWD(e.Currency, date, live, quotes, fx)
		if !rok {
			return 0, 0, false
		}
		totalAssets += e.Value * rate
	}
	return totalAssets, totalLiabilities, true
}

// DepositTotalTWD sums every deposit-type asset's latest value on or before
// date, converted to TWD — the "存款餘額" §2 point 4's expense derivation
// needs. ok=false on any FX-conversion failure, or if any deposit-type
// asset has no snapshot on/before date at all: treating "no history" as
// "balance was 0" would silently manufacture a huge fake expense/income
// swing the moment a new account is added — §8.17.1 requires "at least two
// snapshot periods," not a such-a-row-exists guess.
func DepositTotalTWD(store WealthStore, fx WealthFXStore, quotes QuoteReader, date string, live bool) (float64, bool) {
	var list []db.AssetWithValue
	var err error
	if live {
		list, err = store.ListAssetsWithValue(false)
	} else {
		list, err = store.ListAssetsValueAsOf(date, false)
	}
	if err != nil {
		logger.Errorf("service: deposit total as of %s: %v", date, err)
		return 0, false
	}
	deposits := make([]db.AssetWithValue, 0, len(list))
	for _, a := range list {
		if a.Type != "deposit" {
			continue
		}
		if a.Value == nil {
			return 0, false
		}
		deposits = append(deposits, a)
	}
	total, _, ok := sumEntriesTWD(assetEntries(deposits), date, live, quotes, fx)
	return total, ok
}

// HealthMetrics bundles §2's four health-indicator ratios plus the salary
// they're derived from — every pointer field nil means "not computable yet"
// (no salary set, no deposit history yet, etc.), never a fabricated 0/100%
// (§8.17.1). Shared by /w/balance's JSON response and /networth's Telegram
// summary so the two can never disagree about a number.
type HealthMetrics struct {
	MonthlySalary   *float64
	DebtRatioPct    *float64
	SavingsRatePct  *float64
	ExpenseRatioPct *float64
	LiquidityMonths *float64
}

// ComputeHealthMetrics assembles HealthMetrics as of now, using today's live
// totals plus a month-ago snapshot for the expense derivation (§2 point 4).
func ComputeHealthMetrics(store WealthStore, fx WealthFXStore, quotes QuoteReader, now time.Time) HealthMetrics {
	today := now.Format("2006-01-02")
	var m HealthMetrics

	if ta, tl, ok := AssetLiabilityTotals(store, fx, quotes, today, true); ok {
		if dr, ok := assets.DebtRatio(ta, tl); ok {
			m.DebtRatioPct = &dr
		}
	}

	raw, hasSetting, err := store.GetSetting(AnnualSalarySettingKey)
	if err != nil || !hasSetting {
		return m
	}
	annual, perr := strconv.ParseFloat(raw, 64)
	if perr != nil || annual <= 0 {
		return m
	}
	monthlySalary := annual / 12
	m.MonthlySalary = &monthlySalary

	_, byGroup, todayOK := WealthTotals(store, fx, quotes, today, true)
	depositNow, nowOK := DepositTotalTWD(store, fx, quotes, today, true)
	monthAgo := now.AddDate(0, -1, 0).Format("2006-01-02")
	depositPrev, prevOK := DepositTotalTWD(store, fx, quotes, monthAgo, false)
	if !nowOK || !prevOK {
		return m
	}
	expense := assets.MonthlyExpense(monthlySalary, depositNow, depositPrev)
	if sr, ok := assets.SavingsRate(monthlySalary, expense); ok {
		m.SavingsRatePct = &sr
	}
	if er, ok := assets.ExpenseRatio(monthlySalary, expense); ok {
		m.ExpenseRatioPct = &er
	}
	if todayOK {
		if lm, ok := assets.LiquidityMonths(byGroup["liquid"], expense); ok {
			m.LiquidityMonths = &lm
		}
	}
	return m
}
