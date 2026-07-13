package mcptools

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/render"
)

// cst mirrors internal/llm and internal/bot's fixed Taiwan-time zone (see
// CLAUDE.md's internal/scheduler note on why this is a FixedZone rather than
// a loaded time.Location) — quote/news timestamps are rendered in it so a
// tool result reads the same way the rest of the bot's output does.
var cst = time.FixedZone("CST", 8*3600)

const (
	// defaultNewsLimit mirrors the per-ticker news count internal/bot
	// already fetches for prompt injection (bot.go's fetchStockData).
	defaultNewsLimit = 5
	// defaultEarningsWindowDays mirrors bot.go's earningsPromptWindowDays.
	defaultEarningsWindowDays = 14

	// quoteCacheTTL is short — a quote is the one thing a chat model might
	// legitimately re-check within the same conversation and expect fresh
	// numbers (per PLAN.md's Phase 3.5 "API 防護" item: "報價 30–60 秒").
	quoteCacheTTL = 30 * time.Second
	// shortCacheTTL covers data that moves over minutes/hours, not seconds:
	// news and the market-movers list.
	shortCacheTTL = 5 * time.Minute
	// longCacheTTL covers data that's effectively static within a single
	// chat session: daily history, fundamentals, financial statements, and
	// upcoming earnings dates all change at most once a day.
	longCacheTTL = time.Hour
)

// toolset holds the read-only providers and language every tool handler
// needs. A method value (ts.getQuote, etc.) is what actually gets
// registered with mcp.AddTool — this is the only state the tools carry, no
// package-level globals. cache/limiter are nil-tolerant (withCache treats a
// nil cache/limiter as "disabled") so unit tests can construct a bare
// toolset{...} without wiring either up — see tools_test.go.
type toolset struct {
	lang         i18n.Lang
	provider     data.Provider
	history      data.HistoryProvider
	fundamentals data.FundamentalsProvider
	earnings     data.EarningsProvider
	db           *db.DB
	writeDB      *db.DB
	cache        *ttlCache
	limiter      *tokenBucket
}

// withCache is the single choke point every provider-hitting tool handler
// routes through: a cache hit skips both the rate limiter and the provider
// call entirely; a miss waits for a rate-limit token (bounding worst-case
// cadence against Finnhub's free-tier 60 req/min ceiling — see server.go)
// before calling build, and only caches a successful result so a failed
// call is retryable on the next attempt rather than sticking as a cached
// error.
func (ts *toolset) withCache(ctx context.Context, key string, ttl time.Duration, build func() (*mcp.CallToolResult, error)) (*mcp.CallToolResult, error) {
	if ts.cache != nil {
		if v, ok := ts.cache.get(key); ok {
			return v, nil
		}
	}
	if ts.limiter != nil {
		if err := ts.limiter.Wait(ctx); err != nil {
			return nil, err
		}
	}
	result, err := build()
	if err != nil {
		return nil, err
	}
	if ts.cache != nil {
		ts.cache.set(key, result, ttl)
	}
	return result, nil
}

// registerTools adds every tool this build has a provider for.
// Fundamentals/statements/earnings are Finnhub-only (see internal/data) and
// simply aren't registered when their provider is nil, so a client's
// tools/list never advertises a tool that would always fail — the same
// nil-check-and-degrade shape as Bot.fundamentals elsewhere in the project.
func registerTools(s *mcp.Server, ts *toolset) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_quote",
		Description: "Get the current/latest quote for a US stock ticker: price, change %, open/high/low, volume, previous close, and quote timestamp.",
	}, ts.getQuote)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_history",
		Description: "Get roughly 3 months of daily closing prices for a US stock ticker, oldest first — useful for eyeballing a trend or computing an indicator not already exposed as a tool.",
	}, ts.getHistory)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_news",
		Description: "Get recent news headlines for a US stock ticker, newest first.",
	}, ts.getNews)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_market_movers",
		Description: "Get a list of currently active/high-profile US stock tickers, for when the user asks about \"what's moving\" without naming a specific ticker.",
	}, ts.getMarketMovers)

	if ts.fundamentals != nil {
		mcp.AddTool(s, &mcp.Tool{
			Name:        "get_fundamentals",
			Description: "Get valuation, profitability, financial-health, and growth ratios for a US stock ticker (P/E, P/B, ROE, margins, debt/equity, revenue growth, dividend yield, etc.).",
		}, ts.getFundamentals)

		mcp.AddTool(s, &mcp.Tool{
			Name:        "get_financial_statements",
			Description: "Get the key income statement, balance sheet, and cash flow line items from a US stock ticker's most recent 10-K or 10-Q filing.",
		}, ts.getFinancialStatements)
	}

	if ts.earnings != nil {
		mcp.AddTool(s, &mcp.Tool{
			Name:        "get_upcoming_earnings",
			Description: "Get upcoming earnings report dates for a list of US stock tickers within a look-ahead window.",
		}, ts.getUpcomingEarnings)
	}

	registerDBTools(s, ts)
	registerWriteTools(s, ts)
}

