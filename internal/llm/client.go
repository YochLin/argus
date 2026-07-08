package llm

import (
	"context"
	"sync"

	"argus/internal/data"
	"argus/internal/i18n"
)

// Client drives an LLM through an ordered chain of Providers (today: Claude
// via ACP first, optionally Google Antigravity as a fallback — see
// AddFallback) — the same "try each in order, fall through to the next on
// error" shape as data.Multi, just for LLM calls instead of quote/news
// lookups. It runs two session lifecycles side by side:
//   - GenerateRecommendations/CheckStock use Provider.Prompt, a fresh
//     session per call torn down once the reply arrives — right for
//     one-shot analysis where there's nothing to remember between calls.
//     Each backend in the chain is tried in order until one succeeds.
//   - Chat keeps a single Provider.ChatSession open across calls so the
//     agent retains conversation history, for free-form back-and-forth with
//     the user rather than a single analysis request. The chain is only
//     consulted when (re)starting a session — once open, later calls reuse
//     it until it errors, at which point the next call restarts from the
//     first backend again (so a fallback session that opened because Claude
//     was capped doesn't get "stuck" avoiding Claude forever). Falling back
//     mid-conversation necessarily loses whatever history the old session
//     held, since a Provider's chat memory lives inside its own session, not
//     in Client.
//
// backend.model is provider-specific vocabulary (Claude aliases like
// "opus"/"sonnet" mean nothing to a different backend), so each entry in the
// chain carries its own model strings rather than Client having one
// recommendModel/checkModel/chatModel shared across every backend.
type Client struct {
	backends []backend
	lang     i18n.Lang

	chatMu      sync.Mutex
	chatSession ChatSession // lazily started; nil until the first Chat call
}

// backend pairs a Provider with the model aliases/IDs it should use for
// each of Client's three call sites. An empty model string uses that
// Provider's own default model.
type backend struct {
	provider       Provider
	recommendModel string
	checkModel     string
	chatModel      string
}

// NewClient builds an LLM client backed by Claude via ACP. recommendModel,
// checkModel, and chatModel are Claude model aliases/IDs (e.g. "opus",
// "sonnet") used for /recommend, /check, and free-form chat respectively; an
// empty string uses the ACP agent's own default model. lang controls both
// the language Claude is instructed to answer in and the language of the
// structural markers GenerateRecommendations' output parser looks for (see
// KeyReasonMarker) — it isn't just cosmetic, changing it changes what
// parseRecommendations must match. Call AddFallback to append a second
// backend (e.g. AntigravityProvider) to try when Claude's calls fail.
func NewClient(recommendModel, checkModel, chatModel string, lang i18n.Lang) *Client {
	return &Client{
		backends: []backend{{provider: acpProvider{}, recommendModel: recommendModel, checkModel: checkModel, chatModel: chatModel}},
		lang:     lang,
	}
}

// AddFallback appends provider to the end of Client's backend chain, to be
// tried only if every earlier backend's call fails. recommendModel,
// checkModel, and chatModel are that provider's own model aliases/IDs (not
// Claude's) for /recommend, /check, and free-form chat respectively; an
// empty string uses the provider's own default model.
func (c *Client) AddFallback(provider Provider, recommendModel, checkModel, chatModel string) {
	c.backends = append(c.backends, backend{provider: provider, recommendModel: recommendModel, checkModel: checkModel, chatModel: chatModel})
}

// GenerateRecommendations analyzes watchlist stocks + broad market candidates
// and returns 3–5 recommendations with explanations in the client's
// configured language (c.lang), plus a market-news summary when marketNews is
// non-empty (empty string otherwise — e.g. Finnhub isn't configured). Both
// are extracted from the single underlying LLM reply, not two separate
// calls — see parseMarketSummary/parseRecommendations.
func (c *Client) GenerateRecommendations(ctx context.Context, watchlist []StockData, candidates []StockData, marketNews []data.NewsItem) (summary string, recs []Recommendation, err error) {
	prompt := buildRecommendationPrompt(c.lang, watchlist, candidates, marketNews)
	raw, err := c.prompt(ctx, prompt, func(b backend) string { return b.recommendModel })
	if err != nil {
		return "", nil, err
	}
	if len(marketNews) > 0 {
		summary = parseMarketSummary(raw, i18n.T(c.lang, i18n.KeyMarketSummaryMarker))
	}
	return summary, parseRecommendations(c.lang, raw), nil
}

// CheckStock performs instant analysis of a single ticker.
func (c *Client) CheckStock(ctx context.Context, stock StockData) (string, error) {
	prompt := buildCheckPrompt(c.lang, stock)
	return c.prompt(ctx, prompt, func(b backend) string { return b.checkModel })
}

// Chat sends text on the client's persistent chat session, starting one on
// the first call. Unlike GenerateRecommendations/CheckStock, the session
// stays open across calls so the agent remembers earlier turns.
func (c *Client) Chat(ctx context.Context, text string) (string, error) {
	c.chatMu.Lock()
	defer c.chatMu.Unlock()

	if c.chatSession == nil {
		session, err := c.startChatSession(ctx)
		if err != nil {
			return "", err
		}
		c.chatSession = session
	}

	reply, err := c.chatSession.Send(ctx, text)
	if err != nil {
		// The underlying session is presumably dead; drop it so the next
		// call starts a fresh one instead of repeating the same error.
		c.chatSession.Close()
		c.chatSession = nil
		return "", err
	}
	return reply, nil
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

// prompt tries each backend in the chain in order, using modelFor to pick
// that backend's own model string, and returns the first successful reply —
// same fall-through-on-error shape as data.Multi.
func (c *Client) prompt(ctx context.Context, prompt string, modelFor func(backend) string) (string, error) {
	systemPrompt := i18n.T(c.lang, i18n.KeySystemPromptAnalyst)
	var lastErr error
	for _, b := range c.backends {
		reply, err := b.provider.Prompt(ctx, systemPrompt, modelFor(b), prompt)
		if err == nil {
			return reply, nil
		}
		lastErr = err
	}
	return "", lastErr
}

// startChatSession tries each backend in the chain in order and returns the
// first ChatSession that starts successfully.
func (c *Client) startChatSession(ctx context.Context) (ChatSession, error) {
	systemPrompt := i18n.T(c.lang, i18n.KeySystemPromptChat)
	var lastErr error
	for _, b := range c.backends {
		session, err := b.provider.NewChatSession(ctx, systemPrompt, b.chatModel)
		if err == nil {
			return session, nil
		}
		lastErr = err
	}
	return nil, lastErr
}
