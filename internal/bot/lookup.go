package bot

import (
	"strings"

	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/render"
)

// eventsListLimit/eventsTickerLimit cap /events' two forms — starting-point
// constants per docs/phase-20-price-event-log.md §6, same "not calibrated,
// revisit if it matters" caveat as priceEventWriteupCap (closing_snapshot.go).
const (
	eventsListLimit   = 20
	eventsTickerLimit = 20
)

// handleEvents is Phase 20's /events [TICKER] — with no argument, the most
// recent price events across every ticker; with one, that ticker's full
// history. Both forms must reply even when the result is empty (same UX
// lesson as /recommend's empty-result fix, see docs/phase-20-price-event-log.md
// §4.4) rather than going silent.
func (b *Bot) handleEvents(args string) {
	ticker := normalizeTicker(args)

	var events []db.PriceEvent
	var err error
	var title string
	fullSummary := ticker != ""
	if ticker == "" {
		events, err = b.db.GetRecentPriceEvents(eventsListLimit)
		title = i18n.T(b.lang, i18n.KeyEventsListTitle)
	} else {
		events, err = b.db.GetPriceEventsForTicker(ticker, eventsTickerLimit)
		title = i18n.T(b.lang, i18n.KeyEventsTickerTitle, ticker)
	}
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyQueryFailed, err))
		return
	}
	if len(events) == 0 {
		b.Send(i18n.T(b.lang, i18n.KeyEventsEmpty))
		return
	}

	var sb strings.Builder
	sb.WriteString(title)
	for _, ev := range events {
		summary := ev.Summary
		switch {
		case summary == "":
			summary = i18n.T(b.lang, i18n.KeyEventsNoSummary)
		case !fullSummary:
			// The no-argument form is a cross-ticker digest — show only the
			// summary's first line/sentence so one event doesn't push the
			// rest off the message (see handleEvents' doc comment).
			if i := strings.IndexByte(summary, '\n'); i >= 0 {
				summary = summary[:i]
			}
		}
		sb.WriteString(i18n.T(b.lang, i18n.KeyEventsLine, ev.Date, ev.Ticker, ev.GapPct, ev.ChangePct, ev.CumulativePct, summary))
	}
	b.Send(sb.String())
}

// handleFundamentals shows raw fundamentals/financial-statement data
// directly, without going through the LLM.
func (b *Bot) handleFundamentals(ticker string) {
	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	if ticker == "" {
		b.Send(i18n.T(b.lang, i18n.KeyFundamentalsUsage))
		return
	}
	if b.fundamentals == nil {
		b.Send(i18n.T(b.lang, i18n.KeyFundamentalsNoKey))
		return
	}

	fd, fdErr := b.fundamentals.GetFundamentals(ticker)
	st, stErr := b.fundamentals.GetFinancialStatements(ticker, "annual")
	if fdErr != nil && stErr != nil {
		b.Send(i18n.T(b.lang, i18n.KeyFundamentalsFailed, b.tickerLabel(ticker), fdErr))
		return
	}

	var sb strings.Builder
	sb.WriteString(i18n.T(b.lang, i18n.KeyFundamentalsTitle, b.tickerLabel(ticker)))
	if fd != nil {
		sb.WriteString(render.Fundamentals(b.lang, fd))
		sb.WriteString("\n\n")
	}
	if st != nil {
		sb.WriteString(render.FinancialStatement(b.lang, st))
	}
	b.Send(sb.String())
}
