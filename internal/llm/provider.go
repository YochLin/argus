package llm

import "context"

// Provider is the seam between internal/llm's prompt building/response
// parsing and the underlying LLM backend's session lifecycle. Client talks
// to exactly one Provider today (acpProvider, driving Claude via ACP) — the
// interface exists so a second backend can be added later as a fallback
// (see PLAN.md's "LLM provider 介面 + Google 備援" architecture-debt item)
// without changing GenerateRecommendations/CheckStock/Chat or their callers.
//
// It covers both session lifecycles Client needs: a one-shot Prompt for
// GenerateRecommendations/CheckStock (nothing to remember between calls),
// and a persistent NewChatSession for Chat's free-form back-and-forth (the
// session keeps conversation history across calls until Close).
type Provider interface {
	// Prompt runs a single turn: start a session, send text, return the
	// reply, tear the session down.
	Prompt(ctx context.Context, systemPrompt, model, text string) (string, error)

	// NewChatSession starts a persistent multi-turn session.
	NewChatSession(ctx context.Context, systemPrompt, model string) (ChatSession, error)
}

// ChatSession is one persistent multi-turn conversation against a Provider.
type ChatSession interface {
	// Send sends one user turn and returns the reply. The session retains
	// conversation history across calls until Close.
	Send(ctx context.Context, text string) (string, error)
	// Close ends the session and releases its underlying resources (e.g. the
	// backing agent subprocess).
	Close() error
}
