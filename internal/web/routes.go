package web

// registerRoutes is the dashboard's URL index: each register* below owns one
// area, and the handlers it names live in that area's files — stock pages in
// dashboard.go/chart.go/rounds.go/…, trade writes in trade.go/trade_import.go,
// wealth in wealth_*.go, the token-authenticated /api/v1 surface in apiv1*.go.
// http.ServeMux picks by pattern specificity, not registration order, so
// moving a route between these functions never changes which handler wins.
func (s *Server) registerRoutes() {
	s.registerStockRoutes()
	s.registerWealthRoutes()
	s.registerLLMRoutes()
	s.registerSystemRoutes()
	s.registerAPIV1()
	s.mux.Handle("/", spaHandler())
}

// registerStockRoutes: the US/TW stock dashboard — read pages first, then the
// writes (trade, stop, watchlist, buy alerts, options ledger, thesis, notes,
// CSV import).
func (s *Server) registerStockRoutes() {
	s.mux.HandleFunc("GET /api/config", s.handleConfig)
	s.mux.HandleFunc("GET /api/status", s.handleStatus)
	s.mux.HandleFunc("GET /api/dashboard", s.handleDashboard)
	s.mux.HandleFunc("GET /api/calendar", s.handleCalendar)
	s.mux.HandleFunc("GET /api/rounds", s.handleRounds)
	s.mux.HandleFunc("GET /api/round-detail", s.handleRoundDetail)
	s.mux.HandleFunc("GET /api/reports", s.handleReports)
	s.mux.HandleFunc("GET /api/monthly", s.handleMonthly)
	s.mux.HandleFunc("GET /api/distributions", s.handleDistributions)
	s.mux.HandleFunc("GET /api/chart", s.handleChart)
	s.mux.HandleFunc("GET /api/chart/peers", s.handlePeers)
	s.mux.HandleFunc("GET /api/fill-snapshot", s.handleFillSnapshot)
	s.mux.HandleFunc("GET /api/research-notes", s.handleResearchNotesGet)
	s.mux.HandleFunc("GET /api/tickers", s.handleTickers)
	s.mux.HandleFunc("GET /api/watchlist-summary", s.handleWatchlistSummary)
	s.mux.HandleFunc("GET /api/company-names", s.handleCompanyNames)
	s.mux.HandleFunc("GET /api/risk", s.handleRisk)
	s.mux.HandleFunc("GET /api/rec-performance", s.handleRecPerformance)
	s.mux.HandleFunc("GET /api/paper", s.handlePaper)
	s.mux.HandleFunc("GET /api/options", s.handleOptions)
	s.mux.HandleFunc("GET /api/events", s.handleEvents)
	s.mux.HandleFunc("GET /api/sectorflow", s.handleSectorFlow)
	s.mux.HandleFunc("POST /api/sectorflow/refresh", s.handleSectorFlowRefresh)

	// The TradeExecutor-backed routes carry requireTrade on top of the
	// shared gate — Trade is nil whenever Telegram isn't configured (Phase 17
	// PR1), which is now a reachable state for an otherwise-working
	// dashboard.
	s.mux.HandleFunc("POST /api/trade/buy", s.requireWritable(s.requireAuth(s.requireTrade(s.handleTradeBuy))))
	s.mux.HandleFunc("POST /api/trade/sell", s.requireWritable(s.requireAuth(s.requireTrade(s.handleTradeSell))))
	s.mux.HandleFunc("POST /api/trade/delete", s.requireWritable(s.requireAuth(s.requireTrade(s.handleDeleteTransaction))))
	s.mux.HandleFunc("POST /api/stop", s.requireWritable(s.requireAuth(s.requireTrade(s.handleSetStop))))
	s.mux.HandleFunc("POST /api/watchlist/add", s.requireWritable(s.requireAuth(s.handleWatchlistAdd)))
	s.mux.HandleFunc("POST /api/watchlist/remove", s.requireWritable(s.requireAuth(s.handleWatchlistRemove)))
	s.mux.HandleFunc("POST /api/buy-alerts/add", s.requireWritable(s.requireAuth(s.requireTrade(s.handleBuyAlertAdd))))
	s.mux.HandleFunc("POST /api/buy-alerts/remove", s.requireWritable(s.requireAuth(s.handleBuyAlertRemove)))
	s.mux.HandleFunc("POST /api/options/open", s.requireWritable(s.requireAuth(s.requireTrade(s.handleOptionOpen))))
	s.mux.HandleFunc("POST /api/options/close", s.requireWritable(s.requireAuth(s.requireTrade(s.handleOptionClose))))
	// /api/thesis (Phase 21) is a DB write like any other above — no reason
	// to give it a looser gate than /api/trade/*.
	s.mux.HandleFunc("POST /api/thesis", s.requireWritable(s.requireAuth(s.handleThesisSet)))
	// /api/research-notes/* (chart page's notebook card) is a DB write like
	// /api/thesis above — same gate, body-based id targeting for pin/delete
	// rather than a new {id} path pattern, matching /api/trade/delete.
	s.mux.HandleFunc("POST /api/research-notes", s.requireWritable(s.requireAuth(s.handleResearchNoteSave)))
	s.mux.HandleFunc("POST /api/research-notes/pin", s.requireWritable(s.requireAuth(s.handleResearchNotePin)))
	s.mux.HandleFunc("POST /api/research-notes/delete", s.requireWritable(s.requireAuth(s.handleResearchNoteDelete)))
	// /api/import (Phase 5 §B, optional CSV backfill) reuses the same
	// requireWritable/requireAuth gate as every other write route — a bulk
	// transaction write is no less a write than a single /buy.
	s.mux.HandleFunc("POST /api/import", s.requireWritable(s.requireAuth(s.handleImport)))
}