type tickerInput struct {
	Ticker string `json:"ticker" jsonschema:"US stock ticker symbol, e.g. AAPL"`
}

type newsInput struct {
	Ticker string `json:"ticker" jsonschema:"US stock ticker symbol, e.g. AAPL"`
	Limit  int    `json:"limit,omitempty" jsonschema:"max number of news items to return (default 5)"`
}

type financialStatementInput struct {
	Ticker string `json:"ticker" jsonschema:"US stock ticker symbol, e.g. AAPL"`
	Freq   string `json:"freq,omitempty" jsonschema:"'annual' for the latest 10-K, or 'quarterly' for the latest 10-Q (default annual)"`
}

type earningsInput struct {
	Tickers []string `json:"tickers" jsonschema:"US stock ticker symbols to check for upcoming earnings"`
	Days    int      `json:"days,omitempty" jsonschema:"look-ahead window in days (default 14)"`
}

type emptyInput struct{}

func normalizeTicker(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

// mcpErr builds a tool error from an i18n key rather than returning a
// provider's raw Go error — those are technical/English-only regardless of
// BOT_LANGUAGE ("yahoo: no data for %s"), and the whole point of routing
// tool text through i18n is that a zh-configured bot's chat model sees zh
// error text too.
func (ts *toolset) mcpErr(key i18n.Key, args ...any) error {
	return fmt.Errorf("%s", i18n.T(ts.lang, key, args...))
}

func (ts *toolset) getQuote(ctx context.Context, _ *mcp.CallToolRequest, in tickerInput) (*mcp.CallToolResult, any, error) {
	ticker := normalizeTicker(in.Ticker)
	result, err := ts.withCache(ctx, "get_quote:"+ticker, quoteCacheTTL, func() (*mcp.CallToolResult, error) {
		q, err := ts.provider.GetQuote(ticker)
		if err != nil {
			return nil, ts.mcpErr(i18n.KeyMCPNoQuote, ticker)
		}

		var sb strings.Builder
		sb.WriteString(i18n.T(ts.lang, i18n.KeyMCPTickerHeader, ticker))
		sb.WriteString(i18n.T(ts.lang, i18n.KeyPriceLine, q.Price, q.ChangePercent))
		sb.WriteString(i18n.T(ts.lang, i18n.KeyOHLLine, q.Open, q.High, q.Low))
		sb.WriteString(i18n.T(ts.lang, i18n.KeyVolumeLine, q.Volume, q.PrevClose))
		sb.WriteString(i18n.T(ts.lang, i18n.KeyQuoteTimeLine, q.Timestamp.In(cst).Format("2006-01-02 15:04")))
		return textResult(sb.String()), nil
	})
	return result, nil, err
}

func (ts *toolset) getHistory(ctx context.Context, _ *mcp.CallToolRequest, in tickerInput) (*mcp.CallToolResult, any, error) {
	ticker := normalizeTicker(in.Ticker)
	result, err := ts.withCache(ctx, "get_history:"+ticker, longCacheTTL, func() (*mcp.CallToolResult, error) {
		closes, err := ts.history.GetHistory(ticker)
		if err != nil || len(closes) == 0 {
			return nil, ts.mcpErr(i18n.KeyMCPNoHistory, ticker)
		}

		parts := make([]string, len(closes))
		for i, c := range closes {
			parts[i] = strconv.FormatFloat(c, 'f', 2, 64)
		}
		text := i18n.T(ts.lang, i18n.KeyMCPHistoryResult, ticker, len(closes), strings.Join(parts, ", "))
		return textResult(text), nil
	})
	return result, nil, err
}

func (ts *toolset) getNews(ctx context.Context, _ *mcp.CallToolRequest, in newsInput) (*mcp.CallToolResult, any, error) {
	ticker := normalizeTicker(in.Ticker)
	limit := in.Limit
	if limit <= 0 {
		limit = defaultNewsLimit
	}
	key := fmt.Sprintf("get_news:%s:%d", ticker, limit)
	result, err := ts.withCache(ctx, key, shortCacheTTL, func() (*mcp.CallToolResult, error) {
		items, err := ts.provider.GetNews(ticker, limit)
		if err != nil || len(items) == 0 {
			return nil, ts.mcpErr(i18n.KeyMCPNoNews, ticker)
		}

		var sb strings.Builder
		sb.WriteString(i18n.T(ts.lang, i18n.KeyMCPTickerHeader, ticker))
		for i, n := range items {
			sb.WriteString(i18n.T(ts.lang, i18n.KeyMCPNewsItem, i+1, n.Source, n.Headline, n.PublishedAt.In(cst).Format("2006-01-02"), n.URL))
		}
		return textResult(sb.String()), nil
	})
	return result, nil, err
}

func (ts *toolset) getMarketMovers(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, any, error) {
	result, err := ts.withCache(ctx, "get_market_movers", shortCacheTTL, func() (*mcp.CallToolResult, error) {
		tickers, err := ts.provider.GetMarketMovers()
		if err != nil || len(tickers) == 0 {
			return nil, ts.mcpErr(i18n.KeyMCPNoMovers)
		}
		text := i18n.T(ts.lang, i18n.KeyMCPMoversResult, strings.Join(tickers, ", "))
		return textResult(text), nil
	})
	return result, nil, err
}

func (ts *toolset) getFundamentals(ctx context.Context, _ *mcp.CallToolRequest, in tickerInput) (*mcp.CallToolResult, any, error) {
	ticker := normalizeTicker(in.Ticker)
	result, err := ts.withCache(ctx, "get_fundamentals:"+ticker, longCacheTTL, func() (*mcp.CallToolResult, error) {
		fd, err := ts.fundamentals.GetFundamentals(ticker)
		if err != nil {
			return nil, ts.mcpErr(i18n.KeyMCPNoFundamentals, ticker)
		}
		return textResult(formatFundamentals(ts.lang, ticker, fd)), nil
	})
	return result, nil, err
}

func (ts *toolset) getFinancialStatements(ctx context.Context, _ *mcp.CallToolRequest, in financialStatementInput) (*mcp.CallToolResult, any, error) {
	ticker := normalizeTicker(in.Ticker)
	freq := strings.ToLower(strings.TrimSpace(in.Freq))
	if freq != "annual" && freq != "quarterly" {
		freq = "annual"
	}
	key := fmt.Sprintf("get_financial_statements:%s:%s", ticker, freq)
	result, err := ts.withCache(ctx, key, longCacheTTL, func() (*mcp.CallToolResult, error) {
		st, err := ts.fundamentals.GetFinancialStatements(ticker, freq)
		if err != nil {
			return nil, ts.mcpErr(i18n.KeyMCPNoFinancialStatements, ticker)
		}
		return textResult(formatFinancialStatement(ts.lang, ticker, st)), nil
	})
	return result, nil, err
}

func (ts *toolset) getUpcomingEarnings(ctx context.Context, _ *mcp.CallToolRequest, in earningsInput) (*mcp.CallToolResult, any, error) {
	days := in.Days
	if days <= 0 {
		days = defaultEarningsWindowDays
	}
	tickers := make([]string, len(in.Tickers))
	for i, t := range in.Tickers {
		tickers[i] = normalizeTicker(t)
	}

	key := fmt.Sprintf("get_upcoming_earnings:%s:%d", strings.Join(tickers, ","), days)
	result, err := ts.withCache(ctx, key, longCacheTTL, func() (*mcp.CallToolResult, error) {
		events, err := ts.earnings.GetUpcomingEarnings(tickers, days)
		if err != nil || len(events) == 0 {
			return nil, ts.mcpErr(i18n.KeyMCPNoEarnings, days)
		}

		var sb strings.Builder
		for _, ticker := range tickers {
			e, ok := events[ticker]
			if !ok {
				continue
			}
			sb.WriteString(i18n.T(ts.lang, i18n.KeyMCPEarningsItem, e.Ticker, e.Date, e.Hour))
		}
		return textResult(sb.String()), nil
	})
	return result, nil, err
}

// formatFundamentals renders the full Fundamentals struct, reusing
// internal/render's shared formatter (same one /fundamentals uses), with
// the ticker title this MCP tool's output needs prepended.
func formatFundamentals(lang i18n.Lang, ticker string, fd *data.Fundamentals) string {
	return i18n.T(lang, i18n.KeyFundamentalsTitle, ticker) + render.Fundamentals(lang, fd)
}

// formatFinancialStatement mirrors formatFundamentals' reuse rationale.
func formatFinancialStatement(lang i18n.Lang, ticker string, st *data.FinancialStatement) string {
	return i18n.T(lang, i18n.KeyMCPTickerHeader, ticker) + render.FinancialStatement(lang, st)
}
