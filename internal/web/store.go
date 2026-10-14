package web

import (
	"argus/internal/db"
	"argus/internal/market"
)

// dbReader is the subset of *db.DB the dashboard needs — narrow enough to
// fake in tests without a real SQLite file.
type dbReader interface {
	GetPositions() ([]db.Position, error)
	GetAllTransactions() ([]db.Transaction, error)
	GetDailySnapshotsForTickers(tickers []string, from, to string) ([]db.DailySnapshot, error)
	GetWatchlist() ([]string, error)
	// GetWatchlistByMarket backs calendar.go's earnings-event ticker set — a
	// market-scoped watchlist, so a TW earnings dot doesn't get attributed to
	// the US calendar view and vice versa.
	GetWatchlistByMarket(m market.MarketID) ([]string, error)
	GetLatestSnapshot(ticker string) (db.DailySnapshot, bool, error)
	GetNetWorthOnOrBefore(date string, m market.MarketID) (float64, bool, error)
	GetNetWorthRange(from, to string, m market.MarketID) ([]db.NetWorthPoint, error)
	// GetSetting backs risk.go's cash-balance lookup (Phase 8 PR1) — the same
	// settings key internal/bot's loadCash reads, see service.LoadCash.
	GetSetting(key string) (string, bool, error)
	// GetRecommendationsSince backs recperf.go's /api/rec-performance (Phase
	// 8 PR3) — the same whole-history read argus eval's CLI uses, so the two
	// reports are guaranteed to score identically off the same database.
	GetRecommendationsSince(fromDate string) ([]db.Recommendation, error)
	// GetRoundThesis/GetThesisEntriesInRange (via service.RoundThesis) and
	// GetLessonsForTickers back rounds.go's round-detail thesis/lessons
	// attachment (Phase 8 PR4; thesis became one text per round in Phase 27 P1b,
	// the old journal is only the fallback).
	GetRoundThesis(ticker, roundStart string) (string, bool, error)
	GetThesisEntriesInRange(ticker, from, to string) ([]db.ThesisEntry, error)
	GetLessonsForTickers(tickers []string) (map[string][]db.Lesson, error)
	// GetRealizedPnL backs buildStatus's sidebar account-overview card — the
	// same all-time-SELL-total definition internal/bot's /portfolio uses.
	GetRealizedPnL(m market.MarketID) (float64, error)
	// GetOptionPositions backs risk.go's Phase 12 PR3 collateral summary
	// (CSP locked cash, covered-call locked shares, naked-call flag) — the
	// same read internal/bot's /portfolio options section uses.
	GetOptionPositions() ([]db.OptionPosition, error)
	// GetAllOptionTransactions backs options.go's Phase 12 PR4 closed-
	// position list (/api/options).
	GetAllOptionTransactions() ([]db.OptionTransaction, error)
	// GetUniverse backs sectorflow.go's Phase 18 scan — the same market-wide
	// candidate pool RunUniverseScan/RunTWUniverseScan already scan, not a
	// new ticker set.
	GetUniverse() ([]db.UniverseEntry, error)
	// ListLLMRuns/GetLLMRun back llmruns.go's Phase 19 /llm audit page —
	// GET /api/llm-runs (list) and GET /api/llm-runs/{id} (detail).
	ListLLMRuns(limit int) ([]db.LLMRun, error)
	GetLLMRun(id int64) (db.LLMRunDetail, bool, error)
	// ListBlockedNewsSources backs both newssources.go's GET
	// /api/news-sources/blocked and the /llm page's own data-quality
	// "low-quality source" check (Phase 19 §8.2) — the block/unblock writes
	// themselves go through the separate newsSourceWriter interface.
	ListBlockedNewsSources() ([]db.BlockedNewsSource, error)
	// GetRecentPriceEvents/GetPriceEventsForTicker back events.go's GET
	// /api/events — the same two reads bot.handleEvents' /events command
	// already uses, so the web view and the Telegram command can never
	// disagree about what was recorded.
	GetRecentPriceEvents(limit int) ([]db.PriceEvent, error)
	GetPriceEventsForTicker(ticker string, limit int) ([]db.PriceEvent, error)
	// GetResearchNotesByTicker backs research_notes.go's GET
	// /api/research-notes — the chart page's notebook card history for one
	// ticker; writes go through the separate researchNotesWriter interface.
	GetResearchNotesByTicker(ticker string) ([]db.ResearchNote, error)
	// FillNewsFor backs fillsnapshot.go's GET /api/fill-snapshot — the
	// headlines (with links and LLM labels) stored when a fill on that day was
	// recorded (Phase 27 P7a).
	FillNewsFor(ticker, fillDate string) ([]db.FillNews, error)
	// StrategyAlertsFor and GetRecommendationsForTicker back strategies.go —
	// the chart's strategy markers, each with the LLM's call on it (Phase 27 P5).
	StrategyAlertsFor(ticker string) ([]db.StrategyAlert, error)
	GetRecommendationsForTicker(ticker, from, to string) ([]db.Recommendation, error)
	// ListAssetsWithValue backs wealth_assets.go's GET /api/wealth/assets (Phase 9
	// PR1) — writes go through the separate wealthWriter interface.
	ListAssetsWithValue(includeArchived bool) ([]db.AssetWithValue, error)
	// ListAssetSnapshots backs wealth_assets.go's GET /api/wealth/assets/history — one
	// asset's value records for the edit drawer.
	ListAssetSnapshots(assetID int64) ([]db.AssetSnapshot, error)
	// ListAssetsValueAsOf backs wealth_home.go's YTD/MoM historical totals.
	ListAssetsValueAsOf(asOfDate string, includeArchived bool) ([]db.AssetWithValue, error)
	// GetLoanDetails backs wealth_balance.go's liabilities list (rate/term
	// for the debt-payoff calculator and display).
	GetLoanDetails(assetID int64) (*db.LoanDetails, error)
	// ListRecurringCashflows backs wealth_cash.go's GET /api/wealth/cash
	// (Phase 9 波次2 PR5) — writes go through the separate wealthWriter
	// interface below, same GET-vs-write split as ListAssetsWithValue.
	ListRecurringCashflows(activeOnly bool) ([]db.RecurringCashflow, error)
	// ListGoals/ListAllGoalAssets back wealth_goals.go's GET /api/wealth/goals
	// (Phase 9 波次3 PR6) — writes go through the separate wealthWriter
	// interface below, same GET-vs-write split as ListAssetsWithValue.
	ListGoals() ([]db.Goal, error)
	ListAllGoalAssets() ([]db.GoalAsset, error)
	// ListInsuranceCoverages/GetInsuranceDetails back wealth_insure.go's GET
	// /api/wealth/insure (Phase 9 波次3 PR8) — writes go through the
	// separate wealthWriter interface below, same GET-vs-write split as
	// ListAssetsWithValue.
	ListInsuranceCoverages() ([]db.InsuranceCoverage, error)
	GetInsuranceDetails(assetID int64) (*db.InsuranceDetails, error)
	// GetFundDetails backs wealth_funds.go's GET /api/wealth/funds (Phase 9
	// 波次4 PR10) — writes go through the separate wealthWriter interface
	// below (fund rows are only ever created via CSV import, see
	// wealth_import.go).
	GetFundDetails(assetID int64) (*db.FundDetails, error)
}
