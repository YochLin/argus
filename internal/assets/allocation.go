// Package assets holds Phase 9's wealth-platform domain model and health-
// indicator pure functions (docs/phase-9-asset-platform.md §4.2). Dependency
// discipline mirrors internal/signals: no DB, no network, no LLM — every
// function here takes plain values and returns plain values, so it can be
// unit-tested without SQLite and, if this ever gets pulled into its own
// service, moving it is a directory move, not a rewrite.
package assets

// AllocationModel names one of the three preset target-allocation splits
// the wealth home page lets the user switch between (§8.17.3 — three
// presets to compare, not one saved choice).
type AllocationModel string

const (
	ModelConservative AllocationModel = "conserv"
	ModelBalanced     AllocationModel = "balanced"
	ModelGrowth       AllocationModel = "growth"
)

// AssetGroups is the fixed four-bucket taxonomy every asset carries
// (assets.asset_group, migration 29) — liquidity/growth/income/hard, in the
// order the allocation table renders them.
var AssetGroups = []string{"liquid", "growth", "income", "hard"}

// LoanTypes is which asset.Type values carry a loan_details row (rate,
// remaining term, min payment) — credit-card revolving debt uses the same
// shape as an installment loan (§8.1/§8.5/§8.9), so it shares the table
// rather than getting its own.
var LoanTypes = map[string]bool{"loan": true, "credit_card": true}

// ModelPresets is the target percentage per asset_group for each preset.
// ⚠️ These are illustrative round numbers, not derived from research or
// from the user's own situation — same honesty disclaimer the design spec
// itself makes about its demo data (§8.17.3): run the allocation page with
// real numbers first, adjust these later if they're actually off, don't
// tune blind. They key off asset_group (the 4 buckets the schema actually
// has) rather than the design mock's finer 9-category split, since §8.5
// deliberately collapsed real-estate/gold/crypto/pension into one generic
// "other" type with no per-category detail table to key a finer split off.
var ModelPresets = map[AllocationModel]map[string]float64{
	ModelConservative: {"liquid": 30, "growth": 20, "income": 30, "hard": 20},
	ModelBalanced:     {"liquid": 20, "growth": 35, "income": 20, "hard": 25},
	ModelGrowth:       {"liquid": 10, "growth": 55, "income": 10, "hard": 25},
}

// DriftRow is one line of the wealth home page's allocation table (variant
// B, §8.17.2: group / current% / target% / deviation / market value — no
// "suggested action" column, that belongs to /w/alloc's rebalance-order
// generator, a later PR).
type DriftRow struct {
	Group       string
	MarketValue float64
	CurrentPct  float64
	TargetPct   float64
	DeviationPt float64
}

// ComputeDrift turns each group's market value (already converted to one
// display currency by the caller — this package never touches currency
// conversion, that's a data-layer concern) into the allocation table. A
// non-positive total returns nil rather than dividing by zero — the caller
// should render that as "no data yet," same as a nil AssetWithValue.Value.
func ComputeDrift(byGroup map[string]float64, total float64, model AllocationModel) []DriftRow {
	if total <= 0 {
		return nil
	}
	target := ModelPresets[model]
	rows := make([]DriftRow, 0, len(AssetGroups))
	for _, g := range AssetGroups {
		mv := byGroup[g]
		cur := mv / total * 100
		tgt := target[g]
		rows = append(rows, DriftRow{Group: g, MarketValue: mv, CurrentPct: cur, TargetPct: tgt, DeviationPt: cur - tgt})
	}
	return rows
}