// registerWealthRoutes: the /w/* asset platform and its assistant.
func (s *Server) registerWealthRoutes() {
	// /api/wealth/assets (Phase 9 PR1) — GET is ungated like every other read
	// route; the three writes (create/snapshot/archive) share the same
	// requireWritable/requireAuth gate as /api/trade/*, since a human typing
	// into the quick-add drawer is exactly the "form write, no LLM in the
	// loop" case docs/phase-9-asset-platform.md §5 carves out from
	// write-gating (that rule is about *LLM-produced* writes, unrelated to
	// this auth gate, which every write route carries regardless).
	s.mux.HandleFunc("GET /api/wealth/assets", s.handleWealthAssetsList)
	s.mux.HandleFunc("GET /api/wealth/networth", s.handleWealthHome)
	s.mux.HandleFunc("GET /api/wealth/fx", s.handleWealthFX)
	s.mux.HandleFunc("POST /api/wealth/assets", s.requireWritable(s.requireAuth(s.handleWealthAssetCreate)))
	s.mux.HandleFunc("POST /api/wealth/assets/snapshot", s.requireWritable(s.requireAuth(s.handleWealthAssetSnapshot)))
	s.mux.HandleFunc("POST /api/wealth/assets/snapshot/delete", s.requireWritable(s.requireAuth(s.handleWealthAssetSnapshotDelete)))
	s.mux.HandleFunc("GET /api/wealth/assets/history", s.handleWealthAssetHistory)
	s.mux.HandleFunc("POST /api/wealth/assets/archive", s.requireWritable(s.requireAuth(s.handleWealthAssetArchive)))
	// unarchive/update have no UI yet (the edit-asset drawer is awaiting its
	// design) — same "API first, route stays as surface" shape as goals/earmark.
	s.mux.HandleFunc("POST /api/wealth/assets/unarchive", s.requireWritable(s.requireAuth(s.handleWealthAssetUnarchive)))
	s.mux.HandleFunc("POST /api/wealth/assets/update", s.requireWritable(s.requireAuth(s.handleWealthAssetUpdate)))
	// /api/wealth/balance (`/w/balance`, §8.3-3) — asset/liability grouping,
	// health-metric ratios, quarterly net-worth trend. Read-only: editing
	// values stays on `/w`'s asset list. /api/wealth/profile's GET is
	// likewise ungated; its POST writes profile.annual_salary, the ratios'
	// one denominator (§9.3), behind the usual write gate.
	s.mux.HandleFunc("GET /api/wealth/balance", s.handleWealthBalance)
	s.mux.HandleFunc("GET /api/wealth/debt-payoff", s.handleWealthDebtPayoff)
	s.mux.HandleFunc("GET /api/wealth/profile", s.handleWealthProfileGet)
	s.mux.HandleFunc("POST /api/wealth/profile", s.requireWritable(s.requireAuth(s.handleWealthProfileUpdate)))
	// /api/wealth/import (Phase 9 波次1 PR3', §8.15.1) — CSV paste import for
	// initial data entry, same dryRun-preview/apply shape and write gate as
	// /api/import's trade CSV.
	s.mux.HandleFunc("POST /api/wealth/import", s.requireWritable(s.requireAuth(s.handleWealthImport)))
	// /api/wealth/alloc (`/w/alloc`, §9.4 PR4) — target model, rebalance
	// orders, locked (illiquid) groups, risk share, currency exposure, and
	// single-position concentration warnings. Read-only (no write path —
	// orders are suggestions, not executed trades), ungated like the rest.
	s.mux.HandleFunc("GET /api/wealth/alloc", s.handleWealthAlloc)
	// /api/wealth/cash (`/w/cash`, §9.4 PR5) — recurring income/expense
	// lines and the 90-day cash event table derived from them (plus open
	// option expiries). GET ungated; the two writes (add flow, pause flow)
	// share the usual requireWritable/requireAuth gate.
	s.mux.HandleFunc("GET /api/wealth/cash", s.handleWealthCashList)
	s.mux.HandleFunc("POST /api/wealth/cash", s.requireWritable(s.requireAuth(s.handleWealthCashCreate)))
	s.mux.HandleFunc("POST /api/wealth/cash/deactivate", s.requireWritable(s.requireAuth(s.handleWealthCashDeactivate)))
	s.mux.HandleFunc("POST /api/wealth/cash/resume", s.requireWritable(s.requireAuth(s.handleWealthCashResume)))
	s.mux.HandleFunc("POST /api/wealth/cash/delete", s.requireWritable(s.requireAuth(s.handleWealthCashDelete)))
	// update has no UI (the design has no edit screen for a flow); see assets/update above.
	s.mux.HandleFunc("POST /api/wealth/cash/update", s.requireWritable(s.requireAuth(s.handleWealthCashUpdate)))
	// /api/wealth/goals (`/w/goals`, §9.4 PR6) — goal list with earmarked-
	// asset progress. The page itself is read-only (matches the design
	// template's isWGoals section, which has no write UI), but the writes
	// stay behind the usual gate as an API surface for managing goals.
	s.mux.HandleFunc("GET /api/wealth/goals", s.handleWealthGoalsList)
	s.mux.HandleFunc("POST /api/wealth/goals", s.requireWritable(s.requireAuth(s.handleWealthGoalCreate)))
	s.mux.HandleFunc("POST /api/wealth/goals/update", s.requireWritable(s.requireAuth(s.handleWealthGoalUpdate)))
	s.mux.HandleFunc("POST /api/wealth/goals/delete", s.requireWritable(s.requireAuth(s.handleWealthGoalDelete)))
	s.mux.HandleFunc("POST /api/wealth/goals/earmark", s.requireWritable(s.requireAuth(s.handleWealthGoalEarmark)))
	// /api/wealth/retire (`/w/retire`, §9.4 PR7) — real-return retirement
	// projection off the retirement-earmarked pool (kind="retirement" goal),
	// plus three §10.2② scenarios. GET ungated; the quick-switch save (age/
	// spend) behind the usual write gate — it needs the birth year set via
	// /api/wealth/profile first, since there's no calendar date to save
	// without it.
	s.mux.HandleFunc("GET /api/wealth/retire", s.handleWealthRetireGet)
	s.mux.HandleFunc("POST /api/wealth/retire", s.requireWritable(s.requireAuth(s.handleWealthRetireSave)))
	// POST /api/wealth/retire/assign — the 指定退休資產 drawer: replaces the
	// retirement goal's earmarks in one go (creating the goal when there is
	// none), capped so an asset is never counted over 100% across goals.
	s.mux.HandleFunc("POST /api/wealth/retire/assign", s.requireWritable(s.requireAuth(s.handleWealthRetireAssign)))
	// /api/wealth/insure (`/w/insure`, §9.4 PR8) — insurance gap analysis
	// (have vs a pure-function need estimate, §8.16.1). GET ungated; the
	// add-policy form behind the usual write gate.
	s.mux.HandleFunc("GET /api/wealth/insure", s.handleWealthInsureGet)
	s.mux.HandleFunc("POST /api/wealth/insure", s.requireWritable(s.requireAuth(s.handleWealthInsureCreate)))
	// /api/wealth/funds (`/w/funds`, §9.4 PR10) — DCA/fund holdings' KPI row,
	// 24-month cost-vs-value chart, upcoming contribution schedule, and
	// per-fund performance table. Read-only, GET only — the page has no add
	// form (fund rows only ever arrive via /w/import's CSV path).
	s.mux.HandleFunc("GET /api/wealth/funds", s.handleWealthFundsGet)
	// /api/wealth/assistant (Phase 28) — the chat itself is a write (it
	// stores the conversation and runs the model), so it carries the same
	// gate as every other wealth write.
	s.mux.HandleFunc("GET /api/wealth/assistant", s.handleWealthAssistantGet)
	s.mux.HandleFunc("POST /api/wealth/assistant/chat", s.requireWritable(s.requireAuth(s.requireAssistant(s.handleWealthAssistantChat))))
	s.mux.HandleFunc("POST /api/wealth/assistant/new", s.requireWritable(s.requireAuth(s.requireAssistant(s.handleWealthAssistantNew))))
	s.mux.HandleFunc("POST /api/wealth/assistant/drafts/update", s.requireWritable(s.requireAuth(s.requireAssistant(s.handleWealthDraftUpdate))))
	s.mux.HandleFunc("POST /api/wealth/assistant/drafts/delete", s.requireWritable(s.requireAuth(s.requireAssistant(s.handleWealthDraftDelete))))
	s.mux.HandleFunc("POST /api/wealth/assistant/drafts/clear", s.requireWritable(s.requireAuth(s.requireAssistant(s.handleWealthDraftClear))))
	s.mux.HandleFunc("POST /api/wealth/assistant/apply", s.requireWritable(s.requireAuth(s.requireAssistant(s.handleWealthDraftApply))))
}

