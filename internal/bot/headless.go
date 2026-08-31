package bot

import (
	"context"
	"errors"

	"argus/internal/logger"
)

// errNoChannel is what a headlessChannel returns from the two Channel
// methods whose callers need to know whether delivery actually happened.
// Silently reporting success there would be worse than an error: a pending
// action (an option assignment, a broker-synced trade) would get marked
// "sent" while no human ever saw the confirm/reject prompt, and would then
// sit unconfirmable forever. Reported as an error it stays pending, and
// gets prompted the moment a real channel is configured.
var errNoChannel = errors.New("no messaging channel configured")

// headlessChannel is the Channel a Bot runs on when no messaging transport
// is configured (Phase 24 Stage 3's "Telegram Bot 降級為外掛適配器", taken
// literally). It exists so that the *absence* of Telegram costs only the
// transport and not the orchestration hanging off it — before this, a
// Telegram-less process left internal/app's a.Bot nil, and with it every
// scheduler job, so a cmd/server deployment wrote no daily_snapshots and
// the dashboard's whole P&L curve replayed nothing.
//
// Not the same thing as a "silent mode": alerts published through the
// notification.Dispatcher (b.publishAlert) still reach the in-app store and
// the WebSocket hub. What's dropped here is only what a command reply or a
// job's Telegram-shaped summary text would have printed — content the
// dashboard renders from the DB anyway.
type headlessChannel struct{}

// Listen blocks until ctx is done without ever delivering an Update: there
// is no inbound side to a headless channel. Bot.Run isn't started at all in
// this state (internal/app's Run gates it on Telegram being live), so this
// is belt-and-braces for a caller that starts it anyway.
func (headlessChannel) Listen(ctx context.Context, _ func(Update)) { <-ctx.Done() }

func (headlessChannel) Send(text string) { logger.Debugf("headless: dropped message: %s", text) }

func (headlessChannel) SendWithButtons(string, []Button) error { return errNoChannel }

func (headlessChannel) EditMessage(MessageRef, string) error { return errNoChannel }

func (headlessChannel) AnswerCallback(string) {}

// NewHeadless builds a Bot with no messaging transport — the same wiring as
// New, minus the Telegram channel it can't construct without a token. Every
// scheduler job, the web dashboard's TradeExecutor seam and SyncUniverse go
// on working; only inbound commands and outbound chat text are gone.
//
// cfg.Token/cfg.ChatID are ignored rather than validated: the caller
// (internal/app's Boot) has already decided this is the headless case,
// either because they're unset or because Telegram itself rejected them.
func NewHeadless(cfg Config) *Bot { return NewWithChannel(headlessChannel{}, cfg) }
