package bot

import (
	"context"
	"errors"
	"strings"

	"argus/internal/i18n"
	"argus/internal/logger"
	"argus/internal/service"
)

func (b *Bot) handleAdd(ticker string) {
	watchlist := b.watchlists()
	if watchlist == nil {
		b.Send(i18n.T(b.lang, i18n.KeyAddFailed, errors.New("watchlist service unavailable")))
		return
	}
	normalized, err := watchlist.Add(ticker)
	if errors.Is(err, service.ErrInvalidTicker) {
		b.Send(i18n.T(b.lang, i18n.KeyAddUsage))
		return
	}
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyAddFailed, err))
		return
	}
	b.Send(i18n.T(b.lang, i18n.KeyAddSuccess, b.tickerLabel(normalized)))
}

func (b *Bot) handleRemove(ticker string) {
	watchlist := b.watchlists()
	if watchlist == nil {
		b.Send(i18n.T(b.lang, i18n.KeyRemoveFailed, errors.New("watchlist service unavailable")))
		return
	}
	normalized, err := watchlist.Remove(ticker)
	if errors.Is(err, service.ErrInvalidTicker) {
		b.Send(i18n.T(b.lang, i18n.KeyRemoveUsage))
		return
	}
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyRemoveFailed, err))
		return
	}
	b.Send(i18n.T(b.lang, i18n.KeyRemoveSuccess, b.tickerLabel(normalized)))
}

func (b *Bot) handleList() {
	watchlist := b.watchlists()
	if watchlist == nil {
		b.Send(i18n.T(b.lang, i18n.KeyQueryFailed, errors.New("watchlist service unavailable")))
		return
	}
	tickers, err := watchlist.GetWatchlist()
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyQueryFailed, err))
		return
	}
	if len(tickers) == 0 {
		b.Send(i18n.T(b.lang, i18n.KeyWatchlistEmptyHint))
		return
	}
	labels := make([]string, len(tickers))
	for i, t := range tickers {
		labels[i] = b.tickerLabel(t)
	}
	b.Send(i18n.T(b.lang, i18n.KeyWatchlistTitle) + strings.Join(labels, "\n"))
}

func normalizeTicker(raw string) string {
	t, _ := service.NormalizeTicker(raw)
	return t
}

func (b *Bot) handleStatus(ticker string) {
	ticker = normalizeTicker(ticker)
	if ticker == "" {
		watchlist := b.watchlists()
		if watchlist == nil {
			b.Send(i18n.T(b.lang, i18n.KeyQueryFailed, errors.New("watchlist service unavailable")))
			return
		}
		tickers, err := watchlist.GetWatchlist()
		if err != nil {
			b.Send(i18n.T(b.lang, i18n.KeyQueryFailed, err))
			return
		}
		if len(tickers) == 0 {
			b.Send(i18n.T(b.lang, i18n.KeyWatchlistEmpty))
			return
		}
		var sb strings.Builder
		sb.WriteString(i18n.T(b.lang, i18n.KeyMarketStatusTitle))
		for _, t := range tickers {
			q, err := b.provider.GetQuote(t)
			if err != nil {
				sb.WriteString(i18n.T(b.lang, i18n.KeyQuoteUnavailable, b.tickerLabel(t)))
				continue
			}
			sb.WriteString(formatQuote(b.lang, q, b.tickerLabel(t)))
			sb.WriteString("\n\n")
		}
		b.Send(sb.String())
		return
	}

	q, err := b.provider.GetQuote(ticker)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyQuoteFailed, b.tickerLabel(ticker), err))
		return
	}
	b.Send(formatQuote(b.lang, q, b.tickerLabel(ticker)))
}

