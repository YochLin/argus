package bot

import "context"

// Channel is this project's messaging-transport boundary — it decouples
// command dispatch (bot.go/handlers.go/pending_actions.go/quick_actions.go)
// from any specific chat platform. telegramChannel (telegram.go) is the
// only implementation today; per CLAUDE.md's 訊息通道介面 note, a future
// second channel (Discord/CLI) should get its own package implementing this
// interface rather than being bolted onto Bot directly. See
// NewWithChannel for the injection seam a second implementation (or a test)
// would use instead of New.
type Channel interface {
	// Listen blocks, delivering each inbound Update to handle, until ctx is
	// done or the underlying connection drops.
	Listen(ctx context.Context, handle func(Update))

	// Send delivers plain text, handling whatever the underlying platform
	// requires on its own (e.g. Telegram's ~4096-char message cap) — errors
	// are the implementation's own concern to log, since every call site in
	// this package already treats Send as fire-and-forget.
	Send(text string)

	// SendWithButtons delivers text with a single row of inline action
	// buttons attached — used for per-ticker quick actions and
	// pending-action confirm/reject prompts. Never called with text long
	// enough to need splitting. Unlike Send, callers need the error: it
	// decides whether a pending-action prompt actually reached the user
	// before the caller marks it "sent".
	SendWithButtons(text string, buttons []Button) error

	// EditMessage replaces a previously sent message's text in place (used
	// to turn a pending-action confirm/reject prompt into its resolved
	// outcome without leaving the original buttons visible).
	EditMessage(ref MessageRef, text string) error

	// AnswerCallback acknowledges a button tap (e.g. clears Telegram's
	// loading spinner). A no-op on a channel with no such concept.
	AnswerCallback(id string)
}

// Button is one inline action button: Label is user-facing text, Data is
// the opaque payload handed back on tap via InCallback.Data.
type Button struct {
	Label string
	Data  string
}

// MessageRef identifies a previously sent message for EditMessage. It's
// opaque outside the Channel implementation that produced it (via
// InCallback.MsgRef) — bot.go/pending_actions.go only ever thread it
// through, never inspect it. Today's only implementation (Telegram) needs a
// chat+message ID pair; if a second Channel's needs differ, this may need to
// become an interface{}-shaped value instead — deferred until there's a real
// second implementation to design against.
type MessageRef struct {
	telegramChatID    int64
	telegramMessageID int
}

// Update is a channel-agnostic inbound event: exactly one of Message or
// Callback is set.
type Update struct {
	Message  *InMessage
	Callback *InCallback
}

// InMessage is inbound text. Command parsing ("/cmd args", see
// bot.go's parseCommand) happens above this layer, so a Channel
// implementation doesn't need to know this project's command conventions.
type InMessage struct {
	Text string
}

// InCallback is an inbound button tap.
type InCallback struct {
	ID     string     // opaque id passed back to AnswerCallback
	Data   string     // the tapped button's Data
	MsgRef MessageRef // the message the button was attached to, for EditMessage
}
