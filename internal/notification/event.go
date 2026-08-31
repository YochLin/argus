// Package notification is Phase 24 Stage 2's event bus: the seam between
// business logic that decides something is alert-worthy (stop-loss breach,
// restricted-stock warning, price event, ...) and the channel(s) that
// actually deliver it (Telegram today; an in-app store now so a future
// Web/App surface has history to read, per docs/architecture/
// server-refactor-plan.md §Stage 2). It does not replace synchronous
// command replies (/list, /portfolio, ...) — those still go straight
// through bot.Channel.Send, since a Notifier round-trip (in-app write, a
// future WebSocket broadcast) would add nothing to a reply the user is
// already looking at.
package notification

import (
	"context"
	"time"

	"argus/internal/logger"
)

// Level is an Event's severity, coarse enough for a Notifier to decide
// whether it's worth a distinct treatment (e.g. a future push-notification
// priority) without needing to know every Type.
type Level string

const (
	LevelInfo     Level = "info"
	LevelWarning  Level = "warning"
	LevelCritical Level = "critical"
)

// Event is one notification-worthy occurrence. Type is a short slug (e.g.
// "stop_loss", "price_event") for storage/filtering; Text is the fully
// rendered message body — the same text a Telegram push has always shown,
// kept as one field rather than split into title/body so existing message
// formatting (internal/i18n templates already producing the whole text)
// doesn't need to change to feed this.
//
// ponytail: no structured Payload field yet — every Notifier today
// (TelegramNotifier, the in-app store) only needs Text. Add one when a real
// subscriber (Stage 4's WebSocket/App) needs structured data Text can't
// carry — designing that shape now, with no consumer, would be a guess.
type Event struct {
	Type  string
	Text  string
	Level Level
	Time  time.Time
}

// Notifier delivers an Event to one channel.
type Notifier interface {
	Send(ctx context.Context, e Event) error
}

// Dispatcher fans an Event out to every registered Notifier. A Notifier's
// own failure is logged, not propagated — one broken channel (e.g. the
// Telegram API down) must never stop delivery to another (e.g. the in-app
// store), matching the fire-and-forget contract bot.Channel.Send already
// documents for its callers.
type Dispatcher struct {
	notifiers []Notifier
}

// NewDispatcher builds a Dispatcher over notifiers. Callers with an optional
// Notifier (e.g. one gated on cfg.DB being set) should only append it to the
// slice when actually constructed — passing a typed-nil concrete pointer
// here would produce a non-nil Notifier interface value that still panics on
// Send, the same footgun documented on internal/app/providers.go's coreProviders.
func NewDispatcher(notifiers ...Notifier) *Dispatcher {
	return &Dispatcher{notifiers: notifiers}
}

// Register adds n to the fan-out. Boot-time only (Phase 24 Stage 3 Step 3.2:
// the Dispatcher is now constructed by internal/app before the Telegram
// adapter exists, so the adapter registers itself when it's built) — there is
// no lock here because nothing publishes until every entrypoint has finished
// wiring, and a Notifier that could come and go at runtime would need a
// different design than a slice anyway.
func (d *Dispatcher) Register(n Notifier) {
	d.notifiers = append(d.notifiers, n)
}

// Publish delivers e to every registered Notifier. e.Time defaults to now
// when unset so callers don't need to stamp every Event themselves.
func (d *Dispatcher) Publish(ctx context.Context, e Event) {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	for _, n := range d.notifiers {
		if err := n.Send(ctx, e); err != nil {
			logger.Errorf("notification: %s: %v", e.Type, err)
		}
	}
}