// handleUniverse manages Phase 2.6's candidate scan pool. With no
// arguments it shows a count summary (never the full ~500-ticker list,
// which would blow past Telegram's message size for no reason); "add"/
// "remove" manage the manual tier — the S&P 500 seed tier is read-only
// from here (see db.seedSP500).
func (b *Bot) handleUniverse(args string) {
	fields := strings.Fields(args)
	if len(fields) == 0 {
		b.sendUniverseSummary()
		return
	}

	sub := strings.ToLower(fields[0])
	ticker := ""
	if len(fields) > 1 {
		ticker = normalizeTicker(fields[1])
	}

	switch sub {
	case "add":
		if ticker == "" {
			b.Send(i18n.T(b.lang, i18n.KeyUniverseAddUsage))
			return
		}
		if err := b.db.AddUniverseTicker(ticker, "manual"); err != nil {
			b.Send(i18n.T(b.lang, i18n.KeyUniverseAddFailed, ticker, err))
			return
		}
		b.Send(i18n.T(b.lang, i18n.KeyUniverseAddSuccess, ticker))
	case "remove":
		if ticker == "" {
			b.Send(i18n.T(b.lang, i18n.KeyUniverseRemoveUsage))
			return
		}
		if err := b.db.RemoveUniverseTicker(ticker); err != nil {
			b.Send(i18n.T(b.lang, i18n.KeyUniverseRemoveFailed, ticker, err))
			return
		}
		b.Send(i18n.T(b.lang, i18n.KeyUniverseRemoveSuccess, ticker))
	case "refresh":
		// The same rotation the monthly job runs, on demand — without this
		// the only way to apply a config change (or retry after the daemon
		// was down on the 1st) is to wait a month. Slow enough (five daemon
		// round trips) to warrant the placeholder every other slow handler
		// sends first.
		b.Send(i18n.T(b.lang, i18n.KeyUniverseRefreshStart))
		added, dropped, err := b.scans().RefreshTWUniverse(context.Background())
		if err != nil {
			b.Send(i18n.T(b.lang, i18n.KeyUniverseRefreshFailed, err))
			return
		}
		if added == nil && dropped == nil {
			b.Send(i18n.T(b.lang, i18n.KeyUniverseRefreshNoDaemon))
			return
		}
		b.Send(i18n.T(b.lang, i18n.KeyTWUniverseRefreshed, len(added), len(dropped), strings.Join(added, ", ")))
	default:
		b.Send(i18n.T(b.lang, i18n.KeyUniverseUsage))
	}
}

func (b *Bot) sendUniverseSummary() {
	entries, err := b.db.GetUniverse()
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyQueryFailed, err))
		return
	}

	bySource := make(map[string]int)
	for _, e := range entries {
		bySource[e.Source]++
	}

	var sb strings.Builder
	sb.WriteString(i18n.T(b.lang, i18n.KeyUniverseSummary, len(entries)))
	for _, source := range []string{"sp500", "tw", "manual"} {
		if n := bySource[source]; n > 0 {
			sb.WriteString(i18n.T(b.lang, i18n.KeyUniverseSourceLine, source, n))
		}
	}
	b.Send(sb.String())
}

// SyncUniverse is Phase 2.6 追加項's S&P 500 refresh entry point (see
// docs/phase-2.6-universe-refresh.md), called once from main() right after
// New() (after seedSP500's fresh-install path has already run, so a brand
// new DB syncs to a no-op immediately). New constituents are inserted
// silently — log-only, no Telegram — since there's nothing for the user to
// decide about a new index addition worth scanning. Constituents dropped
// from the embedded list are never auto-removed (being cut from the index
// isn't the same as delisted/dead), so they're surfaced as one consolidated
// Telegram suggestion instead; a query failure also just logs, since this
// runs unattended at startup with no command context to report an error
// against. A fully quiet run (nothing added, nothing delisted) is the
// overwhelmingly common case — the daily-scheduled deploy restarts this
// process far more often than the embedded ticker list actually changes —
// so silence here is a hard requirement, not an edge case: anything else
// would mean a near-daily Telegram notification about nothing.
func (b *Bot) SyncUniverse() {
	added, delisted, err := b.db.SyncSP500()
	if err != nil {
		logger.Errorf("sync universe: %v", err)
		return
	}
	if len(added) > 0 {
		logger.Infof("sync universe: added %d new S&P 500 ticker(s): %s", len(added), strings.Join(added, ", "))
	}
	if len(delisted) == 0 {
		return
	}

	var sb strings.Builder
	sb.WriteString(i18n.T(b.lang, i18n.KeyUniverseDelistedTitle))
	for _, t := range delisted {
		sb.WriteString(i18n.T(b.lang, i18n.KeyUniverseDelistedLine, t, t))
	}
	b.Send(sb.String())
}
