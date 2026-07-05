package llm

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"argus/internal/data"
	"argus/internal/i18n"
	"argus/internal/llm/acp"
)

type StockData struct {
	Quote *data.Quote
	News  []data.NewsItem
	// Fundamentals and Statement are optional (nil when Finnhub isn't
	// configured). Statement is deliberately left unset for broad
	// multi-ticker calls (e.g. /recommend's market-mover candidates) to
	// keep prompts compact — see writeStockSection.
	Fundamentals *data.Fundamentals
	Statement    *data.FinancialStatement
}

type Recommendation struct {
	Ticker string
	Reason string
}

// Client drives Claude via the Agent Client Protocol (ACP). It runs two
// session lifecycles side by side:
//   - GenerateRecommendations/CheckStock spawn a fresh claude-agent-acp
//     process per call and close it once the reply arrives — right for
//     one-shot analysis where there's nothing to remember between calls.
//   - Chat keeps a single ACP session open across calls so the agent
//     retains conversation history, for free-form back-and-forth with the
//     user rather than a single analysis request.
//
// Both authenticate through the operator's existing `claude` CLI login
// (Claude Pro/Max subscription) instead of a metered Anthropic API key.
type Client struct {
	recommendModel string
	checkModel     string
	chatModel      string
	lang           i18n.Lang

	chatMu      sync.Mutex
	chatSession *acp.Session // lazily started; nil until the first Chat call
}

// NewClient builds an LLM client. recommendModel, checkModel, and chatModel
// are Claude model aliases/IDs (e.g. "opus", "sonnet") used for /recommend,
// /check, and free-form chat respectively; an empty string uses the ACP
// agent's own default model. lang controls both the language Claude is
// instructed to answer in and the language of the structural markers
// GenerateRecommendations' output parser looks for (see KeyReasonMarker) —
// it isn't just cosmetic, changing it changes what parseRecommendations must
// match.
func NewClient(recommendModel, checkModel, chatModel string, lang i18n.Lang) *Client {
	return &Client{recommendModel: recommendModel, checkModel: checkModel, chatModel: chatModel, lang: lang}
}

// GenerateRecommendations analyzes watchlist stocks + broad market candidates
// and returns 3–5 recommendations with explanations in the client's
// configured language (c.lang).
func (c *Client) GenerateRecommendations(ctx context.Context, watchlist []StockData, candidates []StockData) ([]Recommendation, error) {
	prompt := buildRecommendationPrompt(c.lang, watchlist, candidates)
	raw, err := c.prompt(ctx, prompt, c.recommendModel)
	if err != nil {
		return nil, err
	}
	return parseRecommendations(c.lang, raw), nil
}

// CheckStock performs instant analysis of a single ticker.
func (c *Client) CheckStock(ctx context.Context, stock StockData) (string, error) {
	prompt := buildCheckPrompt(c.lang, stock)
	return c.prompt(ctx, prompt, c.checkModel)
}

// Chat sends text on the client's persistent ACP session, starting one on
// the first call. Unlike GenerateRecommendations/CheckStock, the session
// stays open across calls so the agent remembers earlier turns.
func (c *Client) Chat(ctx context.Context, text string) (string, error) {
	c.chatMu.Lock()
	defer c.chatMu.Unlock()

	if c.chatSession == nil {
		session, err := acp.StartSession(ctx, os.TempDir(), i18n.T(c.lang, i18n.KeySystemPromptChat), c.chatModel)
		if err != nil {
			return "", fmt.Errorf("acp: %w", err)
		}
		c.chatSession = session
	}

	reply, err := c.chatSession.Prompt(ctx, text)
	if err != nil {
		// The underlying process is presumably dead; drop it so the next
		// call starts a fresh one instead of repeating the same error.
		c.chatSession.Close()
		c.chatSession = nil
		return "", fmt.Errorf("acp: %w", err)
	}
	return strings.TrimSpace(reply), nil
}

// ResetChat closes the persistent chat session. The next Chat call opens a
// new one with no memory of earlier turns.
func (c *Client) ResetChat() {
	c.chatMu.Lock()
	defer c.chatMu.Unlock()
	if c.chatSession != nil {
		c.chatSession.Close()
		c.chatSession = nil
	}
}

// Close shuts down any open persistent session. Call once on bot shutdown —
// without it, an open chat session's claude-agent-acp process would be
// orphaned rather than exiting with the bot.
func (c *Client) Close() {
	c.ResetChat()
}

func (c *Client) prompt(ctx context.Context, prompt, model string) (string, error) {
	// Run from a neutral directory so the agent doesn't pick up this repo's
	// own CLAUDE.md/skills/settings while answering a stock-analysis prompt.
	session, err := acp.StartSession(ctx, os.TempDir(), i18n.T(c.lang, i18n.KeySystemPromptAnalyst), model)
	if err != nil {
		return "", fmt.Errorf("acp: %w", err)
	}
	defer session.Close()

	reply, err := session.Prompt(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("acp: %w", err)
	}
	return strings.TrimSpace(reply), nil
}

