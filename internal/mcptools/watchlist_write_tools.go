package mcptools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"argus/internal/i18n"
)

// registerWriteTools adds the Phase 3.5 "watchlist 寫入工具（最低風險試點）"
// tools — add_to_watchlist/remove_from_watchlist — when ts.writeDB is
// non-nil (see db.OpenForWrites' doc comment for why this is a distinct
// connection from ts.db's hard-enforced-read-only one, not just the same
// thing with query_only left off). Watchlist membership was picked as the
// pilot specifically because it's the lowest-risk mutation in the system:
// fully reversible (add/remove is its own undo) and low-consequence (unlike
// a trade or an expense entry, a wrong watchlist entry costs nothing but a
// stray ticker in a list). It runs unconfirmed — no proposal/approval step
// — which is exactly what this pilot exists to validate before Phase 4's
// higher-stakes writes (trades, expense entries) get the same tool-call
// treatment behind a real confirmation gate.
func registerWriteTools(s *mcp.Server, ts *toolset) {
	if ts.writeDB == nil {
		return
	}

	mcp.AddTool(s, &mcp.Tool{
		Name:        "add_to_watchlist",
		Description: "Add a US stock ticker to the user's watchlist. Takes effect immediately, no confirmation step — this is a fully reversible, low-consequence action (unlike recording a trade).",
	}, ts.addToWatchlist)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "remove_from_watchlist",
		Description: "Remove a US stock ticker from the user's watchlist. Takes effect immediately, no confirmation step — this is a fully reversible, low-consequence action (unlike recording a trade).",
	}, ts.removeFromWatchlist)
}

func (ts *toolset) addToWatchlist(ctx context.Context, _ *mcp.CallToolRequest, in tickerInput) (*mcp.CallToolResult, any, error) {
	ticker := normalizeTicker(in.Ticker)
	if ticker == "" {
		return nil, nil, ts.mcpErr(i18n.KeyAddUsage)
	}
	if err := ts.writeDB.AddTicker(ticker); err != nil {
		return nil, nil, ts.mcpErr(i18n.KeyAddFailed, err)
	}
	ts.invalidateWatchlistCache()
	return textResult(i18n.T(ts.lang, i18n.KeyAddSuccess, ticker)), nil, nil
}

func (ts *toolset) removeFromWatchlist(ctx context.Context, _ *mcp.CallToolRequest, in tickerInput) (*mcp.CallToolResult, any, error) {
	ticker := normalizeTicker(in.Ticker)
	if ticker == "" {
		return nil, nil, ts.mcpErr(i18n.KeyRemoveUsage)
	}
	if err := ts.writeDB.RemoveTicker(ticker); err != nil {
		return nil, nil, ts.mcpErr(i18n.KeyRemoveFailed, err)
	}
	ts.invalidateWatchlistCache()
	return textResult(i18n.T(ts.lang, i18n.KeyRemoveSuccess, ticker)), nil, nil
}

// invalidateWatchlistCache drops get_watchlist's cached result (if any)
// right after a successful add/remove — otherwise a chat model checking the
// watchlist immediately after changing it would see a stale answer for up
// to get_watchlist's own longCacheTTL (an hour). Nil-tolerant like the rest
// of ts.cache's usage, so tests can construct a bare toolset{...} without
// wiring a cache up.
func (ts *toolset) invalidateWatchlistCache() {
	if ts.cache != nil {
		ts.cache.delete("get_watchlist")
	}
}
