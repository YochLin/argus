package bot

import (
	"context"

	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/llm"
	"argus/internal/logger"
	"argus/internal/webfetch"
)

// handleChat replies to a plain-text (non-command) message using the LLM
// client's persistent session, so the agent remembers earlier turns in this
// conversation — unlike /recommend and /check, which are one-shot analysis
// calls with no memory between requests. Every message is prefixed with a
// read-only summary of the watchlist/positions (see buildChatContext) so
// free-form questions like "我自選股裡最近跌最多的是哪檔" are answerable
// without giving the ACP session any tools.
func (b *Bot) handleChat(ctx context.Context, text string) {
	if url, ok := webfetch.ExtractURL(text); ok {
		b.handleChatArticle(ctx, text, url)
		return
	}

	b.Send(i18n.T(b.lang, i18n.KeyThinking))

	if ctxBlock := b.buildChatContext(); ctxBlock != "" {
		text = ctxBlock + text
	}

	reply, _, err := b.llm.Chat(ctx, llm.ChatTelegram, text)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyChatFailed, err))
		return
	}
	b.Send(reply)
	b.sendPendingActionPrompts()
}

// handleChatArticle is handleChat's "article digestion" path (Phase 3): the
// user pasted a URL, possibly alongside their own comment/question in text.
// The page is fetched and its text extracted bot-side (WebFetch in the chat
// session only returns a summary — see internal/llm's acp_provider.go), then
// wrapped in KeyArticleTaskBlock and sent through the same persistent chat
// session as an ordinary message, so the digestion happens inline in the
// conversation rather than as a separate one-shot analysis call. A fetch
// failure (dead link, paywall, JS-only page — see webfetch.Fetch) forwards
// the user's message unchanged instead: the chat session's WebFetch gets a
// second try at the link, as a summary rather than the full text.
func (b *Bot) handleChatArticle(ctx context.Context, text, url string) {
	b.Send(i18n.T(b.lang, i18n.KeyFetchingArticle))

	prompt := text
	article, err := webfetch.Fetch(ctx, url)
	if err != nil {
		// Send the message, URL included, as it is: the chat session has
		// WebFetch for exactly this case, and says so if that fails too.
		logger.Warnf("chat: article fetch %s: %v; leaving it to the session's WebFetch", url, err)
	} else {
		prompt = i18n.T(b.lang, i18n.KeyArticleTaskBlock, article.Title, url, article.Text, text)
	}

	b.Send(i18n.T(b.lang, i18n.KeyThinking))

	if ctxBlock := b.buildChatContext(); ctxBlock != "" {
		prompt = ctxBlock + prompt
	}

	reply, _, err := b.llm.Chat(ctx, llm.ChatTelegram, prompt)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyChatFailed, err))
		return
	}
	b.Send(reply)
	b.sendPendingActionPrompts()
}

// buildChatContext composes formatChatContext's input from the DB: the
// union of watchlist and position tickers (a position ticker should always
// already be on the watchlist via /buy's auto-add, but this covers a
// position added before that existed), each one's latest closing snapshot,
// and every open position. It deliberately reads local snapshots instead of
// fetching live quotes — adding a round of network calls to every chat
// message would make free-form chat feel sluggish, and "as of last close"
// is a fine trade for conversational context (use /status or /portfolio for
// real-time prices). Returns "" if there's nothing to show.
func (b *Bot) buildChatContext() string {
	watchlist, err := b.db.GetWatchlist()
	if err != nil {
		logger.Errorf("chat context: watchlist: %v", err)
	}
	positions, err := b.db.GetPositions()
	if err != nil {
		logger.Errorf("chat context: positions: %v", err)
	}

	tickerSet := make(map[string]bool, len(watchlist))
	tickers := make([]string, 0, len(watchlist)+len(positions))
	for _, t := range watchlist {
		tickerSet[t] = true
		tickers = append(tickers, t)
	}
	posByTicker := make(map[string]db.Position, len(positions))
	for _, p := range positions {
		posByTicker[p.Ticker] = p
		if !tickerSet[p.Ticker] {
			tickerSet[p.Ticker] = true
			tickers = append(tickers, p.Ticker)
		}
	}

	snapshots := make(map[string]db.DailySnapshot, len(tickers))
	for _, t := range tickers {
		snap, ok, err := b.db.GetLatestSnapshot(t)
		if err != nil {
			logger.Errorf("chat context: snapshot %s: %v", t, err)
			continue
		}
		if ok {
			snapshots[t] = snap
		}
	}

	return formatChatContext(b.lang, tickers, posByTicker, snapshots)
}

// handleReset clears the persistent chat session so the next message starts
// a fresh conversation with no memory of earlier turns.
func (b *Bot) handleReset() {
	b.llm.ResetChat(llm.ChatTelegram)
	b.Send(i18n.T(b.lang, i18n.KeyResetDone))
}