func buildRecommendationPrompt(lang i18n.Lang, watchlist []StockData, candidates []StockData) string {
	var sb strings.Builder

	sb.WriteString(i18n.T(lang, i18n.KeyRecPromptIntro))
	sb.WriteString(i18n.T(lang, i18n.KeyRecWatchlistHeader))

	if len(watchlist) == 0 {
		sb.WriteString(i18n.T(lang, i18n.KeyRecNoWatchlist))
	} else {
		for _, s := range watchlist {
			writeStockSection(&sb, lang, s)
		}
	}

	sb.WriteString(i18n.T(lang, i18n.KeyRecMoversHeader))
	if len(candidates) == 0 {
		sb.WriteString(i18n.T(lang, i18n.KeyRecNoCandidates))
	} else {
		for _, s := range candidates {
			writeStockSection(&sb, lang, s)
		}
	}

	marker := i18n.T(lang, i18n.KeyReasonMarker)
	sb.WriteString(i18n.T(lang, i18n.KeyRecTaskBlock, marker, marker))
	return sb.String()
}

func writeStockSection(sb *strings.Builder, lang i18n.Lang, s StockData) {
	q := s.Quote
	if q == nil {
		return
	}
	fmt.Fprint(sb, i18n.T(lang, i18n.KeyStockHeader, q.Ticker))
	fmt.Fprint(sb, i18n.T(lang, i18n.KeyPriceLine, q.Price, q.ChangePercent))
	fmt.Fprint(sb, i18n.T(lang, i18n.KeyOHLLine, q.Open, q.High, q.Low))
	fmt.Fprint(sb, i18n.T(lang, i18n.KeyVolumeLine, q.Volume, q.PrevClose))
	fmt.Fprint(sb, i18n.T(lang, i18n.KeyQuoteTimeLine, q.Timestamp.In(time.FixedZone("CST", 8*3600)).Format("2006-01-02 15:04")))

	if len(s.News) > 0 {
		sb.WriteString(i18n.T(lang, i18n.KeyNewsHeader))
		for i, n := range s.News {
			if i >= 5 {
				break
			}
			fmt.Fprint(sb, i18n.T(lang, i18n.KeyNewsItem, i+1, n.Source, n.Headline))
		}
	}

	if fd := s.Fundamentals; fd != nil {
		fmt.Fprint(sb, i18n.T(lang, i18n.KeyFundamentalsSummaryLine,
			fd.PE, fd.PB, fd.ROE, fd.GrossMarginPct, fd.OperatingMarginPct, fd.NetMarginPct,
			fd.DebtToEquity, fd.RevenueGrowthYoY, fd.EPSGrowthYoY, fd.DividendYieldPct, fd.Beta))
	}

	if st := s.Statement; st != nil {
		fmt.Fprint(sb, i18n.T(lang, i18n.KeyStatementSummaryLine,
			st.Form, st.FiscalYear, st.PeriodEnd,
			st.Revenue/1e6, st.GrossProfit/1e6, st.OperatingIncome/1e6, st.NetIncome/1e6,
			st.TotalAssets/1e6, st.TotalLiabilities/1e6, st.TotalEquity/1e6, st.OperatingCashFlow/1e6, st.FreeCashFlow/1e6))
	}

	sb.WriteString("\n")
}

func buildCheckPrompt(lang i18n.Lang, s StockData) string {
	var sb strings.Builder
	sb.WriteString(i18n.T(lang, i18n.KeyCheckPromptIntro))
	writeStockSection(&sb, lang, s)
	sb.WriteString(i18n.T(lang, i18n.KeyCheckPromptTask))
	return sb.String()
}

// parseRecommendations parses the structured LLM output into Recommendation
// slices. Expected format:
//
//	[TICKER: AAPL]
//	<reason marker>: ...
//
// The reason-line marker is language-dependent (i18n.KeyReasonMarker — "原因:"
// for zh, "Reason:" for en) because buildRecommendationPrompt asks Claude to
// use that same marker in its reply; the two must stay in lockstep, which is
// why both read from the same i18n key instead of each hardcoding a prefix.
func parseRecommendations(lang i18n.Lang, raw string) []Recommendation {
	reasonPrefix := i18n.T(lang, i18n.KeyReasonMarker)
	var recs []Recommendation
	lines := strings.Split(raw, "\n")
	var currentTicker string
	var reasonParts []string

	flush := func() {
		if currentTicker != "" {
			recs = append(recs, Recommendation{
				Ticker: currentTicker,
				Reason: strings.TrimSpace(strings.Join(reasonParts, " ")),
			})
		}
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[TICKER:") && strings.HasSuffix(line, "]") {
			flush()
			ticker := strings.TrimSuffix(strings.TrimPrefix(line, "[TICKER:"), "]")
			currentTicker = strings.TrimSpace(ticker)
			reasonParts = nil
			continue
		}
		if strings.HasPrefix(line, reasonPrefix) {
			reason := strings.TrimPrefix(line, reasonPrefix)
			reasonParts = append(reasonParts, strings.TrimSpace(reason))
			continue
		}
		if currentTicker != "" && len(reasonParts) > 0 && line != "" {
			reasonParts = append(reasonParts, line)
		}
	}
	flush()
	return recs
}
