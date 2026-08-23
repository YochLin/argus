package notification

import "context"

// TextSender is the narrow slice of bot.Channel a TelegramNotifier needs —
// this package doesn't import internal/bot (which will come to depend on
// notification, not the reverse) so the dependency stays one-directional.
type TextSender interface {
	Send(text string)
}

// TelegramNotifier delivers an Event's Text verbatim, unchanged from what a
// direct Channel.Send call has always shown — Stage 2 adds delivery
// channels, it doesn't change what Telegram already looks like.
type TelegramNotifier struct {
	sender TextSender
}

func NewTelegramNotifier(sender TextSender) *TelegramNotifier {
	return &TelegramNotifier{sender: sender}
}

func (n *TelegramNotifier) Send(ctx context.Context, e Event) error {
	n.sender.Send(e.Text)
	return nil
}
