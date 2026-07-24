package bot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"unicode/utf8"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// telegramChannel is the only Channel implementation today — every direct
// use of tgbotapi in this package lives in this file (see channel.go's doc
// comment). It holds the one fixed chatID this single-user bot talks to,
// both as the target for every outbound Send/SendWithButtons and as the
// inbound filter for callback queries (see Listen).
type telegramChannel struct {
	api    *tgbotapi.BotAPI
	chatID int64
}

// NewTelegramChannel authenticates against the Telegram Bot API (or, in
// tests, apiEndpoint's fake HTTP server — see daily_report_e2e_test.go)
// and returns a Channel ready for New/NewWithChannel to hand to a Bot.
func NewTelegramChannel(token, apiEndpoint string, chatID int64) (*telegramChannel, error) {
	var api *tgbotapi.BotAPI
	var err error
	if apiEndpoint != "" {
		api, err = tgbotapi.NewBotAPIWithAPIEndpoint(token, apiEndpoint)
	} else {
		api, err = tgbotapi.NewBotAPI(token)
	}
	if err != nil {
		return nil, fmt.Errorf("telegram: %w", err)
	}
	log.Printf("Telegram bot authorized: @%s", api.Self.UserName)
	return &telegramChannel{api: api, chatID: chatID}, nil
}

// telegramMaxMessageLen is a conservative cap on outgoing message length.
// Telegram's actual sendMessage limit is 4096 characters; this project stays
// well under it since splitMessage counts runes (not the UTF-16 code units
// Telegram's limit is really specified in — astral-plane emoji like 📊 need
// two of those per rune) and BotAPI.Send returns "message is too long" as a
// plain error that Send only logs, never surfaces to the user. /track is the
// command most likely to hit this: its length grows with
// watchlist-size × lookback-days (see handleTrack), so even a modest
// watchlist can produce a multi-thousand-character report after a week of
// daily reports.
const telegramMaxMessageLen = 3500

func (c *telegramChannel) Send(text string) {
	for _, chunk := range splitMessage(text, telegramMaxMessageLen) {
		msg := tgbotapi.NewMessage(c.chatID, chunk)
		msg.ParseMode = "Markdown"
		if _, err := c.api.Send(msg); err != nil {
			log.Printf("send error: %v", err)
		}
	}
}

// SendWithButtons bypasses Send's chunking helper — every caller here is a
// single short line, never long enough to need splitting, and a chunked
// message would leave the buttons attached to just the last fragment.
func (c *telegramChannel) SendWithButtons(text string, buttons []Button) error {
	row := make([]tgbotapi.InlineKeyboardButton, len(buttons))
	for i, b := range buttons {
		row[i] = tgbotapi.NewInlineKeyboardButtonData(b.Label, b.Data)
	}
	msg := tgbotapi.NewMessage(c.chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(row)
	_, err := c.api.Send(msg)
	return err
}

func (c *telegramChannel) EditMessage(ref MessageRef, text string) error {
	edit := tgbotapi.NewEditMessageText(ref.telegramChatID, ref.telegramMessageID, text)
	_, err := c.api.Send(edit)
	return err
}

func (c *telegramChannel) AnswerCallback(id string) {
	if _, err := c.api.Request(tgbotapi.NewCallback(id, "")); err != nil {
		log.Printf("callback query: answer: %v", err)
	}
}

// Listen long-polls Telegram and translates each raw tgbotapi.Update into
// this package's channel-agnostic Update before calling handle — bot.go's
// Run doesn't know tgbotapi's types at all.
func (c *telegramChannel) Listen(ctx context.Context, handle func(Update)) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := c.api.GetUpdatesChan(u)

	for {
		select {
		case <-ctx.Done():
			return
		case update, ok := <-updates:
			if !ok {
				return
			}
			if update.CallbackQuery != nil {
				cq := update.CallbackQuery
				if cq.Message == nil || cq.Message.Chat == nil || cq.Message.Chat.ID != c.chatID {
					continue
				}
				handle(Update{Callback: &InCallback{
					ID:   cq.ID,
					Data: cq.Data,
					MsgRef: MessageRef{
						telegramChatID:    cq.Message.Chat.ID,
						telegramMessageID: cq.Message.MessageID,
					},
				}})
				continue
			}
			if update.Message == nil {
				continue
			}
			handle(Update{Message: &InMessage{Text: update.Message.Text}})
		}
	}
}

// splitMessage breaks text into chunks of at most limit runes, splitting
// only at line boundaries so a Markdown entity opened and closed within a
// single line (e.g. "*AAPL*") never gets split across two messages — every
// i18n line template in this package opens and closes its own markdown
// within one line, so this preserves valid Markdown per chunk. A single line
// longer than limit on its own (shouldn't happen with today's templates) is
// hard-split by rune as a last resort, so content is never silently dropped.
func splitMessage(text string, limit int) []string {
	if utf8.RuneCountInString(text) <= limit {
		return []string{text}
	}

	var chunks []string
	var current strings.Builder
	currentLen := 0
	flush := func() {
		if current.Len() > 0 {
			chunks = append(chunks, current.String())
			current.Reset()
			currentLen = 0
		}
	}

	for _, line := range strings.SplitAfter(text, "\n") {
		if line == "" {
			continue
		}
		lineLen := utf8.RuneCountInString(line)
		if lineLen > limit {
			flush()
			runes := []rune(line)
			for len(runes) > 0 {
				n := limit
				if n > len(runes) {
					n = len(runes)
				}
				chunks = append(chunks, string(runes[:n]))
				runes = runes[n:]
			}
			continue
		}
		if currentLen+lineLen > limit {
			flush()
		}
		current.WriteString(line)
		currentLen += lineLen
	}
	flush()
	return chunks
}