// registerLLMRoutes: Phase 19's /llm page (§4.4/§5.3). See Config.LLMAudit's
// doc comment for why this is the one feature gated by skipping registration
// entirely rather than 404ing/degrading at request time.
func (s *Server) registerLLMRoutes() {
	if !s.llmAudit {
		return
	}
	s.mux.HandleFunc("GET /api/llm-runs", s.handleLLMRuns)
	s.mux.HandleFunc("GET /api/llm-runs/{id}", s.handleLLMRunDetail)
	s.mux.HandleFunc("GET /api/news-sources/blocked", s.handleBlockedNewsSources)
	s.mux.HandleFunc("POST /api/news-sources/block", s.requireWritable(s.requireAuth(s.handleNewsSourceBlock)))
	s.mux.HandleFunc("POST /api/news-sources/unblock", s.requireWritable(s.requireAuth(s.handleNewsSourceUnblock)))
}

// registerSystemRoutes: login and the .env settings editor.
func (s *Server) registerSystemRoutes() {
	// Write routes (Phase 10) are always registered, but requireWritable
	// 404s every one of them when no password is configured (see Config.
	// Password's doc comment) — registering unconditionally, rather than
	// skipping registration, is what makes that a real 404 instead of
	// falling through to spaHandler's SPA-fallback 200. See
	// docs/phase-10-web-trade-input.md §4.1.
	s.mux.HandleFunc("POST /api/login", s.requireWritable(s.handleLogin))
	// /api/settings (Phase 17 PR2) sits behind the same gate as every write
	// route, GET included: the read side reports which credentials are
	// configured, which is not something to hand out unauthenticated.
	s.mux.HandleFunc("GET /api/settings", s.requireWritable(s.requireAuth(s.handleSettingsGet)))
	s.mux.HandleFunc("POST /api/settings", s.requireWritable(s.requireAuth(s.handleSettingsSave)))
}
